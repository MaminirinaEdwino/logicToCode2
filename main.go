package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// --- 1. Structures de données du Graphe ---

type DynamicNode struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Data     map[string]any `json:"data"`
	RawNode  map[string]any `json:"raw_node"`
	Children []*DynamicNode `json:"children"`
}

type ModelField struct {
	Nom  string
	Type string
}

type ModelMeta struct {
	Name   string
	Fields []ModelField
}

type WhereFilter struct {
	Field    string
	Operator string
	Value    any
	ParamIdx int
}

type QueryContext struct {
	Model         ModelMeta
	Operation     string        // SELECT, CREATE, UPDATE, DELETE
	SelectFields  []string      // Champs pour SELECT
	WriteFields   []string      // Champs pour INSERT / UPDATE
	WhereFilters  []WhereFilter // Filtres dynamiques extrait du whereNode
	StatusCodeVar string
}

// --- 2. Construction de la hiérarchie ---

func BuildTurboStackHierarchy(nodesRawJSON string, edgesRawJSON string) ([]*DynamicNode, error) {
	var rawNodes []map[string]any
	if err := json.Unmarshal([]byte(nodesRawJSON), &rawNodes); err != nil {
		return nil, fmt.Errorf("erreur unmarshal nodes: %w", err)
	}

	type Edge struct {
		ID           string `json:"id"`
		Source       string `json:"source"`
		Target       string `json:"target"`
		SourceHandle string `json:"sourceHandle"`
		TargetHandle string `json:"targetHandle"`
	}
	var edges []Edge
	if err := json.Unmarshal([]byte(edgesRawJSON), &edges); err != nil {
		edges = []Edge{}
	}

	nodeMap := make(map[string]*DynamicNode)
	inDegree := make(map[string]int)

	for _, n := range rawNodes {
		id, _ := n["id"].(string)
		nodeType, _ := n["type"].(string)

		dataMap := make(map[string]any)
		if d, ok := n["data"].(map[string]any); ok {
			dataMap = d
		}

		nodeMap[id] = &DynamicNode{
			ID:       id,
			Type:     nodeType,
			Data:     dataMap,
			RawNode:  n,
			Children: []*DynamicNode{},
		}
		inDegree[id] = 0
	}

	for _, edge := range edges {
		parent, parentOk := nodeMap[edge.Source]
		child, childOk := nodeMap[edge.Target]

		if parentOk && childOk {
			if edge.TargetHandle == "response_status" {
				continue
			}
			parent.Children = append(parent.Children, child)
			inDegree[edge.Target]++
		}
	}

	var roots []*DynamicNode
	for id, count := range inDegree {
		if count == 0 {
			roots = append(roots, nodeMap[id])
		}
	}

	return roots, nil
}

// --- 3. Générateur de Code HTTP Native ---

type TurboStackGenerator struct {
	builder strings.Builder
	indent  int
}

func (g *TurboStackGenerator) writeIndent() {
	for i := 0; i < g.indent; i++ {
		g.builder.WriteString("\t")
	}
}

func (g *TurboStackGenerator) GenerateRoute(routePattern string, roots []*DynamicNode) string {
	g.builder.Reset()
	g.indent = 0

	g.builder.WriteString(fmt.Sprintf("mux.HandleFunc(%q, func(w http.ResponseWriter, r *http.Request) {\n", routePattern))
	g.indent++

	g.writeIndent()
	g.builder.WriteString("db := config.ConnectDB()\n")
	g.writeIndent()
	g.builder.WriteString("defer db.Close()\n\n")

	ctx := &QueryContext{
		StatusCodeVar: "http.StatusOK",
	}

	for _, root := range roots {
		g.traverseAndGenerate(root, ctx)
	}

	g.indent--
	g.writeIndent()
	g.builder.WriteString("})\n")

	return g.builder.String()
}

func (g *TurboStackGenerator) traverseAndGenerate(node *DynamicNode, ctx *QueryContext) {
	switch node.Type {

	case "rootNode":
		g.writeIndent()
		g.builder.WriteString("// Extraction des paramètres d'URL\n")
		g.writeIndent()
		g.builder.WriteString("id := r.PathValue(\"id\")\n\n")

	case "modelNode":
		// 1. Chargement des métadonnées du modèle
		if m, ok := node.Data["model"].(map[string]any); ok {
			if nom, ok := m["nom"].(string); ok {
				ctx.Model.Name = nom
			}
			if champs, ok := m["champs"].([]any); ok {
				for _, c := range champs {
					if champMap, ok := c.(map[string]any); ok {
						fNom, _ := champMap["nom"].(string)
						fType, _ := champMap["type"].(string)
						ctx.Model.Fields = append(ctx.Model.Fields, ModelField{Nom: fNom, Type: fType})
					}
				}
			}
		}

		g.writeIndent()
		g.builder.WriteString(fmt.Sprintf("// --- Modèle cible : %s ---\n", ctx.Model.Name))

		// 2. Détection de l'opération CRUD via l'enfant direct
		if len(node.Children) == 0 {
			g.writeIndent()
			g.builder.WriteString("// ERREUR : Aucun nœud d'action relié au modelNode !\n")
			return
		}

		actionNode := node.Children[0]
		switch actionNode.Type {
		case "selectNode":
			ctx.Operation = "SELECT"
		case "createNode":
			ctx.Operation = "CREATE"
		case "updateNode":
			ctx.Operation = "UPDATE"
		case "deleteNode":
			ctx.Operation = "DELETE"
		default:
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("// ERREUR : Action non supportée [%s] après modelNode\n", actionNode.Type))
		}

	case "selectNode":
		var selected []string
		for _, field := range ctx.Model.Fields {
			if val, exists := node.Data[field.Nom]; exists {
				if isSelected, ok := val.(bool); ok && isSelected {
					selected = append(selected, field.Nom)
				}
			}
		}
		if len(selected) == 0 {
			for _, field := range ctx.Model.Fields {
				selected = append(selected, field.Nom)
			}
		}
		ctx.SelectFields = selected

		g.writeIndent()
		g.builder.WriteString(fmt.Sprintf("// Action SELECT sur : [%s]\n", strings.Join(selected, ", ")))

	case "createNode":
		var writeFields []string
		for _, f := range ctx.Model.Fields {
			if f.Nom != "id" {
				writeFields = append(writeFields, f.Nom)
			}
		}
		ctx.WriteFields = writeFields
		ctx.StatusCodeVar = "http.StatusCreated"

		g.writeIndent()
		g.builder.WriteString("// Action CREATE (INSERT)\n")
		g.writeIndent()
		g.builder.WriteString("type createInput struct {\n")
		g.indent++
		for _, fName := range writeFields {
			goType := "string"
			for _, f := range ctx.Model.Fields {
				if f.Nom == fName && f.Type == "int" {
					goType = "int"
				}
			}
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("%s %s `json:%q`\n", capitalize(fName), goType, fName))
		}
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n")
		g.writeIndent()
		g.builder.WriteString("var input createInput\n")
		g.writeIndent()
		g.builder.WriteString("if err := json.NewDecoder(r.Body).Decode(&input); err != nil {\n")
		g.indent++
		g.writeIndent()
		g.builder.WriteString("http.Error(w, \"Payload invalide\", http.StatusBadRequest)\n")
		g.writeIndent()
		g.builder.WriteString("return\n")
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n\n")

	case "updateNode":
		var writeFields []string
		for _, f := range ctx.Model.Fields {
			if f.Nom != "id" {
				writeFields = append(writeFields, f.Nom)
			}
		}
		ctx.WriteFields = writeFields

		g.writeIndent()
		g.builder.WriteString("// Action UPDATE\n")
		g.writeIndent()
		g.builder.WriteString("type updateInput struct {\n")
		g.indent++
		for _, fName := range writeFields {
			goType := "string"
			for _, f := range ctx.Model.Fields {
				if f.Nom == fName && f.Type == "int" {
					goType = "int"
				}
			}
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("%s %s `json:%q`\n", capitalize(fName), goType, fName))
		}
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n")
		g.writeIndent()
		g.builder.WriteString("var input updateInput\n")
		g.writeIndent()
		g.builder.WriteString("if err := json.NewDecoder(r.Body).Decode(&input); err != nil {\n")
		g.indent++
		g.writeIndent()
		g.builder.WriteString("http.Error(w, \"Payload invalide\", http.StatusBadRequest)\n")
		g.writeIndent()
		g.builder.WriteString("return\n")
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n\n")

	case "deleteNode":
		g.writeIndent()
		g.builder.WriteString("// Action DELETE\n")

	case "whereNode":
		g.writeIndent()
		g.builder.WriteString("// Clause d'exclusion / Filtre dynamique (WHERE)\n")

		paramCounter := 1
		if ctx.Operation == "UPDATE" {
			paramCounter = len(ctx.WriteFields) + 1
		}

		for key, val := range node.Data {
			if key == "name" || key == "type" || key == "label" {
				continue
			}

			isModelField := false
			for _, f := range ctx.Model.Fields {
				if f.Nom == key {
					isModelField = true
					break
				}
			}

			if isModelField {
				op := "="
				var filterVal any = val

				if valMap, ok := val.(map[string]any); ok {
					if customOp, ok := valMap["operator"].(string); ok {
						op = customOp
					}
					if customVal, ok := valMap["value"]; ok {
						filterVal = customVal
					}
				}

				ctx.WhereFilters = append(ctx.WhereFilters, WhereFilter{
					Field:    key,
					Operator: op,
					Value:    filterVal,
					ParamIdx: paramCounter,
				})
				paramCounter++
			}
		}

		// Fallback si aucun filtre spécifique n'est défini dans node.Data
		if len(ctx.WhereFilters) == 0 {
			ctx.WhereFilters = append(ctx.WhereFilters, WhereFilter{
				Field:    "id",
				Operator: "=",
				Value:    "id",
				ParamIdx: paramCounter,
			})
		}

	case "returnNode":
		g.writeIndent()
		g.builder.WriteString("// Exécution SQL selon le mode d'opération\n")

		var whereClauses []string
		var whereArgs []string

		for _, filter := range ctx.WhereFilters {
			whereClauses = append(whereClauses, fmt.Sprintf("%s %s $%d", filter.Field, filter.Operator, filter.ParamIdx))

			if strVal, ok := filter.Value.(string); ok {
				if strVal == "id" || strings.HasPrefix(strVal, "r.") {
					whereArgs = append(whereArgs, strVal)
				} else {
					whereArgs = append(whereArgs, fmt.Sprintf("%q", strVal))
				}
			} else {
				whereArgs = append(whereArgs, fmt.Sprintf("%v", filter.Value))
			}
		}

		whereClauseSQL := ""
		if len(whereClauses) > 0 {
			whereClauseSQL = " WHERE " + strings.Join(whereClauses, " AND ")
		}

		switch ctx.Operation {

		case "SELECT":
			g.writeIndent()
			g.builder.WriteString("type returnType struct {\n")
			g.indent++
			var scanPointers []string
			for _, fieldName := range ctx.SelectFields {
				goType := "string"
				for _, f := range ctx.Model.Fields {
					if f.Nom == fieldName && f.Type == "int" {
						goType = "int"
					}
				}
				capName := capitalize(fieldName)
				g.writeIndent()
				g.builder.WriteString(fmt.Sprintf("%s %s `json:%q`\n", capName, goType, fieldName))
				scanPointers = append(scanPointers, fmt.Sprintf("&returnValue.%s", capName))
			}
			g.indent--
			g.writeIndent()
			g.builder.WriteString("}\n\n")

			g.writeIndent()
			g.builder.WriteString("var returnValue returnType\n")
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("query := \"SELECT %s FROM %s%s\"\n",
				strings.Join(ctx.SelectFields, ", "), ctx.Model.Name, whereClauseSQL))
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("err := db.QueryRow(query, %s).Scan(%s)\n",
				strings.Join(whereArgs, ", "), strings.Join(scanPointers, ", ")))

		case "CREATE":
			var placeholders []string
			var args []string
			for i, fName := range ctx.WriteFields {
				placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
				args = append(args, "input."+capitalize(fName))
			}
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("query := \"INSERT INTO %s (%s) VALUES (%s) RETURNING id\"\n",
				ctx.Model.Name, strings.Join(ctx.WriteFields, ", "), strings.Join(placeholders, ", ")))
			g.writeIndent()
			g.builder.WriteString("var newID int\n")
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("err := db.QueryRow(query, %s).Scan(&newID)\n", strings.Join(args, ", ")))

		case "UPDATE":
			var setClauses []string
			var args []string
			for i, fName := range ctx.WriteFields {
				setClauses = append(setClauses, fmt.Sprintf("%s = $%d", fName, i+1))
				args = append(args, "input."+capitalize(fName))
			}
			args = append(args, whereArgs...)

			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("query := \"UPDATE %s SET %s%s\"\n",
				ctx.Model.Name, strings.Join(setClauses, ", "), whereClauseSQL))
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("_, err := db.Exec(query, %s)\n", strings.Join(args, ", ")))

		case "DELETE":
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("query := \"DELETE FROM %s%s\"\n", ctx.Model.Name, whereClauseSQL))
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("_, err := db.Exec(query, %s)\n", strings.Join(whereArgs, ", ")))
		}

		g.writeIndent()
		g.builder.WriteString("if err != nil {\n")
		g.indent++
		g.writeIndent()
		g.builder.WriteString("http.Error(w, \"Erreur lors de l'exécution de la requête\", http.StatusInternalServerError)\n")
		g.writeIndent()
		g.builder.WriteString("return\n")
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n\n")

	case "responseNode":
		g.writeIndent()
		g.builder.WriteString(fmt.Sprintf("w.WriteHeader(%s)\n", ctx.StatusCodeVar))
		if ctx.Operation == "SELECT" {
			g.writeIndent()
			g.builder.WriteString("w.Header().Set(\"Content-Type\", \"application/json\")\n")
			g.writeIndent()
			g.builder.WriteString("json.NewEncoder(w).Encode(returnValue)\n")
		} else if ctx.Operation == "CREATE" {
			g.writeIndent()
			g.builder.WriteString("w.Header().Set(\"Content-Type\", \"application/json\")\n")
			g.writeIndent()
			g.builder.WriteString("json.NewEncoder(w).Encode(map[string]any{\"id\": newID, \"message\": \"Créé avec succès\"})\n")
		} else {
			g.writeIndent()
			g.builder.WriteString("json.NewEncoder(w).Encode(map[string]string{\"message\": \"Opération réussie\"})\n")
		}
	}

	for _, child := range node.Children {
		g.traverseAndGenerate(child, ctx)
	}
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// --- Test d'exécution ---

func main() {
	nodesJSON := `[
		{"id":"rootNode_1","type":"rootNode","data":{"name":"rootNode"}},
		{"id":"modelNode_1","type":"modelNode","data":{"model":{"nom":"voiture","champs":[{"nom":"id","type":"int"},{"nom":"mark","type":"string"},{"nom":"number","type":"string"}]}}},
		{"id":"selectNode_1","type":"selectNode","data":{"name":"selectNode","id":true,"mark":true,"number":false}},
		{"id":"whereNode_1","type":"whereNode","data":{"name":"whereNode"}},
		{"id":"returnNode_1","type":"returnNode","data":{"name":"returnNode"}},
		{"id":"responseNode_1","type":"responseNode","data":{}}
	]`
	edgesJSON := `[
		{"source":"rootNode_1","target":"modelNode_1","id":"e1"},
		{"source":"modelNode_1","target":"selectNode_1","id":"e2"},
		{"source":"selectNode_1","target":"whereNode_1","id":"e3"},
		{"source":"whereNode_1","target":"returnNode_1","id":"e4"},
		{"source":"returnNode_1","target":"responseNode_1","id":"e5"}
	]`

	roots, err := BuildTurboStackHierarchy(nodesJSON, edgesJSON)
	if err != nil {
		log.Fatalf("Erreur: %v", err)
	}

	gen := &TurboStackGenerator{}
	code := gen.GenerateRoute("GET /voitures/{id}", roots)

	fmt.Println(code)
}
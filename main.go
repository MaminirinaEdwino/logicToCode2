package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// --- 1. Structures de données ---

type DynamicNode struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Data     map[string]any `json:"data"`
	Children []*DynamicNode `json:"children"`
}

type Edge struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceHandle string `json:"sourceHandle"`
	TargetHandle string `json:"targetHandle"`
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
	Field       string
	Operator    string
	BoundSource string // ex: "var_userId" ou "r.PathValue(\"id\")"
	ParamIdx    int
}

type QueryContext struct {
	Model         ModelMeta
	Operation     string                 // SELECT, INSERT, UPDATE, DELETE
	SelectFields  []string               // Champs pour SELECT
	BoundInputs   map[string]string      // Mapping champ -> variable (ex: "mark" -> "body.Mark")
	WhereFilters  []WhereFilter          // Filtres issus de bindings ou de data
	StatusCodeVar string                 // Variable du code de statut HTTP
}

// --- 2. Construction de la hiérarchie et résolution des Bindings ---

type LogicPayload struct {
	Edge string `json:"edge"`
	Node string `json:"node"`
}

func ParseTurboStackGraph(nodesRawJSON string, edgesRawJSON string) ([]*DynamicNode, []Edge, map[string]*DynamicNode, error) {
	var rawNodes []map[string]any
	if err := json.Unmarshal([]byte(nodesRawJSON), &rawNodes); err != nil {
		return nil, nil, nil, fmt.Errorf("erreur unmarshal nodes: %w", err)
	}

	var edges []Edge
	if err := json.Unmarshal([]byte(edgesRawJSON), &edges); err != nil {
		return nil, nil, nil, fmt.Errorf("erreur unmarshal edges: %w", err)
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
			Children: []*DynamicNode{},
		}
		inDegree[id] = 0
	}

	// Définition des flux principaux (ignore les connexions de Data Binding pour la hiérarchie pure)
	for _, edge := range edges {
		parent, parentOk := nodeMap[edge.Source]
		child, childOk := nodeMap[edge.Target]

		if parentOk && childOk {
			// On filtre les edges de données (data binding) pour ne garder que le flux de contrôle
			if edge.TargetHandle == "response_status" || 
			   strings.HasPrefix(edge.TargetHandle, "insert-value-") || 
			   strings.HasPrefix(edge.TargetHandle, "voiture_where_") ||
			   strings.HasSuffix(edge.TargetHandle, "_handle") {
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

	return roots, edges, nodeMap, nil
}

// --- 3. Générateur de Code ---

type TurboStackGenerator struct {
	builder strings.Builder
	indent  int
	edges   []Edge
	nodeMap map[string]*DynamicNode
}

func (g *TurboStackGenerator) writeIndent() {
	for i := 0; i < g.indent; i++ {
		g.builder.WriteString("\t")
	}
}

func (g *TurboStackGenerator) GenerateRoute(routePattern string, roots []*DynamicNode, edges []Edge, nodeMap map[string]*DynamicNode) string {
	g.builder.Reset()
	g.indent = 0
	g.edges = edges
	g.nodeMap = nodeMap

	g.builder.WriteString(fmt.Sprintf("mux.HandleFunc(%q, func(w http.ResponseWriter, r *http.Request) {\n", routePattern))
	g.indent++

	g.writeIndent()
	g.builder.WriteString("db := config.ConnectDB()\n")
	g.writeIndent()
	g.builder.WriteString("defer db.Close()\n\n")

	ctx := &QueryContext{
		StatusCodeVar: "http.StatusOK",
		BoundInputs:   make(map[string]string),
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
		g.builder.WriteString("// Extraction des paramètres par défaut\n")
		g.writeIndent()
		g.builder.WriteString("id := r.PathValue(\"id\")\n\n")

	case "varNode":
		varName, _ := node.Data["name"].(string)
		varType, _ := node.Data["type"].(string)
		defaultVal, _ := node.Data["default value"].(string)
		if varType == "int" {
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("var %s int = %s\n", varName, defaultVal))
		} else {
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("%s := %q\n", varName, defaultVal))
		}

	case "modelNode":
		// Extraction du modèle
		if m, ok := node.Data["model"].(map[string]any); ok {
			if nom, ok := m["nom"].(string); ok {
				ctx.Model.Name = nom
			}
			ctx.Model.Fields = []ModelField{}
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
		g.builder.WriteString(fmt.Sprintf("// --- Modèle : %s ---\n", ctx.Model.Name))

	case "selectNode":
		ctx.Operation = "SELECT"
		var selected []string

		// Inspection des attributs booléens dans node.Data
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
		g.builder.WriteString(fmt.Sprintf("// Action SELECT [%s]\n", strings.Join(selected, ", ")))

	case "insertNode":
		ctx.Operation = "INSERT"
		ctx.StatusCodeVar = "http.StatusCreated"
		g.writeIndent()
		g.builder.WriteString("// Action INSERT\n")

		// Résolution des bindings d'entrée via les edges (bodyParamsNode -> insertNode)
		for _, e := range g.edges {
			if e.Target == node.ID && strings.HasPrefix(e.TargetHandle, "insert-value-") {
				fieldName := strings.TrimPrefix(e.TargetHandle, "insert-value-")
				if sourceNode, ok := g.nodeMap[e.Source]; ok && sourceNode.Type == "bodyParamsNode" {
					if bp, ok := sourceNode.Data["bodyParams"].(map[string]any); ok {
						if fieldMap, ok := bp["field"].(map[string]any); ok {
							fName, _ := fieldMap["nom"].(string)
							ctx.BoundInputs[fieldName] = fmt.Sprintf("reqBody.%s", capitalize(fName))
						}
					}
				}
			}
		}

		// Génération de la structure de body
		g.writeIndent()
		g.builder.WriteString("type insertPayload struct {\n")
		g.indent++
		for _, f := range ctx.Model.Fields {
			if f.Nom != "id" {
				g.writeIndent()
				g.builder.WriteString(fmt.Sprintf("%s %s `json:%q`\n", capitalize(f.Nom), f.Type, f.Nom))
			}
		}
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n")
		g.writeIndent()
		g.builder.WriteString("var reqBody insertPayload\n")
		g.writeIndent()
		g.builder.WriteString("if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {\n")
		g.indent++
		g.writeIndent()
		g.builder.WriteString("http.Error(w, \"Payload invalide\", http.StatusBadRequest)\n")
		g.writeIndent()
		g.builder.WriteString("return\n")
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n\n")

	case "updateNode":
		ctx.Operation = "UPDATE"
		g.writeIndent()
		g.builder.WriteString("// Action UPDATE\n")

		// Data Binding pour updateNode
		for _, e := range g.edges {
			if e.Target == node.ID && strings.HasPrefix(e.TargetHandle, "insert-value-") {
				fieldName := strings.TrimPrefix(e.TargetHandle, "insert-value-")
				if sourceNode, ok := g.nodeMap[e.Source]; ok && sourceNode.Type == "bodyParamsNode" {
					if bp, ok := sourceNode.Data["bodyParams"].(map[string]any); ok {
						if fieldMap, ok := bp["field"].(map[string]any); ok {
							fName, _ := fieldMap["nom"].(string)
							ctx.BoundInputs[fieldName] = fmt.Sprintf("reqBody.%s", capitalize(fName))
						}
					}
				}
			}
		}

		g.writeIndent()
		g.builder.WriteString("type updatePayload struct {\n")
		g.indent++
		for _, f := range ctx.Model.Fields {
			if f.Nom != "id" {
				g.writeIndent()
				g.builder.WriteString(fmt.Sprintf("%s %s `json:%q`\n", capitalize(f.Nom), f.Type, f.Nom))
			}
		}
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n")
		g.writeIndent()
		g.builder.WriteString("var reqBody updatePayload\n")
		g.writeIndent()
		g.builder.WriteString("if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {\n")
		g.indent++
		g.writeIndent()
		g.builder.WriteString("http.Error(w, \"Payload invalide\", http.StatusBadRequest)\n")
		g.writeIndent()
		g.builder.WriteString("return\n")
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n\n")

	case "deleteNode":
		ctx.Operation = "DELETE"
		g.writeIndent()
		g.builder.WriteString("// Action DELETE\n")

	case "whereNode":
		g.writeIndent()
		g.builder.WriteString("// Clause Filtre (WHERE)\n")
		paramCounter := 1

		// Vérifier si un varNode ou un autre nœud est connecté au handle du whereNode (ex: voiture_where_idtarget)
		for _, e := range g.edges {
			if e.Target == node.ID && strings.Contains(e.TargetHandle, "_where_") {
				parts := strings.Split(e.TargetHandle, "_where_")
				if len(parts) == 2 {
					fieldName := strings.TrimSuffix(parts[1], "target")
					if sourceNode, ok := g.nodeMap[e.Source]; ok && sourceNode.Type == "varNode" {
						varName, _ := sourceNode.Data["name"].(string)
						ctx.WhereFilters = append(ctx.WhereFilters, WhereFilter{
							Field:       fieldName,
							Operator:    "=",
							BoundSource: varName,
							ParamIdx:    paramCounter,
						})
						paramCounter++
					}
				}
			}
		}

		// Fallback sur le path parameter ou les attributs du data du node
		if len(ctx.WhereFilters) == 0 {
			ctx.WhereFilters = append(ctx.WhereFilters, WhereFilter{
				Field:       "id",
				Operator:    "=",
				BoundSource: "id",
				ParamIdx:    paramCounter,
			})
		}

	case "returnNode":
		g.writeIndent()
		g.builder.WriteString("// Génération de la requête SQL\n")

		var whereClauses []string
		var whereArgs []string

		for _, filter := range ctx.WhereFilters {
			whereClauses = append(whereClauses, fmt.Sprintf("%s %s $%d", filter.Field, filter.Operator, filter.ParamIdx))
			whereArgs = append(whereArgs, filter.BoundSource)
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

		case "INSERT":
			var insertFields []string
			var placeholders []string
			var insertArgs []string

			idx := 1
			for fieldName, boundVar := range ctx.BoundInputs {
				insertFields = append(insertFields, fieldName)
				placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
				insertArgs = append(insertArgs, boundVar)
				idx++
			}

			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("query := \"INSERT INTO %s (%s) VALUES (%s) RETURNING id\"\n",
				ctx.Model.Name, strings.Join(insertFields, ", "), strings.Join(placeholders, ", ")))
			g.writeIndent()
			g.builder.WriteString("var newID int\n")
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("err := db.QueryRow(query, %s).Scan(&newID)\n", strings.Join(insertArgs, ", ")))

		case "UPDATE":
			var setClauses []string
			var updateArgs []string

			idx := 1
			for fieldName, boundVar := range ctx.BoundInputs {
				setClauses = append(setClauses, fmt.Sprintf("%s = $%d", fieldName, idx))
				updateArgs = append(updateArgs, boundVar)
				idx++
			}
			updateArgs = append(updateArgs, whereArgs...)

			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("query := \"UPDATE %s SET %s%s\"\n",
				ctx.Model.Name, strings.Join(setClauses, ", "), whereClauseSQL))
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("_, err := db.Exec(query, %s)\n", strings.Join(updateArgs, ", ")))

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
		g.builder.WriteString("http.Error(w, \"Erreur lors de l'exécution SQL\", http.StatusInternalServerError)\n")
		g.writeIndent()
		g.builder.WriteString("return\n")
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n\n")

	case "responseNode":
		// Détection du status code lié via statusCodeNode
		statusCode := ctx.StatusCodeVar
		for _, e := range g.edges {
			if e.Target == node.ID && e.TargetHandle == "response_status" {
				statusCode = "http.StatusOK"
			}
		}

		g.writeIndent()
		g.builder.WriteString(fmt.Sprintf("w.WriteHeader(%s)\n", statusCode))
		g.writeIndent()
		g.builder.WriteString("w.Header().Set(\"Content-Type\", \"application/json\")\n")

		if ctx.Operation == "SELECT" {
			g.writeIndent()
			g.builder.WriteString("json.NewEncoder(w).Encode(returnValue)\n")
		} else if ctx.Operation == "INSERT" {
			g.writeIndent()
			g.builder.WriteString("json.NewEncoder(w).Encode(map[string]any{\"id\": newID, \"message\": \"Créé avec succès\"})\n")
		} else {
			g.writeIndent()
			g.builder.WriteString("json.NewEncoder(w).Encode(map[string]string{\"message\": \"Opération réussie\"})\n")
		}

	case "ifElseNode":
		g.writeIndent()
		g.builder.WriteString("// Structure Conditionnelle (IF / ELSE)\n")

	case "ifNode":
		g.writeIndent()
		g.builder.WriteString("if true { // Condition issue des equalNodes\n")
		g.indent++

	case "equalNode":
		g.writeIndent()
		g.builder.WriteString("// Évaluation de l'égalité\n")
	}

	for _, child := range node.Children {
		g.traverseAndGenerate(child, ctx)
	}

	if node.Type == "ifNode" {
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n")
	}
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// --- 4. Exécution directe sur les données brutes fournies ---

func main() {
	rawEdge := `[{"source":"rootNode_6pdmo","target":"modelNode_9tfo7","targetHandle":"model_handle_target","id":"xy-edge__rootNode_6pdmo-modelNode_9tfo7model_handle_target"},{"id":"e-modelNode_9tfo7-selectNode_um68t","source":"modelNode_9tfo7","target":"selectNode_um68t","style":{"stroke":"#4ecdc4"}},{"id":"e-selectNode_um68t-whereNode_r9a9o","source":"selectNode_um68t","target":"whereNode_r9a9o","style":{"stroke":"#4ecdc4"}},{"id":"e-whereNode_r9a9o-returnNode_ki1vw","source":"whereNode_r9a9o","target":"returnNode_ki1vw","style":{"stroke":"#4ecdc4"}},{"source":"returnNode_ki1vw","target":"responseNode_fkvv7","targetHandle":"connect_from_parent","id":"xy-edge__returnNode_ki1vw-responseNode_fkvv7connect_from_parent"},{"source":"response_7awgp","sourceHandle":"status_code","target":"responseNode_fkvv7","targetHandle":"response_status","id":"xy-edge__response_7awgpstatus_code-responseNode_fkvv7response_status"},{"source":"rootNode_6pdmo","target":"var_tnt1t","targetHandle":"var_var_tnt1t","id":"xy-edge__rootNode_6pdmo-var_tnt1tvar_var_tnt1t"},{"source":"var_tnt1t","target":"whereNode_r9a9o","targetHandle":"voiture_where_idtarget","id":"xy-edge__var_tnt1t-whereNode_r9a9ovoiture_where_idtarget"}]`

	rawNode := `[{"id":"rootNode_6pdmo","type":"rootNode","data":{"name":"rootNode"}},{"id":"var_tnt1t","type":"varNode","data":{"name":"userId","params":"","type":"int","default value":"1"}},{"id":"modelNode_9tfo7","type":"modelNode","data":{"name":"voiture","model":{"nom":"voiture","champs":[{"nom":"id","type":"int","default_value":"autoincrement"},{"nom":"mark","type":"string","default_value":""},{"nom":"number","type":"string","default_value":""}]}}},{"id":"selectNode_um68t","type":"selectNode","data":{"name":"selectNode","id":true,"mark":true,"number":true}},{"id":"whereNode_r9a9o","type":"whereNode","data":{"name":"whereNode"}},{"id":"returnNode_ki1vw","type":"returnNode","data":{"name":"returnNode"}},{"id":"responseNode_fkvv7","type":"responseNode","data":{}}]`

	roots, edges, nodeMap, err := ParseTurboStackGraph(rawNode, rawEdge)
	if err != nil {
		log.Fatalf("Erreur: %v", err)
	}

	gen := &TurboStackGenerator{}
	code := gen.GenerateRoute("GET /voitures/{id}", roots, edges, nodeMap)

	fmt.Println(code)
}
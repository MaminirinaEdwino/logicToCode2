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

type QueryContext struct {
	Model         ModelMeta
	Operation     string   // SELECT, INSERT, UPDATE, DELETE
	SelectFields  []string // Champs retenus par le Select pour la requête
	WhereParams   []string // Conditions WHERE
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
		g.builder.WriteString("// Extraction des paramètres d'URL (Path Values)\n")
		g.writeIndent()
		g.builder.WriteString("id := r.PathValue(\"id\")\n\n")

	case "modelNode":
		// Chargement des métadonnées du modèle
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

	case "selectNode":
		ctx.Operation = "SELECT"
		var selected []string

		// Vérification explicite : comparaison des clés du data du selectNode avec les champs du modèle
		for _, field := range ctx.Model.Fields {
			if val, exists := node.Data[field.Nom]; exists {
				if isSelected, ok := val.(bool); ok && isSelected {
					selected = append(selected, field.Nom)
				}
			}
		}

		// Fallback : si aucun champ sélectionné n'a été trouvé dans data, on retient tous les champs du modèle
		if len(selected) == 0 {
			for _, field := range ctx.Model.Fields {
				selected = append(selected, field.Nom)
			}
		}

		ctx.SelectFields = selected

		g.writeIndent()
		g.builder.WriteString(fmt.Sprintf("// Sélection configurée sur les champs : [%s]\n", strings.Join(selected, ", ")))

	case "whereNode":
		g.writeIndent()
		g.builder.WriteString("// Clause d'exclusion / Filtre (WHERE)\n")
		ctx.WhereParams = append(ctx.WhereParams, "id = $1")

	case "returnNode":
		g.writeIndent()
		g.builder.WriteString("// Structure de retour générée\n")
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

		whereClause := ""
		if len(ctx.WhereParams) > 0 {
			whereClause = " WHERE " + strings.Join(ctx.WhereParams, " AND ")
		}

		g.writeIndent()
		g.builder.WriteString(fmt.Sprintf("query := \"SELECT %s FROM %s%s\"\n", strings.Join(ctx.SelectFields, ", "), ctx.Model.Name, whereClause))
		g.writeIndent()
		g.builder.WriteString(fmt.Sprintf("err := db.QueryRow(query, id).Scan(%s)\n", strings.Join(scanPointers, ", ")))
		g.writeIndent()
		g.builder.WriteString("if err != nil {\n")
		g.indent++
		g.writeIndent()
		g.builder.WriteString("http.Error(w, \"Ressource introuvable\", http.StatusNotFound)\n")
		g.writeIndent()
		g.builder.WriteString("return\n")
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n\n")

	case "responseNode":
		g.writeIndent()
		g.builder.WriteString(fmt.Sprintf("w.WriteHeader(%s)\n", ctx.StatusCodeVar))
		g.writeIndent()
		g.builder.WriteString("w.Header().Set(\"Content-Type\", \"application/json\")\n")
		g.writeIndent()
		g.builder.WriteString("json.NewEncoder(w).Encode(returnValue)\n")
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
	nodesJSON := `[{"id":"rootNode_6pdmo","type":"rootNode","position":{"x":31.5,"y":186},"data":{"name":"rootNode"}},{"id":"modelNode_9tfo7","type":"modelNode","position":{"x":128,"y":142.5},"data":{"name":"voiture","model":{"nom":"voiture","champs":[{"nom":"id","type":"int","default_value":"autoincrement"},{"nom":"mark","type":"string","default_value":""},{"nom":"number","type":"string","default_value":""}]}}},{"id":"selectNode_um68t","type":"selectNode","position":{"x":501.37,"y":158.99},"data":{"name":"selectNode","selectedType":"ALL","id":true,"mark":true,"number":false}},{"id":"whereNode_r9a9o","type":"whereNode","position":{"x":819.14,"y":167.59},"data":{"name":"whereNode"}},{"id":"returnNode_ki1vw","type":"returnNode","position":{"x":1092.8,"y":187.59},"data":{"name":"returnNode"}},{"id":"responseNode_fkvv7","type":"responseNode","position":{"x":1334.66,"y":247.00},"data":{"response":[]}}]`
	edgesJSON := `[{"source":"rootNode_6pdmo","target":"modelNode_9tfo7","id":"e1"},{"id":"e2","source":"modelNode_9tfo7","target":"selectNode_um68t"},{"id":"e3","source":"selectNode_um68t","target":"whereNode_r9a9o"},{"id":"e4","source":"whereNode_r9a9o","target":"returnNode_ki1vw"},{"source":"returnNode_ki1vw","target":"responseNode_fkvv7","id":"e5"}]`

	roots, err := BuildTurboStackHierarchy(nodesJSON, edgesJSON)
	if err != nil {
		log.Fatalf("Erreur: %v", err)
	}

	gen := &TurboStackGenerator{}
	code := gen.GenerateRoute("GET /voitures/{id}", roots)

	fmt.Println(code)
}
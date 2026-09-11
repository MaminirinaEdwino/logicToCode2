package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// --- 1. Structure de l'Arbre TurboStack ---

type DynamicNode struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Data     map[string]any `json:"data"`
	RawNode  map[string]any `json:"raw_node"`
	Children []*DynamicNode `json:"children"`
}

// --- 2. Construction du Graphe TurboStack ---

func BuildTurboStackHierarchy(nodesRawJSON string, edgesRawJSON string) ([]*DynamicNode, error) {
	var rawNodes []map[string]any
	if err := json.Unmarshal([]byte(nodesRawJSON), &rawNodes); err != nil {
		return nil, fmt.Errorf("erreur unmarshal nodes: %w", err)
	}

	type Edge struct {
		ID     string `json:"id"`
		Source string `json:"source"`
		Target string `json:"target"`
	}
	var edges []Edge
	if err := json.Unmarshal([]byte(edgesRawJSON), &edges); err != nil {
		// Tolérance si edges est vide ("[]")
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

// --- 3. Générateur de Code Native HTTP pour TurboStack ---

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

	method := "GET"
	parts := strings.Split(routePattern, " ")
	if len(parts) > 1 {
		method = strings.ToUpper(parts[0])
	}

	g.builder.WriteString(fmt.Sprintf("mux.HandleFunc(%q, func(w http.ResponseWriter, r *http.Request) {\n", routePattern))
	g.indent++

	g.writeIndent()
	g.builder.WriteString("db := config.ConnectDB()\n")
	g.writeIndent()
	g.builder.WriteString("defer db.Close()\n\n")

	for _, root := range roots {
		g.traverseAndGenerate(root, method)
	}

	g.indent--
	g.writeIndent()
	g.builder.WriteString("})\n")

	return g.builder.String()
}

func (g *TurboStackGenerator) traverseAndGenerate(node *DynamicNode, httpMethod string) {
	fmt.Println("node", node)
	switch node.Type {

	case "rootNode":
		g.writeIndent()
		g.builder.WriteString("// Variables d'URL (Path Values)\n")
		g.writeIndent()
		g.builder.WriteString("id := r.PathValue(\"id\")\n\n")

	case "varNode":
		varName, _ := node.Data["name"].(string)
		varType, _ := node.Data["type"].(string)
		defaultVal, _ := node.Data["default value"].(string)

		if varName != "" {
			goType := "string"
			if varType == "int" {
				goType = "int"
			} else if varType == "bool" {
				goType = "bool"
			}

			g.writeIndent()
			if defaultVal != "" {
				g.builder.WriteString(fmt.Sprintf("var %s %s = %q\n", varName, goType, defaultVal))
			} else {
				g.builder.WriteString(fmt.Sprintf("var %s %s\n", varName, goType))
			}
		}

	case "bodyParamsNode":
		// Extraction des paramètres du body
		if bodyData, ok := node.Data["bodyParams"].(map[string]any); ok {
			if field, ok := bodyData["field"].(map[string]any); ok {
				fNom, _ := field["nom"].(string)
				fType, _ := field["type"].(string)

				goType := "string"
				if fType == "int" {
					goType = "int"
				}

				g.writeIndent()
				g.builder.WriteString(fmt.Sprintf("// Paramètre du body : %s (%s)\n", fNom, goType))
			}
		}

	case "modelNode":
		modelTableName := "User_table"
		var fields []string
		var structFields []string
		var scanPointers []string

		if m, ok := node.Data["model"].(map[string]any); ok {
			if nom, ok := m["nom"].(string); ok {
				modelTableName = nom
			}
			if champs, ok := m["champs"].([]any); ok {
				for _, c := range champs {
					if champMap, ok := c.(map[string]any); ok {
						fNom, _ := champMap["nom"].(string)
						fType, _ := champMap["type"].(string)

						goType := "string"
						if fType == "int" {
							goType = "int"
						}

						capitalizedNom := capitalize(fNom)
						fields = append(fields, fNom)
						structFields = append(structFields, fmt.Sprintf("%s %s `json:%q`", capitalizedNom, goType, fNom))
						scanPointers = append(scanPointers, fmt.Sprintf("&returnValue.%s", capitalizedNom))
					}
				}
			}
		}

		if len(fields) == 0 {
			fields = []string{"id", "username", "password", "email", "role"}
			structFields = []string{
				"Id int `json:\"id\"`",
				"Username string `json:\"username\"`",
				"Password string `json:\"password\"`",
				"Email string `json:\"email\"`",
				"Role string `json:\"role\"`",
			}
			scanPointers = []string{"&returnValue.Id", "&returnValue.Username", "&returnValue.Password", "&returnValue.Email", "&returnValue.Role"}
		}

		g.writeIndent()
		g.builder.WriteString("type returnType struct {\n")
		g.indent++
		for _, sf := range structFields {
			g.writeIndent()
			g.builder.WriteString(sf + "\n")
		}
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n\n")

		g.writeIndent()
		g.builder.WriteString("var returnValue returnType\n")
		g.writeIndent()
		g.builder.WriteString(fmt.Sprintf("query := \"select %s from %s where id = $1\"\n", strings.Join(fields, ", "), modelTableName))
		g.writeIndent()
		g.builder.WriteString(fmt.Sprintf("err := db.QueryRow(query, id).Scan(%s)\n", strings.Join(scanPointers, ", ")))
		g.writeIndent()
		g.builder.WriteString("if err != nil {\n")
		g.indent++
		g.writeIndent()
		g.builder.WriteString("http.Error(w, \"model introuvable\", http.StatusNotFound)\n")
		g.writeIndent()
		g.builder.WriteString("return\n")
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n\n")

	case "tryCatchNode":
		g.writeIndent()
		g.builder.WriteString("// --- Bloc Try / Catch ---\n")

	case "ifElseNode":
		g.writeIndent()
		g.builder.WriteString("if true {\n")
		g.indent++
		g.writeIndent()
		g.builder.WriteString("// Logique IF\n")
		g.indent--
		g.writeIndent()
		g.builder.WriteString("} else {\n")
		g.indent++
		g.writeIndent()
		g.builder.WriteString("// Logique ELSE\n")
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n\n")

	case "forNode":
		g.writeIndent()
		g.builder.WriteString("for i := 0; i < 10; i++ {\n")
		g.indent++
		g.writeIndent()
		g.builder.WriteString("// Logique Boucle FOR\n")
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n\n")

	case "whileNode":
		g.writeIndent()
		g.builder.WriteString("for true {\n")
		g.indent++
		g.writeIndent()
		g.builder.WriteString("// Logique Boucle WHILE\n")
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n\n")

	case "statusCodeNode":
		g.writeIndent()
		g.builder.WriteString("w.WriteHeader(http.StatusOK)\n")

	case "responseNode":
		g.writeIndent()
		g.builder.WriteString("renderTemplate(w, \".html\", map[string]interface{}{\n")
		g.indent++
		g.writeIndent()
		g.builder.WriteString("\"User_table\": returnValue,\n")
		g.writeIndent()
		g.builder.WriteString("\"Id\": id,\n")
		g.indent--
		g.writeIndent()
		g.builder.WriteString("})\n")
	}

	for _, child := range node.Children {
		g.traverseAndGenerate(child, httpMethod)
	}
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// --- Execution sur le Payload TurboStack ---

func main() {
	nodesJSON := "[{\"id\":\"rootNode_6pdmo\",\"type\":\"rootNode\",\"position\":{\"x\":31.5,\"y\":186},\"data\":{\"name\":\"rootNode\"},\"measured\":{\"width\":30,\"height\":30},\"selected\":false,\"dragging\":false},{\"id\":\"modelNode_9tfo7\",\"type\":\"modelNode\",\"position\":{\"x\":128,\"y\":142.5},\"data\":{\"name\":\"voiture\",\"model\":{\"nom\":\"voiture\",\"champs\":[{\"nom\":\"id\",\"type\":\"int\",\"default_value\":\"autoincrement\",\"constraint\":[\"primary key\",\"autoincrement\"]},{\"nom\":\"mark\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[]},{\"nom\":\"number\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[\"unique\"]}]}},\"measured\":{\"width\":160,\"height\":167},\"selected\":false,\"dragging\":false},{\"id\":\"selectNode_um68t\",\"type\":\"selectNode\",\"position\":{\"x\":501.375796178344,\"y\":158.99681528662424},\"data\":{\"name\":\"selectNode\",\"selectedType\":\"ALL\",\"model\":{\"nom\":\"voiture\",\"champs\":[{\"nom\":\"id\",\"type\":\"int\",\"default_value\":\"autoincrement\",\"constraint\":[\"primary key\",\"autoincrement\"]},{\"nom\":\"mark\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[]},{\"nom\":\"number\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[\"unique\"]}]},\"id\":true,\"mark\":true,\"number\":true},\"measured\":{\"width\":232,\"height\":190},\"selected\":false,\"dragging\":false},{\"id\":\"whereNode_r9a9o\",\"type\":\"whereNode\",\"position\":{\"x\":819.1464968152867,\"y\":167.5955414012739},\"data\":{\"name\":\"whereNode\",\"selectedType\":\"ALL\",\"model\":{\"nom\":\"voiture\",\"champs\":[{\"nom\":\"id\",\"type\":\"int\",\"default_value\":\"autoincrement\",\"constraint\":[\"primary key\",\"autoincrement\"]},{\"nom\":\"mark\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[]},{\"nom\":\"number\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[\"unique\"]}]},\"voiture_check_id\":false,\"voiture_check_mark\":false},\"measured\":{\"width\":160,\"height\":151},\"selected\":false,\"dragging\":false},{\"id\":\"returnNode_ki1vw\",\"type\":\"returnNode\",\"position\":{\"x\":1092.80751001931,\"y\":187.5955414012739},\"data\":{\"name\":\"returnNode\",\"model\":{\"nom\":\"voiture\",\"champs\":[{\"nom\":\"id\",\"type\":\"int\",\"default_value\":\"autoincrement\",\"constraint\":[\"primary key\",\"autoincrement\"]},{\"nom\":\"mark\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[]},{\"nom\":\"number\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[\"unique\"]}]},\"id\":true,\"mark\":true,\"number\":true},\"measured\":{\"width\":160,\"height\":137},\"selected\":false,\"dragging\":false},{\"id\":\"responseNode_fkvv7\",\"type\":\"responseNode\",\"position\":{\"x\":1334.6610617441002,\"y\":247.0064328840504},\"data\":{\"response\":[]},\"measured\":{\"width\":160,\"height\":83},\"selected\":false,\"dragging\":false},{\"id\":\"response_7awgp\",\"type\":\"statusCodeNode\",\"position\":{\"x\":1071.6441632974104,\"y\":385.3135856716944},\"data\":{\"response\":[]},\"measured\":{\"width\":210,\"height\":89},\"selected\":true,\"dragging\":false}]"
	edgesJSON := "[{\"source\":\"rootNode_6pdmo\",\"target\":\"modelNode_9tfo7\",\"targetHandle\":\"model_handle_target\",\"id\":\"xy-edge__rootNode_6pdmo-modelNode_9tfo7model_handle_target\"},{\"id\":\"e-modelNode_9tfo7-selectNode_um68t\",\"source\":\"modelNode_9tfo7\",\"target\":\"selectNode_um68t\",\"style\":{\"stroke\":\"#4ecdc4\"}},{\"id\":\"e-modelNode_9tfo7-selectNode_fooop\",\"source\":\"modelNode_9tfo7\",\"target\":\"selectNode_fooop\",\"style\":{\"stroke\":\"#4ecdc4\"}},{\"id\":\"e-selectNode_um68t-whereNode_r9a9o\",\"source\":\"selectNode_um68t\",\"target\":\"whereNode_r9a9o\",\"style\":{\"stroke\":\"#4ecdc4\"}},{\"id\":\"e-selectNode_um68t-whereNode_t5x8q\",\"source\":\"selectNode_um68t\",\"target\":\"whereNode_t5x8q\",\"style\":{\"stroke\":\"#4ecdc4\"}},{\"id\":\"e-whereNode_r9a9o-returnNode_ki1vw\",\"source\":\"whereNode_r9a9o\",\"target\":\"returnNode_ki1vw\",\"style\":{\"stroke\":\"#4ecdc4\"}},{\"id\":\"e-whereNode_r9a9o-returnNode_f9j23\",\"source\":\"whereNode_r9a9o\",\"target\":\"returnNode_f9j23\",\"style\":{\"stroke\":\"#4ecdc4\"}},{\"source\":\"returnNode_ki1vw\",\"target\":\"responseNode_fkvv7\",\"targetHandle\":\"connect_from_parent\",\"id\":\"xy-edge__returnNode_ki1vw-responseNode_fkvv7connect_from_parent\"},{\"source\":\"response_7awgp\",\"sourceHandle\":\"status_code\",\"target\":\"responseNode_fkvv7\",\"targetHandle\":\"response_status\",\"id\":\"xy-edge__response_7awgpstatus_code-responseNode_fkvv7response_status\"}]"


	roots, err := BuildTurboStackHierarchy(nodesJSON, edgesJSON)
	if err != nil {
		log.Fatalf("Erreur: %v", err)
	}

	gen := &TurboStackGenerator{}
	code := gen.GenerateRoute("GET /test/{id}/{username}", roots)

	fmt.Println(code)
}

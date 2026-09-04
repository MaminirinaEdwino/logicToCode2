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
	nodesJSON := `[{"id":"rootNode_876l1","type":"rootNode","position":{"x":-31.491111059891935,"y":127.3580575263534},"data":{"name":"rootNode"},"measured":{"width":30,"height":30},"selected":false,"dragging":false},{"id":"response_m1ojy","type":"statusCodeNode","position":{"x":33.221181086669404,"y":149.42649574619543},"data":{"response":[]},"measured":{"width":210,"height":89},"selected":false,"dragging":false},{"id":"tryCatchNode_aermh","type":"tryCatchNode","position":{"x":313.10074587669027,"y":184.3681895942786},"data":{},"measured":{"width":160,"height":113},"selected":false,"dragging":false},{"id":"whileNode_o6wku","type":"whileNode","position":{"x":375.97400150493144,"y":320.45689194997095},"data":{},"measured":{"width":160,"height":177},"selected":false,"dragging":false},{"id":"forNode_qqsbt","type":"forNode","position":{"x":40.69585091685644,"y":290.1127893976882},"data":{},"measured":{"width":160,"height":267},"selected":false,"dragging":false},{"id":"ifElseNode_ccgp5","type":"ifElseNode","position":{"x":523.7889509812558,"y":154.94360530115588},"data":{},"measured":{"width":160,"height":209},"selected":false,"dragging":false},{"id":"var_y7mti","type":"varNode","position":{"x":335.97973586414963,"y":-69.41884993390447},"data":{"name":"userId","params":"","type":"string","default value":""},"measured":{"width":236,"height":179},"selected":false,"dragging":false},{"id":"responseNode_7pebr","type":"responseNode","position":{"x":65.06786341728053,"y":-1.3744987560582835},"data":{"response":[]},"measured":{"width":160,"height":83},"selected":false,"dragging":false},{"id":"modelNode_2mdlx","type":"modelNode","position":{"x":591.0323076568483,"y":-27.121010012540637},"data":{"name":"voiture","model":{"nom":"voiture","champs":[{"nom":"id","type":"int","default_value":"autoincrement","constraint":["primary key","autoincrement"]},{"nom":"mark","type":"string","default_value":"","constraint":[]},{"nom":"number","type":"string","default_value":"","constraint":["unique"]}]}},"measured":{"width":160,"height":167},"selected":true,"dragging":false},{"id":"bodyParams_bib63","type":"bodyParamsNode","position":{"x":-254.57847676507302,"y":30.80864031454462},"data":{"bodyParams":{"field":{"nom":"mark","type":"string","default_value":"","constraint":[]}}},"measured":{"width":222,"height":88},"selected":false,"dragging":false},{"id":"bodyParams_8caqi","type":"bodyParamsNode","position":{"x":-227.5664332440751,"y":221.14891996068195},"data":{"bodyParams":{"field":{"nom":"number","type":"string","default_value":"","constraint":["unique"]}}},"measured":{"width":222,"height":88},"selected":false,"dragging":false}]`
	edgesJSON := `[]`

	roots, err := BuildTurboStackHierarchy(nodesJSON, edgesJSON)
	if err != nil {
		log.Fatalf("Erreur: %v", err)
	}

	gen := &TurboStackGenerator{}
	code := gen.GenerateRoute("GET /test/{id}/{username}", roots)

	fmt.Println(code)
}
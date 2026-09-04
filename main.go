package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// --- 1. Structure de l'Arbre Dynamique ---

type DynamicNode struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Data     map[string]any `json:"data"`
	RawNode  map[string]any `json:"raw_node"`
	Children []*DynamicNode `json:"children"`
}

// --- 2. Algorithme de Reconstruction de l'Arbre ---

func BuildDynamicHierarchy(nodesRawJSON string, edgesRawJSON string) ([]*DynamicNode, error) {
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
		return nil, fmt.Errorf("erreur unmarshal edges: %w", err)
	}

	nodeMap := make(map[string]*DynamicNode)
	inDegree := make(map[string]int)

	// Étape A: Instanciation dynamique de chaque nœud
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

	// Étape B: Liaison des enfants
	for _, edge := range edges {
		parent, parentOk := nodeMap[edge.Source]
		child, childOk := nodeMap[edge.Target]

		if parentOk && childOk {
			parent.Children = append(parent.Children, child)
			inDegree[edge.Target]++
		}
	}

	// Étape C: Extraction des racines
	var roots []*DynamicNode
	for id, count := range inDegree {
		if count == 0 {
			roots = append(roots, nodeMap[id])
		}
	}

	return roots, nil
}

// --- 3. Générateur de Code Routeur & Handler net/http ---

type NativeHTTPGenerator struct {
	builder strings.Builder
	indent  int
}

func (g *NativeHTTPGenerator) writeIndent() {
	for i := 0; i < g.indent; i++ {
		g.builder.WriteString("\t")
	}
}

func (g *NativeHTTPGenerator) GenerateRoute(routePattern string, roots []*DynamicNode) string {
	g.builder.Reset()
	g.indent = 0

	// Extraction de la méthode HTTP depuis le routePattern (ex: "POST /users" -> "POST")
	method := "GET"
	parts := strings.Split(routePattern, " ")
	if len(parts) > 1 {
		method = strings.ToUpper(parts[0])
	}

	// Déclaration de la route
	g.builder.WriteString(fmt.Sprintf("mux.HandleFunc(%q, func(w http.ResponseWriter, r *http.Request) {\n", routePattern))
	g.indent++

	// Connexion à la BDD
	g.writeIndent()
	g.builder.WriteString("db := config.ConnectDB()\n")
	g.writeIndent()
	g.builder.WriteString("defer db.Close()\n\n")

	// Parcours récursif
	for _, root := range roots {
		g.traverseAndGenerate(root, method)
	}

	g.indent--
	g.writeIndent()
	g.builder.WriteString("})\n")

	return g.builder.String()
}

func (g *NativeHTTPGenerator) traverseAndGenerate(node *DynamicNode, httpMethod string) {
	switch node.Type {
	case "rootNode":
		g.writeIndent()
		g.builder.WriteString("// Reading path parameters if present\n")
		g.writeIndent()
		g.builder.WriteString("id := r.PathValue(\"id\")\n\n")

	case "modelNode":
		modelName := "User_table"
		if m, ok := node.Data["model"].(map[string]any); ok {
			if nom, ok := m["nom"].(string); ok {
				modelName = nom
			}
		}

		// Inspection des champs du modèle
		var fields []string
		var nonPrimaryFields []string
		var structFields []string
		var scanPointers []string
		var valuePointers []string

		if m, ok := node.Data["model"].(map[string]any); ok {
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
						if fNom != "id" {
							nonPrimaryFields = append(nonPrimaryFields, fNom)
							valuePointers = append(valuePointers, fmt.Sprintf("inputData.%s", capitalizedNom))
						}

						structFields = append(structFields, fmt.Sprintf("%s %s `json:%q`", capitalizedNom, goType, fNom))
						scanPointers = append(scanPointers, fmt.Sprintf("&returnValue.%s", capitalizedNom))
					}
				}
			}
		}

		// Traitement selon la méthode HTTP (POST, PUT, DELETE, GET)
		switch httpMethod {
		case "POST":
			// Lecture du Body JSON
			g.writeIndent()
			g.builder.WriteString("type inputType struct {\n")
			g.indent++
			for _, sf := range structFields {
				g.writeIndent()
				g.builder.WriteString(sf + "\n")
			}
			g.indent--
			g.writeIndent()
			g.builder.WriteString("}\n\n")

			g.writeIndent()
			g.builder.WriteString("var inputData inputType\n")
			g.writeIndent()
			g.builder.WriteString("if err := json.NewDecoder(r.Body).Decode(&inputData); err != nil {\n")
			g.indent++
			g.writeIndent()
			g.builder.WriteString("http.Error(w, \"données invalides\", http.StatusBadRequest)\n")
			g.writeIndent()
			g.builder.WriteString("return\n")
			g.indent--
			g.writeIndent()
			g.builder.WriteString("}\n\n")

			// Construction de la requête INSERT SQL
			placeholders := make([]string, len(nonPrimaryFields))
			for i := range nonPrimaryFields {
				placeholders[i] = fmt.Sprintf("$%d", i+1)
			}
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("query := \"insert into %s (%s) values (%s) returning id\"\n",
				modelName, strings.Join(nonPrimaryFields, ", "), strings.Join(placeholders, ", ")))
			g.writeIndent()
			g.builder.WriteString("var newID int\n")
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("err := db.QueryRow(query, %s).Scan(&newID)\n", strings.Join(valuePointers, ", ")))
			g.writeIndent()
			g.builder.WriteString("if err != nil {\n")
			g.indent++
			g.writeIndent()
			g.builder.WriteString("http.Error(w, \"erreur lors de la création\", http.StatusInternalServerError)\n")
			g.writeIndent()
			g.builder.WriteString("return\n")
			g.indent--
			g.writeIndent()
			g.builder.WriteString("}\n\n")

		case "PUT":
			// Lecture du Body JSON
			g.writeIndent()
			g.builder.WriteString("type inputType struct {\n")
			g.indent++
			for _, sf := range structFields {
				g.writeIndent()
				g.builder.WriteString(sf + "\n")
			}
			g.indent--
			g.writeIndent()
			g.builder.WriteString("}\n\n")

			g.writeIndent()
			g.builder.WriteString("var inputData inputType\n")
			g.writeIndent()
			g.builder.WriteString("if err := json.NewDecoder(r.Body).Decode(&inputData); err != nil {\n")
			g.indent++
			g.writeIndent()
			g.builder.WriteString("http.Error(w, \"données invalides\", http.StatusBadRequest)\n")
			g.writeIndent()
			g.builder.WriteString("return\n")
			g.indent--
			g.writeIndent()
			g.builder.WriteString("}\n\n")

			// Construction UPDATE SQL
			setClauses := make([]string, len(nonPrimaryFields))
			for i, field := range nonPrimaryFields {
				setClauses[i] = fmt.Sprintf("%s = $%d", field, i+1)
			}
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("query := \"update %s set %s where id = $%d\"\n",
				modelName, strings.Join(setClauses, ", "), len(nonPrimaryFields)+1))
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("_, err := db.Exec(query, %s, id)\n", strings.Join(append(valuePointers, "id"), ", ")))
			g.writeIndent()
			g.builder.WriteString("if err != nil {\n")
			g.indent++
			g.writeIndent()
			g.builder.WriteString("http.Error(w, \"erreur lors de la mise à jour\", http.StatusInternalServerError)\n")
			g.writeIndent()
			g.builder.WriteString("return\n")
			g.indent--
			g.writeIndent()
			g.builder.WriteString("}\n\n")

		case "DELETE":
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("query := \"delete from %s where id = $1\"\n", modelName))
			g.writeIndent()
			g.builder.WriteString("_, err := db.Exec(query, id)\n")
			g.writeIndent()
			g.builder.WriteString("if err != nil {\n")
			g.indent++
			g.writeIndent()
			g.builder.WriteString("http.Error(w, \"erreur lors de la suppression\", http.StatusInternalServerError)\n")
			g.writeIndent()
			g.builder.WriteString("return\n")
			g.indent--
			g.writeIndent()
			g.builder.WriteString("}\n\n")

		default: // GET
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
			g.builder.WriteString(fmt.Sprintf("query := \"select %s from %s where id = $1\"\n", strings.Join(fields, ", "), modelName))
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
		}

	case "responseNode":
		modelName := "User_table"
		if parentModel, ok := node.Data["model_name"].(string); ok {
			modelName = parentModel
		}

		switch httpMethod {
		case "POST":
			g.writeIndent()
			g.builder.WriteString("renderTemplate(w, \".html\", map[string]interface{}{\n")
			g.indent++
			g.writeIndent()
			g.builder.WriteString("\"Id\": newID,\n")
			g.writeIndent()
			g.builder.WriteString("\"Message\": \"Création réussie\",\n")
			g.indent--
			g.writeIndent()
			g.builder.WriteString("})\n")

		case "PUT", "DELETE":
			g.writeIndent()
			g.builder.WriteString("renderTemplate(w, \".html\", map[string]interface{}{\n")
			g.indent++
			g.writeIndent()
			g.builder.WriteString("\"Id\": id,\n")
			g.writeIndent()
			g.builder.WriteString("\"Message\": \"Opération effectuée avec succès\",\n")
			g.indent--
			g.writeIndent()
			g.builder.WriteString("})\n")

		default: // GET
			g.writeIndent()
			g.builder.WriteString("renderTemplate(w, \".html\", map[string]interface{}{\n")
			g.indent++
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("%q: returnValue,\n", modelName))
			g.writeIndent()
			g.builder.WriteString("\"Id\": id,\n")
			g.indent--
			g.writeIndent()
			g.builder.WriteString("})\n")
		}
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

// --- Exemple de test avec POST ---

func main() {
	nodesJSON := `[
		{"id":"rootNode_6pdmo","type":"rootNode","data":{"name":"rootNode"}},
		{"id":"modelNode_9tfo7","type":"modelNode","data":{"name":"voiture","model":{"nom":"User_table","champs":[{"nom":"id","type":"int"},{"nom":"username","type":"string"},{"nom":"password","type":"string"},{"nom":"email","type":"string"},{"nom":"role","type":"string"}]}}},
		{"id":"responseNode_fkvv7","type":"responseNode","data":{"model_name":"User_table"}}
	]`
	edgesJSON := `[
		{"source":"rootNode_6pdmo","target":"modelNode_9tfo7"},
		{"source":"modelNode_9tfo7","target":"responseNode_fkvv7"}
	]`

	roots, err := BuildDynamicHierarchy(nodesJSON, edgesJSON)
	if err != nil {
		log.Fatalf("Erreur: %v", err)
	}

	gen := &NativeHTTPGenerator{}

	fmt.Println("=== EXEMPLE ROUTE POST ===")
	codePost := gen.GenerateRoute("POST /test/create", roots)
	fmt.Println(codePost)
}
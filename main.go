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

// --- 2. Algorithme de Reconstruction de l'Arbre (map[string]any) ---

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

	// Étape B: Liaison des enfants selon les edges
	for _, edge := range edges {
		parent, parentOk := nodeMap[edge.Source]
		child, childOk := nodeMap[edge.Target]

		if parentOk && childOk {
			parent.Children = append(parent.Children, child)
			inDegree[edge.Target]++
		}
	}

	// Étape C: Identification des racines
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

	// Signature de la route
	g.builder.WriteString(fmt.Sprintf("mux.HandleFunc(%q, func(w http.ResponseWriter, r *http.Request) {\n", routePattern))
	g.indent++

	// Connexion BDD locale
	g.writeIndent()
	g.builder.WriteString("db := config.ConnectDB()\n")
	g.writeIndent()
	g.builder.WriteString("defer db.Close()\n\n")

	// Parcours récursif de l'arbre
	for _, root := range roots {
		g.traverseAndGenerate(root)
	}

	g.indent--
	g.writeIndent()
	g.builder.WriteString("})\n")

	return g.builder.String()
}

func (g *NativeHTTPGenerator) traverseAndGenerate(node *DynamicNode) {
	switch node.Type {
	case "rootNode":
		// Extraction automatique des variables de chemin de l'URL si besoin
		g.writeIndent()
		g.builder.WriteString("// Lecture des paramètres d'URL\n")
		g.writeIndent()
		g.builder.WriteString("id := r.PathValue(\"id\")\n\n")

	case "modelNode":
		modelName := "model_table"
		if m, ok := node.Data["model"].(map[string]any); ok {
			if nom, ok := m["nom"].(string); ok {
				modelName = nom
			}
		}

		// Inspection dynamique des champs du modèle dans la map
		var fields []string
		var structFields []string
		var scanPointers []string

		if m, ok := node.Data["model"].(map[string]any); ok {
			if champs, ok := m["champs"].([]any); ok {
				for _, c := range champs {
					if champMap, ok := c.(map[string]any); ok {
						fNom, _ := champMap["nom"].(string)
						fType, _ := champMap["type"].(string)

						// Mapping simple des types Go
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

		// Si aucun champ n'est trouvé dynamiquement, génération par défaut
		if len(fields) == 0 {
			structFields = []string{"Id int `json:\"id\"`", "Name string `json:\"name\"`"}
			fields = []string{"id", "name"}
			scanPointers = []string{"&returnValue.Id", "&returnValue.Name"}
		}

		// Structure anonyme retournée
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

		// Requête SQL et exécution
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

	case "responseNode":
		// Rendu final avec renderTemplate
		modelName := "data"
		if parentModel, ok := node.Data["model_name"].(string); ok {
			modelName = parentModel
		}

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

	default:
		// Les autres nœuds de transition
	}

	// Traitement récursif des nœuds enfants
	for _, child := range node.Children {
		g.traverseAndGenerate(child)
	}
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// --- Exemple de test ---

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
	code := gen.GenerateRoute("GET /test/{id}/{username}", roots)

	fmt.Println(code)
}
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

// --- 1. Structures d'entrée (JSON Unmarshaling) ---

type InputWrapper struct {
	Logic struct {
		Edge string `json:"edge"`
		Node string `json:"node"`
	} `json:"logic"`
}

type Edge struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceHandle string `json:"sourceHandle,omitempty"`
	TargetHandle string `json:"targetHandle,omitempty"`
}

type NodeData struct {
	Name         string                 `json:"name"`
	SelectedType string                 `json:"selectedType,omitempty"`
	Model        map[string]interface{} `json:"model,omitempty"`
}

type NodeRaw struct {
	ID    string                 `json:"id"`
	Type  string                 `json:"type"`
	Data  map[string]any         `json:"data"`
	Extra map[string]interface{} `json:"-"`
}

// --- 2. Structure d'Arbre Hiérarchique ---

type TreeNode struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Data     map[string]any `json:"data"`
	Children []*TreeNode    `json:"children"`
}

// --- 3. Construction de l'Arbre ---

func BuildHierarchyTree(nodesRaw []NodeRaw, edges []Edge) ([]*TreeNode, error) {
	nodeMap := make(map[string]*TreeNode)
	inDegree := make(map[string]int)

	// Instancier tous les nœuds de l'arbre
	for _, nr := range nodesRaw {
		nodeMap[nr.ID] = &TreeNode{
			ID:       nr.ID,
			Type:     nr.Type,
			Data:     nr.Data,
			Children: []*TreeNode{},
		}
		inDegree[nr.ID] = 0
	}

	// Établir les relations parent -> enfants
	for _, edge := range edges {
		parent, parentExists := nodeMap[edge.Source]
		child, childExists := nodeMap[edge.Target]

		if parentExists && childExists {
			parent.Children = append(parent.Children, child)
			inDegree[edge.Target]++
		}
	}

	// Identifier les nœuds racines (aucun parent ne pointant vers eux)
	var roots []*TreeNode
	for id, count := range inDegree {
		if count == 0 {
			roots = append(roots, nodeMap[id])
		}
	}

	return roots, nil
}

// --- 4. Générateur de Code Go (Traversée de l'arbre) ---

type CodeGenerator struct {
	builder strings.Builder
	indent  int
}

func (g *CodeGenerator) writeIndent() {
	for i := 0; i < g.indent; i++ {
		g.builder.WriteString("\t")
	}
}

func (g *CodeGenerator) Generate(roots []*TreeNode) string {
	g.builder.WriteString("package generated\n\n")
	g.builder.WriteString("import (\n")
	g.builder.WriteString("\t\"context\"\n")
	g.builder.WriteString("\t\"fmt\"\n")
	g.builder.WriteString(")\n\n")
	g.builder.WriteString("// ExecutionPipeline est généré automatiquement depuis le graphe\n")
	g.builder.WriteString("func ExecutionPipeline(ctx context.Context) error {\n")
	g.indent++

	for _, root := range roots {
		g.traverseNode(root)
	}

	g.writeIndent()
	g.builder.WriteString("return nil\n")
	g.indent--
	g.builder.WriteString("}\n")

	return g.builder.String()
}

func (g *CodeGenerator) traverseNode(node *TreeNode) {
	g.writeIndent()

	switch node.Type {
	case "rootNode":
		g.builder.WriteString(fmt.Sprintf("// --- Début du pipeline (%s) ---\n", node.ID))

	case "modelNode":
		modelName := node.Data["name"]
		model, _ := node.Data["model"].(map[string]any)
		if m, ok := model["nom"].(string); ok {
			modelName = m
		}
		g.builder.WriteString(fmt.Sprintf("// Modèle cible: %s\n", modelName))

	case "selectNode":
		name, _ := node.Data["name"].(string)
		g.builder.WriteString(fmt.Sprintf("query := db.Model(&%s{}).Select(\"*\")\n", capitalize(name)))

	case "whereNode":
		g.builder.WriteString("query = query.Where(\"1 = 1\")\n")

	case "returnNode":
		g.builder.WriteString("var results []interface{}\n")
		g.writeIndent()
		g.builder.WriteString("if err := query.Find(&results).Error; err != nil {\n")
		g.indent++
		g.writeIndent()
		g.builder.WriteString("return fmt.Errorf(\"échec de la requête: %w\", err)\n")
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n")

	case "responseNode":
		g.builder.WriteString("fmt.Println(\"Réponse envoyée avec succès\", results)\n")

	case "statusCodeNode":
		g.builder.WriteString("// Statut HTTP configuré\n")

	default:
		g.builder.WriteString(fmt.Sprintf("// Nœud de type inconnu: %s (%s)\n", node.Type, node.ID))
	}

	// Traversée récursive des enfants
	for _, child := range node.Children {
		g.traverseNode(child)
	}
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// --- 5. Main / Point d'entrée ---

func main() {
	rawJSON := `{
		"logic": {
			"edge": "[{\"source\":\"rootNode_6pdmo\",\"target\":\"modelNode_9tfo7\",\"targetHandle\":\"model_handle_target\",\"id\":\"xy-edge__rootNode_6pdmo-modelNode_9tfo7model_handle_target\"},{\"id\":\"e-modelNode_9tfo7-selectNode_um68t\",\"source\":\"modelNode_9tfo7\",\"target\":\"selectNode_um68t\",\"style\":{\"stroke\":\"#4ecdc4\"}},{\"id\":\"e-modelNode_9tfo7-selectNode_fooop\",\"source\":\"modelNode_9tfo7\",\"target\":\"selectNode_fooop\",\"style\":{\"stroke\":\"#4ecdc4\"}},{\"id\":\"e-selectNode_um68t-whereNode_r9a9o\",\"source\":\"selectNode_um68t\",\"target\":\"whereNode_r9a9o\",\"style\":{\"stroke\":\"#4ecdc4\"}},{\"id\":\"e-selectNode_um68t-whereNode_t5x8q\",\"source\":\"selectNode_um68t\",\"target\":\"whereNode_t5x8q\",\"style\":{\"stroke\":\"#4ecdc4\"}},{\"id\":\"e-whereNode_r9a9o-returnNode_ki1vw\",\"source\":\"whereNode_r9a9o\",\"target\":\"returnNode_ki1vw\",\"style\":{\"stroke\":\"#4ecdc4\"}},{\"id\":\"e-whereNode_r9a9o-returnNode_f9j23\",\"source\":\"whereNode_r9a9o\",\"target\":\"returnNode_f9j23\",\"style\":{\"stroke\":\"#4ecdc4\"}},{\"source\":\"returnNode_ki1vw\",\"target\":\"responseNode_fkvv7\",\"targetHandle\":\"connect_from_parent\",\"id\":\"xy-edge__returnNode_ki1vw-responseNode_fkvv7connect_from_parent\"},{\"source\":\"response_7awgp\",\"sourceHandle\":\"status_code\",\"target\":\"responseNode_fkvv7\",\"targetHandle\":\"response_status\",\"id\":\"xy-edge__response_7awgpstatus_code-responseNode_fkvv7response_status\"}]",
			"node": "[{\"id\":\"rootNode_6pdmo\",\"type\":\"rootNode\",\"position\":{\"x\":31.5,\"y\":186},\"data\":{\"name\":\"rootNode\"},\"measured\":{\"width\":30,\"height\":30},\"selected\":false,\"dragging\":false},{\"id\":\"modelNode_9tfo7\",\"type\":\"modelNode\",\"position\":{\"x\":128,\"y\":142.5},\"data\":{\"name\":\"voiture\",\"model\":{\"nom\":\"voiture\",\"champs\":[{\"nom\":\"id\",\"type\":\"int\",\"default_value\":\"autoincrement\",\"constraint\":[\"primary key\",\"autoincrement\"]},{\"nom\":\"mark\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[]},{\"nom\":\"number\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[\"unique\"]}]}},\"measured\":{\"width\":160,\"height\":293},\"selected\":false,\"dragging\":false},{\"id\":\"selectNode_um68t\",\"type\":\"selectNode\",\"position\":{\"x\":501.375796178344,\"y\":158.99681528662424},\"data\":{\"name\":\"selectNode\",\"selectedType\":\"ALL\",\"model\":{\"nom\":\"voiture\",\"champs\":[{\"nom\":\"id\",\"type\":\"int\",\"default_value\":\"autoincrement\",\"constraint\":[\"primary key\",\"autoincrement\"]},{\"nom\":\"mark\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[]},{\"nom\":\"number\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[\"unique\"]}]},\"id\":true,\"mark\":true,\"number\":true},\"measured\":{\"width\":232,\"height\":190},\"selected\":false,\"dragging\":false},{\"id\":\"whereNode_r9a9o\",\"type\":\"whereNode\",\"position\":{\"x\":819.1464968152867,\"y\":167.5955414012739},\"data\":{\"name\":\"whereNode\",\"selectedType\":\"ALL\",\"model\":{\"nom\":\"voiture\",\"champs\":[{\"nom\":\"id\",\"type\":\"int\",\"default_value\":\"autoincrement\",\"constraint\":[\"primary key\",\"autoincrement\"]},{\"nom\":\"mark\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[]},{\"nom\":\"number\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[\"unique\"]}]},\"voiture_check_id\":false,\"voiture_check_mark\":false},\"measured\":{\"width\":160,\"height\":151},\"selected\":false,\"dragging\":false},{\"id\":\"returnNode_ki1vw\",\"type\":\"returnNode\",\"position\":{\"x\":1092.80751001931,\"y\":187.5955414012739},\"data\":{\"name\":\"returnNode\",\"model\":{\"nom\":\"voiture\",\"champs\":[{\"nom\":\"id\",\"type\":\"int\",\"default_value\":\"autoincrement\",\"constraint\":[\"primary key\",\"autoincrement\"]},{\"nom\":\"mark\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[]},{\"nom\":\"number\",\"type\":\"string\",\"default_value\":\"\",\"constraint\":[\"unique\"]}]},\"id\":true,\"mark\":true,\"number\":true},\"measured\":{\"width\":160,\"height\":137},\"selected\":false,\"dragging\":false},{\"id\":\"responseNode_fkvv7\",\"type\":\"responseNode\",\"position\":{\"x\":1334.6610617441002,\"y\":247.0064328840504},\"data\":{\"response\":[]},\"measured\":{\"width\":160,\"height\":83},\"selected\":false,\"dragging\":false},{\"id\":\"response_7awgp\",\"type\":\"statusCodeNode\",\"position\":{\"x\":1071.6441632974104,\"y\":385.3135856716944},\"data\":{\"response\":[]},\"measured\":{\"width\":210,\"height\":89},\"selected\":true,\"dragging\":false}]"
		}
	}`

	// 1. Unmarshal global
	var input InputWrapper
	if err := json.Unmarshal([]byte(rawJSON), &input); err != nil {
		log.Fatalf("Erreur Unmarshal wrapper: %v", err)
	}

	// 2. Unmarshal des sous-chaînes JSON (edge et node)
	var edges []Edge
	if err := json.Unmarshal([]byte(input.Logic.Edge), &edges); err != nil {
		log.Fatalf("Erreur Unmarshal edges: %v", err)
	}

	var nodes []NodeRaw
	if err := json.Unmarshal([]byte(input.Logic.Node), &nodes); err != nil {
		log.Fatalf("Erreur Unmarshal nodes: %v", err)
	}

	// 3. Construction de la hiérarchie
	roots, err := BuildHierarchyTree(nodes, edges)
	if err != nil {
		log.Fatalf("Erreur lors de la construction de l'arbre: %v", err)
	}

	// 4. Génération de code
	gen := &CodeGenerator{}
	generatedCode := gen.Generate(roots)

	fmt.Println("=== CODE GO GÉNÉRÉ ===")
	fmt.Println(generatedCode)
	os.WriteFile("res/res.go", []byte(generatedCode), 0644)
}

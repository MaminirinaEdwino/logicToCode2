package generator

import (
	"strings"
)

type Field struct {
	Nom          string `json:"nom"`
	Type         string `json:"type"`
	DefaultValue string `json:"default_value"`
}

type Model struct {
	Nom    string  `json:"nom"`
	Champs []Field `json:"champs"`
}

type NodeData struct {
	Model         *Model                 `json:"model,omitempty"`
	SelectedType  string                 `json:"selectedType,omitempty"`
	Name          string                 `json:"name,omitempty"`
	Type          string                 `json:"type,omitempty"`
	DefaultValue  string                 `json:"default value,omitempty"`
	Status        int                    `json:"status,omitempty"`
	Increment     string                 `json:"increment,omitempty"`
	Response      []string               `json:"response,omitempty"`
	BodyParams    map[string]interface{} `json:"bodyParams,omitempty"`
	RequestParams string                 `json:"requestParams,omitempty"`
	RawData       map[string]interface{} `json:"-"`
}

type Node struct {
	ID   string                 `json:"id"`
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Handle string `json:"handle,omitempty"`
}

type LogicGraph struct {
	Nodes []Node `json:"node"`
	Edges []Edge `json:"edge"`
}

type CodeGenerator struct {
	nodes map[string]Node
	edges []Edge
	sb    strings.Builder
}

func NewCodeGenerator(graph LogicGraph) *CodeGenerator {
	nodeMap := make(map[string]Node)
	for _, n := range graph.Nodes {
		nodeMap[n.ID] = n
	}
	return &CodeGenerator{
		nodes: nodeMap,
		edges: graph.Edges,
	}
}

// GenerateController traduit le graphe en handler Go (Fiber / Gin / net/http)
// func (cg *CodeGenerator) GenerateController(handlerName string) string {
// 	cg.sb.WriteString(fmt.Sprintf("func %s(c *fiber.Ctx) error {\n", handlerName))

// 	// Recherche du nœud d'entrée principal (ex: RequestParams, BodyParams ou premier nœud d'action)
// 	startNode := cg.findStartNode()
// 	if startNode != nil {
// 		cg.traverseNode(startNode.ID)
// 	}

// 	cg.sb.WriteString("\treturn c.SendStatus(fiber.StatusOK)\n")
// 	cg.sb.WriteString("}\n")

// 	return cg.sb.String()
// }

func (cg *CodeGenerator) findStartNode() *Node {
	// Un nœud de départ n'a pas de cible d'edge entrante
	targetSet := make(map[string]bool)
	for _, e := range cg.edges {
		targetSet[e.Target] = true
	}
	for _, n := range cg.nodes {
		if !targetSet[n.ID] {
			return &n
		}
	}
	return nil
}

func (cg *CodeGenerator) traverseNode(nodeID string) {
	node, exists := cg.nodes[nodeID]
	if !exists {
		return
	}

	switch node.Type {
	case "VarNode":
		cg.generateVar(&node)
	case "RequestParamsNode":
		cg.generateRequestParam(&node)
	case "BodyParamsNode":
		cg.generateBodyParams(&node)
	case "InsertNode":
		cg.generateInsert(&node)
	case "SelectNode":
		cg.generateSelect(&node)
	case "UpdateNode":
		cg.generateUpdate(&node)
	case "DeleteNode":
		cg.generateDelete(&node)
	case "WhereNode":
		cg.generateWhere(&node)
	case "StatusCodeNode":
		cg.generateStatusCode(&node)
	case "ResponseNode":
		cg.generateResponse(&node)
	case "IfNode":
		cg.generateIf(&node)
	case "ElseIfNode":
		cg.generateElseIf(&node)
	case "ElseNode":
		cg.generateElse(&node)
	case "ForNode":
		cg.generateFor(&node)
	case "WhileNode":
		cg.generateWhile(&node)
	}

	// Traverser les nœuds suivants connectés
	for _, edge := range cg.edges {
		if edge.Source == nodeID {
			cg.traverseNode(edge.Target)
		}
	}
}


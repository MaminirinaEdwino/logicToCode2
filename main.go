package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// --- 1. Modèles de Données pour le Graph ---

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
	Nom  string `json:"nom"`
	Type string `json:"type"`
}

type ModelMeta struct {
	Name   string
	Fields []ModelField
}

type WhereFilter struct {
	Field       string
	Operator    string
	BoundSource string
	ParamIdx    int
}

// Contexte véhiculé le long de la branche
type ExecutionContext struct {
	Model        ModelMeta
	Operation    string            // "GET" (SELECT), "CREATE" (INSERT), "UPDATE", "DELETE"
	SelectFields []string          // Champs choisis si Operation == "GET"
	WhereFilters []WhereFilter     // Filtres accumulés par les whereNodes
	BoundInputs  map[string]string // Champs -> sources de données (body / var)
}

// --- 2. Parser du Graph JSON ---

func ParseGraph(nodesRaw string, edgesRaw string) ([]*DynamicNode, []Edge, map[string]*DynamicNode, error) {
	var rawNodes []map[string]any
	if err := json.Unmarshal([]byte(nodesRaw), &rawNodes); err != nil {
		return nil, nil, nil, fmt.Errorf("erreur nodes json: %w", err)
	}

	var edges []Edge
	if err := json.Unmarshal([]byte(edgesRaw), &edges); err != nil {
		return nil, nil, nil, fmt.Errorf("erreur edges json: %w", err)
	}

	nodeMap := make(map[string]*DynamicNode)
	inDegree := make(map[string]int)

	for _, n := range rawNodes {
		id, _ := n["id"].(string)
		nodeType, _ := n["type"].(string)
		dataMap, _ := n["data"].(map[string]any)

		nodeMap[id] = &DynamicNode{
			ID:       id,
			Type:     nodeType,
			Data:     dataMap,
			Children: []*DynamicNode{},
		}
		inDegree[id] = 0
	}

	// Lier uniquement les flux hiérarchiques de contrôle (ignorer les handles isolés de data-binding)
	for _, edge := range edges {
		parent, pOk := nodeMap[edge.Source]
		child, cOk := nodeMap[edge.Target]

		if pOk && cOk {
			// Filtre des liaisons de data binding pures (non-structurelles)
			if strings.Contains(edge.TargetHandle, "_where_") || strings.HasPrefix(edge.TargetHandle, "insert-value-") {
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

// --- 3. Générateur de Code Go ---

type CodeGenerator struct {
	builder strings.Builder
	indent  int
	edges   []Edge
	nodeMap map[string]*DynamicNode
}

func (g *CodeGenerator) writeIndent() {
	for i := 0; i < g.indent; i++ {
		g.builder.WriteString("\t")
	}
}

func (g *CodeGenerator) GenerateHandler(route string, roots []*DynamicNode, edges []Edge, nodeMap map[string]*DynamicNode) string {
	g.builder.Reset()
	g.indent = 0
	g.edges = edges
	g.nodeMap = nodeMap

	g.builder.WriteString(fmt.Sprintf("mux.HandleFunc(%q, func(w http.ResponseWriter, r *http.Request) {\n", route))
	g.indent++

	g.writeIndent()
	g.builder.WriteString("db := config.ConnectDB()\n")
	g.writeIndent()
	g.builder.WriteString("defer db.Close()\n\n")

	for _, root := range roots {
		ctx := &ExecutionContext{BoundInputs: make(map[string]string)}
		g.traverse(root, ctx)
	}

	g.indent--
	g.writeIndent()
	g.builder.WriteString("})\n")

	return g.builder.String()
}

func (g *CodeGenerator) traverse(node *DynamicNode, ctx *ExecutionContext) {
	switch node.Type {

	case "rootNode":
		g.writeIndent()
		g.builder.WriteString("// Point d'entrée de la route\n")

	case "varNode":
		varName, _ := node.Data["name"].(string)
		varType, _ := node.Data["type"].(string)
		defaultVal, _ := node.Data["default value"].(string)
		if varType == "int" {
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("%s := %s\n", varName, defaultVal))
		} else {
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("%s := %q\n", varName, defaultVal))
		}

	case "modelNode":
		// 1. LE MODEL NODE : Recueille UNIQUEMENT les infos du modèle (Nom + Champs)
		if m, ok := node.Data["model"].(map[string]any); ok {
			if nom, ok := m["nom"].(string); ok {
				ctx.Model.Name = nom
			}
			ctx.Model.Fields = nil
			if champs, ok := m["champs"].([]any); ok {
				for _, c := range champs {
					if cMap, ok := c.(map[string]any); ok {
						fNom, _ := cMap["nom"].(string)
						fType, _ := cMap["type"].(string)
						ctx.Model.Fields = append(ctx.Model.Fields, ModelField{Nom: fNom, Type: fType})
					}
				}
			}
		}
		// Aucun code SQL généré ici.

	case "selectNode", "getNode":
		// 2. ENFANT GET (SELECT) : Hérite des champs et sélectionne les attributs du node.Data
		ctx.Operation = "GET"
		ctx.SelectFields = nil

		// On extrait du data les clés correspondant aux champs du modèle valant true
		for _, field := range ctx.Model.Fields {
			if isSelected, ok := node.Data[field.Nom].(bool); ok && isSelected {
				ctx.SelectFields = append(ctx.SelectFields, field.Nom)
			}
		}

		// Si aucun champ spécifié explicitement, on sélectionne tout par défaut
		if len(ctx.SelectFields) == 0 {
			for _, field := range ctx.Model.Fields {
				ctx.SelectFields = append(ctx.SelectFields, field.Nom)
			}
		}

	case "createNode", "insertNode":
		// 2. ENFANT CREATE (INSERT)
		ctx.Operation = "CREATE"

	case "updateNode":
		// 2. ENFANT UPDATE
		ctx.Operation = "UPDATE"

	case "deleteNode":
		// 2. ENFANT DELETE
		ctx.Operation = "DELETE"

	case "whereNode":
		// 3. ENFANT WHERE : Spécifie les filtres pour les champs du modèle parent
		paramIdx := len(ctx.WhereFilters) + 1

		// Vérification des Data-Bindings reliés à ce whereNode via les edges
		boundFromEdge := false
		for _, e := range g.edges {
			if e.Target == node.ID && strings.Contains(e.TargetHandle, "_where_") {
				// ex: handle = "voiture_where_idtarget" -> champ "id"
				parts := strings.Split(e.TargetHandle, "_where_")
				if len(parts) == 2 {
					fieldNom := strings.TrimSuffix(parts[1], "target")
					if srcNode, ok := g.nodeMap[e.Source]; ok && srcNode.Type == "varNode" {
						vName, _ := srcNode.Data["name"].(string)
						ctx.WhereFilters = append(ctx.WhereFilters, WhereFilter{
							Field:       fieldNom,
							Operator:    "=",
							BoundSource: vName,
							ParamIdx:    paramIdx,
						})
						boundFromEdge = true
					}
				}
			}
		}

		// Fallback sur les valeurs lues directement dans data s'il n'y a pas d'edge
		if !boundFromEdge {
			for _, field := range ctx.Model.Fields {
				checkKey := ctx.Model.NomCheckKey(field.Nom)
				if isChecked, ok := node.Data[checkKey].(bool); ok && isChecked {
					opKey := ctx.Model.NomOpKey(field.Nom)
					operator, _ := node.Data[opKey].(string)
					if operator == "" {
						operator = "="
					}
					valKey := ctx.Model.NomValKey(field.Nom)
					val, _ := node.Data[valKey].(string)

					ctx.WhereFilters = append(ctx.WhereFilters, WhereFilter{
						Field:       field.Nom,
						Operator:    operator,
						BoundSource: fmt.Sprintf("%q", val),
						ParamIdx:    paramIdx,
					})
					paramIdx++
				}
			}
		}

	case "returnNode":
		// Génération de la requête SQL finale basée sur tout le contexte accumulé (Model + Opération + Where)
		g.generateSQLQuery(ctx)

	case "responseNode":
		g.writeIndent()
		g.builder.WriteString("w.Header().Set(\"Content-Type\", \"application/json\")\n")
		g.writeIndent()
		if ctx.Operation == "GET" {
			g.builder.WriteString("json.NewEncoder(w).Encode(result)\n")
		} else {
			g.builder.WriteString("json.NewEncoder(w).Encode(map[string]string{\"status\": \"ok\"})\n")
		}
	}

	// Traversée récursive des enfants de la branche
	for _, child := range node.Children {
		g.traverse(child, ctx)
	}
}

// Helpers pour reconstruire les clés de data du WhereNode
func (m *ModelMeta) NomCheckKey(field string) string { return m.Name + "_check_" + field }
func (m *ModelMeta) NomOpKey(field string) string    { return m.Name + "_operator_" + field }
func (m *ModelMeta) NomValKey(field string) string   { return m.Name + "_where_" + field }

func (g *CodeGenerator) generateSQLQuery(ctx *ExecutionContext) {
	var whereClauses []string
	var args []string

	for _, w := range ctx.WhereFilters {
		whereClauses = append(whereClauses, fmt.Sprintf("%s %s $%d", w.Field, w.Operator, w.ParamIdx))
		args = append(args, w.BoundSource)
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	switch ctx.Operation {
	case "GET":
		g.writeIndent()
		g.builder.WriteString(fmt.Sprintf("// Exécution SELECT sur le modèle %s\n", ctx.Model.Name))
		g.writeIndent()
		g.builder.WriteString("type RowResult struct {\n")
		g.indent++
		var scanTargets []string
		for _, fName := range ctx.SelectFields {
			capName := strings.Title(fName)
			g.writeIndent()
			g.builder.WriteString(fmt.Sprintf("%s string `json:%q`\n", capName, fName))
			scanTargets = append(scanTargets, "&result."+capName)
		}
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n")

		g.writeIndent()
		g.builder.WriteString("var result RowResult\n")
		g.writeIndent()
		g.builder.WriteString(fmt.Sprintf("query := \"SELECT %s FROM %s%s\"\n",
			strings.Join(ctx.SelectFields, ", "), ctx.Model.Name, whereSQL))
		g.writeIndent()
		g.builder.WriteString(fmt.Sprintf("err := db.QueryRow(query, %s).Scan(%s)\n",
			strings.Join(args, ", "), strings.Join(scanTargets, ", ")))
		g.writeIndent()
		g.builder.WriteString("if err != nil {\n")
		g.indent++
		g.writeIndent()
		g.builder.WriteString("http.Error(w, err.Error(), http.StatusInternalServerError)\n")
		g.writeIndent()
		g.builder.WriteString("return\n")
		g.indent--
		g.writeIndent()
		g.builder.WriteString("}\n\n")
	}
}

// --- Exemple d'exécution ---

func main() {
	rawEdges := `[
		{"id":"e1","source":"rootNode_1","target":"var_1"},
		{"id":"e2","source":"rootNode_1","target":"modelNode_1"},
		{"id":"e3","source":"modelNode_1","target":"selectNode_1"},
		{"id":"e4","source":"selectNode_1","target":"whereNode_1"},
		{"id":"e5","source":"whereNode_1","target":"returnNode_1"},
		{"id":"e6","source":"returnNode_1","target":"responseNode_1"},
		{"id":"e_data","source":"var_1","target":"whereNode_1","targetHandle":"voiture_where_idtarget"}
	]`

	rawNodes := `[
		{"id":"rootNode_1","type":"rootNode","data":{"name":"rootNode"}},
		{"id":"var_1","type":"varNode","data":{"name":"userId","type":"int","default value":"1"}},
		{"id":"modelNode_1","type":"modelNode","data":{"model":{"nom":"voiture","champs":[{"nom":"id","type":"int"},{"nom":"mark","type":"string"},{"nom":"number","type":"string"}]}}},
		{"id":"selectNode_1","type":"selectNode","data":{"mark":true,"number":true}},
		{"id":"whereNode_1","type":"whereNode","data":{}},
		{"id":"returnNode_1","type":"returnNode","data":{}},
		{"id":"responseNode_1","type":"responseNode","data":{}}
	]`

	roots, edges, nodeMap, err := ParseGraph(rawNodes, rawEdges)
	if err != nil {
		log.Fatalf("Erreur: %v", err)
	}

	gen := &CodeGenerator{}
	code := gen.GenerateHandler("GET /voitures/{id}", roots, edges, nodeMap)
	fmt.Println(code)
}
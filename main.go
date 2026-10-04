package main

import (
	"encoding/json"
	"fmt"
	"log" // Remplacez par votre chemin d'importation

	"github.com/MaminirinaEdwino/logicToCode2/generator"
)

func main() {
	jsonGraph := `{
		"node": [
			{
				"id": "node_role",
				"type": "RequestParamsNode",
				"data": {
					"requestParams": "role"
				}
			},
			{
				"id": "node_cmp",
				"type": "EqualNode",
				"data": {
					"left": "role",
					"right": "\"admin\""
				}
			},
			{
				"id": "node_if",
				"type": "IfNode",
				"data": {}
			},
			{
				"id": "node_select",
				"type": "SelectNode",
				"data": {
					"selectedType": "ALL",
					"model": {
						"nom": "User",
						"champs": [{"nom": "id", "type": "int"}]
					}
				}
			},
			{
				"id": "node_loop",
				"type": "ForNode",
				"data": {
					"collection": "userList"
				}
			},
			{
				"id": "node_status_err",
				"type": "StatusCodeNode",
				"data": {
					"status": 403
				}
			}
		],
		"edge": [
			{
				"id": "e1",
				"source": "node_role",
				"target": "node_if"
			},
			{
				"id": "e-cmp",
				"source": "node_cmp",
				"target": "node_if"
			},
			{
				"id": "e-true",
				"source": "node_if",
				"target": "node_select",
				"handle": "true"
			},
			{
				"id": "e-select-loop",
				"source": "node_select",
				"target": "node_loop"
			},
			{
				"id": "e-false",
				"source": "node_if",
				"target": "node_status_err",
				"handle": "else"
			}
		]
	}`

	var graph generator.LogicGraph
	if err := json.Unmarshal([]byte(jsonGraph), &graph); err != nil {
		log.Fatalf("Erreur de parsing JSON : %v", err)
	}

	gen := generator.NewCodeGenerator(graph)
	code := gen.GenerateController("ProcessUsersByRole")

	fmt.Println("// --- CODE GÉNÉRÉ (CONDITIONS ET BOUCLES) ---")
	fmt.Println(code)
}

package main

import (
	"encoding/json"
	"fmt"
	"log" // Remplacez par votre chemin d'importation

	"github.com/MaminirinaEdwino/logicToCode2/generator"
)

func main() {
	// Exemple JSON : Recherche de produits par catégorie et limite
	jsonGraph := `{
		"node": [
			{
				"id": "node_category",
				"type": "RequestParamsNode",
				"data": {
					"requestParams": "category"
				}
			},
			{
				"id": "node_limit",
				"type": "RequestParamsNode",
				"data": {
					"requestParams": "limit"
				}
			},
			{
				"id": "node_select",
				"type": "SelectNode",
				"data": {
					"selectedType": "ALL",
					"model": {
						"nom": "Product",
						"champs": [
							{"nom": "id", "type": "int"},
							{"nom": "name", "type": "string"},
							{"nom": "price", "type": "float"},
							{"nom": "category", "type": "string"}
						]
					}
				}
			},
			{
				"id": "node_where",
				"type": "WhereNode",
				"data": {
					"model": { "nom": "Product" },
					"Product_check_category": true,
					"Product_operator_category": "=",
					"Product_check_param_type_category": false,
					"Product_where_categorytarget": "category"
				}
			},
			{
				"id": "node_status",
				"type": "StatusCodeNode",
				"data": {
					"status": 200
				}
			},
			{
				"id": "node_response",
				"type": "ResponseNode",
				"data": {
					"response": ["productList"]
				}
			}
		],
		"edge": [
			{
				"id": "e1",
				"source": "node_category",
				"target": "node_limit"
			},
			{
				"id": "e2",
				"source": "node_limit",
				"target": "node_select"
			},
			{
				"id": "e3",
				"source": "node_select",
				"target": "node_where"
			},
			{
				"id": "e4",
				"source": "node_where",
				"target": "node_status"
			},
			{
				"id": "e5",
				"source": "node_status",
				"target": "node_response"
			}
		]
	}`

	var graph generator.LogicGraph
	if err := json.Unmarshal([]byte(jsonGraph), &graph); err != nil {
		log.Fatalf("Erreur de parsing JSON : %v", err)
	}

	gen := generator.NewCodeGenerator(graph)
	code := gen.GenerateController("GetProductsByCategory")

	fmt.Println("// --- CODE GÉNÉRÉ (RECHERCHE PRODUITS HAS_MANY / FILTER) ---")
	fmt.Println(code)
}
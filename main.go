package main

import (
	"encoding/json"
	"fmt"
	"log" // Remplacez par votre chemin d'importation

	"github.com/MaminirinaEdwino/logicToCode2/generator"
)

func main() {
	// Exemple de graphe complexe : Inscription + Mettre à jour des paramètres
	jsonGraph := `{
		"node": [
			{
				"id": "node_1",
				"type": "BodyParamsNode",
				"data": {
					"bodyParams": {
						"email": "string",
						"username": "string",
						"age": "int"
					}
				}
			},
			{
				"id": "node_2",
				"type": "VarNode",
				"data": {
					"name": "defaultRole",
					"type": "string",
					"default value": "user"
				}
			},
			{
				"id": "node_3",
				"type": "InsertNode",
				"data": {
					"model": {
						"nom": "User",
						"champs": [
							{"nom": "email", "type": "string"},
							{"nom": "username", "type": "string"},
							{"nom": "role", "type": "string"}
						]
					}
				}
			},
			{
				"id": "node_4",
				"type": "UpdateNode",
				"data": {
					"model": {
						"nom": "UserProfile",
						"champs": [
							{"nom": "is_active", "type": "bool"}
						]
					}
				}
			},
			{
				"id": "node_5",
				"type": "WhereNode",
				"data": {
					"model": { "nom": "UserProfile" },
					"UserProfile_check_email": true,
					"UserProfile_operator_email": "=",
					"UserProfile_check_param_type_email": false,
					"UserProfile_where_emailtarget": "email"
				}
			},
			{
				"id": "node_6",
				"type": "StatusCodeNode",
				"data": {
					"status": 201
				}
			},
			{
				"id": "node_7",
				"type": "ResponseNode",
				"data": {
					"response": ["email", "username"]
				}
			}
		],
		"edge": [
			{
				"id": "e1-2",
				"source": "node_1",
				"target": "node_2"
			},
			{
				"id": "e2-3",
				"source": "node_2",
				"target": "node_3"
			},
			{
				"id": "e3-4",
				"source": "node_3",
				"target": "node_4"
			},
			{
				"id": "e4-5",
				"source": "node_4",
				"target": "node_5"
			},
			{
				"id": "e5-6",
				"source": "node_5",
				"target": "node_6"
			},
			{
				"id": "e6-7",
				"source": "node_6",
				"target": "node_7"
			},
			{
				"id": "e-bind-email",
				"source": "node_1",
				"target": "node_3",
				"handle": "insert-value-email"
			},
			{
				"id": "e-bind-username",
				"source": "node_1",
				"target": "node_3",
				"handle": "insert-value-username"
			},
			{
				"id": "e-bind-role",
				"source": "node_2",
				"target": "node_3",
				"handle": "insert-value-role"
			}
		]
	}`

	var graph generator.LogicGraph
	if err := json.Unmarshal([]byte(jsonGraph), &graph); err != nil {
		log.Fatalf("Erreur de parsing JSON : %v", err)
	}

	gen := generator.NewCodeGenerator(graph)
	code := gen.GenerateController("RegisterUserHandler")

	fmt.Println("// --- CODE GÉNERÉ (SCÉNARIO COMPLEXE) ---")
	fmt.Println(code)
}
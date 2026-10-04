package generator

import (
	"fmt"
	"strings"
)

func (cg *CodeGenerator) generateInsert(node *Node) {
	modelName := getNestedString(node.Data, "model", "nom")
	fields, _ := node.Data["model"].(map[string]interface{})["champs"].([]interface{})

	varStruct := strings.ToLower(modelName)
	cg.sb.WriteString(fmt.Sprintf("\t%s := models.%s{\n", varStruct, modelName))

	for _, f := range fields {
		fieldMap := f.(map[string]interface{})
		fieldName := fieldMap["nom"].(string)

		handleKey := fmt.Sprintf("insert-value-%s", fieldName)
		val := cg.findSourceValueForHandle(node.ID, handleKey)
		if val != "" {
			cg.sb.WriteString(fmt.Sprintf("\t\t%s: %s,\n", capitalize(fieldName), val))
		}
	}
	cg.sb.WriteString("\t}\n")
	cg.sb.WriteString(fmt.Sprintf("\tif err := db.Create(&%s).Error; err != nil {\n", varStruct))
	cg.sb.WriteString("\t\thttp.Error(w, err.Error(), http.StatusInternalServerError)\n")
	cg.sb.WriteString("\t\treturn\n")
	cg.sb.WriteString("\t}\n")
}

func (cg *CodeGenerator) generateSelect(node *Node) {
	modelName := getNestedString(node.Data, "model", "nom")
	selectType, _ := node.Data["selectedType"].(string)
	varStruct := strings.ToLower(modelName)

	switch selectType {
	case "ALL":
		cg.sb.WriteString(fmt.Sprintf("\tvar %sList []models.%s\n", varStruct, modelName))
		cg.sb.WriteString(fmt.Sprintf("\tif err := db.Find(&%sList).Error; err != nil {\n", varStruct))
		cg.sb.WriteString("\t\thttp.Error(w, err.Error(), http.StatusInternalServerError)\n")
		cg.sb.WriteString("\t\treturn\n")
		cg.sb.WriteString("\t}\n")
	case "ONE":
		cg.sb.WriteString(fmt.Sprintf("\tvar %s models.%s\n", varStruct, modelName))
		cg.sb.WriteString(fmt.Sprintf("\tif err := db.First(&%s).Error; err != nil {\n", varStruct))
		cg.sb.WriteString("\t\thttp.Error(w, err.Error(), http.StatusNotFound)\n")
		cg.sb.WriteString("\t\treturn\n")
		cg.sb.WriteString("\t}\n")
	case "BY":
		cg.sb.WriteString(fmt.Sprintf("\tvar %sList []models.%s\n", varStruct, modelName))
		// La condition WHERE est chaînée via la traversée du WhereNode connecté
	}
}

func (cg *CodeGenerator) generateUpdate(node *Node) {
	modelName := getNestedString(node.Data, "model", "nom")
	fields, _ := node.Data["model"].(map[string]interface{})["champs"].([]interface{})

	cg.sb.WriteString("\tupdates := make(map[string]interface{})\n")

	for _, f := range fields {
		fieldMap := f.(map[string]interface{})
		fieldName := fieldMap["nom"].(string)

		handleKey := fmt.Sprintf("insert-value-%s", fieldName)
		val := cg.findSourceValueForHandle(node.ID, handleKey)
		if val != "" && val != "\"\"" {
			cg.sb.WriteString(fmt.Sprintf("\tupdates[\"%s\"] = %s\n", fieldName, val))
		}
	}

	cg.sb.WriteString(fmt.Sprintf("\tquery := db.Model(&models.%s{})\n", modelName))

	// Raccordement avec le WhereNode rattaché
	for _, edge := range cg.edges {
		if edge.Source == node.ID {
			targetNode, exists := cg.nodes[edge.Target]
			if exists && targetNode.Type == "WhereNode" {
				cg.generateWhere(&targetNode)
			}
		}
	}

	cg.sb.WriteString("\tif err := query.Updates(updates).Error; err != nil {\n")
	cg.sb.WriteString("\t\thttp.Error(w, err.Error(), http.StatusInternalServerError)\n")
	cg.sb.WriteString("\t\treturn\n")
	cg.sb.WriteString("\t}\n")
}

func (cg *CodeGenerator) generateDelete(node *Node) {
	modelName := getNestedString(node.Data, "model", "nom")

	cg.sb.WriteString(fmt.Sprintf("\tquery := db.Model(&models.%s{})\n", modelName))

	// Raccordement avec le WhereNode rattaché
	for _, edge := range cg.edges {
		if edge.Source == node.ID {
			targetNode, exists := cg.nodes[edge.Target]
			if exists && targetNode.Type == "WhereNode" {
				cg.generateWhere(&targetNode)
			}
		}
	}

	cg.sb.WriteString(fmt.Sprintf("\tif err := query.Delete(&models.%s{}).Error; err != nil {\n", modelName))
	cg.sb.WriteString("\t\thttp.Error(w, err.Error(), http.StatusInternalServerError)\n")
	cg.sb.WriteString("\t\treturn\n")
	cg.sb.WriteString("\t}\n")
}

func (cg *CodeGenerator) generateWhere(node *Node) {
	modelName := getNestedString(node.Data, "model", "nom")
	fields, _ := node.Data["model"].(map[string]interface{})["champs"].([]interface{})

	var conditions []string
	var args []string

	for _, f := range fields {
		fieldMap := f.(map[string]interface{})
		fieldName := fieldMap["nom"].(string)

		isCheck, _ := node.Data[modelName+"_check_"+fieldName].(bool)
		if isCheck {
			op, _ := node.Data[modelName+"_operator_"+fieldName].(string)
			if op == "" {
				op = "="
			}

			isParamInput, _ := node.Data[modelName+"_check_param_type_"+fieldName].(bool)
			if isParamInput {
				val := node.Data[modelName+"_where_"+fieldName]
				conditions = append(conditions, fmt.Sprintf("%s %s ?", fieldName, op))
				args = append(args, fmt.Sprintf("%v", val))
			} else {
				targetVal := node.Data[modelName+"_where_"+fieldName+"target"]
				conditions = append(conditions, fmt.Sprintf("%s %s ?", fieldName, op))
				args = append(args, fmt.Sprintf("%v", targetVal))
			}
		}
	}

	if len(conditions) > 0 {
		queryCond := strings.Join(conditions, " AND ")
		queryArgs := strings.Join(args, ", ")
		cg.sb.WriteString(fmt.Sprintf("\tquery = query.Where(\"%s\", %s)\n", queryCond, queryArgs))
	}
}

// func (cg *CodeGenerator) generateStatusCode(node *Node) {
// 	status := node.Data["status"]
// 	if status != nil {
// 		cg.sb.WriteString(fmt.Sprintf("\tc.Status(%v)\n", status))
// 	}
// }

// func (cg *CodeGenerator) generateResponse(node *Node) {
// 	respFields, ok := node.Data["response"].([]interface{})
// 	if !ok || len(respFields) == 0 {
// 		cg.sb.WriteString("\treturn c.SendStatus(fiber.StatusOK)\n")
// 		return
// 	}

// 	cg.sb.WriteString("\treturn c.JSON(fiber.Map{\n")
// 	for _, rf := range respFields {
// 		field := fmt.Sprintf("%v", rf)
// 		cg.sb.WriteString(fmt.Sprintf("\t\t\"%s\": %s,\n", field, field))
// 	}
// 	cg.sb.WriteString("\t})\n")
// }

func (cg *CodeGenerator) generateVar(node *Node) {
	name, _ := node.Data["name"].(string)
	varType, _ := node.Data["type"].(string)
	defaultVal := node.Data["default value"]

	if name == "" {
		return
	}

	// Formatage du type Go
	goType := "string"
	switch strings.ToLower(varType) {
	case "int", "integer":
		goType = "int"
	case "boolean", "bool":
		goType = "bool"
	case "float", "double":
		goType = "float64"
	case "json", "map":
		goType = "map[string]interface{}"
	}

	if defaultVal != nil && defaultVal != "" {
		if goType == "string" {
			cg.sb.WriteString(fmt.Sprintf("\tvar %s %s = \"%v\"\n", name, goType, defaultVal))
		} else {
			cg.sb.WriteString(fmt.Sprintf("\tvar %s %s = %v\n", name, goType, defaultVal))
		}
	} else {
		cg.sb.WriteString(fmt.Sprintf("\tvar %s %s\n", name, goType))
	}
}

// func (cg *CodeGenerator) generateRequestParam(node *Node) {
// 	paramName, _ := node.Data["requestParams"].(string)
// 	if paramName == "" {
// 		return
// 	}

// 	// Extraction du paramètre d'URL/Query via Fiber (ex: /users/:id ou ?id=1)
// 	cg.sb.WriteString(fmt.Sprintf("\t%s := c.Params(\"%s\")\n", paramName, paramName))
// 	cg.sb.WriteString(fmt.Sprintf("\tif %s == \"\" {\n", paramName))
// 	cg.sb.WriteString(fmt.Sprintf("\t\t%s = c.Query(\"%s\")\n", paramName, paramName))
// 	cg.sb.WriteString("\t}\n")
// }

// func (cg *CodeGenerator) generateBodyParams(node *Node) {
// 	bodyParams, ok := node.Data["bodyParams"].(map[string]interface{})
// 	if !ok || len(bodyParams) == 0 {
// 		return
// 	}

// 	// Création d'un DTO temporaire pour binder le body JSON
// 	cg.sb.WriteString("\tvar body struct {\n")
// 	for fieldName := range bodyParams {
// 		cg.sb.WriteString(fmt.Sprintf("\t\t%s string `json:\"%s\"` \n", capitalize(fieldName), fieldName))
// 	}
// 	cg.sb.WriteString("\t}\n")

// 	cg.sb.WriteString("\tif err := c.BodyParser(&body); err != nil {\n")
// 	cg.sb.WriteString("\t\treturn c.Status(400).JSON(fiber.Map{\"error\": \"Invalid request body\"})\n")
// 	cg.sb.WriteString("\t}\n")

// 	// Assignation aux variables locales
// 	for fieldName := range bodyParams {
// 		cg.sb.WriteString(fmt.Sprintf("\t%s := body.%s\n", fieldName, capitalize(fieldName)))
// 	}
// }

// func (cg *CodeGenerator) generateUpdate(node *Node) {
// 	modelName := getNestedString(node.Data, "model", "nom")
// 	fields, _ := node.Data["model"].(map[string]interface{})["champs"].([]interface{})

// 	// varStruct := strings.ToLower(modelName)
// 	cg.sb.WriteString(fmt.Sprintf("\tupdates := make(map[string]interface{})\n"))

// 	for _, f := range fields {
// 		fieldMap := f.(map[string]interface{})
// 		fieldName := fieldMap["nom"].(string)

// 		handleKey := fmt.Sprintf("insert-value-%s", fieldName)
// 		val := cg.findSourceValueForHandle(node.ID, handleKey)
// 		if val != "" && val != "\"\"" {
// 			cg.sb.WriteString(fmt.Sprintf("\tupdates[\"%s\"] = %s\n", fieldName, val))
// 		}
// 	}

// 	cg.sb.WriteString("\tquery := db.Model(&models." + modelName + "{})\n")

// 	// La condition WHERE sera appliquée si un WhereNode est rattaché
// 	for _, edge := range cg.edges {
// 		if edge.Source == node.ID {
// 			targetNode, exists := cg.nodes[edge.Target]
// 			if exists && targetNode.Type == "WhereNode" {
// 				cg.generateWhere(&targetNode)
// 			}
// 		}
// 	}

// 	cg.sb.WriteString(fmt.Sprintf("\tif err := query.Updates(updates).Error; err != nil {\n"))
// 	cg.sb.WriteString("\t\treturn c.Status(500).JSON(fiber.Map{\"error\": err.Error()})\n")
// 	cg.sb.WriteString("\t}\n")
// }

// func (cg *CodeGenerator) generateDelete(node *Node) {
// 	modelName := getNestedString(node.Data, "model", "nom")

// 	cg.sb.WriteString(fmt.Sprintf("\tquery := db.Model(&models.%s{})\n", modelName))

// 	// Application des clauses WHERE si un WhereNode est connecté
// 	for _, edge := range cg.edges {
// 		if edge.Source == node.ID {
// 			targetNode, exists := cg.nodes[edge.Target]
// 			if exists && targetNode.Type == "WhereNode" {
// 				cg.generateWhere(&targetNode)
// 			}
// 		}
// 	}

// 	cg.sb.WriteString(fmt.Sprintf("\tif err := query.Delete(&models.%s{}).Error; err != nil {\n", modelName))
// 	cg.sb.WriteString("\t\treturn c.Status(500).JSON(fiber.Map{\"error\": err.Error()})\n")
// 	cg.sb.WriteString("\t}\n")
// }

func (cg *CodeGenerator) GenerateController(handlerName string) string {
	cg.sb.WriteString(fmt.Sprintf("func %s(w http.ResponseWriter, r *http.Request) {\n", handlerName))

	startNode := cg.findStartNode()
	if startNode != nil {
		cg.traverseNode(startNode.ID)
	}

	cg.sb.WriteString("\tw.WriteHeader(http.StatusOK)\n")
	cg.sb.WriteString("}\n")

	return cg.sb.String()
}

func (cg *CodeGenerator) generateRequestParam(node *Node) {
	paramName, _ := node.Data["requestParams"].(string)
	if paramName == "" {
		return
	}

	// Récupération de la valeur depuis les paramètres de requête (Query) ou via le mux (r.PathValue en Go 1.22+)
	cg.sb.WriteString(fmt.Sprintf("\t%s := r.URL.Query().Get(\"%s\")\n", paramName, paramName))
	cg.sb.WriteString(fmt.Sprintf("\tif %s == \"\" {\n", paramName))
	cg.sb.WriteString(fmt.Sprintf("\t\t%s = r.PathValue(\"%s\")\n", paramName, paramName))
	cg.sb.WriteString("\t}\n")
}

func (cg *CodeGenerator) generateBodyParams(node *Node) {
	bodyParams, ok := node.Data["bodyParams"].(map[string]interface{})
	if !ok || len(bodyParams) == 0 {
		return
	}

	cg.sb.WriteString("\tvar body struct {\n")
	for fieldName := range bodyParams {
		cg.sb.WriteString(fmt.Sprintf("\t\t%s string `json:\"%s\"` \n", capitalize(fieldName), fieldName))
	}
	cg.sb.WriteString("\t}\n")

	// Remplacement de c.BodyParser par json.NewDecoder(r.Body)
	cg.sb.WriteString("\tif err := json.NewDecoder(r.Body).Decode(&body); err != nil {\n")
	cg.sb.WriteString("\t\thttp.Error(w, \"Invalid request body\", http.StatusBadRequest)\n")
	cg.sb.WriteString("\t\treturn\n")
	cg.sb.WriteString("\t}\n")

	for fieldName := range bodyParams {
		cg.sb.WriteString(fmt.Sprintf("\t%s := body.%s\n", fieldName, capitalize(fieldName)))
	}
}

func (cg *CodeGenerator) generateStatusCode(node *Node) {
	status := node.Data["status"]
	if status != nil {
		cg.sb.WriteString(fmt.Sprintf("\tw.WriteHeader(%v)\n", status))
	}
}

func (cg *CodeGenerator) generateResponse(node *Node) {
	respFields, ok := node.Data["response"].([]interface{})
	if !ok || len(respFields) == 0 {
		cg.sb.WriteString("\tw.WriteHeader(http.StatusOK)\n")
		cg.sb.WriteString("\treturn\n")
		return
	}

	cg.sb.WriteString("\tw.Header().Set(\"Content-Type\", \"application/json\")\n")
	cg.sb.WriteString("\tjson.NewEncoder(w).Encode(map[string]interface{}{\n")
	for _, rf := range respFields {
		field := fmt.Sprintf("%v", rf)
		cg.sb.WriteString(fmt.Sprintf("\t\t\"%s\": %s,\n", field, field))
	}
	cg.sb.WriteString("\t})\n")
	cg.sb.WriteString("\treturn\n")
}

func (cg *CodeGenerator) generateIf(node *Node) {
	condition := cg.buildCondition(node.ID)
	cg.sb.WriteString(fmt.Sprintf("\tif %s {\n", condition))

	// Exécution de la branche "true" (handle target : "then" ou "true")
	cg.traverseBranch(node.ID, "true")
	cg.sb.WriteString("\t}\n")

	// Exécution de la branche "else" si connectée
	cg.traverseBranch(node.ID, "else")
}

func (cg *CodeGenerator) generateElseIf(node *Node) {
	condition := cg.buildCondition(node.ID)
	cg.sb.WriteString(fmt.Sprintf("\telse if %s {\n", condition))

	cg.traverseBranch(node.ID, "true")
	cg.sb.WriteString("\t}\n")

	cg.traverseBranch(node.ID, "else")
}

func (cg *CodeGenerator) generateElse(node *Node) {
	cg.sb.WriteString("\telse {\n")
	cg.traverseBranch(node.ID, "body")
	cg.sb.WriteString("\t}\n")
}

// Reconstitution d'une condition d'évaluation
func (cg *CodeGenerator) buildCondition(nodeID string) string {
	// Recherche d'un nœud de comparaison (Equal, Different, Superior, Inferior) connecté
	for _, edge := range cg.edges {
		if edge.Target == nodeID {
			sourceNode, exists := cg.nodes[edge.Source]
			if !exists {
				continue
			}

			left := cg.findSourceValueForHandle(sourceNode.ID, "left")
			right := cg.findSourceValueForHandle(sourceNode.ID, "right")
			if left == "" {
				left = fmt.Sprintf("%v", sourceNode.Data["left"])
			}
			if right == "" {
				right = fmt.Sprintf("%v", sourceNode.Data["right"])
			}

			switch sourceNode.Type {
			case "EqualNode":
				return fmt.Sprintf("%s == %s", left, right)
			case "DifferentNode":
				return fmt.Sprintf("%s != %s", left, right)
			case "SuperiorNode":
				return fmt.Sprintf("%s > %s", left, right)
			case "InferiorNode":
				return fmt.Sprintf("%s < %s", left, right)
			}
		}
	}

	// Condition par défaut si aucune connexion complexe
	cond, ok := cg.nodes[nodeID].Data["condition"].(string)
	if ok && cond != "" {
		return cond
	}
	return "true"
}

// --- GESTION DES BOUCLES (FOR / WHILE) ---

func (cg *CodeGenerator) generateFor(node *Node) {
	varName, _ := node.Data["var"].(string)
	if varName == "" {
		varName = "i"
	}
	startVal := node.Data["start"]
	if startVal == nil {
		startVal = 0
	}
	endVal := node.Data["end"]
	if endVal == nil {
		endVal = 10
	}
	increment, _ := node.Data["increment"].(string)
	if increment == "" {
		increment = "i++"
	}

	// Si le nœud est un For Each (ex: itération sur un slice/array)
	collection, isSlice := node.Data["collection"].(string)
	if isSlice && collection != "" {
		cg.sb.WriteString(fmt.Sprintf("\tfor _, item := range %s {\n", collection))
	} else {
		// Boucle for standard indexée
		cg.sb.WriteString(fmt.Sprintf("\tfor %s := %v; %s < %v; %s {\n", varName, startVal, varName, endVal, increment))
	}

	// Corps de la boucle (handle "loop" ou "body")
	cg.traverseBranch(node.ID, "loop")
	cg.sb.WriteString("\t}\n")
}

func (cg *CodeGenerator) generateWhile(node *Node) {
	condition := cg.buildCondition(node.ID)
	cg.sb.WriteString(fmt.Sprintf("\tfor %s {\n", condition))

	cg.traverseBranch(node.ID, "loop")
	cg.sb.WriteString("\t}\n")
}

func (cg *CodeGenerator) traverseBranch(nodeID string, handleName string) {
	for _, edge := range cg.edges {
		if edge.Source == nodeID && (edge.Handle == handleName || handleName == "") {
			cg.traverseNode(edge.Target)
		}
	}
}
package generator

import (
	"fmt"
	"strings"
)

func (cg *CodeGenerator) findSourceValueForHandle(targetID, handle string) string {
	for _, edge := range cg.edges {
		if edge.Target == targetID && edge.Handle == handle {
			sourceNode := cg.nodes[edge.Source]
			if sourceNode.Type == "VarNode" {
				return fmt.Sprintf("%v", sourceNode.Data["name"])
			}
			if sourceNode.Type == "RequestParamsNode" {
				return fmt.Sprintf("%v", sourceNode.Data["requestParams"])
			}
		}
	}
	return "\"\""
}

func getNestedString(m map[string]interface{}, keys ...string) string {
	var current interface{} = m
	for _, key := range keys {
		currMap, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current = currMap[key]
	}
	if str, ok := current.(string); ok {
		return str
	}
	return ""
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
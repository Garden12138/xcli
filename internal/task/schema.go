package task

import (
	"encoding/json"
	"sort"
)

func object(properties map[string]any) map[string]any {
	required := make([]string, 0, len(properties))
	for key := range properties {
		required = append(required, key)
	}
	// Stable schema files make the native invocation and offline evidence
	// reproducible, independent of Go map iteration.
	sort.Strings(required)
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func roleSchema(role string) []byte {
	text := map[string]any{"type": "string"}
	texts := map[string]any{"type": "array", "items": text}
	var schema map[string]any
	switch role {
	case "coordinator":
		requirement := object(map[string]any{"path": text, "contains": texts, "sources": texts})
		kind := func(values ...string) map[string]any { return map[string]any{"type": "string", "enum": values} }
		action := map[string]any{"anyOf": []any{
			object(map[string]any{"type": kind("delegate"), "role": kind("worker", "reviewer"), "agent": text, "objective": text}),
			object(map[string]any{"type": kind("verify"), "argv": texts, "cwd": text}),
			object(map[string]any{"type": kind("revise_plan", "request_input"), "objective": text}),
			object(map[string]any{"type": kind("complete")}),
		}}
		schema = object(map[string]any{"version": map[string]any{"type": "integer", "enum": []int{1}}, "collaboration_required": map[string]any{"type": "boolean"}, "reason": text, "acceptance": map[string]any{"type": "array", "items": requirement}, "actions": map[string]any{"type": "array", "items": action}})
	case "reviewer":
		schema = object(map[string]any{"passed": map[string]any{"type": "boolean"}, "findings": texts, "coverage": texts, "sources": texts})
	default:
		schema = object(map[string]any{"summary": text, "facts": texts, "decisions": texts, "sources": texts, "unresolved": texts})
	}
	data, _ := json.Marshal(schema)
	return data
}

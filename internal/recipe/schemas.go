package recipe

import (
	"encoding/json"
	"sort"
)

func object(properties map[string]any) map[string]any {
	required := make([]string, 0, len(properties))
	for name := range properties {
		required = append(required, name)
	}
	sort.Strings(required)
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
}

func encoded(value any) []byte             { raw, _ := json.Marshal(value); return raw }
func str() map[string]any                  { return map[string]any{"type": "string"} }
func nullable() map[string]any             { return map[string]any{"type": []string{"string", "null"}} }
func enum(values ...string) map[string]any { return map[string]any{"type": "string", "enum": values} }

func ReviewSchema() []byte {
	return encoded(object(map[string]any{
		"verdict": enum("pass", "changes_requested"), "summary": str(),
		"issues": map[string]any{"type": "array", "items": object(map[string]any{
			"severity": enum("blocker", "major", "minor"), "file": nullable(), "description": str(),
		})},
	}))
}

func ImageResultSchema() []byte {
	return encoded(object(map[string]any{"files": map[string]any{"type": "array", "items": str()}}))
}

func PlanSchema(agents []string, maxSteps int) []byte {
	choices := []string{"auto"}
	seen := map[string]bool{"auto": true}
	for _, name := range agents {
		if !seen[name] {
			choices, seen[name] = append(choices, name), true
		}
	}
	return encoded(object(map[string]any{
		"reasoning": str(),
		"steps": map[string]any{"type": "array", "minItems": 1, "maxItems": maxSteps, "items": object(map[string]any{
			"id":   map[string]any{"type": "string", "pattern": "^[a-z0-9_-]{1,32}$"},
			"kind": enum("ask", "code", "review", "image"), "agent": enum(choices...), "instruction": str(),
			"depends_on": map[string]any{"type": "array", "items": str()}, "review_by": nullable(), "output": nullable(),
		})},
	}))
}

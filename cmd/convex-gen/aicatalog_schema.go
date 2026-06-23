package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// argsToJSONSchema converts a function's parsed args into a compact JSON Schema string.
func argsToJSONSchema(fn ConvexFunction) string {
	if fn.UseFunctionArgs {
		// Args too complex for the regex parser to model faithfully: permissive object.
		b, _ := json.Marshal(map[string]any{"type": "object", "additionalProperties": true})
		return string(b)
	}

	properties := map[string]any{}
	required := []string{}
	for _, a := range fn.Args {
		properties[a.Name] = argSchema(a)
		if !a.Optional {
			required = append(required, a.Name)
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	b, _ := json.Marshal(schema)
	return string(b)
}

func argSchema(a ArgInfo) map[string]any {
	switch {
	case a.IsArrayID:
		return map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string", "description": fmt.Sprintf("Id of a %q document", a.TableName)},
		}
	case a.IsID:
		return map[string]any{"type": "string", "description": fmt.Sprintf("Id of a %q document", a.TableName)}
	}

	t := a.Type
	switch {
	case strings.HasSuffix(t, "[]") || strings.HasPrefix(t, "Array<"):
		return map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	case strings.Contains(t, "number"):
		return map[string]any{"type": "number"}
	case strings.Contains(t, "boolean"):
		return map[string]any{"type": "boolean"}
	case strings.Contains(t, "string"):
		return map[string]any{"type": "string"}
	case strings.Contains(t, "{") || strings.Contains(t, "object"):
		return map[string]any{"type": "object", "additionalProperties": true}
	default:
		return map[string]any{} // unknown → any
	}
}

// fallbackDescription builds a deterministic description from the path + arg names.
func fallbackDescription(fnPath string, fn ConvexFunction) string {
	names := make([]string, 0, len(fn.Args))
	for _, a := range fn.Args {
		names = append(names, a.Name)
	}
	argStr := "none"
	if len(names) > 0 {
		argStr = strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s (%s). Arguments: %s.", fnPath, fn.Type, argStr)
}

// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package structured

// ToStrictWireSchema converts a standard JSON schema into OpenAI-compatible strict JSON schema.
// OpenAI structured outputs reject any schema whose required array does not list every property,
// or where additionalProperties is not false.
func ToStrictWireSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}

	cloned := deepCloneMap(schema)
	makeStrictRequired(cloned)
	return cloned
}

func allowsNull(node any) bool {
	m, ok := node.(map[string]any)
	if !ok {
		return false
	}
	if t, ok := m["type"].(string); ok && t == "null" {
		return true
	}
	if types, ok := m["type"].([]any); ok {
		for _, t := range types {
			if str, ok := t.(string); ok && str == "null" {
				return true
			}
		}
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		if list, ok := m[key].([]any); ok {
			for _, item := range list {
				if allowsNull(item) {
					return true
				}
			}
		}
	}
	return false
}

func makeStrictRequired(node any) {
	if node == nil {
		return
	}

	if list, ok := node.([]any); ok {
		for _, item := range list {
			makeStrictRequired(item)
		}
		return
	}

	m, ok := node.(map[string]any)
	if !ok {
		return
	}

	// If object type with properties
	if m["type"] == "object" {
		m["additionalProperties"] = false

		if props, ok := m["properties"].(map[string]any); ok {
			reqSet := make(map[string]bool)
			if existingReq, ok := m["required"].([]any); ok {
				for _, r := range existingReq {
					if str, ok := r.(string); ok {
						reqSet[str] = true
					}
				}
			}

			var allKeys []any
			for propName, propVal := range props {
				allKeys = append(allKeys, propName)
				if !reqSet[propName] {
					if !allowsNull(propVal) {
						props[propName] = map[string]any{
							"anyOf": []any{
								propVal,
								map[string]any{"type": "null"},
							},
						}
					}
				}
			}
			m["required"] = allKeys
		}
	}

	// Recurse into common schema keywords
	for _, key := range []string{"properties", "items", "anyOf", "oneOf", "allOf", "$defs", "definitions"} {
		if child, exists := m[key]; exists {
			if childMap, ok := child.(map[string]any); ok {
				for _, v := range childMap {
					makeStrictRequired(v)
				}
			} else if childList, ok := child.([]any); ok {
				for _, v := range childList {
					makeStrictRequired(v)
				}
			}
		}
	}
}

func deepCloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case map[string]any:
			out[k] = deepCloneMap(val)
		case []any:
			out[k] = deepCloneSlice(val)
		default:
			out[k] = val
		}
	}
	return out
}

func deepCloneSlice(s []any) []any {
	out := make([]any, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[string]any:
			out[i] = deepCloneMap(val)
		case []any:
			out[i] = deepCloneSlice(val)
		default:
			out[i] = val
		}
	}
	return out
}

// StripNullProps recursively strips nil/null properties from a JSON-like object so that lenient unmarshaling treats them as omitted/absent.
func StripNullProps(val any) any {
	switch v := val.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			if item == nil {
				continue
			}
			out[k] = StripNullProps(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = StripNullProps(item)
		}
		return out
	default:
		return v
	}
}


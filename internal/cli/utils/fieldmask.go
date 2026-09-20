// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"encoding/json"
	"strings"
)

// FIELD MASK PARSING & TARGET EXTRACTION

// ParseFieldList parses comma-separated field selectors into a slice of field paths.
func ParseFieldList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{})
	var fields []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			if _, exists := seen[trimmed]; !exists {
				seen[trimmed] = struct{}{}
				fields = append(fields, trimmed)
			}
		}
	}
	return fields
}

// DYNAMIC PAYLOAD FILTERING

// ApplyFieldMask filters an arbitrary object according to dot-separated field paths.
func ApplyFieldMask(obj any, fields []string) (any, error) {
	if len(fields) == 0 {
		return obj, nil
	}

	// Marshal and unmarshal into generic map/slice structure
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	return filterGeneric(raw, fields), nil
}

func filterGeneric(data any, fields []string) any {
	switch v := data.(type) {
	case map[string]any:
		res := make(map[string]any)
		nested := make(map[string][]string)

		for _, f := range fields {
			parts := strings.SplitN(f, ".", 2)
			key := parts[0]
			if len(parts) > 1 {
				nested[key] = append(nested[key], parts[1])
			} else {
				if val, ok := v[key]; ok {
					res[key] = val
				}
			}
		}

		for key, subFields := range nested {
			if val, ok := v[key]; ok {
				res[key] = filterGeneric(val, subFields)
			}
		}

		return res

	case []any:
		var res []any
		for _, item := range v {
			res = append(res, filterGeneric(item, fields))
		}
		return res

	default:
		return data
	}
}

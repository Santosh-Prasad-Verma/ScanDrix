// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MaskedProjector performs advanced nested filtering with support for slice wildcards (*).
type MaskedProjector struct {
	fieldPaths []string
}

// NewMaskedProjector creates a projector from a slice of dot-delimited field paths.
func NewMaskedProjector(paths []string) *MaskedProjector {
	var clean []string
	seen := make(map[string]struct{})
	for _, p := range paths {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			if _, ok := seen[trimmed]; !ok {
				seen[trimmed] = struct{}{}
				clean = append(clean, trimmed)
			}
		}
	}
	return &MaskedProjector{fieldPaths: clean}
}

// ProjectMap projects a map[string]any according to configured field paths.
func (mp *MaskedProjector) ProjectMap(input map[string]any) map[string]any {
	if len(mp.fieldPaths) == 0 {
		return input
	}
	return projectMapRecursive(input, mp.fieldPaths)
}

// ProjectJSON takes a raw JSON byte slice and returns the projected JSON representation.
func (mp *MaskedProjector) ProjectJSON(data []byte) ([]byte, error) {
	if len(mp.fieldPaths) == 0 {
		return data, nil
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON for field masking: %w", err)
	}

	projected := mp.ProjectAny(raw)
	return json.Marshal(projected)
}

// ProjectAny filters an arbitrary Go primitive, map, or slice against the field paths.
func (mp *MaskedProjector) ProjectAny(data any) any {
	if len(mp.fieldPaths) == 0 {
		return data
	}

	switch v := data.(type) {
	case map[string]any:
		return projectMapRecursive(v, mp.fieldPaths)
	case []any:
		var result []any
		for _, item := range v {
			result = append(result, mp.ProjectAny(item))
		}
		return result
	default:
		return data
	}
}

func projectMapRecursive(m map[string]any, paths []string) map[string]any {
	out := make(map[string]any)
	nestedMap := make(map[string][]string)

	for _, p := range paths {
		parts := strings.SplitN(p, ".", 2)
		head := parts[0]
		if len(parts) == 1 {
			if val, ok := m[head]; ok {
				out[head] = val
			}
		} else {
			nestedMap[head] = append(nestedMap[head], parts[1])
		}
	}

	for head, subPaths := range nestedMap {
		val, ok := m[head]
		if !ok {
			continue
		}

		switch subVal := val.(type) {
		case map[string]any:
			out[head] = projectMapRecursive(subVal, subPaths)
		case []any:
			var projectedSlice []any
			for _, elem := range subVal {
				if elemMap, ok := elem.(map[string]any); ok {
					projectedSlice = append(projectedSlice, projectMapRecursive(elemMap, subPaths))
				} else {
					projectedSlice = append(projectedSlice, elem)
				}
			}
			out[head] = projectedSlice
		default:
			out[head] = subVal
		}
	}

	return out
}

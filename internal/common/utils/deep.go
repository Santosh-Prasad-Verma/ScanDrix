package utils

import (
	"encoding/json"
	"reflect"
	"sort"
)

// DeepMerge merges multiple generic maps recursively into a new map.
func DeepMerge(objects ...map[string]any) map[string]any {
	if len(objects) == 0 {
		return make(map[string]any)
	}

	result := make(map[string]any)

	for _, obj := range objects {
		if obj == nil {
			continue
		}
		for key, val := range obj {
			if val == nil {
				continue
			}

			currVal, exists := result[key]
			if !exists {
				result[key] = cloneValue(val)
				continue
			}

			currMap, currIsMap := currVal.(map[string]any)
			newMap, newIsMap := val.(map[string]any)

			if currIsMap && newIsMap {
				result[key] = DeepMerge(currMap, newMap)
			} else {
				result[key] = cloneValue(val)
			}
		}
	}

	return result
}

// DeepDifference returns a map of keys in target that differ from base.
func DeepDifference(base, target map[string]any) map[string]any {
	result := make(map[string]any)
	if target == nil {
		return result
	}
	if base == nil {
		return DeepMerge(target)
	}

	for key, targetVal := range target {
		baseVal, exists := base[key]
		if !exists {
			result[key] = cloneValue(targetVal)
			continue
		}

		baseMap, baseIsMap := baseVal.(map[string]any)
		targetMap, targetIsMap := targetVal.(map[string]any)

		if baseIsMap && targetIsMap {
			nestedDiff := DeepDifference(baseMap, targetMap)
			if len(nestedDiff) > 0 {
				result[key] = nestedDiff
			}
			continue
		}

		baseSlice, baseIsSlice := baseVal.([]any)
		targetSlice, targetIsSlice := targetVal.([]any)

		if baseIsSlice && targetIsSlice {
			baseJSON, _ := json.Marshal(baseSlice)
			targetJSON, _ := json.Marshal(targetSlice)
			if string(baseJSON) != string(targetJSON) {
				result[key] = cloneValue(targetVal)
			}
			continue
		}

		if !reflect.DeepEqual(baseVal, targetVal) {
			result[key] = cloneValue(targetVal)
		}
	}

	return result
}

func cloneValue(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case map[string]any:
		res := make(map[string]any, len(val))
		for k, item := range val {
			res[k] = cloneValue(item)
		}
		return res
	case []any:
		res := make([]any, len(val))
		for i, item := range val {
			res[i] = cloneValue(item)
		}
		return res
	default:
		return val
	}
}

// DeepSortMap returns a canonically sorted representation of the input value.
func DeepSortMap(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]any:
		sortedKeys := make([]string, 0, len(val))
		for k := range val {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)

		sortedMap := make(map[string]any, len(val))
		for _, k := range sortedKeys {
			sortedMap[k] = DeepSortMap(val[k])
		}
		return sortedMap

	case []any:
		sortedSlice := make([]any, len(val))
		for i, item := range val {
			sortedSlice[i] = DeepSortMap(item)
		}
		return sortedSlice

	default:
		return val
	}
}

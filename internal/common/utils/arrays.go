package utils

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// JoinArrayValues joins elements of a slice into a comma-separated string.
func JoinArrayValues(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case []string:
		return strings.Join(val, ", ")
	case []any:
		strs := make([]string, len(val))
		for i, item := range val {
			strs[i] = fmt.Sprintf("%v", item)
		}
		return strings.Join(strs, ", ")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ArraysHaveSameValues checks if two slices contain equal elements regardless of order.
func ArraysHaveSameValues(arr1, arr2 []string) bool {
	if len(arr1) != len(arr2) {
		return false
	}
	counts := make(map[string]int)
	for _, x := range arr1 {
		counts[x]++
	}
	for _, x := range arr2 {
		counts[x]--
		if counts[x] < 0 {
			return false
		}
	}
	return true
}

// ConvertArrayToJSONL converts a slice of objects into newline-delimited JSON.
func ConvertArrayToJSONL(slice any) (string, error) {
	val := reflect.ValueOf(slice)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return "", fmt.Errorf("input must be a slice or array")
	}

	var sb strings.Builder
	for i := 0; i < val.Len(); i++ {
		bytes, err := json.Marshal(val.Index(i).Interface())
		if err != nil {
			return "", err
		}
		sb.Write(bytes)
		if i < val.Len()-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String(), nil
}

// ConvertJSONToJSONL walks an arbitrarily nested object/map and returns a JSONL representation.
func ConvertJSONToJSONL(data map[string]any) string {
	var lines []string

	var recurse func(obj any, parentKey string)
	recurse = func(obj any, parentKey string) {
		switch val := obj.(type) {
		case map[string]any:
			for k, v := range val {
				curKey := k
				if parentKey != "" {
					curKey = parentKey + "." + k
				}
				recurse(v, curKey)
			}
		case []any:
			for i, v := range val {
				curKey := fmt.Sprintf("%s[%d]", parentKey, i)
				recurse(v, curKey)
			}
		default:
			entry := map[string]any{parentKey: obj}
			bytes, _ := json.Marshal(entry)
			lines = append(lines, string(bytes))
		}
	}

	recurse(data, "")
	return strings.Join(lines, "\n")
}

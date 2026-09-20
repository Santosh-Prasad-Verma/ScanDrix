package utils

import (
	"reflect"
	"testing"
)

func TestDeepMerge(t *testing.T) {
	o1 := map[string]any{
		"a": "1",
		"b": map[string]any{
			"c": 10,
			"d": 20,
		},
	}
	o2 := map[string]any{
		"b": map[string]any{
			"d": 99,
			"e": 30,
		},
		"f": true,
	}

	merged := DeepMerge(o1, o2)

	if merged["a"] != "1" || merged["f"] != true {
		t.Errorf("failed primitive merge")
	}

	bMap, ok := merged["b"].(map[string]any)
	if !ok || bMap["c"] != 10 || bMap["d"] != 99 || bMap["e"] != 30 {
		t.Errorf("failed nested map merge: %+v", bMap)
	}
}

func TestDeepDifference(t *testing.T) {
	base := map[string]any{
		"a": "1",
		"b": map[string]any{"x": 10, "y": 20},
		"arr": []any{"item1"},
	}
	target := map[string]any{
		"a": "1", // same
		"b": map[string]any{"x": 10, "y": 99}, // y changed
		"c": "new", // added
		"arr": []any{"item1", "item2"}, // changed
	}

	diff := DeepDifference(base, target)

	if _, exists := diff["a"]; exists {
		t.Errorf("expected identical key 'a' to not be in diff")
	}
	if diff["c"] != "new" {
		t.Errorf("expected added key 'c' to be in diff")
	}

	bDiff, ok := diff["b"].(map[string]any)
	if !ok || bDiff["y"] != 99 {
		t.Errorf("failed nested diff: %+v", bDiff)
	}
	if _, exists := bDiff["x"]; exists {
		t.Errorf("expected unchanged nested key 'x' to be omitted")
	}
}

func TestDeepSortMap(t *testing.T) {
	m := map[string]any{
		"z": 1,
		"a": map[string]any{
			"y": 2,
			"b": 3,
		},
	}
	sorted := DeepSortMap(m)
	if !reflect.DeepEqual(sorted, m) {
		t.Errorf("deep sort altered semantics")
	}
}

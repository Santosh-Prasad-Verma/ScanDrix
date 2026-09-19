package utils

import (
	"strings"
	"testing"
)

func TestJoinArrayValues(t *testing.T) {
	if JoinArrayValues([]string{"a", "b", "c"}) != "a, b, c" {
		t.Errorf("failed string slice join")
	}
	if JoinArrayValues([]any{1, "two", 3.0}) != "1, two, 3" {
		t.Errorf("failed any slice join")
	}
	if JoinArrayValues("single") != "single" {
		t.Errorf("failed scalar fallback")
	}
	if JoinArrayValues(nil) != "" {
		t.Errorf("failed nil handling")
	}
}

func TestArraysHaveSameValues(t *testing.T) {
	if !ArraysHaveSameValues([]string{"a", "b", "c"}, []string{"c", "a", "b"}) {
		t.Errorf("expected true for permutation")
	}
	if ArraysHaveSameValues([]string{"a", "b"}, []string{"a", "b", "c"}) {
		t.Errorf("expected false for different lengths")
	}
	if ArraysHaveSameValues([]string{"a", "a"}, []string{"a", "b"}) {
		t.Errorf("expected false for different elements")
	}
}

func TestConvertArrayToJSONL(t *testing.T) {
	items := []map[string]any{
		{"id": "1", "name": "first"},
		{"id": "2", "name": "second"},
	}
	jsonl, err := ConvertArrayToJSONL(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(jsonl, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
}

func TestConvertJSONToJSONL(t *testing.T) {
	data := map[string]any{
		"user": map[string]any{
			"name": "Alice",
		},
		"tags": []any{"dev", "admin"},
	}
	jsonl := ConvertJSONToJSONL(data)
	if !strings.Contains(jsonl, `"user.name":"Alice"`) {
		t.Errorf("missing user.name line in jsonl: %s", jsonl)
	}
	if !strings.Contains(jsonl, `"tags[0]":"dev"`) {
		t.Errorf("missing tags[0] line in jsonl: %s", jsonl)
	}
}

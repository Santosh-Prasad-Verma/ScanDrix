package gateway

import (
	"context"
	"testing"
)

func TestGetBuiltinTools(t *testing.T) {
	tools, handlers := GetBuiltinTools()
	if len(tools) != 4 {
		t.Fatalf("expected 4 builtin tools, got %d", len(tools))
	}

	expectedNames := map[string]bool{
		"scandrix_analyze_snippet":          false,
		"scandrix_catalog_search":           false,
		"scandrix_validate_syntax":          false,
		"scandrix_lookup_osv_vulnerability": false,
	}

	for _, tool := range tools {
		if _, ok := expectedNames[tool.Name]; ok {
			expectedNames[tool.Name] = true
		}
		if _, ok := handlers[tool.Name]; !ok {
			t.Errorf("missing handler for tool: %s", tool.Name)
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("tool %s was not found in tool list", name)
		}
	}
}

func TestHandleLookupOSV_Validation(t *testing.T) {
	ctx := context.Background()

	// Missing all params
	res, err := handleLookupOSV(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected error result when missing package and commit")
	}

	// Valid package query schema
	res, err = handleLookupOSV(ctx, map[string]any{
		"package":   "nonexistent-test-package-xyz-12345",
		"version":   "0.0.1",
		"ecosystem": "npm",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || len(res.Content) == 0 {
		t.Fatalf("expected non-empty response content")
	}
}

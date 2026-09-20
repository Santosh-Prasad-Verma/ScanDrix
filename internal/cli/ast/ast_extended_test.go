// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ast

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	cases := []struct {
		path     string
		expected Language
	}{
		{"main.go", LangGo},
		{"service.ts", LangTypeScript},
		{"component.tsx", LangTypeScript},
		{"index.js", LangJavaScript},
		{"view.jsx", LangJavaScript},
		{"module.mjs", LangJavaScript},
		{"script.cjs", LangJavaScript},
		{"app.py", LangPython},
		{"config.yaml", LangUnknown},
		{"README.md", LangUnknown},
	}

	for _, c := range cases {
		got := DetectLanguage(c.path)
		if got != c.expected {
			t.Errorf("DetectLanguage(%q) = %v, expected %v", c.path, got, c.expected)
		}
	}
}

func TestParseGoFile_ComplexControlFlowAndSmells(t *testing.T) {
	tempDir := t.TempDir()
	// Create a deeply nested and complex function that triggers code smells
	code := `package main

func OverlyComplexFunction(x, y, z int) int {
	result := 0
	if x > 0 {
		if y > 0 {
			if z > 0 {
				if x == y {
					if y == z {
						result = 100
					}
				}
			}
		}
	}
	switch x {
	case 1:
		result += 1
	case 2:
		result += 2
	case 3:
		result += 3
	case 4:
		result += 4
	case 5:
		result += 5
	case 6:
		result += 6
	case 7:
		result += 7
	case 8:
		result += 8
	case 9:
		result += 9
	case 10:
		result += 10
	case 11:
		result += 11
	case 12:
		result += 12
	default:
		result += 0
	}
	return result
}
`
	filePath := filepath.Join(tempDir, "complex.go")
	if err := os.WriteFile(filePath, []byte(code), 0600); err != nil {
		t.Fatalf("failed creating test file: %v", err)
	}

	analysis, err := ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if len(analysis.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(analysis.Functions))
	}

	fn := analysis.Functions[0]
	if fn.Name != "OverlyComplexFunction" {
		t.Errorf("expected function name OverlyComplexFunction, got %s", fn.Name)
	}
	if fn.NestingDepth < 5 {
		t.Errorf("expected nesting depth >= 5, got %d", fn.NestingDepth)
	}

	// Should have detected code smells (high complexity and/or deep nesting)
	if len(analysis.Smells) == 0 {
		t.Errorf("expected code smells to be detected, got 0")
	}

	hasDeepNesting := false
	for _, smell := range analysis.Smells {
		if smell.Type == SmellDeepNesting {
			hasDeepNesting = true
			break
		}
	}
	if !hasDeepNesting {
		t.Errorf("expected SmellDeepNesting in detected smells: %+v", analysis.Smells)
	}
}

func TestParseJSFile_AsyncAndArrowFunctions(t *testing.T) {
	tempDir := t.TempDir()
	jsCode := `
async function fetchUserData(userId) {
    if (!userId) {
        throw new Error("Invalid user ID");
    }
    const res = await api.get('/users/' + userId);
    return res.data;
}

const formatName = (first, last) => {
    return first + ' ' + last;
};
`
	filePath := filepath.Join(tempDir, "user.ts")
	if err := os.WriteFile(filePath, []byte(jsCode), 0600); err != nil {
		t.Fatalf("failed creating test file: %v", err)
	}

	analysis, err := ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if analysis.Language != LangTypeScript {
		t.Errorf("expected LangTypeScript, got %v", analysis.Language)
	}
	if len(analysis.Functions) < 1 {
		t.Fatalf("expected at least 1 function, got %d", len(analysis.Functions))
	}
}

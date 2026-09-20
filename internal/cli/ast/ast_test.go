// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ast

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGoFile(t *testing.T) {
	tempDir := t.TempDir()
	goCode := `package example

type Service struct{}

func (s *Service) ProcessData(items []int) int {
	total := 0
	for _, item := range items {
		if item > 0 {
			if item%2 == 0 {
				total += item
			} else {
				total -= item
			}
		}
	}
	return total
}

func SimpleHelper() string {
	return "ok"
}
`
	filePath := filepath.Join(tempDir, "service.go")
	_ = os.WriteFile(filePath, []byte(goCode), 0600)

	analysis, err := ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if analysis.Language != LangGo {
		t.Errorf("expected LangGo, got %v", analysis.Language)
	}
	if len(analysis.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(analysis.Functions))
	}

	fn := analysis.Functions[0]
	if fn.Name != "ProcessData" {
		t.Errorf("expected ProcessData, got %s", fn.Name)
	}
	if fn.Receiver == "" {
		t.Errorf("expected receiver for method ProcessData")
	}
	if fn.CyclomaticComplexity < 4 {
		t.Errorf("expected cyclomatic complexity >= 4 for ProcessData, got %d", fn.CyclomaticComplexity)
	}
	if fn.NestingDepth < 3 {
		t.Errorf("expected nesting depth >= 3, got %d", fn.NestingDepth)
	}

	fn2 := analysis.Functions[1]
	if fn2.Name != "SimpleHelper" || !fn2.IsExported {
		t.Errorf("expected exported SimpleHelper, got %+v", fn2)
	}
}

func TestParsePythonFile(t *testing.T) {
	tempDir := t.TempDir()
	pyCode := `def calculate(a, b):
    if a > 0:
        if b > 0:
            return a + b
    return 0

def helper():
    pass
`
	filePath := filepath.Join(tempDir, "calc.py")
	_ = os.WriteFile(filePath, []byte(pyCode), 0600)

	analysis, err := ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile python failed: %v", err)
	}

	if analysis.Language != LangPython {
		t.Errorf("expected LangPython, got %v", analysis.Language)
	}
	if len(analysis.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(analysis.Functions))
	}

	fn := analysis.Functions[0]
	if fn.Name != "calculate" {
		t.Errorf("expected calculate, got %s", fn.Name)
	}
	if fn.CyclomaticComplexity < 2 {
		t.Errorf("expected cyclomatic >= 2, got %d", fn.CyclomaticComplexity)
	}
}

func TestCodeSmells_Detection(t *testing.T) {
	tempDir := t.TempDir()
	complexCode := `package complex

func Spaghetti() {
	if true {
		if true {
			if true {
				if true {
					if true {
						println("deep")
					}
				}
			}
		}
	}
}
`
	filePath := filepath.Join(tempDir, "spaghetti.go")
	_ = os.WriteFile(filePath, []byte(complexCode), 0600)

	analysis, err := ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if len(analysis.Smells) == 0 {
		t.Fatalf("expected deep nesting code smell to be detected")
	}

	deepNestingFound := false
	for _, smell := range analysis.Smells {
		if smell.Type == SmellDeepNesting {
			deepNestingFound = true
			if !strings.Contains(smell.Description, "Spaghetti") {
				t.Errorf("expected smell description to name function Spaghetti")
			}
		}
	}

	if !deepNestingFound {
		t.Errorf("expected SmellDeepNesting to be detected")
	}
}

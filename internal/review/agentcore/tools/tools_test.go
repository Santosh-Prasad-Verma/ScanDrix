package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

type mockSandbox struct {
	execFunc func(ctx context.Context, cmd string) (stdout, stderr string, exitCode int, err error)
}

func (m *mockSandbox) Exec(ctx context.Context, cmd string) (stdout, stderr string, exitCode int, err error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, cmd)
	}
	return "", "", 0, nil
}

type mockFS struct {
	files map[string]string
}

func (m *mockFS) ReadFile(ctx context.Context, path string, startLine, endLine int) (string, error) {
	content, ok := m.files[path]
	if !ok {
		return "", fmt.Errorf("file not found: %s", path)
	}
	lines := strings.Split(content, "\n")
	if startLine > 0 || endLine > 0 {
		sl := startLine
		if sl < 1 {
			sl = 1
		}
		el := endLine
		if el <= 0 || el > len(lines) {
			el = len(lines)
		}
		if sl > len(lines) {
			return "", nil
		}
		return strings.Join(lines[sl-1:el], "\n"), nil
	}
	return content, nil
}

func (m *mockFS) ListDir(ctx context.Context, path string) ([]string, error) {
	res := make([]string, 0)
	for k := range m.files {
		if strings.HasPrefix(k, path) {
			res = append(res, k)
		}
	}
	return res, nil
}

func (m *mockFS) Grep(ctx context.Context, query, path string) (string, error) {
	var sb strings.Builder
	for file, content := range m.files {
		for i, line := range strings.Split(content, "\n") {
			if strings.Contains(line, query) {
				sb.WriteString(fmt.Sprintf("%s:%d:%s\n", file, i+1, line))
			}
		}
	}
	return sb.String(), nil
}

func (m *mockFS) GetCallers(ctx context.Context, symbol, file string) (string, error) {
	return fmt.Sprintf("Caller of %s:\n  ← pkg/api.go:42", symbol), nil
}

func (m *mockFS) GitDiff(ctx context.Context, path string) (string, error) {
	return "+ new added line", nil
}

type mockGit struct{}

func (m *mockGit) Blame(ctx context.Context, path string, startLine, endLine int) (string, error) {
	return fmt.Sprintf("a1b2c3d (Alice 2026-09-17 %d) code line", startLine), nil
}

func (m *mockGit) Log(ctx context.Context, path string, maxCommits int) (string, error) {
	return "f4e3d2c feat(auth): add token rotation\nb5a4c3d fix(concurrency): avoid mutex race", nil
}

func (m *mockGit) DiffRange(ctx context.Context, baseRef, headRef, path string) (string, error) {
	return "diff --git a/pkg/api.go b/pkg/api.go\n+ func NewHandler() {}", nil
}

type mockFetcher struct{}

func (m *mockFetcher) FetchFile(ctx context.Context, repo, path, ref string) (string, error) {
	return "# Standard Coding Guidelines\nNever use global state.", nil
}

type mockDocsAdapter struct{}

func (m *mockDocsAdapter) Search(ctx context.Context, packageName, query string) ([]DocumentationSnippet, error) {
	return []DocumentationSnippet{
		{
			Title:   "TypeORM Transaction Isolation",
			URL:     "https://typeorm.io/transactions",
			Query:   query,
			Snippet: "Transactions can be rolled back using queryRunner.rollbackTransaction()",
			Source:  "typeorm-docs",
		},
	}, nil
}

func TestComplete18ToolsSuite(t *testing.T) {
	ctx := context.Background()
	toolCtx := contracts.ToolContext{
		RunID:   "test-run-101",
		Context: ctx,
	}

	fs := &mockFS{
		files: map[string]string{
			"pkg/service.go": "package pkg\n\nimport (\n\t\"fmt\"\n)\n\ntype Service struct{}\n\nfunc (s *Service) ProcessRefund(amount int) error {\n\treturn nil\n}\n",
			"cmd/main.go":    "package main\n\nimport \"pkg\"\n\nfunc main() {\n\ts := &pkg.Service{}\n\ts.ProcessRefund(100)\n}\n",
			"pkg/vulnerable.go": "package pkg\n\nimport \"fmt\"\n\nfunc RunQuery(id string) {\n\tquery := fmt.Sprintf(\"SELECT * FROM users WHERE id = '%s'\", id)\n\t_ = query\n}\n",
		},
	}

	sandbox := &mockSandbox{
		execFunc: func(ctx context.Context, cmd string) (stdout, stderr string, exitCode int, err error) {
			if strings.Contains(cmd, "rg") {
				if strings.Contains(cmd, "ProcessRefund") {
					return "pkg/service.go:8:func (s *Service) ProcessRefund(amount int) error {\n", "", 0, nil
				}
				return "", "", 1, nil
			}
			if strings.Contains(cmd, "cat") || strings.Contains(cmd, "sed") {
				for path, content := range fs.files {
					if strings.Contains(cmd, path) {
						return content, "", 0, nil
					}
				}
				return "", "file not found", 1, nil
			}
			if strings.Contains(cmd, "find") || strings.Contains(cmd, "fd") {
				if strings.Contains(cmd, "findFile") || strings.Contains(cmd, "service") {
					return "pkg/service.go\n", "", 0, nil
				}
				var sb strings.Builder
				for k := range fs.files {
					sb.WriteString(k + "\n")
				}
				return sb.String(), "", 0, nil
			}
			if strings.Contains(cmd, "[ -f") {
				return "pkg/service.go\ncmd/main.go\n", "", 0, nil
			}
			if strings.Contains(cmd, "go vet") || strings.Contains(cmd, "go build") {
				return "", "", 0, nil
			}
			if strings.Contains(cmd, "pytest") {
				return "1 passed in 0.05s", "", 0, nil
			}
			return "", "", 0, nil
		},
	}

	callGraph := "ProcessRefund\n  ← cmd/main.go:6 (main)\n  → fmt.Println\n"

	registry, cache, metrics := BuildCompleteToolRegistry(ReviewToolOptions{
		Sandbox:              sandbox,
		FS:                   fs,
		Git:                  &mockGit{},
		ReferenceFetcher:     &mockFetcher{},
		DocumentationAdapter: &mockDocsAdapter{},
		CallGraphData:        callGraph,
		EnableCaching:        true,
		EnableOutlineFirst:   true,
	})

	if len(registry.List()) != 18 {
		t.Fatalf("expected 18 registered tools, got %d", len(registry.List()))
	}

	t.Run("Tool 1: grep", func(t *testing.T) {
		tool, ok := registry.Get("grep")
		if !ok {
			t.Fatal("grep tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"pattern": "ProcessRefund",
		})
		if err != nil || res.IsError {
			t.Fatalf("grep execution failed: %v, output: %s", err, res.Output)
		}
		if !strings.Contains(res.Output, "ProcessRefund") {
			t.Errorf("expected grep output to contain match, got: %s", res.Output)
		}
	})

	t.Run("Tool 2: readFile", func(t *testing.T) {
		tool, ok := registry.Get("readFile")
		if !ok {
			t.Fatal("readFile tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"path":      "pkg/service.go",
			"startLine": 1,
			"endLine":   5,
		})
		if err != nil || res.IsError {
			t.Fatalf("readFile failed: %v", err)
		}
		if !strings.Contains(res.Output, "1: package pkg") {
			t.Errorf("expected line numbered output, got: %s", res.Output)
		}

		// Test near-miss suggestions
		resMiss, _ := tool.Execute(toolCtx, map[string]any{
			"path": "pkg/servce.go", // typo
		})
		if !strings.Contains(resMiss.Output, "Did you mean:") {
			t.Errorf("expected near-miss suggestion for typo, got: %s", resMiss.Output)
		}
	})

	t.Run("Tool 3: listDir", func(t *testing.T) {
		tool, ok := registry.Get("listDir")
		if !ok {
			t.Fatal("listDir tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"path": "pkg",
		})
		if err != nil || res.IsError {
			t.Fatalf("listDir failed: %v", err)
		}
		if !strings.Contains(res.Output, "pkg/service.go") {
			t.Errorf("expected listDir to include pkg/service.go, got: %s", res.Output)
		}
	})

	t.Run("Tool 4: findFile", func(t *testing.T) {
		tool, ok := registry.Get("findFile")
		if !ok {
			t.Fatal("findFile tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"pattern": "service",
		})
		if err != nil || res.IsError {
			t.Fatalf("findFile failed: %v", err)
		}
		if !strings.Contains(res.Output, "pkg/service.go") {
			t.Errorf("expected findFile to match, got: %s", res.Output)
		}
	})

	t.Run("Tool 5: checkTypes", func(t *testing.T) {
		tool, ok := registry.Get("checkTypes")
		if !ok {
			t.Fatal("checkTypes tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"path": "pkg/service.go",
		})
		if err != nil || res.IsError {
			t.Fatalf("checkTypes failed: %v", err)
		}
		if !strings.Contains(res.Output, "no type errors") {
			t.Errorf("expected clean build report, got: %s", res.Output)
		}
	})

	t.Run("Tool 6: getCallers", func(t *testing.T) {
		tool, ok := registry.Get("getCallers")
		if !ok {
			t.Fatal("getCallers tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"functionName": "ProcessRefund",
		})
		if err != nil || res.IsError {
			t.Fatalf("getCallers failed: %v", err)
		}
		if !strings.Contains(res.Output, "← cmd/main.go:6") {
			t.Errorf("expected caller trace, got: %s", res.Output)
		}
	})

	t.Run("Tool 7: readReference", func(t *testing.T) {
		tool, ok := registry.Get("readReference")
		if !ok {
			t.Fatal("readReference tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"repo": "org/shared-standards",
			"path": "guidelines.md",
		})
		if err != nil || res.IsError {
			t.Fatalf("readReference failed: %v", err)
		}
		if !strings.Contains(res.Output, "Never use global state") {
			t.Errorf("expected reference content, got: %s", res.Output)
		}
	})

	t.Run("Tool 8: searchDocs", func(t *testing.T) {
		tool, ok := registry.Get("searchDocs")
		if !ok {
			t.Fatal("searchDocs tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"packageName": "typeorm",
			"query":       "transaction rollback",
		})
		if err != nil || res.IsError {
			t.Fatalf("searchDocs failed: %v", err)
		}
		if !strings.Contains(res.Output, "TypeORM Transaction Isolation") {
			t.Errorf("expected snippet hit, got: %s", res.Output)
		}
	})

	t.Run("Tool 9: submitResult", func(t *testing.T) {
		tool, ok := registry.Get("submitResult")
		if !ok {
			t.Fatal("submitResult tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"reasoning": "Audit completed clean",
			"suggestions": []any{
				map[string]any{
					"relevantFile":      "pkg/service.go",
					"suggestionContent": "WHAT: Add validation\nWHY: Prevent zero refund\nHOW: if amount <= 0 return error",
					"existingCode":      "func (s *Service) ProcessRefund(amount int) error {",
					"improvedCode":      "func (s *Service) ProcessRefund(amount int) error {\n\tif amount <= 0 { return fmt.Errorf(\"invalid amount\") }",
					"severity":          "high",
				},
			},
		})
		if err != nil || res.IsError {
			t.Fatalf("submitResult failed: %v", err)
		}
		if !strings.Contains(res.Output, "Recorded 1 verified findings") {
			t.Errorf("expected result confirmation, got: %s", res.Output)
		}
	})

	t.Run("Tool 10: submitVerdict", func(t *testing.T) {
		tool, ok := registry.Get("submitVerdict")
		if !ok {
			t.Fatal("submitVerdict tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"keep":       true,
			"rationale":  "Verified through caller analysis",
			"confidence": "high",
		})
		if err != nil || res.IsError {
			t.Fatalf("submitVerdict failed: %v", err)
		}
		if !strings.Contains(res.Output, "candidate kept") {
			t.Errorf("expected verdict kept confirmation, got: %s", res.Output)
		}
	})

	t.Run("Tool 11: gitBlame", func(t *testing.T) {
		tool, ok := registry.Get("gitBlame")
		if !ok {
			t.Fatal("gitBlame tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"path":      "pkg/service.go",
			"startLine": 1,
			"endLine":   5,
		})
		if err != nil || res.IsError {
			t.Fatalf("gitBlame failed: %v", err)
		}
		if !strings.Contains(res.Output, "Alice") {
			t.Errorf("expected author in blame, got: %s", res.Output)
		}
	})

	t.Run("Tool 12: gitLog", func(t *testing.T) {
		tool, ok := registry.Get("gitLog")
		if !ok {
			t.Fatal("gitLog tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"path": "pkg/service.go",
		})
		if err != nil || res.IsError {
			t.Fatalf("gitLog failed: %v", err)
		}
		if !strings.Contains(res.Output, "feat(auth)") {
			t.Errorf("expected commit message in log, got: %s", res.Output)
		}
	})

	t.Run("Tool 13: gitDiffRange", func(t *testing.T) {
		tool, ok := registry.Get("gitDiffRange")
		if !ok {
			t.Fatal("gitDiffRange tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"baseRef": "main",
			"headRef": "HEAD",
		})
		if err != nil || res.IsError {
			t.Fatalf("gitDiffRange failed: %v", err)
		}
		if !strings.Contains(res.Output, "diff --git") {
			t.Errorf("expected diff header, got: %s", res.Output)
		}
	})

	t.Run("Tool 14: runTests", func(t *testing.T) {
		tool, ok := registry.Get("runTests")
		if !ok {
			t.Fatal("runTests tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"path": "tests/unit/test_api.py",
		})
		if err != nil || res.IsError {
			t.Fatalf("runTests failed: %v", err)
		}
		if !strings.Contains(res.Output, "PASS") {
			t.Errorf("expected test pass output, got: %s", res.Output)
		}
	})

	t.Run("Tool 15: symbolOutline", func(t *testing.T) {
		tool, ok := registry.Get("symbolOutline")
		if !ok {
			t.Fatal("symbolOutline tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"path": "pkg/service.go",
		})
		if err != nil || res.IsError {
			t.Fatalf("symbolOutline failed: %v", err)
		}
		if !strings.Contains(res.Output, "ProcessRefund") {
			t.Errorf("expected symbol outline, got: %s", res.Output)
		}
	})

	t.Run("Tool 16: astQuery", func(t *testing.T) {
		tool, ok := registry.Get("astQuery")
		if !ok {
			t.Fatal("astQuery tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"path":      "pkg/service.go",
			"queryType": "func",
			"name":      "ProcessRefund",
		})
		if err != nil || res.IsError {
			t.Fatalf("astQuery failed: %v", err)
		}
		if !strings.Contains(res.Output, "func (receiver) ProcessRefund()") {
			t.Errorf("expected AST method match, got: %s", res.Output)
		}
	})

	t.Run("Tool 17: importGraph", func(t *testing.T) {
		tool, ok := registry.Get("importGraph")
		if !ok {
			t.Fatal("importGraph tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"path": "pkg/service.go",
		})
		if err != nil || res.IsError {
			t.Fatalf("importGraph failed: %v", err)
		}
		if !strings.Contains(res.Output, "fmt") {
			t.Errorf("expected import graph to contain fmt, got: %s", res.Output)
		}
	})

	t.Run("Tool 18: securityScanner", func(t *testing.T) {
		tool, ok := registry.Get("securityScanner")
		if !ok {
			t.Fatal("securityScanner tool not found")
		}
		res, err := tool.Execute(toolCtx, map[string]any{
			"path": "pkg/vulnerable.go",
		})
		if err != nil || res.IsError {
			t.Fatalf("securityScanner failed: %v", err)
		}
		if !strings.Contains(res.Output, "SQL Injection") {
			t.Errorf("expected SQL injection hotspot alert, got: %s", res.Output)
		}
	})

	// Verify caching and metrics
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
	snap := metrics.Snapshot()
	if len(snap) == 0 {
		t.Error("expected metrics snapshot to record tool calls")
	}
}

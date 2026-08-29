package advanced_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/rules/advanced"
)

func TestRuleCompilerAndScanning(t *testing.T) {
	compiler := advanced.NewRuleCompiler()
	catalog := advanced.BuiltinEnterpriseCatalog()

	var detectors []*advanced.CompiledDetector
	for _, rule := range catalog {
		d, err := compiler.CompileRule(rule)
		if err != nil {
			t.Fatalf("failed compiling rule %s: %v", rule.RuleKey, err)
		}
		detectors = append(detectors, d)
	}

	// 1. Scan Vulnerable Code Sample
	vulnerableCode := `package main

import (
	"crypto/tls"
	"os/exec"
)

func Handler(req string) {
	// CWE-89: SQL Injection
	q := "SELECT * FROM users WHERE id = " + req

	// CWE-78: Command Injection
	cmd := exec.Command("bash", "-c", "echo " + req)

	// CWE-798: Hardcoded Secret
	apiKey := "AKIAIOSFODNN7EXAMPLE"

	// CWE-295: Disabled TLS
	cfg := &tls.Config{
		InsecureSkipVerify: true,
	}
}
`

	findings := advanced.ScanText(detectors, "pkg/handler/vulnerable.go", vulnerableCode)
	if len(findings) < 4 {
		t.Fatalf("expected at least 4 security findings, got %d", len(findings))
	}

	foundCWEs := make(map[string]bool)
	for _, f := range findings {
		foundCWEs[f.CWE] = true
	}

	expectedCWEs := []string{"CWE-89", "CWE-78", "CWE-798", "CWE-295"}
	for _, cwe := range expectedCWEs {
		if !foundCWEs[cwe] {
			t.Fatalf("expected detection for %s, but was not found", cwe)
		}
	}

	// 2. Scan Clean Code -> Zero Findings
	cleanCode := `package main

import (
	"database/sql"
	"os/exec"
)

func SafeHandler(db *sql.DB, id string) error {
	row := db.QueryRow("SELECT * FROM users WHERE id = $1", id)
	cmd := exec.Command("echo", id)
	return cmd.Run()
}
`
	cleanFindings := advanced.ScanText(detectors, "pkg/handler/safe.go", cleanCode)
	if len(cleanFindings) != 0 {
		t.Fatalf("expected 0 findings for safe code, got %d", len(cleanFindings))
	}
}

func TestRuleDetectorSweeperBudgetAndIdempotency(t *testing.T) {
	compiler := advanced.NewRuleCompiler()
	catalog := advanced.BuiltinEnterpriseCatalog()

	// Sweeper with small budget of 3
	sweeper := advanced.NewRuleDetectorSweeper(compiler, 3)
	wsID := uuid.New()
	sweeper.RegisterWorkspaceRules(wsID, catalog)

	ctx := context.Background()

	// 1. First run: should compile exactly 3 (budget cap hit)
	res1 := sweeper.RunSweep(ctx, true)
	if res1.TotalProcessed != 3 || res1.TotalCompiled != 3 {
		t.Fatalf("expected 3 processed and 3 compiled due to budget limit, got %+v", res1)
	}

	// 2. Second run with larger budget
	sweeperFull := advanced.NewRuleDetectorSweeper(compiler, 100)
	sweeperFull.RegisterWorkspaceRules(wsID, catalog)

	resFull := sweeperFull.RunSweep(ctx, true)
	if resFull.TotalCompiled != len(catalog) {
		t.Fatalf("expected %d compiled, got %d", len(catalog), resFull.TotalCompiled)
	}

	// 3. Steady-state run (onlyMissing=true) -> 0 compiled, 0 errored
	resSteady := sweeperFull.RunSweep(ctx, true)
	if resSteady.TotalCompiled != 0 {
		t.Fatalf("steady-state run should compile 0 new rules, got %d", resSteady.TotalCompiled)
	}
}

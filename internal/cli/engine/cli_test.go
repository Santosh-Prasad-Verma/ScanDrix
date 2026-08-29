package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/pkg/models"
)

func TestCLIRunnerAndOutputFormats(t *testing.T) {
	ctx := context.Background()
	runner := engine.NewCLIRunner()
	formatter := engine.NewOutputFormatter()

	// 1. Test Clean Diff
	cleanDiff := `diff --git a/pkg/calc.go b/pkg/calc.go
--- a/pkg/calc.go
+++ b/pkg/calc.go
@@ -1,3 +1,4 @@
 package pkg
+func Multiply(a, b int) int { return a * b }
`
	cleanResult, err := runner.RunReview(ctx, cleanDiff, engine.CLIOptions{
		SeverityThreshold: models.SeverityHigh,
	})
	if err != nil {
		t.Fatalf("clean review failed: %v", err)
	}
	if cleanResult.TotalFindings != 0 || cleanResult.ExitCode != 0 {
		t.Fatalf("expected 0 findings and exit code 0, got %+v", cleanResult)
	}

	// 2. Test Vulnerable Diff
	vulnDiff := `diff --git a/pkg/auth.go b/pkg/auth.go
--- a/pkg/auth.go
+++ b/pkg/auth.go
@@ -10,3 +10,4 @@
 package pkg
+const AwsSecret = "AKIAIOSFODNN7EXAMPLE"
`
	vulnResult, err := runner.RunReview(ctx, vulnDiff, engine.CLIOptions{
		SeverityThreshold: models.SeverityHigh,
	})
	if err != nil {
		t.Fatalf("vuln review failed: %v", err)
	}
	if vulnResult.TotalFindings != 1 || vulnResult.ExitCode != 1 || !vulnResult.IsBlocking {
		t.Fatalf("expected 1 finding and exit code 1, got %+v", vulnResult)
	}

	// 3. Test Dry-Run (Findings present, ExitCode must be 0)
	dryRunResult, err := runner.RunReview(ctx, vulnDiff, engine.CLIOptions{
		SeverityThreshold: models.SeverityHigh,
		DryRun:            true,
	})
	if err != nil {
		t.Fatalf("dry-run review failed: %v", err)
	}
	if dryRunResult.TotalFindings != 1 || dryRunResult.ExitCode != 0 {
		t.Fatalf("expected dry-run exit code 0, got %d", dryRunResult.ExitCode)
	}

	// 4. Test Table Formatter
	var tableBuf bytes.Buffer
	if err := formatter.Render(&tableBuf, vulnResult, engine.FormatTable); err != nil {
		t.Fatalf("table formatting failed: %v", err)
	}
	tableOutput := tableBuf.String()
	if !strings.Contains(tableOutput, "ScanDrix Code Review Summary") || !strings.Contains(tableOutput, "CRITICAL") {
		t.Fatalf("unexpected table output: %s", tableOutput)
	}

	// 5. Test JSON Formatter
	var jsonBuf bytes.Buffer
	if err := formatter.Render(&jsonBuf, vulnResult, engine.FormatJSON); err != nil {
		t.Fatalf("json formatting failed: %v", err)
	}
	var parsedJSON engine.CLIResult
	if err := json.Unmarshal(jsonBuf.Bytes(), &parsedJSON); err != nil || parsedJSON.TotalFindings != 1 {
		t.Fatalf("invalid json output: %s", jsonBuf.String())
	}

	// 6. Test SARIF Formatter
	var sarifBuf bytes.Buffer
	if err := formatter.Render(&sarifBuf, vulnResult, engine.FormatSARIF); err != nil {
		t.Fatalf("sarif formatting failed: %v", err)
	}
	var sarifLog engine.SARIFLog
	if err := json.Unmarshal(sarifBuf.Bytes(), &sarifLog); err != nil || sarifLog.Version != "2.1.0" {
		t.Fatalf("invalid sarif output: %s", sarifBuf.String())
	}
	if len(sarifLog.Runs) != 1 || len(sarifLog.Runs[0].Results) != 1 {
		t.Fatalf("expected 1 sarif run with 1 result, got %+v", sarifLog.Runs)
	}
	if sarifLog.Runs[0].Results[0].Level != "error" {
		t.Fatalf("expected error level for critical finding, got %s", sarifLog.Runs[0].Results[0].Level)
	}
}

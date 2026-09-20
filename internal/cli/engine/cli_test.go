package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
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
	if !strings.Contains(tableOutput, "ScanDrix Security & Review Summary") || !strings.Contains(tableOutput, "CRITICAL") {
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

	// 7. Test Markdown Formatter
	var mdBuf bytes.Buffer
	if err := formatter.Render(&mdBuf, vulnResult, engine.FormatMarkdown); err != nil {
		t.Fatalf("markdown formatting failed: %v", err)
	}
	mdOutput := mdBuf.String()
	if !strings.Contains(mdOutput, "## 🛡️ ScanDrix Automated Security & Quality Review") || !strings.Contains(mdOutput, "<details>") {
		t.Fatalf("unexpected markdown output: %s", mdOutput)
	}

	// 8. Test CSV Formatter
	var csvBuf bytes.Buffer
	if err := formatter.Render(&csvBuf, vulnResult, engine.FormatCSV); err != nil {
		t.Fatalf("csv formatting failed: %v", err)
	}
	csvOutput := csvBuf.String()
	if !strings.Contains(csvOutput, "ID,Severity,Category,File,StartLine,EndLine,Title,Description,Remediation") {
		t.Fatalf("unexpected csv header output: %s", csvOutput)
	}
	if !strings.Contains(csvOutput, "CRITICAL") {
		t.Fatalf("expected CSV to contain CRITICAL finding, got: %s", csvOutput)
	}

	// 8. Test Agent Mode Formatter
	var agentBuf bytes.Buffer
	if err := formatter.Render(&agentBuf, vulnResult, engine.FormatAgent); err != nil {
		t.Fatalf("agent formatting failed: %v", err)
	}
	var agentData struct {
		Status   string `json:"status"`
		Findings []any  `json:"findings"`
	}
	if err := json.Unmarshal(agentBuf.Bytes(), &agentData); err != nil || agentData.Status != "failed" || len(agentData.Findings) != 1 {
		t.Fatalf("unexpected agent output: %s", agentBuf.String())
	}
}

func TestIgnoreEngine(t *testing.T) {
	ign := engine.NewIgnoreEngine("")

	// Default ignore assertions
	if !ign.ShouldIgnore("node_modules/express/index.js") {
		t.Error("expected node_modules to be ignored")
	}
	if !ign.ShouldIgnore("pnpm-lock.yaml") {
		t.Error("expected pnpm-lock.yaml to be ignored")
	}
	if !ign.ShouldIgnore("dist/bundle.min.js") {
		t.Error("expected dist bundle to be ignored")
	}
	if !ign.ShouldIgnore("src/assets/logo.png") {
		t.Error("expected binary asset to be ignored")
	}
	if ign.ShouldIgnore("src/auth/service.go") {
		t.Error("expected source code file not to be ignored")
	}
}

func TestApplyFindingFix(t *testing.T) {
	tempDir := t.TempDir()
	testFile := "test_sample.go"
	fullPath := tempDir + "/" + testFile

	initialCode := `package main

const ApiKey = "AKIAIOSFODNN7EXAMPLE"
func main() {}
`
	if err := os.WriteFile(fullPath, []byte(initialCode), 0644); err != nil {
		t.Fatalf("failed writing test file: %v", err)
	}

	finding := models.CodeFinding{
		FilePath:      testFile,
		StartLine:     3,
		EndLine:       3,
		SuggestedDiff: `const ApiKey = os.Getenv("AWS_API_KEY")`,
	}

	msg, err := engine.ApplyFindingFix(tempDir, finding)
	if err != nil {
		t.Fatalf("failed applying fix: %v", err)
	}
	if !strings.Contains(msg, "Applied fix") {
		t.Fatalf("unexpected message: %s", msg)
	}

	updated, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("failed reading updated file: %v", err)
	}

	if strings.Contains(string(updated), "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("secret was not removed from file: %s", string(updated))
	}
	if !strings.Contains(string(updated), `const ApiKey = os.Getenv("AWS_API_KEY")`) {
		t.Errorf("suggested fix was not written to file: %s", string(updated))
	}
}

func TestApplyBatchFixesDescending(t *testing.T) {
	tempDir := t.TempDir()
	testFile := "multi_vuln.go"
	fullPath := tempDir + "/" + testFile

	initialCode := `package main

// Issue 1
const Secret1 = "HARDCODED_SECRET_1"

// Issue 2
const Secret2 = "HARDCODED_SECRET_2"

func main() {}
`
	if err := os.WriteFile(fullPath, []byte(initialCode), 0644); err != nil {
		t.Fatalf("failed writing test file: %v", err)
	}

	findings := []models.CodeFinding{
		{
			FilePath:      testFile,
			StartLine:     4,
			EndLine:       4,
			SuggestedDiff: `const Secret1 = os.Getenv("SECRET_1")`,
		},
		{
			FilePath:      testFile,
			StartLine:     7,
			EndLine:       7,
			SuggestedDiff: `const Secret2 = os.Getenv("SECRET_2")`,
		},
	}

	res, err := engine.ApplyBatchFixes(tempDir, findings)
	if err != nil {
		t.Fatalf("ApplyBatchFixes failed: %v", err)
	}
	if res.Applied != 2 || res.Failed != 0 {
		t.Fatalf("expected 2 applied, 0 failed, got %+v", res)
	}

	updated, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("failed reading updated file: %v", err)
	}

	content := string(updated)
	if strings.Contains(content, "HARDCODED_SECRET_1") || strings.Contains(content, "HARDCODED_SECRET_2") {
		t.Errorf("secrets not removed: %s", content)
	}
	if !strings.Contains(content, `const Secret1 = os.Getenv("SECRET_1")`) || !strings.Contains(content, `const Secret2 = os.Getenv("SECRET_2")`) {
		t.Errorf("remediations not applied cleanly: %s", content)
	}

	// Verify preview generator
	preview := engine.GenerateDiffPreview(findings[0])
	if !strings.Contains(preview, "File: multi_vuln.go") {
		t.Errorf("unexpected preview: %s", preview)
	}

	// Verify CanApplyFix
	if !engine.CanApplyFix(tempDir, findings[0]) {
		t.Errorf("expected CanApplyFix to return true")
	}
}

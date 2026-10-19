// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License").

package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create sample codebase structure
	srcDir := filepath.Join(tempDir, "src")
	vendorDir := filepath.Join(tempDir, "vendor")
	nodeModulesDir := filepath.Join(tempDir, "node_modules")

	_ = os.MkdirAll(srcDir, 0755)
	_ = os.MkdirAll(vendorDir, 0755)
	_ = os.MkdirAll(nodeModulesDir, 0755)

	// Valid source files
	_ = os.WriteFile(filepath.Join(srcDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	_ = os.WriteFile(filepath.Join(srcDir, "app.ts"), []byte("console.log('ScanDrix');\n"), 0644)

	// Excluded directories
	_ = os.WriteFile(filepath.Join(vendorDir, "dep.go"), []byte("package dep\n"), 0644)
	_ = os.WriteFile(filepath.Join(nodeModulesDir, "pkg.js"), []byte("module.exports = {};\n"), 0644)

	// Binary file
	binaryData := []byte{0x7f, 'E', 'L', 'F', 0x00, 0x01, 0x01, 0x00}
	_ = os.WriteFile(filepath.Join(srcDir, "app.bin"), binaryData, 0755)

	target := ScanTarget{
		RootDir: tempDir,
	}

	files, err := CollectFiles(target)
	if err != nil {
		t.Fatalf("CollectFiles failed: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected exactly 2 files (main.go and app.ts), got %d: %+v", len(files), files)
	}

	fileMap := make(map[string]ScannedFile)
	for _, f := range files {
		fileMap[f.RelativePath] = f
	}

	if _, ok := fileMap["src/main.go"]; !ok {
		t.Errorf("expected src/main.go in collected files")
	}
	if _, ok := fileMap["src/app.ts"]; !ok {
		t.Errorf("expected src/app.ts in collected files")
	}
}

func TestPartitionIntoBatches(t *testing.T) {
	files := []ScannedFile{
		{RelativePath: "auth/login.go", EstimatedTokens: 500},
		{RelativePath: "auth/token.go", EstimatedTokens: 800},
		{RelativePath: "billing/invoice.go", EstimatedTokens: 600},
		{RelativePath: "billing/stripe.go", EstimatedTokens: 700},
	}

	// Token budget of 1500 tokens should group auth files into batch 1 (1300 tokens)
	// and billing files into batch 2 (1300 tokens)
	batches := PartitionIntoBatches(files, 1500)
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}

	if batches[0].BatchID != 1 || batches[0].TotalTokens != 1300 {
		t.Errorf("batch 1 total tokens mismatch: got %d, want 1300", batches[0].TotalTokens)
	}
	if batches[1].BatchID != 2 || batches[1].TotalTokens != 1300 {
		t.Errorf("batch 2 total tokens mismatch: got %d, want 1300", batches[1].TotalTokens)
	}
}

func TestRepositoryScanner_Scan(t *testing.T) {
	tempDir := t.TempDir()

	vulnFileContent := `package handlers

import (
	"database/sql"
	"fmt"
)

const apiKey = "mock_secret_token_placeholder_for_test_1234"

func QueryUser(db *sql.DB, username string) error {
	query := "SELECT * FROM users WHERE name = " + username
	_, err := db.Exec(query)
	return err
}
`
	_ = os.WriteFile(filepath.Join(tempDir, "handler.go"), []byte(vulnFileContent), 0644)

	cleanFileContent := `package main

import "fmt"

func main() {
	fmt.Println("Clean ScanDrix Codebase")
}
`
	_ = os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(cleanFileContent), 0644)

	scanner := NewRepositoryScanner()
	result, err := scanner.Scan(context.Background(), ScanTarget{
		RootDir: tempDir,
		Mode:    ModeSecurity,
	})

	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}

	if result.Summary.TotalFiles != 2 {
		t.Errorf("expected 2 total files, got %d", result.Summary.TotalFiles)
	}

	for _, f := range result.Findings {
		t.Logf("Found finding: rule=%s title=%s line=%d", f.RuleID, f.Title, f.StartLine)
	}

	if len(result.Findings) < 2 {
		t.Fatalf("expected at least 2 findings (secret and sql injection), got %d", len(result.Findings))
	}

	var hasSecret, hasSQL bool
	for _, f := range result.Findings {
		if f.RuleID == "SDRX-SEC-001" {
			hasSecret = true
		}
		if f.RuleID == "SDRX-SEC-002" {
			hasSQL = true
		}
	}

	if !hasSecret {
		t.Errorf("expected SDRX-SEC-001 hardcoded secret finding")
	}
	if !hasSQL {
		t.Errorf("expected SDRX-SEC-002 SQL injection finding")
	}

	// Health score check: 100 - (1 * 25) - (1 * 10) = 65
	if result.Summary.CodeHealthScore > 70 {
		t.Errorf("health score should reflect deductions, got %d", result.Summary.CodeHealthScore)
	}
}

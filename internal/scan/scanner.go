// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License").

package scan

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// RepositoryScanner manages whole-repository analysis across all files and subsystems.
type RepositoryScanner struct{}

// NewRepositoryScanner constructs a new whole-repository scanner.
func NewRepositoryScanner() *RepositoryScanner {
	return &RepositoryScanner{}
}

// Scan executes whole-codebase analysis on target and generates a comprehensive report.
func (s *RepositoryScanner) Scan(ctx context.Context, target ScanTarget) (*ScanResult, error) {
	start := time.Now()

	files, err := CollectFiles(target)
	if err != nil {
		return nil, fmt.Errorf("scan collector failed: %w", err)
	}

	budget := target.TokenBudgetPerBatch
	if budget <= 0 {
		budget = 32000
	}

	batches := PartitionIntoBatches(files, budget)

	var allFindings []ScanFinding
	var totalLines int
	var totalBytes int64
	var totalTokens int

	for _, f := range files {
		totalLines += f.LineCount
		totalBytes += f.ByteSize
		totalTokens += f.EstimatedTokens

		// Analyze file
		findings := s.analyzeFile(f, target.Mode)
		allFindings = append(allFindings, findings...)
	}

	// Compute summary metrics
	bySeverity := map[string]int{
		"CRITICAL": 0,
		"HIGH":     0,
		"MEDIUM":   0,
		"LOW":      0,
		"INFO":     0,
	}
	byCategory := make(map[string]int)

	for _, f := range allFindings {
		sev := string(f.Severity)
		bySeverity[sev]++
		byCategory[f.Category]++
	}

	// Health score calculation: 100 base, deductions for findings
	healthScore := 100
	healthScore -= bySeverity["CRITICAL"] * 25
	healthScore -= bySeverity["HIGH"] * 10
	healthScore -= bySeverity["MEDIUM"] * 3
	healthScore -= bySeverity["LOW"] * 1
	if healthScore < 0 {
		healthScore = 0
	}

	summary := ScanSummary{
		TotalFiles:         len(files),
		TotalLines:         totalLines,
		TotalBytes:         totalBytes,
		TotalBatches:       len(batches),
		TotalTokens:        totalTokens,
		FindingsBySeverity: bySeverity,
		FindingsByCategory: byCategory,
		CodeHealthScore:    healthScore,
		Duration:           time.Since(start),
		ScannedAt:          start,
	}

	return &ScanResult{
		Target:   target,
		Summary:  summary,
		Findings: allFindings,
		Batches:  batches,
	}, nil
}

// Invariant patterns for codebase security and bug detection
var (
	hardcodedSecretPattern = regexp.MustCompile(`(?i)(?:api_?key|secret_?key|private_?key|auth_?token|password|bearer)\s*[:=]\s*["'][a-zA-Z0-9_\-\.]{16,}["']`)
	sqlInjectionPattern    = regexp.MustCompile(`(?i)(?:SELECT|INSERT|UPDATE|DELETE|FROM|WHERE)\s+.*?\+\s*[a-zA-Z0-9_]+`)
	commandExecPattern     = regexp.MustCompile(`(?i)(?:exec\.Command|os\.system|subprocess\.Popen|child_process\.exec)\s*\([^)]*?\+`)
	pathTraversalPattern   = regexp.MustCompile(`(?i)(?:filepath\.Join|os\.Open|fs\.readFile)\s*\([^)]*?(?:req|query|param|input)`)
)

func (s *RepositoryScanner) analyzeFile(file ScannedFile, mode ScanMode) []ScanFinding {
	var findings []ScanFinding
	lines := strings.Split(file.Content, "\n")

	for idx, line := range lines {
		lineNum := idx + 1
		trimmed := strings.TrimSpace(line)

		// Ignore comments
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// 1. Hardcoded Secrets
		if hardcodedSecretPattern.MatchString(line) {
			findings = append(findings, ScanFinding{
				ID:          uuid.New().String(),
				FilePath:    file.RelativePath,
				StartLine:   lineNum,
				EndLine:     lineNum,
				Category:    "security",
				Severity:    models.SeverityCritical,
				Title:       "Hardcoded Secret Detected",
				Description: fmt.Sprintf("Line contains hardcoded credential or token: %s", truncateSnippet(trimmed)),
				Remediation: "Extract secret into environment variable or secrets manager (e.g. AWS Secrets Manager, Vault, or .env).",
				RuleID:      "SDRX-SEC-001",
				Confidence:  0.95,
			})
		}

		// 2. SQL Concatenation
		if sqlInjectionPattern.MatchString(line) {
			findings = append(findings, ScanFinding{
				ID:          uuid.New().String(),
				FilePath:    file.RelativePath,
				StartLine:   lineNum,
				EndLine:     lineNum,
				Category:    "security",
				Severity:    models.SeverityHigh,
				Title:       "Potential SQL Injection via String Concatenation",
				Description: fmt.Sprintf("SQL query constructed using string concatenation rather than parameterized query: %s", truncateSnippet(trimmed)),
				Remediation: "Use parameterized queries, prepared statements, or ORM parameter binding.",
				RuleID:      "SDRX-SEC-002",
				Confidence:  0.90,
			})
		}

		// 3. Command Injection
		if commandExecPattern.MatchString(line) {
			findings = append(findings, ScanFinding{
				ID:          uuid.New().String(),
				FilePath:    file.RelativePath,
				StartLine:   lineNum,
				EndLine:     lineNum,
				Category:    "security",
				Severity:    models.SeverityCritical,
				Title:       "Potential Command Injection",
				Description: fmt.Sprintf("Process execution command dynamically concatenated with unvalidated input: %s", truncateSnippet(trimmed)),
				Remediation: "Pass arguments as distinct slices without shell expansion, or sanitize with strict allowlist.",
				RuleID:      "SDRX-SEC-003",
				Confidence:  0.92,
			})
		}

		// 4. Path Traversal
		if pathTraversalPattern.MatchString(line) {
			findings = append(findings, ScanFinding{
				ID:          uuid.New().String(),
				FilePath:    file.RelativePath,
				StartLine:   lineNum,
				EndLine:     lineNum,
				Category:    "security",
				Severity:    models.SeverityHigh,
				Title:       "Potential Path Traversal",
				Description: fmt.Sprintf("File system path constructed directly from request/user input: %s", truncateSnippet(trimmed)),
				Remediation: "Validate path against allowed root directory using filepath.Clean and ensure it has proper prefix.",
				RuleID:      "SDRX-SEC-004",
				Confidence:  0.88,
			})
		}

		// Mode-specific additional checks
		if mode != ModeQuick {
			// Bug: unhandled error in Go
			if file.Language == "go" && strings.HasPrefix(trimmed, "_, _ = ") {
				findings = append(findings, ScanFinding{
					ID:          uuid.New().String(),
					FilePath:    file.RelativePath,
					StartLine:   lineNum,
					EndLine:     lineNum,
					Category:    "bug",
					Severity:    models.SeverityLow,
					Title:       "Silently Suppressed Error Return",
					Description: "Error return value is explicitly discarded with blank identifier.",
					Remediation: "Inspect error return and handle or log accordingly.",
					RuleID:      "SDRX-BUG-001",
					Confidence:  0.80,
				})
			}
		}
	}

	return findings
}

func truncateSnippet(s string) string {
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}

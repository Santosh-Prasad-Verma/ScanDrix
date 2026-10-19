// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License").

package scan

import (
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// ScanMode defines the depth and scope of whole-repository analysis.
type ScanMode string

const (
	// ModeQuick performs fast heuristic and invariant rule checks.
	ModeQuick ScanMode = "quick"
	// ModeDeep performs multi-pass AST, semantic analysis, and LLM specialist review.
	ModeDeep ScanMode = "deep"
	// ModeSecurity focuses exclusively on vulnerability chains, OWASP Top 10, and STRIDE threats.
	ModeSecurity ScanMode = "security"
)

// ScanTarget specifies the repository or directory parameters for a whole-codebase audit.
type ScanTarget struct {
	RootDir              string            `json:"root_dir"`
	IncludePaths         []string          `json:"include_paths,omitempty"`
	ExcludePaths         []string          `json:"exclude_paths,omitempty"`
	LanguageFilters      []string          `json:"language_filters,omitempty"`
	MaxFileSize          int64             `json:"max_file_size,omitempty"`           // default 1MB (1048576 bytes)
	TokenBudgetPerBatch  int               `json:"token_budget_per_batch,omitempty"` // default 32000
	Mode                 ScanMode          `json:"mode,omitempty"`                   // quick, deep, security
	Metadata             map[string]string `json:"metadata,omitempty"`
}

// ScannedFile represents an individual codebase file eligible for audit.
type ScannedFile struct {
	RelativePath    string `json:"relative_path"`
	AbsolutePath    string `json:"absolute_path"`
	Language        string `json:"language"`
	LineCount       int    `json:"line_count"`
	ByteSize        int64  `json:"byte_size"`
	EstimatedTokens int    `json:"estimated_tokens"`
	Content         string `json:"content,omitempty"`
}

// Batch represents a semantically grouped partition of files analyzed together.
type Batch struct {
	BatchID          int           `json:"batch_id"`
	Files            []ScannedFile `json:"files"`
	TotalTokens      int           `json:"total_tokens"`
	SubsystemContext string        `json:"subsystem_context"`
}

// ScanFinding represents an individual defect, security vulnerability, or architectural issue.
type ScanFinding struct {
	ID            string                 `json:"id"`
	FilePath      string                 `json:"file_path"`
	StartLine     int                    `json:"start_line"`
	EndLine       int                    `json:"end_line"`
	Category      string                 `json:"category"` // security, bug, performance, style, architecture
	Severity      models.FindingSeverity `json:"severity"` // CRITICAL, HIGH, MEDIUM, LOW, INFO
	Title         string                 `json:"title"`
	Description   string                 `json:"description"`
	SuggestedDiff string                 `json:"suggested_diff,omitempty"`
	Remediation   string                 `json:"remediation,omitempty"`
	RuleID        string                 `json:"rule_id,omitempty"`
	Confidence    float64                `json:"confidence"`
}

// ScanSummary aggregates high-level metrics and overall codebase health.
type ScanSummary struct {
	TotalFiles          int                    `json:"total_files"`
	TotalLines          int                    `json:"total_lines"`
	TotalBytes          int64                  `json:"total_bytes"`
	TotalBatches        int                    `json:"total_batches"`
	TotalTokens         int                    `json:"total_tokens"`
	FindingsBySeverity  map[string]int         `json:"findings_by_severity"`
	FindingsByCategory  map[string]int         `json:"findings_by_category"`
	CodeHealthScore     int                    `json:"code_health_score"` // 0-100
	Duration            time.Duration          `json:"duration"`
	ScannedAt           time.Time              `json:"scanned_at"`
}

// ScanResult is the comprehensive report produced by a whole-repository audit.
type ScanResult struct {
	Target   ScanTarget    `json:"target"`
	Summary  ScanSummary   `json:"summary"`
	Findings []ScanFinding `json:"findings"`
	Batches  []Batch       `json:"batches,omitempty"`
}

// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package knowledge

import (
	"time"

	"github.com/google/uuid"
)

// SymbolKind categorizes programmatic definitions.
type SymbolKind string

const (
	KindFunction  SymbolKind = "FUNCTION"
	KindMethod    SymbolKind = "METHOD"
	KindStruct    SymbolKind = "STRUCT"
	KindInterface SymbolKind = "INTERFACE"
	KindClass     SymbolKind = "CLASS"
	KindType      SymbolKind = "TYPE"
	KindVariable  SymbolKind = "VARIABLE"
	KindConstant  SymbolKind = "CONSTANT"
)

// SymbolLocation records the exact file and line position of a symbol.
type SymbolLocation struct {
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// SymbolDefinition models a concrete function, type, or interface defined in the codebase.
type SymbolDefinition struct {
	ID          uuid.UUID      `json:"id"`
	Name        string         `json:"name"`
	Package     string         `json:"package"`
	Kind        SymbolKind     `json:"kind"`
	Location    SymbolLocation `json:"location"`
	Signature   string         `json:"signature,omitempty"`
	Docstring   string         `json:"docstring,omitempty"`
	Exported    bool           `json:"exported"`
	Language    string         `json:"language"`
	Receiver    string         `json:"receiver,omitempty"` // For methods: struct/type name
	CallerCount int            `json:"caller_count"`
}

// SymbolReference records an invocation or usage site of a symbol.
type SymbolReference struct {
	SymbolName string         `json:"symbol_name"`
	CallerName string         `json:"caller_name"`
	Location   SymbolLocation `json:"location"`
}

// CallGraphEdge represents a directed call relationship from Caller to Callee.
type CallGraphEdge struct {
	CallerName     string `json:"caller_name"`
	CallerFile     string `json:"caller_file"`
	CallerLine     int    `json:"caller_line"`
	CalleeName     string `json:"callee_name"`
	CalleeFile     string `json:"callee_file"`
	IsCrossPackage bool   `json:"is_cross_package"`
	IsCrossFile    bool   `json:"is_cross_file"`
}

// RiskRating indicates the potential impact of a change set.
type RiskRating string

const (
	RiskLow      RiskRating = "LOW"
	RiskMedium   RiskRating = "MEDIUM"
	RiskHigh     RiskRating = "HIGH"
	RiskCritical RiskRating = "CRITICAL"
)

// BlastRadiusReport summarizes the upstream and downstream blast radius of a pull request diff.
type BlastRadiusReport struct {
	DirectlyModifiedFiles   []string            `json:"directly_modified_files"`
	DirectlyModifiedSymbols []string            `json:"directly_modified_symbols"`
	Tier1ImpactedFiles      []string            `json:"tier1_impacted_files"`
	Tier1ImpactedSymbols    []string            `json:"tier1_impacted_symbols"`
	Tier2ImpactedFiles      []string            `json:"tier2_impacted_files"`
	TotalImpactedFiles      int                 `json:"total_impacted_files"`
	ImpactScore             float64             `json:"impact_score"` // 0.0 to 1.0
	RiskRating              RiskRating          `json:"risk_rating"`
	CallPaths               [][]string          `json:"call_paths,omitempty"` // Traces from changed code to entry points
	GeneratedAt             time.Time           `json:"generated_at"`
}

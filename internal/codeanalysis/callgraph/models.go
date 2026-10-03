package callgraph

import (
	"context"
)

// SymbolChange records a modified declaration within a pull request diff.
type SymbolChange struct {
	SymbolName   string `json:"symbol_name"`
	FilePath     string `json:"file_path"`
	OldSignature string `json:"old_signature"`
	NewSignature string `json:"new_signature"`
	IsBreaking   bool   `json:"is_breaking"`
}

// CallSite identifies an invocation of a modified symbol in repository files.
type CallSite struct {
	CallerFile string `json:"caller_file"`
	LineNumber int    `json:"line_number"`
	Snippet    string `json:"snippet"`
}

// ImpactReport maps downstream blast radius of changed symbols across unchanged files.
type ImpactReport struct {
	ModifiedSymbols []SymbolChange `json:"modified_symbols"`
	AffectedFiles   []string       `json:"affected_files"`
	CallSites       []CallSite     `json:"call_sites"`
}

// FileProvider abstracts file content reading across workspace or repo filesystem.
type FileProvider interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	ListFiles(ctx context.Context) ([]string, error)
}

// ImpactAnalyzer is the canonical interface specced in Enterprise Edition Phase 2 (REQ-3.1–3.3).
type ImpactAnalyzer interface {
	AnalyzeDiff(ctx context.Context, repoID string, diffText string) (*ImpactReport, error)
	AnalyzeDiffWithFiles(ctx context.Context, repoID string, diffText string, files FileProvider) (*ImpactReport, error)
}

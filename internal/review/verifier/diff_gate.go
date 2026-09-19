package verifier

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/review/checker"
	"github.com/scandrix/backend/pkg/models"
)

// DiffGateDecision indicates whether a proposed diff meets all safety and compilation criteria.
type DiffGateDecision string

const (
	DecisionCommittable DiffGateDecision = "COMMITTABLE"
	DecisionAdvisory    DiffGateDecision = "ADVISORY"
	DecisionRejected    DiffGateDecision = "REJECTED"
)

// DiffGateResult encapsulates the outcome of gating a proposed code suggestion.
type DiffGateResult struct {
	FindingID          string                   `json:"finding_id"`
	Decision           DiffGateDecision         `json:"decision"`
	IsCommittable      bool                     `json:"is_committable"`
	ConfidenceScore    float64                  `json:"confidence_score"`
	RejectionReason    string                   `json:"rejection_reason,omitempty"`
	AdvisoryReason     string                   `json:"advisory_reason,omitempty"`
	SyntaxReport       *checker.SyntaxValidationReport `json:"syntax_report,omitempty"`
	PatchedContent     string                   `json:"patched_content,omitempty"`
	ValidatedStartLine int                      `json:"validated_start_line"`
	ValidatedEndLine   int                      `json:"validated_end_line"`
	EvaluatedAt        time.Time                `json:"evaluated_at"`
}

// DiffGateOptions configures threshold limits and strictness for suggestion diff verification.
type DiffGateOptions struct {
	MaxLinesThreshold int     `json:"max_lines_threshold"` // Default: 15 lines
	MaxCharsThreshold int     `json:"max_chars_threshold"` // Default: 1000 chars
	RejectNoOps       bool    `json:"reject_no_ops"`
	RejectCosmetic    bool    `json:"reject_cosmetic"`
	RequireCleanMerge bool    `json:"require_clean_merge"`
	MinConfidence     float64 `json:"min_confidence"`
}

// DefaultDiffGateOptions supplies production thresholds.
func DefaultDiffGateOptions() DiffGateOptions {
	return DiffGateOptions{
		MaxLinesThreshold: 15,
		MaxCharsThreshold: 1000,
		RejectNoOps:       true,
		RejectCosmetic:    true,
		RequireCleanMerge: true,
		MinConfidence:     0.70,
	}
}

// DiffGate verifies and guarantees that AI code suggestions apply cleanly and maintain syntactical validity.
type DiffGate struct {
	mu              sync.RWMutex
	syntaxValidator *checker.ASTSyntaxValidator
	options         DiffGateOptions
}

// NewDiffGate initializes a DiffGate with a dedicated AST syntax validator.
func NewDiffGate(validator *checker.ASTSyntaxValidator, opts ...DiffGateOptions) *DiffGate {
	if validator == nil {
		validator = checker.NewASTSyntaxValidator()
	}
	opt := DefaultDiffGateOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}
	return &DiffGate{
		syntaxValidator: validator,
		options:         opt,
	}
}

// ValidateFindingDiff performs end-to-end verification of a finding's suggested code replacement.
func (g *DiffGate) ValidateFindingDiff(
	ctx context.Context,
	finding models.CodeFinding,
	fileContent string,
) DiffGateResult {
	res := DiffGateResult{
		FindingID:          finding.ID.String(),
		ValidatedStartLine: finding.StartLine,
		ValidatedEndLine:   finding.EndLine,
		ConfidenceScore:    0.85,
		EvaluatedAt:        time.Now().UTC(),
	}

	codeSnippet := finding.SuggestedDiff
	if codeSnippet == "" {
		codeSnippet = finding.Remediation
	}
	codeSnippet = strings.TrimSpace(codeSnippet)

	// 1. If finding has no code replacement, it is inherently advisory
	if codeSnippet == "" {
		res.Decision = DecisionAdvisory
		res.IsCommittable = false
		res.AdvisoryReason = "no concrete code replacement provided; advisory comment only"
		res.ConfidenceScore = 0.75
		return res
	}

	// 2. Line threshold checks
	lines := strings.Split(codeSnippet, "\n")
	lineCount := len(lines)
	charCount := len(codeSnippet)

	if lineCount > g.options.MaxLinesThreshold || charCount > g.options.MaxCharsThreshold {
		res.Decision = DecisionAdvisory
		res.IsCommittable = false
		res.AdvisoryReason = fmt.Sprintf(
			"suggestion exceeds committable threshold (%d lines > %d, or %d chars > %d); downgraded to advisory",
			lineCount, g.options.MaxLinesThreshold, charCount, g.options.MaxCharsThreshold,
		)
		res.ConfidenceScore = 0.80
		return res
	}

	// 3. No-op detection
	fileLines := strings.Split(fileContent, "\n")
	if fileContent != "" && finding.StartLine > 0 && finding.StartLine <= len(fileLines) {
		end := finding.EndLine
		if end < finding.StartLine || end > len(fileLines) {
			end = finding.StartLine
		}
		targetOriginal := strings.Join(fileLines[finding.StartLine-1:end], "\n")
		if g.options.RejectNoOps && strings.TrimSpace(targetOriginal) == codeSnippet {
			res.Decision = DecisionRejected
			res.IsCommittable = false
			res.RejectionReason = "suggested code is identical to existing source lines (no-op diff)"
			res.ConfidenceScore = 0.0
			return res
		}
	}

	// 4. Cosmetic-only rejection for Bug / Security categories
	if g.options.RejectCosmetic {
		isCosmetic := isCommentOrWhitespaceOnly(codeSnippet)
		cat := strings.ToLower(finding.Category)
		sev := strings.ToLower(string(finding.Severity))
		if isCosmetic && (cat == "security" || cat == "bug" || sev == "critical" || sev == "high") {
			res.Decision = DecisionRejected
			res.IsCommittable = false
			res.RejectionReason = "critical or security finding cannot propose purely cosmetic comments or formatting"
			res.ConfidenceScore = 0.10
			return res
		}
	}

	// 5. AST Syntax verification of snippet
	snippetReport := g.syntaxValidator.ValidateSnippet(ctx, finding.FilePath, codeSnippet)
	res.SyntaxReport = &snippetReport

	if !snippetReport.IsValid {
		res.Decision = DecisionAdvisory
		res.IsCommittable = false
		var errMsgs []string
		for _, d := range snippetReport.Diagnostics {
			errMsgs = append(errMsgs, fmt.Sprintf("line %d: %s", d.Line, d.Message))
		}
		res.AdvisoryReason = fmt.Sprintf("syntax error in suggested replacement: %s", strings.Join(errMsgs, "; "))
		res.ConfidenceScore = 0.50
		return res
	}

	// 6. Simulate patch merge into source file if file content is available
	if fileContent != "" && finding.StartLine > 0 {
		patched, mergeOk := simulateHunkPatch(fileContent, finding.StartLine, finding.EndLine, codeSnippet)
		if !mergeOk {
			if g.options.RequireCleanMerge {
				res.Decision = DecisionAdvisory
				res.IsCommittable = false
				res.AdvisoryReason = "failed simulating patch hunk replacement against target file line range"
				res.ConfidenceScore = 0.60
				return res
			}
		} else {
			res.PatchedContent = patched

			// Full file AST syntax check on merged result
			fullReport := g.syntaxValidator.ValidateFile(ctx, finding.FilePath, patched)
			if !fullReport.IsValid {
				res.Decision = DecisionAdvisory
				res.IsCommittable = false
				res.AdvisoryReason = fmt.Sprintf("synthesized file fails full AST validation: %v", fullReport.Diagnostics)
				res.ConfidenceScore = 0.55
				return res
			}
		}
	}

	// All gates passed cleanly!
	res.Decision = DecisionCommittable
	res.IsCommittable = true
	res.ConfidenceScore = 0.95
	if finding.Severity == models.SeverityCritical {
		res.ConfidenceScore = 0.98
	}

	return res
}

// ValidateFindingsBatch processes an array of findings in parallel and attaches decisions.
func (g *DiffGate) ValidateFindingsBatch(
	ctx context.Context,
	findings []models.CodeFinding,
	fileContexts map[string]string,
) ([]models.CodeFinding, map[string]DiffGateResult) {
	results := make(map[string]DiffGateResult)
	var filteredFindings []models.CodeFinding
	var mu sync.Mutex

	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // Bounded concurrency

	for _, f := range findings {
		wg.Add(1)
		go func(finding models.CodeFinding) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fileContent := fileContexts[finding.FilePath]
			gateRes := g.ValidateFindingDiff(ctx, finding, fileContent)

			mu.Lock()
			results[finding.ID.String()] = gateRes
			if gateRes.Decision != DecisionRejected {
				// Retain findings unless explicitly rejected as no-op or invalid cosmetic churn
				filteredFindings = append(filteredFindings, finding)
			}
			mu.Unlock()
		}(f)
	}

	wg.Wait()
	return filteredFindings, results
}

func simulateHunkPatch(original string, startLine, endLine int, replacement string) (string, bool) {
	lines := strings.Split(original, "\n")
	if startLine < 1 || startLine > len(lines) {
		return "", false
	}
	if endLine < startLine {
		endLine = startLine
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}

	var patchedLines []string
	patchedLines = append(patchedLines, lines[:startLine-1]...)
	patchedLines = append(patchedLines, replacement)
	if endLine < len(lines) {
		patchedLines = append(patchedLines, lines[endLine:]...)
	}

	return strings.Join(patchedLines, "\n"), true
}

func isCommentOrWhitespaceOnly(code string) bool {
	lines := strings.Split(code, "\n")
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if !strings.HasPrefix(t, "//") && !strings.HasPrefix(t, "/*") && !strings.HasPrefix(t, "*") &&
			!strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "--") {
			return false
		}
	}
	return true
}

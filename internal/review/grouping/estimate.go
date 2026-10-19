package grouping

import (
	"fmt"
	"strings"
)

const (
	// promptOverheadTokens approximates the system prompt, tool schemas, and repository rules.
	promptOverheadTokens = 1800

	// avgSpecialistRoundsPerFile models the multi-agent deliberation rounds for a file.
	avgSpecialistRoundsPerFile = 5

	// avgOutputTokensPerRound models standard structured completion size per round.
	avgOutputTokensPerRound = 600
)

// Estimate provides pre-flight token and complexity projections before running review LLMs.
type Estimate struct {
	Files        int   `json:"files"`
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

// EstimateDiffTokens approximates token count for a patch text using 4 characters per token heuristic.
func EstimateDiffTokens(diffText string) int64 {
	trimmed := strings.TrimSpace(diffText)
	if len(trimmed) == 0 {
		return 0
	}
	// Approximate 1 token per 3.8 characters for source code/diffs
	tokens := int64(len(trimmed)) / 4
	if tokens == 0 {
		return 1
	}
	return tokens
}

// EstimateChangeSet projects input and output token consumption for a pull request diff.
func EstimateChangeSet(diffs []DiffItem) Estimate {
	var est Estimate

	for _, d := range diffs {
		if d.IsDeleted || strings.TrimSpace(d.DiffText) == "" {
			continue
		}
		est.Files++
		diffTokens := EstimateDiffTokens(d.DiffText)

		// Initial analysis pass
		est.InputTokens += diffTokens + promptOverheadTokens
		est.OutputTokens += 350

		// Specialist council deliberation rounds
		est.InputTokens += (diffTokens + promptOverheadTokens) * avgSpecialistRoundsPerFile
		est.OutputTokens += avgOutputTokensPerRound * avgSpecialistRoundsPerFile
	}

	est.TotalTokens = est.InputTokens + est.OutputTokens
	return est
}

// EstimateReview calculates projected token consumption for a set of file paths and total diff size.
func EstimateReview(files []string, diffBytes int) Estimate {
	if len(files) == 0 {
		return Estimate{}
	}
	bytesPerFile := diffBytes / len(files)
	var items []DiffItem
	for _, f := range files {
		items = append(items, DiffItem{
			NewPath:  f,
			DiffText: strings.Repeat("x", bytesPerFile),
		})
	}
	return EstimateChangeSet(items)
}

// FormatHumanTokens formats a token count into a human-readable string (e.g., "1.4M", "850K").
func FormatHumanTokens(tokens int64) string {
	switch {
	case tokens >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	case tokens >= 1_000:
		return fmt.Sprintf("%.1fK", float64(tokens)/1_000)
	default:
		return fmt.Sprintf("%d", tokens)
	}
}

// String returns a clean single-line representation of the pre-flight estimate.
func (e Estimate) String() string {
	return fmt.Sprintf("~%d file(s): %s in + %s out ≈ %s total tokens",
		e.Files, FormatHumanTokens(e.InputTokens), FormatHumanTokens(e.OutputTokens), FormatHumanTokens(e.TotalTokens))
}

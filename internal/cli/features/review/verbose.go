package review

import (
	"encoding/json"
	"fmt"

	"github.com/scandrix/backend/internal/cli/types"
)

// CreateAnalyzeStartVerboseMessages logs review flags and diff dimensions.
func CreateAnalyzeStartVerboseMessages(diff string, rulesOnly, fast bool) []string {
	return []string{
		fmt.Sprintf("[verbose] Review config: rulesOnly=%t, fast=%t", rulesOnly, fast),
		fmt.Sprintf("[verbose] Diff size: %d characters", len(diff)),
	}
}

// CreateFullFileContentsVerboseMessages produces audit lines for inlined file payloads.
func CreateFullFileContentsVerboseMessages(files []types.FileContent) []string {
	msgs := []string{
		fmt.Sprintf("[verbose] Full file contents: %d file(s)", len(files)),
	}
	for _, f := range files {
		msgs = append(msgs, fmt.Sprintf("[verbose]   - %s: %d chars, status=%s", f.Path, len(f.Content), f.Status))
	}
	return msgs
}

// CreateAnalyzeAPIRequestVerboseMessages formats outgoing payload details.
func CreateAnalyzeAPIRequestVerboseMessages(diff string, cfg *ReviewPayloadConfig, mode string, branch, remote string) []string {
	var msgs []string
	if mode == "team-key" {
		msgs = append(msgs, "[verbose] Using team key with metrics")
		msgs = append(msgs, fmt.Sprintf("[verbose] Git info: branch=%s, remote=%s", branch, remote))
	} else {
		msgs = append(msgs, "[verbose] Using personal token (no metrics)")
	}

	cfgJSON, _ := json.Marshal(cfg)
	msgs = append(msgs,
		"[verbose] Sending to API:",
		fmt.Sprintf("[verbose]   - diff length: %d chars", len(diff)),
		fmt.Sprintf("[verbose]   - config: %s", string(cfgJSON)),
	)
	return msgs
}

// CreateAnalyzeAPIResponseVerboseMessages logs API review completion stats.
func CreateAnalyzeAPIResponseVerboseMessages(summary string, issuesCount, filesAnalyzed int) []string {
	return []string{
		"[verbose] API response:",
		fmt.Sprintf("[verbose]   - summary: %s", summary),
		fmt.Sprintf("[verbose]   - issues: %d", issuesCount),
		fmt.Sprintf("[verbose]   - filesAnalyzed: %d", filesAnalyzed),
	}
}

// CreateTrialAnalyzeStartVerboseMessages produces diff preview for trial reviews.
func CreateTrialAnalyzeStartVerboseMessages(diff string) []string {
	previewLen := 300
	if len(diff) < previewLen {
		previewLen = len(diff)
	}
	preview := diff[:previewLen]
	trunc := ""
	if len(diff) > 300 {
		trunc = "\n... (truncated)"
	}
	return []string{
		"[verbose] Running trial analyze",
		fmt.Sprintf("[verbose] Diff size: %d characters", len(diff)),
		fmt.Sprintf("[verbose] Diff preview:\n%s%s", preview, trunc),
	}
}

// CreateTrialAnalyzeResponseVerboseMessages logs trial review completion stats.
func CreateTrialAnalyzeResponseVerboseMessages(summary string, issuesCount, filesAnalyzed int) []string {
	return []string{
		"[verbose] Trial API response:",
		fmt.Sprintf("[verbose]   - summary: %s", summary),
		fmt.Sprintf("[verbose]   - issues: %d", issuesCount),
		fmt.Sprintf("[verbose]   - filesAnalyzed: %d", filesAnalyzed),
	}
}

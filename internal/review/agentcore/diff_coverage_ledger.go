// Package agentcore provides the deep core agent loop and execution components for code reviews.
package agentcore

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// CoverageTier defines file importance for code review.
type CoverageTier string

const (
	TierCritical CoverageTier = "critical"
	TierWarm     CoverageTier = "warm"
	TierOptional CoverageTier = "optional"

	// TieredTotalCoverageThreshold is the total coverage fraction required when tiering is active (70%).
	TieredTotalCoverageThreshold = 0.70
	DefaultCoverageThreshold     = TieredTotalCoverageThreshold
)

// CoverageTouch records a specific tool observation on a target.
type CoverageTouch struct {
	Tool string `json:"tool"`
	Path string `json:"path"`
	Step int    `json:"step"`
}

// DiffHunk represents an individual diff chunk with line coordinates.
type DiffHunk struct {
	OldStart int    `json:"oldStart"`
	OldLines int    `json:"oldLines"`
	NewStart int    `json:"newStart"`
	NewLines int    `json:"newLines"`
	Header   string `json:"header,omitempty"`
}

// ChangedFile represents a modified file in a Pull Request with optional unified diff patch.
type ChangedFile struct {
	Filename  string     `json:"filename"`
	Patch     string     `json:"patch,omitempty"`
	Hunks     []DiffHunk `json:"hunks,omitempty"`
	Additions int        `json:"additions,omitempty"`
	Deletions int        `json:"deletions,omitempty"`
}

// CoverageTarget represents a specific file or hunk target that should be reviewed.
type CoverageTarget struct {
	ID            string          `json:"id"`
	File          string          `json:"file"`
	ChangedRanges [][2]int        `json:"changedRanges"`
	TouchedRanges [][2]int        `json:"touchedRanges"`
	Status        string          `json:"status"` // "pending" | "touched"
	TouchedBy     []CoverageTouch `json:"touchedBy"`
	Tier          CoverageTier    `json:"tier,omitempty"`
	StepSeen      int             `json:"stepSeen,omitempty"`
	Hunk          DiffHunk        `json:"hunk,omitempty"`
}

// CoverageSummary holds aggregated metrics across all review targets.
type CoverageSummary struct {
	TotalTargets    int      `json:"totalTargets"`
	TouchedTargets  int      `json:"touchedTargets"`
	PendingTargets  int      `json:"pendingTargets"`
	TouchedFiles    []string `json:"touchedFiles"`
	PendingFiles    []string `json:"pendingFiles"`
	CriticalTotal   int      `json:"criticalTotal"`
	CriticalTouched int      `json:"criticalTouched"`
	CriticalPending int      `json:"criticalPending"`
	WarmTotal       int      `json:"warmTotal"`
	WarmTouched     int      `json:"warmTouched"`
	WarmPending     int      `json:"warmPending"`
	OptionalTotal   int      `json:"optionalTotal"`
	OptionalTouched int      `json:"optionalTouched"`
	OptionalPending int      `json:"optionalPending"`
}

// DiffCoverageLedgerParams configures the diff coverage ledger.
type DiffCoverageLedgerParams struct {
	ChangedFiles      []ChangedFile
	FileTiers         map[string]CoverageTier
	CoverageThreshold float64
}

// DiffCoverageLedger implements contracts.ProgressLedger tracking PR diff coverage across steps.
// Coverage is per-hunk, not per-file: reading a small slice of a multi-hunk file does not
// mark other hunks as touched until they fall within the merged union of read ranges.
type DiffCoverageLedger struct {
	mu                sync.RWMutex
	targets           []*CoverageTarget
	coverageThreshold float64
}

var (
	hunkHeaderRegex = regexp.MustCompile(`(?m)^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
)

// NormalizeRepoPath canonicalizes file paths across OS conventions.
func NormalizeRepoPath(path string) string {
	clean := strings.ReplaceAll(path, "\\", "/")
	clean = strings.TrimLeft(clean, "./")
	clean = strings.TrimLeft(clean, "/")
	return strings.TrimSpace(clean)
}

// ExtractChangedLineRanges parses hunk line boundaries from a unified diff patch.
func ExtractChangedLineRanges(patch string) [][2]int {
	if len(patch) == 0 {
		return nil
	}

	var ranges [][2]int
	matches := hunkHeaderRegex.FindAllStringSubmatch(patch, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		start, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		count := 1
		if len(m) >= 3 && m[2] != "" {
			if parsed, err := strconv.Atoi(m[2]); err == nil {
				count = parsed
			}
		}
		end := start
		if count > 0 {
			end = start + count - 1
		}
		ranges = append(ranges, [2]int{start, end})
	}
	return ranges
}

// MergeRanges merges contiguous or overlapping line ranges.
// Contiguous ranges [1, 50] and [51, 100] are merged into [1, 100].
func MergeRanges(ranges [][2]int) [][2]int {
	if len(ranges) <= 1 {
		result := make([][2]int, len(ranges))
		copy(result, ranges)
		return result
	}

	sorted := make([][2]int, len(ranges))
	for i, r := range ranges {
		s, e := r[0], r[1]
		if s > e {
			s, e = e, s
		}
		sorted[i] = [2]int{s, e}
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i][0] < sorted[j][0]
	})

	merged := [][2]int{sorted[0]}
	for i := 1; i < len(sorted); i++ {
		prev := &merged[len(merged)-1]
		cur := sorted[i]
		if cur[0] <= prev[1]+1 {
			if cur[1] > prev[1] {
				prev[1] = cur[1]
			}
		} else {
			merged = append(merged, cur)
		}
	}
	return merged
}

// IsRangeCoveredByUnion checks if range r is covered by any range in union.
// It is covered if union covers r completely, or if union covers from r's start through at least 70% of r.
func IsRangeCoveredByUnion(r [2]int, union [][2]int) bool {
	for _, u := range union {
		// Complete containment: u covers r
		if u[0] <= r[0] && u[1] >= r[1] {
			return true
		}
		// Substantial overlap: u covers from start of r through at least 70% of r
		if u[0] <= r[0] && u[1] >= r[0] {
			coveredLen := u[1] - r[0] + 1
			totalLen := r[1] - r[0] + 1
			if totalLen <= 0 || float64(coveredLen)/float64(totalLen) >= 0.70 || coveredLen >= 15 {
				return true
			}
		}
		// Substantial overlap: u covers up to end of r
		if u[1] >= r[1] && u[0] <= r[1] {
			coveredLen := r[1] - u[0] + 1
			totalLen := r[1] - r[0] + 1
			if totalLen <= 0 || float64(coveredLen)/float64(totalLen) >= 0.70 || coveredLen >= 15 {
				return true
			}
		}
	}
	return false
}

// PendingHunks returns the subset of changed ranges that are not yet covered.
func PendingHunks(target CoverageTarget) [][2]int {
	if len(target.ChangedRanges) == 0 {
		return nil
	}
	var pending [][2]int
	for _, cr := range target.ChangedRanges {
		if !IsRangeCoveredByUnion(cr, target.TouchedRanges) {
			pending = append(pending, cr)
		}
	}
	return pending
}

// IsTargetFullyCovered returns true if all declared changed ranges have been read.
func IsTargetFullyCovered(target CoverageTarget) bool {
	if len(target.ChangedRanges) == 0 {
		return len(target.TouchedRanges) > 0
	}
	return len(PendingHunks(target)) == 0
}

// PathsMatch checks if target and observed paths match via exact match or directory-boundary suffix.
// Notice: Intentionally no basename-only fallback to avoid false collisions across different folders.
func PathsMatch(targetFile, observedPath string) bool {
	tNorm := NormalizeRepoPath(targetFile)
	oNorm := NormalizeRepoPath(observedPath)
	if tNorm == oNorm {
		return true
	}
	if strings.HasSuffix(oNorm, "/"+tNorm) || strings.HasSuffix(tNorm, "/"+oNorm) {
		return true
	}
	return false
}

// NewDiffCoverageLedger creates an enterprise DiffCoverageLedger with configurable thresholds.
func NewDiffCoverageLedger(params DiffCoverageLedgerParams) *DiffCoverageLedger {
	threshold := params.CoverageThreshold
	if threshold <= 0 {
		threshold = DefaultCoverageThreshold
		if val := os.Getenv("SCANDRIX_COVERAGE_THRESHOLD"); val != "" {
			if parsed, err := strconv.ParseFloat(val, 64); err == nil && parsed > 0 && parsed <= 1.0 {
				threshold = parsed
			}
		}
	}

	targets := make([]*CoverageTarget, 0, len(params.ChangedFiles))
	for _, f := range params.ChangedFiles {
		norm := NormalizeRepoPath(f.Filename)
		if norm == "" {
			continue
		}

		tier := TierWarm
		if t, ok := params.FileTiers[norm]; ok {
			tier = t
		} else if isCriticalPath(norm) {
			tier = TierCritical
		}

		ranges := ExtractChangedLineRanges(f.Patch)
		if len(ranges) == 0 && len(f.Hunks) > 0 {
			for _, h := range f.Hunks {
				end := h.NewStart
				if h.NewLines > 0 {
					end = h.NewStart + h.NewLines - 1
				}
				ranges = append(ranges, [2]int{h.NewStart, end})
			}
		}

		var firstHunk DiffHunk
		if len(f.Hunks) > 0 {
			firstHunk = f.Hunks[0]
		}

		targets = append(targets, &CoverageTarget{
			ID:            norm,
			File:          norm,
			ChangedRanges: ranges,
			TouchedRanges: nil,
			Status:        "pending",
			TouchedBy:     nil,
			Tier:          tier,
			Hunk:          firstHunk,
		})
	}

	return &DiffCoverageLedger{
		targets:           targets,
		coverageThreshold: threshold,
	}
}

func isCriticalPath(path string) bool {
	lower := strings.ToLower(path)
	criticalKeywords := []string{
		"auth", "security", "crypto", "password", "token", "secret", "permission", "rbac", "payment",
		"billing", "webhook", "cert", "oauth", "jwt",
	}
	for _, kw := range criticalKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// MarkFromToolCall inspects tool invocations (e.g. readFile) and updates target touched ranges.
func (d *DiffCoverageLedger) MarkFromToolCall(toolName string, input any, step int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	inputMap, ok := input.(map[string]any)
	if !ok {
		return
	}

	path := ""
	for _, key := range []string{"path", "filePath", "file"} {
		if val, ok := inputMap[key].(string); ok && val != "" {
			path = val
			break
		}
	}
	if path == "" {
		return
	}

	path = NormalizeRepoPath(path)

	// Extract start and end lines if specified
	var startLine, endLine int
	if s, ok := inputMap["startLine"].(int); ok && s > 0 {
		startLine = s
	} else if s, ok := inputMap["startLine"].(float64); ok && s > 0 {
		startLine = int(s)
	}

	if e, ok := inputMap["endLine"].(int); ok && e > 0 {
		endLine = e
	} else if e, ok := inputMap["endLine"].(float64); ok && e > 0 {
		endLine = int(e)
	}

	for _, target := range d.targets {
		if !PathsMatch(target.File, path) {
			continue
		}

		// Record touch trace
		alreadyTouchedBy := false
		for _, t := range target.TouchedBy {
			if t.Tool == toolName && t.Path == path {
				alreadyTouchedBy = true
				break
			}
		}
		if !alreadyTouchedBy {
			target.TouchedBy = append(target.TouchedBy, CoverageTouch{
				Tool: toolName,
				Path: path,
				Step: step,
			})
		}

		if startLine > 0 || endLine > 0 {
			readStart := startLine
			if readStart <= 0 {
				readStart = 1
			}
			readEnd := endLine
			if readEnd <= 0 {
				readEnd = readStart
			}
			if readStart > readEnd {
				readStart, readEnd = readEnd, readStart
			}
			target.TouchedRanges = MergeRanges(append(target.TouchedRanges, [2]int{readStart, readEnd}))
		} else {
			// Full file read covers all lines
			target.TouchedRanges = [][2]int{{1, math.MaxInt32}}
		}

		if IsTargetFullyCovered(*target) {
			target.Status = "touched"
			target.StepSeen = step
		}
	}
}

// Summary projects the domain coverage onto contracts.ProgressSummary.
func (d *DiffCoverageLedger) Summary() contracts.ProgressSummary {
	d.mu.RLock()
	defer d.mu.RUnlock()

	s := d.coverageSummaryLocked()
	return contracts.ProgressSummary{
		TotalTargets:    s.TotalTargets,
		PendingTargets:  s.PendingTargets,
		CriticalTotal:   s.CriticalTotal,
		CriticalPending: s.CriticalPending,
	}
}

// CoverageSummary returns full rich domain metrics.
func (d *DiffCoverageLedger) CoverageSummary() CoverageSummary {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.coverageSummaryLocked()
}

func (d *DiffCoverageLedger) coverageSummaryLocked() CoverageSummary {
	var s CoverageSummary
	s.TotalTargets = len(d.targets)

	for _, t := range d.targets {
		if t.Status == "touched" {
			s.TouchedTargets++
			s.TouchedFiles = append(s.TouchedFiles, t.File)
		} else {
			s.PendingTargets++
			s.PendingFiles = append(s.PendingFiles, t.File)
		}

		switch t.Tier {
		case TierCritical:
			s.CriticalTotal++
			if t.Status == "touched" {
				s.CriticalTouched++
			} else {
				s.CriticalPending++
			}
		case TierWarm:
			s.WarmTotal++
			if t.Status == "touched" {
				s.WarmTouched++
			} else {
				s.WarmPending++
			}
		case TierOptional:
			s.OptionalTotal++
			if t.Status == "touched" {
				s.OptionalTouched++
			} else {
				s.OptionalPending++
			}
		}
	}

	return s
}

// DebtNote returns a human-readable guidance note of pending critical targets,
// or nil if all critical targets have been addressed.
func (d *DiffCoverageLedger) DebtNote() *string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	debt := formatCoverageDebtLocked(d.targets, 8)
	if debt == "" {
		return nil
	}
	return &debt
}

// IsSatisfied returns true if all critical targets have been inspected.
func (d *DiffCoverageLedger) IsSatisfied() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	s := d.coverageSummaryLocked()
	if s.TotalTargets == 0 {
		return true
	}
	if s.CriticalTotal > 0 {
		return s.CriticalPending == 0
	}
	return s.PendingTargets == 0
}

// IsCoverageSatisfied returns true if all critical targets are satisfied AND total coverage meets threshold.
func (d *DiffCoverageLedger) IsCoverageSatisfied() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	s := d.coverageSummaryLocked()
	if s.TotalTargets == 0 {
		return true
	}
	if s.CriticalPending > 0 {
		return false
	}
	ratio := float64(s.TouchedTargets) / float64(s.TotalTargets)
	return ratio >= d.coverageThreshold
}

// IsLowCoverage returns true if total coverage is below the configured threshold.
func (d *DiffCoverageLedger) IsLowCoverage() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	s := d.coverageSummaryLocked()
	if s.TotalTargets == 0 {
		return false
	}
	ratio := float64(s.TouchedTargets) / float64(s.TotalTargets)
	return ratio < d.coverageThreshold
}

// Targets returns a snapshot copy of current targets.
func (d *DiffCoverageLedger) Targets() []*CoverageTarget {
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make([]*CoverageTarget, len(d.targets))
	for i, t := range d.targets {
		cp := *t
		out[i] = &cp
	}
	return out
}

// FormatCoverageDebt formats remaining pending hunks and guidance for prompt injection.
func FormatCoverageDebt(targets []*CoverageTarget, maxItems int) string {
	return formatCoverageDebtLocked(targets, maxItems)
}

func formatCoverageDebtLocked(targets []*CoverageTarget, maxItems int) string {
	var pending []*CoverageTarget
	for _, t := range targets {
		if t.Status == "pending" {
			pending = append(pending, t)
		}
	}
	if len(pending) == 0 {
		return ""
	}

	hasTiers := false
	for _, t := range targets {
		if t.Tier != "" {
			hasTiers = true
			break
		}
	}

	if !hasTiers {
		var lines []string
		count := 0
		for _, t := range pending {
			if count >= maxItems {
				break
			}
			lines = append(lines, fmt.Sprintf("- %s", describeCoverageDebtTarget(*t)))
			count++
		}
		if len(pending) > maxItems {
			lines = append(lines, fmt.Sprintf("- ... (%d more changed files)", len(pending)-maxItems))
		}
		return strings.Join([]string{
			"Coverage debt remains for these hunks:",
			strings.Join(lines, "\n"),
			"Do not finalize until the pending line ranges above have been inspected with readFile.",
			"grep or listDir alone do not count as coverage.",
		}, "\n")
	}

	var critPending, warmPending, optPending []*CoverageTarget
	for _, t := range pending {
		switch t.Tier {
		case TierCritical:
			critPending = append(critPending, t)
		case TierWarm:
			warmPending = append(warmPending, t)
		case TierOptional:
			optPending = append(optPending, t)
		}
	}

	var blocks []string
	blocks = append(blocks, "Coverage status:")

	if len(critPending) > 0 {
		var lines []string
		for i, t := range critPending {
			if i >= maxItems {
				lines = append(lines, fmt.Sprintf("  - ... (%d more)", len(critPending)-maxItems))
				break
			}
			lines = append(lines, fmt.Sprintf("  - %s", describeCoverageDebtTarget(*t)))
		}
		blocks = append(blocks, fmt.Sprintf(
			"CRITICAL pending (%d) — readFile the pending line ranges below before finalizing:\n%s",
			len(critPending), strings.Join(lines, "\n"),
		))
	} else {
		blocks = append(blocks, "CRITICAL pending: 0 — all critical files covered.")
	}

	if len(warmPending) > 0 {
		var lines []string
		for i, t := range warmPending {
			if i >= 5 {
				lines = append(lines, fmt.Sprintf("  - ... (%d more)", len(warmPending)-5))
				break
			}
			lines = append(lines, fmt.Sprintf("  - %s", describeCoverageDebtTarget(*t)))
		}
		blocks = append(blocks, fmt.Sprintf(
			"WARM pending (%d) — readFile the pending line ranges below if step budget allows:\n%s",
			len(warmPending), strings.Join(lines, "\n"),
		))
	}

	if len(optPending) > 0 {
		blocks = append(blocks, fmt.Sprintf(
			"OPTIONAL pending: %d (readFile only if a concrete hypothesis points to one of them).",
			len(optPending),
		))
	}

	touchedCount := len(targets) - len(pending)
	pct := 0
	if len(targets) > 0 {
		pct = int(float64(touchedCount) / float64(len(targets)) * 100)
	}

	blocks = append(blocks,
		fmt.Sprintf("Total coverage: %d/%d files fully covered (%d%%).", touchedCount, len(targets), pct),
		"A file is covered only when every hunk in the diff has been readFile'd. Reading one hunk of a multi-hunk file leaves the rest pending.",
		fmt.Sprintf("You may finalize once ALL critical files are covered AND total coverage >= %d%%.", int(TieredTotalCoverageThreshold*100)),
		"readFile counts as coverage; grep/listDir do not.",
	)

	return strings.Join(blocks, "\n")
}

func describeCoverageDebtTarget(target CoverageTarget) string {
	pending := PendingHunks(target)
	if len(pending) == 0 {
		if len(target.ChangedRanges) > 0 {
			return fmt.Sprintf("%s (all hunks covered)", target.File)
		}
		return target.File
	}
	return fmt.Sprintf("%s (%s)", target.File, formatRanges(pending, "pending lines"))
}

func formatRanges(ranges [][2]int, label string) string {
	if len(ranges) == 0 {
		return ""
	}
	var parts []string
	for i, r := range ranges {
		if i >= 3 {
			parts = append(parts, "...")
			break
		}
		if r[0] == r[1] {
			parts = append(parts, fmt.Sprintf("%d", r[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", r[0], r[1]))
		}
	}
	return fmt.Sprintf("%s %s", label, strings.Join(parts, ", "))
}

// FormatCoverageTargetsForPrompt formats coverage targets for the agent system or user prompt.
func FormatCoverageTargetsForPrompt(changedFiles []ChangedFile, maxItems int, fileTiers map[string]CoverageTier) string {
	ledger := NewDiffCoverageLedger(DiffCoverageLedgerParams{
		ChangedFiles: changedFiles,
		FileTiers:    fileTiers,
	})
	targets := ledger.Targets()
	if len(targets) == 0 {
		return ""
	}

	hasTiers := false
	for _, t := range targets {
		if t.Tier != "" {
			hasTiers = true
			break
		}
	}

	if !hasTiers {
		var lines []string
		for i, t := range targets {
			if i >= maxItems {
				lines = append(lines, fmt.Sprintf("- ... (%d more changed files)", len(targets)-maxItems))
				break
			}
			lines = append(lines, fmt.Sprintf("- %s", describeTarget(*t)))
		}
		return strings.Join(lines, "\n")
	}

	var critical, warm, optional []*CoverageTarget
	for _, t := range targets {
		switch t.Tier {
		case TierCritical:
			critical = append(critical, t)
		case TierWarm:
			warm = append(warm, t)
		case TierOptional:
			optional = append(optional, t)
		}
	}

	var blocks []string
	appendTierBlock := func(label string, group []*CoverageTarget, hint string) {
		if len(group) == 0 {
			return
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%s files (%d) — %s:", label, len(group), hint))
		for i, t := range group {
			if i >= maxItems {
				lines = append(lines, fmt.Sprintf("- ... (%d more %s files)", len(group)-maxItems, strings.ToLower(label)))
				break
			}
			lines = append(lines, fmt.Sprintf("- %s", describeTarget(*t)))
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}

	appendTierBlock("CRITICAL", critical, "every hunk listed must be readFile-covered before finalizing")
	appendTierBlock("WARM", warm, "full diff above; readFile the hunks if budget allows — partial reads count per-hunk")
	appendTierBlock("OPTIONAL", optional, "hunk headers only; readFile only if a concrete hypothesis points to them")

	blocks = append(blocks,
		fmt.Sprintf("Finalization rule: ALL critical files must be fully hunk-covered, AND total coverage must reach >= %d%%. Warm/optional contribute to the total.", int(TieredTotalCoverageThreshold*100)),
		"Hunk-covered = every line range listed for the file has been inside a readFile range. Reading the first hunk of a multi-hunk file does NOT cover the rest.",
	)

	return strings.Join(blocks, "\n\n")
}

func describeTarget(target CoverageTarget) string {
	if len(target.ChangedRanges) == 0 {
		return target.File
	}
	return fmt.Sprintf("%s (%s)", target.File, formatRanges(target.ChangedRanges, "changed lines"))
}

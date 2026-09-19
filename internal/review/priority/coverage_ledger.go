// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package priority

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CoverageTier classifies the priority tier of a file under review.
type CoverageTier string

const (
	TierCritical CoverageTier = "critical"
	TierWarm     CoverageTier = "warm"
	TierOptional CoverageTier = "optional"
)

// TargetStatus records whether all changed hunks in a file have been inspected.
type TargetStatus string

const (
	StatusPending TargetStatus = "pending"
	StatusTouched TargetStatus = "touched"
)

// LineInterval represents an inclusive [Start, End] range of 1-indexed lines.
type LineInterval struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Valid returns true if Start <= End and Start > 0.
func (iv LineInterval) Valid() bool {
	return iv.Start > 0 && iv.Start <= iv.End
}

// Contains returns true if the given line falls within the interval.
func (iv LineInterval) Contains(line int) bool {
	return line >= iv.Start && line <= iv.End
}

// Overlaps returns true if two intervals intersect.
func (iv LineInterval) Overlaps(other LineInterval) bool {
	return iv.Start <= other.End && other.Start <= iv.End
}

// MergeLineIntervals merges overlapping or contiguous intervals into an optimized, disjoint sorted list.
func MergeLineIntervals(intervals []LineInterval) []LineInterval {
	if len(intervals) <= 1 {
		return intervals
	}

	sorted := make([]LineInterval, 0, len(intervals))
	for _, iv := range intervals {
		if iv.Valid() {
			sorted = append(sorted, iv)
		}
	}

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Start == sorted[j].Start {
			return sorted[i].End < sorted[j].End
		}
		return sorted[i].Start < sorted[j].Start
	})

	if len(sorted) == 0 {
		return nil
	}

	merged := []LineInterval{sorted[0]}
	for i := 1; i < len(sorted); i++ {
		curr := sorted[i]
		lastIdx := len(merged) - 1
		last := merged[lastIdx]

		if curr.Start <= last.End+1 {
			if curr.End > last.End {
				merged[lastIdx].End = curr.End
			}
		} else {
			merged = append(merged, curr)
		}
	}

	return merged
}

// IntervalSubsumed returns true if the target interval is completely covered by the union of source intervals.
func IntervalSubsumed(target LineInterval, sources []LineInterval) bool {
	if !target.Valid() {
		return true
	}
	merged := MergeLineIntervals(sources)
	for _, s := range merged {
		if s.Start <= target.Start && s.End >= target.End {
			return true
		}
	}
	return false
}

// CalculateIntervalCoverageRatio returns the fraction (0.0 to 1.0) of target lines covered by sources.
func CalculateIntervalCoverageRatio(targets []LineInterval, sources []LineInterval) float64 {
	mergedTargets := MergeLineIntervals(targets)
	mergedSources := MergeLineIntervals(sources)

	if len(mergedTargets) == 0 {
		return 1.0
	}

	totalTargetLines := 0
	for _, t := range mergedTargets {
		totalTargetLines += (t.End - t.Start + 1)
	}

	if totalTargetLines == 0 {
		return 1.0
	}

	coveredLines := 0
	for _, t := range mergedTargets {
		for _, s := range mergedSources {
			if !t.Overlaps(s) {
				continue
			}
			overlapStart := max(t.Start, s.Start)
			overlapEnd := min(t.End, s.End)
			if overlapEnd >= overlapStart {
				coveredLines += (overlapEnd - overlapStart + 1)
			}
		}
	}

	ratio := float64(coveredLines) / float64(totalTargetLines)
	if ratio > 1.0 {
		ratio = 1.0
	}
	return ratio
}

// CoverageTouch records an individual tool inspection action.
type CoverageTouch struct {
	ToolName    string    `json:"tool_name"`
	FilePath    string    `json:"file_path"`
	StartLine   int       `json:"start_line"`
	EndLine     int       `json:"end_line"`
	Step        int       `json:"step"`
	Timestamp   time.Time `json:"timestamp"`
	ObservedBy  string    `json:"observed_by,omitempty"`
}

// CoverageTarget represents the verification status of a changed file in a pull request.
type CoverageTarget struct {
	File          string          `json:"file"`
	ChangedRanges []LineInterval  `json:"changed_ranges"`
	TouchedRanges []LineInterval  `json:"touched_ranges"`
	Status        TargetStatus    `json:"status"`
	Tier          CoverageTier    `json:"tier"`
	TouchedBy     []CoverageTouch `json:"touched_by"`
}

// IsFullyCovered checks whether every changed hunk in the file is subsumed by touched intervals.
func (ct *CoverageTarget) IsFullyCovered() bool {
	if len(ct.ChangedRanges) == 0 {
		return true
	}
	for _, hunk := range ct.ChangedRanges {
		if !IntervalSubsumed(hunk, ct.TouchedRanges) {
			return false
		}
	}
	return true
}

// CoverageRatio returns the fraction of modified lines that have been inspected.
func (ct *CoverageTarget) CoverageRatio() float64 {
	return CalculateIntervalCoverageRatio(ct.ChangedRanges, ct.TouchedRanges)
}

// CoverageSummary provides aggregate review progress statistics.
type CoverageSummary struct {
	TotalTargets    int      `json:"total_targets"`
	TouchedTargets  int      `json:"touched_targets"`
	PendingTargets  int      `json:"pending_targets"`
	OverallRatio    float64  `json:"overall_ratio"` // 0.0 to 1.0
	TouchedFiles    []string `json:"touched_files"`
	PendingFiles    []string `json:"pending_files"`

	// Tier counters
	CriticalTotal   int `json:"critical_total"`
	CriticalTouched int `json:"critical_touched"`
	CriticalPending int `json:"critical_pending"`

	WarmTotal       int `json:"warm_total"`
	WarmTouched     int `json:"warm_touched"`
	WarmPending     int `json:"warm_pending"`

	OptionalTotal   int `json:"optional_total"`
	OptionalTouched int `json:"optional_touched"`
	OptionalPending int `json:"optional_pending"`
}

// CoverageLedger tracks and enforces code review completeness across files and hunks.
type CoverageLedger struct {
	mu             sync.RWMutex
	targets        map[string]*CoverageTarget
	orderedFiles   []string
	tieredTotalReq float64
}

// NewCoverageLedger initializes an empty coverage ledger.
func NewCoverageLedger(tieredTotalReq ...float64) *CoverageLedger {
	req := 0.70 // Default 70% overall coverage required
	if len(tieredTotalReq) > 0 && tieredTotalReq[0] > 0 {
		req = tieredTotalReq[0]
	}

	return &CoverageLedger{
		targets:        make(map[string]*CoverageTarget),
		orderedFiles:   make([]string, 0),
		tieredTotalReq: req,
	}
}

// NormalizePath cleans and standardizes file paths for deterministic cross-platform comparison.
func NormalizePath(p string) string {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(p)))
	clean = strings.TrimPrefix(clean, "./")
	clean = strings.TrimPrefix(clean, "/")
	return clean
}

// ExtractChangedRangesFromPatch extracts modified right-side line ranges from a unified diff patch.
func ExtractChangedRangesFromPatch(patch string) []LineInterval {
	if strings.TrimSpace(patch) == "" {
		return nil
	}

	lines := strings.Split(patch, "\n")
	var intervals []LineInterval

	for _, line := range lines {
		if !strings.HasPrefix(line, "@@") {
			continue
		}

		// Find second "@@"
		parts := strings.Split(line, "@@")
		if len(parts) < 3 {
			continue
		}

		header := strings.TrimSpace(parts[1])
		// header looks like "-oldStart,oldCount +newStart,newCount"
		subParts := strings.Fields(header)
		for _, sp := range subParts {
			if strings.HasPrefix(sp, "+") {
				newRangeStr := strings.TrimPrefix(sp, "+")
				rangeParts := strings.Split(newRangeStr, ",")
				start, err := strconv.Atoi(rangeParts[0])
				if err != nil || start <= 0 {
					continue
				}

				count := 1
				if len(rangeParts) > 1 {
					if c, err := strconv.Atoi(rangeParts[1]); err == nil && c >= 0 {
						count = c
					}
				}

				if count == 0 {
					// Pure deletion, line range is single anchor line
					intervals = append(intervals, LineInterval{Start: start, End: start})
				} else {
					intervals = append(intervals, LineInterval{Start: start, End: start + count - 1})
				}
			}
		}
	}

	return MergeLineIntervals(intervals)
}

// RegisterTarget adds a changed file to the coverage ledger with its hunks and priority tier.
func (cl *CoverageLedger) RegisterTarget(filePath string, patch string, tier CoverageTier) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	norm := NormalizePath(filePath)
	if norm == "" {
		return
	}

	changedRanges := ExtractChangedRangesFromPatch(patch)

	if target, exists := cl.targets[norm]; exists {
		target.ChangedRanges = changedRanges
		target.Tier = tier
		if target.IsFullyCovered() {
			target.Status = StatusTouched
		} else {
			target.Status = StatusPending
		}
		return
	}

	target := &CoverageTarget{
		File:          norm,
		ChangedRanges: changedRanges,
		TouchedRanges: make([]LineInterval, 0),
		Status:        StatusPending,
		Tier:          tier,
		TouchedBy:     make([]CoverageTouch, 0),
	}

	if len(changedRanges) == 0 {
		target.Status = StatusTouched
	}

	cl.targets[norm] = target
	cl.orderedFiles = append(cl.orderedFiles, norm)
}

// RecordObservation logs an inspection action and updates hunk coverage status.
func (cl *CoverageLedger) RecordObservation(
	filePath string,
	startLine int,
	endLine int,
	toolName string,
	step int,
	observedBy string,
) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	norm := NormalizePath(filePath)
	target, exists := cl.targets[norm]
	if !exists {
		// Attempt suffix matching for partial paths (e.g. "pkg/handler.go" matching "services/pkg/handler.go")
		for k, v := range cl.targets {
			if strings.HasSuffix(k, norm) || strings.HasSuffix(norm, k) {
				target = v
				norm = k
				exists = true
				break
			}
		}
		if !exists {
			return false
		}
	}

	if startLine <= 0 {
		startLine = 1
	}
	if endLine < startLine {
		endLine = startLine
	}

	newInterval := LineInterval{Start: startLine, End: endLine}
	target.TouchedRanges = append(target.TouchedRanges, newInterval)
	target.TouchedRanges = MergeLineIntervals(target.TouchedRanges)

	touch := CoverageTouch{
		ToolName:   toolName,
		FilePath:   norm,
		StartLine:  startLine,
		EndLine:    endLine,
		Step:       step,
		Timestamp:  time.Now().UTC(),
		ObservedBy: observedBy,
	}
	target.TouchedBy = append(target.TouchedBy, touch)

	wasPending := target.Status == StatusPending
	if target.IsFullyCovered() {
		target.Status = StatusTouched
	}

	return wasPending && target.Status == StatusTouched
}

// MarkFileTouched marks all hunks in a file as inspected (e.g. whole-file AST review).
func (cl *CoverageLedger) MarkFileTouched(filePath string, toolName string, step int, observedBy string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	norm := NormalizePath(filePath)
	target, exists := cl.targets[norm]
	if !exists {
		return
	}

	for _, hunk := range target.ChangedRanges {
		target.TouchedRanges = append(target.TouchedRanges, hunk)
	}
	target.TouchedRanges = MergeLineIntervals(target.TouchedRanges)

	target.TouchedBy = append(target.TouchedBy, CoverageTouch{
		ToolName:   toolName,
		FilePath:   norm,
		StartLine:  1,
		EndLine:    999999,
		Step:       step,
		Timestamp:  time.Now().UTC(),
		ObservedBy: observedBy,
	})

	target.Status = StatusTouched
}

// GetSummary compiles aggregate coverage metrics.
func (cl *CoverageLedger) GetSummary() CoverageSummary {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	summary := CoverageSummary{
		TotalTargets: len(cl.targets),
		TouchedFiles: make([]string, 0),
		PendingFiles: make([]string, 0),
	}

	totalRatios := 0.0

	for _, file := range cl.orderedFiles {
		target := cl.targets[file]
		ratio := target.CoverageRatio()
		totalRatios += ratio

		switch target.Tier {
		case TierCritical:
			summary.CriticalTotal++
			if target.Status == StatusTouched {
				summary.CriticalTouched++
			} else {
				summary.CriticalPending++
			}
		case TierWarm:
			summary.WarmTotal++
			if target.Status == StatusTouched {
				summary.WarmTouched++
			} else {
				summary.WarmPending++
			}
		case TierOptional:
			summary.OptionalTotal++
			if target.Status == StatusTouched {
				summary.OptionalTouched++
			} else {
				summary.OptionalPending++
			}
		}

		if target.Status == StatusTouched {
			summary.TouchedTargets++
			summary.TouchedFiles = append(summary.TouchedFiles, target.File)
		} else {
			summary.PendingTargets++
			summary.PendingFiles = append(summary.PendingFiles, target.File)
		}
	}

	if summary.TotalTargets > 0 {
		summary.OverallRatio = totalRatios / float64(summary.TotalTargets)
	} else {
		summary.OverallRatio = 1.0
	}

	return summary
}

// IsCoverageSatisfied checks whether review invariants have been fulfilled:
// 1. 100% of Critical tier files must be completely touched.
// 2. Warm tier files must meet the tiered threshold.
// 3. Overall ratio must meet tieredTotalReq.
func (cl *CoverageLedger) IsCoverageSatisfied() (bool, string) {
	summary := cl.GetSummary()

	// 1. Critical tier requirement
	if summary.CriticalPending > 0 {
		return false, fmt.Sprintf("Critical coverage incomplete: %d of %d critical files remain uninspected",
			summary.CriticalPending, summary.CriticalTotal)
	}

	// 2. Overall ratio requirement
	if summary.TotalTargets > 0 && summary.OverallRatio < cl.tieredTotalReq {
		return false, fmt.Sprintf("Overall coverage %.1f%% is below minimum required threshold %.1f%% (%d pending files)",
			summary.OverallRatio*100.0, cl.tieredTotalReq*100.0, summary.PendingTargets)
	}

	return true, "Coverage invariants satisfied"
}

// FormatCoverageReminderPrompt generates a structured reminder markdown prompt
// advising the agent model which high-priority files and line ranges still require inspection.
func (cl *CoverageLedger) FormatCoverageReminderPrompt() string {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	var pendingCritical []string
	var pendingWarm []string
	var pendingOptional []string

	for _, file := range cl.orderedFiles {
		target := cl.targets[file]
		if target.Status == StatusTouched {
			continue
		}

		var uninspectedHunks []string
		for _, hunk := range target.ChangedRanges {
			if !IntervalSubsumed(hunk, target.TouchedRanges) {
				uninspectedHunks = append(uninspectedHunks, fmt.Sprintf("L%d-L%d", hunk.Start, hunk.End))
			}
		}

		hunkDesc := strings.Join(uninspectedHunks, ", ")
		if hunkDesc == "" {
			hunkDesc = "all modified lines"
		}

		entry := fmt.Sprintf("- `%s` (unreviewed hunks: %s)", target.File, hunkDesc)

		switch target.Tier {
		case TierCritical:
			pendingCritical = append(pendingCritical, entry)
		case TierWarm:
			pendingWarm = append(pendingWarm, entry)
		case TierOptional:
			pendingOptional = append(pendingOptional, entry)
		}
	}

	if len(pendingCritical) == 0 && len(pendingWarm) == 0 && len(pendingOptional) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### ⚠️ ScanDrix Code Review Coverage Ledger Notice\n")
	sb.WriteString("The following modified files or hunks have not yet been inspected with read tools:\n\n")

	if len(pendingCritical) > 0 {
		sb.WriteString("**Critical Priority (Mandatory Inspection Required)**:\n")
		for _, item := range pendingCritical {
			sb.WriteString(item + "\n")
		}
		sb.WriteString("\n")
	}

	if len(pendingWarm) > 0 {
		sb.WriteString("**Standard Priority**:\n")
		for _, item := range pendingWarm {
			sb.WriteString(item + "\n")
		}
		sb.WriteString("\n")
	}

	if len(pendingOptional) > 0 && (len(pendingCritical) > 0 || len(pendingWarm) > 0) {
		sb.WriteString(fmt.Sprintf("**Optional Priority**: %d files pending (e.g. tests, documentation)\n\n", len(pendingOptional)))
	}

	sb.WriteString("Please inspect the unreviewed critical and standard hunks before finalizing your review verdict.")
	return sb.String()
}

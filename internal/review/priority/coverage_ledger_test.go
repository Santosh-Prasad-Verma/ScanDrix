// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package priority_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/scandrix/backend/internal/review/priority"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLineInterval_Operations(t *testing.T) {
	t.Run("Valid and Contains", func(t *testing.T) {
		iv := priority.LineInterval{Start: 10, End: 20}
		assert.True(t, iv.Valid())
		assert.True(t, iv.Contains(10))
		assert.True(t, iv.Contains(15))
		assert.True(t, iv.Contains(20))
		assert.False(t, iv.Contains(9))
		assert.False(t, iv.Contains(21))

		invalid := priority.LineInterval{Start: 25, End: 20}
		assert.False(t, invalid.Valid())
	})

	t.Run("Overlaps", func(t *testing.T) {
		a := priority.LineInterval{Start: 10, End: 20}
		b := priority.LineInterval{Start: 15, End: 25}
		c := priority.LineInterval{Start: 21, End: 30}
		d := priority.LineInterval{Start: 1, End: 9}

		assert.True(t, a.Overlaps(b))
		assert.True(t, b.Overlaps(a))
		assert.False(t, a.Overlaps(c))
		assert.False(t, a.Overlaps(d))
	})

	t.Run("MergeLineIntervals", func(t *testing.T) {
		input := []priority.LineInterval{
			{Start: 15, End: 25},
			{Start: 10, End: 14}, // contiguous with 15
			{Start: 30, End: 40},
			{Start: 35, End: 45}, // overlapping
			{Start: 50, End: 55},
		}

		merged := priority.MergeLineIntervals(input)
		require.Len(t, merged, 3)

		assert.Equal(t, priority.LineInterval{Start: 10, End: 25}, merged[0])
		assert.Equal(t, priority.LineInterval{Start: 30, End: 45}, merged[1])
		assert.Equal(t, priority.LineInterval{Start: 50, End: 55}, merged[2])
	})

	t.Run("IntervalSubsumed", func(t *testing.T) {
		sources := []priority.LineInterval{
			{Start: 1, End: 10},
			{Start: 11, End: 20},
		}

		assert.True(t, priority.IntervalSubsumed(priority.LineInterval{Start: 5, End: 15}, sources))
		assert.False(t, priority.IntervalSubsumed(priority.LineInterval{Start: 5, End: 25}, sources))
	})

	t.Run("CalculateIntervalCoverageRatio", func(t *testing.T) {
		targets := []priority.LineInterval{
			{Start: 1, End: 10},  // 10 lines
			{Start: 21, End: 30}, // 10 lines
		} // total = 20 lines

		sources := []priority.LineInterval{
			{Start: 1, End: 10}, // covers all first target (10 lines)
			{Start: 21, End: 25}, // covers half second target (5 lines)
		} // total covered = 15 lines -> 75%

		ratio := priority.CalculateIntervalCoverageRatio(targets, sources)
		assert.InDelta(t, 0.75, ratio, 0.001)
	})
}

func TestExtractChangedRangesFromPatch(t *testing.T) {
	patch := `diff --git a/pkg/auth/token.go b/pkg/auth/token.go
--- a/pkg/auth/token.go
+++ b/pkg/auth/token.go
@@ -10,3 +10,5 @@
 func Verify() {
+    log.Println("start")
+    check()
@@ -40,2 +42,0 @@
-func Old() {
`
	ranges := priority.ExtractChangedRangesFromPatch(patch)
	require.Len(t, ranges, 2)

	assert.Equal(t, priority.LineInterval{Start: 10, End: 14}, ranges[0])
	assert.Equal(t, priority.LineInterval{Start: 42, End: 42}, ranges[1])
}

func TestCoverageLedger_Lifecycle(t *testing.T) {
	ledger := priority.NewCoverageLedger(0.60) // 60% threshold (2 of 3 touched = 66.7%)

	patchAuth := "@@ -10,5 +10,10 @@\n"   // L10-L19 (10 lines)
	patchHandler := "@@ -20,2 +20,4 @@\n" // L20-L23 (4 lines)
	patchDoc := "@@ -1,1 +1,2 @@\n"       // L1-L2 (2 lines)

	ledger.RegisterTarget("services/auth/token.go", patchAuth, priority.TierCritical)
	ledger.RegisterTarget("services/api/handler.go", patchHandler, priority.TierWarm)
	ledger.RegisterTarget("README.md", patchDoc, priority.TierOptional)

	// Initially all pending
	summary := ledger.GetSummary()
	assert.Equal(t, 3, summary.TotalTargets)
	assert.Equal(t, 0, summary.TouchedTargets)
	assert.Equal(t, 3, summary.PendingTargets)
	assert.Equal(t, 1, summary.CriticalPending)
	assert.Equal(t, 1, summary.WarmPending)
	assert.Equal(t, 1, summary.OptionalPending)

	satisfied, reason := ledger.IsCoverageSatisfied()
	assert.False(t, satisfied)
	assert.Contains(t, reason, "Critical coverage incomplete")

	// Inspect partial auth file (L10-L14)
	touchedAll := ledger.RecordObservation("services/auth/token.go", 10, 14, "readFile", 1, "security_agent")
	assert.False(t, touchedAll, "partial hunk coverage does not flip status to touched")

	// Complete inspecting auth file (L15-L20)
	touchedAll = ledger.RecordObservation("services/auth/token.go", 15, 20, "readFile", 2, "security_agent")
	assert.True(t, touchedAll, "subsumed all hunks -> status flipped to touched")

	summary = ledger.GetSummary()
	assert.Equal(t, 1, summary.TouchedTargets)
	assert.Equal(t, 0, summary.CriticalPending)
	assert.Equal(t, 1, summary.CriticalTouched)

	// Now check prompt reminder: auth is satisfied, but handler is pending
	reminder := ledger.FormatCoverageReminderPrompt()
	assert.Contains(t, reminder, "Standard Priority")
	assert.Contains(t, reminder, "services/api/handler.go")
	assert.NotContains(t, reminder, "services/auth/token.go")

	// Mark handler touched via MarkFileTouched
	ledger.MarkFileTouched("services/api/handler.go", "readWholeFile", 3, "generalist_agent")

	// Now Critical and Warm are 100% satisfied
	satisfied, _ = ledger.IsCoverageSatisfied()
	assert.True(t, satisfied, "critical and warm satisfied, meeting 70% threshold")
}

func TestCoverageLedger_HighConcurrencyRaceTest(t *testing.T) {
	ledger := priority.NewCoverageLedger(0.70)

	for i := 1; i <= 50; i++ {
		patch := fmt.Sprintf("@@ -1,5 +1,%d @@\n", (i*2)+5)
		tier := priority.TierWarm
		if i%5 == 0 {
			tier = priority.TierCritical
		} else if i%3 == 0 {
			tier = priority.TierOptional
		}
		ledger.RegisterTarget(fmt.Sprintf("pkg/mod%d/file.go", i), patch, tier)
	}

	var wg sync.WaitGroup
	concurrency := 100

	for c := 0; c < concurrency; c++ {
		wg.Add(1)
		go func(iter int) {
			defer wg.Done()
			fileIdx := (iter % 50) + 1
			filePath := fmt.Sprintf("pkg/mod%d/file.go", fileIdx)
			start := (iter % 10) + 1
			end := start + 5

			ledger.RecordObservation(filePath, start, end, "grepTool", iter, "agent-concurrency")
			_ = ledger.GetSummary()
			_ = ledger.FormatCoverageReminderPrompt()
			_, _ = ledger.IsCoverageSatisfied()
		}(c)
	}

	wg.Wait()

	summary := ledger.GetSummary()
	assert.Equal(t, 50, summary.TotalTargets)
}

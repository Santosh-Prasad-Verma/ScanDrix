// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package orchestrator

import (
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const samplePatch = `@@ -10,6 +10,12 @@ package main
 import "fmt"
 
 func run() {
+	val := compute()
+	if val == nil {
+		return
+	}
+	fmt.Println(val)
 }
@@ -30,5 +36,5 @@ func compute() *Data {
-	return nil
+	return &Data{Value: 42}
 }`

func TestDiffBoundaryValidator_ExtractValidDiffLines(t *testing.T) {
	v := NewDiffBoundaryValidator()

	ranges := v.ExtractValidDiffLines(samplePatch)
	require.Len(t, ranges, 2)

	// First hunk starts at 10, covers context and additions
	assert.Equal(t, 10, ranges[0].Start)
	assert.True(t, ranges[0].End >= 18)

	// Second hunk starts at line 36
	assert.Equal(t, 36, ranges[1].Start)
}

func TestDiffBoundaryValidator_IsLineInDiff(t *testing.T) {
	v := NewDiffBoundaryValidator()

	assert.True(t, v.IsLineInDiff(samplePatch, 10))
	assert.True(t, v.IsLineInDiff(samplePatch, 13))
	assert.True(t, v.IsLineInDiff(samplePatch, 36))

	assert.False(t, v.IsLineInDiff(samplePatch, 1))
	assert.False(t, v.IsLineInDiff(samplePatch, 25))
	assert.False(t, v.IsLineInDiff(samplePatch, 99))
}

func TestDiffBoundaryValidator_ClipToValidRange(t *testing.T) {
	v := NewDiffBoundaryValidator()

	// Line completely within hunk
	start, end, ok := v.ClipToValidRange(samplePatch, 12, 14)
	assert.True(t, ok)
	assert.Equal(t, 12, start)
	assert.Equal(t, 14, end)

	// Line nearby hunk (within 3 lines) snaps
	start, end, ok = v.ClipToValidRange(samplePatch, 8, 9)
	assert.True(t, ok)
	assert.Equal(t, 10, start)

	// Line far away from any hunk
	_, _, ok = v.ClipToValidRange(samplePatch, 90, 95)
	assert.False(t, ok)
}

func TestDiffBoundaryValidator_FilterFindings(t *testing.T) {
	v := NewDiffBoundaryValidator()

	files := []ChangedFile{
		{
			Filename: "main.go",
			Patch:    samplePatch,
		},
	}

	findings := []AgentFinding{
		{
			ID:        uuid.New(),
			FilePath:  "main.go",
			StartLine: 12,
			EndLine:   14,
			Severity:  models.SeverityHigh,
			Title:     "Nil check vulnerability",
		},
		{
			ID:        uuid.New(),
			FilePath:  "main.go",
			StartLine: 90,
			EndLine:   95,
			Severity:  models.SeverityLow,
			Title:     "Distant issue",
		},
		{
			ID:        uuid.New(),
			FilePath:  "README.md",
			StartLine: 1,
			EndLine:   5,
			Severity:  models.SeverityInfo,
			Title:     "Doc smell",
		},
	}

	accepted, discarded := v.FilterFindings(findings, files)
	assert.Len(t, accepted, 2) // main.go line 12 and README.md (no patch)
	assert.Len(t, discarded, 1) // main.go line 90
	assert.Equal(t, "Distant issue", discarded[0].Title)
}

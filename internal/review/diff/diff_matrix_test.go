// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package diff

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffMatrix_ExtendedGitHeaders(t *testing.T) {
	rawDiff := `diff --git a/pkg/util/old_name.go b/pkg/util/new_name.go
similarity index 92%
rename from pkg/util/old_name.go
rename to pkg/util/new_name.go
--- a/pkg/util/old_name.go
+++ b/pkg/util/new_name.go
@@ -10,4 +10,5 @@ package util
 func Helper() {
-    println("old")
+    println("new")
+    println("extra")
 }
diff --git a/scripts/run.sh b/scripts/run.sh
old mode 100644
new mode 100755
--- a/scripts/run.sh
+++ b/scripts/run.sh
@@ -1,2 +1,3 @@
 #!/bin/bash
+echo "Starting service"
 exit 0
diff --git a/assets/logo.png b/assets/logo.png
new file mode 100644
Binary files /dev/null and b/assets/logo.png differ
diff --git a/submodules/shared b/submodules/shared
index abcdef1..1234567 160000
--- a/submodules/shared
+++ b/submodules/shared
@@ -1 +1 @@
-Subproject commit abcdef1234567890abcdef1234567890abcdef12
+Subproject commit 1234567890abcdef1234567890abcdef12345678
`

	patches, err := ParseExtendedUnifiedDiff(strings.NewReader(rawDiff))
	require.NoError(t, err)
	require.Len(t, patches, 4)

	// 1. Rename verification
	renamePatch := patches[0]
	assert.Equal(t, "pkg/util/old_name.go", renamePatch.OldPath)
	assert.Equal(t, "pkg/util/new_name.go", renamePatch.NewPath)
	assert.True(t, renamePatch.IsRename)
	assert.Equal(t, "pkg/util/old_name.go", renamePatch.RenameFrom)
	assert.Equal(t, "pkg/util/new_name.go", renamePatch.RenameTo)
	assert.Equal(t, 92, renamePatch.SimilarityIndex)
	assert.Equal(t, 2, renamePatch.Additions)
	assert.Equal(t, 1, renamePatch.Deletions)

	// 2. Mode change verification
	modePatch := patches[1]
	assert.Equal(t, "scripts/run.sh", modePatch.NewPath)
	assert.Equal(t, "100644", modePatch.OldMode)
	assert.Equal(t, "100755", modePatch.NewMode)
	assert.Equal(t, 1, modePatch.Additions)

	// 3. Binary file verification
	binaryPatch := patches[2]
	assert.Equal(t, "assets/logo.png", binaryPatch.NewPath)
	assert.True(t, binaryPatch.IsBinary)
	assert.True(t, binaryPatch.IsNew)
	assert.Equal(t, "100644", binaryPatch.NewMode)

	// 4. Submodule commit verification
	subPatch := patches[3]
	assert.Equal(t, "submodules/shared", subPatch.NewPath)
	assert.True(t, subPatch.IsSubmodule)
	assert.Equal(t, "abcdef1234567890abcdef1234567890abcdef12", subPatch.SubmoduleOldCommit)
	assert.Equal(t, "1234567890abcdef1234567890abcdef12345678", subPatch.SubmoduleNewCommit)
}

func TestDiffMatrix_SCMCoordinateResolver(t *testing.T) {
	rawDiff := `diff --git a/internal/handler.go b/internal/handler.go
--- a/internal/handler.go
+++ b/internal/handler.go
@@ -10,6 +10,7 @@ func HandleRequest(w http.ResponseWriter, r *http.Request) {
     if r.Method != "POST" {
         return
     }
-    processV1(r)
+    // Added comment
+    processV2(r)
     w.WriteHeader(200)
 }
@@ -30,4 +31,4 @@ func processV2(r *http.Request) {
     log.Println("v2")
-    return
+    return nil
 }
`

	patches, err := ParseExtendedUnifiedDiff(strings.NewReader(rawDiff))
	require.NoError(t, err)
	require.Len(t, patches, 1)

	patch := patches[0]
	resolver := NewSCMCoordinateResolver()

	// 1. Check line 14 on RIGHT side (line is "+    processV2(r)")
	// In hunk 1:
	// pos 1: @@ -10,6 +10,7 @@
	// pos 2:      if r.Method != "POST" { (ctx, old 10, new 10)
	// pos 3:          return               (ctx, old 11, new 11)
	// pos 4:      }                        (ctx, old 12, new 12)
	// pos 5: -    processV1(r)             (del, old 13)
	// pos 6: +    // Added comment         (add, new 13)
	// pos 7: +    processV2(r)             (add, new 14)
	coord := resolver.ResolveLinePosition(patch, 14, "RIGHT")
	assert.True(t, coord.IsValidTarget)
	assert.Equal(t, 7, coord.DiffPosition)
	assert.Equal(t, 0, coord.HunkIndex)
	assert.Equal(t, "RIGHT", coord.Side)

	// 2. Check line 13 on LEFT side (line is "-    processV1(r)")
	coordLeft := resolver.ResolveLinePosition(patch, 13, "LEFT")
	assert.True(t, coordLeft.IsValidTarget)
	assert.Equal(t, 5, coordLeft.DiffPosition)
	assert.Equal(t, "LEFT", coordLeft.Side)

	// 3. Check line outside any hunk (e.g. line 20)
	coordOutside := resolver.ResolveLinePosition(patch, 20, "RIGHT")
	assert.False(t, coordOutside.IsValidTarget)

	// 4. Validate multi-line range within single hunk (lines 13 to 15)
	start, end, ok := resolver.ValidateMultiLineRange(patch, 13, 15, "RIGHT")
	assert.True(t, ok)
	assert.Equal(t, 13, start)
	assert.Equal(t, 15, end)

	// 5. Multi-line range jumping across hunks clamps to first hunk end
	cStart, cEnd, cOk := resolver.ValidateMultiLineRange(patch, 13, 33, "RIGHT")
	assert.True(t, cOk)
	assert.Equal(t, 13, cStart)
	assert.Equal(t, 16, cEnd) // First hunk ends at new line 10 + 7 - 1 = 16
}

func TestDiffMatrix_ChunkingByTokenBudget(t *testing.T) {
	patch1 := &ExtendedFilePatch{
		NewPath: "services/user.go",
		Hunks: []Hunk{
			{
				Header:   "User service hunk",
				NewStart: 1,
				NewLines: 10,
				Lines: []DiffLine{
					{Type: LineAddition, Content: strings.Repeat("a", 200)},
				},
			},
		},
	}

	patch1Test := &ExtendedFilePatch{
		NewPath: "services/user_test.go",
		Hunks: []Hunk{
			{
				Header:   "User test hunk",
				NewStart: 1,
				NewLines: 10,
				Lines: []DiffLine{
					{Type: LineAddition, Content: strings.Repeat("b", 200)},
				},
			},
		},
	}

	patch2 := &ExtendedFilePatch{
		NewPath: "services/billing.go",
		Hunks: []Hunk{
			{
				Header:   "Billing service hunk",
				NewStart: 1,
				NewLines: 10,
				Lines: []DiffLine{
					{Type: LineAddition, Content: strings.Repeat("c", 200)},
				},
			},
		},
	}

	chunker := NewDiffChunker()
	batches := chunker.ChunkPatchesByTokenBudget([]*ExtendedFilePatch{patch1, patch2, patch1Test}, 250)
	require.NotEmpty(t, batches)

	// All 3 patches must be preserved across batches
	totalCount := 0
	for _, b := range batches {
		totalCount += len(b)
	}
	assert.Equal(t, 3, totalCount)
}

func TestDiffMatrix_PatchSynthesisAndRoundTrip(t *testing.T) {
	originalPatch := &ExtendedFilePatch{
		OldPath:  "pkg/config/app.go",
		NewPath:  "pkg/config/app.go",
		OldMode:  "100644",
		NewMode:  "100644",
		IsRename: false,
		Hunks: []Hunk{
			{
				OldStart: 5,
				OldLines: 3,
				NewStart: 5,
				NewLines: 4,
				Header:   "type AppConfig struct",
				Lines: []DiffLine{
					{Type: LineContext, OldLineNo: 5, NewLineNo: 5, Content: "type AppConfig struct {"},
					{Type: LineDeletion, OldLineNo: 6, NewLineNo: 0, Content: "\tPort int"},
					{Type: LineAddition, OldLineNo: 0, NewLineNo: 6, Content: "\tPort string"},
					{Type: LineAddition, OldLineNo: 0, NewLineNo: 7, Content: "\tHost string"},
					{Type: LineContext, OldLineNo: 7, NewLineNo: 8, Content: "}"},
				},
			},
		},
	}

	synthesizer := NewDiffSynthesizer()
	synthesizedText := synthesizer.SynthesizePatch(originalPatch)
	assert.Contains(t, synthesizedText, "diff --git a/pkg/config/app.go b/pkg/config/app.go")
	assert.Contains(t, synthesizedText, "@@ -5,3 +5,4 @@ type AppConfig struct")
	assert.Contains(t, synthesizedText, "+\tPort string")
	assert.Contains(t, synthesizedText, "-\tPort int")

	// Re-parse synthesized text to verify round-trip fidelity
	parsedPatches, err := ParseExtendedUnifiedDiff(strings.NewReader(synthesizedText))
	require.NoError(t, err)
	require.Len(t, parsedPatches, 1)

	reparsed := parsedPatches[0]
	assert.Equal(t, originalPatch.NewPath, reparsed.NewPath)
	assert.Equal(t, 2, reparsed.Additions)
	assert.Equal(t, 1, reparsed.Deletions)
	require.Len(t, reparsed.Hunks, 1)
	assert.Equal(t, 5, reparsed.Hunks[0].OldStart)
	assert.Equal(t, 3, reparsed.Hunks[0].OldLines)
	assert.Equal(t, 5, reparsed.Hunks[0].NewStart)
	assert.Equal(t, 4, reparsed.Hunks[0].NewLines)
}

func TestDiffMatrix_ConcurrentParsingThroughput(t *testing.T) {
	samplePatchText := `diff --git a/internal/worker.go b/internal/worker.go
--- a/internal/worker.go
+++ b/internal/worker.go
@@ -10,3 +10,4 @@ func RunWorker() {
     initQueue()
+    startConsumer()
     waitForShutdown()
 }
`

	workers := 16
	iterations := 100

	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				r := strings.NewReader(samplePatchText)
				patches, err := ParseExtendedUnifiedDiff(r)
				if err != nil {
					t.Errorf("worker %d iter %d error: %v", workerID, i, err)
					return
				}
				if len(patches) != 1 {
					t.Errorf("worker %d expected 1 patch got %d", workerID, len(patches))
					return
				}
				coordResolver := NewSCMCoordinateResolver()
				coord := coordResolver.ResolveLinePosition(patches[0], 12, "RIGHT")
				if !coord.IsValidTarget {
					t.Errorf("worker %d expected valid target", workerID)
					return
				}
			}
		}(w)
	}

	wg.Wait()
}

package diff_test

import (
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/review/diff"
)

func TestExtractHunkBoundariesAndSnapping(t *testing.T) {
	rawDiff := `diff --git a/auth/token.go b/auth/token.go
index 1111111..2222222 100644
--- a/auth/token.go
+++ b/auth/token.go
@@ -10,5 +10,7 @@ package auth
 func Validate() {
+    // check token
+    if token == "" { return }
 }
@@ -50,6 +52,8 @@ func Refresh() {
+    newToken := mint()
+    return newToken
 }
`
	patches, err := diff.ParseUnifiedDiff(strings.NewReader(rawDiff))
	if err != nil || len(patches) != 1 {
		t.Fatalf("failed parsing diff: %v", err)
	}

	intervals := diff.ExtractHunkLineBoundaries(patches[0])
	if len(intervals) != 2 {
		t.Fatalf("expected 2 hunk intervals, got %d", len(intervals))
	}

	// 1. Finding precisely on changed line 11-12 -> Should keep [11, 12]
	s1, e1, ok1 := diff.SnapFindingCoordinates(11, 12, intervals)
	if !ok1 || s1 != 11 || e1 != 12 {
		t.Fatalf("expected snapped [11, 12], got [%d, %d] ok=%v", s1, e1, ok1)
	}

	// 2. Finding overlapping boundary [8, 14] -> Should snap to [10, 14] (clamped to hunk)
	s2, e2, ok2 := diff.SnapFindingCoordinates(8, 14, intervals)
	_ = e2
	if !ok2 || s2 != 10 {
		t.Fatalf("expected start snapped to 10, got %d ok=%v", s2, ok2)
	}

	// 3. Finding on unchanged legacy line 30-35 -> Should return ok=false (drop to avoid SCM 422 error)
	_, _, ok3 := diff.SnapFindingCoordinates(30, 35, intervals)
	if ok3 {
		t.Fatal("expected out-of-hunk finding to be rejected (ok=false), but was accepted")
	}
}

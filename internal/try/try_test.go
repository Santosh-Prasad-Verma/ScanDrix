package try_test

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/try"
)

func TestParseGitHubURL(t *testing.T) {
	cases := []struct {
		url       string
		wantOwner string
		wantRepo  string
		wantPR    int
		wantErr   bool
	}{
		{"https://github.com/facebook/react/pull/24589", "facebook", "react", 24589, false},
		{"http://github.com/golang/go/pull/42/", "golang", "go", 42, false},
		{"https://github.com/invalid-format", "", "", 0, true},
		{"not-a-url", "", "", 0, true},
	}

	for _, c := range cases {
		owner, repo, pr, err := try.ParseGitHubURL(c.url)
		if (err != nil) != c.wantErr {
			t.Errorf("url %q: got err %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if !c.wantErr {
			if owner != c.wantOwner || repo != c.wantRepo || pr != c.wantPR {
				t.Errorf("url %q: got (%s, %s, %d), want (%s, %s, %d)", c.url, owner, repo, pr, c.wantOwner, c.wantRepo, c.wantPR)
			}
		}
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := try.NewRateLimiter(2, 100*time.Millisecond)

	// 1st request -> Allowed
	r1 := limiter.CheckAndRecord("device-1")
	if !r1.Allowed || r1.Remaining != 1 {
		t.Fatalf("expected 1st request allowed with 1 remaining, got %+v", r1)
	}

	// 2nd request -> Allowed
	r2 := limiter.CheckAndRecord("device-1")
	if !r2.Allowed || r2.Remaining != 0 {
		t.Fatalf("expected 2nd request allowed with 0 remaining, got %+v", r2)
	}

	// 3rd request -> Blocked
	r3 := limiter.CheckAndRecord("device-1")
	if r3.Allowed {
		t.Fatalf("expected 3rd request blocked, got %+v", r3)
	}

	// Different device -> Allowed
	rOther := limiter.CheckAndRecord("device-2")
	if !rOther.Allowed {
		t.Fatalf("expected device-2 allowed, got %+v", rOther)
	}

	// Wait for window to expire
	time.Sleep(120 * time.Millisecond)
	rAfter := limiter.CheckAndRecord("device-1")
	if !rAfter.Allowed {
		t.Fatalf("expected device-1 allowed after window expiration, got %+v", rAfter)
	}
}

func TestFeaturedRegistry(t *testing.T) {
	reg := try.NewFeaturedRegistry()
	summaries := reg.ListSummaries()
	if len(summaries) < 2 {
		t.Fatalf("expected at least 2 default featured summaries, got %d", len(summaries))
	}

	detail, ok := reg.GetDetail("react-fizz-ssr-abort")
	if !ok {
		t.Fatalf("expected 'react-fizz-ssr-abort' detail to exist")
	}
	if len(detail.Result.Issues) == 0 {
		t.Errorf("expected featured detail issues to be populated")
	}
}

func TestPublicServiceEnqueueAndPoll(t *testing.T) {
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())
	limiter := try.NewRateLimiter(5, time.Hour)
	featured := try.NewFeaturedRegistry()
	svc := try.NewPublicReviewService(limiter, featured, evaluator)

	_, err := svc.EnqueueReview(context.Background(), try.EnqueueRequest{
		PRURL:       "invalid-url",
		Fingerprint: "fp-test",
	})
	if err == nil {
		t.Fatalf("expected error on invalid URL")
	}
}

func TestParseUnifiedDiff(t *testing.T) {
	diffText := `diff --git a/src/index.ts b/src/index.ts
new file mode 100644
index 0000000..e69de29
--- /dev/null
+++ b/src/index.ts
@@ -0,0 +1,3 @@
+console.log("hello world");
+export const answer = 42;
+
diff --git a/src/old.ts b/src/new.ts
rename from src/old.ts
rename to src/new.ts
index e69de29..d69de29 100644
--- a/src/old.ts
+++ b/src/new.ts
@@ -1,3 +1,3 @@
-const oldVar = 1;
+const newVar = 2;
 const stable = 10;
-const toRemove = true;
\ No newline at end of file
`

	files := try.ParseUnifiedDiff(diffText)
	if len(files) != 2 {
		t.Fatalf("expected 2 files parsed, got %d", len(files))
	}

	// File 1: Added
	f1 := files[0]
	if f1.Path != "src/index.ts" || f1.Status != try.FileStatusAdded {
		t.Errorf("f1: expected added src/index.ts, got %s, status: %s", f1.Path, f1.Status)
	}
	if f1.Additions != 3 || f1.Deletions != 0 {
		t.Errorf("f1: expected +3/-0, got +%d/-%d", f1.Additions, f1.Deletions)
	}
	if len(f1.Hunks) != 1 || len(f1.Hunks[0].Lines) != 3 {
		t.Errorf("f1: unexpected hunk structure")
	}

	// File 2: Renamed / Modified
	f2 := files[1]
	if f2.Path != "src/new.ts" || f2.OldPath == nil || *f2.OldPath != "src/old.ts" {
		t.Errorf("f2: expected rename from src/old.ts to src/new.ts, got path %s, old %v", f2.Path, f2.OldPath)
	}
	if f2.Status != try.FileStatusRenamed {
		t.Errorf("f2: expected status renamed, got %s", f2.Status)
	}
	if f2.Additions != 1 || f2.Deletions != 2 {
		t.Errorf("f2: expected +1/-2, got +%d/-%d", f2.Additions, f2.Deletions)
	}

	// Test empty
	if empty := try.ParseUnifiedDiff(""); empty != nil {
		t.Errorf("expected empty diff to return nil, got %v", empty)
	}
}

func TestJobIdAndSlug(t *testing.T) {
	validUUID := "c23f66f9-012b-47e2-8926-d62f6b864a7c"
	invalidUUID := "not-a-uuid"
	validSlug := "react-fizz-ssr-abort"
	invalidSlug := "React Slug With Spaces!"

	if !try.IsJobID(validUUID) {
		t.Errorf("expected %s to be recognized as JobID", validUUID)
	}
	if try.IsJobID(validSlug) {
		t.Errorf("expected %s NOT to be recognized as JobID", validSlug)
	}

	if !try.IsFeaturedSlug(validSlug) {
		t.Errorf("expected %s to be recognized as featured slug", validSlug)
	}
	if try.IsFeaturedSlug(validUUID) {
		t.Errorf("expected %s NOT to be recognized as featured slug", validUUID)
	}

	if err := try.ValidateJobID(validUUID); err != nil {
		t.Errorf("valid UUID failed validation: %v", err)
	}
	if err := try.ValidateJobID(invalidUUID); err == nil {
		t.Errorf("invalid UUID unexpectedly passed validation")
	}

	if err := try.ValidateSlug(validSlug); err != nil {
		t.Errorf("valid slug failed validation: %v", err)
	}
	if err := try.ValidateSlug(invalidSlug); err == nil {
		t.Errorf("invalid slug unexpectedly passed validation")
	}
}

func TestFingerprint(t *testing.T) {
	fp, err := try.GenerateFingerprint()
	if err != nil {
		t.Fatalf("failed generating fingerprint: %v", err)
	}
	if len(fp) != 32 {
		t.Errorf("expected 32-char hex fingerprint, got %s", fp)
	}

	if err := try.ValidateFingerprint(fp); err != nil {
		t.Errorf("valid fingerprint failed validation: %v", err)
	}
	if err := try.ValidateFingerprint(""); err == nil {
		t.Errorf("empty fingerprint unexpectedly passed validation")
	}

	norm := try.NormalizeFingerprint("   test-fp   ")
	if norm != "test-fp" {
		t.Errorf("expected trimmed fingerprint, got %q", norm)
	}
}

func TestPreferences(t *testing.T) {
	def := try.DefaultPreferences()
	if def.DiffStyle != try.DiffStyleUnified || def.FileTreeMode != try.FileTreeModeGrouped {
		t.Errorf("unexpected default preferences: %+v", def)
	}

	if err := try.ValidatePreferences(def); err != nil {
		t.Errorf("default preferences failed validation: %v", err)
	}

	invalid := def
	invalid.DiffStyle = "invalid-style"
	if err := try.ValidatePreferences(invalid); err == nil {
		t.Errorf("invalid diff style unexpectedly passed validation")
	}

	invalidTree := def
	invalidTree.FileTreeMode = "invalid-mode"
	if err := try.ValidatePreferences(invalidTree); err == nil {
		t.Errorf("invalid file tree mode unexpectedly passed validation")
	}
}

func TestSnapshotStore(t *testing.T) {
	store := try.NewSnapshotStore(2)

	snap1 := try.ReviewSnapshot{
		PR:   try.PrInfo{Owner: "org", Repo: "repo1", PRNumber: 1, Title: "PR 1"},
		Diff: "diff 1",
	}
	snap2 := try.ReviewSnapshot{
		PR:   try.PrInfo{Owner: "org", Repo: "repo2", PRNumber: 2, Title: "PR 2"},
		Diff: "diff 2",
	}
	snap3 := try.ReviewSnapshot{
		PR:   try.PrInfo{Owner: "org", Repo: "repo3", PRNumber: 3, Title: "PR 3"},
		Diff: "diff 3",
	}

	store.Save("job-1", snap1)
	store.Save("job-2", snap2)

	loaded, ok := store.Load("job-1")
	if !ok || loaded.PR.Title != "PR 1" {
		t.Errorf("expected job-1 to load properly, got ok=%v, loaded=%+v", ok, loaded)
	}

	// Adding 3rd should evict the oldest
	time.Sleep(10 * time.Millisecond)
	store.Save("job-3", snap3)

	_, ok3 := store.Load("job-3")
	if !ok3 {
		t.Errorf("expected job-3 to be present")
	}

	store.Delete("job-3")
	if _, okDeleted := store.Load("job-3"); okDeleted {
		t.Errorf("expected job-3 to be deleted")
	}
}

func TestViewedStore(t *testing.T) {
	store := try.NewViewedStore()

	v1 := store.SetViewed("job-123", "src/file1.ts", true)
	if !v1["src/file1.ts"] {
		t.Errorf("expected src/file1.ts to be marked viewed")
	}

	store.SetViewed("job-123", "src/file2.ts", true)
	vMap := store.GetViewed("job-123")
	if len(vMap) != 2 || !vMap["src/file2.ts"] {
		t.Errorf("expected 2 viewed files, got %+v", vMap)
	}

	store.SetViewed("job-123", "src/file1.ts", false)
	if store.GetViewed("job-123")["src/file1.ts"] {
		t.Errorf("expected src/file1.ts to be marked false")
	}

	store.Clear("job-123")
	if len(store.GetViewed("job-123")) != 0 {
		t.Errorf("expected viewed map to be empty after clear")
	}
}

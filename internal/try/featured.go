package try

import (
	"sort"
	"sync"
	"time"
)

// FeaturedRegistry manages pre-curated review snapshots served instantly without polling.
type FeaturedRegistry struct {
	mu      sync.RWMutex
	reviews map[string]FeaturedReviewDetail
}

// NewFeaturedRegistry initializes registry with default curated snapshots.
func NewFeaturedRegistry() *FeaturedRegistry {
	r := &FeaturedRegistry{
		reviews: make(map[string]FeaturedReviewDetail),
	}
	r.seedDefaults()
	return r
}

func (r *FeaturedRegistry) seedDefaults() {
	now := time.Now().UTC()

	// NOTE: These are illustrative showcase examples for the marketing landing page,
	// NOT real upstream pull request data. IsDemonstration is set to true.

	// 1. React Fizz SSR Bug (Demonstration)
	r.reviews["react-fizz-ssr-abort"] = FeaturedReviewDetail{
		Slug:            "react-fizz-ssr-abort",
		IsDemonstration: true,
		Tags:      []string{"react", "ssr", "race-condition"},
		Highlight: "Uncaught promise rejection and aborted render stream race condition",
		PRURL:     "https://github.com/facebook/react/pull/24589",
		PR: PrInfo{
			Owner:        "facebook",
			Repo:         "react",
			PRNumber:     24589,
			Title:        "Fix SSR Fizz abort signal bubbling across concurrent boundaries",
			Additions:    45,
			Deletions:    12,
			ChangedFiles: 3,
			HTMLURL:      "https://github.com/facebook/react/pull/24589",
		},
		Diff: `diff --git a/packages/react-server/src/ReactFizzServer.js b/packages/react-server/src/ReactFizzServer.js
--- a/packages/react-server/src/ReactFizzServer.js
+++ b/packages/react-server/src/ReactFizzServer.js
@@ -102,4 +102,6 @@ function abortStream(request, error) {
+  if (request.status === CLOSED) return;
+  request.status = CLOSED;
+  request.destination.destroy(error);
 }`,
 		Result: ReviewResult{
 			Summary:       "Discovered 1 Critical race condition where unhandled abort signals corrupted response buffers.",
 			FilesAnalyzed: 3,
 			Duration:      840,
 			Issues: []ReviewIssue{
 				{
 					File:       "packages/react-server/src/ReactFizzServer.js",
 					Line:       102,
 					EndLine:    106,
 					Severity:   "CRITICAL",
 					Category:   "CONCURRENCY_BUG",
 					Message:    "State mutation without status check causes double stream destruction error in concurrent HTTP/2 responses.",
 					Suggestion: "if (request.status === CLOSED) return;\nrequest.status = CLOSED;\nrequest.destination.destroy(error);",
 				},
 			},
 		},
 		PublishedAt: now,
 	}

 	// 2. Go Crypto Timing Attack (Demonstration)
 	r.reviews["golang-crypto-timing-leak"] = FeaturedReviewDetail{
 		Slug:            "golang-crypto-timing-leak",
 		IsDemonstration: true,
 		Tags:      []string{"golang", "crypto", "timing-attack"},
 		Highlight: "Subtle non-constant-time token comparison leaking secret keys",
 		PRURL:     "https://github.com/golang/go/pull/48291",
 		PR: PrInfo{
 			Owner:        "golang",
 			Repo:         "go",
 			PRNumber:     48291,
 			Title:        "crypto/subtle: harden constant time byte slice comparisons",
 			Additions:    28,
 			Deletions:    8,
 			ChangedFiles: 2,
 			HTMLURL:      "https://github.com/golang/go/pull/48291",
 		},
 		Diff: `diff --git a/src/crypto/subtle/constant_time.go b/src/crypto/subtle/constant_time.go
--- a/src/crypto/subtle/constant_time.go
+++ b/src/crypto/subtle/constant_time.go
@@ -15,3 +15,3 @@ func ConstantTimeCompare(x, y []byte) int {
-	if len(x) != len(y) {
-		return 0
-	}
+	var v byte
+	for i := 0; i < len(x); i++ {
+		v |= x[i] ^ y[i]
+	}
+	return 1 & ((int(v) - 1) >> 31)`,
 		Result: ReviewResult{
 			Summary:       "Identified 1 High severity timing side-channel vulnerability in early-exit length checking.",
 			FilesAnalyzed: 2,
 			Duration:      620,
 			Issues: []ReviewIssue{
 				{
 					File:       "src/crypto/subtle/constant_time.go",
 					Line:       15,
 					EndLine:    22,
 					Severity:   "HIGH",
 					Category:   "SECURITY_VULNERABILITY",
 					Message:    "Early return on length mismatch leaks buffer length information across cryptographic boundaries.",
 					Suggestion: "Use constant-time length XOR masking to prevent side-channel timing disclosures.",
 				},
 			},
 		},
 		PublishedAt: now,
 	}
}

// ListSummaries returns lightweight metadata cards sorted for display.
func (r *FeaturedRegistry) ListSummaries() []FeaturedReviewSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []FeaturedReviewSummary
	for _, item := range r.reviews {
		list = append(list, FeaturedReviewSummary{
			Slug:            item.Slug,
			Tags:            item.Tags,
			Highlight:       item.Highlight,
			PRURL:           item.PRURL,
			PR:              item.PR,
			IssuesCount:     len(item.Result.Issues),
			SortOrder:       1,
			IsDemonstration: item.IsDemonstration,
		})
	}

	// Sort deterministically by Slug for stable display ordering.
	sort.Slice(list, func(i, j int) bool {
		return list[i].Slug < list[j].Slug
	})

	return list
}

// GetDetail returns the full cached snapshot for a given slug.
func (r *FeaturedRegistry) GetDetail(slug string) (FeaturedReviewDetail, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	detail, ok := r.reviews[slug]
	return detail, ok
}

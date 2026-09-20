package azuredevops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAzureDevOpsAdvancedService_IterationsAndThreads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/myorg/myproj/_apis/git/repositories/myrepo/pullRequests/10/iterations":
			resp := map[string]any{
				"value": []map[string]any{
					{
						"id":          1,
						"description": "Initial commit iteration",
						"author":      map[string]any{"displayName": "Alice Engineer"},
						"createdDate": time.Now().Format(time.RFC3339),
						"updatedDate": time.Now().Format(time.RFC3339),
						"sourceRefCommit": map[string]any{"commitId": "sha111"},
						"targetRefCommit": map[string]any{"commitId": "sha222"},
						"commonRefCommit": map[string]any{"commitId": "sha000"},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/git/repositories/myrepo/pullRequests/10/threads" && r.Method == http.MethodGet:
			resp := map[string]any{
				"value": []map[string]any{
					{
						"id":     101,
						"status": "active",
						"threadContext": map[string]any{
							"filePath": "/src/main.go",
							"rightFile": map[string]any{
								"line": 42,
							},
						},
						"comments": []map[string]any{
							{
								"id":              1,
								"parentCommentId": 0,
								"content":         "Consider using a sync.Pool here for allocation efficiency.",
								"commentType":     "text",
								"publishedDate":   time.Now().Format(time.RFC3339),
								"author":          map[string]any{"displayName": "Drixy Reviewer"},
							},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/git/repositories/myrepo/pullRequests/10/threads/101/comments" && r.Method == http.MethodPost:
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := map[string]any{
				"id":              2,
				"parentCommentId": 1,
				"content":         req["content"],
				"commentType":     "text",
				"publishedDate":   time.Now().Format(time.RFC3339),
				"author":          map[string]any{"displayName": "Alice Engineer"},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/git/repositories/myrepo/pullRequests/10/threads/101" && r.Method == http.MethodPatch:
			w.WriteHeader(http.StatusOK)

		case r.URL.Path == "/myorg/myproj/_apis/policy/evaluations":
			resp := map[string]any{
				"value": []map[string]any{
					{
						"evaluationId": "eval-1",
						"status":       "approved",
						"configuration": map[string]any{
							"type": map[string]any{
								"displayName": "Minimum number of reviewers",
							},
							"isBlocking": true,
						},
						"startedDate":   time.Now().Format(time.RFC3339),
						"completedDate": time.Now().Format(time.RFC3339),
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/git/repositories/myrepo/pullRequests/10/workitems":
			resp := map[string]any{
				"value": []map[string]any{
					{
						"id":  "4200",
						"url": "https://dev.azure.com/myorg/myproj/_workitems/edit/4200",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/git/repositories/myrepo/pullRequests/10/reviewers/rev-1" && r.Method == http.MethodPut:
			var req map[string]int
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := map[string]any{
				"id":          "rev-1",
				"displayName": "Drixy AI",
				"uniqueName":  "drixy@scandrix.dev",
				"vote":        req["vote"],
				"isRequired":  true,
				"hasDeclined": false,
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewAzureDevOpsAdvancedService(nil, server.Client(), server.URL)
	ctx := context.Background()

	// 1. Get iterations
	iterations, err := svc.GetPullRequestIterations(ctx, "mock-token", "myorg", "myproj", "myrepo", 10)
	if err != nil {
		t.Fatalf("GetPullRequestIterations failed: %v", err)
	}
	if len(iterations) != 1 || iterations[0].ID != 1 {
		t.Errorf("unexpected iterations: %+v", iterations)
	}

	// 2. Get threads
	threads, err := svc.GetPullRequestThreads(ctx, "mock-token", "myorg", "myproj", "myrepo", 10)
	if err != nil {
		t.Fatalf("GetPullRequestThreads failed: %v", err)
	}
	if len(threads) != 1 || threads[0].ID != 101 || threads[0].Line != 42 {
		t.Errorf("unexpected threads: %+v", threads)
	}

	// 3. Create thread comment
	comment, err := svc.CreateThreadComment(ctx, "mock-token", "myorg", "myproj", "myrepo", 10, 101, "Fixed in latest commit!")
	if err != nil {
		t.Fatalf("CreateThreadComment failed: %v", err)
	}
	if comment.ID != 2 || comment.Content != "Fixed in latest commit!" {
		t.Errorf("unexpected comment: %+v", comment)
	}

	// 4. Update thread status
	err = svc.UpdateThreadStatus(ctx, "mock-token", "myorg", "myproj", "myrepo", 10, 101, "fixed")
	if err != nil {
		t.Fatalf("UpdateThreadStatus failed: %v", err)
	}

	// 5. Policy evaluations
	evals, err := svc.GetPolicyEvaluations(ctx, "mock-token", "myorg", "myproj", "vstfs:///Git/PullRequestId/10")
	if err != nil {
		t.Fatalf("GetPolicyEvaluations failed: %v", err)
	}
	if len(evals) != 1 || evals[0].Status != "approved" {
		t.Errorf("unexpected policy evaluations: %+v", evals)
	}

	// 6. Work items
	workItems, err := svc.GetPullRequestWorkItems(ctx, "mock-token", "myorg", "myproj", "myrepo", 10)
	if err != nil {
		t.Fatalf("GetPullRequestWorkItems failed: %v", err)
	}
	if len(workItems) != 1 || workItems[0].ID != "4200" {
		t.Errorf("unexpected work items: %+v", workItems)
	}

	// 7. Set reviewer vote
	vote, err := svc.SetReviewerVote(ctx, "mock-token", "myorg", "myproj", "myrepo", 10, "rev-1", 10)
	if err != nil {
		t.Fatalf("SetReviewerVote failed: %v", err)
	}
	if vote.Vote != 10 || vote.UniqueName != "drixy@scandrix.dev" {
		t.Errorf("unexpected reviewer vote: %+v", vote)
	}

	// 8. Calculate diff line position
	diff := `diff --git a/pkg/service.go b/pkg/service.go
index abc..def 100644
--- a/pkg/service.go
+++ b/pkg/service.go
@@ -10,4 +10,5 @@
 package pkg
 
 func Run() {
+	println("running")
 }`
	rightLine, _, err := svc.CalculateDiffLinePosition(diff, "pkg/service.go", 13)
	if err != nil {
		t.Fatalf("CalculateDiffLinePosition failed: %v", err)
	}
	if rightLine != 13 {
		t.Errorf("expected right line 13, got %d", rightLine)
	}
}

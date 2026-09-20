// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGitHubGraphQLService_ReviewThreadsAndResolutions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req.Variables["prNumber"] != nil {
			// Query threads
			resp := map[string]any{
				"data": map[string]any{
					"repository": map[string]any{
						"pullRequest": map[string]any{
							"reviewThreads": map[string]any{
								"nodes": []map[string]any{
									{
										"id":         "PRRT_kwDOABC123",
										"isResolved": false,
										"path":       "main.go",
										"line":       42,
										"startLine":  40,
										"comments": map[string]any{
											"nodes": []map[string]any{
												{
													"id":        "PRRC_kwDOABC999",
													"body":      "Check error return value here.",
													"createdAt": time.Now().Format(time.RFC3339),
													"author": map[string]any{
														"login": "drixy-ai",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if req.Variables["threadId"] == "PRRT_kwDOABC123" {
			// Resolve/unresolve mutation
			resp := map[string]any{
				"data": map[string]any{
					"resolveReviewThread": map[string]any{
						"thread": map[string]any{
							"id":         "PRRT_kwDOABC123",
							"isResolved": true,
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	svc := NewGitHubGraphQLService(server.Client(), server.URL)
	ctx := context.Background()

	// 1. Get review threads
	threads, err := svc.GetPullRequestReviewThreads(ctx, "mock-token", "scandrix", "backend", 42)
	if err != nil {
		t.Fatalf("GetPullRequestReviewThreads failed: %v", err)
	}
	if len(threads) != 1 || threads[0].ID != "PRRT_kwDOABC123" || threads[0].Line != 42 {
		t.Errorf("unexpected threads: %+v", threads)
	}
	if len(threads[0].Comments) != 1 || threads[0].Comments[0].Author != "drixy-ai" {
		t.Errorf("unexpected comments: %+v", threads[0].Comments)
	}

	// 2. Resolve thread
	err = svc.ResolveReviewThread(ctx, "mock-token", "PRRT_kwDOABC123")
	if err != nil {
		t.Fatalf("ResolveReviewThread failed: %v", err)
	}
}

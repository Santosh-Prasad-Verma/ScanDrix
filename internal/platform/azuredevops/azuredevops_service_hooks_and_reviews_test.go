// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package azuredevops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAzureDevOps_ServiceHooks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/myorg/_apis/hooks/subscriptions" && r.Method == http.MethodGet:
			pub := r.URL.Query().Get("publisherId")
			evt := r.URL.Query().Get("eventType")
			if pub != "tfs" || evt != "git.pullrequest.created" {
				t.Errorf("unexpected query params: publisherId=%s, eventType=%s", pub, evt)
			}
			resp := map[string]any{
				"value": []map[string]any{
					{
						"id":               "sub-101",
						"url":              "https://dev.azure.com/myorg/_apis/hooks/subscriptions/sub-101",
						"status":           "enabled",
						"publisherId":      "tfs",
						"eventType":        "git.pullrequest.created",
						"consumerId":       "webHooks",
						"consumerActionId": "httpRequest",
						"consumerInputs": map[string]string{
							"url": "https://api.scandrix.dev/webhooks/azure",
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/_apis/hooks/subscriptions" && r.Method == http.MethodPost:
			var body ServiceHookSubscription
			_ = json.NewDecoder(r.Body).Decode(&body)
			resp := map[string]any{
				"id":               "sub-102",
				"url":              "https://dev.azure.com/myorg/_apis/hooks/subscriptions/sub-102",
				"status":           "enabled",
				"publisherId":      body.PublisherID,
				"eventType":        body.EventType,
				"consumerId":       body.ConsumerID,
				"consumerActionId": body.ConsumerActionID,
				"consumerInputs":   body.ConsumerInputs,
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/_apis/hooks/subscriptions/sub-102" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)

		default:
			t.Errorf("unhandled route: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewAzureDevOpsAdvancedService(nil, server.Client(), server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. List subscriptions
	subs, err := svc.ListServiceHookSubscriptions(ctx, "pat-token", "myorg", "tfs", "git.pullrequest.created")
	if err != nil {
		t.Fatalf("ListServiceHookSubscriptions failed: %v", err)
	}
	if len(subs) != 1 || subs[0].ID != "sub-101" {
		t.Errorf("unexpected subs: %+v", subs)
	}

	// 2. Create subscription
	newSub, err := svc.CreateServiceHookSubscription(ctx, "pat-token", "myorg", ServiceHookSubscription{
		EventType: "git.pullrequest.updated",
		ConsumerInputs: map[string]string{
			"url": "https://api.scandrix.dev/webhooks/azure",
		},
	})
	if err != nil {
		t.Fatalf("CreateServiceHookSubscription failed: %v", err)
	}
	if newSub.ID != "sub-102" || newSub.EventType != "git.pullrequest.updated" {
		t.Errorf("unexpected newSub: %+v", newSub)
	}

	// 3. Delete subscription
	if err := svc.DeleteServiceHookSubscription(ctx, "pat-token", "myorg", "sub-102"); err != nil {
		t.Fatalf("DeleteServiceHookSubscription failed: %v", err)
	}
}

func TestAzureDevOps_ReviewersAndThreads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/myorg/myproj/_apis/git/repositories/myrepo/pullRequests/42/reviewers" && r.Method == http.MethodGet:
			resp := map[string]any{
				"value": []map[string]any{
					{
						"id":          "rev-1",
						"displayName": "Drixy Reviewer",
						"uniqueName":  "drixy@scandrix.dev",
						"vote":        10,
						"isRequired":  true,
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/git/repositories/myrepo/pullRequests/42/reviewers/rev-1" && r.Method == http.MethodPut:
			var req ReviewerVoteRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := map[string]any{
				"id":          "rev-1",
				"displayName": "Drixy Reviewer",
				"uniqueName":  "drixy@scandrix.dev",
				"vote":        req.Vote,
				"isRequired":  req.IsRequired,
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/git/repositories/myrepo/pullRequests/42/threads/100" && r.Method == http.MethodPatch:
			var req PRThreadStatusUpdate
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Status != "fixed" {
				t.Errorf("expected status 'fixed', got %s", req.Status)
			}
			w.WriteHeader(http.StatusOK)

		case r.URL.Path == "/myorg/myproj/_apis/git/repositories/myrepo/pullRequests/42/threads/100/comments" && r.Method == http.MethodPost:
			var req ThreadCommentRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := map[string]any{
				"id":            201,
				"content":       req.Content,
				"publishedDate": time.Now().Format(time.RFC3339),
				"commentType":   "text",
				"author": map[string]any{
					"displayName": "Drixy AI",
				},
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(resp)

		default:
			t.Errorf("unhandled route: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewAzureDevOpsAdvancedService(nil, server.Client(), server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. List Reviewers
	reviewers, err := svc.ListReviewers(ctx, "pat-token", "myorg", "myproj", "myrepo", 42)
	if err != nil {
		t.Fatalf("ListReviewers failed: %v", err)
	}
	if len(reviewers) != 1 || reviewers[0].Vote != VoteApproved {
		t.Errorf("unexpected reviewers: %+v", reviewers)
	}

	// 2. Set Reviewer Vote
	updatedVote, err := svc.SetReviewerVoteWithDetails(ctx, "pat-token", "myorg", "myproj", "myrepo", 42, "rev-1", ReviewerVoteRequest{
		Vote:       VoteApprovedWithSuggestion,
		IsRequired: true,
	})
	if err != nil {
		t.Fatalf("SetReviewerVote failed: %v", err)
	}
	if updatedVote.Vote != VoteApprovedWithSuggestion {
		t.Errorf("expected VoteApprovedWithSuggestion, got %d", updatedVote.Vote)
	}

	// 3. Update Thread Status
	if err := svc.UpdateThreadStatusExtended(ctx, "pat-token", "myorg", "myproj", "myrepo", 42, 100, "fixed"); err != nil {
		t.Fatalf("UpdateThreadStatus failed: %v", err)
	}

	// 4. Add Comment To Thread
	comment, err := svc.AddCommentToThread(ctx, "pat-token", "myorg", "myproj", "myrepo", 42, 100, "Fixed in latest commit.")
	if err != nil {
		t.Fatalf("AddCommentToThread failed: %v", err)
	}
	if comment.ID != 201 || comment.Author != "Drixy AI" {
		t.Errorf("unexpected comment: %+v", comment)
	}
}

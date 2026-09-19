// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package bitbucket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBitbucketServerActivitiesAndDiffs(t *testing.T) {
	mux := http.NewServeMux()

	// Activities
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo-1/pull-requests/42/activities", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"values": []ServerPRActivity{
				{
					ID:          101,
					CreatedDate: 1600000000000,
					Action:      "COMMENTED",
					Comment: &ServerComment{
						ID:      501,
						Version: 1,
						Text:    "Looks good to me!",
						State:   "OPEN",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	// Commits
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo-1/pull-requests/42/commits", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"values": []ServerCommit{
				{
					ID:        "sha-commit-1",
					DisplayID: "sha-1",
					Message:   "feat: add feature",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	// Diff
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo-1/pull-requests/42.diff", func(w http.ResponseWriter, r *http.Request) {
		diff := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n"
		_, _ = w.Write([]byte(diff))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewBitbucketServerActivitiesService(server.URL, "test-token", server.Client())
	ctx := context.Background()

	// 1. List Activities
	activities, err := client.ListPullRequestActivities(ctx, "PROJ", "repo-1", 42, 25, 0)
	if err != nil {
		t.Fatalf("ListPullRequestActivities failed: %v", err)
	}
	if len(activities) != 1 || activities[0].Action != "COMMENTED" || activities[0].Comment.Text != "Looks good to me!" {
		t.Fatalf("unexpected activities: %+v", activities)
	}

	// 2. List Commits
	commits, err := client.ListPullRequestCommits(ctx, "PROJ", "repo-1", 42, 25, 0)
	if err != nil {
		t.Fatalf("ListPullRequestCommits failed: %v", err)
	}
	if len(commits) != 1 || commits[0].DisplayID != "sha-1" {
		t.Fatalf("unexpected commits: %+v", commits)
	}

	// 3. Get Diff
	diff, err := client.GetPullRequestDiff(ctx, "PROJ", "repo-1", 42, 3)
	if err != nil {
		t.Fatalf("GetPullRequestDiff failed: %v", err)
	}
	if diff == "" || !json.Valid([]byte("{}")) {
		t.Fatalf("empty diff returned")
	}
}

func TestBitbucketServerMergeAndComments(t *testing.T) {
	mux := http.NewServeMux()

	// Merge Status
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo-1/pull-requests/42/merge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			status := ServerMergeStatus{
				CanMerge:   true,
				Conflicted: false,
				Outcome:    "CLEAN",
			}
			_ = json.NewEncoder(w).Encode(status)
			return
		}
		if r.Method == http.MethodPost {
			activity := ServerPRActivity{
				ID:     202,
				Action: "MERGED",
			}
			_ = json.NewEncoder(w).Encode(activity)
			return
		}
	})

	// Decline PR
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo-1/pull-requests/42/decline", func(w http.ResponseWriter, r *http.Request) {
		activity := ServerPRActivity{
			ID:     203,
			Action: "DECLINED",
		}
		_ = json.NewEncoder(w).Encode(activity)
	})

	// Reopen PR
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo-1/pull-requests/42/reopen", func(w http.ResponseWriter, r *http.Request) {
		activity := ServerPRActivity{
			ID:     204,
			Action: "REOPENED",
		}
		_ = json.NewEncoder(w).Encode(activity)
	})

	// Comments
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo-1/pull-requests/42/comments", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		comment := ServerComment{
			ID:      601,
			Version: 1,
			Text:    payload["text"].(string),
			State:   "OPEN",
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(comment)
	})

	// Resolve Comment
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo-1/pull-requests/42/comments/601", func(w http.ResponseWriter, r *http.Request) {
		comment := ServerComment{
			ID:      601,
			Version: 2,
			Text:    "Fixed!",
			State:   "RESOLVED",
		}
		_ = json.NewEncoder(w).Encode(comment)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewBitbucketServerActivitiesService(server.URL, "test-token", server.Client())
	ctx := context.Background()

	// 1. Get Merge Status
	status, err := client.GetMergeStatus(ctx, "PROJ", "repo-1", 42)
	if err != nil {
		t.Fatalf("GetMergeStatus failed: %v", err)
	}
	if !status.CanMerge || status.Conflicted {
		t.Fatalf("unexpected merge status: %+v", status)
	}

	// 2. Merge Pull Request
	mergeAct, err := client.MergePullRequest(ctx, "PROJ", "repo-1", 42, 1, "Merging PR")
	if err != nil {
		t.Fatalf("MergePullRequest failed: %v", err)
	}
	if mergeAct.Action != "MERGED" {
		t.Fatalf("unexpected merge activity: %+v", mergeAct)
	}

	// 3. Decline Pull Request
	declineAct, err := client.DeclinePullRequest(ctx, "PROJ", "repo-1", 42, 2)
	if err != nil {
		t.Fatalf("DeclinePullRequest failed: %v", err)
	}
	if declineAct.Action != "DECLINED" {
		t.Fatalf("unexpected decline activity: %+v", declineAct)
	}

	// 4. Reopen Pull Request
	reopenAct, err := client.ReopenPullRequest(ctx, "PROJ", "repo-1", 42, 3)
	if err != nil {
		t.Fatalf("ReopenPullRequest failed: %v", err)
	}
	if reopenAct.Action != "REOPENED" {
		t.Fatalf("unexpected reopen activity: %+v", reopenAct)
	}

	// 5. Add Comment
	newComment, err := client.AddPullRequestComment(ctx, "PROJ", "repo-1", 42, "Please update this variable", nil)
	if err != nil {
		t.Fatalf("AddPullRequestComment failed: %v", err)
	}
	if newComment.ID != 601 || newComment.Text != "Please update this variable" {
		t.Fatalf("unexpected comment: %+v", newComment)
	}

	// 6. Resolve Comment
	resolved, err := client.ResolveComment(ctx, "PROJ", "repo-1", 42, 601, 1)
	if err != nil {
		t.Fatalf("ResolveComment failed: %v", err)
	}
	if resolved.State != "RESOLVED" {
		t.Fatalf("unexpected resolved state: %+v", resolved)
	}
}

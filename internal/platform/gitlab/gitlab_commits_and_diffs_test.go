// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCommitsAndDiffsService(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/projects/123/merge_requests/42/changes":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"changes": [
					{"old_path": "a.go", "new_path": "a.go", "diff": "@@ -1,2 +1,3 @@\n-old\n+new\n+line2"},
					{"old_path": "b.go", "new_path": "b.go", "diff": "@@ -0,0 +1 @@\n+added"}
				]
			}`))
		case r.URL.Path == "/api/v4/projects/123/merge_requests" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{
					"id": 1001,
					"iid": 42,
					"title": "Feature 1",
					"state": "opened",
					"created_at": "2026-09-01T00:00:00Z",
					"updated_at": "2026-09-02T00:00:00Z",
					"draft": false,
					"web_url": "https://gitlab.com/test/repo/-/merge_requests/42",
					"author": {"username": "alice"},
					"reviewers": [{"username": "bob"}]
				}
			]`))
		case r.URL.Path == "/api/v4/projects/123/repository/compare":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"diffs": [
					{"old_path": "foo.go", "new_path": "foo.go", "new_file": false, "diff": "@@ -1 +1 @@\n-a\n+b"}
				]
			}`))
		case r.URL.Path == "/api/v4/projects/123/repository/commits":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": "c0ffee123456"}`))
		case r.URL.Path == "/api/v4/projects/123/merge_requests" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{
				"id": 1002,
				"iid": 43,
				"title": "Automated fixes",
				"state": "opened",
				"web_url": "https://gitlab.com/test/repo/-/merge_requests/43",
				"author": {"username": "drixy-bot"}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := NewAdapter(ts.URL, "test-token")
	svc := NewCommitsAndDiffsService(client)
	ctx := context.Background()

	// 1. CountChangesInMergeRequest
	changes, err := svc.CountChangesInMergeRequest(ctx, 123, 42)
	if err != nil {
		t.Fatalf("unexpected error counting changes: %v", err)
	}
	if changes.Files != 2 || changes.Additions != 3 || changes.Deletions != 1 || changes.Changes != 4 {
		t.Fatalf("unexpected change count: %+v", changes)
	}

	// 2. GetPullRequestsForRTTM
	rttm, err := svc.GetPullRequestsForRTTM(ctx, 123, "opened", 10)
	if err != nil {
		t.Fatalf("unexpected error getting RTTM MRs: %v", err)
	}
	if len(rttm) != 1 || rttm[0].IID != 42 || rttm[0].Author != "alice" || len(rttm[0].Reviewers) != 1 {
		t.Fatalf("unexpected RTTM results: %+v", rttm)
	}

	// 3. GetChangedFilesSinceLastCommit
	diffs, err := svc.GetChangedFilesSinceLastCommit(ctx, 123, "sha-from", "sha-to")
	if err != nil {
		t.Fatalf("unexpected error comparing commits: %v", err)
	}
	if len(diffs) != 1 || diffs[0].NewPath != "foo.go" {
		t.Fatalf("unexpected diffs: %+v", diffs)
	}

	// 4. UploadFilesCommit
	commitSHA, err := svc.UploadFilesCommit(ctx, 123, MultiFileCommitRequest{
		Branch:        "feature-branch",
		CommitMessage: "test commit",
		Actions: []CommitFileAction{
			{Action: "create", FilePath: "test.txt", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error uploading files commit: %v", err)
	}
	if commitSHA != "c0ffee123456" {
		t.Fatalf("unexpected commit SHA: %s", commitSHA)
	}

	// 5. CreateMergeRequestWithFiles
	mr, err := svc.CreateMergeRequestWithFiles(ctx, 123, "main", "feature-patch", "Automated fixes", "Fixing issues", []CommitFileAction{
		{Action: "create", FilePath: "fix.txt", Content: "fixed"},
	})
	if err != nil {
		t.Fatalf("unexpected error creating MR with files: %v", err)
	}
	if mr.Number != 43 || mr.SourceBranch != "feature-patch" || mr.HeadSHA != "c0ffee123456" {
		t.Fatalf("unexpected MR result: %+v", mr)
	}
}

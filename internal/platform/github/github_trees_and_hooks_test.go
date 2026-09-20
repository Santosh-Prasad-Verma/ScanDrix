// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubTreesAndBlobs(t *testing.T) {
	mux := http.NewServeMux()

	// Tree Get & Create
	mux.HandleFunc("/repos/scandrix/backend/git/trees/main-sha", func(w http.ResponseWriter, r *http.Request) {
		tree := GitHubTree{
			SHA: "main-sha",
			Tree: []GitHubTreeEntry{
				{
					Path: "main.go",
					Mode: "100644",
					Type: "blob",
					SHA:  "blob-sha-1",
					Size: 120,
				},
			},
		}
		_ = json.NewEncoder(w).Encode(tree)
	})

	mux.HandleFunc("/repos/scandrix/backend/git/trees", func(w http.ResponseWriter, r *http.Request) {
		var req CreateGitHubTreeRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		tree := GitHubTree{
			SHA: "new-tree-sha",
			Tree: []GitHubTreeEntry{
				{
					Path: req.Tree[0].Path,
					Mode: req.Tree[0].Mode,
					Type: req.Tree[0].Type,
					SHA:  "new-blob-sha",
				},
			},
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(tree)
	})

	// Blob Get & Create
	mux.HandleFunc("/repos/scandrix/backend/git/blobs/blob-sha-1", func(w http.ResponseWriter, r *http.Request) {
		blob := GitHubBlob{
			SHA:      "blob-sha-1",
			Size:     14,
			Content:  "cGFja2FnZSBtYWlu",
			Encoding: "base64",
		}
		_ = json.NewEncoder(w).Encode(blob)
	})

	mux.HandleFunc("/repos/scandrix/backend/git/blobs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"sha": "created-blob-sha"})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewGitHubTreesAndHooksService(server.URL, "test-token", server.Client())
	ctx := context.Background()

	// 1. Get Tree
	tree, err := client.GetTree(ctx, "scandrix", "backend", "main-sha", true)
	if err != nil {
		t.Fatalf("GetTree failed: %v", err)
	}
	if tree.SHA != "main-sha" || len(tree.Tree) != 1 || tree.Tree[0].Path != "main.go" {
		t.Fatalf("unexpected tree: %+v", tree)
	}

	// 2. Create Tree
	newTree, err := client.CreateTree(ctx, "scandrix", "backend", CreateGitHubTreeRequest{
		Tree: []CreateGitHubTreeEntry{
			{Path: "README.md", Mode: "100644", Type: "blob", Content: "# Welcome"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTree failed: %v", err)
	}
	if newTree.SHA != "new-tree-sha" || newTree.Tree[0].Path != "README.md" {
		t.Fatalf("unexpected created tree: %+v", newTree)
	}

	// 3. Get Blob
	blob, err := client.GetBlob(ctx, "scandrix", "backend", "blob-sha-1")
	if err != nil {
		t.Fatalf("GetBlob failed: %v", err)
	}
	if blob.SHA != "blob-sha-1" || blob.Encoding != "base64" {
		t.Fatalf("unexpected blob: %+v", blob)
	}

	// 4. Create Blob
	sha, err := client.CreateBlob(ctx, "scandrix", "backend", CreateGitHubBlobRequest{
		Content:  "package main\n\nfunc main() {}",
		Encoding: "utf-8",
	})
	if err != nil {
		t.Fatalf("CreateBlob failed: %v", err)
	}
	if sha != "created-blob-sha" {
		t.Fatalf("unexpected blob sha: %s", sha)
	}
}

func TestGitHubHooksService(t *testing.T) {
	mux := http.NewServeMux()

	// Hooks List & Create
	mux.HandleFunc("/repos/scandrix/backend/hooks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			hooks := []GitHubHook{
				{
					ID:     1,
					Name:   "web",
					Active: true,
					Events: []string{"push", "pull_request"},
					Config: GitHubHookConfig{
						URL:         "https://scandrix.dev/api/webhooks",
						ContentType: "json",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(hooks)
			return
		}
		if r.Method == http.MethodPost {
			var req CreateGitHubHookRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			hook := GitHubHook{
				ID:     2,
				Name:   req.Name,
				Active: req.Active,
				Events: req.Events,
				Config: req.Config,
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(hook)
			return
		}
	})

	mux.HandleFunc("/repos/scandrix/backend/hooks/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			hook := GitHubHook{
				ID:     1,
				Name:   "web",
				Active: true,
				Events: []string{"push", "pull_request"},
			}
			_ = json.NewEncoder(w).Encode(hook)
			return
		}
		if r.Method == http.MethodPatch {
			var req UpdateGitHubHookRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			active := true
			if req.Active != nil {
				active = *req.Active
			}
			hook := GitHubHook{
				ID:     1,
				Name:   "web",
				Active: active,
				Events: req.Events,
			}
			_ = json.NewEncoder(w).Encode(hook)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	})

	mux.HandleFunc("/repos/scandrix/backend/hooks/1/pings", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewGitHubTreesAndHooksService(server.URL, "test-token", server.Client())
	ctx := context.Background()

	// 1. List Hooks
	hooks, err := client.ListHooks(ctx, "scandrix", "backend")
	if err != nil {
		t.Fatalf("ListHooks failed: %v", err)
	}
	if len(hooks) != 1 || hooks[0].ID != 1 {
		t.Fatalf("unexpected hooks: %+v", hooks)
	}

	// 2. Get Hook
	h, err := client.GetHook(ctx, "scandrix", "backend", 1)
	if err != nil {
		t.Fatalf("GetHook failed: %v", err)
	}
	if h.Name != "web" || !h.Active {
		t.Fatalf("unexpected hook: %+v", h)
	}

	// 3. Create Hook
	created, err := client.CreateHook(ctx, "scandrix", "backend", CreateGitHubHookRequest{
		Name:   "web",
		Active: true,
		Events: []string{"pull_request"},
		Config: GitHubHookConfig{
			URL: "https://scandrix.dev/webhook",
		},
	})
	if err != nil {
		t.Fatalf("CreateHook failed: %v", err)
	}
	if created.ID != 2 || len(created.Events) != 1 {
		t.Fatalf("unexpected created hook: %+v", created)
	}

	// 4. Update Hook
	active := false
	updated, err := client.UpdateHook(ctx, "scandrix", "backend", 1, UpdateGitHubHookRequest{
		Active: &active,
		Events: []string{"push"},
	})
	if err != nil {
		t.Fatalf("UpdateHook failed: %v", err)
	}
	if updated.Active {
		t.Fatalf("expected inactive hook: %+v", updated)
	}

	// 5. Ping Hook
	if err := client.PingHook(ctx, "scandrix", "backend", 1); err != nil {
		t.Fatalf("PingHook failed: %v", err)
	}

	// 6. Delete Hook
	if err := client.DeleteHook(ctx, "scandrix", "backend", 1); err != nil {
		t.Fatalf("DeleteHook failed: %v", err)
	}
}

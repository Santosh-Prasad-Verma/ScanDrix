package azuredevops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAzureDevOpsRepositoriesAndForks(t *testing.T) {
	mux := http.NewServeMux()

	// List & Create Repositories
	mux.HandleFunc("/my-org/my-proj/_apis/git/repositories", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			resp := map[string]any{
				"value": []AzureDevOpsRepo{
					{
						ID:            "repo-1",
						Name:          "core-service",
						DefaultBranch: "refs/heads/main",
						Size:          1024,
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		if r.Method == http.MethodPost {
			var req CreateAzureRepoRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			isFork := req.ParentRepository != nil
			repo := AzureDevOpsRepo{
				ID:               "repo-2",
				Name:             req.Name,
				IsFork:           isFork,
				ParentRepository: req.ParentRepository,
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(repo)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// Get Repository
	mux.HandleFunc("/my-org/my-proj/_apis/git/repositories/repo-1", func(w http.ResponseWriter, r *http.Request) {
		repo := AzureDevOpsRepo{
			ID:            "repo-1",
			Name:          "core-service",
			DefaultBranch: "refs/heads/main",
		}
		_ = json.NewEncoder(w).Encode(repo)
	})

	// List Forks
	mux.HandleFunc("/my-org/my-proj/_apis/git/repositories/repo-1/forks", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"value": []AzureDevOpsRepoRef{
				{
					ID:   "repo-2",
					Name: "core-service-fork",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewAzureDevOpsRepositoriesAndRefsService(server.URL, "pat-token", server.Client())
	ctx := context.Background()

	// 1. List Repositories
	repos, err := client.ListRepositories(ctx, "my-org", "my-proj", false, true)
	if err != nil {
		t.Fatalf("ListRepositories failed: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "core-service" {
		t.Fatalf("unexpected repos: %+v", repos)
	}

	// 2. Get Repository
	repo, err := client.GetRepository(ctx, "my-org", "my-proj", "repo-1")
	if err != nil {
		t.Fatalf("GetRepository failed: %v", err)
	}
	if repo.ID != "repo-1" {
		t.Fatalf("unexpected repo: %+v", repo)
	}

	// 3. Create Repository
	newRepo, err := client.CreateRepository(ctx, "my-org", "my-proj", CreateAzureRepoRequest{
		Name: "new-repo",
	})
	if err != nil {
		t.Fatalf("CreateRepository failed: %v", err)
	}
	if newRepo.Name != "new-repo" {
		t.Fatalf("unexpected new repo: %+v", newRepo)
	}

	// 4. Fork Repository
	forkedRepo, err := client.ForkRepository(ctx, "my-org", "my-proj", "repo-1", "core-service-fork")
	if err != nil {
		t.Fatalf("ForkRepository failed: %v", err)
	}
	if !forkedRepo.IsFork || forkedRepo.ParentRepository.ID != "repo-1" {
		t.Fatalf("unexpected fork: %+v", forkedRepo)
	}

	// 5. List Forks
	forks, err := client.ListForks(ctx, "my-org", "my-proj", "repo-1")
	if err != nil {
		t.Fatalf("ListForks failed: %v", err)
	}
	if len(forks) != 1 || forks[0].ID != "repo-2" {
		t.Fatalf("unexpected forks: %+v", forks)
	}
}

func TestAzureDevOpsGitRefsAndLocks(t *testing.T) {
	mux := http.NewServeMux()

	// List & Update Refs
	mux.HandleFunc("/my-org/my-proj/_apis/git/repositories/repo-1/refs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			resp := map[string]any{
				"value": []AzureGitRefDetails{
					{
						Name:     "refs/heads/main",
						ObjectID: "sha-1111",
						IsLocked: false,
					},
					{
						Name:     "refs/heads/feature-1",
						ObjectID: "sha-2222",
						IsLocked: true,
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		if r.Method == http.MethodPost {
			var updates []AzureGitRefUpdate
			_ = json.NewDecoder(r.Body).Decode(&updates)
			var results []AzureGitRefUpdateResult
			for _, u := range updates {
				results = append(results, AzureGitRefUpdateResult{
					Name:         u.Name,
					OldObjectID:  u.OldObjectID,
					NewObjectID:  u.NewObjectID,
					Success:      true,
					UpdateStatus: "succeeded",
				})
			}
			resp := map[string]any{"value": results}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		if r.Method == http.MethodPatch {
			filter := r.URL.Query().Get("filter")
			var payload map[string]bool
			_ = json.NewDecoder(r.Body).Decode(&payload)
			ref := AzureGitRefDetails{
				Name:     filter,
				ObjectID: "sha-1111",
				IsLocked: payload["isLocked"],
			}
			_ = json.NewEncoder(w).Encode(ref)
			return
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewAzureDevOpsRepositoriesAndRefsService(server.URL, "pat-token", server.Client())
	ctx := context.Background()

	// 1. List Git Refs
	refs, err := client.ListGitRefs(ctx, "my-org", "my-proj", "repo-1", "heads/")
	if err != nil {
		t.Fatalf("ListGitRefs failed: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}

	// 2. Get Git Ref
	ref, err := client.GetGitRef(ctx, "my-org", "my-proj", "repo-1", "refs/heads/main")
	if err != nil {
		t.Fatalf("GetGitRef failed: %v", err)
	}
	if ref.ObjectID != "sha-1111" {
		t.Fatalf("unexpected object ID: %s", ref.ObjectID)
	}

	// 3. Update Git Refs
	results, err := client.UpdateGitRefs(ctx, "my-org", "my-proj", "repo-1", []AzureGitRefUpdate{
		{
			Name:        "refs/heads/main",
			OldObjectID: "sha-1111",
			NewObjectID: "sha-3333",
		},
	})
	if err != nil {
		t.Fatalf("UpdateGitRefs failed: %v", err)
	}
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("unexpected update results: %+v", results)
	}

	// 4. Lock Ref
	lockedRef, err := client.LockGitRef(ctx, "my-org", "my-proj", "repo-1", "heads/main")
	if err != nil {
		t.Fatalf("LockGitRef failed: %v", err)
	}
	if !lockedRef.IsLocked {
		t.Fatalf("expected ref to be locked")
	}

	// 5. Unlock Ref
	unlockedRef, err := client.UnlockGitRef(ctx, "my-org", "my-proj", "repo-1", "heads/main")
	if err != nil {
		t.Fatalf("UnlockGitRef failed: %v", err)
	}
	if unlockedRef.IsLocked {
		t.Fatalf("expected ref to be unlocked")
	}
}

func TestAzureDevOpsGitItemsBlobsTreesAndPushes(t *testing.T) {
	mux := http.NewServeMux()

	// Items
	mux.HandleFunc("/my-org/my-proj/_apis/git/repositories/repo-1/items", func(w http.ResponseWriter, r *http.Request) {
		item := AzureGitItem{
			ObjectID:      "blob-123",
			GitObjectType: "blob",
			Path:          "/src/main.go",
			Content:       "package main\nfunc main() {}\n",
		}
		_ = json.NewEncoder(w).Encode(item)
	})

	// Blobs
	mux.HandleFunc("/my-org/my-proj/_apis/git/repositories/repo-1/blobs/blob-123", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("raw file content here"))
	})

	// Trees
	mux.HandleFunc("/my-org/my-proj/_apis/git/repositories/repo-1/trees/tree-456", func(w http.ResponseWriter, r *http.Request) {
		tree := AzureGitTree{
			ObjectID: "tree-456",
			TreeEntries: []AzureGitTreeEntry{
				{
					Mode:          "100644",
					GitObjectType: "blob",
					ObjectID:      "blob-123",
					RelativePath:  "src/main.go",
					Size:          32,
				},
			},
		}
		_ = json.NewEncoder(w).Encode(tree)
	})

	// Pushes
	mux.HandleFunc("/my-org/my-proj/_apis/git/repositories/repo-1/pushes", func(w http.ResponseWriter, r *http.Request) {
		push := AzureGitPush{
			PushID: 888,
			Commits: []AzureGitCommitInfo{
				{
					CommitID: "commit-999",
					Comment:  "Add main.go",
				},
			},
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(push)
	})

	// Commits
	mux.HandleFunc("/my-org/my-proj/_apis/git/repositories/repo-1/commits/commit-999", func(w http.ResponseWriter, r *http.Request) {
		commit := AzureGitCommitInfo{
			CommitID: "commit-999",
			Comment:  "Add main.go",
		}
		_ = json.NewEncoder(w).Encode(commit)
	})

	mux.HandleFunc("/my-org/my-proj/_apis/git/repositories/repo-1/commits", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"value": []AzureGitCommitInfo{
				{
					CommitID: "commit-999",
					Comment:  "Add main.go",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewAzureDevOpsRepositoriesAndRefsService(server.URL, "pat-token", server.Client())
	ctx := context.Background()

	// 1. Get Git Item
	item, err := client.GetGitItem(ctx, "my-org", "my-proj", "repo-1", "/src/main.go", "main", "branch")
	if err != nil {
		t.Fatalf("GetGitItem failed: %v", err)
	}
	if item.Path != "/src/main.go" || item.Content == "" {
		t.Fatalf("unexpected item: %+v", item)
	}

	// 2. Get Git Blob
	blob, err := client.GetGitBlob(ctx, "my-org", "my-proj", "repo-1", "blob-123")
	if err != nil {
		t.Fatalf("GetGitBlob failed: %v", err)
	}
	if blob.Content != "raw file content here" {
		t.Fatalf("unexpected blob content: %s", blob.Content)
	}

	// 3. Get Git Tree
	tree, err := client.GetGitTree(ctx, "my-org", "my-proj", "repo-1", "tree-456", true)
	if err != nil {
		t.Fatalf("GetGitTree failed: %v", err)
	}
	if len(tree.TreeEntries) != 1 || tree.TreeEntries[0].RelativePath != "src/main.go" {
		t.Fatalf("unexpected tree: %+v", tree)
	}

	// 4. Create Git Push
	push, err := client.CreateGitPush(ctx, "my-org", "my-proj", "repo-1", CreateAzureGitPushRequest{
		RefUpdates: []AzureGitRefUpdate{
			{
				Name:        "refs/heads/main",
				OldObjectID: "sha-0000",
				NewObjectID: "sha-1111",
			},
		},
		Commits: []CreateAzureGitCommit{
			{
				Comment: "Add main.go",
				Changes: []AzureGitChange{
					{
						ChangeType: "add",
						Item:       &AzureGitItemDescriptor{Path: "/src/main.go"},
						NewContent: &AzureGitItemContent{Content: "package main", ContentType: "rawtext"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateGitPush failed: %v", err)
	}
	if push.PushID != 888 || len(push.Commits) != 1 {
		t.Fatalf("unexpected push: %+v", push)
	}

	// 5. Get Git Commit
	commit, err := client.GetGitCommit(ctx, "my-org", "my-proj", "repo-1", "commit-999")
	if err != nil {
		t.Fatalf("GetGitCommit failed: %v", err)
	}
	if commit.CommitID != "commit-999" {
		t.Fatalf("unexpected commit: %+v", commit)
	}

	// 6. List Git Commits
	commits, err := client.ListGitCommits(ctx, "my-org", "my-proj", "repo-1", 10, 0)
	if err != nil {
		t.Fatalf("ListGitCommits failed: %v", err)
	}
	if len(commits) != 1 || commits[0].CommitID != "commit-999" {
		t.Fatalf("unexpected commits: %+v", commits)
	}
}

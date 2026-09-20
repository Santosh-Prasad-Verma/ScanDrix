// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package forgejo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestForgejoMilestonesAndLabels(t *testing.T) {
	mux := http.NewServeMux()

	// Milestones
	mux.HandleFunc("/api/v1/repos/owner/repo/milestones", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			milestones := []ForgejoMilestone{
				{
					ID:    101,
					Title: "v1.0.0",
					State: "open",
				},
			}
			_ = json.NewEncoder(w).Encode(milestones)
			return
		}
		if r.Method == http.MethodPost {
			var req CreateForgejoMilestoneRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			m := ForgejoMilestone{
				ID:    102,
				Title: req.Title,
				State: "open",
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(m)
			return
		}
	})

	mux.HandleFunc("/api/v1/repos/owner/repo/milestones/101", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			m := ForgejoMilestone{ID: 101, Title: "v1.0.0", State: "open"}
			_ = json.NewEncoder(w).Encode(m)
			return
		}
		if r.Method == http.MethodPatch {
			var req UpdateForgejoMilestoneRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			m := ForgejoMilestone{ID: 101, Title: req.Title, State: "closed"}
			_ = json.NewEncoder(w).Encode(m)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	})

	// Labels
	mux.HandleFunc("/api/v1/repos/owner/repo/labels", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			labels := []ForgejoLabel{
				{ID: 1, Name: "bug", Color: "e11d21"},
			}
			_ = json.NewEncoder(w).Encode(labels)
			return
		}
		if r.Method == http.MethodPost {
			var req CreateForgejoLabelRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			l := ForgejoLabel{ID: 2, Name: req.Name, Color: req.Color}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(l)
			return
		}
	})

	// Issue Labels
	mux.HandleFunc("/api/v1/repos/owner/repo/issues/42/labels", func(w http.ResponseWriter, r *http.Request) {
		labels := []ForgejoLabel{
			{ID: 1, Name: "bug", Color: "e11d21"},
		}
		_ = json.NewEncoder(w).Encode(labels)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewForgejoMilestonesAndLabelsService(server.URL, "secret-token", server.Client())
	ctx := context.Background()

	// 1. List Milestones
	milestones, err := client.ListMilestones(ctx, "owner", "repo", "open")
	if err != nil {
		t.Fatalf("ListMilestones failed: %v", err)
	}
	if len(milestones) != 1 || milestones[0].ID != 101 {
		t.Fatalf("unexpected milestones: %+v", milestones)
	}

	// 2. Get Milestone
	m, err := client.GetMilestone(ctx, "owner", "repo", 101)
	if err != nil {
		t.Fatalf("GetMilestone failed: %v", err)
	}
	if m.Title != "v1.0.0" {
		t.Fatalf("unexpected milestone title: %s", m.Title)
	}

	// 3. Create Milestone
	createdM, err := client.CreateMilestone(ctx, "owner", "repo", CreateForgejoMilestoneRequest{
		Title: "v2.0.0",
	})
	if err != nil {
		t.Fatalf("CreateMilestone failed: %v", err)
	}
	if createdM.ID != 102 || createdM.Title != "v2.0.0" {
		t.Fatalf("unexpected created milestone: %+v", createdM)
	}

	// 4. Update Milestone
	updatedM, err := client.UpdateMilestone(ctx, "owner", "repo", 101, UpdateForgejoMilestoneRequest{
		Title: "v1.0.0-final",
		State: "closed",
	})
	if err != nil {
		t.Fatalf("UpdateMilestone failed: %v", err)
	}
	if updatedM.State != "closed" {
		t.Fatalf("unexpected updated milestone: %+v", updatedM)
	}

	// 5. Delete Milestone
	if err := client.DeleteMilestone(ctx, "owner", "repo", 101); err != nil {
		t.Fatalf("DeleteMilestone failed: %v", err)
	}

	// 6. List Labels
	labels, err := client.ListLabels(ctx, "owner", "repo")
	if err != nil {
		t.Fatalf("ListLabels failed: %v", err)
	}
	if len(labels) != 1 || labels[0].Name != "bug" {
		t.Fatalf("unexpected labels: %+v", labels)
	}

	// 7. Create Label
	newLabel, err := client.CreateLabel(ctx, "owner", "repo", CreateForgejoLabelRequest{
		Name:  "enhancement",
		Color: "84b6eb",
	})
	if err != nil {
		t.Fatalf("CreateLabel failed: %v", err)
	}
	if newLabel.ID != 2 || newLabel.Name != "enhancement" {
		t.Fatalf("unexpected label: %+v", newLabel)
	}

	// 8. Add Labels to Issue/PR
	applied, err := client.AddLabelsToIssueOrPR(ctx, "owner", "repo", 42, []int64{1})
	if err != nil {
		t.Fatalf("AddLabelsToIssueOrPR failed: %v", err)
	}
	if len(applied) != 1 || applied[0].ID != 1 {
		t.Fatalf("unexpected applied labels: %+v", applied)
	}
}

func TestForgejoBranchesAndReleases(t *testing.T) {
	mux := http.NewServeMux()

	// Branches
	mux.HandleFunc("/api/v1/repos/owner/repo/branches", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			branches := []ForgejoBranch{
				{Name: "main", Protected: true},
			}
			_ = json.NewEncoder(w).Encode(branches)
			return
		}
		if r.Method == http.MethodPost {
			var req CreateForgejoBranchRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			b := ForgejoBranch{Name: req.NewBranchName, Protected: false}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(b)
			return
		}
	})

	mux.HandleFunc("/api/v1/repos/owner/repo/branches/feature-x", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	})

	// Releases
	mux.HandleFunc("/api/v1/repos/owner/repo/releases", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			releases := []ForgejoRelease{
				{ID: 10, TagName: "v1.0.0", Name: "Initial Release"},
			}
			_ = json.NewEncoder(w).Encode(releases)
			return
		}
		if r.Method == http.MethodPost {
			var req CreateForgejoReleaseRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			rel := ForgejoRelease{
				ID:      11,
				TagName: req.TagName,
				Name:    req.Name,
				Body:    req.Body,
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(rel)
			return
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewForgejoMilestonesAndLabelsService(server.URL, "secret-token", server.Client())
	ctx := context.Background()

	// 1. List Branches
	branches, err := client.ListBranches(ctx, "owner", "repo")
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}
	if len(branches) != 1 || branches[0].Name != "main" {
		t.Fatalf("unexpected branches: %+v", branches)
	}

	// 2. Create Branch
	newBranch, err := client.CreateBranch(ctx, "owner", "repo", CreateForgejoBranchRequest{
		NewBranchName: "feature-x",
		OldRefName:    "main",
	})
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	if newBranch.Name != "feature-x" {
		t.Fatalf("unexpected branch: %+v", newBranch)
	}

	// 3. Delete Branch
	if err := client.DeleteBranch(ctx, "owner", "repo", "feature-x"); err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	// 4. List Releases
	releases, err := client.ListReleases(ctx, "owner", "repo")
	if err != nil {
		t.Fatalf("ListReleases failed: %v", err)
	}
	if len(releases) != 1 || releases[0].TagName != "v1.0.0" {
		t.Fatalf("unexpected releases: %+v", releases)
	}

	// 5. Create Release
	newRel, err := client.CreateRelease(ctx, "owner", "repo", CreateForgejoReleaseRequest{
		TagName: "v2.0.0",
		Name:    "Version 2",
		Body:    "Major release",
	})
	if err != nil {
		t.Fatalf("CreateRelease failed: %v", err)
	}
	if newRel.ID != 11 || newRel.TagName != "v2.0.0" {
		t.Fatalf("unexpected release: %+v", newRel)
	}
}

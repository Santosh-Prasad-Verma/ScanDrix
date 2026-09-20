// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitLabEpicsAndIterations(t *testing.T) {
	mux := http.NewServeMux()

	// Group Epics
	mux.HandleFunc("/api/v4/groups/my-group/epics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			epics := []GitLabEpic{
				{
					ID:      1,
					IID:     10,
					GroupID: 55,
					Title:   "Security Modernization",
					State:   "opened",
				},
			}
			_ = json.NewEncoder(w).Encode(epics)
			return
		}
		if r.Method == http.MethodPost {
			var req CreateGitLabEpicRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			epic := GitLabEpic{
				ID:      2,
				IID:     11,
				GroupID: 55,
				Title:   req.Title,
				State:   "opened",
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(epic)
			return
		}
	})

	mux.HandleFunc("/api/v4/groups/my-group/epics/10", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			epic := GitLabEpic{
				ID:      1,
				IID:     10,
				GroupID: 55,
				Title:   "Security Modernization",
				State:   "opened",
			}
			_ = json.NewEncoder(w).Encode(epic)
			return
		}
		if r.Method == http.MethodPut {
			var req UpdateGitLabEpicRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			epic := GitLabEpic{
				ID:      1,
				IID:     10,
				GroupID: 55,
				Title:   req.Title,
				State:   "closed",
			}
			_ = json.NewEncoder(w).Encode(epic)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	})

	// Group Iterations
	mux.HandleFunc("/api/v4/groups/my-group/iterations", func(w http.ResponseWriter, r *http.Request) {
		iterations := []GitLabIteration{
			{
				ID:        100,
				IID:       1,
				Sequence:  1,
				Title:     "Sprint 1",
				State:     2,
				StartDate: "2026-09-01",
				DueDate:   "2026-09-14",
			},
		}
		_ = json.NewEncoder(w).Encode(iterations)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewGitLabEpicsAndIterationsService(server.URL, "secret-pat", server.Client())
	ctx := context.Background()

	// 1. List Epics
	epics, err := client.ListGroupEpics(ctx, "my-group", "opened")
	if err != nil {
		t.Fatalf("ListGroupEpics failed: %v", err)
	}
	if len(epics) != 1 || epics[0].IID != 10 {
		t.Fatalf("unexpected epics: %+v", epics)
	}

	// 2. Get Epic
	epic, err := client.GetGroupEpic(ctx, "my-group", 10)
	if err != nil {
		t.Fatalf("GetGroupEpic failed: %v", err)
	}
	if epic.Title != "Security Modernization" {
		t.Fatalf("unexpected epic: %+v", epic)
	}

	// 3. Create Epic
	created, err := client.CreateGroupEpic(ctx, "my-group", CreateGitLabEpicRequest{
		Title: "Platform Migration",
	})
	if err != nil {
		t.Fatalf("CreateGroupEpic failed: %v", err)
	}
	if created.IID != 11 || created.Title != "Platform Migration" {
		t.Fatalf("unexpected created epic: %+v", created)
	}

	// 4. Update Epic
	updated, err := client.UpdateGroupEpic(ctx, "my-group", 10, UpdateGitLabEpicRequest{
		Title:      "Security Modernization Completed",
		StateEvent: "close",
	})
	if err != nil {
		t.Fatalf("UpdateGroupEpic failed: %v", err)
	}
	if updated.State != "closed" {
		t.Fatalf("unexpected updated epic: %+v", updated)
	}

	// 5. Delete Epic
	if err := client.DeleteGroupEpic(ctx, "my-group", 10); err != nil {
		t.Fatalf("DeleteGroupEpic failed: %v", err)
	}

	// 6. List Iterations
	iterations, err := client.ListGroupIterations(ctx, "my-group", "current")
	if err != nil {
		t.Fatalf("ListGroupIterations failed: %v", err)
	}
	if len(iterations) != 1 || iterations[0].Title != "Sprint 1" {
		t.Fatalf("unexpected iterations: %+v", iterations)
	}
}

func TestGitLabMilestonesAndCommitComments(t *testing.T) {
	mux := http.NewServeMux()

	// Project Milestones
	mux.HandleFunc("/api/v4/projects/my-proj/milestones", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			milestones := []GitLabMilestone{
				{
					ID:        201,
					IID:       1,
					ProjectID: 99,
					Title:     "Q3 Release",
					State:     "active",
				},
			}
			_ = json.NewEncoder(w).Encode(milestones)
			return
		}
		if r.Method == http.MethodPost {
			var req CreateGitLabMilestoneRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			m := GitLabMilestone{
				ID:        202,
				IID:       2,
				ProjectID: 99,
				Title:     req.Title,
				State:     "active",
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(m)
			return
		}
	})

	// Commit Comments
	mux.HandleFunc("/api/v4/projects/my-proj/repository/commits/sha123/comments", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		comment := GitLabCommitComment{
			Note:     payload["note"].(string),
			Path:     payload["path"].(string),
			Line:     int(payload["line"].(float64)),
			LineType: payload["line_type"].(string),
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(comment)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewGitLabEpicsAndIterationsService(server.URL, "secret-pat", server.Client())
	ctx := context.Background()

	// 1. List Milestones
	milestones, err := client.ListProjectMilestones(ctx, "my-proj", "active")
	if err != nil {
		t.Fatalf("ListProjectMilestones failed: %v", err)
	}
	if len(milestones) != 1 || milestones[0].Title != "Q3 Release" {
		t.Fatalf("unexpected milestones: %+v", milestones)
	}

	// 2. Create Milestone
	newM, err := client.CreateProjectMilestone(ctx, "my-proj", CreateGitLabMilestoneRequest{
		Title: "Q4 Release",
	})
	if err != nil {
		t.Fatalf("CreateProjectMilestone failed: %v", err)
	}
	if newM.ID != 202 || newM.Title != "Q4 Release" {
		t.Fatalf("unexpected created milestone: %+v", newM)
	}

	// 3. Create Commit Comment
	comment, err := client.CreateCommitComment(ctx, "my-proj", "sha123", "Nice refactor!", "main.go", 10, "new")
	if err != nil {
		t.Fatalf("CreateCommitComment failed: %v", err)
	}
	if comment.Note != "Nice refactor!" || comment.Line != 10 {
		t.Fatalf("unexpected commit comment: %+v", comment)
	}
}

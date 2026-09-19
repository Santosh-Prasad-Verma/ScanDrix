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

func TestGitHubReleasesService_Releases(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/repos/scandrix/backend/releases", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.Method == http.MethodGet {
			releases := []Release{
				{
					ID:              1,
					TagName:         "v1.0.0",
					TargetCommitish: "main",
					Name:            "v1.0.0 Initial Release",
					HTMLURL:         "https://github.com/scandrix/backend/releases/v1.0.0",
					CreatedAt:       time.Now(),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(releases)
			return
		}

		if r.Method == http.MethodPost {
			var req CreateReleaseRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			rel := Release{
				ID:              2,
				TagName:         req.TagName,
				TargetCommitish: req.TargetCommitish,
				Name:            req.Name,
				Body:            req.Body,
				HTMLURL:         "https://github.com/scandrix/backend/releases/" + req.TagName,
				CreatedAt:       time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rel)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/repos/scandrix/backend/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		rel := Release{
			ID:      1,
			TagName: "v1.0.0",
			Name:    "v1.0.0 Initial Release",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rel)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	svc := NewGitHubReleasesService(server.Client(), server.URL)
	ctx := context.Background()

	// ListReleases
	list, err := svc.ListReleases(ctx, "test-token", "scandrix", "backend", 10, 1)
	if err != nil {
		t.Fatalf("ListReleases failed: %v", err)
	}
	if len(list) != 1 || list[0].TagName != "v1.0.0" {
		t.Errorf("Unexpected releases list: %+v", list)
	}

	// GetLatestRelease
	latest, err := svc.GetLatestRelease(ctx, "test-token", "scandrix", "backend")
	if err != nil {
		t.Fatalf("GetLatestRelease failed: %v", err)
	}
	if latest.TagName != "v1.0.0" {
		t.Errorf("Unexpected latest release: %+v", latest)
	}

	// CreateRelease
	created, err := svc.CreateRelease(ctx, "test-token", "scandrix", "backend", CreateReleaseRequest{
		TagName: "v1.1.0",
		Name:    "v1.1.0 Feature Release",
		Body:    "Added advanced code review",
	})
	if err != nil {
		t.Fatalf("CreateRelease failed: %v", err)
	}
	if created.ID != 2 || created.TagName != "v1.1.0" {
		t.Errorf("Unexpected created release: %+v", created)
	}
}

func TestGitHubReleasesService_TopicsAndCollaborators(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/repos/scandrix/backend/topics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string][]string{
				"names": {"code-review", "ai-assistant", "golang"},
			})
			return
		}

		if r.Method == http.MethodPut {
			var body map[string][]string
			json.NewDecoder(r.Body).Decode(&body)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(body)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/repos/scandrix/backend/collaborators", func(w http.ResponseWriter, r *http.Request) {
		users := []CollaboratorUser{
			{
				ID:       10,
				Login:    "octocat",
				RoleName: "admin",
				Permissions: map[string]bool{
					"admin": true,
					"push":  true,
					"pull":  true,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	svc := NewGitHubReleasesService(server.Client(), server.URL)
	ctx := context.Background()

	// Get topics
	topics, err := svc.GetRepositoryTopics(ctx, "token", "scandrix", "backend")
	if err != nil {
		t.Fatalf("GetRepositoryTopics failed: %v", err)
	}
	if len(topics) != 3 || topics[0] != "code-review" {
		t.Errorf("Unexpected topics: %+v", topics)
	}

	// Replace topics
	updated, err := svc.ReplaceRepositoryTopics(ctx, "token", "scandrix", "backend", []string{"code-review", "ai"})
	if err != nil {
		t.Fatalf("ReplaceRepositoryTopics failed: %v", err)
	}
	if len(updated) != 2 || updated[1] != "ai" {
		t.Errorf("Unexpected updated topics: %+v", updated)
	}

	// List collaborators
	collabs, err := svc.ListCollaborators(ctx, "token", "scandrix", "backend", "all")
	if err != nil {
		t.Fatalf("ListCollaborators failed: %v", err)
	}
	if len(collabs) != 1 || collabs[0].Login != "octocat" || !collabs[0].Permissions["admin"] {
		t.Errorf("Unexpected collaborators: %+v", collabs)
	}
}

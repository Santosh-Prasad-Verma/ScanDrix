package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

// CodeManagementController manages tracked repositories, branches, and code structures.
type CodeManagementController struct {
	repo *database.Repository
}

// NewCodeManagementController initializes the repository management controller with database persistence.
func NewCodeManagementController(repo *database.Repository) *CodeManagementController {
	return &CodeManagementController{repo: repo}
}

// Routes mounts repository management routes.
func (c *CodeManagementController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", c.handleListRepositories)
	r.Post("/track", c.handleTrackRepository)
	r.Get("/{id}/branches", c.handleListBranches)
	r.Get("/{id}/tree", c.handleGetFileTree)

	return r
}

func (c *CodeManagementController) handleListRepositories(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var repos []models.TrackedRepository
	if c.repo != nil {
		var err error
		repos, err = c.repo.ListTrackedRepositories(r.Context(), wsID)
		if err != nil {
			http.Error(w, `{"error":"failed listing repositories"}`, http.StatusInternalServerError)
			return
		}
	}

	res := make([]dtos.RepositoryResponse, 0, len(repos))
	for _, repo := range repos {
		res = append(res, dtos.RepositoryResponse{
			ID:            repo.ID,
			WorkspaceID:   repo.WorkspaceID,
			Provider:      repo.Provider,
			ExternalID:    repo.ExternalID,
			NamespacePath: repo.NamespacePath,
			DefaultBranch: repo.DefaultBranch,
			IsActive:      repo.IsActive,
			CreatedAt:     repo.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (c *CodeManagementController) handleTrackRepository(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req dtos.TrackRepositoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NamespacePath == "" {
		http.Error(w, `{"error":"namespace_path is required"}`, http.StatusBadRequest)
		return
	}

	if req.DefaultBranch == "" {
		req.DefaultBranch = "main"
	}
	if req.Provider == "" {
		req.Provider = models.ProviderGitHub
	}
	if req.ExternalID == "" {
		req.ExternalID = req.NamespacePath
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	repo, err := c.repo.TrackRepository(r.Context(), wsID, req.Provider, req.ExternalID, req.NamespacePath, req.DefaultBranch)
	if err != nil {
		http.Error(w, `{"error":"failed tracking repository"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.RepositoryResponse{
		ID:            repo.ID,
		WorkspaceID:   wsID,
		Provider:      repo.Provider,
		ExternalID:    repo.ExternalID,
		NamespacePath: repo.NamespacePath,
		DefaultBranch: repo.DefaultBranch,
		IsActive:      repo.IsActive,
		CreatedAt:     repo.CreatedAt,
	})
}

func (c *CodeManagementController) handleListBranches(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	repoIDStr := chi.URLParam(r, "id")
	repoID, err := uuid.Parse(repoIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid repository id"}`, http.StatusBadRequest)
		return
	}

	defaultBranch := "main"
	branches := []string{"main"}
	if c.repo != nil {
		if repos, err := c.repo.ListTrackedRepositories(r.Context(), wsID); err == nil {
			for _, tr := range repos {
				if tr.ID == repoID {
					if tr.DefaultBranch != "" {
						defaultBranch = tr.DefaultBranch
						branches = []string{tr.DefaultBranch}
					}
					break
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.BranchListResponse{
		DefaultBranch: defaultBranch,
		Branches:      branches,
	})
}

func (c *CodeManagementController) handleGetFileTree(w http.ResponseWriter, r *http.Request) {
	_, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	repoIDStr := chi.URLParam(r, "id")
	repoID, err := uuid.Parse(repoIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid repository id"}`, http.StatusBadRequest)
		return
	}

	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = "HEAD"
	}

	entries := []dtos.FileTreeEntry{}
	if c.repo != nil {
		if nodes, err := c.repo.GetASTNodesByRepository(r.Context(), repoID); err == nil && len(nodes) > 0 {
			seenPaths := make(map[string]bool)
			for _, node := range nodes {
				if node.FilePath != "" && !seenPaths[node.FilePath] {
					seenPaths[node.FilePath] = true
					entries = append(entries, dtos.FileTreeEntry{
						Path: node.FilePath,
						Type: "blob",
						Size: 0,
					})
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.FileTreeResponse{
		CommitSHA: ref,
		Entries:   entries,
	})
}

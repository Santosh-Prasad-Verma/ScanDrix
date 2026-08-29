package controllers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

// CodeManagementController manages tracked repositories, branches, and code structures.
type CodeManagementController struct{}

// NewCodeManagementController initializes the repository management controller.
func NewCodeManagementController() *CodeManagementController {
	return &CodeManagementController{}
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]dtos.RepositoryResponse{
		{
			ID:            uuid.MustParse("00000000-0000-0000-0003-000000000001"),
			WorkspaceID:   wsID,
			Provider:      models.ProviderGitHub,
			ExternalID:    "999001",
			NamespacePath: "acme/payment-service",
			DefaultBranch: "main",
			IsActive:      true,
			CreatedAt:     time.Now().AddDate(0, -1, 0),
		},
		{
			ID:            uuid.MustParse("00000000-0000-0000-0003-000000000002"),
			WorkspaceID:   wsID,
			Provider:      models.ProviderGitHub,
			ExternalID:    "999002",
			NamespacePath: "acme/auth-service",
			DefaultBranch: "main",
			IsActive:      true,
			CreatedAt:     time.Now().AddDate(0, -2, 0),
		},
	})
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

	repoID := uuid.New()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.RepositoryResponse{
		ID:            repoID,
		WorkspaceID:   wsID,
		Provider:      req.Provider,
		ExternalID:    req.ExternalID,
		NamespacePath: req.NamespacePath,
		DefaultBranch: req.DefaultBranch,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	})
}

func (c *CodeManagementController) handleListBranches(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.BranchListResponse{
		DefaultBranch: "main",
		Branches:      []string{"main", "staging", "develop", "feat/oauth2-pkce", "fix/db-pool-leak"},
	})
}

func (c *CodeManagementController) handleGetFileTree(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.FileTreeResponse{
		CommitSHA: "abcdef1234567890",
		Entries: []dtos.FileTreeEntry{
			{Path: "cmd/api/main.go", Type: "blob", Size: 4096},
			{Path: "internal/auth", Type: "tree"},
			{Path: "internal/auth/auth.go", Type: "blob", Size: 3200},
			{Path: "internal/rules/catalog.go", Type: "blob", Size: 5120},
			{Path: "go.mod", Type: "blob", Size: 840},
			{Path: "Dockerfile", Type: "blob", Size: 650},
		},
	})
}

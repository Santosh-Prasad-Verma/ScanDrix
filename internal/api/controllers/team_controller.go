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

// TeamController manages developer teams and team member assignments.
type TeamController struct{}

// NewTeamController initializes the team controller.
func NewTeamController() *TeamController {
	return &TeamController{}
}

// Routes mounts team endpoints.
func (c *TeamController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/", c.handleCreateTeam)
	r.Get("/", c.handleListTeams)
	r.Get("/{id}/members", c.handleListTeamMembers)
	r.Post("/{id}/members", c.handleAddTeamMember)
	r.Delete("/{id}/members/{userId}", c.handleRemoveTeamMember)

	return r
}

func (c *TeamController) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req dtos.CreateTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"team name is required"}`, http.StatusBadRequest)
		return
	}

	teamID := uuid.New()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.TeamResponse{
		ID:          teamID,
		WorkspaceID: wsID,
		Name:        req.Name,
		Description: req.Description,
		MemberCount: 1,
		CreatedAt:   time.Now().UTC(),
	})
}

func (c *TeamController) handleListTeams(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]dtos.TeamResponse{
		{
			ID:          uuid.MustParse("00000000-0000-0000-0002-000000000001"),
			WorkspaceID: wsID,
			Name:        "Core Platform & Security",
			Description: "Maintains identity, reviews, and infrastructure",
			MemberCount: 8,
			CreatedAt:   time.Now().AddDate(0, -2, 0),
		},
		{
			ID:          uuid.MustParse("00000000-0000-0000-0002-000000000002"),
			WorkspaceID: wsID,
			Name:        "Product Engineering",
			Description: "Builds user-facing web and mobile applications",
			MemberCount: 14,
			CreatedAt:   time.Now().AddDate(0, -1, 0),
		},
	})
}

func (c *TeamController) handleListTeamMembers(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	teamID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid team id"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]dtos.TeamMemberResponse{
		{
			ID:          uuid.New(),
			TeamID:      teamID,
			UserID:      uuid.New(),
			Email:       "alice@company.com",
			DisplayName: "Alice Smith",
			Role:        models.RoleAdmin,
			JoinedAt:    time.Now().AddDate(0, -1, 0),
		},
		{
			ID:          uuid.New(),
			TeamID:      teamID,
			UserID:      uuid.New(),
			Email:       "bob@company.com",
			DisplayName: "Bob Jones",
			Role:        models.RoleMember,
			JoinedAt:    time.Now().AddDate(0, 0, -14),
		},
	})
}

func (c *TeamController) handleAddTeamMember(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	teamID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid team id"}`, http.StatusBadRequest)
		return
	}

	var req dtos.AddTeamMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		http.Error(w, `{"error":"email is required"}`, http.StatusBadRequest)
		return
	}

	if req.Role == "" {
		req.Role = models.RoleMember
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.TeamMemberResponse{
		ID:          uuid.New(),
		TeamID:      teamID,
		UserID:      uuid.New(),
		Email:       req.Email,
		DisplayName: req.Email,
		Role:        req.Role,
		JoinedAt:    time.Now().UTC(),
	})
}

func (c *TeamController) handleRemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

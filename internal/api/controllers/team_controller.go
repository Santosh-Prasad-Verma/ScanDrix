package controllers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

// TeamController manages developer teams and team member assignments.
type TeamController struct {
	repo *database.Repository
}

// NewTeamController initializes the team controller with database persistence.
func NewTeamController(repo *database.Repository) *TeamController {
	return &TeamController{repo: repo}
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

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	team, err := c.repo.CreateTeam(r.Context(), wsID, req.Name, req.Description)
	if err != nil {
		http.Error(w, `{"error":"failed creating team"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.TeamResponse{
		ID:          team.ID,
		WorkspaceID: team.WorkspaceID,
		Name:        team.Name,
		Description: team.Description,
		MemberCount: 0,
		CreatedAt:   team.CreatedAt,
	})
}

func (c *TeamController) handleListTeams(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var teams []models.Team
	if c.repo != nil {
		var err error
		teams, err = c.repo.ListTeams(r.Context(), wsID)
		if err != nil {
			http.Error(w, `{"error":"failed listing teams"}`, http.StatusInternalServerError)
			return
		}
	}

	res := make([]dtos.TeamResponse, 0, len(teams))
	for _, t := range teams {
		members, _ := c.repo.ListTeamMembers(r.Context(), t.ID)
		res = append(res, dtos.TeamResponse{
			ID:          t.ID,
			WorkspaceID: t.WorkspaceID,
			Name:        t.Name,
			Description: t.Description,
			MemberCount: len(members),
			CreatedAt:   t.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (c *TeamController) handleListTeamMembers(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	teamID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid team id"}`, http.StatusBadRequest)
		return
	}

	members, err := c.repo.ListTeamMembers(r.Context(), teamID)
	if err != nil {
		http.Error(w, `{"error":"failed listing members"}`, http.StatusInternalServerError)
		return
	}

	res := make([]dtos.TeamMemberResponse, 0, len(members))
	for _, m := range members {
		res = append(res, dtos.TeamMemberResponse{
			ID:          m.ID,
			TeamID:      m.TeamID,
			UserID:      m.UserID,
			Email:       m.Email,
			DisplayName: m.Email,
			Role:        models.UserRole(m.Role),
			JoinedAt:    m.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
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

	var userID uuid.UUID
	if user, err := c.repo.GetUserByEmail(r.Context(), req.Email); err == nil && user != nil {
		userID = user.UUID
	} else {
		userID = uuid.New()
	}

	err = c.repo.AddTeamMember(r.Context(), teamID, userID, req.Email, string(req.Role))
	if err != nil {
		http.Error(w, `{"error":"failed adding member"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.TeamMemberResponse{
		ID:          uuid.New(),
		TeamID:      teamID,
		UserID:      userID,
		Email:       req.Email,
		DisplayName: req.Email,
		Role:        req.Role,
		JoinedAt:    time.Now().UTC(),
	})
}

func (c *TeamController) handleRemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	teamID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid team id"}`, http.StatusBadRequest)
		return
	}

	userIDStr := chi.URLParam(r, "userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid user id"}`, http.StatusBadRequest)
		return
	}

	if err := c.repo.RemoveTeamMember(r.Context(), teamID, userID); err != nil {
		http.Error(w, `{"error":"failed removing team member"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

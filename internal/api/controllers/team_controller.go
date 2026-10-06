package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/enterprise/license"
	teamusecases "github.com/scandrix/backend/internal/organization/application/usecases/team"
	memberusecases "github.com/scandrix/backend/internal/organization/application/usecases/teammembers"
	teamclikeydomain "github.com/scandrix/backend/internal/organization/domain/teamclikey"
	memberdomain "github.com/scandrix/backend/internal/organization/domain/teammembers"
	"github.com/scandrix/backend/pkg/models"
)

// TeamRepository defines the legacy data access contract for teams and team members.
type TeamRepository interface {
	CreateTeam(ctx context.Context, wsID uuid.UUID, name, description string) (*models.Team, error)
	ListTeams(ctx context.Context, wsID uuid.UUID) ([]models.Team, error)
	ListTeamMembers(ctx context.Context, wsID, teamID uuid.UUID) ([]models.TeamMember, error)
	GetUserByEmail(ctx context.Context, email string) (*database.UserRecord, error)
	AddTeamMember(ctx context.Context, wsID, teamID, userID uuid.UUID, email, role string) error
	RemoveTeamMember(ctx context.Context, wsID, teamID, userID uuid.UUID) error
	ListAPIKeys(ctx context.Context, workspaceID uuid.UUID) ([]*models.TeamCLIKey, error)
	SaveAPIKey(ctx context.Context, id, workspaceID uuid.UUID, name, keyHash, prefix string, expiresAt *time.Time) error
	RevokeAPIKey(ctx context.Context, workspaceID, keyID uuid.UUID) error
}

// TeamController manages developer teams, member assignments, and CLI access keys via Clean Architecture.
type TeamController struct {
	repo                        TeamRepository
	createTeamUC                *teamusecases.CreateTeamUseCase
	listTeamsUC                 *teamusecases.ListTeamsUseCase
	listTeamsWithIntegrationsUC *teamusecases.ListTeamsWithIntegrationsUseCase
	createMemberUC              *memberusecases.CreateOrUpdateTeamMembersUseCase
	deleteMemberUC              *memberusecases.DeleteTeamMemberUseCase
	getMembersUC                *memberusecases.GetTeamMembersUseCase
	cliKeyService               teamclikeydomain.ITeamCliKeyService
	entitlementResolver         *license.Resolver
}

// NewTeamController initializes the team controller with repository persistence.
func NewTeamController(repo TeamRepository) *TeamController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &TeamController{repo: repo}
}

// HandleListWorkspaceCLIKeys lists every active CLI key in the workspace.
//
// The dashboard needs a workspace-scoped view of CLI credentials, and the
// team-scoped route only covers one team. This reads the same table through the
// existing repository query; the key material itself is never returned, only the
// prefix and metadata, because the plaintext key is shown once at creation.
func (c *TeamController) HandleListWorkspaceCLIKeys(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	keys, err := c.repo.ListAPIKeys(r.Context(), wsID)
	if err != nil {
		slog.Error("Failed listing workspace CLI keys", "error", err)
		http.Error(w, `{"error":"failed listing CLI keys"}`, http.StatusInternalServerError)
		return
	}

	type cliKeyResponse struct {
		ID         string  `json:"id"`
		Name       string  `json:"name"`
		Prefix     string  `json:"prefix"`
		Active     bool    `json:"active"`
		TeamID     *string `json:"teamId,omitempty"`
		CreatedAt  string  `json:"createdAt"`
		LastUsedAt *string `json:"lastUsedAt,omitempty"`
		ExpiresAt  *string `json:"expiresAt,omitempty"`
	}

	out := make([]cliKeyResponse, 0, len(keys))
	for _, k := range keys {
		if k == nil {
			continue
		}
		item := cliKeyResponse{
			ID:        k.ID.String(),
			Name:      k.Name,
			Prefix:    k.KeyPrefix,
			Active:    k.Active,
			CreatedAt: k.CreatedAt.UTC().Format(time.RFC3339),
		}
		if k.TeamID != nil {
			teamID := k.TeamID.String()
			item.TeamID = &teamID
		}
		if k.LastUsedAt != nil {
			formatted := k.LastUsedAt.UTC().Format(time.RFC3339)
			item.LastUsedAt = &formatted
		}
		if k.ExpiresAt != nil {
			formatted := k.ExpiresAt.UTC().Format(time.RFC3339)
			item.ExpiresAt = &formatted
		}
		out = append(out, item)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": out, "total": len(out)})
}

// HandleRevokeWorkspaceCLIKey deactivates a CLI key in the workspace.
//
// The key row is kept and marked inactive rather than deleted, so the audit
// trail retains the record that a credential existed. Ownership is enforced by
// the workspace scope in the repository query, not by the caller-supplied id.
func (c *TeamController) HandleRevokeWorkspaceCLIKey(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	keyID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "keyId")))
	if err != nil {
		http.Error(w, `{"error":"invalid key id"}`, http.StatusBadRequest)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	if err := c.repo.RevokeAPIKey(r.Context(), wsID, keyID); err != nil {
		slog.Error("Failed revoking CLI key", "error", err)
		http.Error(w, `{"error":"failed revoking CLI key"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": keyID.String(), "active": false})
}

// WithEntitlements attaches the entitlement resolver used to enforce seat
// quotas when members are invited.
func (c *TeamController) WithEntitlements(r *license.Resolver) *TeamController {
	c.entitlementResolver = r
	return c
}

// WithUseCases injects the Clean Architecture domain use cases and services.
func (c *TeamController) WithUseCases(
	createTeam *teamusecases.CreateTeamUseCase,
	listTeams *teamusecases.ListTeamsUseCase,
	listIntegrations *teamusecases.ListTeamsWithIntegrationsUseCase,
	createMember *memberusecases.CreateOrUpdateTeamMembersUseCase,
	deleteMember *memberusecases.DeleteTeamMemberUseCase,
	getMembers *memberusecases.GetTeamMembersUseCase,
	cliKeyService teamclikeydomain.ITeamCliKeyService,
) *TeamController {
	c.createTeamUC = createTeam
	c.listTeamsUC = listTeams
	c.listTeamsWithIntegrationsUC = listIntegrations
	c.createMemberUC = createMember
	c.deleteMemberUC = deleteMember
	c.getMembersUC = getMembers
	c.cliKeyService = cliKeyService
	return c
}

// Routes mounts team endpoints.
func (c *TeamController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/", c.handleCreateTeam)
	r.Get("/", c.handleListTeams)
	r.Get("/list-with-integrations", c.handleListWithIntegrations)
	r.Get("/{id}/members", c.handleListTeamMembers)
	r.Post("/{id}/members", c.handleAddTeamMember)
	r.Delete("/{id}/members/{userId}", auth.RoleGuard(models.RoleAdmin, c.handleRemoveTeamMember))

	// Team CLI keys management (/teams/:id/cli-keys)
	r.Get("/{id}/cli-keys", c.handleListTeamCLIKeys)
	r.Post("/{id}/cli-keys", auth.RoleGuard(models.RoleAdmin, c.handleCreateTeamCLIKey))
	r.Patch("/{id}/cli-keys/{keyId}/config", auth.RoleGuard(models.RoleAdmin, c.handleUpdateTeamCLIKeyConfig))
	r.Patch("/{id}/cli-keys/{keyId}", auth.RoleGuard(models.RoleAdmin, c.handleUpdateTeamCLIKeyConfig))
	r.Delete("/{id}/cli-keys/{keyId}", auth.RoleGuard(models.RoleAdmin, c.handleRevokeTeamCLIKey))

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

	if c.createTeamUC != nil {
		team, err := c.createTeamUC.Execute(r.Context(), wsID, req.Name, req.Description, "least_busy")
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(dtos.TeamResponse{
			ID:          team.UUID,
			WorkspaceID: team.WorkspaceID,
			Name:        team.Name,
			Description: team.Description,
			MemberCount: 0,
			CreatedAt:   team.CreatedAt,
		})
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

	if c.listTeamsUC != nil {
		profile, ok := auth.AccountProfileFromContext(r.Context())
		var userID *uuid.UUID
		var userRole string
		if ok && profile != nil {
			userID = &profile.ID
			userRole = string(profile.Role)
		}
		teams, err := c.listTeamsUC.ExecuteWithRole(r.Context(), wsID, userID, userRole)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		res := make([]dtos.TeamResponse, 0, len(teams))
		for _, t := range teams {
			memberCount := 0
			if c.getMembersUC != nil {
				mems, _ := c.getMembersUC.Execute(r.Context(), wsID, t.UUID, false)
				memberCount = len(mems)
			}
			res = append(res, dtos.TeamResponse{
				ID:          t.UUID,
				WorkspaceID: t.WorkspaceID,
				Name:        t.Name,
				Description: t.Description,
				MemberCount: memberCount,
				CreatedAt:   t.CreatedAt,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
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
		members, _ := c.repo.ListTeamMembers(r.Context(), wsID, t.ID)
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

func (c *TeamController) handleListWithIntegrations(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	if c.listTeamsWithIntegrationsUC != nil {
		teams, err := c.listTeamsWithIntegrationsUC.Execute(r.Context(), wsID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(teams)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]any{})
}

func (c *TeamController) handleListTeamMembers(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())
	idStr := chi.URLParam(r, "id")
	teamID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid team id"}`, http.StatusBadRequest)
		return
	}

	if c.getMembersUC != nil && wsID != uuid.Nil {
		members, err := c.getMembersUC.Execute(r.Context(), wsID, teamID, false)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		res := make([]dtos.TeamMemberResponse, 0, len(members))
		for _, m := range members {
			res = append(res, dtos.TeamMemberResponse{
				ID:          m.UUID,
				TeamID:      m.TeamID,
				UserID:      m.UserID,
				Email:       m.Email,
				DisplayName: m.Email,
				Role:        models.UserRole(m.Role),
				JoinedAt:    m.JoinedAt,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	members, err := c.repo.ListTeamMembers(r.Context(), wsID, teamID)
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
	wsID, _ := auth.WorkspaceFromContext(r.Context())
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

	inviterEmail := ""
	if p, ok := auth.AccountProfileFromContext(r.Context()); ok && p != nil {
		inviterEmail = p.Email
	}

	// Seat quota is enforced on the backend before any member is written, so a
	// workspace at its limit cannot grow past it (Master Rule 4.3).
	if c.entitlementResolver != nil && wsID != uuid.Nil {
		if err := c.entitlementResolver.CheckSeats(r.Context(), wsID, 1); err != nil {
			writeSeatError(w, err)
			return
		}
	}

	if c.createMemberUC != nil && wsID != uuid.Nil {
		role := memberdomain.RoleMember
		if req.Role == models.RoleAdmin || req.Role == models.RoleOwner {
			role = memberdomain.RoleAdmin
		}
		results, err := c.createMemberUC.Execute(r.Context(), wsID, teamID, []memberdomain.MemberItem{
			{Email: req.Email, TeamRole: role},
		}, inviterEmail)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		if results != nil && len(results.Results) > 0 {
			res := results.Results[0]
			newID := uuid.New()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(dtos.TeamMemberResponse{
				ID:          newID,
				TeamID:      teamID,
				UserID:      newID,
				Email:       res.Email,
				DisplayName: res.Email,
				Role:        req.Role,
				JoinedAt:    time.Now().UTC(),
			})
			return
		}
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	var userID uuid.UUID
	if user, err := c.repo.GetUserByEmail(r.Context(), req.Email); err == nil && user != nil {
		userID = user.UUID
	} else {
		userID = uuid.New()
	}

	err = c.repo.AddTeamMember(r.Context(), wsID, teamID, userID, req.Email, string(req.Role))
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
	wsID, _ := auth.WorkspaceFromContext(r.Context())
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

	if c.deleteMemberUC != nil && wsID != uuid.Nil {
		if err := c.deleteMemberUC.ExecuteByID(r.Context(), teamID, userID); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if c.repo == nil {
		// This answered 204 No Content without removing anything, so a caller
		// would believe a team member had been removed while the membership
		// remained active (AUDIT_REMEDIATION.md F-10).
		http.Error(w, `{"error":"cannot remove team member: no data source"}`, http.StatusServiceUnavailable)
		return
	}

	if err := c.repo.RemoveTeamMember(r.Context(), wsID, teamID, userID); err != nil {
		http.Error(w, `{"error":"failed removing team member"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TeamMembersRoutes mounts /team-members routes used by the web dashboard.
func (c *TeamController) TeamMembersRoutes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", c.handleInviteTeamMembers)
	r.Get("/", c.handleListTeamMembersGeneric)
	r.Delete("/{uuid}", c.handleDeleteTeamMemberGeneric)
	return r
}

func (c *TeamController) handleInviteTeamMembers(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())
	var req struct {
		TeamID  string `json:"teamId"`
		Members []struct {
			UUID              *uuid.UUID                                  `json:"uuid,omitempty"`
			Active            *bool                                       `json:"active,omitempty"`
			Email             string                                      `json:"email"`
			Name              string                                      `json:"name,omitempty"`
			Role              string                                      `json:"role,omitempty"`
			TeamRole          string                                      `json:"teamRole,omitempty"`
			Avatar            string                                      `json:"avatar,omitempty"`
			CommunicationID   string                                      `json:"communicationId,omitempty"`
			Communication     *memberdomain.CommunicationMemberConfig     `json:"communication,omitempty"`
			CodeManagement    *memberdomain.CodeManagementMemberConfig    `json:"codeManagement,omitempty"`
			ProjectManagement *memberdomain.ProjectManagementMemberConfig `json:"projectManagement,omitempty"`
			UserID            *uuid.UUID                                  `json:"userId,omitempty"`
		} `json:"members"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	teamID, err := uuid.Parse(req.TeamID)
	if err != nil {
		http.Error(w, `{"error":"invalid team ID"}`, http.StatusBadRequest)
		return
	}

	inviterEmail := ""
	if p, ok := auth.AccountProfileFromContext(r.Context()); ok && p != nil {
		inviterEmail = p.Email
	}

	// Seat quota is enforced on the backend before any member is written, so a
	// workspace at its limit cannot grow past it (Master Rule 4.3).
	if c.entitlementResolver != nil && wsID != uuid.Nil {
		if err := c.entitlementResolver.CheckSeats(r.Context(), wsID, len(req.Members)); err != nil {
			writeSeatError(w, err)
			return
		}
	}

	if c.createMemberUC != nil && wsID != uuid.Nil {
		memberItems := make([]memberdomain.MemberItem, 0, len(req.Members))
		for _, m := range req.Members {
			role := memberdomain.RoleMember
			if m.Role == "admin" || m.Role == "lead" || m.TeamRole == "admin" || m.TeamRole == "lead" {
				role = memberdomain.RoleAdmin
			}
			active := true
			if m.Active != nil {
				active = *m.Active
			}
			memberItems = append(memberItems, memberdomain.MemberItem{
				UUID:              m.UUID,
				Active:            active,
				CommunicationID:   m.CommunicationID,
				TeamRole:          role,
				Role:              m.Role,
				Avatar:            m.Avatar,
				Name:              m.Name,
				Communication:     m.Communication,
				CodeManagement:    m.CodeManagement,
				ProjectManagement: m.ProjectManagement,
				Email:             m.Email,
				UserID:            m.UserID,
			})
		}

		results, err := c.createMemberUC.Execute(r.Context(), wsID, teamID, memberItems, inviterEmail)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(results)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"results": []any{},
	})
}

func (c *TeamController) handleListTeamMembersGeneric(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())
	teamIDStr := r.URL.Query().Get("teamId")
	teamID := uuid.Nil
	if teamIDStr != "" {
		if tid, err := uuid.Parse(teamIDStr); err == nil {
			teamID = tid
		}
	}

	if c.getMembersUC != nil && wsID != uuid.Nil {
		formattedMembers, err := c.getMembersUC.ExecuteFormatted(r.Context(), wsID, teamID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"members": formattedMembers,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"members": []any{},
	})
}

func (c *TeamController) handleDeleteTeamMemberGeneric(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())
	uuidStr := chi.URLParam(r, "uuid")
	memberUUID, err := uuid.Parse(uuidStr)
	if err != nil {
		http.Error(w, `{"error":"invalid member uuid"}`, http.StatusBadRequest)
		return
	}

	removeAll := r.URL.Query().Get("removeAll") == "true"

	var actorUserID *uuid.UUID
	if p, ok := auth.AccountProfileFromContext(r.Context()); ok && p != nil && p.ID != uuid.Nil {
		actorUserID = &p.ID
	}

	if c.deleteMemberUC != nil && wsID != uuid.Nil {
		otherTeams, err := c.deleteMemberUC.Execute(r.Context(), wsID, memberUUID, actorUserID, removeAll)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		if len(otherTeams) > 0 {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": otherTeams,
			})
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *TeamController) handleListTeamCLIKeys(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	teamIDStr := chi.URLParam(r, "id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid team id"}`, http.StatusBadRequest)
		return
	}

	if c.cliKeyService != nil {
		allKeys, err := c.cliKeyService.ListByWorkspace(r.Context(), wsID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		res := make([]dtos.APIKeyResponse, 0, len(allKeys))
		for _, k := range allKeys {
			if k.TeamID != nil && *k.TeamID != teamID {
				continue
			}
			res = append(res, dtos.APIKeyResponse{
				ID:        k.UUID,
				Name:      k.Name,
				KeyPrefix: k.KeyPrefix,
				CreatedAt: k.CreatedAt,
				ExpiresAt: k.ExpiresAt,
				Config:    k.Config,
				Active:    k.Active,
				LastUsedAt: func() *time.Time {
					// Normalised to UTC so the client does not have to guess a zone.
					if k.LastUsedAt == nil {
						return nil
					}
					t := k.LastUsedAt.UTC()
					return &t
				}(),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database repository unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	keys, err := c.repo.ListAPIKeys(r.Context(), wsID)
	if err != nil {
		http.Error(w, `{"error":"failed listing CLI keys"}`, http.StatusInternalServerError)
		return
	}

	res := make([]dtos.APIKeyResponse, 0, len(keys))
	for _, k := range keys {
		res = append(res, dtos.APIKeyResponse{
			ID:        k.ID,
			Name:      k.Name,
			KeyPrefix: k.KeyPrefix,
			CreatedAt: k.CreatedAt,
			ExpiresAt: k.ExpiresAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (c *TeamController) handleCreateTeamCLIKey(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	teamIDStr := chi.URLParam(r, "id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid team id"}`, http.StatusBadRequest)
		return
	}

	var req dtos.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"key name is required"}`, http.StatusBadRequest)
		return
	}

	if c.cliKeyService != nil {
		rawKey, keyEntity, err := c.cliKeyService.GenerateKey(r.Context(), wsID, &teamID, req.Name, nil, req.ExpiresAt)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		// Active and Config are included so a freshly created key maps through
		// the same shape as a listed one. The dashboard used to synthesise
		// createdAt locally and assume a key prefix, which made a new key look
		// different from the same key after a refresh.
		_ = json.NewEncoder(w).Encode(dtos.APIKeyResponse{
			ID:        keyEntity.UUID,
			Name:      keyEntity.Name,
			KeyPrefix: keyEntity.KeyPrefix,
			PlainKey:  rawKey,
			CreatedAt: keyEntity.CreatedAt,
			ExpiresAt: keyEntity.ExpiresAt,
			Config:    keyEntity.Config,
			Active:    keyEntity.Active,
		})
		return
	}

	plainKey, hashedKey, err := auth.GenerateAPIKey()
	if err != nil {
		http.Error(w, `{"error":"failed generating key"}`, http.StatusInternalServerError)
		return
	}

	keyID := uuid.New()
	prefix := plainKey[:14]

	if c.repo != nil {
		_ = c.repo.SaveAPIKey(r.Context(), keyID, wsID, req.Name, hashedKey, prefix, req.ExpiresAt)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.APIKeyResponse{
		ID:        keyID,
		Name:      req.Name,
		KeyPrefix: prefix,
		PlainKey:  plainKey,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: req.ExpiresAt,
	})
}

func (c *TeamController) handleRevokeTeamCLIKey(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	keyIDStr := chi.URLParam(r, "keyId")
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid key id"}`, http.StatusBadRequest)
		return
	}

	if c.cliKeyService != nil {
		if err := c.cliKeyService.Revoke(r.Context(), wsID, keyID); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "key revoked successfully"})
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database repository unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	if err := c.repo.RevokeAPIKey(r.Context(), wsID, keyID); err != nil {
		http.Error(w, `{"error":"failed revoking key"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "key revoked successfully"})
}

func (c *TeamController) handleUpdateTeamCLIKeyConfig(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	keyIDStr := chi.URLParam(r, "keyId")
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid key id"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Config map[string]any `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if c.cliKeyService != nil {
		configBytes, _ := json.Marshal(req.Config)
		if err := c.cliKeyService.UpdateConfig(r.Context(), wsID, keyID, configBytes); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uuid":        keyID,
			"workspaceId": wsID,
			"active":      true,
			"config":      req.Config,
			"updatedAt":   time.Now().UTC(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"uuid":        keyID,
		"workspaceId": wsID,
		"active":      true,
		"config":      req.Config,
		"updatedAt":   time.Now().UTC(),
	})
}

// writeSeatError maps a seat-quota failure onto an HTTP response. A quota
// refusal is 409 Conflict because the request is well formed but conflicts with
// the workspace's current entitlement; an unreadable seat count is 503, because
// the check failed closed and retrying may succeed.
func writeSeatError(w http.ResponseWriter, err error) {
	var limitErr *license.SeatLimitError
	if errors.As(err, &limitErr) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":          limitErr.Error(),
			"code":           "seat_quota_exceeded",
			"seatsLimit":     limitErr.Limit,
			"seatsUsed":      limitErr.Used,
			"seatsRequested": limitErr.Requested,
		})
		return
	}

	if errors.Is(err, license.ErrSeatsUnavailable) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "seat quota could not be verified; no members were added",
			"code":  "seat_check_unavailable",
		})
		return
	}

	http.Error(w, `{"error":"failed processing member request"}`, http.StatusInternalServerError)
}

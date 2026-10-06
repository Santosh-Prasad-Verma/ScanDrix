package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/enterprise/rbac"
	"github.com/scandrix/backend/pkg/models"
)

// PermissionsController handles user capabilities and RBAC authorization introspection.
type PermissionsController struct {
	policyEngine   *rbac.PolicyEngine
	repoAccessCtrl *rbac.RepositoryAccessController
}

// NewPermissionsController initializes the permissions controller with optional database or custom assignment store.
func NewPermissionsController(store ...any) *PermissionsController {
	engine := rbac.NewPolicyEngine()
	var assignmentStore rbac.RepositoryAssignmentStore

	if len(store) > 0 && store[0] != nil {
		switch s := store[0].(type) {
		case rbac.RepositoryAssignmentStore:
			assignmentStore = s
		case rbac.DatabaseAssignmentStore:
			assignmentStore = rbac.NewPostgresRepositoryAssignmentStore(s)
		}
	}

	return &PermissionsController{
		policyEngine:   engine,
		repoAccessCtrl: rbac.NewRepositoryAccessController(assignmentStore, engine),
	}
}

// Routes mounts permission endpoints.
func (c *PermissionsController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", c.handleGetMyPermissions)
	r.Get("/me", c.handleGetMyPermissions)
	r.Get("/can-access", c.handleCanAccess)
	r.Get("/matrix", c.handleGetPermissionMatrix)
	r.Post("/check", c.handleCheckPermission)

	// Per-User Fine-Grained Repository Access Control (Enterprise RBAC)
	r.Post("/users/{userId}/repositories", auth.RoleGuard(models.RoleAdmin, c.handleAssignRepositories))
	r.Get("/users/{userId}/repositories", c.handleGetAssignedRepositories)
	r.Delete("/users/{userId}/repositories/{repoId}", auth.RoleGuard(models.RoleAdmin, c.handleRevokeRepository))
	r.Post("/assign-repos", auth.RoleGuard(models.RoleAdmin, c.handleAssignRepositoriesLegacy))
	r.Get("/assigned-repos", c.handleGetAssignedRepositoriesLegacy)

	return r
}

func (c *PermissionsController) handleGetMyPermissions(w http.ResponseWriter, r *http.Request) {
	role := models.RoleMember
	if profile, ok := auth.AccountProfileFromContext(r.Context()); ok && profile != nil && profile.Role != "" {
		role = profile.Role
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"role":        role,
		"permissions": c.policyEngine.GetRolePermissions(role),
	})
}

func (c *PermissionsController) handleGetPermissionMatrix(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string][]string{
		string(models.RoleOwner): {
			"workspace:*", "review:*", "rule:*", "integration:*", "audit:*", "billing:*",
		},
		string(models.RoleAdmin): {
			"workspace:read", "workspace:write", "review:*", "rule:*", "integration:*", "audit:read",
		},
		string(models.RoleMember): {
			"workspace:read", "review:create", "review:read", "review:approve",
		},
		string(models.RoleViewer): {
			"workspace:read", "review:read",
		},
	})
}

func (c *PermissionsController) handleCheckPermission(w http.ResponseWriter, r *http.Request) {
	var req dtos.PermissionCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Permission == "" {
		http.Error(w, `{"error":"permission is required"}`, http.StatusBadRequest)
		return
	}

	role := models.RoleMember
	if profile, ok := auth.AccountProfileFromContext(r.Context()); ok && profile != nil && profile.Role != "" {
		role = profile.Role
	}

	err := c.policyEngine.CheckPermission(role, rbac.Permission(req.Permission))
	allowed := err == nil
	reason := ""
	if !allowed {
		reason = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.PermissionCheckResponse{
		Allowed: allowed,
		Reason:  reason,
	})
}

func (c *PermissionsController) handleCanAccess(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")
	resource := r.URL.Query().Get("resource")

	role := models.RoleMember
	if profile, ok := auth.AccountProfileFromContext(r.Context()); ok && profile != nil && profile.Role != "" {
		role = profile.Role
	}

	allowed := false
	if action != "" && resource != "" {
		allowed = c.policyEngine.Can(role, rbac.Action(action), rbac.Resource(resource))
		if !allowed {
			// Check legacy permission formats
			if err := c.policyEngine.CheckPermission(role, rbac.Permission(resource+":"+action)); err == nil {
				allowed = true
			} else if err := c.policyEngine.CheckPermission(role, rbac.Permission(action+":"+resource)); err == nil {
				allowed = true
			}
		}
	} else if action == "" && resource == "" {
		// Default sanity check
		allowed = c.policyEngine.Can(role, rbac.ActionRead, rbac.ResourceWorkspace)
	}

	w.Header().Set("Content-Type", "application/json")

	// If a specific repository check is requested, evaluate fine-grained per-user assignments
	repoIDStr := r.URL.Query().Get("repository_id")
	if repoIDStr == "" {
		repoIDStr = r.URL.Query().Get("repo_id")
	}
	if allowed && repoIDStr != "" {
		if repoUUID, err := uuid.Parse(repoIDStr); err == nil {
			wsID, _ := auth.WorkspaceFromContext(r.Context())
			var userID uuid.UUID
			if profile, ok := auth.AccountProfileFromContext(r.Context()); ok && profile != nil {
				userID = profile.ID
			}
			allowed = c.repoAccessCtrl.CanAccessRepository(r.Context(), wsID, userID, role, repoUUID)
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"allowed":  allowed,
		"action":   action,
		"resource": resource,
		"role":     role,
	})
}

// handleAssignRepositories assigns specific repositories to a user.
func (c *PermissionsController) handleAssignRepositories(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"unauthenticated: missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	userIDStr := chi.URLParam(r, "userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid userId"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		RepoIDs []uuid.UUID `json:"repo_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	assignedBy := "admin"
	if profile, ok := auth.AccountProfileFromContext(r.Context()); ok && profile != nil {
		assignedBy = profile.Email
	}

	assignment, err := c.repoAccessCtrl.AssignRepositories(r.Context(), wsID, userID, req.RepoIDs, assignedBy)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(assignment)
}

// handleGetAssignedRepositories returns the repositories assigned to a user.
func (c *PermissionsController) handleGetAssignedRepositories(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"unauthenticated: missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	userIDStr := chi.URLParam(r, "userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid userId"}`, http.StatusBadRequest)
		return
	}

	repoIDs, err := c.repoAccessCtrl.GetAssignedRepositories(r.Context(), wsID, userID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"user_id":  userID,
		"repo_ids": repoIDs,
	})
}

// handleRevokeRepository revokes a specific repository assignment.
func (c *PermissionsController) handleRevokeRepository(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"unauthenticated: missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	userIDStr := chi.URLParam(r, "userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid userId"}`, http.StatusBadRequest)
		return
	}

	repoIDStr := chi.URLParam(r, "repoId")
	repoID, err := uuid.Parse(repoIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid repoId"}`, http.StatusBadRequest)
		return
	}

	assignment, err := c.repoAccessCtrl.RevokeRepository(r.Context(), wsID, userID, repoID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(assignment)
}

// handleAssignRepositoriesLegacy compatible with enterprise POST /permissions/assign-repos
func (c *PermissionsController) handleAssignRepositoriesLegacy(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())
	var req struct {
		UserID  string      `json:"userId"`
		RepoIDs []uuid.UUID `json:"repoIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		http.Error(w, `{"error":"userId and repoIds required"}`, http.StatusBadRequest)
		return
	}

	uid, err := uuid.Parse(req.UserID)
	if err != nil {
		http.Error(w, `{"error":"invalid userId"}`, http.StatusBadRequest)
		return
	}

	assignment, err := c.repoAccessCtrl.AssignRepositories(r.Context(), wsID, uid, req.RepoIDs, "admin")
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(assignment)
}

// handleGetAssignedRepositoriesLegacy compatible with enterprise GET /permissions/assigned-repos
func (c *PermissionsController) handleGetAssignedRepositoriesLegacy(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())
	userIDStr := r.URL.Query().Get("userId")
	if userIDStr == "" {
		if profile, ok := auth.AccountProfileFromContext(r.Context()); ok && profile != nil {
			userIDStr = profile.ID.String()
		}
	}

	uid, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, `{"error":"userId required"}`, http.StatusBadRequest)
		return
	}

	repoIDs, err := c.repoAccessCtrl.GetAssignedRepositories(r.Context(), wsID, uid)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"userId":  uid.String(),
		"repoIds": repoIDs,
	})
}

package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/enterprise/rbac"
	"github.com/scandrix/backend/pkg/models"
)

// PermissionsController handles user capabilities and RBAC authorization introspection.
type PermissionsController struct {
	policyEngine *rbac.PolicyEngine
}

// NewPermissionsController initializes the permissions controller.
func NewPermissionsController() *PermissionsController {
	return &PermissionsController{policyEngine: rbac.NewPolicyEngine()}
}

// Routes mounts permission endpoints.
func (c *PermissionsController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/me", c.handleGetMyPermissions)
	r.Get("/matrix", c.handleGetPermissionMatrix)
	r.Post("/check", c.handleCheckPermission)

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

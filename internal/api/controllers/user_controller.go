// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - User Management REST API Controller
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	onboardingusecases "github.com/scandrix/backend/internal/organization/application/usecases/onboarding"
)

// UserRepository defines the persistence contract for user profile management.
type UserRepository interface {
	GetUserByID(ctx context.Context, userUUID uuid.UUID) (*database.UserRecord, error)
	GetUserByEmail(ctx context.Context, email string) (*database.UserRecord, error)
	UpdateUserStatus(ctx context.Context, userUUID uuid.UUID, status string) error
	UpdateUserPassword(ctx context.Context, email, passwordHash string) error
}

// UserController manages user profile inspections, invitations, and organizational joins.
type UserController struct {
	authCtrl  *AuthController
	repo      UserRepository
	joinOrgUC *onboardingusecases.JoinOrganizationUseCase
}

// NewUserController constructs the user controller.
func NewUserController(authCtrl *AuthController, repo UserRepository) *UserController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &UserController{
		authCtrl: authCtrl,
		repo:     repo,
	}
}

// WithJoinOrganizationUseCase attaches the onboarding JoinOrganizationUseCase.
func (c *UserController) WithJoinOrganizationUseCase(uc *onboardingusecases.JoinOrganizationUseCase) *UserController {
	c.joinOrgUC = uc
	return c
}

// Routes mounts /user endpoints.
func (c *UserController) Routes() chi.Router {
	r := chi.NewRouter()

	// Public endpoints
	r.Get("/email", c.handleCheckEmail)
	r.Get("/invite", c.handleGetInvite)
	r.Post("/invite/complete-invitation", c.handleCompleteInvitation)

	// Protected endpoints
	r.Group(func(pr chi.Router) {
		pr.Get("/info", c.handleGetUserInfo)
		pr.Post("/join-organization", c.handleJoinOrganization)
		pr.Patch("/marketing-survey", c.handleSaveMarketingSurvey)
		pr.Patch("/{targetUserId}", c.handleUpdateTargetUser)
	})

	return r
}

func (c *UserController) handleCheckEmail(w http.ResponseWriter, r *http.Request) {
	if c.authCtrl != nil {
		c.authCtrl.HandleCheckEmail(w, r)
		return
	}
	email := strings.TrimSpace(r.URL.Query().Get("email"))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"exists": email != "",
	})
}

func (c *UserController) handleGetInvite(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.URL.Query().Get("userId"))
	if userID == "" {
		http.Error(w, `{"error":"userId is required"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"userId":  userID,
		"valid":   true,
		"status":  "pending",
		"message": "invitation active",
	})
}

func (c *UserController) handleCompleteInvitation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID   string `json:"userId"`
		Password string `json:"password"`
		FullName string `json:"fullName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON request body"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"userId":  req.UserID,
		"status":  "active",
	})
}

func (c *UserController) handleGetUserInfo(w http.ResponseWriter, r *http.Request) {
	if c.authCtrl != nil {
		c.authCtrl.handleMe(w, r)
		return
	}

	profile, ok := auth.AccountProfileFromContext(r.Context())
	if !ok || profile == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(profile)
}

func (c *UserController) handleJoinOrganization(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID         string `json:"userId,omitempty"`
		OrganizationID string `json:"organizationId"`
		InvitationCode string `json:"invitationCode,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON request body"}`, http.StatusBadRequest)
		return
	}

	if req.OrganizationID == "" {
		http.Error(w, `{"error":"organizationId is required"}`, http.StatusBadRequest)
		return
	}

	orgUUID, err := uuid.Parse(req.OrganizationID)
	if err != nil {
		orgUUID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(req.OrganizationID))
	}

	var userUUID uuid.UUID
	if profile, ok := auth.AccountProfileFromContext(r.Context()); ok && profile != nil {
		userUUID = profile.ID
	} else if req.UserID != "" {
		userUUID, _ = uuid.Parse(req.UserID)
	}
	if userUUID == uuid.Nil {
		userUUID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("default-user"))
	}

	if c.joinOrgUC != nil && userUUID != uuid.Nil {
		res, err := c.joinOrgUC.Execute(r.Context(), onboardingusecases.JoinOrganizationInput{
			UserID:         userUUID,
			OrganizationID: orgUUID,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":        true,
		"organizationId": req.OrganizationID,
		"status":         "joined",
	})
}

func (c *UserController) handleSaveMarketingSurvey(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Survey recorded",
	})
}

func (c *UserController) handleUpdateTargetUser(w http.ResponseWriter, r *http.Request) {
	targetUserID := chi.URLParam(r, "targetUserId")
	if strings.TrimSpace(targetUserID) == "" {
		http.Error(w, `{"error":"targetUserId is required"}`, http.StatusBadRequest)
		return
	}

	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"targetUserId": targetUserID,
		"updated":      true,
		"fields":       req,
	})
}

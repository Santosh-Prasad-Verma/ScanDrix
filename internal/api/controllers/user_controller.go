// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - User Management REST API Controller
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	onboardingusecases "github.com/scandrix/backend/internal/organization/application/usecases/onboarding"
	"github.com/scandrix/backend/pkg/models"
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

	// Public endpoints.
	//
	// GET /email must stay reachable without a session: the signup form checks
	// email availability before an account exists. It is an account-existence
	// oracle by construction and is accepted as that trade-off, mitigated by
	// the shared public rate limiter. If the signup UI is ever changed to rely
	// on the 409 that POST /auth/register already returns for a taken address,
	// this route should be deleted outright.
	//
	// These were previously rate limited by a duplicate registration in the
	// public block of router.go, which shadowed this controller entirely. The
	// limiter now lives here so it cannot drift away from the route again.
	//
	// GET /invite used to sit here too. It was moved to the authenticated
	// group because it discloses real account status (AUDIT_REMEDIATION.md
	// F-49), and it now reports only the caller's own account.
	r.Group(func(pub chi.Router) {
		pub.Use(c.publicRateLimitMiddleware())
		pub.Get("/email", c.handleCheckEmail)
	})

	// Everything below mutates or discloses account state and requires a valid
	// session. This group previously carried a "Protected endpoints" comment and
	// no middleware, which left POST /join-organization reachable by anonymous
	// callers (AUDIT_REMEDIATION.md F-02).
	r.Group(func(pr chi.Router) {
		pr.Use(c.authMiddleware())
		pr.Get("/info", c.handleGetUserInfo)
		pr.Get("/invite", c.handleGetInvite)
		pr.Post("/join-organization", c.handleJoinOrganization)
		pr.Patch("/marketing-survey", c.handleSaveMarketingSurvey)
		pr.Patch("/{targetUserId}", c.handleUpdateTargetUser)
	})

	return r
}

// publicRateLimitMiddleware returns the shared authentication rate limiter, or
// a pass-through when no AuthController is wired.
func (c *UserController) publicRateLimitMiddleware() func(http.Handler) http.Handler {
	if c.authCtrl == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return c.authCtrl.RateLimitMiddleware()
}

// authMiddleware returns the session-authentication middleware, or a
// fail-closed stub when no AuthController is wired.
func (c *UserController) authMiddleware() func(http.Handler) http.Handler {
	if c.authCtrl == nil {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"error":"authentication service unavailable"}`, http.StatusServiceUnavailable)
			})
		}
	}
	return c.authCtrl.AuthMiddleware()
}

// authenticatedProfile returns the caller's profile, or writes 401 and reports
// false. This is the single way handlers in this file obtain the acting user.
func authenticatedProfile(w http.ResponseWriter, r *http.Request) (*models.AccountProfile, bool) {
	profile, ok := auth.AccountProfileFromContext(r.Context())
	if !ok || profile == nil || profile.ID == uuid.Nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return nil, false
	}
	return profile, true
}

func (c *UserController) handleCheckEmail(w http.ResponseWriter, r *http.Request) {
	if c.authCtrl != nil {
		c.authCtrl.HandleCheckEmail(w, r)
		return
	}

	// No AuthController is wired. The previous fallback answered
	// {"exists": email != ""}, which claimed every address was taken. Answer
	// honestly instead: the check cannot be performed.
	http.Error(w, `{"error":"email availability check unavailable"}`, http.StatusServiceUnavailable)
}

// handleGetInvite reports the invitation state for the calling account.
//
// AUDIT_REMEDIATION.md F-49. This was mounted publicly and answered with the
// real stored status of whatever `userId` the caller supplied. The previous
// comment argued the caller "is expected to already hold the userId from the
// invite link", which is not an authorization control: any caller who learned
// or guessed a UUID could enumerate account status across tenants.
//
// It now ignores the query parameter entirely and reports only the
// authenticated caller's own account. That is a stronger guarantee than
// requiring a session, which would still have let one user probe another's
// status by passing a different userId.
func (c *UserController) handleGetInvite(w http.ResponseWriter, r *http.Request) {
	profile, ok := authenticatedProfile(w, r)
	if !ok {
		return
	}
	userUUID := profile.ID

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	record, err := c.repo.GetUserByID(r.Context(), userUUID)
	if err != nil || record == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"userId":  userUUID.String(),
			"valid":   false,
			"message": "no invitation found for this account",
		})
		return
	}

	// An account is invite-pending only while it is awaiting acceptance.
	invitePending := record.Status == "pending" ||
		record.Status == "awaiting_approval" ||
		record.Status == "pending_email"

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"userId":  userUUID.String(),
		"valid":   invitePending,
		"status":  record.Status,
		"message": invitationMessage(invitePending),
	})
}

func invitationMessage(pending bool) string {
	if pending {
		return "invitation active"
	}
	return "this account is not awaiting invitation acceptance"
}

// handleCompleteInvitation accepted an invitation.
//
// It performed no write: it echoed the submitted userId back with
// {"success":true,"status":"active"}, so a user completing an invite was told
// their account was activated when nothing had changed (AGENTS.md §2.2). The
// route has been removed rather than "fixed" because there is no invitation
// repository on this controller to implement it against.
//
// Invite acceptance is not a supported flow. Register via POST /auth/register,
// or authenticate and use an OAuth provider.

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
	// The acting user is the authenticated caller, full stop. It is never read
	// from the request body and never defaulted: both were unauthenticated
	// cross-tenant account takeover (AUDIT_REMEDIATION.md F-02).
	profile, ok := authenticatedProfile(w, r)
	if !ok {
		return
	}

	var req struct {
		OrganizationID string `json:"organizationId"`
		InvitationCode string `json:"invitationCode,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON request body"}`, http.StatusBadRequest)
		return
	}

	req.OrganizationID = strings.TrimSpace(req.OrganizationID)
	req.InvitationCode = strings.TrimSpace(req.InvitationCode)
	if req.OrganizationID == "" {
		http.Error(w, `{"error":"organizationId is required"}`, http.StatusBadRequest)
		return
	}

	// Reject a malformed identifier outright. The previous code silently derived
	// a deterministic UUID from the string, so a typo looked like a valid target
	// and the write landed somewhere unintended.
	orgUUID, err := uuid.Parse(req.OrganizationID)
	if err != nil {
		http.Error(w, `{"error":"organizationId must be a UUID"}`, http.StatusBadRequest)
		return
	}

	if c.joinOrgUC == nil {
		http.Error(w, `{"error":"organization join is unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	res, err := c.joinOrgUC.Execute(r.Context(), onboardingusecases.JoinOrganizationInput{
		UserID:         profile.ID,
		OrganizationID: orgUUID,
		InvitationCode: req.InvitationCode,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// handleSaveMarketingSurvey acknowledges the payload.
//
// This endpoint has no persistence layer. It previously answered
// {"success":true,"message":"Survey recorded"} without storing anything, which
// told the client its telemetry was captured when it was discarded
// (AUDIT_REMEDIATION.md, AGENTS.md §2.2). It now says so explicitly rather
// than reporting a write that never happened.
func (c *UserController) handleSaveMarketingSurvey(w http.ResponseWriter, r *http.Request) {
	profile, ok := authenticatedProfile(w, r)
	if !ok {
		return
	}

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON request body"}`, http.StatusBadRequest)
		return
	}

	slog.Info("marketing_survey.received",
		"event", "marketing_survey.received",
		"user_id", profile.ID,
		"field_count", len(req),
		"persisted", false,
	)

	w.Header().Set("Content-Type", "application/json")
	// 202: accepted and read, but deliberately not persisted. Distinguishable
	// from 200 so a client can tell the difference.
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "received_not_persisted",
		"persisted": false,
		"message":   "Survey payload accepted but not stored; no persistence layer is implemented",
	})
}

// handleUpdateTargetUser updates a user profile.
//
// Two defects are fixed here (AUDIT_REMEDIATION.md F-02):
//  1. there was no ownership check, so any authenticated caller could name any
//     target user id;
//  2. it answered {"updated":true} without performing any write.
//
// It still performs no write — there is no profile-update repository on this
// controller — so it reports 501 honestly instead of a false success. Callers
// must not treat a 2xx here as "the profile changed".
func (c *UserController) handleUpdateTargetUser(w http.ResponseWriter, r *http.Request) {
	profile, ok := authenticatedProfile(w, r)
	if !ok {
		return
	}

	rawTarget := strings.TrimSpace(chi.URLParam(r, "targetUserId"))
	if rawTarget == "" {
		http.Error(w, `{"error":"targetUserId is required"}`, http.StatusBadRequest)
		return
	}

	targetID, err := uuid.Parse(rawTarget)
	if err != nil {
		http.Error(w, `{"error":"targetUserId must be a UUID"}`, http.StatusBadRequest)
		return
	}

	// A caller may always update themselves. Updating anyone else requires an
	// administrative role; fail closed when the role cannot be determined.
	isSelf := targetID == profile.ID
	isAdmin := profile.Role == models.RoleOwner || profile.Role == models.RoleAdmin

	if !isSelf && !isAdmin {
		http.Error(w, `{"error":"forbidden: you may only update your own profile"}`, http.StatusForbidden)
		return
	}

	http.Error(w,
		`{"error":"profile updates are not implemented on this endpoint","persisted":false}`,
		http.StatusNotImplemented,
	)
}

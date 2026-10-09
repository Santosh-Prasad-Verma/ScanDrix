package scim

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/enterprise/license"
)

// SCIMService provides in-memory/database backed SCIM 2.0 provisioning.
type SCIMService struct {
	mu sync.RWMutex
	// users is an in-process cache. Groups are NOT cached: they are read from
	// and written to the database on every request, because group state that
	// only lives in memory disappears on restart and diverges between replicas.
	users       map[string]SCIMUser
	repo        *database.Repository
	bearerToken string
	// workspaceID scopes provisioned users and their seat consumption. It is
	// unset for a service that is not bound to a tenant, in which case seat
	// accounting is skipped rather than guessed.
	workspaceID uuid.UUID
	resolver    *license.Resolver
}

// NewSCIMService initializes the SCIM 2.0 provisioning engine with optional database persistence.
func NewSCIMService(repo ...*database.Repository) *SCIMService {
	var r *database.Repository
	if len(repo) > 0 {
		r = repo[0]
	}
	return &SCIMService{
		users: make(map[string]SCIMUser),
		repo:  r,
	}
}

// SetEntitlement binds the service to a tenant and its entitlement resolver.
//
// Without this the seat-quota check is skipped entirely, which is how a SCIM
// connection could provision past the licensed seat count: the check existed in
// handleCreateUser but nothing ever supplied the resolver it reads. Both
// binaries must call this, or SCIM provisions seats for free.
func (s *SCIMService) SetEntitlement(workspaceID uuid.UUID, resolver *license.Resolver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaceID = workspaceID
	s.resolver = resolver
}

// SetBearerToken configures the required Bearer token for SCIM operations.
func (s *SCIMService) SetBearerToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bearerToken = token
}

func (s *SCIMService) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			s.writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing Bearer authorization token")
			return
		}
		providedToken := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if providedToken == "" {
			s.writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing Bearer authorization token")
			return
		}

		s.mu.RLock()
		resolver, fallbackWS := s.resolver, s.workspaceID
		s.mu.RUnlock()

		if resolver == nil {
			s.writeError(w, http.StatusServiceUnavailable, "mutability",
				"SCIM provisioning is not bound to an organization")
			return
		}

		// The tenant is resolved from the credential that was presented, not
		// from server state. Previously a single workspace was bound at startup
		// -- chosen as the oldest row in the workspaces table -- so on a
		// multi-tenant deployment every customer's directory sync wrote into the
		// same workspace and drew on the same seat quota.
		wsID, err := s.resolveWorkspace(r.Context(), providedToken, fallbackWS)
		if err != nil {
			slog.Error("SCIM tenant resolution failed", "error", err)
			s.writeError(w, http.StatusServiceUnavailable, "mutability",
				"SCIM tenant could not be resolved; no change was made")
			return
		}
		if wsID == uuid.Nil {
			// A valid-looking token that no workspace owns is unauthorized, not
			// a server fault.
			s.writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing Bearer authorization token")
			return
		}

		// Entitlement check lives here, not in route middleware. SCIM
		// authenticates with a bearer token and has no session, so a
		// workspace-scoped feature gate could never pass and the whole
		// integration was unreachable. The resolved tenant is on the request,
		// so the question can be answered directly.
		if !resolver.Allows(r.Context(), wsID, license.FeatureSCIM) {
			slog.Warn("SCIM request refused: workspace is not entitled",
				"workspace_id", wsID.String())
			s.writeError(w, http.StatusForbidden, "mutability",
				"SCIM provisioning is not included in this organization's plan")
			return
		}

		next.ServeHTTP(w, r.WithContext(WithSCIMWorkspace(r.Context(), wsID)))
	})
}

// resolveWorkspace maps a presented token to the workspace that owns it.
//
// A per-workspace token is authoritative. The single-tenant fallback is only
// consulted when the deployment genuinely has one workspace and the token
// matches the server-wide secret, which keeps self-hosted installs working
// without reintroducing cross-tenant resolution.
func (s *SCIMService) resolveWorkspace(ctx context.Context, providedToken string, fallbackWS uuid.UUID) (uuid.UUID, error) {
	if s.repo != nil {
		wsID, err := s.repo.ResolveSCIMWorkspaceByToken(ctx, hashSCIMToken(providedToken))
		if err != nil {
			return uuid.Nil, err
		}
		if wsID != uuid.Nil {
			return wsID, nil
		}
	}

	// No per-workspace token matched. Fall back only for a single-tenant
	// install presenting the configured secret.
	s.mu.RLock()
	serverToken := s.bearerToken
	s.mu.RUnlock()
	if serverToken == "" || fallbackWS == uuid.Nil {
		return uuid.Nil, nil
	}
	if subtle.ConstantTimeCompare([]byte(providedToken), []byte(serverToken)) != 1 {
		return uuid.Nil, nil
	}
	if s.repo == nil {
		return fallbackWS, nil
	}
	// Only accept the fallback when there is no ambiguity about the tenant.
	wsID, err := s.repo.ResolveSCIMWorkspace(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	return wsID, nil
}

// hashSCIMToken returns the SHA-256 hex digest of a provisioning token. The
// plaintext is never persisted or logged.
func hashSCIMToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// errSCIMWorkspaceRequired is returned when a tenant-scoped operation is asked
// for without a workspace. Callers use it to distinguish a bad request from an
// internal failure.
var errSCIMWorkspaceRequired = errors.New("workspace is required")

// ErrSCIMWorkspaceRequired exposes that sentinel so HTTP callers can map a
// missing workspace to 400 rather than 500.
var ErrSCIMWorkspaceRequired = errSCIMWorkspaceRequired

// scimTokenPrefixLabel is the fixed, non-secret leading marker on every token so
// a leaked one is identifiable in a log or a config paste.
const scimTokenPrefixLabel = "scim_"

// scimTokenPrefixChars is how much of the random body is kept alongside the
// marker. Enough to recognise a key, far too little to reconstruct it.
const scimTokenPrefixChars = 6

// IssueToken mints a provisioning token for a workspace and returns the
// plaintext exactly once.
//
// Only the SHA-256 hash is stored, so the token cannot be recovered later. A
// caller that loses it must rotate: IssueToken again.
//
// The token is not usable until the workspace is entitled for SCIM; the
// entitlement check in authMiddleware is what gates provisioning, not issuance.
func (s *SCIMService) IssueToken(ctx context.Context, wsID uuid.UUID) (plaintext, prefix string, err error) {
	if s.repo == nil {
		return "", "", errors.New("scim persistence is not configured")
	}
	if wsID == uuid.Nil {
		return "", "", errSCIMWorkspaceRequired
	}

	// 32 bytes of entropy, URL-safe so the value can be pasted into an IdP
	// secret field without escaping.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("failed generating scim token: %w", err)
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	plaintext = scimTokenPrefixLabel + body
	prefix = scimTokenPrefixLabel + body[:scimTokenPrefixChars]

	if err := s.repo.SetSCIMToken(ctx, wsID, hashSCIMToken(plaintext), prefix); err != nil {
		return "", "", err
	}
	slog.Info("SCIM provisioning token issued",
		"workspace_id", wsID.String(), "prefix", prefix)
	return plaintext, prefix, nil
}

// RevokeToken disables SCIM provisioning for a workspace by clearing its token.
func (s *SCIMService) RevokeToken(ctx context.Context, wsID uuid.UUID) error {
	if s.repo == nil {
		return errors.New("scim persistence is not configured")
	}
	if wsID == uuid.Nil {
		return errSCIMWorkspaceRequired
	}
	return s.repo.SetSCIMToken(ctx, wsID, "", "")
}

// TokenState reports whether SCIM is enabled for a workspace, for a settings
// screen that must not invent an answer.
func (s *SCIMService) TokenState(ctx context.Context, wsID uuid.UUID) (bool, error) {
	if s.repo == nil {
		return false, errors.New("scim persistence is not configured")
	}
	return s.repo.HasSCIMToken(ctx, wsID)
}

// ListGroups returns the SCIM groups stored for the given workspace.
func (s *SCIMService) ListGroups(ctx context.Context, wsID uuid.UUID) ([]database.SCIMGroupRecord, error) {
	if s.repo == nil {
		return nil, errors.New("scim persistence is not configured")
	}
	return s.repo.ListSCIMGroups(ctx, wsID)
}

// Routes mounts the standard SCIM 2.0 sub-router.
func (s *SCIMService) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/ServiceProviderConfig", s.handleServiceProviderConfig)
	r.Get("/Schemas", s.handleSchemas)

	r.Group(func(gu chi.Router) {
		gu.Use(s.authMiddleware)
		gu.Route("/Users", func(u chi.Router) {
			u.Get("/", s.handleListUsers)
			u.Post("/", s.handleCreateUser)
			u.Get("/{id}", s.handleGetUser)
			u.Patch("/{id}", s.handlePatchUser)
			u.Delete("/{id}", s.handleDeleteUser)
		})
		gu.Route("/Groups", func(g chi.Router) {
			g.Get("/", s.handleListGroups)
			g.Post("/", s.handleCreateGroup)
			g.Get("/{id}", s.handleGetGroup)
			g.Put("/{id}", s.handleUpdateGroup)
			g.Patch("/{id}", s.handlePatchGroup)
			g.Delete("/{id}", s.handleDeleteGroup)
		})
	})

	return r
}

func (s *SCIMService) handleServiceProviderConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"schemas":        []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"patch":          map[string]bool{"supported": true},
		"bulk":           map[string]bool{"supported": false},
		"filter":         map[string]any{"supported": true, "maxResults": 100},
		"changePassword": map[string]bool{"supported": false},
		"sort":           map[string]bool{"supported": false},
		"etag":           map[string]bool{"supported": false},
		"authenticationSchemes": []map[string]any{
			{
				"name":        "OAuth Bearer Token",
				"description": "Authentication scheme using the OAuth Bearer Token Standard",
				"specUri":     "http://www.rfc-editor.org/info/rfc6750",
				"type":        "oauthbearertoken",
				"primary":     true,
			},
		},
	})
}

func (s *SCIMService) handleSchemas(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(SCIMListResponse{
		Schemas:      []string{ListResponseURN},
		TotalResults: 2,
		StartIndex:   1,
		ItemsPerPage: 2,
		Resources: []map[string]any{
			{"id": UserSchemaURN, "name": "User"},
			{"id": GroupSchemaURN, "name": "Group"},
		},
	})
}

func (s *SCIMService) handleListUsers(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	startIndex, _ := strconv.Atoi(r.URL.Query().Get("startIndex"))
	if startIndex < 1 {
		startIndex = 1
	}
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count <= 0 || count > 100 {
		count = 100
	}

	filter := r.URL.Query().Get("filter")

	filterUser := ""
	if filter != "" && strings.Contains(filter, "userName eq") {
		parts := strings.Split(filter, "\"")
		if len(parts) >= 2 {
			filterUser = parts[1]
		}
	}

	// Prefer the database: it is the record of who actually has a seat, and it
	// survives a restart. The in-memory map is only a fallback for a deployment
	// with no database bound.
	if s.repo != nil {
		records, err := s.repo.ListSCIMUsers(r.Context(), s.workspaceFor(r), filterUser, startIndex, count)
		if err != nil {
			slog.Error("SCIM user list failed", "workspace_id", s.workspaceFor(r).String(), "error", err)
			s.writeError(w, http.StatusServiceUnavailable, "mutability", "user directory is temporarily unavailable")
			return
		}
		resources := make([]SCIMUser, 0, len(records))
		for _, rec := range records {
			resources = append(resources, scimUserFromRecord(rec))
		}

		total, err := s.repo.CountSCIMUsers(r.Context(), s.workspaceFor(r), filterUser)
		if err != nil {
			slog.Error("SCIM user count failed", "workspace_id", s.workspaceFor(r).String(), "error", err)
			s.writeError(w, http.StatusServiceUnavailable, "mutability", "user directory is temporarily unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/scim+json")
		_ = json.NewEncoder(w).Encode(SCIMListResponse{
			Schemas:      []string{ListResponseURN},
			TotalResults: total,
			StartIndex:   startIndex,
			ItemsPerPage: len(resources),
			Resources:    resources,
		})
		return
	}

	// The in-memory map is not keyed by workspace. Serving it without a
	// resolved tenant would hand one customer every user any customer created,
	// and answering 200 with an empty list would claim the directory is simply
	// empty. Both are wrong; refuse instead.
	if s.workspaceFor(r) == uuid.Nil {
		slog.Error("SCIM user list refused: tenant could not be resolved")
		s.writeError(w, http.StatusServiceUnavailable, "mutability",
			"SCIM tenant could not be resolved; the directory was not read")
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	userList := make([]SCIMUser, 0, len(s.users))
	for _, u := range s.users {
		if filterUser != "" && u.UserName != filterUser {
			continue
		}
		userList = append(userList, u)
	}

	if len(userList) > count {
		userList = userList[:count]
	}

	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(SCIMListResponse{
		Schemas:      []string{ListResponseURN},
		TotalResults: len(userList),
		StartIndex:   startIndex,
		ItemsPerPage: len(userList),
		Resources:    userList,
	})
}

func (s *SCIMService) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var input SCIMUser
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalidSyntax", "Invalid SCIM JSON payload")
		return
	}

	if input.UserName == "" {
		s.writeError(w, http.StatusBadRequest, "invalidValue", "userName is required")
		return
	}

	email := input.UserName
	if len(input.Emails) > 0 && input.Emails[0].Value != "" {
		email = input.Emails[0].Value
	}

	// Provisioning consumes a seat, so the quota is checked before any write.
	//
	// Unbound is a refusal, not a skip. When the service has no tenant or no
	// resolver there is nothing to meter against, and quietly provisioning anyway
	// is how seats go out unmetered - which is the state both binaries were in
	// before BindTenant existed.
	wsID := s.workspaceFor(r)
	if s.resolver == nil || wsID == uuid.Nil {
		slog.Error("SCIM provisioning refused: service is not bound to a tenant")
		s.writeError(w, http.StatusServiceUnavailable, "mutability",
			"SCIM provisioning is not bound to an organization; no user was provisioned")
		return
	}

	if err := s.resolver.CheckSeats(r.Context(), s.workspaceFor(r), 1); err != nil {
		var limitErr *license.SeatLimitError
		if errors.As(err, &limitErr) {
			s.writeError(w, http.StatusConflict, "mutability", limitErr.Error())
			return
		}
		var invalidErr *license.EntitlementInvalidError
		if errors.As(err, &invalidErr) {
			// The workspace has no valid license: 409, not 503. Nothing is
			// broken, the customer simply is not entitled.
			s.writeError(w, http.StatusConflict, "mutability", invalidErr.Error())
			return
		}
		s.writeError(w, http.StatusServiceUnavailable, "mutability",
			"seat quota could not be verified; no user was provisioned")
		return
	}

	// Persist before mutating the in-memory map, so a failed insert does not
	// leave a user that exists in memory but not in the database.
	var orgID *uuid.UUID
	if wsID != uuid.Nil {
		orgID = &wsID
	}
	if s.repo != nil {
		if _, err := s.repo.CreateUser(r.Context(), email, "scim-sso-managed", "member", orgID); err != nil {
			s.writeError(w, http.StatusInternalServerError, "mutability",
				"failed to persist the provisioned user")
			return
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	newID := uuid.New().String()
	input.ID = newID
	input.Schemas = []string{UserSchemaURN}
	input.Meta.ResourceType = "User"
	input.Meta.Location = "/scim/v2/Users/" + newID
	s.users[newID] = input

	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(input)
}

func (s *SCIMService) handleGetUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// The database is the record of who exists. The in-memory map is a fallback
	// for a deployment with no store bound; reading it first made every
	// database-backed user 404 on GET, PATCH and DELETE, which silently disabled
	// IdP deprovisioning.
	if s.repo != nil {
		rec, err := s.repo.GetSCIMUserByID(r.Context(), s.workspaceFor(r), id)
		if err != nil {
			slog.Error("SCIM user read failed", "workspace_id", s.workspaceFor(r).String(), "id", id, "error", err)
			s.writeError(w, http.StatusServiceUnavailable, "mutability", "user directory is temporarily unavailable")
			return
		}
		if rec == nil {
			s.writeError(w, http.StatusNotFound, "", "User not found")
			return
		}
		w.Header().Set("Content-Type", "application/scim+json")
		_ = json.NewEncoder(w).Encode(scimUserFromRecord(*rec))
		return
	}

	s.mu.RLock()
	user, exists := s.users[id]
	s.mu.RUnlock()

	if !exists {
		s.writeError(w, http.StatusNotFound, "", "User not found")
		return
	}

	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(user)
}

func (s *SCIMService) handlePatchUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var patch SCIMPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalidSyntax", "Invalid SCIM patch payload")
		return
	}

	// Resolve the user from the database. The in-memory map does not know about
	// users provisioned by an earlier process, so a deprovisioning request for a
	// real user used to 404 - the one request an enterprise depends on.
	var (
		email  string
		active = true
	)
	if s.repo != nil {
		rec, err := s.repo.GetSCIMUserByID(r.Context(), s.workspaceFor(r), id)
		if err != nil {
			slog.Error("SCIM user read failed", "workspace_id", s.workspaceFor(r).String(), "id", id, "error", err)
			s.writeError(w, http.StatusServiceUnavailable, "mutability", "user directory is temporarily unavailable")
			return
		}
		if rec == nil {
			s.writeError(w, http.StatusNotFound, "", "User not found")
			return
		}
		email, active = rec.Email, rec.Active
	} else {
		s.mu.RLock()
		user, exists := s.users[id]
		s.mu.RUnlock()
		if !exists {
			s.writeError(w, http.StatusNotFound, "", "User not found")
			return
		}
		email, active = user.UserName, user.Active
	}

	for _, op := range patch.Operations {
		if strings.EqualFold(op.Op, "replace") && strings.EqualFold(op.Path, "active") {
			if activeBool, ok := op.Value.(bool); ok {
				active = activeBool
			}
		}
	}

	// Persist before revoking, so a failed write cannot leave sessions revoked
	// for a user the product still treats as active.
	if s.repo != nil {
		if err := s.repo.SetSCIMUserActive(r.Context(), s.workspaceFor(r), id, active); err != nil {
			slog.Error("SCIM user update failed", "workspace_id", s.workspaceFor(r).String(), "id", id, "error", err)
			s.writeError(w, http.StatusInternalServerError, "mutability", "failed to update user")
			return
		}
	} else {
		s.mu.Lock()
		user := s.users[id]
		user.Active = active
		s.users[id] = user
		s.mu.Unlock()
	}

	// Deprovisioning Enforcement: a deactivated user must lose every session.
	// "inactive" is a real users_status_enum value; the previous "suspended" was
	// not, so the status update failed silently and only the in-memory flag moved.
	if !active && s.repo != nil {
		if dbUser, err := s.repo.GetUserByEmail(r.Context(), email); err == nil && dbUser != nil {
			if err := s.repo.InvalidateAllUserRefreshTokens(r.Context(), dbUser.UUID); err != nil {
				slog.Error("SCIM deprovisioning: token revocation failed",
					"user_id", dbUser.UUID.String(), "error", err)
			}
			if err := s.repo.UpdateUserStatus(r.Context(), dbUser.UUID, "inactive"); err != nil {
				slog.Error("SCIM deprovisioning: status update failed",
					"user_id", dbUser.UUID.String(), "error", err)
			} else {
				slog.Info("SCIM deprovisioning: sessions revoked and account deactivated",
					"user_id", dbUser.UUID.String(), "email", email)
			}
		} else {
			slog.Warn("SCIM deprovisioning: no user record matched for session revocation", "email", email)
		}
	}

	w.Header().Set("Content-Type", "application/scim+json")
	if s.repo != nil {
		if rec, err := s.repo.GetSCIMUserByID(r.Context(), s.workspaceFor(r), id); err == nil && rec != nil {
			_ = json.NewEncoder(w).Encode(scimUserFromRecord(*rec))
			return
		}
	}
	_ = json.NewEncoder(w).Encode(SCIMUser{
		Schemas:  []string{UserSchemaURN},
		ID:       id,
		UserName: email,
		Active:   active,
	})
}

func (s *SCIMService) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	s.mu.Lock()
	user, exists := s.users[id]
	if exists {
		delete(s.users, id)
	}
	s.mu.Unlock()

	if !exists {
		s.writeError(w, http.StatusNotFound, "", "User not found")
		return
	}

	// Deprovisioning Enforcement (§8.3): Delete user sessions and revoke all active refresh tokens
	if s.repo != nil {
		email := user.UserName
		if len(user.Emails) > 0 && user.Emails[0].Value != "" {
			email = user.Emails[0].Value
		}
		if dbUser, err := s.repo.GetUserByEmail(r.Context(), email); err == nil && dbUser != nil {
			_ = s.repo.InvalidateAllUserRefreshTokens(r.Context(), dbUser.UUID)
			_ = s.repo.UpdateUserStatus(r.Context(), dbUser.UUID, "inactive")
			slog.Info("SCIM deprovisioning (delete): user sessions revoked and account deactivated", "user_id", dbUser.UUID, "email", email)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *SCIMService) writeError(w http.ResponseWriter, statusCode int, scimType, detail string) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(SCIMError{
		Schemas:  []string{ErrorSchemaURN},
		Status:   strconv.Itoa(statusCode),
		ScimType: scimType,
		Detail:   detail,
	})
}

func (s *SCIMService) handleListGroups(w http.ResponseWriter, r *http.Request) {
	if !s.bound(r) {
		s.writeError(w, http.StatusServiceUnavailable, "", "SCIM storage is not configured")
		return
	}

	wsID := s.workspaceFor(r)
	groups, err := s.repo.ListSCIMGroups(r.Context(), wsID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "", "failed to list groups")
		return
	}

	all := make([]SCIMGroup, 0, len(groups))
	for _, rec := range groups {
		all = append(all, scimGroupFromRecord(rec))
	}

	startIndex, _ := strconv.Atoi(r.URL.Query().Get("startIndex"))
	if startIndex < 1 {
		startIndex = 1
	}
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count <= 0 {
		count = len(all)
	}

	total := len(all)
	start := startIndex - 1
	if start > total {
		start = total
	}
	end := start + count
	if end > total {
		end = total
	}

	_ = json.NewEncoder(w).Encode(SCIMListResponse{
		Schemas:      []string{ListResponseURN},
		TotalResults: total,
		StartIndex:   startIndex,
		ItemsPerPage: end - start,
		Resources:    all[start:end],
	})
}

func (s *SCIMService) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	if !s.bound(r) {
		s.writeError(w, http.StatusServiceUnavailable, "", "SCIM storage is not configured")
		return
	}

	var group SCIMGroup
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&group); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalidSyntax", "Malformed SCIM Group JSON")
		return
	}
	if strings.TrimSpace(group.DisplayName) == "" {
		s.writeError(w, http.StatusBadRequest, "invalidValue", "displayName is required for SCIM Group")
		return
	}
	group.Schemas = []string{GroupSchemaURN}
	group.ID = strings.TrimSpace(group.ID)
	if group.ID == "" {
		group.ID = uuid.NewString()
	}
	group.Meta = SCIMMeta{
		ResourceType: "Group",
		Created:      time.Now().UTC(),
		LastModified: time.Now().UTC(),
		Location:     "/v2/Groups/" + group.ID,
	}

	if err := s.repo.UpsertSCIMGroup(r.Context(), s.workspaceFor(r), group.toRecord()); err != nil {
		s.writeError(w, http.StatusInternalServerError, "", "failed to store group")
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(group)
}

func (s *SCIMService) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	if !s.bound(r) {
		s.writeError(w, http.StatusServiceUnavailable, "", "SCIM storage is not configured")
		return
	}
	id := chi.URLParam(r, "id")

	rec, err := s.repo.GetSCIMGroup(r.Context(), s.workspaceFor(r), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "", "failed to load group")
		return
	}
	if rec == nil {
		s.writeError(w, http.StatusNotFound, "", "Group not found")
		return
	}

	_ = json.NewEncoder(w).Encode(scimGroupFromRecord(*rec))
}

func (s *SCIMService) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	if !s.bound(r) {
		s.writeError(w, http.StatusServiceUnavailable, "", "SCIM storage is not configured")
		return
	}
	id := chi.URLParam(r, "id")

	var updated SCIMGroup
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&updated); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalidSyntax", "Malformed SCIM Group JSON")
		return
	}

	existing, err := s.repo.GetSCIMGroup(r.Context(), s.workspaceFor(r), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "", "failed to load group")
		return
	}
	if existing == nil {
		s.writeError(w, http.StatusNotFound, "", "Group not found")
		return
	}

	updated.ID = id
	updated.Schemas = []string{GroupSchemaURN}
	updated.DisplayName = strings.TrimSpace(updated.DisplayName)
	updated.Meta = SCIMMeta{
		ResourceType: "Group",
		Created:      existing.CreatedAt.UTC(),
		LastModified: time.Now().UTC(),
		Location:     existing.ID,
	}
	if updated.Members == nil {
		updated.Members = []SCIMGroupMember{}
	}

	// PUT replaces the group including its membership, so the stored membership
	// is rewritten wholesale rather than merged.
	if err := s.repo.UpsertSCIMGroup(r.Context(), s.workspaceFor(r), updated.toRecord()); err != nil {
		s.writeError(w, http.StatusInternalServerError, "", "failed to store group")
		return
	}

	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(updated)
}

func (s *SCIMService) handlePatchGroup(w http.ResponseWriter, r *http.Request) {
	if !s.bound(r) {
		s.writeError(w, http.StatusServiceUnavailable, "", "SCIM storage is not configured")
		return
	}
	id := chi.URLParam(r, "id")

	var patchReq SCIMPatchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&patchReq); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalidSyntax", "Malformed SCIM PATCH payload")
		return
	}

	rec, err := s.repo.GetSCIMGroup(r.Context(), s.workspaceFor(r), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "", "failed to load group")
		return
	}
	if rec == nil {
		s.writeError(w, http.StatusNotFound, "", "Group not found")
		return
	}

	group := scimGroupFromRecord(*rec)
	for _, op := range patchReq.Operations {
		opLower := strings.ToLower(op.Op)
		pathLower := strings.ToLower(op.Path)

		switch opLower {
		case "replace":
			if pathLower == "displayname" {
				if strVal, ok := op.Value.(string); ok && strVal != "" {
					group.DisplayName = strVal
				}
			}
		case "add":
			if pathLower == "members" || pathLower == "" {
				if membersList, ok := op.Value.([]any); ok {
					for _, m := range membersList {
						if mMap, ok := m.(map[string]any); ok {
							val, _ := mMap["value"].(string)
							disp, _ := mMap["display"].(string)
							if val != "" {
								group.Members = append(group.Members, SCIMGroupMember{Value: val, Display: disp})
							}
						}
					}
				}
			}
		case "remove":
			if strings.HasPrefix(pathLower, "members") {
				group.Members = []SCIMGroupMember{}
			}
		}
	}

	group.Meta.LastModified = time.Now().UTC()
	if err := s.repo.UpsertSCIMGroup(r.Context(), s.workspaceFor(r), group.toRecord()); err != nil {
		s.writeError(w, http.StatusInternalServerError, "", "failed to store group")
		return
	}

	w.Header().Set("Content-Type", "application/scim+json")
	_ = json.NewEncoder(w).Encode(group)
}

func (s *SCIMService) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	if !s.bound(r) {
		s.writeError(w, http.StatusServiceUnavailable, "", "SCIM storage is not configured")
		return
	}
	id := chi.URLParam(r, "id")

	deleted, err := s.repo.DeleteSCIMGroup(r.Context(), s.workspaceFor(r), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "", "failed to delete group")
		return
	}
	if !deleted {
		s.writeError(w, http.StatusNotFound, "", "Group not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// scimGroupFromRecord renders a stored group as a SCIM resource.
func scimGroupFromRecord(rec database.SCIMGroupRecord) SCIMGroup {
	g := SCIMGroup{
		Schemas:     []string{GroupSchemaURN},
		ID:          rec.ID,
		DisplayName: rec.DisplayName,
		Members:     make([]SCIMGroupMember, 0, len(rec.Members)),
		Meta: SCIMMeta{
			ResourceType: "Group",
			Created:      rec.CreatedAt.UTC(),
			LastModified: rec.UpdatedAt.UTC(),
			Location:     "/v2/Groups/" + rec.ID,
		},
	}
	for _, m := range rec.Members {
		g.Members = append(g.Members, SCIMGroupMember{Value: m.Ref, Display: m.Display})
	}
	return g
}

// toRecord converts an outbound SCIM group into its storage shape.
func (g SCIMGroup) toRecord() database.SCIMGroupRecord {
	rec := database.SCIMGroupRecord{ID: g.ID, DisplayName: g.DisplayName}
	rec.Members = make([]database.SCIMGroupMemberRecord, 0, len(g.Members))
	for _, m := range g.Members {
		rec.Members = append(rec.Members, database.SCIMGroupMemberRecord{Ref: m.Value, Display: m.Display})
	}
	return rec
}

// bound reports whether the service has a tenant and a database, which is the
// only configuration in which group state is durable. Without both we answer
// 503 rather than serving in-memory state that silently disappears on restart.
// bound reports whether this request has both persistence and a tenant.
//
// It takes the request because the tenant is now resolved per request. The old
// signature checked only the startup-bound workspace, so a deployment that had
// not yet called BindTenant would 503 even when the presented token named a
// valid tenant.
func (s *SCIMService) bound(r *http.Request) bool {
	return s.repo != nil && s.workspaceFor(r) != uuid.Nil
}

// scimUserFromRecord renders a neutral directory record as a SCIM user
// resource. The email doubles as userName, matching what every IdP sends.
func scimUserFromRecord(rec database.SCIMUserRecord) SCIMUser {
	user := SCIMUser{
		Schemas:  []string{UserSchemaURN},
		ID:       rec.ID,
		UserName: rec.UserName,
		Active:   rec.Active,
		Emails: []SCIMEmail{
			{Value: rec.Email, Primary: true, Type: "work"},
		},
		Meta: SCIMMeta{
			ResourceType: "User",
			Created:      rec.CreatedAt.UTC(),
			LastModified: rec.ModifiedAt.UTC(),
			Location:     "/scim/v2/Users/" + rec.ID,
		},
	}
	if rec.GivenName != "" || rec.FamilyName != "" {
		user.Name = SCIMName{GivenName: rec.GivenName, FamilyName: rec.FamilyName}
	}
	return user
}

// BindTenant resolves the workspace a SCIM connection provisions into and binds
// it, with an entitlement resolver, to the service.
//
// Both binaries must call this. The seat-quota check in handleCreateUser reads
// the workspace and resolver from the service, and until they are set the check
// is skipped - which is how an IdP could provision past the licensed seat count
// while every log said the quota was enforced.
//
// Returns the bound workspace id, or uuid.Nil when the instance has no
// workspace yet (a fresh install, before the first signup), in which case
// provisioning stays unbound and the caller should surface that.
func BindTenant(ctx context.Context, repo *database.Repository, svc *SCIMService, resolver *license.Resolver) uuid.UUID {
	if svc == nil || repo == nil {
		return uuid.Nil
	}

	wsID, err := repo.ResolveSCIMWorkspace(ctx)
	if err != nil {
		slog.Error("SCIM tenant binding failed; seat quota is NOT enforced", "error", err)
		return uuid.Nil
	}
	if wsID == uuid.Nil {
		slog.Warn("No workspace exists yet; SCIM seat quota cannot be enforced")
		return uuid.Nil
	}

	svc.SetEntitlement(wsID, resolver)
	return wsID
}

// ═══════════════════════════════════════════════════════════════════════════
// TENANT SCOPE PROPAGATION
//
// The workspace is resolved per request from the presented token and carried on
// the context. Reading it from a field on the service instead would make every
// concurrent request share one tenant, which is how a second customer's sync
// ended up writing into the first customer's workspace.
// ═══════════════════════════════════════════════════════════════════════════

type scimWorkspaceKey struct{}

// WithSCIMWorkspace returns a context carrying the tenant resolved for this
// request.
func WithSCIMWorkspace(ctx context.Context, wsID uuid.UUID) context.Context {
	return context.WithValue(ctx, scimWorkspaceKey{}, wsID)
}

// SCIMWorkspaceFrom returns the tenant resolved by authMiddleware. It returns
// uuid.Nil when the request did not pass through that middleware, so a handler
// mounted outside it fails closed rather than reading a default.
func SCIMWorkspaceFrom(ctx context.Context) uuid.UUID {
	wsID, _ := ctx.Value(scimWorkspaceKey{}).(uuid.UUID)
	return wsID
}

// workspaceFor resolves the tenant for a request, falling back to the
// startup-bound workspace only when the request carries no scope. Handlers use
// this so they cannot silently operate on a stale global.
func (s *SCIMService) workspaceFor(r *http.Request) uuid.UUID {
	if wsID := SCIMWorkspaceFrom(r.Context()); wsID != uuid.Nil {
		return wsID
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.workspaceID
}

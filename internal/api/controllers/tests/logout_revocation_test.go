package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

// stubAuthRepo implements only the identity/session methods this test drives.
// The embedded interface is deliberate: any other call panics loudly instead of
// silently returning a zero value, which is what let the original F-18 defect
// hide behind a nil repository.
type stubAuthRepo struct {
	controllers.AuthRepository

	refreshOwner  uuid.UUID
	refreshKnown  bool
	markedUsed    bool
	revokeCalls   int
	revokeCutoffs []int64
	cutoffs       []int64
	revokeErr     error
	isRevokedErr  error
}

// In-memory stand-in for migrations/041 with the same cutoff semantics:
// any token whose iat <= a recorded cutoff is revoked.
type revokedSet struct {
	cutoffs []int64
}

// handleMe reads the caller's row, so the stub has to answer it.
func (s *stubAuthRepo) GetUserByID(_ context.Context, id uuid.UUID) (*database.UserRecord, error) {
	return &database.UserRecord{UUID: id, Email: "owner@scandrix.internal", Status: "active"}, nil
}

func (s *stubAuthRepo) GetRefreshToken(_ context.Context, _ string) (*database.RefreshTokenRecord, error) {
	if !s.refreshKnown {
		return nil, errors.New("no such refresh token")
	}
	return &database.RefreshTokenRecord{UserUUID: s.refreshOwner}, nil
}

func (s *stubAuthRepo) MarkRefreshTokenUsed(context.Context, string) error {
	s.markedUsed = true
	return nil
}

func (s *stubAuthRepo) RevokeAccessTokensUpTo(_ context.Context, userID uuid.UUID, cutoff int64) error {
	if s.revokeErr != nil {
		return s.revokeErr
	}
	s.revokeCalls++
	s.revokeCutoffs = append(s.revokeCutoffs, cutoff)
	s.cutoffs = append(s.cutoffs, cutoff)
	return nil
}

func (s *stubAuthRepo) IsAccessTokenRevoked(_ context.Context, _ uuid.UUID, issuedAt int64) (bool, error) {
	if s.isRevokedErr != nil {
		return false, s.isRevokedErr
	}
	for _, cutoff := range s.cutoffs {
		if issuedAt <= cutoff {
			return true, nil
		}
	}
	return false, nil
}

func (s *stubAuthRepo) revoked(issuedAt int64) bool {
	r, _ := s.IsAccessTokenRevoked(context.Background(), uuid.Nil, issuedAt)
	return r
}

func login(t *testing.T, router http.Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func logout(t *testing.T, router http.Handler, refresh string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(dtos.LogoutRequest{RefreshToken: refresh})
	req := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// The core F-18 regression: a bearer token that authenticated successfully a
// moment earlier must stop working the instant logout completes.
func TestLogoutRevokesPreviouslyValidAccessToken(t *testing.T) {
	authSvc := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	userID := uuid.New()
	wsID := uuid.New()
	repo := &stubAuthRepo{refreshOwner: userID, refreshKnown: true}

	// Mirror the router's production wiring of the revocation hook.
	authSvc.SetRevocationChecker(func(ctx context.Context, u uuid.UUID, issuedAt int64) bool {
		revoked, err := repo.IsAccessTokenRevoked(ctx, u, issuedAt)
		if err != nil {
			return true // fail closed, as the router does
		}
		return revoked
	})

	router := controllers.NewAuthController(authSvc, repo).Routes()

	accessToken, refreshToken, err := authSvc.GenerateTokenPair(userID, wsID, models.RoleOwner)
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}

	// Before logout the token works.
	if rec := login(t, router, accessToken); rec.Code != http.StatusOK {
		t.Fatalf("precondition: expected the fresh access token to authenticate, got %d: %s",
			rec.Code, rec.Body.String())
	}

	if rec := logout(t, router, refreshToken); rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /logout, got %d: %s", rec.Code, rec.Body.String())
	}
	if !repo.markedUsed {
		t.Error("logout must mark the refresh token used")
	}
	if repo.revokeCalls != 1 {
		t.Fatalf("expected exactly one revocation write, got %d", repo.revokeCalls)
	}

	// The same bearer token must now be rejected. Before this fix it kept
	// working for the full 15-minute lifetime.
	rec := login(t, router, accessToken)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d: %s", rec.Code, rec.Body.String())
	}
}

// A token minted after the cutoff is a new session and must survive, otherwise
// a refresh racing with logout would be swallowed.
//
// The boundary is asserted on explicit issued-at values rather than on wall
// clock, because a JWT carries second-granularity iat: a token minted in the
// same second as the logout has an iat indistinguishable from the cutoff and is
// therefore revoked. That is the intended, conservative outcome -- admitting it
// would mean admitting any token an attacker minted concurrently with the
// victim's logout.
func TestRevocationCutoffOnlyRejectsTokensIssuedAtOrBeforeIt(t *testing.T) {
	authSvc := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	userID := uuid.New()
	repo := &stubAuthRepo{refreshOwner: userID, refreshKnown: true}
	router := controllers.NewAuthController(authSvc, repo).Routes()

	_, refreshToken, err := authSvc.GenerateTokenPair(userID, uuid.New(), models.RoleOwner)
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}
	if rec := logout(t, router, refreshToken); rec.Code != http.StatusOK {
		t.Fatalf("logout: %d %s", rec.Code, rec.Body.String())
	}
	if repo.revokeCalls != 1 {
		t.Fatalf("expected one revocation write, got %d", repo.revokeCalls)
	}

	cutoff := repo.revokeCutoffs[0]
	if cutoff <= 0 {
		t.Fatalf("expected a positive unix cutoff, got %d", cutoff)
	}

	// Same second as the cutoff: revoked. Concurrent minting must not survive.
	if !repo.revoked(cutoff) {
		t.Error("a token issued at the cutoff second must be revoked")
	}
	// Earlier: revoked.
	if !repo.revoked(cutoff - 60) {
		t.Error("a token issued before the cutoff must be revoked")
	}
	// Later: survives, so a refresh racing the logout yields a new session.
	if repo.revoked(cutoff + 1) {
		t.Error("a token issued after the cutoff must not be revoked")
	}
	if repo.revoked(cutoff + 3600) {
		t.Error("a token issued well after the cutoff must not be revoked")
	}
}

// An unreachable revocation store must reject, not admit: answering
// "not revoked" on a database error is an authentication bypass.
func TestRevocationStoreErrorFailsClosed(t *testing.T) {
	authSvc := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	repo := &stubAuthRepo{isRevokedErr: errors.New("connection refused")}

	authSvc.SetRevocationChecker(func(ctx context.Context, u uuid.UUID, issuedAt int64) bool {
		revoked, err := repo.IsAccessTokenRevoked(ctx, u, issuedAt)
		if err != nil {
			return true
		}
		return revoked
	})

	router := controllers.NewAuthController(authSvc, repo).Routes()
	token, _, err := authSvc.GenerateTokenPair(uuid.New(), uuid.New(), models.RoleOwner)
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}
	if rec := login(t, router, token); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when the revocation store is unreachable, got %d", rec.Code)
	}
}

// Logout with no repository cannot end the session, so it must not claim
// success while leaving the access token live.
func TestLogoutWithoutRepositoryFailsClosed(t *testing.T) {
	authSvc := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	router := controllers.NewAuthController(authSvc, nil).Routes()

	rec := logout(t, router, "irrelevant")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when logout cannot reach the repository, got %d: %s",
			rec.Code, rec.Body.String())
	}
}

// A logout with no refresh token has no identity to revoke against. It must
// clear cookies but must not guess a user and revoke their session.
func TestLogoutWithoutRefreshTokenDoesNotRevokeAnyone(t *testing.T) {
	authSvc := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	repo := &stubAuthRepo{refreshOwner: uuid.New(), refreshKnown: true}
	router := controllers.NewAuthController(authSvc, repo).Routes()

	if rec := logout(t, router, ""); rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if repo.revokeCalls != 0 {
		t.Fatalf("logout must not revoke without an attributable identity, got %d writes", repo.revokeCalls)
	}
}

// A failure to record the revocation must be surfaced. Returning 200 while the
// access token stays live is exactly the lie F-18 is about.
func TestLogoutSurfacesRevocationFailure(t *testing.T) {
	authSvc := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	userID := uuid.New()
	repo := &stubAuthRepo{
		refreshOwner: userID,
		refreshKnown: true,
		revokeErr:    errors.New("deadlock detected"),
	}
	router := controllers.NewAuthController(authSvc, repo).Routes()

	_, refreshToken, err := authSvc.GenerateTokenPair(userID, uuid.New(), models.RoleOwner)
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}
	if rec := logout(t, router, refreshToken); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when revocation cannot be recorded, got %d: %s",
			rec.Code, rec.Body.String())
	}
}

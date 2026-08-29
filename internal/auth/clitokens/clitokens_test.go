package clitokens_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/clitokens"
)

func TestTokenServiceAndMiddleware(t *testing.T) {
	ctx := context.Background()
	service := clitokens.NewTokenService()

	wsID := uuid.New()
	teamID := uuid.New()
	creatorID := uuid.New()

	// 1. Mint Token
	mintResp, err := service.MintToken(ctx, clitokens.MintTokenRequest{
		WorkspaceID: wsID,
		TeamID:      teamID,
		Name:        "CI/CD Pipeline Key",
		Scopes:      []clitokens.TokenScope{clitokens.ScopeReviewRead, clitokens.ScopeReviewWrite},
		TTL:         1 * time.Hour,
		CreatedBy:   creatorID,
	})
	if err != nil {
		t.Fatalf("mint token failed: %v", err)
	}

	if !strings.HasPrefix(mintResp.Token, "scandrix_team_") {
		t.Fatalf("unexpected token prefix: %s", mintResp.Token)
	}
	if mintResp.Record.UsageCount != 0 || mintResp.Record.IsRevoked {
		t.Fatalf("unexpected initial record state: %+v", mintResp.Record)
	}

	// 2. Validate Token with correct scope
	rec, err := service.ValidateToken(ctx, mintResp.Token, clitokens.ScopeReviewWrite)
	if err != nil || rec.UsageCount != 1 || rec.LastUsedAt == nil {
		t.Fatalf("token validation failed: %+v, err: %v", rec, err)
	}

	// 3. Validate Token with missing scope (must return ErrScopeMissing)
	_, errMissingScope := service.ValidateToken(ctx, mintResp.Token, clitokens.ScopeAdmin)
	if errMissingScope != clitokens.ErrScopeMissing {
		t.Fatalf("expected ErrScopeMissing, got: %v", errMissingScope)
	}

	// 4. Test HTTP Middleware with X-Team-Key header
	mw := clitokens.NewCLITokenMiddleware(service)
	protectedHandler := mw.RequireScope(clitokens.ScopeReviewWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("authorized"))
	}))

	// Case A: Valid X-Team-Key
	reqA := httptest.NewRequest(http.MethodGet, "/api/review", nil)
	reqA.Header.Set("X-Team-Key", mintResp.Token)
	wA := httptest.NewRecorder()
	protectedHandler.ServeHTTP(wA, reqA)
	if wA.Code != http.StatusOK || wA.Body.String() != "authorized" {
		t.Fatalf("expected 200 OK, got %d: %s", wA.Code, wA.Body.String())
	}

	// Case B: Valid Bearer Header
	reqB := httptest.NewRequest(http.MethodGet, "/api/review", nil)
	reqB.Header.Set("Authorization", "Bearer "+mintResp.Token)
	wB := httptest.NewRecorder()
	protectedHandler.ServeHTTP(wB, reqB)
	if wB.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with Bearer, got %d", wB.Code)
	}

	// Case C: Invalid Token
	reqC := httptest.NewRequest(http.MethodGet, "/api/review", nil)
	reqC.Header.Set("X-Team-Key", "scandrix_team_bogus_token")
	wC := httptest.NewRecorder()
	protectedHandler.ServeHTTP(wC, reqC)
	if wC.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized on bogus token, got %d", wC.Code)
	}

	// Case D: Token Revocation
	if err := service.RevokeToken(ctx, mintResp.Record.ID); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	reqD := httptest.NewRequest(http.MethodGet, "/api/review", nil)
	reqD.Header.Set("X-Team-Key", mintResp.Token)
	wD := httptest.NewRecorder()
	protectedHandler.ServeHTTP(wD, reqD)
	if wD.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized on revoked token, got %d", wD.Code)
	}
}

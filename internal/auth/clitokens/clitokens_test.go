package clitokens_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/clitokens"
	"github.com/scandrix/backend/pkg/models"
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

type mockRepo struct {
	savedKeys []*models.TeamCLIKey
	touched   uuid.UUID
	revoked   uuid.UUID
}

func (m *mockRepo) SaveAPIKey(ctx context.Context, id, workspaceID uuid.UUID, name, keyHash, prefix string, expiresAt *time.Time) error {
	m.savedKeys = append(m.savedKeys, &models.TeamCLIKey{
		ID:          id,
		WorkspaceID: &workspaceID,
		Name:        name,
		KeyHash:     keyHash,
		KeyPrefix:   prefix,
		Active:      true,
		ExpiresAt:   expiresAt,
		CreatedAt:   time.Now().UTC(),
	})
	return nil
}

func (m *mockRepo) GetAPIKeyByHash(ctx context.Context, keyHash string) (*models.TeamCLIKey, error) {
	for _, k := range m.savedKeys {
		if k.KeyHash == keyHash {
			return k, nil
		}
	}
	return nil, clitokens.ErrTokenNotFound
}

func (m *mockRepo) TouchAPIKeyUsage(ctx context.Context, keyID uuid.UUID) error {
	m.touched = keyID
	return nil
}

func (m *mockRepo) RevokeAPIKey(ctx context.Context, workspaceID, keyID uuid.UUID) error {
	m.revoked = keyID
	for _, k := range m.savedKeys {
		if k.ID == keyID {
			k.Active = false
		}
	}
	return nil
}

func (m *mockRepo) ListAPIKeys(ctx context.Context, workspaceID uuid.UUID) ([]*models.TeamCLIKey, error) {
	return m.savedKeys, nil
}

func TestTokenServiceWithRepository(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{}
	service := clitokens.NewTokenService(repo)

	wsID := uuid.New()
	teamID := uuid.New()
	creatorID := uuid.New()

	// 1. Mint token persists to repository
	mintResp, err := service.MintToken(ctx, clitokens.MintTokenRequest{
		WorkspaceID: wsID,
		TeamID:      teamID,
		Name:        "DB Backed Key",
		Scopes:      []clitokens.TokenScope{clitokens.ScopeReviewRead},
		TTL:         1 * time.Hour,
		CreatedBy:   creatorID,
	})
	if err != nil {
		t.Fatalf("mint token failed: %v", err)
	}
	if len(repo.savedKeys) != 1 {
		t.Fatalf("expected 1 saved key in repository, got %d", len(repo.savedKeys))
	}

	// 2. Clear in-memory cache to simulate fresh server restart
	serviceFresh := clitokens.NewTokenService(repo)

	// 3. Validate token from repository
	rec, err := serviceFresh.ValidateToken(ctx, mintResp.Token, clitokens.ScopeReviewRead)
	if err != nil {
		t.Fatalf("validation against repo failed: %v", err)
	}
	if rec.ID != mintResp.Record.ID {
		t.Fatalf("mismatched key ID from repo: got %s, want %s", rec.ID, mintResp.Record.ID)
	}
	if repo.touched != rec.ID {
		t.Fatalf("expected TouchAPIKeyUsage to be called for %s, got %s", rec.ID, repo.touched)
	}

	// 4. Revocation propagates to repository
	if err := serviceFresh.RevokeToken(ctx, rec.ID); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	if repo.revoked != rec.ID {
		t.Fatalf("expected RevokeAPIKey to be called for %s", rec.ID)
	}

	// 5. Subsequent validation rejects revoked token
	_, errRevoked := serviceFresh.ValidateToken(ctx, mintResp.Token, clitokens.ScopeReviewRead)
	if errRevoked != clitokens.ErrTokenRevoked {
		t.Fatalf("expected ErrTokenRevoked after repo revocation, got %v", errRevoked)
	}
}

func TestTokenServiceLeastPrivilegeScopes(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{}
	service := clitokens.NewTokenService(repo)

	plaintext := "scandrix_team_restricted_key_9999"
	h := sha256.Sum256([]byte(plaintext))
	keyHash := hex.EncodeToString(h[:])

	keyID := uuid.New()
	wsID := uuid.New()
	repo.savedKeys = append(repo.savedKeys, &models.TeamCLIKey{
		ID:          keyID,
		WorkspaceID: &wsID,
		Name:        "Read-Only Reviewer",
		KeyHash:     keyHash,
		KeyPrefix:   "scandrix_team_rest",
		Active:      true,
		Config:      []byte(`{"scopes":["review:read"]}`),
		CreatedAt:   time.Now().UTC(),
	})

	// 1. Review read scope succeeds
	rec, err := service.ValidateToken(ctx, plaintext, clitokens.ScopeReviewRead)
	if err != nil {
		t.Fatalf("expected validation success for read scope, got: %v", err)
	}
	if rec.Name != "Read-Only Reviewer" {
		t.Fatalf("unexpected record name: %s", rec.Name)
	}

	// 2. Admin scope must be rejected because key only has review:read
	_, errAdmin := service.ValidateToken(ctx, plaintext, clitokens.ScopeAdmin)
	if errAdmin != clitokens.ErrScopeMissing {
		t.Fatalf("expected ErrScopeMissing for ScopeAdmin, got: %v", errAdmin)
	}

	// 3. Write scope must also be rejected
	_, errWrite := service.ValidateToken(ctx, plaintext, clitokens.ScopeReviewWrite)
	if errWrite != clitokens.ErrScopeMissing {
		t.Fatalf("expected ErrScopeMissing for ScopeReviewWrite, got: %v", errWrite)
	}
}

package contextresolver_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	contextresolver "github.com/scandrix/backend/internal/core/context_resolver"
	"github.com/scandrix/backend/internal/core/crypto"
	"github.com/scandrix/backend/internal/core/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockWorkspaceRepo implements domain.WorkspaceRepository for context resolver tests.
type mockWorkspaceRepo struct {
	mu         sync.RWMutex
	workspaces map[uuid.UUID]*domain.Workspace
}

func newMockWorkspaceRepo() *mockWorkspaceRepo {
	return &mockWorkspaceRepo{
		workspaces: make(map[uuid.UUID]*domain.Workspace),
	}
}

func (m *mockWorkspaceRepo) Create(ctx context.Context, ws *domain.Workspace) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workspaces[ws.ID] = ws
	return nil
}

func (m *mockWorkspaceRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Workspace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ws, ok := m.workspaces[id]
	if !ok {
		return nil, nil
	}
	return ws, nil
}

func (m *mockWorkspaceRepo) FindBySlug(ctx context.Context, slug string) (*domain.Workspace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ws := range m.workspaces {
		if ws.Slug == slug {
			return ws, nil
		}
	}
	return nil, nil
}

func (m *mockWorkspaceRepo) Update(ctx context.Context, ws *domain.Workspace) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workspaces[ws.ID] = ws
	return nil
}

func (m *mockWorkspaceRepo) Delete(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.workspaces, id)
	return nil
}

func (m *mockWorkspaceRepo) List(ctx context.Context, query domain.PaginationQuery) (*domain.PaginatedResult[*domain.Workspace], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*domain.Workspace
	for _, ws := range m.workspaces {
		res = append(res, ws)
	}
	return &domain.PaginatedResult[*domain.Workspace]{
		Items:      res,
		TotalCount: int64(len(res)),
		Page:       1,
		PageSize:   len(res),
		TotalPages: 1,
	}, nil
}

// mockTeamCliKeyLookup implements contextresolver.TeamCliKeyLookup.
type mockTeamCliKeyLookup struct {
	mu       sync.RWMutex
	keys     map[string]*domain.TeamCliKey
	usedKeys map[uuid.UUID]time.Time
}

func newMockTeamCliKeyLookup() *mockTeamCliKeyLookup {
	return &mockTeamCliKeyLookup{
		keys:     make(map[string]*domain.TeamCliKey),
		usedKeys: make(map[uuid.UUID]time.Time),
	}
}

func (m *mockTeamCliKeyLookup) FindByHash(ctx context.Context, keyHash string) (*domain.TeamCliKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	k, ok := m.keys[keyHash]
	if !ok {
		return nil, nil
	}
	return k, nil
}

func (m *mockTeamCliKeyLookup) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usedKeys[id] = time.Now().UTC()
	return nil
}

// TestResolverStress_JWTSigningAndValidationMatrix tests a battery of valid, malformed,
// expired, and tampered JWT tokens to verify complete security boundary enforcement.
func TestResolverStress_JWTSigningAndValidationMatrix(t *testing.T) {
	secret := "correct-super-secret-key-that-is-at-least-32-chars-long"
	wrongSecret := "different-key-used-for-tamper-tests-at-least-32-chars"

	wsRepo := newMockWorkspaceRepo()
	activeWs := &domain.Workspace{
		BaseEntity: domain.BaseEntity{ID: uuid.New()},
		Name:       "Test Corp",
		Slug:       "test-corp",
		Status:     "ACTIVE",
		Tier:       "ENTERPRISE",
	}
	require.NoError(t, wsRepo.Create(context.Background(), activeWs))

	suspendedWs := &domain.Workspace{
		BaseEntity: domain.BaseEntity{ID: uuid.New()},
		Name:       "Suspended Corp",
		Slug:       "suspended-corp",
		Status:     "SUSPENDED",
		Tier:       "COMMUNITY",
	}
	require.NoError(t, wsRepo.Create(context.Background(), suspendedWs))

	resolver := contextresolver.NewResolver(secret, wsRepo, nil)

	// Subtest 1: Valid JWT with active workspace
	t.Run("Valid Token Active Workspace", func(t *testing.T) {
		userID := uuid.New()
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub":          userID.String(),
			"workspace_id": activeWs.ID.String(),
			"email":        "developer@testcorp.com",
			"role":         "ADMIN",
			"is_super":     false,
			"exp":          time.Now().Add(time.Hour).Unix(),
			"iat":          time.Now().Unix(),
		})
		tokenString, err := token.SignedString([]byte(secret))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/repos", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)

		user, tenant, err := resolver.ExtractFromRequest(req)
		require.NoError(t, err)
		require.NotNil(t, user)
		require.NotNil(t, tenant)
		assert.Equal(t, userID, user.UserID)
		assert.Equal(t, activeWs.ID, user.WorkspaceID)
		assert.Equal(t, "developer@testcorp.com", user.Email)
		assert.Equal(t, "ADMIN", user.Role)
		assert.Equal(t, "ENTERPRISE", tenant.Tier)
	})

	// Subtest 2: Suspended Workspace Rejection
	t.Run("Valid Token Suspended Workspace", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub":          uuid.New().String(),
			"workspace_id": suspendedWs.ID.String(),
			"email":        "user@suspended.com",
			"exp":          time.Now().Add(time.Hour).Unix(),
		})
		tokenString, err := token.SignedString([]byte(secret))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/repos", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)

		_, _, err = resolver.ExtractFromRequest(req)
		assert.ErrorIs(t, err, contextresolver.ErrWorkspaceSuspended)
	})

	// Subtest 3: Expired Token
	t.Run("Expired Token", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub":          uuid.New().String(),
			"workspace_id": activeWs.ID.String(),
			"exp":          time.Now().Add(-10 * time.Minute).Unix(),
		})
		tokenString, err := token.SignedString([]byte(secret))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/repos", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)

		_, _, err = resolver.ExtractFromRequest(req)
		assert.ErrorIs(t, err, contextresolver.ErrInvalidToken)
	})

	// Subtest 4: Tampered Signature (signed with wrong secret)
	t.Run("Tampered Signature", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub":          uuid.New().String(),
			"workspace_id": activeWs.ID.String(),
			"exp":          time.Now().Add(time.Hour).Unix(),
		})
		tokenString, err := token.SignedString([]byte(wrongSecret))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/repos", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)

		_, _, err = resolver.ExtractFromRequest(req)
		assert.ErrorIs(t, err, contextresolver.ErrInvalidToken)
	})

	// Subtest 5: Garbage Token Strings
	t.Run("Garbage String Tokens", func(t *testing.T) {
		malformed := []string{
			"not-even-a-token",
			"Bearer",
			"a.b",
			"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.badpayload.badsignature",
			"",
		}

		for _, bad := range malformed {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/repos", nil)
			if bad != "" {
				req.Header.Set("Authorization", "Bearer "+bad)
			}
			_, _, err := resolver.ExtractFromRequest(req)
			assert.Error(t, err)
		}
	})
}

// TestResolverStress_TeamCliKeyAuthentication tests ScanDrix CLI team key authentication
// via both 'x-team-key' and 'Bearer scandrix_*' headers with hashing and revocation checks.
func TestResolverStress_TeamCliKeyAuthentication(t *testing.T) {
	wsRepo := newMockWorkspaceRepo()
	wsID := uuid.New()
	teamID := uuid.New()
	ws := &domain.Workspace{
		BaseEntity: domain.BaseEntity{ID: wsID},
		Name:       "CLI Dev Team",
		Slug:       "cli-dev-team",
		Status:     "ACTIVE",
		Tier:       "PRO",
	}
	require.NoError(t, wsRepo.Create(context.Background(), ws))

	keyLookup := newMockTeamCliKeyLookup()
	rawKey := "scandrix_live_secret1234567890abcdef"
	hashed := crypto.HashToken(rawKey)

	keyRecord := &domain.TeamCliKey{
		TenantScopedEntity: domain.TenantScopedEntity{
			BaseEntity:  domain.BaseEntity{ID: uuid.New()},
			WorkspaceID: wsID,
		},
		TeamID:    teamID,
		Name:      "CI Runner Key",
		KeyHash:   hashed,
		KeyPrefix: "scandrix_live_",
	}
	keyLookup.keys[hashed] = keyRecord

	// Revoked key setup
	revokedRawKey := "scandrix_live_revoked_key_0000000000"
	revokedHashed := crypto.HashToken(revokedRawKey)
	revokedAt := time.Now().UTC()
	keyLookup.keys[revokedHashed] = &domain.TeamCliKey{
		TenantScopedEntity: domain.TenantScopedEntity{
			BaseEntity:  domain.BaseEntity{ID: uuid.New()},
			WorkspaceID: wsID,
		},
		TeamID:    teamID,
		KeyHash:   revokedHashed,
		KeyPrefix: "scandrix_live_",
		RevokedAt: &revokedAt,
	}

	resolver := contextresolver.NewResolver("secret", wsRepo, nil).WithTeamKeyLookup(keyLookup)

	// 1. Valid key in x-team-key header
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/scan", nil)
	req1.Header.Set("x-team-key", rawKey)
	user, tenant, err := resolver.ExtractFromRequest(req1)
	require.NoError(t, err)
	assert.Equal(t, wsID, tenant.WorkspaceID)
	assert.Equal(t, "TEAM_API_KEY", user.AuthMethod)
	assert.Contains(t, user.TeamIDs, teamID)

	// 2. Valid key in Authorization: Bearer header
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/scan", nil)
	req2.Header.Set("Authorization", "Bearer "+rawKey)
	user2, tenant2, err := resolver.ExtractFromRequest(req2)
	require.NoError(t, err)
	assert.Equal(t, wsID, tenant2.WorkspaceID)
	assert.Equal(t, "TEAM_API_KEY", user2.AuthMethod)

	// 3. Revoked key rejection
	reqRevoked := httptest.NewRequest(http.MethodPost, "/api/v1/scan", nil)
	reqRevoked.Header.Set("x-team-key", revokedRawKey)
	_, _, err = resolver.ExtractFromRequest(reqRevoked)
	assert.ErrorIs(t, err, contextresolver.ErrInvalidToken)

	// 4. Unknown team key rejection
	reqUnknown := httptest.NewRequest(http.MethodPost, "/api/v1/scan", nil)
	reqUnknown.Header.Set("x-team-key", "scandrix_live_unknown_nonexistent")
	_, _, err = resolver.ExtractFromRequest(reqUnknown)
	assert.ErrorIs(t, err, contextresolver.ErrInvalidToken)

	// 5. Non-scandrix key rejection
	reqInvalidPrefix := httptest.NewRequest(http.MethodPost, "/api/v1/scan", nil)
	reqInvalidPrefix.Header.Set("x-team-key", "invalid_prefix_key_123")
	_, _, err = resolver.ExtractFromRequest(reqInvalidPrefix)
	assert.ErrorIs(t, err, contextresolver.ErrInvalidToken)
}

// TestResolverStress_ContextInjectionAndConcurrency verifies thread-safe context injection
// and extraction under 50 concurrent goroutines executing simultaneous operations.
func TestResolverStress_ContextInjectionAndConcurrency(t *testing.T) {
	const concurrency = 50
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			wsID := uuid.New()
			userID := uuid.New()

			user := &contextresolver.UserIdentity{
				UserID:      userID,
				WorkspaceID: wsID,
				Email:       fmt.Sprintf("user%d@scandrix.internal", idx),
				Role:        "DEVELOPER",
				Permissions: map[string]bool{"reviews:create": true},
			}

			tenant := &contextresolver.TenantContext{
				WorkspaceID: wsID,
				Tier:        "ENTERPRISE",
				SpendLimit:  1000.0,
				SpendUsage:  float64(idx * 10),
			}

			ctx := context.Background()
			injectedCtx := contextresolver.WithContext(ctx, tenant, user)

			// Extract and verify
			retrievedUser, ok := contextresolver.GetUserIdentity(injectedCtx)
			assert.True(t, ok)
			assert.Equal(t, userID, retrievedUser.UserID)
			assert.Equal(t, user.Email, retrievedUser.Email)

			retrievedTenant, ok := contextresolver.GetTenantContext(injectedCtx)
			assert.True(t, ok)
			assert.Equal(t, wsID, retrievedTenant.WorkspaceID)
			assert.Equal(t, float64(idx*10), retrievedTenant.SpendUsage)

			// Verify permission enforcement
			assert.NoError(t, contextresolver.RequirePermission(injectedCtx, "reviews:create"))
			assert.ErrorIs(t, contextresolver.RequirePermission(injectedCtx, "admin:destroy_all"), contextresolver.ErrForbidden)
		}(i)
	}

	wg.Wait()
}

// TestResolverStress_EmptyContextExtraction confirms that calling getters on a bare context
// without injection properly returns false / ErrUnauthorized instead of panicking.
func TestResolverStress_EmptyContextExtraction(t *testing.T) {
	bareCtx := context.Background()

	user, ok := contextresolver.GetUserIdentity(bareCtx)
	assert.Nil(t, user)
	assert.False(t, ok)

	tenant, ok := contextresolver.GetTenantContext(bareCtx)
	assert.Nil(t, tenant)
	assert.False(t, ok)

	err := contextresolver.RequirePermission(bareCtx, "some:permission")
	assert.ErrorIs(t, err, contextresolver.ErrUnauthorized)
}

package contextresolver_test

import (
	"context"
	"net/http"
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

func TestContextResolverJWT(t *testing.T) {
	jwtSecret := "super_secret_jwt_signing_key_32_bytes!!"
	resolver := contextresolver.NewResolver(jwtSecret, nil, nil)

	userID := uuid.New()
	wsID := uuid.New()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":            userID.String(),
		"workspace_id":   wsID.String(),
		"email":          "engineer@scandrix.dev",
		"role":           "ADMIN",
		"is_super_admin": false,
		"exp":            time.Now().Add(time.Hour).Unix(),
	})

	tokenStr, err := token.SignedString([]byte(jwtSecret))
	require.NoError(t, err)

	req, err := http.NewRequest("GET", "/api/v1/reviews", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	user, tenant, err := resolver.ExtractFromRequest(req)
	require.NoError(t, err)
	assert.Equal(t, userID, user.UserID)
	assert.Equal(t, wsID, user.WorkspaceID)
	assert.Equal(t, "engineer@scandrix.dev", user.Email)
	assert.Equal(t, "ADMIN", user.Role)
	assert.Equal(t, wsID, tenant.WorkspaceID)

	// Context propagation test
	ctx := contextresolver.WithContext(context.Background(), tenant, user)
	extractedTenant, ok := contextresolver.GetTenantContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, wsID, extractedTenant.WorkspaceID)

	extractedUser, ok := contextresolver.GetUserIdentity(ctx)
	assert.True(t, ok)
	assert.Equal(t, userID, extractedUser.UserID)

	// Permission checks
	assert.NoError(t, contextresolver.RequirePermission(ctx, "review:create"))
}

func TestContextResolverTeamKey(t *testing.T) {
	resolver := contextresolver.NewResolver("secret", nil, nil)

	// 1. scandrix_ prefix
	req, err := http.NewRequest("POST", "/api/v1/reviews", nil)
	require.NoError(t, err)
	req.Header.Set("x-team-key", "scandrix_live_0123456789abcdef0123456789abcdef")

	user, _, err := resolver.ExtractFromRequest(req)
	require.NoError(t, err)
	assert.Equal(t, "CLI_AGENT", user.Role)
	assert.Equal(t, "TEAM_API_KEY", user.AuthMethod)
	assert.True(t, user.Permissions["review:create"])

	// 2. Bearer Authorization header with scandrix_ prefix
	req2, err := http.NewRequest("POST", "/api/v1/reviews", nil)
	require.NoError(t, err)
	req2.Header.Set("Authorization", "Bearer scandrix_live_abcdef0123456789abcdef0123456789")

	user2, _, err := resolver.ExtractFromRequest(req2)
	require.NoError(t, err)
	assert.Equal(t, "CLI_AGENT", user2.Role)

	// 3. invalid prefix rejected (including any other non-scandrix prefix)
	req3, err := http.NewRequest("POST", "/api/v1/reviews", nil)
	require.NoError(t, err)
	req3.Header.Set("x-team-key", "invalid_prefix_key")
	_, _, err = resolver.ExtractFromRequest(req3)
	assert.ErrorIs(t, err, contextresolver.ErrInvalidToken)

	req4, err := http.NewRequest("POST", "/api/v1/reviews", nil)
	require.NoError(t, err)
	req4.Header.Set("Authorization", "Bearer unauthorized_prefix_abcdef")
	_, _, err = resolver.ExtractFromRequest(req4)
	assert.ErrorIs(t, err, contextresolver.ErrInvalidToken)
}

type mockTeamKeyLookup struct {
	keyMap map[string]*domain.TeamCliKey
}

func (m *mockTeamKeyLookup) FindByHash(ctx context.Context, keyHash string) (*domain.TeamCliKey, error) {
	if key, ok := m.keyMap[keyHash]; ok {
		return key, nil
	}
	return nil, nil
}

func (m *mockTeamKeyLookup) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	return nil
}

func TestContextResolverTeamKeyWithDatabase(t *testing.T) {
	rawKey := "scandrix_live_secret1234567890abcdef"
	hashed := crypto.HashToken(rawKey)
	wsID := uuid.New()
	teamID := uuid.New()

	lookup := &mockTeamKeyLookup{
		keyMap: map[string]*domain.TeamCliKey{
			hashed: {
				TenantScopedEntity: domain.TenantScopedEntity{
					BaseEntity:  domain.BaseEntity{ID: uuid.New()},
					WorkspaceID: wsID,
				},
				TeamID:  teamID,
				Name:    "Test Key",
				KeyHash: hashed,
			},
		},
	}

	resolver := contextresolver.NewResolver("secret", nil, nil).WithTeamKeyLookup(lookup)

	req, err := http.NewRequest("POST", "/api/v1/reviews", nil)
	require.NoError(t, err)
	req.Header.Set("x-team-key", rawKey)

	user, tenant, err := resolver.ExtractFromRequest(req)
	require.NoError(t, err)
	assert.Equal(t, wsID, user.WorkspaceID)
	assert.Contains(t, user.TeamIDs, teamID)
	assert.Equal(t, wsID, tenant.WorkspaceID)

	// Revoked key check
	revokedAt := time.Now()
	lookup.keyMap[hashed].RevokedAt = &revokedAt
	_, _, err = resolver.ExtractFromRequest(req)
	assert.ErrorIs(t, err, contextresolver.ErrInvalidToken)
}


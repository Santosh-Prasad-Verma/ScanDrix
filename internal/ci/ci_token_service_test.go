// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package ci

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCITokenService_Lifecycle(t *testing.T) {
	secret := "test_ci_signing_secret_key_abcdef"
	svc := NewCITokenService(secret)

	orgID := uuid.New()
	repoName := "scandrix/backend"
	permissions := []string{"ci:review", "ci:checks"}

	// 1. Issue Token
	tokenStr, claims, err := svc.IssueToken(orgID, "repo-123", repoName, permissions, 1*time.Hour)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(tokenStr, "scandrix_ci_"))
	assert.Equal(t, repoName, claims.RepoFullName)

	// 2. Validate Valid Token
	validatedClaims, err := svc.ValidateToken(tokenStr, repoName)
	require.NoError(t, err)
	assert.Equal(t, claims.TokenID, validatedClaims.TokenID)
	assert.Equal(t, orgID, validatedClaims.OrganizationID)
	assert.Equal(t, permissions, validatedClaims.Permissions)

	// 3. Reject Wrong Repo Scope
	_, err = svc.ValidateToken(tokenStr, "other-org/other-repo")
	assert.ErrorIs(t, err, ErrScopeMismatch)

	// 4. Reject Tampered Signature
	tampered := tokenStr[:len(tokenStr)-4] + "dead"
	_, err = svc.ValidateToken(tampered, repoName)
	assert.ErrorIs(t, err, ErrTokenSignature)

	// 5. Reject Revoked Token
	svc.RevokeToken(claims.TokenID)
	_, err = svc.ValidateToken(tokenStr, repoName)
	assert.ErrorIs(t, err, ErrTokenRevoked)

	// 6. Reject Expired Token
	expiredToken, _, err := svc.IssueToken(orgID, "repo-123", repoName, permissions, -10*time.Minute)
	require.NoError(t, err)
	_, err = svc.ValidateToken(expiredToken, repoName)
	assert.ErrorIs(t, err, ErrTokenExpired)
}

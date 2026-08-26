package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/codehound/codehound/core/pkg/auth"
	"github.com/codehound/codehound/core/pkg/config"
	"github.com/codehound/codehound/core/pkg/database"
	sharedauth "github.com/codehound/codehound/shared/pkg/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestAuthMiddlewareWithAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, database.DefaultConfig(cfg.DatabaseURL))
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	// 1. Seed tenant and API key
	tenantID := uuid.New()
	keyObj, err := sharedauth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, plan, status)
		VALUES ($1, $2, 'Auth Test Tenant', 'ENTERPRISE', 'ACTIVE')
		ON CONFLICT (slug) DO NOTHING;
	`, tenantID, "auth-test-"+tenantID.String()[:8])
	if err != nil {
		t.Fatalf("failed to seed tenant: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO api_keys (tenant_id, name, key_prefix, key_hash, scopes)
		VALUES ($1, 'Test CI Key', $2, $3, '{"read:audit", "write:audit"}');
	`, tenantID, keyObj.KeyPrefix, keyObj.KeyHash)
	if err != nil {
		t.Fatalf("failed to seed api key: %v", err)
	}

	authenticator := auth.NewAuthenticator(pool)

	r := gin.New()
	r.Use(authenticator.RequireAuth())
	r.GET("/protected", func(c *gin.Context) {
		tid, ok := auth.GetTenantID(c)
		if !ok || tid != tenantID {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant id mismatch"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "authenticated"})
	})

	// Test 1: Valid Bearer Token
	reqValid, _ := http.NewRequest("GET", "/protected", nil)
	reqValid.Header.Set("Authorization", "Bearer "+keyObj.RawKey)
	wValid := httptest.NewRecorder()
	r.ServeHTTP(wValid, reqValid)

	if wValid.Code != http.StatusOK {
		t.Errorf("expected status 200 for valid key, got %d: %s", wValid.Code, wValid.Body.String())
	}

	// Test 2: Invalid Token
	reqInvalid, _ := http.NewRequest("GET", "/protected", nil)
	reqInvalid.Header.Set("Authorization", "Bearer ch_live_invalid_key_12345678901234567890123456789012")
	wInvalid := httptest.NewRecorder()
	r.ServeHTTP(wInvalid, reqInvalid)

	if wInvalid.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 for invalid key, got %d", wInvalid.Code)
	}
}

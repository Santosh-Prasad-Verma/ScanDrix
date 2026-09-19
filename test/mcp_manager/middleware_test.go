// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp_manager_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/scandrix/backend/internal/mcp/manager/api"
	"github.com/scandrix/backend/internal/mcp/manager/config"
)

func TestAuthGuardEdgeCases(t *testing.T) {
	jwtSecret := "test-jwt-secret-key-32-chars-long!"

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := r.Context().Value(api.OrgIDContextKey).(string)
		if !ok || orgID == "" {
			http.Error(w, "missing orgID", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(orgID))
	})

	guard := api.AuthGuard(jwtSecret)(nextHandler)

	// 1. Missing Authorization header -> 401
	req := httptest.NewRequest("GET", "/mcp/test", nil)
	rr := httptest.NewRecorder()
	guard.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing header, got %d", rr.Code)
	}

	// 2. Malformed Header (no Bearer prefix) -> 401
	req = httptest.NewRequest("GET", "/mcp/test", nil)
	req.Header.Set("Authorization", "Token abcxyz")
	rr = httptest.NewRecorder()
	guard.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for bad prefix, got %d", rr.Code)
	}

	// 3. Expired token -> 401
	expiredClaims := jwt.MapClaims{
		"sub":            "user-1",
		"organizationId": "org-exp",
		"exp":            time.Now().Add(-1 * time.Hour).Unix(),
	}
	expToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims).SignedString([]byte(jwtSecret))

	req = httptest.NewRequest("GET", "/mcp/test", nil)
	req.Header.Set("Authorization", "Bearer "+expToken)
	rr = httptest.NewRecorder()
	guard.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", rr.Code)
	}

	// 4. Token signed with different secret -> 401
	wrongKeyClaims := jwt.MapClaims{
		"sub":            "user-1",
		"organizationId": "org-wrong",
		"exp":            time.Now().Add(1 * time.Hour).Unix(),
	}
	wrongToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, wrongKeyClaims).SignedString([]byte("wrong-secret-key-123456789012345"))

	req = httptest.NewRequest("GET", "/mcp/test", nil)
	req.Header.Set("Authorization", "Bearer "+wrongToken)
	rr = httptest.NewRecorder()
	guard.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong signature key, got %d", rr.Code)
	}

	// 5. Valid token with organizationId claim -> 200
	validClaims := jwt.MapClaims{
		"sub":            "user-1",
		"organizationId": "org-success-123",
		"exp":            time.Now().Add(1 * time.Hour).Unix(),
	}
	validToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims).SignedString([]byte(jwtSecret))

	req = httptest.NewRequest("GET", "/mcp/test", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	rr = httptest.NewRecorder()
	guard.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != "org-success-123" {
		t.Errorf("expected 200 with org-success-123, got code %d body %s", rr.Code, rr.Body.String())
	}
}

func TestDocsBasicAuthScenarios(t *testing.T) {
	cfg := &config.MCPManagerConfig{
		DocsEnabled: true,
		DocsUser:    "admin",
		DocsPass:    "secretpass123",
	}

	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("docs content"))
	})

	authMiddleware := api.DocsBasicAuth(cfg.DocsUser, cfg.DocsPass)(dummyHandler)

	// 1. Missing Auth -> 401
	req := httptest.NewRequest("GET", "/docs", nil)
	rr := httptest.NewRecorder()
	authMiddleware.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing basic auth, got %d", rr.Code)
	}

	// 2. Wrong user -> 401
	badUserHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("wronguser:secretpass123"))
	req = httptest.NewRequest("GET", "/docs", nil)
	req.Header.Set("Authorization", badUserHeader)
	rr = httptest.NewRecorder()
	authMiddleware.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong user, got %d", rr.Code)
	}

	// 3. Wrong password -> 401
	badPassHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:wrongpass"))
	req = httptest.NewRequest("GET", "/docs", nil)
	req.Header.Set("Authorization", badPassHeader)
	rr = httptest.NewRecorder()
	authMiddleware.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong pass, got %d", rr.Code)
	}

	// 4. Valid credentials -> 200
	validHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:secretpass123"))
	req = httptest.NewRequest("GET", "/docs", nil)
	req.Header.Set("Authorization", validHeader)
	rr = httptest.NewRecorder()
	authMiddleware.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != "docs content" {
		t.Errorf("expected 200 docs content, got code %d body %s", rr.Code, rr.Body.String())
	}
}

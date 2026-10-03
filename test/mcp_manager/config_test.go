// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp_manager_test

import (
	"os"
	"testing"

	"github.com/scandrix/backend/internal/mcp/manager/config"
)

func TestConfigDefaultsAndOverrides(t *testing.T) {
	// Set custom environment variables
	os.Setenv("MCP_MANAGER_PORT", "4500")
	// AUDIT_REMEDIATION.md F-46: Load() no longer invents a DATABASE_URL. It
	// used to fall back to postgres://postgres:postgres@..., a superuser that
	// bypasses row-level security, so this test has to supply a real
	// least-privilege DSN of its own.
	os.Setenv("DATABASE_URL", "postgres://scandrix_runtime:test@127.0.0.1:5432/scandrix?sslmode=disable")
	testSecret := generateRandomTestKey()
	os.Setenv("MCP_MANAGER_SECRET", testSecret)
	os.Setenv("JWT_SECRET", generateRandomTestKey())
	os.Setenv("API_MCP_MANAGER_CORS_ORIGINS", "http://localhost:3000, https://app.scandrix.io")
	os.Setenv("MCP_DOCS_USER", "secureuser")
	os.Setenv("MCP_DOCS_PASSWORD", "securepass")
	os.Setenv("SCANDRIX_MCP_SERVER_URL", "https://custom-mcp.scandrix.io")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("MCP_MANAGER_PORT")
		os.Unsetenv("MCP_MANAGER_SECRET")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("API_MCP_MANAGER_CORS_ORIGINS")
		os.Unsetenv("MCP_DOCS_USER")
		os.Unsetenv("MCP_DOCS_PASSWORD")
		os.Unsetenv("SCANDRIX_MCP_SERVER_URL")
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed loading config: %v", err)
	}

	if cfg.Port != 4500 {
		t.Errorf("expected port 4500, got %d", cfg.Port)
	}
	if cfg.EncryptionSecret != testSecret {
		t.Errorf("expected custom encryption secret, got %s", cfg.EncryptionSecret)
	}
	if len(cfg.CORSOrigins) != 2 || cfg.CORSOrigins[0] != "http://localhost:3000" {
		t.Errorf("CORS origins not parsed properly: %v", cfg.CORSOrigins)
	}
	if !cfg.DocsEnabled || cfg.DocsUser != "secureuser" || cfg.DocsPass != "securepass" {
		t.Errorf("docs auth configuration mismatch: enabled=%v user=%s", cfg.DocsEnabled, cfg.DocsUser)
	}
	if cfg.ServerBaseURL != "https://custom-mcp.scandrix.io" {
		t.Errorf("expected serverBaseURL https://custom-mcp.scandrix.io, got %s", cfg.ServerBaseURL)
	}
}

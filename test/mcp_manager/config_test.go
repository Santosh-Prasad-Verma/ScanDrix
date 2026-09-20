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
	testSecret := generateRandomTestKey()
	os.Setenv("MCP_MANAGER_SECRET", testSecret)
	os.Setenv("JWT_SECRET", generateRandomTestKey())
	os.Setenv("API_MCP_MANAGER_CORS_ORIGINS", "http://localhost:3000, https://app.scandrix.io")
	os.Setenv("MCP_DOCS_USER", "secureuser")
	os.Setenv("MCP_DOCS_PASSWORD", "securepass")
	os.Setenv("SCANDRIX_MCP_SERVER_URL", "https://custom-mcp.scandrix.io")
	defer func() {
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

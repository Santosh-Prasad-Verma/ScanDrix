// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package config

import (
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

//go:embed integration-descriptions.json
var EmbeddedIntegrationDescriptions []byte

//go:embed managed-mcp-servers.json
var EmbeddedManagedMCPServers []byte

// MCPManagerConfig encapsulates runtime options for the MCP Manager microservice.
type MCPManagerConfig struct {
	Port             int
	DatabaseURL      string
	JWTSecret        string
	EncryptionSecret string
	RedirectURI      string
	CORSOrigins      []string
	ServerBaseURL    string
	DocsEnabled      bool
	DocsUser         string
	DocsPass         string
	DocsPath         string
	DocsSpecPath     string
}

// Load reads and validates configuration from environment variables.
func Load() (*MCPManagerConfig, error) {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	port := 3101
	if portStr := os.Getenv("API_MCP_MANAGER_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			port = p
		}
	} else if portStr := os.Getenv("MCP_MANAGER_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			port = p
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	// Fail closed. This previously fell back to
	// postgres://postgres:postgres@127.0.0.1:5432/scandrix?sslmode=disable,
	// a superuser with a trivial password, so a missing DATABASE_URL silently
	// produced a connection with full database rights. The same function
	// correctly refuses to guess the encryption and JWT secrets a few lines
	// below; a database credential is no different (AUDIT_REMEDIATION.md F-46).
	if dbURL == "" {
		return nil, errors.New("DATABASE_URL is required and cannot be empty")
	}
	if err := rejectSuperuserDSN(dbURL); err != nil {
		return nil, err
	}

	jwtSecret := os.Getenv("API_MCP_MANAGER_JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = os.Getenv("JWT_SECRET")
	}

	encryptionSecret := os.Getenv("API_MCP_MANAGER_ENCRYPTION_SECRET")
	if encryptionSecret == "" {
		encryptionSecret = os.Getenv("MCP_MANAGER_SECRET")
	}
	if encryptionSecret == "" {
		encryptionSecret = os.Getenv("KMS_MASTER_KEY")
	}
	if encryptionSecret == "" {
		return nil, errors.New("MCP_MANAGER_SECRET (or API_MCP_MANAGER_ENCRYPTION_SECRET / KMS_MASTER_KEY) is required and cannot be empty")
	}

	if jwtSecret == "" {
		return nil, errors.New("JWT_SECRET (or API_MCP_MANAGER_JWT_SECRET) is required and cannot be empty")
	}

	redirectURI := os.Getenv("API_MCP_MANAGER_REDIRECT_URI")
	if redirectURI == "" {
		redirectURI = os.Getenv("MCP_OAUTH_REDIRECT_URI")
	}
	if redirectURI == "" {
		redirectURI = "http://localhost:3000/setup/mcp/oauth"
	}

	corsOriginsRaw := os.Getenv("API_MCP_MANAGER_CORS_ORIGINS")
	var corsOrigins []string
	if corsOriginsRaw != "" {
		for _, o := range strings.Split(corsOriginsRaw, ",") {
			trimmed := strings.TrimSpace(o)
			if trimmed != "" {
				corsOrigins = append(corsOrigins, trimmed)
			}
		}
	}

	serverURL := os.Getenv("SCANDRIX_SERVER_URL")
	if serverURL == "" {
		serverURL = os.Getenv("API_SCANDRIX_MCP_SERVER_URL")
	}
	if serverURL == "" {
		serverURL = os.Getenv("SCANDRIX_MCP_SERVER_URL")
	}
	if serverURL == "" {
		serverURL = "http://localhost:3001"
	}

	docsEnabled := strings.EqualFold(os.Getenv("API_DOCS_ENABLED"), "true") || os.Getenv("MCP_DOCS_USER") != ""
	docsUser := os.Getenv("API_DOCS_BASIC_USER")
	if docsUser == "" {
		docsUser = os.Getenv("MCP_DOCS_USER")
	}
	docsPass := os.Getenv("API_DOCS_BASIC_PASS")
	if docsPass == "" {
		docsPass = os.Getenv("MCP_DOCS_PASSWORD")
	}
	docsPath := os.Getenv("API_DOCS_PATH")
	if docsPath == "" {
		docsPath = "/docs"
	}
	docsSpecPath := os.Getenv("API_DOCS_SPEC_PATH")
	if docsSpecPath == "" {
		docsSpecPath = "/openapi.json"
	}

	return &MCPManagerConfig{
		Port:             port,
		DatabaseURL:      dbURL,
		JWTSecret:        jwtSecret,
		EncryptionSecret: encryptionSecret,
		RedirectURI:      redirectURI,
		CORSOrigins:      corsOrigins,
		ServerBaseURL:    serverURL,
		DocsEnabled:      docsEnabled && docsUser != "" && docsPass != "",
		DocsUser:         docsUser,
		DocsPass:         docsPass,
		DocsPath:         docsPath,
		DocsSpecPath:     docsSpecPath,
	}, nil
}

// rejectSuperuserDSN refuses database URLs that name a superuser account.
//
// AUDIT_REMEDIATION.md F-46. The MCP manager runs with whatever DATABASE_URL
// it is given, and the previous fallback pointed it at `postgres:postgres`.
// A superuser bypasses every row-level security policy in the schema, so a
// single misconfigured environment turned this binary into a way to read and
// write every tenant's data regardless of tenant context.
//
// The check is deliberately about the *role*, not about specific passwords: a
// password that leaks in a log is recoverable by rotation, whereas a superuser
// role silently defeats isolation.
func rejectSuperuserDSN(dsn string) error {
	u, err := url.Parse(dsn)
	if err != nil {
		// Not parseable as a URL: let the driver produce the real error rather
		// than guessing here.
		return nil
	}
	if u.User == nil {
		return nil
	}
	user := u.User.Username()
	switch strings.ToLower(user) {
	case "postgres", "root", "admin", "superuser", "scandrix_app":
		return fmt.Errorf(
			"DATABASE_URL must not use a superuser account (found %q): it bypasses row-level "+
				"security and would grant this process unrestricted access to every tenant. "+
				"Use a least-privilege application role such as scandrix_runtime", user)
	}
	return nil
}

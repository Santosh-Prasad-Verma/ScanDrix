// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package config

import (
	_ "embed"
	"errors"
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
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@127.0.0.1:5432/scandrix?sslmode=disable"
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

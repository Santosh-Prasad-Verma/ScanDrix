// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Tools Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ─── UTILS TESTS ─────────────────────────────────────────────────────────────

func TestUtils_Normalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"GitHubMCP", "github"},
		{"  GitLab-MCP  ", "gitlab"},
		{"azure_repos_mcp", "azurerepos"},
		{"mcp", "mcp"}, // length <= 3, not stripped
		{"", ""},
		{"Jira", "jira"},
	}

	for _, tt := range tests {
		got := NormalizeProviderKey(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeProviderKey(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}

	toolTests := []struct {
		input    string
		expected string
	}{
		{"read_file", "readfile"},
		{"Write-File", "writefile"},
		{"", ""},
	}

	for _, tt := range toolTests {
		got := NormalizeToolKey(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeToolKey(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestUtils_AliasResolution(t *testing.T) {
	aliasMap := make(map[string]string)
	RegisterProviderAliases(aliasMap, "GitHub", []string{"gh", "github-server", "git-hub"})

	if got := ResolveCanonicalProvider(aliasMap, "gh"); got != "GitHub" {
		t.Errorf("expected 'GitHub', got %q", got)
	}
	if got := ResolveCanonicalProvider(aliasMap, "githubmcp"); got != "GitHub" {
		t.Errorf("expected 'GitHub' for normalized alias, got %q", got)
	}

	toolAliases := make(map[string]map[string]string)
	providerAliases := map[string]string{"gh": "GitHub"}
	RegisterToolAliases(toolAliases, "GitHub", "read_file")

	resolved := ResolveCanonicalTool(toolAliases, providerAliases, "gh", "READ_FILE")
	if resolved != "read_file" {
		t.Errorf("expected 'read_file', got %q", resolved)
	}
}

// ─── JWT VALIDATOR TESTS ─────────────────────────────────────────────────────

func TestJWTValidator_Validation(t *testing.T) {
	secret := []byte("super-secure-production-test-key-32-bytes")
	audience := "mcp://api.scandrix.io/tenant-123"

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": "scandrix-auth",
		"sub": "user-456",
		"aud": audience,
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"nbf": time.Now().Add(-1 * time.Minute).Unix(),
		"iat": time.Now().Unix(),
	})
	tokenStr, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("failed to sign test JWT: %v", err)
	}

	validator := NewJWTValidator(JWTOptions{
		SecretOrPublicKey: secret,
		Issuer:            "scandrix-auth",
		Audience:          []string{audience},
		Algorithms:        []string{"HS256"},
	})

	if !validator.IsValidJWTFormat(tokenStr) {
		t.Fatalf("token string should be recognized as valid JWT format")
	}

	claims, err := validator.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.Subject != "user-456" {
		t.Errorf("expected subject 'user-456', got %q", claims.Subject)
	}

	if !validator.ValidateAudience(tokenStr, audience) {
		t.Errorf("ValidateAudience should return true for %s", audience)
	}
	if validator.ValidateAudience(tokenStr, "wrong-audience") {
		t.Errorf("ValidateAudience should return false for wrong audience")
	}

	if validator.IsExpired(tokenStr) {
		t.Errorf("token should not be expired")
	}
}

// ─── SESSION MANAGER TESTS ───────────────────────────────────────────────────

func TestSessionManager_Lifecycle(t *testing.T) {
	sm := NewSessionManager(500 * time.Millisecond)
	defer sm.Destroy()

	tenantID := "tenant-alpha"
	userID := "user-bravo"

	sessID := sm.CreateSession(tenantID, userID)
	if sessID == "" {
		t.Fatalf("expected non-empty session ID")
	}

	// Validate matching user
	if !sm.ValidateSession(sessID, userID) {
		t.Errorf("ValidateSession should succeed for correct user binding")
	}

	// Validate mismatched user (anti-session hijacking)
	if sm.ValidateSession(sessID, "impostor-user") {
		t.Errorf("ValidateSession must fail for hijacked user context")
	}

	// Metadata management
	sm.UpdateSessionMetadata(sessID, map[string]any{"role": "reviewer"}, userID)
	meta := sm.GetSessionMetadata(sessID, userID)
	if meta == nil || meta["role"] != "reviewer" {
		t.Errorf("expected metadata role 'reviewer', got %v", meta)
	}

	// Destroy session
	sm.DestroySession(sessID, userID)
	if sm.ValidateSession(sessID, userID) {
		t.Errorf("ValidateSession must fail after explicit destruction")
	}
}

// ─── SCHEMA VALIDATOR TESTS ──────────────────────────────────────────────────

func TestSchemaValidator_Validation(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"username": map[string]any{
				"type":      "string",
				"minLength": float64(3),
				"maxLength": float64(20),
			},
			"email": map[string]any{
				"type":   "string",
				"format": "email",
			},
			"score": map[string]any{
				"type":    "integer",
				"minimum": float64(0),
				"maximum": float64(100),
			},
			"isActive": map[string]any{
				"type": "boolean",
			},
			"tags": map[string]any{
				"type":        "array",
				"minItems":    float64(1),
				"uniqueItems": true,
			},
		},
		"required": []string{"username", "email"},
	}

	if !ValidateMCPSchema(schema) {
		t.Fatalf("schema should be recognized as valid MCP schema")
	}

	// Valid input
	validInput := map[string]any{
		"username": "alice",
		"email":    "alice@example.com",
		"score":    95,
		"isActive": true,
		"tags":     []any{"admin", "reviewer"},
	}
	if err := ValidateInputAgainstSchema(schema, validInput); err != nil {
		t.Errorf("expected valid input to pass, got error: %v", err)
	}

	// Missing required property
	missingInput := map[string]any{
		"username": "bob",
	}
	if err := ValidateInputAgainstSchema(schema, missingInput); err == nil {
		t.Errorf("expected error for missing required 'email', got nil")
	}

	// Invalid email format
	invalidEmail := map[string]any{
		"username": "charlie",
		"email":    "not-an-email",
	}
	if err := ValidateInputAgainstSchema(schema, invalidEmail); err == nil {
		t.Errorf("expected error for invalid email, got nil")
	}
}

// ─── TOOL CONVERSION & COLLISION DETECTION ───────────────────────────────────

func TestTools_ConversionAndConflicts(t *testing.T) {
	tools := []MCPToolRawWithServer{
		{
			MCPToolRaw: MCPToolRaw{
				Name:        "analyze_code",
				Description: "Performs AST and security checks",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"filePath": map[string]any{"type": "string", "required": true},
					},
				},
			},
			ServerName: "server-1",
		},
		{
			MCPToolRaw: MCPToolRaw{
				Name:        "analyze_code", // duplicate name on another server!
				Description: "Conflicting tool",
			},
			ServerName: "server-2",
		},
	}

	// Conflict detection test
	_, err := MCPToolsToEngineTools(tools)
	if err == nil {
		t.Errorf("expected conflict error for tool 'analyze_code' across server-1 and server-2, got nil")
	}

	// Path traversal in tool name check
	badTool := MCPToolRawWithServer{
		MCPToolRaw: MCPToolRaw{
			Name: "dangerous/../tool",
		},
		ServerName: "server-1",
	}
	_, err = MCPToolToEngineTool(badTool)
	if err == nil {
		t.Errorf("expected error for path traversal tool name, got nil")
	}
}

// ─── SECURITY MANAGER & CLIENT INTEGRATION TESTS ─────────────────────────────

func TestSecurityManager_FileAccess(t *testing.T) {
	sm := NewSecurityManager(&SecurityPolicy{
		PreventPathTraversal: true,
		AllowedURIPatterns: []*regexp.Regexp{
			regexp.MustCompile(`^file:///workspace/`),
		},
		BlockedURIPatterns: []*regexp.Regexp{
			regexp.MustCompile(`.*\.env.*`),
		},
	}, &TenantContext{
		AllowedRoots: []string{"file:///workspace/src"},
		Permissions:  []string{"tools:*", "resources:read"},
	})

	// Allowed URI
	if err := sm.ValidateFileAccess("file:///workspace/src/main.go"); err != nil {
		t.Errorf("expected valid URI to be allowed, got: %v", err)
	}

	// Path traversal sequence blocked
	if err := sm.ValidateFileAccess("file:///workspace/src/../secrets"); err == nil {
		t.Errorf("expected path traversal sequence to be blocked")
	}

	// Blocked URI pattern (.env)
	if err := sm.ValidateFileAccess("file:///workspace/src/.env.local"); err == nil {
		t.Errorf("expected blocked pattern to be rejected")
	}

	// Outside allowed tenant roots
	if err := sm.ValidateFileAccess("file:///workspace/other/file.go"); err == nil {
		t.Errorf("expected URI outside tenant roots to be rejected")
	}

	// Permissions
	if !sm.CheckPermission("tools:call") {
		t.Errorf("permission 'tools:call' should be granted")
	}
	if sm.CheckPermission("admin:delete") {
		t.Errorf("permission 'admin:delete' should not be granted")
	}
}

// ─── HTTP MCP SERVER & ADAPTER END-TO-END TEST ───────────────────────────────

func TestMCPAdapter_EndToEnd(t *testing.T) {
	// 1. Create a mock upstream MCP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)

		method, _ := req["method"].(string)
		id := req["id"]

		w.Header().Set("Content-Type", "application/json")

		switch method {
		case "initialize":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"protocolVersion": "2025-06-18",
					"capabilities": map[string]any{
						"tools": map[string]any{"listChanged": true},
					},
					"serverInfo": map[string]any{"name": "test-upstream", "version": "1.0"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case "tools/list":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []any{
						map[string]any{
							"name":        "run_security_scan",
							"description": "Runs static security checks",
							"inputSchema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"target": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case "tools/call":
			params, _ := req["params"].(map[string]any)
			args, _ := params["arguments"].(map[string]any)
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"isError": false,
					"content": []any{
						map[string]any{
							"type": "text",
							"text": "Security scan completed for " + args["target"].(string),
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	// 2. Configure adapter
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	adapter := CreateMCPAdapter(MCPAdapterConfig{
		Servers: []MCPServerConfig{
			{
				Name:    "upstream-scanner",
				Type:    TransportHTTP,
				URL:     mockServer.URL,
				Timeout: 2 * time.Second,
			},
		},
	})

	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("adapter.Connect failed: %v", err)
	}
	defer func() { _ = adapter.Disconnect(ctx) }()

	// 3. Query tools
	tools, err := adapter.GetTools(ctx)
	if err != nil {
		t.Fatalf("adapter.GetTools failed: %v", err)
	}

	if len(tools) != 1 || tools[0].Name != "run_security_scan" {
		t.Fatalf("expected tool 'run_security_scan', got %v", tools)
	}

	// 4. Has tool
	has, err := adapter.HasTool(ctx, "run_security_scan")
	if err != nil || !has {
		t.Errorf("HasTool('run_security_scan') returned %v, %v", has, err)
	}

	// 5. Execute tool
	execRes, err := adapter.ExecuteTool(ctx, "run_security_scan", map[string]any{"target": "internal/auth"})
	if err != nil {
		t.Fatalf("ExecuteTool failed: %v", err)
	}

	resMap, ok := execRes.(map[string]any)
	if !ok {
		t.Fatalf("expected map result from execution, got %T", execRes)
	}
	contentList := resMap["content"].([]any)
	firstItem := contentList[0].(map[string]any)
	if !strings.Contains(firstItem["text"].(string), "Security scan completed") {
		t.Errorf("unexpected execution output: %v", firstItem)
	}
}

// ─── METADATA SERVICE TESTS ──────────────────────────────────────────────────

func TestMCPToolMetadataService_Resolution(t *testing.T) {
	svc := NewMCPToolMetadataService()

	metaMap := map[string]*MCPToolMetadata{
		"GitHub|read_file": {
			RequiredArgs: []string{"path", "repo"},
			InputSchema: map[string]any{
				"type": "object",
			},
		},
	}

	// Direct match
	meta := svc.GetMetadataForTool(metaMap, "GitHub", "read_file")
	if meta == nil || len(meta.RequiredArgs) != 2 {
		t.Fatalf("expected direct metadata match, got %v", meta)
	}

	// Normalized match (case & suffix insensitive)
	metaNorm := svc.GetMetadataForTool(metaMap, "githubmcp", "READ_FILE")
	if metaNorm == nil || len(metaNorm.RequiredArgs) != 2 {
		t.Fatalf("expected normalized metadata match, got %v", metaNorm)
	}
}

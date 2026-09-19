// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp_manager_test

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/scandrix/backend/internal/mcp/manager/oauth"
)

func TestPKCEGeneration(t *testing.T) {
	verifier, challenge, err := oauth.GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE failed: %v", err)
	}

	if verifier == "" || challenge == "" {
		t.Fatalf("Verifier and challenge must not be empty")
	}

	// Verify SHA-256 S256 hash
	h := sha256.Sum256([]byte(verifier))
	expectedChallenge := base64.RawURLEncoding.EncodeToString(h[:])

	if challenge != expectedChallenge {
		t.Fatalf("Challenge did not match expected SHA-256 base64url: expected %s, got %s", expectedChallenge, challenge)
	}
}

func TestCanonicalResourceURI(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"https://mcp.linear.app/", "https://mcp.linear.app"},
		{"https://MCP.SENTRY.DEV/mcp/", "https://mcp.sentry.dev/mcp"},
		{"https://api.atlassian.com:443/v1/mcp", "https://api.atlassian.com:443/v1/mcp"},
	}

	for _, c := range cases {
		res, err := oauth.GetCanonicalResourceURI(c.input)
		if err != nil {
			t.Fatalf("Failed canonicalizing %s: %v", c.input, err)
		}
		if res != c.expected {
			t.Fatalf("For %s expected %s, got %s", c.input, c.expected, res)
		}
	}
}

func TestBuildWellKnownURL(t *testing.T) {
	u, err := oauth.BuildWellKnownURL("https://mcp.sentry.dev/mcp", "oauth-authorization-server")
	if err != nil {
		t.Fatalf("BuildWellKnownURL failed: %v", err)
	}
	expected := "https://mcp.sentry.dev/.well-known/oauth-authorization-server/mcp"
	if u != expected {
		t.Fatalf("Expected %s, got %s", expected, u)
	}
}

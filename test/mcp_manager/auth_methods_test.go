// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp_manager_test

import (
	"testing"

	"github.com/scandrix/backend/internal/mcp/manager/models"
	"github.com/scandrix/backend/internal/mcp/manager/providers/scandrixmcp"
)

func TestNormalizeAuthMethods(t *testing.T) {
	// 1. Empty methods should default to none
	cfgEmpty := scandrixmcp.RawManagedConfig{
		ID:          "test-none",
		AuthMethods: []models.PublicAuthMethod{},
	}
	normalized := scandrixmcp.NormalizeAuthMethods(cfgEmpty)
	if len(normalized) != 1 {
		t.Fatalf("Expected 1 method, got %d", len(normalized))
	}
	if normalized[0].Type != models.AuthTypeNone || !normalized[0].Default {
		t.Fatalf("Expected default method of type 'none'")
	}

	// 2. Multi-methods should maintain default flag
	cfgMulti := scandrixmcp.RawManagedConfig{
		ID: "jira-test",
		AuthMethods: []models.PublicAuthMethod{
			{ID: "oauth", Type: models.AuthTypeOAuth2, Default: true},
			{ID: "token", Type: models.AuthTypeBasic, Default: false},
		},
	}
	normalizedMulti := scandrixmcp.NormalizeAuthMethods(cfgMulti)
	if len(normalizedMulti) != 2 {
		t.Fatalf("Expected 2 methods, got %d", len(normalizedMulti))
	}
	if !normalizedMulti[0].Default || normalizedMulti[1].Default {
		t.Fatalf("Expected first method to be default")
	}
}

func TestGetAuthMethod(t *testing.T) {
	methods := []models.PublicAuthMethod{
		{ID: "oauth", Type: models.AuthTypeOAuth2, Default: true},
		{ID: "token", Type: models.AuthTypeBasic, Default: false},
	}

	// Lookup by explicit ID
	m := scandrixmcp.GetAuthMethod(methods, "token")
	if m == nil || m.ID != "token" {
		t.Fatalf("Expected 'token' method, got %v", m)
	}

	// Lookup by empty ID should return default
	def := scandrixmcp.GetAuthMethod(methods, "")
	if def == nil || def.ID != "oauth" {
		t.Fatalf("Expected default 'oauth' method, got %v", def)
	}

	// Nonexistent ID should return nil
	none := scandrixmcp.GetAuthMethod(methods, "unknown")
	if none != nil {
		t.Fatalf("Expected nil for unknown ID, got %v", none)
	}
}

func TestValidateTokenSubmission(t *testing.T) {
	basicMethod := &models.PublicAuthMethod{
		ID:      "token",
		Type:    models.AuthTypeBasic,
		Default: false,
		UserFields: []models.ManagedAuthUserField{
			{Name: "email", Label: "Email", Required: true, Secret: false},
			{Name: "apiToken", Label: "Token", Required: true, Secret: true},
			{Name: "cloudId", Label: "Cloud ID", Required: false, Secret: false},
		},
	}

	// Valid submission
	validDTO := models.ConnectTokenDTO{
		AuthMethod: "token",
		Secret:     "secret_api_token_123",
		Fields: map[string]string{
			"email":   "engineer@company.com",
			"cloudId": "cloud-uuid-456",
		},
	}
	cred, err := scandrixmcp.ValidateTokenSubmission(basicMethod, validDTO)
	if err != nil {
		t.Fatalf("Expected valid submission to succeed, got: %v", err)
	}
	if cred.Secret != "secret_api_token_123" {
		t.Fatalf("Secret mismatch")
	}
	if cred.Fields["email"] != "engineer@company.com" || cred.Fields["cloudId"] != "cloud-uuid-456" {
		t.Fatalf("Fields mismatch: %v", cred.Fields)
	}

	// Missing secret
	missingSecretDTO := models.ConnectTokenDTO{
		AuthMethod: "token",
		Secret:     "",
		Fields:     map[string]string{"email": "engineer@company.com"},
	}
	_, err = scandrixmcp.ValidateTokenSubmission(basicMethod, missingSecretDTO)
	if err == nil {
		t.Fatalf("Expected error for missing secret, got nil")
	}

	// Missing required non-secret field (email)
	missingEmailDTO := models.ConnectTokenDTO{
		AuthMethod: "token",
		Secret:     "secret_123",
		Fields:     map[string]string{},
	}
	_, err = scandrixmcp.ValidateTokenSubmission(basicMethod, missingEmailDTO)
	if err == nil {
		t.Fatalf("Expected error for missing required field, got nil")
	}

	// Rejects OAuth method
	oauthMethod := &models.PublicAuthMethod{
		ID:   "oauth",
		Type: models.AuthTypeOAuth2,
	}
	_, err = scandrixmcp.ValidateTokenSubmission(oauthMethod, validDTO)
	if err == nil {
		t.Fatalf("Expected error submitting token to OAuth method, got nil")
	}
}

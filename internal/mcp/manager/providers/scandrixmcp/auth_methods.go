// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package scandrixmcp

import (
	"os"

	"github.com/scandrix/backend/internal/mcp/manager/models"
)

// NormalizeAuthMethods ensures every managed integration has at least one default auth method.
func NormalizeAuthMethods(cfg RawManagedConfig) []models.PublicAuthMethod {
	methods := cfg.AuthMethods
	if len(methods) == 0 {
		return []models.PublicAuthMethod{
			{ID: "none", Type: models.AuthTypeNone, Default: true},
		}
	}

	hasDefault := false
	for i := range methods {
		if methods[i].Default {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		methods[0].Default = true
	}

	return methods
}

// GetAuthMethod retrieves an authentication method by ID, or returns the default.
func GetAuthMethod(methods []models.PublicAuthMethod, id string) *models.PublicAuthMethod {
	if id != "" {
		for i := range methods {
			if methods[i].ID == id {
				return &methods[i]
			}
		}
		return nil
	}

	for i := range methods {
		if methods[i].Default {
			return &methods[i]
		}
	}
	if len(methods) > 0 {
		return &methods[0]
	}
	return nil
}

// ResolveOAuthCredentials retrieves client ID and Secret from environment when specified.
func ResolveOAuthCredentials(clientIDEnv, clientSecretEnv string) (string, string) {
	var id, secret string
	if clientIDEnv != "" {
		id = os.Getenv(clientIDEnv)
	}
	if clientSecretEnv != "" {
		secret = os.Getenv(clientSecretEnv)
	}
	return id, secret
}

// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package scandrixmcp

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/mcp/manager/models"
)

// ValidateTokenSubmission validates a user's static token submission and extracts a clean credential.
func ValidateTokenSubmission(method *models.PublicAuthMethod, submission models.ConnectTokenDTO) (*models.ManagedTokenCredential, error) {
	if method.Type == models.AuthTypeOAuth2 || method.Type == models.AuthTypeNone {
		return nil, fmt.Errorf("auth method '%s' does not accept a submitted token", method.ID)
	}

	if strings.TrimSpace(submission.Secret) == "" {
		secretName := "secret"
		for _, f := range method.UserFields {
			if f.Secret {
				secretName = f.Name
				break
			}
		}
		return nil, fmt.Errorf("missing required secret field: %s", secretName)
	}

	submittedFields := submission.Fields
	if submittedFields == nil {
		submittedFields = make(map[string]string)
	}

	cleanedFields := make(map[string]string)
	missing := make([]string, 0)

	for _, f := range method.UserFields {
		if f.Secret {
			continue // Secret is handled separately in submission.Secret
		}
		val, exists := submittedFields[f.Name]
		if !exists || strings.TrimSpace(val) == "" {
			if f.Required {
				missing = append(missing, f.Name)
			}
			continue
		}
		cleanedFields[f.Name] = val
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required field(s): %s", strings.Join(missing, ", "))
	}

	return &models.ManagedTokenCredential{
		Kind:         "managed-token",
		AuthMethodID: method.ID,
		AuthType:     method.Type,
		Secret:       submission.Secret,
		Fields:       cleanedFields,
	}, nil
}

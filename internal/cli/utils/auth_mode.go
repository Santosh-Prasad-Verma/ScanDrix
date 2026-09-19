// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"os"
	"strings"
)

// AuthMode identifies the active authentication mechanism.
type AuthMode string

const (
	AuthModeToken    AuthMode = "token"
	AuthModeTeamKey  AuthMode = "team-key"
	AuthModeLoggedIn AuthMode = "logged-in"
	AuthModeTrial    AuthMode = "trial"
)

// AuthSource identifies where credentials originated.
type AuthSource string

const (
	AuthSourceEnv    AuthSource = "env"
	AuthSourceStored AuthSource = "stored"
	AuthSourceNone   AuthSource = "none"
)

// AuthModeSummary encapsulates the resolved auth state for telemetry, status and banners.
type AuthModeSummary struct {
	Mode   AuthMode   `json:"mode"`
	Source AuthSource `json:"source"`
	Label  string     `json:"label"`
}

// GetAuthModeSummary resolves current authentication credentials in precedence order.
func GetAuthModeSummary() *AuthModeSummary {
	// 1. Direct environment token
	if envToken := strings.TrimSpace(os.Getenv("SCANDRIX_TOKEN")); envToken != "" {
		return &AuthModeSummary{
			Mode:   AuthModeToken,
			Source: AuthSourceEnv,
			Label:  "token (env)",
		}
	}

	// 2. Direct environment team key
	if envTeamKey := strings.TrimSpace(os.Getenv("SCANDRIX_TEAM_KEY")); envTeamKey != "" {
		return &AuthModeSummary{
			Mode:   AuthModeTeamKey,
			Source: AuthSourceEnv,
			Label:  "team key (env)",
		}
	}

	// 3. Stored credentials (OAuth or login token)
	creds, err := LoadCredentials()
	if err == nil && creds != nil {
		if creds.AccessToken != "" || creds.RefreshToken != "" {
			return &AuthModeSummary{
				Mode:   AuthModeLoggedIn,
				Source: AuthSourceStored,
				Label:  "logged in",
			}
		}
		if creds.TeamKey != "" {
			return &AuthModeSummary{
				Mode:   AuthModeTeamKey,
				Source: AuthSourceStored,
				Label:  "team key",
			}
		}
	}

	// 4. Default to unauthenticated trial
	return &AuthModeSummary{
		Mode:   AuthModeTrial,
		Source: AuthSourceNone,
		Label:  "trial",
	}
}

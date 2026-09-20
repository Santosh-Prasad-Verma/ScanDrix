// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"os"
	"testing"
)

func TestGetAuthModeSummary(t *testing.T) {
	// Clean env
	origToken := os.Getenv("SCANDRIX_TOKEN")
	origTeamKey := os.Getenv("SCANDRIX_TEAM_KEY")
	defer func() {
		os.Setenv("SCANDRIX_TOKEN", origToken)
		os.Setenv("SCANDRIX_TEAM_KEY", origTeamKey)
	}()

	os.Unsetenv("SCANDRIX_TOKEN")
	os.Unsetenv("SCANDRIX_TEAM_KEY")

	// 1. Token from env
	os.Setenv("SCANDRIX_TOKEN", "scandrix_token_123")
	s1 := GetAuthModeSummary()
	if s1.Mode != AuthModeToken || s1.Source != AuthSourceEnv {
		t.Errorf("expected token/env, got %s/%s", s1.Mode, s1.Source)
	}

	// 2. Team key from env
	os.Unsetenv("SCANDRIX_TOKEN")
	os.Setenv("SCANDRIX_TEAM_KEY", "scandrix_team_456")
	s2 := GetAuthModeSummary()
	if s2.Mode != AuthModeTeamKey || s2.Source != AuthSourceEnv {
		t.Errorf("expected team-key/env, got %s/%s", s2.Mode, s2.Source)
	}

	// 3. Fallback when none
	os.Unsetenv("SCANDRIX_TEAM_KEY")
	s3 := GetAuthModeSummary()
	if s3.Mode != AuthModeTrial && s3.Mode != AuthModeLoggedIn {
		t.Errorf("expected trial or logged-in, got %s", s3.Mode)
	}
}

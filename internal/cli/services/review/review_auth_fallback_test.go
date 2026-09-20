// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package review

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/scandrix/backend/internal/cli/configcli"
)

func TestWithTeamKeyFallback_Success(t *testing.T) {
	ctx := context.Background()
	initialToken := "bearer_test_user_token"

	res, err := WithTeamKeyFallback(ctx, initialToken, func(tok string) (string, error) {
		return "ok:" + tok, nil
	})

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if res != "ok:bearer_test_user_token" {
		t.Errorf("unexpected result: %s", res)
	}
}

func TestWithTeamKeyFallback_FallbackToTeamKey(t *testing.T) {
	tempHome := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tempHome)

	// Configure team key
	cfg := configcli.Load(".")
	cfg.APIKey = "scandrix_team_fallback_123"
	if err := configcli.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	userToken := "expired_bearer_token"
	attempts := 0

	res, err := WithTeamKeyFallback(ctx, userToken, func(tok string) (string, error) {
		attempts++
		if tok == userToken {
			return "", errors.New("API error (status 401): unauthorized token")
		}
		if tok == "scandrix_team_fallback_123" {
			return "fallback-success", nil
		}
		return "", errors.New("invalid token")
	})

	if err != nil {
		t.Fatalf("expected fallback success, got: %v", err)
	}
	if res != "fallback-success" {
		t.Errorf("expected 'fallback-success', got %s", res)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

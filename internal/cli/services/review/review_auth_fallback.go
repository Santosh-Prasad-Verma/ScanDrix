// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package review

import (
	"context"
	"errors"
	"strings"

	"github.com/scandrix/backend/internal/cli/configcli"
	"github.com/scandrix/backend/internal/cli/utils"
)

// WithTeamKeyFallback executes an operation with the given token.
// If the operation returns HTTP 401 Unauthorized and the initial token was a user bearer token
// (i.e. not a scandrix_ team key), it automatically falls back to the configured team API key.
func WithTeamKeyFallback[T any](ctx context.Context, token string, operation func(tok string) (T, error)) (T, error) {
	result, err := operation(token)
	if err == nil {
		return result, nil
	}

	// Check if this error represents an unauthorized / 401 condition
	var cmdErr *utils.CommandError
	is401 := false

	if errors.As(err, &cmdErr) && cmdErr.Code == utils.ErrCodeAuthRequired {
		is401 = true
	} else {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized") || strings.Contains(errStr, "status 401") {
			is401 = true
		}
	}

	// If already using a team key, do not retry with the same key
	if !is401 || strings.HasPrefix(token, "scandrix_") {
		var zero T
		return zero, err
	}

	// Attempt fallback to team API key
	cfg := configcli.Load(".")
	if cfg == nil || cfg.APIKey == "" {
		var zero T
		return zero, err
	}

	// Retry using team API key
	return operation(cfg.APIKey)
}

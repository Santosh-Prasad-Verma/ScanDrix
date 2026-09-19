// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"errors"
	"fmt"
	"strings"
)

// ERROR CODES & STRUCTURED COMMAND ERROR

type CommandErrorCode string

const (
	ErrCodeAuthRequired     CommandErrorCode = "AUTH_REQUIRED"
	ErrCodeAPIRequestFailed CommandErrorCode = "API_REQUEST_FAILED"
	ErrCodeNotInGitRepo     CommandErrorCode = "NOT_IN_GIT_REPO"
	ErrCodeNoChanges        CommandErrorCode = "NO_CHANGES"
	ErrCodeInvalidInput     CommandErrorCode = "INVALID_INPUT"
	ErrCodeInternalError    CommandErrorCode = "INTERNAL_ERROR"
)

// CommandError represents a typed error with an associated exit code and details.
type CommandError struct {
	Code     CommandErrorCode `json:"code"`
	Message  string           `json:"message"`
	ExitCode int              `json:"exit_code"`
	Details  any              `json:"details,omitempty"`
}

func (e *CommandError) Error() string {
	return e.Message
}

// NewCommandError constructs a CommandError with optional exit code and details.
func NewCommandError(code CommandErrorCode, message string, extra ...any) *CommandError {
	exitCode := 1
	var details any
	if len(extra) > 0 {
		if ec, ok := extra[0].(int); ok {
			exitCode = ec
			if len(extra) > 1 {
				details = extra[1]
			}
		} else {
			details = extra[0]
		}
	}
	return &CommandError{
		Code:     code,
		Message:  message,
		ExitCode: exitCode,
		Details:  details,
	}
}

// ERROR NORMALIZATION (Deterministic Error & Exit Codes)

// NormalizedCommandError is the normalized structure for CLI errors.
type NormalizedCommandError struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	ExitCode int    `json:"exit_code"`
	Details  any    `json:"details,omitempty"`
}

// NormalizeCommandError inspects any Go error and maps it to a standard error code and exit code.
func NormalizeCommandError(err error) NormalizedCommandError {
	if err == nil {
		return NormalizedCommandError{
			Code:     "",
			Message:  "",
			ExitCode: 0,
		}
	}

	var cmdErr *CommandError
	if errors.As(err, &cmdErr) {
		return NormalizedCommandError{
			Code:     string(cmdErr.Code),
			Message:  cmdErr.Message,
			ExitCode: cmdErr.ExitCode,
			Details:  cmdErr.Details,
		}
	}

	msg := err.Error()
	lowerMsg := strings.ToLower(msg)

	if strings.Contains(lowerMsg, "not authenticated") ||
		strings.Contains(lowerMsg, "auth required") ||
		strings.Contains(lowerMsg, "session expired") ||
		strings.Contains(lowerMsg, "token required") ||
		strings.Contains(lowerMsg, "unauthorized") {
		return NormalizedCommandError{
			Code:     string(ErrCodeAuthRequired),
			Message:  msg,
			ExitCode: 1,
		}
	}

	if strings.Contains(lowerMsg, "not a git repository") ||
		strings.Contains(lowerMsg, "run inside a git repo") ||
		strings.Contains(lowerMsg, "missing .git directory") {
		return NormalizedCommandError{
			Code:     string(ErrCodeNotInGitRepo),
			Message:  msg,
			ExitCode: 1,
		}
	}

	if strings.Contains(lowerMsg, "no changes to review") ||
		strings.Contains(lowerMsg, "no local changes found") ||
		strings.Contains(lowerMsg, "no diff found") {
		return NormalizedCommandError{
			Code:     string(ErrCodeNoChanges),
			Message:  msg,
			ExitCode: 0,
		}
	}

	if strings.Contains(lowerMsg, "invalid input") ||
		strings.Contains(lowerMsg, "invalid flag") ||
		strings.Contains(lowerMsg, "unknown command") ||
		strings.Contains(lowerMsg, "unknown flag") ||
		strings.Contains(lowerMsg, "cannot be used together") {
		return NormalizedCommandError{
			Code:     string(ErrCodeInvalidInput),
			Message:  msg,
			ExitCode: 1,
		}
	}

	if strings.Contains(lowerMsg, "connection refused") ||
		strings.Contains(lowerMsg, "connection to scandrix api") ||
		strings.Contains(lowerMsg, "failed connecting") ||
		strings.Contains(lowerMsg, "api returned status") {
		return NormalizedCommandError{
			Code:     string(ErrCodeAPIRequestFailed),
			Message:  fmt.Sprintf("Could not reach the ScanDrix API: %s", msg),
			ExitCode: 1,
		}
	}

	return NormalizedCommandError{
		Code:     string(ErrCodeInternalError),
		Message:  msg,
		ExitCode: 1,
	}
}

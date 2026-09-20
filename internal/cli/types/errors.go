package types

import (
	"fmt"
)

// CliErrorCode defines typed error category constants.
type CliErrorCode string

const (
	ErrCodeAuthRequired       CliErrorCode = "AUTH_REQUIRED"
	ErrCodeInvalidCredentials CliErrorCode = "INVALID_CREDENTIALS"
	ErrCodeNetworkError       CliErrorCode = "NETWORK_ERROR"
	ErrCodeAPIError           CliErrorCode = "API_ERROR"
	ErrCodeRateLimited        CliErrorCode = "RATE_LIMITED"
	ErrCodeGitNotFound        CliErrorCode = "GIT_NOT_FOUND"
	ErrCodeNoChanges          CliErrorCode = "NO_CHANGES"
	ErrCodeReviewFailed       CliErrorCode = "REVIEW_FAILED"
	ErrCodeConfigError        CliErrorCode = "CONFIG_ERROR"
	ErrCodeValidationError    CliErrorCode = "VALIDATION_ERROR"
	ErrCodeTimeoutError       CliErrorCode = "TIMEOUT_ERROR"
	ErrCodePermissionDenied   CliErrorCode = "PERMISSION_DENIED"
	ErrCodeInternalError      CliErrorCode = "INTERNAL_ERROR"
)

// Standard CLI Exit Codes
const (
	ExitCodeSuccess         = 0
	ExitCodeGeneralError    = 1
	ExitCodeBlockingIssues  = 2
	ExitCodeAuthError       = 3
	ExitCodeNetworkError    = 4
	ExitCodeConfigError     = 5
)

// CliError represents a structured, user-facing error with action recommendation.
type CliError struct {
	Code       CliErrorCode `json:"code"`
	Message    string       `json:"message"`
	Action     string       `json:"action,omitempty"`
	Detail     string       `json:"detail,omitempty"`
	ExitCode   int          `json:"exit_code"`
	Underlying error        `json:"-"`
}

func (e *CliError) Error() string {
	if e.Action != "" {
		return fmt.Sprintf("[%s] %s (Action: %s)", e.Code, e.Message, e.Action)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *CliError) Unwrap() error {
	return e.Underlying
}

// NewCliError constructs a new typed CliError.
func NewCliError(code CliErrorCode, message string, action string, exitCode int, underlying error) *CliError {
	return &CliError{
		Code:       code,
		Message:    message,
		Action:     action,
		ExitCode:   exitCode,
		Underlying: underlying,
	}
}

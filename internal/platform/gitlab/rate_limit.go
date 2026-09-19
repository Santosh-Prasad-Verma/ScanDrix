// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package gitlab

import (
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/types"
)

const (
	// DefaultGitLabRateLimitWindow is the standard GitLab burst rate limit recovery window.
	DefaultGitLabRateLimitWindow = 60 * time.Second
	// MaxRetryAttempts specifies the upper bound for transient retries.
	MaxRetryAttempts = 5
	// InitialBackoffDuration is the initial retry delay.
	InitialBackoffDuration = 500 * time.Millisecond
	// MaxBackoffDuration is the upper cap for any single retry backoff.
	MaxBackoffDuration = 30 * time.Second
)

// GitLabRateLimitError records rate limiting or temporary backpressure from GitLab.
type GitLabRateLimitError struct {
	ResetAt        time.Time
	Message        string
	StatusCode     int
	OrganizationID string
	TeamID         string
}

func (e *GitLabRateLimitError) Error() string {
	return e.Message
}

// IsGitLabRateLimitError determines if an HTTP status or error indicates rate limiting.
func IsGitLabRateLimitError(resp *http.Response, err error) bool {
	if resp != nil {
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusBadGateway {
			return true
		}
	}
	if err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "429") ||
			strings.Contains(errStr, "rate limit") ||
			strings.Contains(errStr, "retry-after") ||
			strings.Contains(errStr, "too many requests") ||
			strings.Contains(errStr, "502") ||
			strings.Contains(errStr, "bad gateway") {
			return true
		}
	}
	return false
}

// ParseRetryAfter extracts delay from the Retry-After header.
func ParseRetryAfter(headerVal string) time.Duration {
	if headerVal == "" {
		return DefaultGitLabRateLimitWindow
	}
	// Try parsing as seconds
	if seconds, err := strconv.Atoi(headerVal); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	// Try parsing as HTTP-date (RFC1123 or RFC850)
	if t, err := http.ParseTime(headerVal); err == nil {
		diff := time.Until(t)
		if diff > 0 {
			return diff
		}
	}
	return DefaultGitLabRateLimitWindow
}

// CalculateBackoff computes full jitter exponential backoff for attempt number.
func CalculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return InitialBackoffDuration
	}
	multiplier := math.Pow(2, float64(attempt))
	delay := float64(InitialBackoffDuration) * multiplier
	if delay > float64(MaxBackoffDuration) {
		delay = float64(MaxBackoffDuration)
	}
	// Add jitter (0% - 25%)
	jitter := rand.Float64() * 0.25 * delay
	return time.Duration(delay + jitter)
}

// ToGitLabRateLimitError wraps the error into a standardized GitLabRateLimitError.
func ToGitLabRateLimitError(resp *http.Response, originalErr error, orgData types.OrganizationAndTeamData) *GitLabRateLimitError {
	retryDuration := DefaultGitLabRateLimitWindow
	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
		retryDuration = ParseRetryAfter(resp.Header.Get("Retry-After"))
	}

	msg := "GitLab refused the request rate: rate limit or service unavailable"
	if originalErr != nil {
		msg = "GitLab refused the request rate: " + originalErr.Error()
	}

	return &GitLabRateLimitError{
		ResetAt:        time.Now().Add(retryDuration),
		Message:        msg,
		StatusCode:     statusCode,
		OrganizationID: orgData.OrganizationID,
		TeamID:         orgData.TeamID,
	}
}

package workflow

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/core/domain"
)

// SCMResponseError encapsulates upstream SCM provider HTTP error details.
type SCMResponseError struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
	Message    string            `json:"message"`
}

func (e *SCMResponseError) Error() string {
	return fmt.Sprintf("SCM HTTP %d: %s", e.StatusCode, e.Message)
}

// ErrorClassifierService mirrors ScanDrix ErrorClassifierService and classify-github-error.ts.
type ErrorClassifierService struct{}

// NewErrorClassifierService instantiates a new ErrorClassifierService.
func NewErrorClassifierService() *ErrorClassifierService {
	return &ErrorClassifierService{}
}

// Classify categorizes errors into RETRYABLE, PERMANENT, or RATE_LIMITED.
func (c *ErrorClassifierService) Classify(err error) domain.ErrorClassification {
	if err == nil {
		return domain.ErrorClassificationRetryable
	}

	// 1. Concrete RateLimitError check
	if _, ok := err.(*RateLimitError); ok {
		return domain.ErrorClassificationRateLimited
	}

	// 2. Structured SCM error inspection
	if scmErr, ok := err.(*SCMResponseError); ok {
		if rateErr := ClassifySCMRateLimit(scmErr); rateErr != nil {
			return domain.ErrorClassificationRateLimited
		}
		if scmErr.StatusCode == 401 || scmErr.StatusCode == 403 || scmErr.StatusCode == 404 || scmErr.StatusCode == 422 {
			return domain.ErrorClassificationPermanent
		}
	}

	msg := strings.ToLower(err.Error())

	// 3. String-based Rate limit inspection
	if strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "secondary rate limit") ||
		strings.Contains(msg, "retry-after") ||
		strings.Contains(msg, "429") {
		return domain.ErrorClassificationRateLimited
	}

	// 4. Network / timeout errors are retryable
	if strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "network") ||
		strings.Contains(msg, "econnrefused") ||
		strings.Contains(msg, "etimedout") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "502") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "504") {
		return domain.ErrorClassificationRetryable
	}

	// 5. Permanent / validation / authorization errors
	if strings.Contains(msg, "validation") ||
		strings.Contains(msg, "invalid") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "bad credentials") ||
		strings.Contains(msg, "forbidden") {
		return domain.ErrorClassificationNonRetryable
	}

	// Default fallback: assume retryable
	return domain.ErrorClassificationRetryable
}

// ClassifySCMRateLimit inspects SCM response status and headers (GitHub, GitLab, Bitbucket, Azure)
// mirroring ScanDrix classifyGitHubError.
func ClassifySCMRateLimit(err *SCMResponseError) *RateLimitError {
	if err == nil {
		return nil
	}
	if err.StatusCode != 403 && err.StatusCode != 429 {
		return nil
	}

	// Case-insensitive header lookup
	getHeader := func(name string) string {
		for k, v := range err.Headers {
			if strings.EqualFold(k, name) {
				return v
			}
		}
		return ""
	}

	remaining := getHeader("x-ratelimit-remaining")
	reset := getHeader("x-ratelimit-reset")
	retryAfter := getHeader("retry-after")

	// Primary rate limit: explicit x-ratelimit-remaining == 0 and x-ratelimit-reset
	if remaining == "0" && reset != "" {
		resetUnix, parseErr := strconv.ParseInt(reset, 10, 64)
		if parseErr == nil && resetUnix > 0 {
			resetAt := time.Unix(resetUnix, 0).UTC()
			return &RateLimitError{
				ResetAt:   resetAt,
				Remaining: 0,
				Message:   fmt.Sprintf("Primary rate limit exhausted, reset at %s", resetAt.Format(time.RFC3339)),
			}
		}
	}

	// Secondary / burst rate limit: retry-after header present
	if retryAfter != "" {
		waitSec, parseErr := strconv.Atoi(retryAfter)
		if parseErr == nil && waitSec > 0 {
			resetAt := time.Now().UTC().Add(time.Duration(waitSec) * time.Second)
			return &RateLimitError{
				ResetAt:   resetAt,
				Remaining: 0,
				Message:   fmt.Sprintf("Secondary rate limit throttle, retry after %d seconds", waitSec),
			}
		}
	}

	if err.StatusCode == 429 {
		// Generic 429 fallback: 60-second backoff
		resetAt := time.Now().UTC().Add(60 * time.Second)
		return &RateLimitError{
			ResetAt:   resetAt,
			Remaining: 0,
			Message:   "Upstream SCM returned HTTP 429 Too Many Requests",
		}
	}

	return nil
}

// ClassifyGitHubError inspects errors and returns *RateLimitError if an upstream 403/429 rate limit is detected.
func ClassifyGitHubError(err error) error {
	if err == nil {
		return nil
	}
	if scmErr, ok := err.(*SCMResponseError); ok {
		if rateErr := ClassifySCMRateLimit(scmErr); rateErr != nil {
			return rateErr
		}
	}
	return err
}

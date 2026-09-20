// Package httpclient provides retry policies with exponential backoff and jitter.
package httpclient

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Default retry constants
const (
	DefaultMaxAttempts = 4
	DefaultBaseDelay   = 1 * time.Second
	DefaultMaxDelay    = 30 * time.Second
)

// HTTPResponseCarrier is an interface for errors that carry an HTTP response status code or headers.
type HTTPResponseCarrier interface {
	StatusCode() int
	Header() http.Header
}

// HTTPStatusError represents an HTTP error response.
type HTTPStatusError struct {
	Code    int
	Headers http.Header
	Message string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("http error %d: %s", e.Code, e.Message)
}

func (e *HTTPStatusError) StatusCode() int {
	return e.Code
}

func (e *HTTPStatusError) Header() http.Header {
	return e.Headers
}

// WithRetryOptions configures the retry loop behavior.
type WithRetryOptions struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Label       string
	Logger      *slog.Logger
}

// With429Retry executes fn with exponential backoff and jitter upon encountering 429 or rate limits.
func With429Retry[T any](ctx context.Context, opts WithRetryOptions, fn func(ctx context.Context, attempt int) (T, error)) (T, error) {
	var zero T
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	baseDelay := opts.BaseDelay
	if baseDelay <= 0 {
		baseDelay = DefaultBaseDelay
	}
	maxDelay := opts.MaxDelay
	if maxDelay <= 0 {
		maxDelay = DefaultMaxDelay
	}
	label := opts.Label
	if label == "" {
		label = "with429Retry"
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		default:
		}

		res, err := fn(ctx, attempt)
		if err == nil {
			return res, nil
		}

		lastErr = err
		if !Is429Error(err) && !IsTransientError(err) {
			return zero, err
		}

		if attempt == maxAttempts-1 {
			log.Warn(fmt.Sprintf("%s: exhausted %d attempts on retryable error", label, maxAttempts),
				slog.Any("error", err),
				slog.Int("attempts", maxAttempts))
			return zero, err
		}

		delay := resolveDelay(err, attempt, baseDelay, maxDelay)
		log.Warn(fmt.Sprintf("%s: rate limited/transient error, sleeping %v before retry %d/%d",
			label, delay, attempt+2, maxAttempts),
			slog.Any("error", err),
			slog.Duration("delay", delay))

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		case <-timer.C:
		}
	}

	return zero, lastErr
}

// Is429Error tests whether an error is caused by rate limiting (HTTP 429).
func Is429Error(err error) bool {
	if err == nil {
		return false
	}

	var carrier HTTPResponseCarrier
	if errors.As(err, &carrier) {
		if carrier.StatusCode() == http.StatusTooManyRequests {
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "ratelimit")
}

// IsTransientError tests whether an error is a recoverable network/transport error.
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "i/o timeout")
}

func resolveDelay(err error, attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	if retryAfter := parseRetryAfter(err); retryAfter > 0 {
		if retryAfter > maxDelay {
			return maxDelay
		}
		return retryAfter
	}

	// Exponential backoff with full jitter
	multiplier := math.Pow(2, float64(attempt))
	capDuration := float64(baseDelay) * multiplier
	if capDuration > float64(maxDelay) {
		capDuration = float64(maxDelay)
	}

	jitter := rand.Float64() * capDuration
	return time.Duration(jitter)
}

func parseRetryAfter(err error) time.Duration {
	var carrier HTTPResponseCarrier
	if !errors.As(err, &carrier) {
		return 0
	}

	headers := carrier.Header()
	if headers == nil {
		return 0
	}

	raw := headers.Get("Retry-After")
	if raw == "" {
		raw = headers.Get("retry-after")
	}
	if raw == "" {
		return 0
	}

	// Try seconds as integer
	if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}

	// Try HTTP-date formats (RFC1123, RFC850, ANSIC)
	formats := []string{
		http.TimeFormat,
		time.RFC850,
		time.ANSIC,
	}
	for _, fmtStr := range formats {
		if t, err := time.Parse(fmtStr, raw); err == nil {
			delta := time.Until(t)
			if delta > 0 {
				return delta
			}
			return 0
		}
	}

	return 0
}

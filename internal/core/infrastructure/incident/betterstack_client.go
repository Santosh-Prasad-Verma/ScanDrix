// Package incident provides BetterStack API v2 integration, heartbeat monitoring, and incident deduplication.
package incident

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/telemetry"
)

var (
	ErrCircuitBreakerOpen = errors.New("betterstack circuit breaker is open; skipping request")
)

const (
	DefaultBetterStackBaseURL = "https://uptime.betterstack.com/api/v2"
	DefaultCircuitMaxFailures = 5
	DefaultCircuitCooldown    = 60 * time.Second
	DefaultHTTPTimeout        = 10 * time.Second
)

// CircuitBreakerState represents the operational status of the circuit breaker.
type CircuitBreakerState string

const (
	CircuitClosed   CircuitBreakerState = "CLOSED"
	CircuitOpen     CircuitBreakerState = "OPEN"
	CircuitHalfOpen CircuitBreakerState = "HALF_OPEN"
)

// CircuitBreaker protects the application against cascading delays when external APIs fail.
type CircuitBreaker struct {
	mu          sync.Mutex
	state       CircuitBreakerState
	failures    int
	maxFailures int
	lastFailure time.Time
	cooldown    time.Duration
}

// NewCircuitBreaker creates a new circuit breaker instance.
func NewCircuitBreaker(maxFailures int, cooldown time.Duration) *CircuitBreaker {
	if maxFailures <= 0 {
		maxFailures = DefaultCircuitMaxFailures
	}
	if cooldown <= 0 {
		cooldown = DefaultCircuitCooldown
	}
	return &CircuitBreaker{
		state:       CircuitClosed,
		maxFailures: maxFailures,
		cooldown:    cooldown,
	}
}

// Allow reports whether a new request is permitted through the circuit.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	if cb.state == CircuitOpen {
		if now.Sub(cb.lastFailure) >= cb.cooldown {
			cb.state = CircuitHalfOpen
			return true
		}
		return false
	}
	return true
}

// RecordSuccess records a successful call, resetting the failure counter and closing the circuit.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.state = CircuitClosed
}

// RecordFailure records an error and trips the circuit open if threshold is reached.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailure = time.Now()
	if cb.failures >= cb.maxFailures {
		cb.state = CircuitOpen
	}
}

// State returns the current circuit breaker status.
func (cb *CircuitBreaker) State() CircuitBreakerState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// CreateIncidentPayload mirrors BetterStack incident generation payload.
type CreateIncidentPayload struct {
	Name      string `json:"name"`
	Summary   string `json:"summary"`
	Severity  string `json:"severity,omitempty"` // critical | major | minor
	Requester string `json:"requester,omitempty"`
}

// BetterStackClient communicates with BetterStack Uptime & Incident API.
type BetterStackClient struct {
	apiToken   string
	baseURL    string
	httpClient *http.Client
	breaker    *CircuitBreaker
	logger     *telemetry.LoggerWrapper
}

// NewBetterStackClient creates a BetterStack client instance.
func NewBetterStackClient(apiToken string, baseURL string, client *http.Client) *BetterStackClient {
	if baseURL == "" {
		baseURL = DefaultBetterStackBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: DefaultHTTPTimeout}
	}
	return &BetterStackClient{
		apiToken:   apiToken,
		baseURL:    baseURL,
		httpClient: client,
		breaker:    NewCircuitBreaker(DefaultCircuitMaxFailures, DefaultCircuitCooldown),
		logger:     telemetry.DefaultLogger(),
	}
}

// PingHeartbeat sends an HTTP GET heartbeat to signal healthy operation.
func (c *BetterStackClient) PingHeartbeat(ctx context.Context, heartbeatURL string) error {
	if !c.breaker.Allow() {
		return ErrCircuitBreakerOpen
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, heartbeatURL, nil)
	if err != nil {
		return fmt.Errorf("failed creating heartbeat request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("heartbeat ping failed for %s: %w", c.RedactURL(heartbeatURL), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		c.breaker.RecordFailure()
		return fmt.Errorf("heartbeat ping returned non-2xx status %d for %s", resp.StatusCode, c.RedactURL(heartbeatURL))
	}

	c.breaker.RecordSuccess()
	return nil
}

// FailHeartbeat sends an HTTP POST with failure telemetry to trigger alert escalation.
func (c *BetterStackClient) FailHeartbeat(ctx context.Context, heartbeatURL string, message string, contextData map[string]any) error {
	if !c.breaker.Allow() {
		return ErrCircuitBreakerOpen
	}

	bodyData := map[string]any{
		"message": message,
	}
	if len(contextData) > 0 {
		bodyData["context"] = contextData
	}

	jsonBytes, err := json.Marshal(bodyData)
	if err != nil {
		return fmt.Errorf("failed marshaling heartbeat failure body: %w", err)
	}

	failURL := heartbeatURL
	// BetterStack heartbeat failure endpoint convention appends /fail if not already present
	parsed, err := url.Parse(heartbeatURL)
	if err == nil && parsed.Path != "" && parsed.Path[len(parsed.Path)-5:] != "/fail" {
		failURL = heartbeatURL + "/fail"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, failURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return fmt.Errorf("failed creating fail-heartbeat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("heartbeat failure report failed for %s: %w", c.RedactURL(failURL), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		c.breaker.RecordFailure()
		return fmt.Errorf("heartbeat failure report returned status %d for %s", resp.StatusCode, c.RedactURL(failURL))
	}

	c.breaker.RecordSuccess()
	return nil
}

// CreateIncident generates a high-priority incident on BetterStack.
func (c *BetterStackClient) CreateIncident(ctx context.Context, payload CreateIncidentPayload) error {
	if c.apiToken == "" {
		c.logger.Debug("BetterStack API token not configured; skipping incident creation")
		return nil
	}

	if !c.breaker.Allow() {
		return ErrCircuitBreakerOpen
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed marshaling incident payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s/incidents", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed creating incident request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("failed to dispatch incident to BetterStack: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		c.breaker.RecordFailure()
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("betterstack returned status %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

var tokenPathRegex = regexp.MustCompile(`(/[a-zA-Z0-9_-]{16,})`)

// RedactURL masks sensitive tokens embedded within heartbeat URLs for safe logging.
func (c *BetterStackClient) RedactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "[INVALID_URL]"
	}

	u.RawQuery = ""
	maskedPath := tokenPathRegex.ReplaceAllString(u.Path, "/[REDACTED]")
	return fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, maskedPath)
}

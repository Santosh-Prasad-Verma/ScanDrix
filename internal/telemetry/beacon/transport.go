// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Telemetry Subsystem
// Package: beacon
// File: transport.go
// ═══════════════════════════════════════════════════════════════

package beacon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	DefaultTelemetryEndpoint = "https://telemetry.scandrix.dev/v1/heartbeat"
	DefaultTimeout           = 5 * time.Second
)

var optOutRegex = regexp.MustCompile(`^(?i:1|true|yes|on)$`)

// IBeaconHTTPProvider outlines the network transmission interface for self-hosted heartbeats.
type IBeaconHTTPProvider interface {
	IsDisabled() bool
	Send(ctx context.Context, payload map[string]interface{}, version string) (bool, error)
}

// BeaconHTTPProvider provides robust, non-blocking HTTP transport for anonymous heartbeat pings.
type BeaconHTTPProvider struct {
	httpClient *http.Client
	logger     *slog.Logger
	endpointFn func() string
}

// NewBeaconHTTPProvider constructs a configured HTTP transmission provider.
func NewBeaconHTTPProvider(logger *slog.Logger) *BeaconHTTPProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &BeaconHTTPProvider{
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		logger: logger.With("component", "beacon_http_provider"),
		endpointFn: func() string {
			if ep := strings.TrimSpace(os.Getenv("SCANDRIX_TELEMETRY_ENDPOINT")); ep != "" {
				return ep
			}
			return DefaultTelemetryEndpoint
		},
	}
}

// IsDisabled checks dynamic runtime environment opt-out flags.
func (p *BeaconHTTPProvider) IsDisabled() bool {
	if val := os.Getenv("SCANDRIX_TELEMETRY_DISABLED"); optOutRegex.MatchString(strings.TrimSpace(val)) {
		return true
	}
	if val := os.Getenv("DO_NOT_TRACK"); optOutRegex.MatchString(strings.TrimSpace(val)) {
		return true
	}
	return false
}

// Send serializes and transmits the heartbeat payload to the telemetry ingest endpoint.
// In accordance with enterprise resilience rules, transport failures are logged and swallowed.
func (p *BeaconHTTPProvider) Send(ctx context.Context, payload map[string]interface{}, version string) (bool, error) {
	if p.IsDisabled() {
		return false, nil
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		p.logger.Warn("Failed to marshal self-hosted heartbeat payload", "error", err)
		return false, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	endpoint := p.endpointFn()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		p.logger.Warn("Failed to create beacon HTTP request", "error", err)
		return false, nil
	}

	if version == "" {
		version = "1.0.0"
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("scandrix-self-hosted/%s", version))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Warn("Self-hosted beacon network transmission failed (swallowed)", "error", err)
		return false, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return true, nil
	}

	p.logger.Warn("Telemetry beacon rejected heartbeat submission", "status", resp.StatusCode)
	return false, nil
}

package security

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

var (
	// ErrAirGapEgressBlocked is returned when an outbound network request is prevented by the air-gap firewall.
	ErrAirGapEgressBlocked = errors.New("egress violation: outbound connection blocked by air-gapped enforcement gate")
)

// AuditEgressReporter logs blocked egress events to the audit / SIEM stream.
type AuditEgressReporter interface {
	ReportBlockedEgress(ctx context.Context, targetURL string, reason string)
}

// AirGapConfig defines parameters for sovereign air-gapped execution.
type AirGapConfig struct {
	Enabled      bool
	AllowedHosts []string
	Reporter     AuditEgressReporter
}

// AirGapGate enforces deny-by-default outbound network access when running air-gapped.
type AirGapGate struct {
	mu           sync.RWMutex
	enabled      bool
	allowedHosts map[string]bool
	allowedCIDRs []*net.IPNet
	reporter     AuditEgressReporter
	baseRoundTripper http.RoundTripper
}

// NewAirGapGate initializes the air-gap gate from environment and parameters.
func NewAirGapGate(cfg AirGapConfig) *AirGapGate {
	enabled := cfg.Enabled
	if !enabled {
		envVal := strings.ToLower(os.Getenv("AIR_GAPPED"))
		if envVal == "true" || envVal == "1" || envVal == "yes" {
			enabled = true
		}
	}

	gate := &AirGapGate{
		enabled:          enabled,
		allowedHosts:     make(map[string]bool),
		allowedCIDRs:     make([]*net.IPNet, 0),
		reporter:         cfg.Reporter,
		baseRoundTripper: http.DefaultTransport,
	}

	// Always permit local loopback
	gate.allowedHosts["localhost"] = true
	gate.allowedHosts["127.0.0.1"] = true
	gate.allowedHosts["::1"] = true

	// Parse custom allowed hosts or CIDRs
	allowedFromEnv := os.Getenv("AIRGAP_ALLOWED_HOSTS")
	allHosts := append([]string{}, cfg.AllowedHosts...)
	if allowedFromEnv != "" {
		allHosts = append(allHosts, strings.Split(allowedFromEnv, ",")...)
	}

	for _, h := range allHosts {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if _, ipNet, err := net.ParseCIDR(h); err == nil {
			gate.allowedCIDRs = append(gate.allowedCIDRs, ipNet)
		} else {
			// strip port if present
			hostName := h
			if hostOnly, _, err := net.SplitHostPort(h); err == nil {
				hostName = hostOnly
			}
			gate.allowedHosts[strings.ToLower(hostName)] = true
		}
	}

	return gate
}

// IsAirGapped reports whether the runtime gate is active.
func (g *AirGapGate) IsAirGapped() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.enabled
}

// SetAirGapped dynamically toggles air-gap enforcement (e.g., loaded from signed license).
func (g *AirGapGate) SetAirGapped(enabled bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.enabled = enabled
}

// RoundTrip intercepts outbound HTTP requests, enforcing deny-by-default when air-gapped.
func (g *AirGapGate) RoundTrip(req *http.Request) (*http.Response, error) {
	if !g.IsAirGapped() {
		return g.baseRoundTripper.RoundTrip(req)
	}

	if req == nil || req.URL == nil {
		return nil, errors.New("invalid nil request in air-gap round-trip")
	}

	host := req.URL.Hostname()
	if host == "" {
		host = req.Host
		if hostOnly, _, err := net.SplitHostPort(host); err == nil {
			host = hostOnly
		}
	}

	if !g.isHostAllowed(host) {
		slog.Warn("Air-gapped firewall blocked outbound network request",
			"host", host,
			"url", req.URL.Redacted(),
			"method", req.Method,
		)

		if g.reporter != nil {
			g.reporter.ReportBlockedEgress(req.Context(), req.URL.Redacted(), "destination not in airgap allowlist")
		}

		return nil, fmt.Errorf("%w: %s", ErrAirGapEgressBlocked, host)
	}

	return g.baseRoundTripper.RoundTrip(req)
}

func (g *AirGapGate) isHostAllowed(host string) bool {
	cleanHost := strings.ToLower(strings.TrimSpace(host))
	if cleanHost == "" {
		return false
	}

	// Check exact allowed hosts
	if g.allowedHosts[cleanHost] {
		return true
	}

	// Check IP / CIDR matches
	ip := net.ParseIP(cleanHost)
	if ip != nil {
		if ip.IsLoopback() {
			return true
		}
		for _, cidr := range g.allowedCIDRs {
			if cidr.Contains(ip) {
				return true
			}
		}
	}

	return false
}

// WrapClient wraps an http.Client's transport with the AirGapGate.
func (g *AirGapGate) WrapClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	client.Transport = g
	return client
}

// CheckAddressValid asserts that an address string is acceptable under current air-gap policy.
func (g *AirGapGate) CheckAddressValid(rawURL string) error {
	if !g.IsAirGapped() {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	host := u.Hostname()
	if !g.isHostAllowed(host) {
		return fmt.Errorf("%w: %s", ErrAirGapEgressBlocked, host)
	}
	return nil
}

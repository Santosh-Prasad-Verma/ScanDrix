// Package netguard validates outbound URLs before a process makes a request to
// them.
//
// # Why
//
// An outbound HTTP call to a URL that came from configuration, an environment
// variable or a parameter is a server-side request forgery primitive: the
// request originates inside the trust boundary, so it reaches hosts the caller
// could not reach itself -- most importantly the cloud instance metadata
// endpoint, which hands out credentials to anything that asks.
//
// Three properties are checked, because each closes a different route:
//
//   - Scheme. Plain http to a non-loopback host lets an on-path attacker rewrite
//     the response. Loopback is exempted so tests against an httptest server and
//     local development tooling keep working, and because a loopback request
//     cannot leave the host.
//   - Host. Empty, malformed, or userinfo-bearing hosts ("http://evil.com@"
//     reads as userinfo to a human and as evil.com to a parser) are rejected.
//   - Redirects. A validated URL can redirect to an unvalidated one, so
//     ValidateRedirect re-applies the same rules to every hop rather than
//     trusting the first one.
package netguard

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Options narrows what a caller is willing to reach.
type Options struct {
	// AllowedHosts, when non-empty, is an exact-match allowlist of hostnames.
	// An empty list allows any host that passes the scheme and host checks.
	AllowedHosts []string

	// AllowHostSuffix, when non-empty, additionally permits any host ending in
	// "."+suffix. Use for a vendor's own domain family, e.g. "discord.com".
	AllowHostSuffix string

	// AllowHTTPLoopback permits http:// to 127.0.0.0/8, ::1 and localhost.
	// Set this in tests and local tooling; leave it off in servers.
	AllowHTTPLoopback bool
}

// Validate parses raw and returns it when it satisfies opts.
func Validate(raw string, opts Options) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("url is empty")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("url %q is not parseable: %w", raw, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("url %q must be absolute, with a scheme and a host", raw)
	}
	// Userinfo is how "https://trusted.example@evil.test/" is smuggled past a
	// reader skimming for a hostname.
	if u.User != nil {
		return nil, fmt.Errorf("url %q must not contain credentials", raw)
	}

	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("url %q has no hostname", raw)
	}

	switch strings.ToLower(u.Scheme) {
	case "https":
		// Always allowed by scheme.
	case "http":
		if !opts.AllowHTTPLoopback {
			return nil, fmt.Errorf("url %q uses http; https is required", raw)
		}
		if !isLoopback(host) {
			return nil, fmt.Errorf("url %q uses http to non-loopback host %q", raw, host)
		}
	default:
		return nil, fmt.Errorf("url %q uses unsupported scheme %q", raw, u.Scheme)
	}

	if len(opts.AllowedHosts) > 0 || opts.AllowHostSuffix != "" {
		if !hostAllowed(host, opts) {
			return nil, fmt.Errorf("host %q is not an allowed destination", host)
		}
	}

	return u, nil
}

// RedirectPolicy returns a CheckRedirect that revalidates every hop, so a
// permitted URL cannot bounce the request to a forbidden one.
//
// hop is 1-based: CheckRedirect is called with the upcoming request and the
// number of requests already made.
func RedirectPolicy(opts Options, maxHops int) func(req *http.Request, via []*http.Request) error {
	if maxHops <= 0 {
		maxHops = 3
	}
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxHops {
			return fmt.Errorf("stopped after %d redirects", maxHops)
		}
		if _, err := Validate(req.URL.String(), opts); err != nil {
			return fmt.Errorf("refusing redirect to %q: %w", req.URL.Redacted(), err)
		}
		return nil
	}
}

// NewClient returns an http.Client that applies opts to the initial request and
// to every redirect hop.
func NewClient(timeoutSeconds int, opts Options) *http.Client {
	return &http.Client{
		Timeout:       time.Duration(timeoutSeconds) * time.Second,
		CheckRedirect: RedirectPolicy(opts, 3),
	}
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func hostAllowed(host string, opts Options) bool {
	for _, a := range opts.AllowedHosts {
		if strings.EqualFold(a, host) {
			return true
		}
	}
	if opts.AllowHostSuffix != "" {
		if strings.EqualFold(host, opts.AllowHostSuffix) {
			return true
		}
		if strings.HasSuffix(strings.ToLower(host), "."+strings.ToLower(opts.AllowHostSuffix)) {
			return true
		}
	}
	return false
}

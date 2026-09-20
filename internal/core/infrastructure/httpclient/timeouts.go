// Package httpclient provides resilient HTTP pooling, timeouts, and connection reuse.
package httpclient

import (
	"net"
	"net/http"
	"time"
)

const (
	// IntegrationRequestTimeout covers GitHub, GitLab, Azure Repos, Bitbucket REST calls.
	IntegrationRequestTimeout = 60 * time.Second

	// MCPRequestTimeout is the fail-fast timeout for MCP Manager RPC calls.
	MCPRequestTimeout = 60 * time.Second

	// LLMRequestTimeout handles large reasoning calls (Gemini, Claude, OpenAI) without premature aborts.
	LLMRequestTimeout = 10 * time.Minute

	// DefaultConnectTimeout is the TCP dialer connection timeout.
	DefaultConnectTimeout = 10 * time.Second

	// DefaultKeepAliveTimeout keeps TCP keep-alive pings active.
	DefaultKeepAliveTimeout = 30 * time.Second

	// DefaultMaxIdleConns specifies the maximum pool size of idle HTTP connections.
	DefaultMaxIdleConns = 100

	// DefaultMaxIdleConnsPerHost specifies the maximum idle pool per host.
	DefaultMaxIdleConnsPerHost = 20

	// DefaultIdleConnTimeout is the duration idle connections remain in pool.
	DefaultIdleConnTimeout = 90 * time.Second
)

// NewPooledTransport constructs an optimized http.RoundTripper with connection pooling.
func NewPooledTransport() *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   DefaultConnectTimeout,
			KeepAlive: DefaultKeepAliveTimeout,
		}).DialContext,
		MaxIdleConns:        DefaultMaxIdleConns,
		MaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
		IdleConnTimeout:     DefaultIdleConnTimeout,
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   true,
	}
}

// NewPooledClient creates an http.Client with custom connection pooling and the given timeout.
func NewPooledClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = IntegrationRequestTimeout
	}
	return &http.Client{
		Transport: NewPooledTransport(),
		Timeout:   timeout,
	}
}

// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// NetworkDiagnostic classifies network failures and provides actionable troubleshooting steps.
type NetworkDiagnostic struct {
	IsNetworkError bool   `json:"is_network_error"`
	Category       string `json:"category"` // connection_refused, dns_lookup, tls_handshake, timeout, proxy
	TargetHost     string `json:"target_host,omitempty"`
	RemedyHint     string `json:"remedy_hint"`
}

// DiagnoseNetworkError inspects an error for network, DNS, and TLS failure signatures.
func DiagnoseNetworkError(err error, endpointURL string) NetworkDiagnostic {
	if err == nil {
		return NetworkDiagnostic{}
	}

	msg := strings.ToLower(err.Error())
	diag := NetworkDiagnostic{
		IsNetworkError: true,
	}

	if endpointURL != "" {
		if parsed, parseErr := url.Parse(endpointURL); parseErr == nil {
			diag.TargetHost = parsed.Host
		}
	}

	// 1. Connection Refused
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "econnrefused") {
		diag.Category = "connection_refused"
		if diag.TargetHost != "" && (strings.Contains(diag.TargetHost, "localhost") || strings.Contains(diag.TargetHost, "127.0.0.1")) {
			diag.RemedyHint = fmt.Sprintf("Local ScanDrix server on %s appears offline. Start the backend with 'pnpm run docker:start:api' or configure cloud URL.", diag.TargetHost)
		} else {
			diag.RemedyHint = fmt.Sprintf("Unable to connect to %s. Verify server is online and port is accessible.", diag.TargetHost)
		}
		return diag
	}

	// 2. DNS / Hostname resolution failure
	var dnsErr *net.DNSError
	if strings.Contains(msg, "no such host") || strings.Contains(msg, "enotfound") || (err != nil && netErrorIsDNS(err, &dnsErr)) {
		diag.Category = "dns_lookup"
		diag.RemedyHint = "DNS resolution failed. Check your internet connection, VPN, or custom DNS configuration."
		return diag
	}

	// 3. TLS Handshake / Certificate errors
	if strings.Contains(msg, "certificate") || strings.Contains(msg, "x509") || strings.Contains(msg, "tls handshake") {
		diag.Category = "tls_handshake"
		diag.RemedyHint = "TLS verification failed. Check enterprise SSL proxy certificates or system root trust store."
		return diag
	}

	// 4. Timeout
	if strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "etimedout") {
		diag.Category = "timeout"
		diag.RemedyHint = "Request timed out. Check network latency, proxy settings, or corporate firewall rules."
		return diag
	}

	// 5. Proxy errors
	if strings.Contains(msg, "proxy") || os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" {
		diag.Category = "proxy"
		diag.RemedyHint = "Proxy error detected. Verify HTTP_PROXY and HTTPS_PROXY environment variables."
		return diag
	}

	diag.Category = "unknown_network"
	diag.RemedyHint = "Verify outbound network connectivity to the ScanDrix API."
	return diag
}

func netErrorIsDNS(err error, target **net.DNSError) bool {
	if nErr, ok := err.(net.Error); ok {
		_ = nErr
	}
	return false
}

// BuildErrorRemedyHints returns targeted, actionable CLI instructions based on the failure category.
func BuildErrorRemedyHints(err error) []string {
	if err == nil {
		return nil
	}

	msg := strings.ToLower(err.Error())
	var hints []string

	if strings.Contains(msg, "auth required") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "401") {
		hints = append(hints,
			"Run 'scandrix auth login' to authenticate with GitHub or GitLab.",
			"For CI/CD or headless environments, set SCANDRIX_API_KEY=scandrix_... or run 'scandrix auth team-key'.",
		)
	}

	if strings.Contains(msg, "not a git repository") || strings.Contains(msg, "not in git") {
		hints = append(hints,
			"Ensure you are inside the root or a subfolder of an active Git repository.",
			"Run 'git init' or change directory to your project workspace.",
		)
	}

	if strings.Contains(msg, "no changes to review") {
		hints = append(hints,
			"Modify some files or use 'scandrix review --branch <base-branch>' to review against a branch.",
			"Use 'scandrix review --commit <sha>' to inspect a specific historical commit.",
		)
	}

	if strings.Contains(msg, "rate limit") || strings.Contains(msg, "429") {
		hints = append(hints,
			"API rate limit exceeded. Please wait a moment before re-submitting.",
			"Upgrade your workspace plan or provide a custom LLM BYOK key in repository settings.",
		)
	}

	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "api request failed") {
		hints = append(hints,
			"Verify your SCANDRIX_API_URL environment variable or network firewall.",
			"Check service availability at https://status.scandrix.dev.",
		)
	}

	return hints
}

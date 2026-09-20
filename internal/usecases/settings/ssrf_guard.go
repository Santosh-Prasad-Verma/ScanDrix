package settings

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var (
	regionPattern = regexp.MustCompile(`^[a-z0-9-]{2,32}$`)

	privateIPBlocks []*net.IPNet
)

func init() {
	cidrs := []string{
		"127.0.0.0/8",    // Loopback
		"10.0.0.0/8",     // RFC1918 Private
		"172.16.0.0/12",  // RFC1918 Private
		"192.168.0.0/16", // RFC1918 Private
		"169.254.0.0/16", // Link-local / Cloud Metadata (169.254.169.254)
		"100.64.0.0/10",  // Carrier-grade NAT
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique local
		"fe80::/10",      // IPv6 link-local
	}

	for _, cidr := range cidrs {
		_, block, err := net.ParseCIDR(cidr)
		if err == nil {
			privateIPBlocks = append(privateIPBlocks, block)
		}
	}
}

// NewSafeHTTPClient returns an http.Client configured with socket-level IP pinning to block DNS rebinding.
func NewSafeHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	allowLoopback := isTestMode()

	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				host = address
			}
			ip := net.ParseIP(host)
			if ip != nil && isRestrictedIP(ip) && !(allowLoopback && (ip.IsLoopback() || ip.Equal(net.IPv4(127, 0, 0, 1)) || ip.Equal(net.IPv6loopback))) {
				return fmt.Errorf("connection to restricted IP %s blocked by SSRF defense", ip.String())
			}
			return nil
		},
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if isRestrictedIP(ip) && !(allowLoopback && (ip.IsLoopback() || ip.Equal(net.IPv4(127, 0, 0, 1)) || ip.Equal(net.IPv6loopback))) {
					return nil, fmt.Errorf("SSRF protection: IP %s is restricted", ip.String())
				}
			}
			if len(ips) == 0 {
				return nil, errors.New("no IP addresses found for host")
			}
			// Pin connection directly to verified IP
			pinnedAddr := net.JoinHostPort(ips[0].String(), port)
			return dialer.DialContext(ctx, network, pinnedAddr)
		},
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

// AssertSafeEndpoint ensures an external custom LLM endpoint does not probe internal or private infrastructure.
func AssertSafeEndpoint(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return errors.New("endpoint URL cannot be empty")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL format: %w", err)
	}

	if parsed.Scheme != "https" {
		return errors.New("endpoint URL must use HTTPS for enterprise security")
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return errors.New("endpoint URL missing valid hostname")
	}

	// Resolve hostname IPs
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("failed resolving endpoint hostname %q: %w", hostname, err)
	}

	for _, ip := range ips {
		if isRestrictedIP(ip) {
			return fmt.Errorf("endpoint resolves to restricted or private IP address (%s); must point to public provider", ip.String())
		}
	}

	return nil
}

// AssertSafeRegion validates cloud region naming against path traversal or command injection.
func AssertSafeRegion(region string) error {
	if region == "" {
		return nil
	}
	if !regionPattern.MatchString(region) {
		return fmt.Errorf("invalid cloud region %q: expected lowercase letters, numbers, or hyphens (2-32 chars)", region)
	}
	return nil
}

func isRestrictedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
		return true
	}
	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

func isTestMode() bool {
	if os.Getenv("GO_ENV") == "test" || os.Getenv("TESTING") == "true" || os.Getenv("API_NODE_ENV") == "test" || os.Getenv("SCANDRIX_ALLOW_LOCAL_LLM") == "true" {
		return true
	}
	return flag.Lookup("test.v") != nil
}

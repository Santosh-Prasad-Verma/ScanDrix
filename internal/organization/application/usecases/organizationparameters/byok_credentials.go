// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgparamusecases

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

var privateIPBlocks []*net.IPNet

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

func isRestrictedIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return true
	}
	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// MaskSecret masks an API key or sensitive secret string.
func MaskSecret(s string) string {
	if len(s) <= 6 {
		return "••••"
	}
	return s[:3] + "..." + s[len(s)-3:]
}

// IsMasked checks if a string is a masked placeholder.
func IsMasked(s string) bool {
	return s == "••••" || strings.Contains(s, "...")
}

// AssertSafeURL verifies that a target URL is not a loopback, link-local, or private IP address.
func AssertSafeURL(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("unsupported scheme %q: only http and https are permitted", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return errors.New("endpoint URL missing valid hostname")
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(host, ".localhost") {
		return errors.New("localhost URLs are rejected for enterprise safety")
	}

	if strings.HasSuffix(host, ".invalid") {
		return nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("failed resolving endpoint hostname %q: %w", host, err)
	}

	for _, ip := range ips {
		if isRestrictedIP(ip) {
			return fmt.Errorf("URL resolves to protected network address %s", ip.String())
		}
	}

	return nil
}

// NewSafeHTTPClient returns an http.Client configured with socket-level IP pinning to block DNS rebinding.
func NewSafeHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				host = address
			}
			ip := net.ParseIP(host)
			if ip != nil && isRestrictedIP(ip) {
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
				if isRestrictedIP(ip) {
					return nil, fmt.Errorf("SSRF protection: IP %s is restricted", ip.String())
				}
			}
			if len(ips) == 0 {
				return nil, errors.New("no IP addresses found for host")
			}
			// Pin connection directly to verified IP to prevent DNS rebinding
			pinnedAddr := net.JoinHostPort(ips[0].String(), port)
			return dialer.DialContext(ctx, network, pinnedAddr)
		},
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConns:        50,
		IdleConnTimeout:     60 * time.Second,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func resolveEncryptionKey() ([]byte, error) {
	keyStr := strings.TrimSpace(os.Getenv("SCANDRIX_ENCRYPTION_KEY"))
	if keyStr == "" {
		keyStr = strings.TrimSpace(os.Getenv("APP_ENCRYPTION_KEY"))
	}
	if len(keyStr) == 64 {
		decoded, err := hex.DecodeString(keyStr)
		if err == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}
	if len(keyStr) == 32 {
		return []byte(keyStr), nil
	}
	if keyStr != "" {
		h := sha256.Sum256([]byte(keyStr))
		return h[:], nil
	}
	// Deterministic derivation for test environments ensuring exact 32-byte requirement
	h := sha256.Sum256([]byte("scandrix-test-master-key-seed-32"))
	return h[:], nil
}

// EncryptSecret encrypts a secret with AES-256-GCM using SCANDRIX_ENCRYPTION_KEY.
func EncryptSecret(plaintext string) (string, error) {
	key, err := resolveEncryptionKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// DecryptSecret decrypts a hex-encoded AES-256-GCM ciphertext.
func DecryptSecret(cipherHex string) (string, error) {
	key, err := resolveEncryptionKey()
	if err != nil {
		return "", err
	}

	data, err := hex.DecodeString(cipherHex)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("malformed ciphertext")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

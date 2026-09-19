package utils

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var (
	absoluteSchemeRegex = regexp.MustCompile(`(?i)^https?://`)
	portOnlyRegex       = regexp.MustCompile(`^:\d+`)
)

// URLValidator enforces safe URL rules to prevent Server-Side Request Forgery (SSRF)
// and credential leakage when a baseURL is configured.
type URLValidator struct{}

// ValidateRelativeURL verifies that a target URL is strictly relative (not absolute or protocol-relative).
func ValidateRelativeURL(rawURL string) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return errors.New("URL must be a non-empty string")
	}

	// Reject absolute URLs (http://, https://)
	if absoluteSchemeRegex.MatchString(trimmed) {
		return errors.New("absolute URLs are not allowed when baseURL is configured; this prevents SSRF attacks and credential leakage")
	}

	// Reject protocol-relative URLs (//example.com)
	if strings.HasPrefix(trimmed, "//") {
		return errors.New("protocol-relative URLs are not allowed when baseURL is configured")
	}

	// Reject port-only URLs (:8080/path)
	if strings.HasPrefix(trimmed, ":") || portOnlyRegex.MatchString(trimmed) {
		return errors.New("invalid URL format: cannot start with port delimiter")
	}

	return nil
}

// IsAbsoluteURL checks if a URL includes an absolute http:// or https:// scheme.
func IsAbsoluteURL(rawURL string) bool {
	return absoluteSchemeRegex.MatchString(strings.TrimSpace(rawURL))
}

// IsProtocolRelativeURL checks if a URL begins with '//'.
func IsProtocolRelativeURL(rawURL string) bool {
	return strings.HasPrefix(strings.TrimSpace(rawURL), "//")
}

// ValidateAndSanitizeURL validates that a URL is relative and returns a clean trimmed representation.
func ValidateAndSanitizeURL(rawURL string) (string, error) {
	if err := ValidateRelativeURL(rawURL); err != nil {
		return "", err
	}

	trimmed := strings.TrimSpace(rawURL)
	// Parse with standard library to guarantee structural validity
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", errors.New("invalid URL structure")
	}

	if parsed.IsAbs() || parsed.Host != "" {
		return "", errors.New("absolute host or scheme detected in relative URL")
	}

	return trimmed, nil
}

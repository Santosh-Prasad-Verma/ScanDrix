package utils

import (
	"testing"
)

func TestURLValidator(t *testing.T) {
	// 1. Empty URL
	if err := ValidateRelativeURL(""); err == nil {
		t.Errorf("expected error for empty URL")
	}

	// 2. Absolute URLs
	absoluteCases := []string{
		"http://example.com/api",
		"https://example.com/api",
		"HTTP://EXAMPLE.COM/PATH",
		"https://169.254.169.254/latest/meta-data",
		"http://localhost:8080/admin",
	}
	for _, u := range absoluteCases {
		if err := ValidateRelativeURL(u); err == nil {
			t.Errorf("expected error for absolute URL: %s", u)
		}
		if !IsAbsoluteURL(u) {
			t.Errorf("expected IsAbsoluteURL to return true for: %s", u)
		}
	}

	// 3. Protocol-relative URLs
	protoRelativeCases := []string{
		"//evil.com/path",
		"//127.0.0.1:8080",
		"//internal.corp/admin",
	}
	for _, u := range protoRelativeCases {
		if err := ValidateRelativeURL(u); err == nil {
			t.Errorf("expected error for protocol-relative URL: %s", u)
		}
		if !IsProtocolRelativeURL(u) {
			t.Errorf("expected IsProtocolRelativeURL to return true for: %s", u)
		}
	}

	// 4. Port-only attempts
	portCases := []string{
		":8080/path",
		":443",
	}
	for _, u := range portCases {
		if err := ValidateRelativeURL(u); err == nil {
			t.Errorf("expected error for port-only URL: %s", u)
		}
	}

	// 5. Valid relative URLs
	validCases := []string{
		"/api/v1/users",
		"api/v1/repos",
		"/pulls/123/comments",
		"search?q=test&page=1",
		"/webhook/callback",
	}
	for _, u := range validCases {
		if err := ValidateRelativeURL(u); err != nil {
			t.Errorf("expected valid relative URL %s to pass, got: %v", u, err)
		}
		sanitized, err := ValidateAndSanitizeURL(u)
		if err != nil || sanitized != u {
			t.Errorf("expected sanitize to succeed for %s, got %s, err: %v", u, sanitized, err)
		}
	}
}

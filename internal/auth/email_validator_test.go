package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateRegistrationEmail(t *testing.T) {
	tests := []struct {
		name                 string
		email                string
		customBlockedDomains []string
		expectedErr          error
		expectedCanonical    string
	}{
		{
			name:              "Valid Gmail address",
			email:             "Developer@GMAIL.com",
			expectedErr:       nil,
			expectedCanonical: "Developer@gmail.com",
		},
		{
			name:              "Valid corporate domain",
			email:             "engineer@acmecorp.io",
			expectedErr:       nil,
			expectedCanonical: "engineer@acmecorp.io",
		},
		{
			name:        "Empty email string",
			email:       "",
			expectedErr: ErrInvalidEmailFormat,
		},
		{
			name:        "Missing domain part",
			email:       "developer@",
			expectedErr: ErrInvalidEmailFormat,
		},
		{
			name:        "Missing local part",
			email:       "@domain.com",
			expectedErr: ErrInvalidEmailFormat,
		},
		{
			name:        "Malformed syntax",
			email:       "not-an-email",
			expectedErr: ErrInvalidEmailFormat,
		},
		{
			name:        "Strict rejection of display-name smuggling",
			email:       "Attacker Name <victim@gmail.com>",
			expectedErr: ErrInvalidEmailFormat,
		},
		{
			name:              "Valid quoted local part containing @",
			email:             `"john@doe"@acmecorp.com`,
			expectedErr:       nil,
			expectedCanonical: `"john@doe"@acmecorp.com`,
		},
		{
			name:        "Mailinator disposable domain",
			email:       "attacker@mailinator.com",
			expectedErr: ErrDisposableEmailNotAllowed,
		},
		{
			name:        "Mailinator disposable subdomain",
			email:       "bot@sub.mailinator.com",
			expectedErr: ErrDisposableEmailNotAllowed,
		},
		{
			name:        "GuerrillaMail disposable domain",
			email:       "spammer@guerrillamail.com",
			expectedErr: ErrDisposableEmailNotAllowed,
		},
		{
			name:        "Sharklasers disposable domain",
			email:       "sybil@sharklasers.com",
			expectedErr: ErrDisposableEmailNotAllowed,
		},
		{
			name:                 "Custom blocked domain match",
			email:                "test@maliciouscorp.org",
			customBlockedDomains: []string{"maliciouscorp.org", "spamdomain.net"},
			expectedErr:          ErrDisposableEmailNotAllowed,
		},
		{
			name:                 "Custom blocked subdomain match",
			email:                "test@sub.maliciouscorp.org",
			customBlockedDomains: []string{"maliciouscorp.org"},
			expectedErr:          ErrDisposableEmailNotAllowed,
		},
		{
			name:                 "Allowed domain when other custom domains blocked",
			email:                "legit@legitcorp.org",
			customBlockedDomains: []string{"maliciouscorp.org"},
			expectedErr:          nil,
			expectedCanonical:    "legit@legitcorp.org",
		},
		{
			name:        "Exceeds max RFC 5321 length",
			email:       strings.Repeat("a", 250) + "@domain.com",
			expectedErr: ErrInvalidEmailFormat,
		},
		{
			name:        "Exceeds local part 64 octet limit",
			email:       strings.Repeat("a", 65) + "@domain.com",
			expectedErr: ErrInvalidEmailFormat,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			canonical, err := ValidateRegistrationEmail(tc.email, tc.customBlockedDomains)
			if tc.expectedErr == nil {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				if canonical != tc.expectedCanonical {
					t.Fatalf("expected canonical %q, got %q", tc.expectedCanonical, canonical)
				}
			} else {
				if !errors.Is(err, tc.expectedErr) {
					t.Fatalf("expected error %v, got %v", tc.expectedErr, err)
				}
			}
		})
	}
}

func TestDisposableDomainAudit(t *testing.T) {
	// Assert that every entry in defaultDisposableDomains contains at least one dot separating a TLD
	for domain := range defaultDisposableDomains {
		if !strings.Contains(domain, ".") {
			t.Fatalf("invalid disposable domain entry without TLD dot: %q", domain)
		}
		if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
			t.Fatalf("invalid disposable domain entry with leading/trailing dot: %q", domain)
		}
	}
}

package controllers

import (
	"os"
	"testing"
)

// TestCLIPublicRegistrationIsClosedByDefault pins AUDIT_REMEDIATION.md F-13.
//
// The CLI approve endpoint is unauthenticated and is deliberately exempt from
// the registration rate limiter. It can create an account with role "owner"
// plus a fresh workspace, so when it is open it is an unthrottled
// owner-account factory for anyone who knows the URL.
//
// The policy used to be opt-out: with ALLOW_PUBLIC_CLI_REGISTRATION unset the
// code fell straight through to account creation.
func TestCLIPublicRegistrationIsClosedByDefault(t *testing.T) {
	const allowKey = "ALLOW_PUBLIC_CLI_REGISTRATION"
	const strictKey = "AUTH_STRICT_INVITES_ONLY"

	// Isolate from the developer's own environment.
	t.Setenv(allowKey, "")
	t.Setenv(strictKey, "")
	if err := os.Unsetenv(allowKey); err != nil {
		t.Fatalf("unset %s: %v", allowKey, err)
	}
	if err := os.Unsetenv(strictKey); err != nil {
		t.Fatalf("unset %s: %v", strictKey, err)
	}

	if cliPublicRegistrationAllowed() {
		t.Fatalf("public CLI registration must be closed when %s is unset", allowKey)
	}

	tests := []struct {
		name   string
		allow  string
		strict string
		want   bool
	}{
		{"unset", "", "", false},
		{"explicitly false", "false", "", false},
		{"nonsense value is not an opt-in", "yes", "", false},
		{"one is not true", "1", "", false},
		{"explicit opt-in", "true", "", true},
		{"case insensitive opt-in", "TRUE", "", true},
		{"strict invites only overrides opt-in", "true", "true", false},
		{"strict invites only alone", "", "true", false},
		{"strict one is not true", "true", "1", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.allow == "" {
				_ = os.Unsetenv(allowKey)
			} else {
				t.Setenv(allowKey, tc.allow)
			}
			if tc.strict == "" {
				_ = os.Unsetenv(strictKey)
			} else {
				t.Setenv(strictKey, tc.strict)
			}

			if got := cliPublicRegistrationAllowed(); got != tc.want {
				t.Fatalf("cliPublicRegistrationAllowed() = %v, want %v (allow=%q strict=%q)",
					got, tc.want, tc.allow, tc.strict)
			}
		})
	}
}

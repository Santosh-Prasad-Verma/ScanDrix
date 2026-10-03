package auth

import "testing"

// TestValidatePasswordRejectsWeak pins AUDIT_REMEDIATION.md F-21.
//
// The only rule was `len(password) < 8`, so `password1` was accepted on every
// path that set a password. The audit called out ASVS V2.1.1: blocklist
// breached and predictable values, rather than relying on composition rules
// that push users toward `Password1!`.
func TestValidatePasswordRejectsWeak(t *testing.T) {
	rejected := []struct {
		name     string
		password string
		want     error
	}{
		{"empty", "", ErrPasswordTooShort},
		{"too short", "Ab3!x", ErrPasswordTooShort},
		// The audit's example. It is only 9 characters, so the length rule
		// fires first; either rejection is correct, and the length message is
		// the more useful one to show.
		{"the audit's example", "password1", ErrPasswordTooShort},
		{"blocklist exact", "Password123", ErrPasswordTooShort},
		{"de-leeted", "p@ssw0rd", ErrPasswordTooShort},
		{"long blocklisted value", "Password1234!", ErrPasswordInBlocklist},
		{"long de-leeted value", "p@ssw0rd-2024", ErrPasswordInBlocklist},
		{"numeric run", "123456789012", ErrPasswordTooSimple},
		{"blocklist substring", "myP@ssw0rd-2024", ErrPasswordInBlocklist},
		{"qwerty substring", "aQwertyThing", ErrPasswordInBlocklist},
		{"single repeated char", "aaaaaaaaaaaa", ErrPasswordTooSimple},
		{"long symbol run", "hunter2!!!!!!", ErrPasswordTooSimple},
		{"too long", string(make([]byte, 200)), ErrPasswordExceedsMax},
	}

	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidatePassword(tc.password); err != tc.want {
				t.Fatalf("ValidatePassword(%q) = %v, want %v", tc.password, err, tc.want)
			}
		})
	}
}

// TestValidatePasswordRejectsAccountIdentifiers ensures a password cannot be
// the account's own email or workspace name, which are public.
func TestValidatePasswordRejectsAccountIdentifiers(t *testing.T) {
	const email = "jordan.blake@example.com"
	if err := ValidatePassword("jordan.blake@example.com", email); err == nil {
		t.Fatal("the password must not be the account email")
	}
	if err := ValidatePassword("jordan.blake-91x", email); err == nil {
		t.Fatal("the password must not contain the email local part")
	}
	if err := ValidatePassword("Acme-Corp-Team-9", "someone@example.com", "Acme Corp Team"); err == nil {
		t.Fatal("the password must not contain the workspace name")
	}
	// A very short identifier must not disqualify an unrelated strong password.
	if err := ValidatePassword("Zq7-mv2-Kd4-pL9", "a", "jo"); err != nil {
		t.Fatalf("short identifiers must not cause a rejection: %v", err)
	}
}

// TestValidatePasswordAcceptsStrong checks the policy does not become
// unreasonable. Multi-byte passphrases count by rune, not byte, so a
// non-Latin passphrase is not penalised for its encoding.
func TestValidatePasswordAcceptsStrong(t *testing.T) {
	accepted := []struct {
		name     string
		password string
	}{
		{"12 chars mixed", "Zr7-kq2-Vn4p"},
		{"long passphrase", "correct horse battery staple"},
		{"passphrase with spaces", "the quick brown fox jumps over"},
		{"multi-byte passphrase", "красиво-большой-пароль-здесь"},
		{"no composition rules required", "alllowercaseonlyhere"},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidatePassword(tc.password); err != nil {
				t.Fatalf("ValidatePassword(%q) = %v, want nil", tc.password, err)
			}
		})
	}
}

// TestPasswordLengthCountsRunes documents that length is measured in runes.
func TestPasswordLengthCountsRunes(t *testing.T) {
	// 12 multi-byte runes: 12 characters but far more than 12 bytes.
	pass := "пароль-длинный"
	if err := ValidatePassword(pass); err != nil {
		t.Fatalf("a 12-rune passphrase must satisfy the 12-character minimum: %v", err)
	}
	// 10 runes must still fail, however many bytes they occupy.
	if err := ValidatePassword("пароль-кор"); err != ErrPasswordTooShort {
		t.Fatalf("a 10-rune passphrase must be rejected, got %v", err)
	}
}

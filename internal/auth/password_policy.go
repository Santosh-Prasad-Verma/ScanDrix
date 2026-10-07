package auth

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// Password policy (AUDIT_REMEDIATION.md F-21).
//
// The only rule was `len(password) < 8`, applied in two places, so `password1`
// was accepted everywhere.
//
// Following OWASP ASVS V2.1.1 and NIST SP 800-63B-1:
//
//   - Length is the primary control. 12 characters is the floor here because
//     this product authenticates with a password alone, with no MFA enforced at
//     registration.
//   - Composition rules (must contain a digit, a symbol, mixed case) are NOT
//     enforced. The standards are explicit that they push users toward
//     predictable patterns like `Password1!` without meaningfully raising
//     resistance to guessing, and they cause people to reuse passwords.
//   - A blocklist of passwords known to be breached or trivially guessable IS
//     enforced, because that catches exactly the cases length alone misses.

const (
	// MinPasswordLength is the floor for a new password.
	MinPasswordLength = 12
	// MaxPasswordLength bounds the work an attacker forces per guess. Long
	// inputs are truncated for hashing cost, matching the bcrypt limit already
	// enforced in HashPassword.
	MaxPasswordLength = 128
)

var (
	ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	// Distinct from password.go's ErrPasswordTooLong, which reports the bcrypt
	// 72-byte hash limit rather than the policy maximum.
	ErrPasswordExceedsMax  = fmt.Errorf("password must be at most %d characters", MaxPasswordLength)
	ErrPasswordInBlocklist         = errors.New("this password is too common; choose something not used elsewhere")
	ErrPasswordTooSimple           = errors.New("password is too predictable; avoid repeated characters and simple sequences")
	ErrPasswordContainsIdentifier = errors.New("password cannot be based on your name, email, or workspace name")
)

// commonPasswords is a deliberately small, offline list. It is not a
// substitute for checking a real breach corpus (see HaveIBeenPwned below), but
// it costs nothing, needs no network at registration, and covers the passwords
// that actually show up in credential-stuffing lists.
//
// Entries are lower-case and compared case-insensitively.
var commonPasswords = map[string]bool{
	"password": true, "password1": true, "password12": true, "password123": true,
	"password1234": true, "passw0rd": true, "p@ssword": true, "p@ssw0rd": true,
	"123456": true, "1234567": true, "12345678": true, "123456789": true, "1234567890": true,
	"12345678901": true, "123123123": true, "11111111": true, "00000000": true,
	"qwerty": true, "qwertyui": true, "qwerty123": true, "qwertyuiop": true,
	"letmein": true, "welcome": true, "welcome1": true, "admin": true, "admin123": true,
	"administrator": true, "root": true, "root123": true, "toor": true,
	"iloveyou": true, "princess": true, "sunshine": true, "football": true, "baseball": true,
	"dragon": true, "monkey": true, "shadow": true, "master": true, "superman": true,
	"trustno1": true, "starwars": true, "whatever": true, "freedom": true,
	"changeme": true, "change_me": true, "please": true, "secret": true, "secret123": true,
	"company": true, "company123": true, "scandrix": true, "scandrix123": true,
	"test": true, "test123": true, "test1234": true, "testing": true, "testing123": true,
	"default": true, "guest": true, "login": true, "abc123": true, "abcd1234": true,
	"qwerty1": true, "asdfgh": true, "asdf1234": true, "zaq12wsx": true, "!@#$%^&*": true,
}

// commonPasswordSubstrings are fragments that, combined with anything, give
// away the shape of a real password. Checked as a substring so
// "Summer2024Passw0rd" is rejected too.
var commonPasswordSubstrings = []string{
	"password", "passw0rd", "qwerty", "letmein", "welcome",
	"admin", "iloveyou", "sunshine", "football", "monkey", "dragon",
}

// sequentialRunes are runs that add no entropy even though they add length.
var sequentialRunes = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

// repeatedRunes are characters commonly used to reach a length target.
var repeatedRunes = []rune("!@#$%^&*()-_=+.,;:'\"\\/ ")

// ValidatePassword enforces the password policy.
//
// The optional identifiers (typically the email address and the workspace
// name) are checked so a password cannot be the account's own name; they are
// optional because not every caller has them.
func ValidatePassword(password string, identifiers ...string) error {
	// Length is measured in runes, not bytes, so a multi-byte passphrase is not
	// penalised for its encoding.
	n := len([]rune(password))
	switch {
	case n == 0:
		return ErrPasswordTooShort
	case n < MinPasswordLength:
		return ErrPasswordTooShort
	case n > MaxPasswordLength:
		return ErrPasswordExceedsMax
	}

	lower := strings.ToLower(password)

	// Exact blocklist, plus a de-leeted comparison so `P@ssw0rd` cannot slip
	// past an entry for `password`.
	if commonPasswords[lower] {
		return ErrPasswordInBlocklist
	}
	if commonPasswords[deLeet(lower)] {
		return ErrPasswordInBlocklist
	}
	// Checked against the de-leeted form as well, so `p@ssw0rd-2024` is caught
	// by the "password" fragment the same way `myP@ssw0rd` is.
	deletted := deLeet(lower)
	for _, frag := range commonPasswordSubstrings {
		if strings.Contains(lower, frag) || strings.Contains(deletted, frag) {
			return ErrPasswordInBlocklist
		}
	}

	// Must contain at least one non-whitespace character class that is not a
	// single repeated character. Long runs of one character are a length
	// target, not a secret.
	if isSingleRepeatedRune(password) {
		return ErrPasswordTooSimple
	}

	// Reject the account's own identifiers, ignoring a very short one so "a" in
	// `a-user@example.com` is not disqualifying on its own.
	//
	// Comparison happens on alphanumeric-only forms of both sides, so a
	// workspace named "Acme Corp Team" is still caught by `Acme-Corp-Team-9`.
	// A password that merely contains an identifier is permitted if the
	// non-identifier remainder still provides sufficient independent entropy
	// (at least MinPasswordLength non-identifier alphanumeric characters).
	flat := alphanumericOnly(lower)
	for _, id := range identifiers {
		id = strings.ToLower(strings.TrimSpace(id))
		if len([]rune(id)) < 4 {
			continue
		}
		if flatIdentifier := alphanumericOnly(id); flatIdentifier != "" && len([]rune(flatIdentifier)) >= 4 {
			if strings.Contains(flat, flatIdentifier) {
				remainder := strings.ReplaceAll(flat, flatIdentifier, "")
				if len([]rune(remainder)) < MinPasswordLength {
					return ErrPasswordContainsIdentifier
				}
			}
		}
		// The local part of an email address is the most guessable piece.
		if at := strings.Index(id, "@"); at > 0 {
			if local := alphanumericOnly(id[:at]); len([]rune(local)) >= 4 {
				if strings.Contains(flat, local) {
					remainder := strings.ReplaceAll(flat, local, "")
					if len([]rune(remainder)) < MinPasswordLength {
						return ErrPasswordContainsIdentifier
					}
				}
			}
		}
	}

	// A long run of one character, or an unbroken ascending/descending
	// sequence, adds length without adding entropy.
	if hasLongRun(password, repeatedRunes, 6) {
		return ErrPasswordTooSimple
	}
	if hasLongRun(password, sequentialRunes, 6) {
		return ErrPasswordTooSimple
	}
	// A repeated-character run has to be checked separately: hasLongRun looks
	// for a step of +/-1 between distinct members of the set, so "!!!!!!" is
	// invisible to it even though it is six characters of nothing.
	if hasRepeatedRun(password, 6) {
		return ErrPasswordTooSimple
	}

	return nil
}

// alphanumericOnly lower-cases and strips everything that is not a letter or
// digit, so separator and symbol differences do not hide a match.
func alphanumericOnly(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// deLeet folds the substitutions people use to defeat naive blocklists.
func deLeet(s string) string {
	r := strings.NewReplacer("0", "o", "1", "i", "3", "e", "4", "a", "5", "s", "7", "t", "@", "a", "$", "s", "!", "i")
	return r.Replace(s)
}

// isSingleRepeatedRune reports whether the password is one character repeated.
func isSingleRepeatedRune(s string) bool {
	r := []rune(s)
	if len(r) < MinPasswordLength {
		return false
	}
	for _, c := range r[1:] {
		if c != r[0] {
			return false
		}
	}
	return true
}

// hasRepeatedRun reports whether s contains at least `n` consecutive copies
// of the same rune.
func hasRepeatedRun(s string, n int) bool {
	run, prev, have := 0, rune(0), false
	for _, c := range s {
		if have && c == prev {
			run++
		} else {
			run = 1
		}
		prev, have = c, true
		if run >= n {
			return true
		}
	}
	return false
}

// hasLongRun reports whether s contains at least `n` consecutive runes drawn
// from the given set, in either ascending or descending order.
func hasLongRun(s string, set []rune, n int) bool {
	idx := make(map[rune]int, len(set))
	for i, c := range set {
		idx[c] = i
	}
	run, prev, have := 1, 0, false
	for _, c := range s {
		i, ok := idx[c]
		if !ok {
			run, have = 1, false
			continue
		}
		if have && (i == prev+1 || i == prev-1) {
			run++
		} else {
			run = 1
		}
		prev, have = i, true
		if run >= n {
			return true
		}
	}
	return false
}

// passwordHasSymbolOrDigit reports whether a password mixes character classes.
// It exists for callers that want to log a hint; it is deliberately not part of
// the rejection rules.
func passwordHasSymbolOrDigit(s string) bool {
	var hasLetter, hasOther bool
	for _, r := range s {
		if unicode.IsLetter(r) {
			hasLetter = true
			continue
		}
		if unicode.IsDigit(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			hasOther = true
		}
	}
	return hasLetter && hasOther
}

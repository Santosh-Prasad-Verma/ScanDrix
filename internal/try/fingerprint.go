package try

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
)

// StorageKeyFingerprint provides the client storage key for device fingerprinting.
const StorageKeyFingerprint = "scandrix-try-fingerprint"

// GenerateFingerprint generates a cryptographically secure 128-bit random fingerprint string.
func GenerateFingerprint() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// ValidateFingerprint verifies that the provided fingerprint meets security and length requirements.
func ValidateFingerprint(fingerprint string) error {
	clean := strings.TrimSpace(fingerprint)
	if clean == "" {
		return errors.New("missing fingerprint")
	}
	if len(clean) > 256 {
		return errors.New("fingerprint exceeds maximum allowed length of 256 characters")
	}
	return nil
}

// NormalizeFingerprint trims whitespace and sanitizes the client fingerprint identifier.
func NormalizeFingerprint(fingerprint string) string {
	return strings.TrimSpace(fingerprint)
}

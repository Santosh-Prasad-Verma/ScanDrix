package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	KeyPrefixLive   = "ch_live_"
	PrefixLength    = 6
	SecretLength    = 32
	TotalMinLength  = len(KeyPrefixLive) + PrefixLength + 1 + SecretLength
)

// GeneratedKey holds the raw token (only shown once to user) and its database-storable attributes.
type GeneratedKey struct {
	RawKey    string `json:"raw_key"`
	KeyPrefix string `json:"key_prefix"`
	KeyHash   string `json:"key_hash"`
}

// GenerateAPIKey creates a cryptographically secure, high-entropy CodeHound API key.
// Format: ch_live_<12_hex_prefix>_<32_hex_secret>
func GenerateAPIKey() (*GeneratedKey, error) {
	prefixBytes := make([]byte, PrefixLength/2)
	if _, err := rand.Read(prefixBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random prefix: %w", err)
	}
	prefixPart := hex.EncodeToString(prefixBytes)

	secretBytes := make([]byte, SecretLength/2)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random secret: %w", err)
	}
	secretPart := hex.EncodeToString(secretBytes)

	keyPrefix := KeyPrefixLive + prefixPart
	rawKey := fmt.Sprintf("%s_%s", keyPrefix, secretPart)
	keyHash := HashKey(rawKey)

	return &GeneratedKey{
		RawKey:    rawKey,
		KeyPrefix: keyPrefix,
		KeyHash:   keyHash,
	}, nil
}

// HashKey computes the SHA-256 cryptographic hash of a raw API key.
func HashKey(rawKey string) string {
	h := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(h[:])
}

// VerifyKey performs constant-time comparison to prevent timing attacks.
func VerifyKey(rawKey, expectedKeyHash string) bool {
	computedHash := HashKey(rawKey)
	return subtle.ConstantTimeCompare([]byte(computedHash), []byte(expectedKeyHash)) == 1
}

// ExtractPrefix parses the identifying prefix from a raw key string.
func ExtractPrefix(rawKey string) (string, error) {
	if !strings.HasPrefix(rawKey, KeyPrefixLive) {
		return "", fmt.Errorf("invalid key format: missing '%s' prefix", KeyPrefixLive)
	}
	parts := strings.Split(rawKey, "_")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid key structure")
	}
	return parts[0] + "_" + parts[1] + "_" + parts[2], nil
}

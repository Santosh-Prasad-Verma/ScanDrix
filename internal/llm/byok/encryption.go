// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package byok

import (
	"crypto/sha256"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/scandrix/backend/pkg/crypto"
)

var encryptionKeyOnce sync.Once
var encryptionKey []byte

// GetEncryptionKey resolves the 32-byte AES key from environment variables.
// In production, panics if SCANDRIX_ENCRYPTION_KEY is not set.
// In development (APP_ENV=development), logs a warning and uses a deterministic dev-only key.
func GetEncryptionKey() []byte {
	encryptionKeyOnce.Do(func() {
		keyStr := os.Getenv("SCANDRIX_ENCRYPTION_KEY")
		if keyStr == "" {
			env := strings.ToLower(os.Getenv("APP_ENV"))
			if env == "" || env == "development" || env == "test" {
				slog.Warn("SCANDRIX_ENCRYPTION_KEY not set — using insecure dev-only key. DO NOT use in production.")
				keyStr = "scandrix-dev-only-insecure-key-never-use-in-prod"
			} else {
				panic("FATAL: SCANDRIX_ENCRYPTION_KEY environment variable must be set in production. " +
					"All BYOK API keys are encrypted with this key. " +
					"Generate a random 32+ character string and set it before starting the server.")
			}
		}
		hash := sha256.Sum256([]byte(keyStr))
		encryptionKey = hash[:]
	})
	return encryptionKey
}

// EncryptKey encrypts a sensitive API key or secret for persistent storage.
func EncryptKey(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	return crypto.EncryptStringAESGCM(GetEncryptionKey(), plaintext)
}

// DecryptKey decrypts ciphertext into plaintext for immediate local execution.
func DecryptKey(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	// If it doesn't look like base64 or is already a plaintext key (e.g. env var starting with sk- or similar)
	decrypted, err := crypto.DecryptStringAESGCM(GetEncryptionKey(), ciphertext)
	if err != nil {
		// Fallback: return as-is if raw unencrypted key was passed (e.g. from env)
		return ciphertext, nil
	}
	return decrypted, nil
}

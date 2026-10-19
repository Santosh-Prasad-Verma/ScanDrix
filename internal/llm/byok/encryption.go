// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package byok

import (
	"context"
	"crypto/sha256"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/scandrix/backend/internal/security/kms"
	"github.com/scandrix/backend/pkg/crypto"
)

var encryptionKeyOnce sync.Once
var encryptionKey []byte
var kmsEngineOnce sync.Once
var kmsEngine *kms.KMSEnvelopeEngine

func getKMSEnvelopeEngine() *kms.KMSEnvelopeEngine {
	kmsEngineOnce.Do(func() {
		k := GetEncryptionKey()
		if len(k) > 0 {
			if staticKMS, err := kms.NewStaticKeyKMS(k, "scandrix-byok-master-key"); err == nil {
				kmsEngine = kms.NewKMSEnvelopeEngine(staticKMS)
			}
		}
	})
	return kmsEngine
}

// GetEncryptionKey resolves the 32-byte AES key from environment variables.
// In production, panics if SCANDRIX_ENCRYPTION_KEY is not set.
// In development (APP_ENV=development), logs a warning and uses a deterministic dev-only key.
func GetEncryptionKey() []byte {
	encryptionKeyOnce.Do(func() {
		keyStr := os.Getenv("SCANDRIX_ENCRYPTION_KEY")
		if keyStr == "" {
			keyStr = os.Getenv("SCANDRIX_MASTER_ENCRYPTION_KEY")
		}
		if keyStr == "" {
			keyStr = os.Getenv("DATABASE_ENCRYPTION_KEY")
		}
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

// EncryptKeyEnvelope encrypts a sensitive key using 2-tier KMS envelope encryption with ephemeral DEKs.
func EncryptKeyEnvelope(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if engine := getKMSEnvelopeEngine(); engine != nil {
		return engine.EncryptString(context.Background(), "scandrix-byok-master-key", plaintext)
	}
	return EncryptKey(plaintext)
}

// DecryptKey decrypts ciphertext into plaintext for immediate local execution.
// Supports both 2-tier KMS envelope JSON (NIST SP 800-57) and AES-256-GCM base64 ciphertext.
func DecryptKey(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	trimmed := strings.TrimSpace(ciphertext)
	// 1. Check for 2-tier KMS Envelope JSON
	if strings.HasPrefix(trimmed, "{") {
		if engine := getKMSEnvelopeEngine(); engine != nil {
			if dec, err := engine.DecryptString(context.Background(), trimmed); err == nil && dec != "" {
				return dec, nil
			}
		}
	}
	// 2. Standard AES-256-GCM base64
	decrypted, err := crypto.DecryptStringAESGCM(GetEncryptionKey(), ciphertext)
	if err != nil {
		// Fallback: return as-is if raw unencrypted key was passed (e.g. from env)
		return ciphertext, nil
	}
	return decrypted, nil
}

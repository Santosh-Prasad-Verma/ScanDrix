// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp_manager_test

import (
	"testing"

	"github.com/scandrix/backend/internal/mcp/manager/crypto"
)

func TestCryptoEncryptDecryptRoundTrip(t *testing.T) {
	secret := generateRandomTestKey()
	encryptor, err := crypto.NewEncryptor(secret)
	if err != nil {
		t.Fatalf("Failed creating encryptor: %v", err)
	}

	payloads := []string{
		"super_secret_github_token_ghp_1234567890",
		`{"apiKey":"lin_api_abcdef123456","user":"developer@scandrix.io"}`,
		"simple_text",
		"a",
		`{"tokens":{"accessToken":"eyJhbGciOi...","refreshToken":"refresh_123","expiresAt":1790000000}}`,
	}

	for _, original := range payloads {
		encrypted, err := encryptor.Encrypt(original)
		if err != nil {
			t.Fatalf("Encrypt failed for payload: %v", err)
		}

		if encrypted == original {
			t.Fatalf("Encrypted payload should not match original plaintext")
		}

		decrypted, err := encryptor.Decrypt(encrypted)
		if err != nil {
			t.Fatalf("Decrypt failed for payload: %v", err)
		}

		if decrypted != original {
			t.Fatalf("Decrypted does not match original. Expected: %s, Got: %s", original, decrypted)
		}
	}
}

func TestCryptoTamperedCiphertext(t *testing.T) {
	secret := generateRandomTestKey()
	encryptor, err := crypto.NewEncryptor(secret)
	if err != nil {
		t.Fatalf("Failed creating encryptor: %v", err)
	}

	encrypted, err := encryptor.Encrypt("sensitive_access_token")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Tamper with ciphertext
	tampered := encrypted[:len(encrypted)-4] + "AAAA"
	_, err = encryptor.Decrypt(tampered)
	if err == nil {
		t.Fatalf("Expected error when decrypting tampered ciphertext, got nil")
	}
}

func TestCryptoDifferentKeys(t *testing.T) {
	encryptor1, _ := crypto.NewEncryptor(generateRandomTestKey())
	encryptor2, _ := crypto.NewEncryptor(generateRandomTestKey())

	encrypted, err := encryptor1.Encrypt("secret-token")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = encryptor2.Decrypt(encrypted)
	if err == nil {
		t.Fatalf("Decryption with different key should fail")
	}
}

package crypto_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/scandrix/backend/pkg/crypto"
)

func TestAESGCMEncryptionAndDecryption(t *testing.T) {
	key, err := crypto.GenerateSecureRandomBytes(32)
	if err != nil {
		t.Fatalf("failed generating key: %v", err)
	}

	plaintext := []byte("secret payload for enterprise code review: token=kodus_live_12345")

	// 1. Encrypt
	ciphertext, nonce, err := crypto.EncryptAESGCM(key, plaintext)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}
	if bytes.Equal(ciphertext, plaintext) {
		t.Fatalf("ciphertext must not match plaintext")
	}

	// 2. Decrypt with valid key and nonce
	decrypted, err := crypto.DecryptAESGCM(key, ciphertext, nonce)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted text %q does not match original plaintext %q", string(decrypted), string(plaintext))
	}

	// 3. Tampering: Decrypt with corrupted ciphertext must fail
	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[0] ^= 0xFF
	_, err = crypto.DecryptAESGCM(key, tampered, nonce)
	if err == nil {
		t.Fatalf("expected decryption error on tampered ciphertext")
	}
}

func TestHMACSHA256Verification(t *testing.T) {
	secret := []byte("webhook_secret_key_999")
	payload := []byte(`{"action":"opened","pull_request":{"id":42}}`)

	sig := crypto.ComputeHMACSHA256(secret, payload)
	if sig == "" {
		t.Fatalf("expected non-empty signature")
	}

	// Valid verification
	if !crypto.VerifyHMACSHA256(secret, payload, sig) {
		t.Fatalf("expected valid signature verification to pass")
	}

	// Invalid signature
	if crypto.VerifyHMACSHA256(secret, payload, "deadbeef1234") {
		t.Fatalf("expected invalid signature verification to fail")
	}

	// Tampered payload
	if crypto.VerifyHMACSHA256(secret, []byte(`{"action":"closed"}`), sig) {
		t.Fatalf("expected tampered payload verification to fail")
	}
}

func TestGenerateSecureToken(t *testing.T) {
	token, err := crypto.GenerateSecureToken("kodus_", 24)
	if err != nil {
		t.Fatalf("failed generating token: %v", err)
	}

	if !strings.HasPrefix(token, "kodus_") {
		t.Fatalf("expected token prefix 'kodus_', got %s", token)
	}
	if len(token) < 20 {
		t.Fatalf("token too short: %s", token)
	}
}

func TestFingerprintSHA256(t *testing.T) {
	fp1 := crypto.FingerprintSHA256("code_finding_101")
	fp2 := crypto.FingerprintSHA256("code_finding_101")
	fp3 := crypto.FingerprintSHA256("code_finding_102")

	if fp1 != fp2 {
		t.Fatalf("fingerprint must be deterministic")
	}
	if fp1 == fp3 {
		t.Fatalf("different inputs must produce different fingerprints")
	}
}

func TestReEncryptSecretAndBatch(t *testing.T) {
	oldKey, _ := crypto.GenerateSecureRandomBytes(32)
	newKey, _ := crypto.GenerateSecureRandomBytes(32)

	plaintext := []byte("api_key_sk_ant_12345678")
	cipherOld, nonceOld, err := crypto.EncryptAESGCM(oldKey, plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// 1. Single Re-encrypt
	cipherNew, nonceNew, err := crypto.ReEncryptSecret(oldKey, newKey, cipherOld, nonceOld)
	if err != nil {
		t.Fatalf("re-encrypt failed: %v", err)
	}

	decrypted, err := crypto.DecryptAESGCM(newKey, cipherNew, nonceNew)
	if err != nil {
		t.Fatalf("decrypt with new key failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("expected %q, got %q", string(plaintext), string(decrypted))
	}

	// 2. Batch Re-encrypt
	records := []crypto.EncryptedRecord{
		{ID: "rec-1", Ciphertext: cipherOld, Nonce: nonceOld},
	}
	reencrypted, err := crypto.BatchReEncrypt(oldKey, newKey, records)
	if err != nil {
		t.Fatalf("batch re-encrypt failed: %v", err)
	}
	if len(reencrypted) != 1 {
		t.Fatalf("expected 1 record, got %d", len(reencrypted))
	}
}


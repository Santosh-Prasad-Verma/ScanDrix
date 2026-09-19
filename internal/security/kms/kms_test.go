package kms_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/scandrix/backend/internal/security/kms"
)

func TestKMSEnvelopeEncryptionAndRotation(t *testing.T) {
	ctx := context.Background()
	keyID := "scandrix-byok-master-key"

	kmsProvider, err := kms.NewLocalMemoryKMS(keyID)
	if err != nil {
		t.Fatalf("failed initializing KMS provider: %v", err)
	}

	engine := kms.NewKMSEnvelopeEngine(kmsProvider)

	originalSecret := []byte("github_pat_11AEXAMPLE_SUPER_SECRET_TOKEN_DO_NOT_LEAK")

	// 1. Initial Encryption with Version 1
	env1, err := engine.Encrypt(ctx, keyID, originalSecret)
	if err != nil {
		t.Fatalf("envelope encryption failed: %v", err)
	}

	if env1.KeyVersion != 1 {
		t.Fatalf("expected version 1, got %d", env1.KeyVersion)
	}

	decrypted1, err := engine.Decrypt(ctx, env1)
	if err != nil {
		t.Fatalf("envelope decryption failed: %v", err)
	}
	if !bytes.Equal(decrypted1, originalSecret) {
		t.Fatalf("decrypted mismatch: got %s, want %s", string(decrypted1), string(originalSecret))
	}

	// 2. Proves DEK uniqueness: Second encryption produces distinct DEK and Ciphertext
	env2, err := engine.Encrypt(ctx, keyID, originalSecret)
	if err != nil {
		t.Fatalf("second encryption failed: %v", err)
	}
	if bytes.Equal(env1.EncryptedDEK, env2.EncryptedDEK) {
		t.Fatal("DEK collision: expected distinct ephemeral DEK per encryption")
	}
	if bytes.Equal(env1.Ciphertext, env2.Ciphertext) {
		t.Fatal("Ciphertext collision: expected distinct ciphertext per encryption")
	}

	// 3. Master Key Rotation: Rotate to Version 2
	newVersion, err := kmsProvider.Rotate(ctx, keyID)
	if err != nil {
		t.Fatalf("master key rotation failed: %v", err)
	}
	if newVersion != 2 {
		t.Fatalf("expected rotated version 2, got %d", newVersion)
	}

	// Historical envelope (version 1) must remain decryptable!
	historicalDecrypted, err := engine.Decrypt(ctx, env1)
	if err != nil {
		t.Fatalf("historical envelope decryption failed after rotation: %v", err)
	}
	if !bytes.Equal(historicalDecrypted, originalSecret) {
		t.Fatalf("historical decrypted mismatch: got %s, want %s", string(historicalDecrypted), string(originalSecret))
	}

	// New encryptions must use Version 2
	env3, err := engine.Encrypt(ctx, keyID, originalSecret)
	if err != nil {
		t.Fatalf("encryption after rotation failed: %v", err)
	}
	if env3.KeyVersion != 2 {
		t.Fatalf("expected new encryption to use version 2, got %d", env3.KeyVersion)
	}

	// 4. Re-Encryption / Upgrade Envelope to latest version
	upgradedEnv1, changed, err := engine.ReEncrypt(ctx, env1)
	if err != nil {
		t.Fatalf("re-encryption failed: %v", err)
	}
	if !changed || upgradedEnv1.KeyVersion != 2 {
		t.Fatalf("expected upgraded envelope version 2, got %d (changed: %v)", upgradedEnv1.KeyVersion, changed)
	}

	upgradedDecrypted, err := engine.Decrypt(ctx, upgradedEnv1)
	if err != nil {
		t.Fatalf("upgraded envelope decryption failed: %v", err)
	}
	if !bytes.Equal(upgradedDecrypted, originalSecret) {
		t.Fatalf("upgraded decrypted mismatch: got %s, want %s", string(upgradedDecrypted), string(originalSecret))
	}

	// 5. Tamper Resistance: Tampering with ciphertext or nonce must fail
	tampered := *env3
	tampered.Ciphertext = append([]byte(nil), env3.Ciphertext...)
	tampered.Ciphertext[0] ^= 0xFF // Flip bits

	_, err = engine.Decrypt(ctx, &tampered)
	if err == nil {
		t.Fatal("tamper resistance failure: expected decryption error on tampered ciphertext")
	}
}

func TestStaticKMSAndEnvelopeString(t *testing.T) {
	ctx := context.Background()
	rawKey := make([]byte, 32)
	for i := range rawKey {
		rawKey[i] = byte(i + 1)
	}

	staticKMS, err := kms.NewStaticKeyKMS(rawKey, "test-static-key")
	if err != nil {
		t.Fatalf("failed initializing StaticKeyKMS: %v", err)
	}

	engine := kms.NewKMSEnvelopeEngine(staticKMS)
	plainToken := "ghp_PersonalAccessToken_SecretValue12345"

	// 1. Encrypt string to envelope JSON
	envJSON, err := engine.EncryptString(ctx, "test-workspace-key", plainToken)
	if err != nil {
		t.Fatalf("failed EncryptString: %v", err)
	}

	if !bytes.HasPrefix([]byte(envJSON), []byte("{")) {
		t.Fatalf("expected JSON envelope format, got: %s", envJSON)
	}

	// 2. Decrypt string from envelope JSON
	decrypted, err := engine.DecryptString(ctx, envJSON)
	if err != nil {
		t.Fatalf("failed DecryptString: %v", err)
	}

	if decrypted != plainToken {
		t.Fatalf("decrypted string mismatch: got %q, want %q", decrypted, plainToken)
	}
}

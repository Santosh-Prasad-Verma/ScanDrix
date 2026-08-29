package kms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"time"
)

// KMSEnvelopeEngine performs 2-tier envelope encryption combining KEK with ephemeral DEKs.
type KMSEnvelopeEngine struct {
	provider MasterKeyProvider
}

// NewKMSEnvelopeEngine initializes the envelope engine.
func NewKMSEnvelopeEngine(provider MasterKeyProvider) *KMSEnvelopeEngine {
	return &KMSEnvelopeEngine{
		provider: provider,
	}
}

// Encrypt generates an ephemeral DEK, encrypts payload, and seals the DEK under KMS master key.
func (e *KMSEnvelopeEngine) Encrypt(ctx context.Context, keyID string, plaintext []byte) (*EncryptedEnvelope, error) {
	// 1. Generate 256-bit ephemeral Data Encryption Key (DEK)
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("failed generating ephemeral DEK: %w", err)
	}

	// 2. Encrypt plaintext payload with ephemeral DEK using AES-256-GCM
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("failed creating cipher block: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed initializing GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// 3. Encrypt the DEK using the KMS Master Key (KEK)
	encryptedDEK, version, err := e.provider.Encrypt(ctx, keyID, dek)
	if err != nil {
		return nil, fmt.Errorf("failed encrypting DEK under master key: %w", err)
	}

	return &EncryptedEnvelope{
		KeyID:        keyID,
		KeyVersion:   version,
		EncryptedDEK: encryptedDEK,
		Ciphertext:   ciphertext,
		Nonce:        nonce,
		Algorithm:    "AES-256-GCM",
		CreatedAt:    time.Now().UTC(),
	}, nil
}

// Decrypt opens the envelope by decrypting the DEK with KMS and decrypting the payload.
func (e *KMSEnvelopeEngine) Decrypt(ctx context.Context, env *EncryptedEnvelope) ([]byte, error) {
	if env == nil {
		return nil, fmt.Errorf("envelope is nil")
	}

	// 1. Recover ephemeral DEK by decrypting under specified master key version
	dek, err := e.provider.Decrypt(ctx, env.KeyID, env.KeyVersion, env.EncryptedDEK)
	if err != nil {
		return nil, fmt.Errorf("failed decrypting DEK with KMS: %w", err)
	}

	// 2. Decrypt payload using recovered DEK
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("failed creating cipher block from DEK: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed initializing GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, env.Nonce, env.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("payload decryption/authentication failed: %w", err)
	}

	return plaintext, nil
}

// ReEncrypt updates an older envelope's DEK to the newest master key version without re-encrypting ciphertext.
func (e *KMSEnvelopeEngine) ReEncrypt(ctx context.Context, env *EncryptedEnvelope) (*EncryptedEnvelope, bool, error) {
	activeVersion, err := e.provider.GetActiveVersion(ctx, env.KeyID)
	if err != nil {
		return nil, false, err
	}

	if env.KeyVersion == activeVersion {
		return env, false, nil // Already at current active version
	}

	// Decrypt DEK with old version
	dek, err := e.provider.Decrypt(ctx, env.KeyID, env.KeyVersion, env.EncryptedDEK)
	if err != nil {
		return nil, false, fmt.Errorf("failed recovering DEK during re-encryption: %w", err)
	}

	// Re-encrypt DEK with active version
	newEncryptedDEK, newVersion, err := e.provider.Encrypt(ctx, env.KeyID, dek)
	if err != nil {
		return nil, false, fmt.Errorf("failed sealing DEK under new master key version: %w", err)
	}

	upgraded := &EncryptedEnvelope{
		KeyID:        env.KeyID,
		KeyVersion:   newVersion,
		EncryptedDEK: newEncryptedDEK,
		Ciphertext:   env.Ciphertext,
		Nonce:        env.Nonce,
		Algorithm:    env.Algorithm,
		CreatedAt:    env.CreatedAt,
	}

	return upgraded, true, nil
}

package kms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"time"
)

// MasterKeyProvider defines the contract for master key management (KEK operations).
type MasterKeyProvider interface {
	Encrypt(ctx context.Context, keyID string, plaintext []byte) ([]byte, int, error)
	Decrypt(ctx context.Context, keyID string, version int, ciphertext []byte) ([]byte, error)
	Rotate(ctx context.Context, keyID string) (int, error)
	GetActiveVersion(ctx context.Context, keyID string) (int, error)
}

// LocalMemoryKMS is a software KMS with versioned key rotation and CSPRNG generation.
type LocalMemoryKMS struct {
	mu   sync.RWMutex
	keys map[string]map[int]*MasterKey // keyID -> version -> MasterKey
}

// NewLocalMemoryKMS initializes the in-memory KMS with a root key.
func NewLocalMemoryKMS(defaultKeyID string) (*LocalMemoryKMS, error) {
	kms := &LocalMemoryKMS{
		keys: make(map[string]map[int]*MasterKey),
	}

	// Initialize version 1 of default key
	_, err := kms.Rotate(context.Background(), defaultKeyID)
	if err != nil {
		return nil, fmt.Errorf("failed provisioning initial master key: %w", err)
	}

	return kms, nil
}

// Rotate generates a new active version of the specified master key while preserving older versions.
func (k *LocalMemoryKMS) Rotate(ctx context.Context, keyID string) (int, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	keyMap, exists := k.keys[keyID]
	if !exists {
		keyMap = make(map[int]*MasterKey)
		k.keys[keyID] = keyMap
	}

	// Deactivate existing active versions
	nextVersion := 1
	for _, mk := range keyMap {
		if mk.Active {
			mk.Active = false
			mk.RotatedAt = time.Now().UTC()
		}
		if mk.Version >= nextVersion {
			nextVersion = mk.Version + 1
		}
	}

	rawKey := make([]byte, 32) // 256-bit AES
	if _, err := io.ReadFull(rand.Reader, rawKey); err != nil {
		return 0, fmt.Errorf("failed generating entropy for master key: %w", err)
	}

	newKey := &MasterKey{
		ID:        keyID,
		Version:   nextVersion,
		RawSecret: rawKey,
		Active:    true,
		CreatedAt: time.Now().UTC(),
	}

	keyMap[nextVersion] = newKey
	return nextVersion, nil
}

// Encrypt encrypts plaintext using the currently active master key version.
func (k *LocalMemoryKMS) Encrypt(ctx context.Context, keyID string, plaintext []byte) ([]byte, int, error) {
	k.mu.RLock()
	keyMap, exists := k.keys[keyID]
	if !exists {
		k.mu.RUnlock()
		return nil, 0, fmt.Errorf("master key '%s' not found", keyID)
	}

	var activeKey *MasterKey
	for _, mk := range keyMap {
		if mk.Active {
			activeKey = mk
			break
		}
	}
	k.mu.RUnlock()

	if activeKey == nil {
		return nil, 0, fmt.Errorf("no active version for master key '%s'", keyID)
	}

	block, err := aes.NewCipher(activeKey.RawSecret)
	if err != nil {
		return nil, 0, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, 0, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, 0, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, activeKey.Version, nil
}

// Decrypt decrypts ciphertext using the specified master key version.
func (k *LocalMemoryKMS) Decrypt(ctx context.Context, keyID string, version int, ciphertext []byte) ([]byte, error) {
	k.mu.RLock()
	keyMap, exists := k.keys[keyID]
	if !exists {
		k.mu.RUnlock()
		return nil, fmt.Errorf("master key '%s' not found", keyID)
	}

	mk, ok := keyMap[version]
	k.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("master key '%s' version %d not found", keyID, version)
	}

	block, err := aes.NewCipher(mk.RawSecret)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("kms master key decryption failed: %w", err)
	}

	return plaintext, nil
}

// GetActiveVersion returns the current version number for the given key ID.
func (k *LocalMemoryKMS) GetActiveVersion(ctx context.Context, keyID string) (int, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	keyMap, exists := k.keys[keyID]
	if !exists {
		return 0, fmt.Errorf("master key '%s' not found", keyID)
	}

	for _, mk := range keyMap {
		if mk.Active {
			return mk.Version, nil
		}
	}
	return 0, fmt.Errorf("no active version for master key '%s'", keyID)
}

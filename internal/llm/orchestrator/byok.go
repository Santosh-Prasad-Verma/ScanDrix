package orchestrator

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

// BYOKManager secures and manages customer-supplied LLM API keys.
type BYOKManager struct {
	masterKey []byte
}

// NewBYOKManager initializes the encryption engine with a 256-bit AES master key.
func NewBYOKManager(masterKeyHex string) (*BYOKManager, error) {
	if len(masterKeyHex) != 64 {
		// Provide deterministic 32-byte key fallback for test/dev if empty
		hash := sha256.Sum256([]byte(masterKeyHex))
		return &BYOKManager{masterKey: hash[:]}, nil
	}

	key, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid hex master key: %w", err)
	}

	return &BYOKManager{masterKey: key}, nil
}

// EncryptKey encrypts an API key with AES-256-GCM and generates audit fingerprints.
func (m *BYOKManager) EncryptKey(wsID uuid.UUID, provider LLMProviderType, rawKey string) (*BYOKCredential, error) {
	if rawKey == "" {
		return nil, errors.New("raw API key cannot be empty")
	}

	block, err := aes.NewCipher(m.masterKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(rawKey), nil)

	// Compute deterministic fingerprint
	fpHash := sha256.Sum256([]byte(rawKey))
	fingerprint := hex.EncodeToString(fpHash[:16])

	// Mask key for safe UI display
	masked := maskKey(rawKey)

	return &BYOKCredential{
		WorkspaceID:    wsID,
		Provider:       provider,
		EncryptedKey:   hex.EncodeToString(ciphertext),
		Nonce:          hex.EncodeToString(nonce),
		KeyFingerprint: fingerprint,
		MaskedKey:      masked,
		UpdatedAt:      time.Now().UTC(),
	}, nil
}

// DecryptKey decrypts an encrypted credential back to plaintext at execution time.
func (m *BYOKManager) DecryptKey(cred *BYOKCredential) (string, error) {
	ciphertext, err := hex.DecodeString(cred.EncryptedKey)
	if err != nil {
		return "", err
	}

	nonce, err := hex.DecodeString(cred.Nonce)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(m.masterKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt API key: %w", err)
	}

	return string(plaintext), nil
}

func maskKey(raw string) string {
	if len(raw) <= 8 {
		return "********"
	}
	prefix := raw[:min(4, len(raw))]
	suffix := raw[max(0, len(raw)-4):]
	return fmt.Sprintf("%s...%s", prefix, suffix)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

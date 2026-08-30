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
// masterKeyHex must be a cryptographically random 64-character hex string (32 bytes).
// Passing an empty string activates a deterministic dev/test fallback — never use
// that path in production. Any non-empty string that isn't exactly 64 hex chars
// is rejected with an error rather than silently degrading security.
func NewBYOKManager(masterKeyHex string) (*BYOKManager, error) {
	if masterKeyHex == "" {
		// Dev/test only: derive a fixed 32-byte key so the engine starts
		// without configuration. Set BYOK_MASTER_KEY to a real random
		// 64-char hex value before deploying to any real environment.
		hash := sha256.Sum256([]byte("dev-fallback-do-not-use-in-production"))
		return &BYOKManager{masterKey: hash[:]}, nil
	}

	if len(masterKeyHex) != 64 {
		return nil, fmt.Errorf(
			"master key must be a 64-character hex string (32 bytes); got %d characters",
			len(masterKeyHex),
		)
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
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(rawKey), nil)

	// Deterministic fingerprint for audit logs — truncated to 16 bytes so
	// the full SHA-256 digest is never exposed, preventing pre-image attacks.
	fpHash := sha256.Sum256([]byte(rawKey))
	fingerprint := hex.EncodeToString(fpHash[:16])

	return &BYOKCredential{
		WorkspaceID:    wsID,
		Provider:       provider,
		EncryptedKey:   hex.EncodeToString(ciphertext),
		Nonce:          hex.EncodeToString(nonce),
		KeyFingerprint: fingerprint,
		MaskedKey:      maskKey(rawKey),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}, nil
}

// DecryptKey decrypts an encrypted credential back to plaintext at execution time.
func (m *BYOKManager) DecryptKey(cred *BYOKCredential) (string, error) {
	if cred == nil {
		return "", errors.New("credential cannot be nil")
	}

	ciphertext, err := hex.DecodeString(cred.EncryptedKey)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	nonce, err := hex.DecodeString(cred.Nonce)
	if err != nil {
		return "", fmt.Errorf("failed to decode nonce: %w", err)
	}

	block, err := aes.NewCipher(m.masterKey)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt API key (wrong master key or corrupted ciphertext): %w", err)
	}

	return string(plaintext), nil
}

// ValidateFingerprint checks whether a raw API key matches the fingerprint stored
// in a credential — useful for audit, key-rotation diffs, and integrity checks
// without decrypting the full ciphertext.
func (m *BYOKManager) ValidateFingerprint(cred *BYOKCredential, rawKey string) bool {
	if cred == nil || rawKey == "" {
		return false
	}
	fpHash := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(fpHash[:16]) == cred.KeyFingerprint
}

// RotateKey re-encrypts a workspace credential with a new raw API key, preserving
// the original CreatedAt timestamp and producing a fresh ciphertext + fingerprint.
func (m *BYOKManager) RotateKey(wsID uuid.UUID, provider LLMProviderType, oldCred *BYOKCredential, newRawKey string) (*BYOKCredential, error) {
	newCred, err := m.EncryptKey(wsID, provider, newRawKey)
	if err != nil {
		return nil, fmt.Errorf("key rotation failed: %w", err)
	}

	// Preserve the original creation timestamp so audit trails stay intact.
	if oldCred != nil {
		newCred.CreatedAt = oldCred.CreatedAt
	}

	return newCred, nil
}

// maskKey returns a safe display version of an API key — first 4 + last 4 chars.
// Keys of 8 characters or fewer are fully masked to prevent trivial brute-force
// from a masked display value.
func maskKey(raw string) string {
	if len(raw) <= 8 {
		return "********"
	}
	// len(raw) > 8 is guaranteed above, so both slice bounds are always safe
	// without needing min/max guards.
	return fmt.Sprintf("%s...%s", raw[:4], raw[len(raw)-4:])
}

package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// EncryptAESGCM encrypts plaintext using AES-256-GCM with a random 12-byte nonce.
func EncryptAESGCM(key []byte, plaintext []byte) (ciphertext []byte, nonce []byte, err error) {
	if len(key) != 32 {
		return nil, nil, errors.New("invalid key length: AES-256 requires a 32-byte key")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed creating GCM block: %w", err)
	}

	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed generating random nonce: %w", err)
	}

	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// DecryptAESGCM decrypts ciphertext using AES-256-GCM and the provided nonce.
func DecryptAESGCM(key []byte, ciphertext []byte, nonce []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("invalid key length: AES-256 requires a 32-byte key")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed creating GCM block: %w", err)
	}

	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid nonce length: expected %d bytes, got %d", gcm.NonceSize(), len(nonce))
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (authentication tag mismatch): %w", err)
	}

	return plaintext, nil
}

// ComputeHMACSHA256 generates a hex-encoded HMAC-SHA256 signature for a payload.
func ComputeHMACSHA256(secret, payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyHMACSHA256 performs a constant-time HMAC-SHA256 signature verification.
func VerifyHMACSHA256(secret, payload []byte, expectedHexSignature string) bool {
	expectedSig, err := hex.DecodeString(expectedHexSignature)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	actualSig := mac.Sum(nil)

	return subtle.ConstantTimeCompare(actualSig, expectedSig) == 1
}

// GenerateSecureRandomBytes returns cryptographically secure random bytes.
func GenerateSecureRandomBytes(n int) ([]byte, error) {
	if n <= 0 {
		return nil, errors.New("byte length must be positive")
	}

	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, fmt.Errorf("failed reading random bytes: %w", err)
	}
	return b, nil
}

// GenerateSecureToken returns a URL-safe base64 random token with the specified prefix.
func GenerateSecureToken(prefix string, byteLength int) (string, error) {
	b, err := GenerateSecureRandomBytes(byteLength)
	if err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	if prefix != "" {
		return prefix + token, nil
	}
	return token, nil
}

// FingerprintSHA256 computes a deterministic hex SHA-256 fingerprint of input data.
func FingerprintSHA256(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// Package crypto provides symmetric encryption, hashing, and token cryptographic helpers for ScanDrix.
package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	// ErrInvalidKeyLength indicates that the key is not 32 bytes (256 bits).
	ErrInvalidKeyLength = errors.New("crypto: key must be 32 bytes in hexadecimal")
	// ErrInvalidCiphertext indicates that the ciphertext format is invalid.
	ErrInvalidCiphertext = errors.New("crypto: invalid encrypted text format")
	// ErrInvalidPadding indicates corrupted or invalid PKCS#7 padding.
	ErrInvalidPadding = errors.New("crypto: invalid padding")
)

// PKCS7Pad appends PKCS#7 padding to data for block size.
func PKCS7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padtext...)
}

// PKCS7Unpad removes PKCS#7 padding from data.
func PKCS7Unpad(data []byte, blockSize int) ([]byte, error) {
	length := len(data)
	if length == 0 || length%blockSize != 0 {
		return nil, ErrInvalidPadding
	}
	padding := int(data[length-1])
	if padding == 0 || padding > blockSize || padding > length {
		return nil, ErrInvalidPadding
	}
	for i := length - padding; i < length; i++ {
		if data[i] != byte(padding) {
			return nil, ErrInvalidPadding
		}
	}
	return data[:length-padding], nil
}

// Encrypt encrypts plain text using AES-256-CBC matching ScanDrix standard format: ivHex:ciphertextHex.
func Encrypt(text string, keyHex string) (string, error) {
	if text == "" {
		return "", nil
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return "", ErrInvalidKeyLength
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", fmt.Errorf("failed to generate IV: %w", err)
	}

	paddedText := PKCS7Pad([]byte(text), aes.BlockSize)
	ciphertext := make([]byte, len(paddedText))

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, paddedText)

	return fmt.Sprintf("%s:%s", hex.EncodeToString(iv), hex.EncodeToString(ciphertext)), nil
}

// Decrypt decrypts text formatted as ivHex:ciphertextHex using AES-256-CBC.
func Decrypt(encryptedText string, keyHex string) (string, error) {
	if encryptedText == "" {
		return "", nil
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return "", ErrInvalidKeyLength
	}

	parts := strings.Split(encryptedText, ":")
	if len(parts) != 2 {
		return "", ErrInvalidCiphertext
	}

	iv, err := hex.DecodeString(parts[0])
	if err != nil || len(iv) != aes.BlockSize {
		return "", ErrInvalidCiphertext
	}

	ciphertext, err := hex.DecodeString(parts[1])
	if err != nil || len(ciphertext)%aes.BlockSize != 0 {
		return "", ErrInvalidCiphertext
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	unpadded, err := PKCS7Unpad(plaintext, aes.BlockSize)
	if err != nil {
		return "", err
	}

	return string(unpadded), nil
}

// EncryptGCM encrypts plaintext using modern AES-256-GCM with a 12-byte nonce.
func EncryptGCM(plaintext []byte, key []byte) (string, error) {
	if len(key) != 32 {
		return "", ErrInvalidKeyLength
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aesGCM.Seal(nil, nonce, plaintext, nil)
	return fmt.Sprintf("%s:%s", hex.EncodeToString(nonce), hex.EncodeToString(ciphertext)), nil
}

// DecryptGCM decrypts a nonce:ciphertext string using AES-256-GCM.
func DecryptGCM(encryptedText string, key []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKeyLength
	}
	parts := strings.Split(encryptedText, ":")
	if len(parts) != 2 {
		return nil, ErrInvalidCiphertext
	}

	nonce, err := hex.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	ciphertext, err := hex.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidCiphertext
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return aesGCM.Open(nil, nonce, ciphertext, nil)
}

// HashSHA256 returns the hexadecimal representation of SHA-256 hash of data.
func HashSHA256(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// HashHMACSHA256 returns the hexadecimal HMAC-SHA256 signature for data with secret.
func HashHMACSHA256(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// SecureRandomBytes generates n cryptographically secure random bytes.
func SecureRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, err
	}
	return b, nil
}

// SecureRandomHex generates a hex string of n random bytes (length 2*n).
func SecureRandomHex(n int) (string, error) {
	b, err := SecureRandomBytes(n)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

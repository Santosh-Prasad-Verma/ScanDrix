package webhooks

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// GenerateWebhookToken encrypts plainToken using AES-256-CBC with secretKeyHex (32 bytes).
// It returns "<iv-hex>:<encrypted-hex>".
func GenerateWebhookToken(secretKeyHex, plainToken string) (string, error) {
	key, err := hex.DecodeString(secretKeyHex)
	if err != nil || len(key) != 32 {
		return "", errors.New("secret key must be 32 bytes in hexadecimal")
	}
	if plainToken == "" {
		return "", errors.New("plain token is required")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed creating cipher: %w", err)
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", fmt.Errorf("failed generating IV: %w", err)
	}

	padded := pkcs7Pad([]byte(plainToken), aes.BlockSize)
	ciphertext := make([]byte, len(padded))

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	return fmt.Sprintf("%s:%s", hex.EncodeToString(iv), hex.EncodeToString(ciphertext)), nil
}

// ValidateWebhookToken decrypts the encryptedToken and verifies it matches plainToken
// using constant-time comparison.
func ValidateWebhookToken(secretKeyHex, plainToken, encryptedToken string) bool {
	if plainToken == "" || encryptedToken == "" {
		return false
	}

	key, err := hex.DecodeString(secretKeyHex)
	if err != nil || len(key) != 32 {
		return false
	}

	parts := strings.Split(encryptedToken, ":")
	if len(parts) != 2 {
		return false
	}

	iv, err := hex.DecodeString(parts[0])
	if err != nil || len(iv) != aes.BlockSize {
		return false
	}

	ciphertext, err := hex.DecodeString(parts[1])
	if err != nil || len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return false
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return false
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	unpadded, err := pkcs7Unpad(plaintext)
	if err != nil {
		return false
	}

	return subtle.ConstantTimeCompare(unpadded, []byte(plainToken)) == 1
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padtext...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, errors.New("empty data")
	}
	padding := int(data[length-1])
	if padding > length || padding == 0 {
		return nil, errors.New("invalid padding")
	}
	for i := length - padding; i < length; i++ {
		if data[i] != byte(padding) {
			return nil, errors.New("invalid padding bytes")
		}
	}
	return data[:length-padding], nil
}

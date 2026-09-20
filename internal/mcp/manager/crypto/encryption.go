// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Encryptor provides cryptographic encryption and decryption of secrets at rest.
type Encryptor struct {
	key []byte
}

// NewEncryptor derives a 32-byte AES-256 key from a passphrase secret using SHA-256.
func NewEncryptor(secret string) (*Encryptor, error) {
	if secret == "" {
		return nil, errors.New("encryption secret cannot be empty")
	}
	hash := sha256.Sum256([]byte(secret))
	return &Encryptor{key: hash[:]}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM with a random 12-byte nonce.
// Returns formatted string: "<nonce_hex>:<base64_ciphertext_and_tag>".
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("failed creating aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed creating gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed generating nonce: %w", err)
	}

	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return fmt.Sprintf("%s:%s", hex.EncodeToString(nonce), base64.StdEncoding.EncodeToString(sealed)), nil
}

// Decrypt unpacks and decrypts ciphertext formatted as "<nonce_or_iv_hex>:<base64_ciphertext>".
// Automatically detects and supports both modern AES-256-GCM and legacy AES-256-CBC.
func (e *Encryptor) Decrypt(encrypted string) (string, error) {
	if encrypted == "" {
		return "", errors.New("empty encrypted payload")
	}

	parts := strings.Split(encrypted, ":")
	if len(parts) != 2 {
		return "", errors.New("invalid encrypted data format; expected 'nonce_or_iv_hex:ciphertext_base64'")
	}

	ivOrNonce, err := hex.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("failed decoding hex IV/nonce: %w", err)
	}

	cipherBytes, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("failed decoding base64 ciphertext: %w", err)
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("failed creating aes cipher: %w", err)
	}

	// Case 1: 12-byte GCM Nonce
	if len(ivOrNonce) == 12 {
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return "", fmt.Errorf("failed creating gcm: %w", err)
		}

		plain, err := gcm.Open(nil, ivOrNonce, cipherBytes, nil)
		if err != nil {
			return "", fmt.Errorf("failed decrypting gcm payload: %w", err)
		}
		return string(plain), nil
	}

	// Case 2: 16-byte CBC IV (Legacy Node.js crypto compatibility)
	if len(ivOrNonce) == aes.BlockSize {
		if len(cipherBytes)%aes.BlockSize != 0 {
			return "", errors.New("ciphertext is not a multiple of the block size")
		}

		mode := cipher.NewCBCDecrypter(block, ivOrNonce)
		plain := make([]byte, len(cipherBytes))
		mode.CryptBlocks(plain, cipherBytes)

		// PKCS#7 unpadding
		unpadded, err := pkcs7Unpad(plain, aes.BlockSize)
		if err != nil {
			return "", fmt.Errorf("pkcs7 unpadding error: %w", err)
		}
		return string(unpadded), nil
	}

	return "", fmt.Errorf("unsupported IV/nonce length: %d bytes", len(ivOrNonce))
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	length := len(data)
	if length == 0 || length%blockSize != 0 {
		return nil, errors.New("invalid padding size")
	}
	padLen := int(data[length-1])
	if padLen == 0 || padLen > blockSize {
		return nil, errors.New("invalid padding value")
	}
	for i := length - padLen; i < length; i++ {
		if data[i] != byte(padLen) {
			return nil, errors.New("invalid padding byte")
		}
	}
	return data[:length-padLen], nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

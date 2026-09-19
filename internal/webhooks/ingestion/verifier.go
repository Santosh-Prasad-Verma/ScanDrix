package ingestion

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"strings"
)

// WebhookVerifier cryptographically checks authenticity of inbound webhook HTTP requests.
type WebhookVerifier struct{}

// NewWebhookVerifier initializes the verifier.
func NewWebhookVerifier() *WebhookVerifier {
	return &WebhookVerifier{}
}

// VerifyGitHub validates the X-Hub-Signature-256 HMAC-SHA256 header.
func (v *WebhookVerifier) VerifyGitHub(signatureHeader string, body []byte, secret string) bool {
	if secret == "" {
		return false
	}
	if !strings.HasPrefix(signatureHeader, "sha256=") {
		return false
	}

	sigHex := strings.TrimPrefix(signatureHeader, "sha256=")
	expectedSig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	calculatedSig := mac.Sum(nil)

	return hmac.Equal(expectedSig, calculatedSig)
}

// VerifyGitLab validates the X-Gitlab-Token header using constant time comparison.
func (v *WebhookVerifier) VerifyGitLab(tokenHeader, expectedToken string) bool {
	if expectedToken == "" || tokenHeader == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(tokenHeader), []byte(expectedToken)) == 1
}

// VerifyForgejo validates the X-Gitea-Signature HMAC-SHA256 header.
func (v *WebhookVerifier) VerifyForgejo(signatureHeader string, body []byte, secret string) bool {
	if secret == "" || signatureHeader == "" {
		return false
	}

	sigHex := strings.TrimPrefix(signatureHeader, "sha256=")
	expectedSig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	calculatedSig := mac.Sum(nil)

	return hmac.Equal(expectedSig, calculatedSig)
}

// VerifyBitbucket validates the X-Hub-Signature HMAC-SHA256 header for Bitbucket.
func (v *WebhookVerifier) VerifyBitbucket(signatureHeader string, body []byte, secret string) bool {
	if secret == "" || signatureHeader == "" {
		return false
	}

	sigHex := strings.TrimPrefix(signatureHeader, "sha256=")
	expectedSig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	calculatedSig := mac.Sum(nil)

	return hmac.Equal(expectedSig, calculatedSig)
}

// VerifyAzureDevOps validates the Authorization header using constant time comparison.
func (v *WebhookVerifier) VerifyAzureDevOps(authHeader, expectedToken string) bool {
	if expectedToken == "" || authHeader == "" {
		return false
	}
	cleanHeader := strings.TrimPrefix(authHeader, "Bearer ")
	cleanHeader = strings.TrimPrefix(cleanHeader, "Basic ")
	return subtle.ConstantTimeCompare([]byte(cleanHeader), []byte(expectedToken)) == 1
}

// VerifyAzureWebhookToken validates an encrypted token in format `<ivHex>:<encryptedHex>`
// using AES-256-CBC encryption format.
func (v *WebhookVerifier) VerifyAzureWebhookToken(encryptedToken, hexSecret, expectedPlainToken string) bool {
	if encryptedToken == "" || hexSecret == "" || expectedPlainToken == "" {
		return false
	}
	key, err := hex.DecodeString(hexSecret)
	if err != nil || len(key) != 32 {
		return false
	}
	parts := strings.Split(encryptedToken, ":")
	if len(parts) != 2 {
		return false
	}
	iv, err := hex.DecodeString(parts[0])
	if err != nil || len(iv) != 16 {
		return false
	}
	cipherBytes, err := hex.DecodeString(parts[1])
	if err != nil || len(cipherBytes) == 0 || len(cipherBytes)%16 != 0 {
		return false
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return false
	}
	decrypted := make([]byte, len(cipherBytes))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(decrypted, cipherBytes)

	unpadded, err := pkcs7Unpad(decrypted, 16)
	if err != nil {
		return false
	}

	return subtle.ConstantTimeCompare(unpadded, []byte(expectedPlainToken)) == 1
}

// VerifyBilling validates the HMAC-SHA256 signature against the raw webhook body.
func (v *WebhookVerifier) VerifyBilling(signatureHeader string, rawBody []byte, secret string) bool {
	if secret == "" || signatureHeader == "" || len(rawBody) == 0 {
		return false
	}
	cleanSig := strings.TrimSpace(signatureHeader)
	expectedSig, err := hex.DecodeString(cleanSig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	calculatedSig := mac.Sum(nil)

	return hmac.Equal(expectedSig, calculatedSig)
}

// GenerateAzureWebhookToken generates an AES-256-CBC encrypted token for Azure DevOps webhooks.
func GenerateAzureWebhookToken(plainToken, hexSecret string) (string, error) {
	if plainToken == "" {
		return "", errors.New("plainToken cannot be empty")
	}
	key, err := hex.DecodeString(hexSecret)
	if err != nil || len(key) != 32 {
		return "", errors.New("hexSecret must be 32 bytes in hexadecimal")
	}
	iv := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	padded := pkcs7Pad([]byte(plainToken), 16)
	cipherBytes := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(cipherBytes, padded)

	return hex.EncodeToString(iv) + ":" + hex.EncodeToString(cipherBytes), nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - (len(data) % blockSize)
	padding := make([]byte, padLen)
	for i := range padding {
		padding[i] = byte(padLen)
	}
	return append(data, padding...)
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errors.New("invalid padding: data length is not a multiple of block size")
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > blockSize || padLen > len(data) {
		return nil, errors.New("invalid padding: pad length out of bounds")
	}
	for i := len(data) - padLen; i < len(data); i++ {
		if data[i] != byte(padLen) {
			return nil, errors.New("invalid padding: corrupt padding bytes")
		}
	}
	return data[:len(data)-padLen], nil
}

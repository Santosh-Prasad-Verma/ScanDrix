package ingestion

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
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

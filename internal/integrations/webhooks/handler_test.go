package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyGitHubSignature(t *testing.T) {
	secret := "super-secure-webhook-secret"
	body := []byte(`{"action":"opened","number":42}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validHeader := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifyGitHubSignature(secret, body, validHeader) {
		t.Error("expected signature to be valid")
	}

	invalidHeader := "sha256=1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	if verifyGitHubSignature(secret, body, invalidHeader) {
		t.Error("expected invalid signature to be rejected")
	}

	malformedHeader := "md5=invalid"
	if verifyGitHubSignature(secret, body, malformedHeader) {
		t.Error("expected malformed signature to be rejected")
	}
}

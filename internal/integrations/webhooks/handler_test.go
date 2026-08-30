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
}

func TestVerifyGitLabToken(t *testing.T) {
	secret := "gitlab-secret-token-123"

	if !verifyGitLabToken(secret, "gitlab-secret-token-123") {
		t.Error("expected valid token to match")
	}

	if verifyGitLabToken(secret, "wrong-token") {
		t.Error("expected wrong token to be rejected")
	}

	if verifyGitLabToken(secret, "") {
		t.Error("expected empty token to be rejected")
	}

	if verifyGitLabToken("", "some-token") {
		t.Error("expected empty secret to be rejected")
	}
}

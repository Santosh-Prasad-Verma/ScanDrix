package ingestion_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/scandrix/backend/internal/webhooks/ingestion"
)

func TestAzureDevOpsWebhookTokenCrypto(t *testing.T) {
	verifier := ingestion.NewWebhookVerifier()

	// 32-byte key in hex (64 chars)
	secretHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	expectedPlain := "scandrix-azure-webhook-token-super-secret"

	// Generate token
	encryptedToken, err := ingestion.GenerateAzureWebhookToken(expectedPlain, secretHex)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// Verify valid token
	if !verifier.VerifyAzureWebhookToken(encryptedToken, secretHex, expectedPlain) {
		t.Fatalf("expected valid token to verify successfully")
	}

	// Verify wrong plain token fails
	if verifier.VerifyAzureWebhookToken(encryptedToken, secretHex, "wrong-plain-token") {
		t.Fatalf("expected wrong plain token to fail verification")
	}

	// Verify wrong secret fails
	wrongSecretHex := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	if verifier.VerifyAzureWebhookToken(encryptedToken, wrongSecretHex, expectedPlain) {
		t.Fatalf("expected wrong secret to fail verification")
	}

	// Verify malformed token fails
	if verifier.VerifyAzureWebhookToken("malformed-token", secretHex, expectedPlain) {
		t.Fatalf("expected malformed token to fail verification")
	}

	if verifier.VerifyAzureWebhookToken("", secretHex, expectedPlain) {
		t.Fatalf("expected empty token to fail verification")
	}
}

func TestBillingSignatureVerification(t *testing.T) {
	verifier := ingestion.NewWebhookVerifier()
	secret := "billing-webhook-secret-999"
	body := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_123"}}}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := hex.EncodeToString(mac.Sum(nil))

	if !verifier.VerifyBilling(validSig, body, secret) {
		t.Fatalf("expected valid billing signature to verify")
	}

	if verifier.VerifyBilling("invalidsig123", body, secret) {
		t.Fatalf("expected invalid billing signature to fail")
	}

	if verifier.VerifyBilling(validSig, []byte(`tampered-body`), secret) {
		t.Fatalf("expected tampered body to fail verification")
	}
}

package crypto

import (
	"testing"
)

func TestEncryptDecryptCBC(t *testing.T) {
	keyHex, err := SecureRandomHex(32)
	if err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}

	testCases := []string{
		"hello world",
		"ScanDrix is an enterprise code review platform",
		"",
		"a very long text with emojis 🔥 🚀 and special characters !@#$%^&*()_+-=[]{}|;':,.<>/?",
	}

	for _, tc := range testCases {
		enc, err := Encrypt(tc, keyHex)
		if err != nil {
			t.Fatalf("failed to encrypt %q: %v", tc, err)
		}
		if tc == "" {
			if enc != "" {
				t.Fatalf("expected empty string for empty input, got %q", enc)
			}
			continue
		}

		dec, err := Decrypt(enc, keyHex)
		if err != nil {
			t.Fatalf("failed to decrypt %q: %v", tc, err)
		}
		if dec != tc {
			t.Fatalf("expected %q, got %q", tc, dec)
		}
	}
}

func TestEncryptDecryptGCM(t *testing.T) {
	key, err := SecureRandomBytes(32)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	plaintext := []byte("confidential token data for enterprise customer")
	encrypted, err := EncryptGCM(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptGCM failed: %v", err)
	}

	decrypted, err := DecryptGCM(encrypted, key)
	if err != nil {
		t.Fatalf("DecryptGCM failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Fatalf("expected %s, got %s", string(plaintext), string(decrypted))
	}
}

func TestWebhookToken(t *testing.T) {
	secretHex, err := SecureRandomHex(32)
	if err != nil {
		t.Fatalf("failed to generate secret: %v", err)
	}

	token := "webhook_secret_verification_token_12345"
	encryptedToken, err := GenerateWebhookToken(secretHex, token)
	if err != nil {
		t.Fatalf("GenerateWebhookToken failed: %v", err)
	}

	if !ValidateWebhookToken(encryptedToken, secretHex, token) {
		t.Fatalf("expected valid webhook token")
	}

	if ValidateWebhookToken(encryptedToken, secretHex, "wrong_token") {
		t.Fatalf("expected invalid webhook token for mismatch")
	}

	if ValidateWebhookToken("invalid:garbage", secretHex, token) {
		t.Fatalf("expected invalid for corrupt token")
	}
}

func TestHashSHA256AndHMAC(t *testing.T) {
	hash := HashSHA256("test data")
	if len(hash) != 64 {
		t.Fatalf("expected 64 character hex string, got %d", len(hash))
	}

	hmacSig := HashHMACSHA256("payload string", "secret_key")
	if len(hmacSig) != 64 {
		t.Fatalf("expected 64 character hex string for HMAC, got %d", len(hmacSig))
	}
}

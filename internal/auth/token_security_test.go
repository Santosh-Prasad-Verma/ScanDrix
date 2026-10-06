package auth_test

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
)

func TestAccessTokenRejectsInvalidSecurityClaims(t *testing.T) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal("could not create test signing key")
	}
	a := auth.NewAuthenticator(string(secret))
	now := time.Now().Unix()
	baseline := map[string]any{"sub": uuid.NewString(), "ws": uuid.NewString(), "role": "OWNER", "iat": now, "exp": now + 900}
	sign := func(claims map[string]any) string {
		payload, err := json.Marshal(claims)
		if err != nil {
			t.Fatal("could not encode test claims")
		}
		content := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload)
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte(content))
		return content + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
	if _, err := a.VerifyToken(sign(baseline)); err != nil {
		t.Fatal("control: a valid access token was rejected")
	}
	for _, test := range []struct {
		name, field string
		value       any
	}{
		{"missing expiry", "exp", nil}, {"zero expiry", "exp", 0}, {"expiry boundary", "exp", now},
		{"missing issued at", "iat", nil}, {"future issued at", "iat", now + 60},
		{"future not before", "nbf", now + 60}, {"empty user", "sub", uuid.Nil.String()},
		{"empty workspace", "ws", uuid.Nil.String()}, {"unknown role", "role", "ROOT"},
	} {
		t.Run(test.name, func(t *testing.T) {
			claims := make(map[string]any, len(baseline))
			for key, value := range baseline {
				claims[key] = value
			}
			if test.value == nil {
				delete(claims, test.field)
			} else {
				claims[test.field] = test.value
			}
			if _, err := a.VerifyToken(sign(claims)); err == nil {
				t.Fatal("invalid security claims were accepted")
			}
		})
	}
}

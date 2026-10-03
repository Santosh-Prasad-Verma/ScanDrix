package oauth

import (
	"crypto/sha256"
	"encoding/base64"
)

func base64RawURLEncode(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// S256Challenge returns the RFC 7636 S256 code challenge for a verifier:
//
//	challenge = BASE64URL(SHA256(ASCII(verifier)))
//
// AUDIT_REMEDIATION.md: the user OAuth flow had no PKCE. The verifier stays
// server-side; only this challenge is sent to the provider.
func S256Challenge(verifier string) string {
	if verifier == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(verifier))
	return base64RawURLEncode(sum[:])
}

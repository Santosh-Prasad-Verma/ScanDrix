package sso

import (
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// OIDCHandler handles OpenID Connect ID token decoding, cryptographic signature validation, and claim assertions.
type OIDCHandler struct {
	signingKey any // *rsa.PublicKey or []byte
}

// NewOIDCHandler initializes the OIDC handler.
func NewOIDCHandler() *OIDCHandler {
	return &OIDCHandler{}
}

// SetSigningKey configures an RSA public key or HMAC secret for token signature verification.
func (h *OIDCHandler) SetSigningKey(key any) {
	h.signingKey = key
}

// OIDCHeader represents standard JWT JOSE header.
type OIDCHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid,omitempty"`
}

// OIDCClaims represents standard OIDC JWT claims.
type OIDCClaims struct {
	Issuer        string   `json:"iss"`
	Subject       string   `json:"sub"`
	Audience      any      `json:"aud"` // string or []string
	ExpiresAt     int64    `json:"exp"`
	IssuedAt      int64    `json:"iat"`
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	GivenName     string   `json:"given_name"`
	FamilyName    string   `json:"family_name"`
	Name          string   `json:"name"`
	Groups        []string `json:"groups"`
	HD            string   `json:"hd"` // Hosted domain for Google Workspace
}

// VerifySignature cryptographically validates the JWT signature against a public key or shared secret.
func (h *OIDCHandler) VerifySignature(rawJWT string, key any) error {
	parts := strings.Split(rawJWT, ".")
	if len(parts) != 3 {
		return fmt.Errorf("invalid JWT structure: expected 3 segments, got %d", len(parts))
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("failed decoding header: %w", err)
	}

	var header OIDCHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return fmt.Errorf("malformed header JSON: %w", err)
	}

	// Master Rule 5.1: alg: none never accepted when verification key is configured
	if strings.ToLower(header.Alg) == "none" {
		return errors.New("insecure algorithm 'none' rejected by enterprise security baseline")
	}

	signedContent := parts[0] + "." + parts[1]
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("failed decoding signature: %w", err)
	}

	switch k := key.(type) {
	case *rsa.PublicKey:
		hashed := sha256.Sum256([]byte(signedContent))
		if err := rsa.VerifyPKCS1v15(k, crypto.SHA256, hashed[:], sigBytes); err != nil {
			return fmt.Errorf("RSA signature verification failed: %w", err)
		}
		return nil
	case []byte:
		mac := hmac.New(sha256.New, k)
		mac.Write([]byte(signedContent))
		expectedSig := mac.Sum(nil)
		if !hmac.Equal(sigBytes, expectedSig) {
			return errors.New("HMAC signature verification failed")
		}
		return nil
	default:
		return fmt.Errorf("unsupported key type: %T", key)
	}
}

// ParseAndVerifyIDToken decodes the JWT and validates standard claims (iss, aud, exp, email_verified).
func (h *OIDCHandler) ParseAndVerifyIDToken(rawJWT string, expectedIssuer string, expectedAudience string, now time.Time) (*FederatedIdentity, error) {
	parts := strings.Split(rawJWT, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT structure: expected 3 segments, got %d", len(parts))
	}

	// 0. Verify Signature if key is configured
	if h.signingKey != nil {
		if err := h.VerifySignature(rawJWT, h.signingKey); err != nil {
			return nil, err
		}
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed decoding JWT payload: %w", err)
	}

	var claims OIDCClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("malformed OIDC claims JSON: %w", err)
	}

	// 1. Verify Issuer
	if expectedIssuer != "" && claims.Issuer != expectedIssuer {
		return nil, fmt.Errorf("issuer mismatch: expected '%s', got '%s'", expectedIssuer, claims.Issuer)
	}

	// 2. Verify Audience
	if expectedAudience != "" {
		audMatched := false
		switch v := claims.Audience.(type) {
		case string:
			audMatched = (v == expectedAudience)
		case []any:
			for _, item := range v {
				if str, ok := item.(string); ok && str == expectedAudience {
					audMatched = true
					break
				}
			}
		}
		if !audMatched {
			return nil, fmt.Errorf("audience mismatch: token does not match expected client ID '%s'", expectedAudience)
		}
	}

	// 3. Verify Expiry (with 1-minute clock skew tolerance)
	expTime := time.Unix(claims.ExpiresAt, 0)
	if now.Add(-1 * time.Minute).After(expTime) {
		return nil, fmt.Errorf("OIDC ID token expired on %s", expTime.Format(time.RFC3339))
	}

	// 4. Verify Email presence
	if claims.Email == "" {
		return nil, fmt.Errorf("OIDC token contains no email claim")
	}

	firstName := claims.GivenName
	lastName := claims.FamilyName
	if firstName == "" && claims.Name != "" {
		names := strings.SplitN(claims.Name, " ", 2)
		firstName = names[0]
		if len(names) > 1 {
			lastName = names[1]
		}
	}

	return &FederatedIdentity{
		ExternalID: claims.Subject,
		Email:      claims.Email,
		FirstName:  firstName,
		LastName:   lastName,
		Groups:     claims.Groups,
		MappedRole: models.RoleMember,
		Provider:   ProviderTypeOIDC,
		RawClaims: map[string]string{
			"iss": claims.Issuer,
			"sub": claims.Subject,
			"hd":  claims.HD,
		},
	}, nil
}

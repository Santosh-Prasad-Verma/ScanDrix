package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// LicenseClaims specifies enterprise tier allowances and features.
type LicenseClaims struct {
	LicenseID          uuid.UUID `json:"license_id"`
	WorkspaceID        uuid.UUID `json:"workspace_id"`
	CustomerName       string    `json:"customer_name"`
	Tier               string    `json:"tier"` // standard, enterprise, unlimited
	MaxSeats           int       `json:"max_seats"`
	AllowBYOK          bool      `json:"allow_byok"`
	AllowAirGap        bool      `json:"allow_air_gap"`
	AllowDORAAnalytics bool      `json:"allow_dora_analytics"`
	IssuedAt           time.Time `json:"issued_at"`
	ExpiresAt          time.Time `json:"expires_at"`
}

// SignedLicenseEnvelope contains the JSON payload and Ed25519 signature.
type SignedLicenseEnvelope struct {
	PayloadB64   string `json:"payload"`
	SignatureB64 string `json:"signature"`
}

// LicenseVerifier validates digital licenses against an enterprise root public key.
type LicenseVerifier struct {
	publicKey ed25519.PublicKey
}

func NewLicenseVerifier(pubKey ed25519.PublicKey) *LicenseVerifier {
	return &LicenseVerifier{publicKey: pubKey}
}

// VerifyLicense validates the cryptographic Ed25519 signature and temporal validity.
func (v *LicenseVerifier) VerifyLicense(rawJSON []byte) (*LicenseClaims, error) {
	var env SignedLicenseEnvelope
	if err := json.Unmarshal(rawJSON, &env); err != nil {
		return nil, fmt.Errorf("invalid license JSON format: %w", err)
	}

	payloadBytes, err := base64.StdEncoding.DecodeString(env.PayloadB64)
	if err != nil {
		return nil, fmt.Errorf("invalid payload base64: %w", err)
	}

	sigBytes, err := base64.StdEncoding.DecodeString(env.SignatureB64)
	if err != nil {
		return nil, fmt.Errorf("invalid signature base64: %w", err)
	}

	if !ed25519.Verify(v.publicKey, payloadBytes, sigBytes) {
		return nil, errors.New("license signature verification failed: invalid or tampered license")
	}

	var claims LicenseClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("failed decoding claims: %w", err)
	}

	now := time.Now().UTC()
	if now.After(claims.ExpiresAt) {
		return nil, fmt.Errorf("license expired on %s", claims.ExpiresAt.Format(time.RFC3339))
	}

	return &claims, nil
}

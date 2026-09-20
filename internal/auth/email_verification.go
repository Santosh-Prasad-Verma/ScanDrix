package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrEmailTokenExpired is returned when the email verification token has expired.
	ErrEmailTokenExpired = errors.New("email verification token has expired")
	// ErrEmailTokenInvalid is returned when the token is forged, malformed, or exceeds size limits.
	ErrEmailTokenInvalid = errors.New("invalid or tampered email verification token")
)

// EmailConfirmationClaims contains verified metadata from an email confirmation token.
type EmailConfirmationClaims struct {
	TokenID   uuid.UUID
	UserID    uuid.UUID
	Email     string
	ExpiresAt time.Time
}

type emailConfirmationPayload struct {
	TokenID   uuid.UUID `json:"tid"`
	UserID    uuid.UUID `json:"uid"`
	Email     string    `json:"email"`
	ExpiresAt int64     `json:"exp"`
	Purpose   string    `json:"pur"`
}

// CreateEmailConfirmationToken generates a cryptographically signed, JSON-encoded email confirmation token.
// Guarantees ttl > 0 to prevent issuing expired or invalid tokens.
func CreateEmailConfirmationToken(userID uuid.UUID, email, signingSecret string, ttl time.Duration) (string, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	if cleanEmail == "" || signingSecret == "" {
		return "", errors.New("email and signingSecret are required")
	}
	if ttl <= 0 {
		return "", errors.New("ttl must be positive")
	}

	expiresAt := time.Now().UTC().Add(ttl).Unix()
	payload := emailConfirmationPayload{
		TokenID:   uuid.New(),
		UserID:    userID,
		Email:     cleanEmail,
		ExpiresAt: expiresAt,
		Purpose:   "confirm",
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed marshaling email confirmation payload: %w", err)
	}

	b64Payload := base64.RawURLEncoding.EncodeToString(payloadJSON)

	mac := hmac.New(sha256.New, []byte(signingSecret+":email-confirmation"))
	mac.Write([]byte(b64Payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("%s.%s", b64Payload, sig), nil
}

// VerifyEmailConfirmationToken validates the HMAC-SHA256 signature and expiry of an email confirmation token.
// Enforces a 1024-byte maximum size limit to prevent memory DoS attacks before decoding.
func VerifyEmailConfirmationToken(tokenStr, signingSecret string) (*EmailConfirmationClaims, error) {
	// Guard against unbounded/pathological token strings
	if len(tokenStr) == 0 || len(tokenStr) > 1024 {
		return nil, ErrEmailTokenInvalid
	}

	parts := strings.Split(tokenStr, ".")
	if len(parts) != 2 {
		return nil, ErrEmailTokenInvalid
	}

	b64Payload, receivedSig := parts[0], parts[1]

	mac := hmac.New(sha256.New, []byte(signingSecret+":email-confirmation"))
	mac.Write([]byte(b64Payload))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(receivedSig), []byte(expectedSig)) != 1 {
		return nil, ErrEmailTokenInvalid
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(b64Payload)
	if err != nil {
		return nil, ErrEmailTokenInvalid
	}

	var payload emailConfirmationPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, ErrEmailTokenInvalid
	}

	if payload.Purpose != "confirm" || payload.UserID == uuid.Nil || payload.Email == "" {
		return nil, ErrEmailTokenInvalid
	}

	expiresAt := time.Unix(payload.ExpiresAt, 0).UTC()
	if time.Now().UTC().After(expiresAt) {
		return nil, ErrEmailTokenExpired
	}

	return &EmailConfirmationClaims{
		TokenID:   payload.TokenID,
		UserID:    payload.UserID,
		Email:     payload.Email,
		ExpiresAt: expiresAt,
	}, nil
}

package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrResetTokenExpired  = errors.New("password reset token has expired")
	ErrResetTokenInvalid  = errors.New("invalid or tampered password reset token")
	ErrResetTokenStale    = errors.New("password reset token is stale (password already updated)")
)

// ResetClaims represents verified metadata extracted from a password reset token.
type ResetClaims struct {
	UserID    uuid.UUID
	Email     string
	ExpiresAt time.Time
}

// CreatePasswordResetToken generates a cryptographically signed, time-limited reset token.
// The HMAC-SHA256 signature binds the current password hash, ensuring that as soon as the password
// is reset, any previously issued tokens are invalidated instantly.
func CreatePasswordResetToken(userID uuid.UUID, email, currentPasswordHash, jwtSecret string, ttl time.Duration) (string, error) {
	if email == "" || jwtSecret == "" {
		return "", errors.New("email and jwtSecret are required")
	}

	expiresAt := time.Now().UTC().Add(ttl).Unix()
	rawPayload := fmt.Sprintf("%s:%s:%d", userID.String(), email, expiresAt)
	b64Payload := base64.RawURLEncoding.EncodeToString([]byte(rawPayload))

	// Bind signature to server secret + user current password hash
	mac := hmac.New(sha256.New, []byte(jwtSecret+currentPasswordHash))
	mac.Write([]byte(b64Payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("%s.%s", b64Payload, sig), nil
}

// VerifyPasswordResetToken verifies the integrity, expiration, and password-binding of a reset token.
func VerifyPasswordResetToken(tokenStr, currentPasswordHash, jwtSecret string) (*ResetClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 2 {
		return nil, ErrResetTokenInvalid
	}

	b64Payload, receivedSig := parts[0], parts[1]

	// 1. Recompute expected signature using current password hash
	mac := hmac.New(sha256.New, []byte(jwtSecret+currentPasswordHash))
	mac.Write([]byte(b64Payload))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(receivedSig), []byte(expectedSig)) != 1 {
		return nil, ErrResetTokenInvalid
	}

	// 2. Decode payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(b64Payload)
	if err != nil {
		return nil, ErrResetTokenInvalid
	}

	elements := strings.Split(string(payloadBytes), ":")
	if len(elements) != 3 {
		return nil, ErrResetTokenInvalid
	}

	parsedID, err := uuid.Parse(elements[0])
	if err != nil {
		return nil, ErrResetTokenInvalid
	}

	email := elements[1]
	expUnix, err := strconv.ParseInt(elements[2], 10, 64)
	if err != nil {
		return nil, ErrResetTokenInvalid
	}

	expiresAt := time.Unix(expUnix, 0).UTC()
	if time.Now().UTC().After(expiresAt) {
		return nil, ErrResetTokenExpired
	}

	return &ResetClaims{
		UserID:    parsedID,
		Email:     email,
		ExpiresAt: expiresAt,
	}, nil
}

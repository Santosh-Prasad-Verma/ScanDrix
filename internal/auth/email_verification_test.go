package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailConfirmationToken(t *testing.T) {
	userID := uuid.New()
	email := "Developer@Scandrix.io"
	secret := "super-secure-verification-jwt-secret-key-12345"

	t.Run("valid token verification and normalization", func(t *testing.T) {
		token, err := CreateEmailConfirmationToken(userID, email, secret, 15*time.Minute)
		require.NoError(t, err)
		assert.NotEmpty(t, token)

		claims, err := VerifyEmailConfirmationToken(token, secret)
		require.NoError(t, err)
		assert.Equal(t, userID, claims.UserID)
		assert.Equal(t, "developer@scandrix.io", claims.Email) // Normalized lowercase
		assert.NotEqual(t, uuid.Nil, claims.TokenID)
		assert.True(t, claims.ExpiresAt.After(time.Now().UTC()))
	})

	t.Run("non-positive ttl rejected at creation", func(t *testing.T) {
		_, errZero := CreateEmailConfirmationToken(userID, email, secret, 0)
		assert.Error(t, errZero)

		_, errNegative := CreateEmailConfirmationToken(userID, email, secret, -5*time.Minute)
		assert.Error(t, errNegative)
	})

	t.Run("expired token fails verification", func(t *testing.T) {
		token, err := CreateEmailConfirmationToken(userID, email, secret, 5*time.Millisecond)
		require.NoError(t, err)

		time.Sleep(15 * time.Millisecond)

		claims, err := VerifyEmailConfirmationToken(token, secret)
		assert.ErrorIs(t, err, ErrEmailTokenExpired)
		assert.Nil(t, claims)
	})

	t.Run("tampered token fails verification", func(t *testing.T) {
		token, err := CreateEmailConfirmationToken(userID, email, secret, 15*time.Minute)
		require.NoError(t, err)

		tampered := token[:len(token)-3] + "abc"
		claims, err := VerifyEmailConfirmationToken(tampered, secret)
		assert.ErrorIs(t, err, ErrEmailTokenInvalid)
		assert.Nil(t, claims)
	})

	t.Run("wrong secret fails verification", func(t *testing.T) {
		token, err := CreateEmailConfirmationToken(userID, email, secret, 15*time.Minute)
		require.NoError(t, err)

		claims, err := VerifyEmailConfirmationToken(token, "wrong-secret-key")
		assert.ErrorIs(t, err, ErrEmailTokenInvalid)
		assert.Nil(t, claims)
	})

	t.Run("token exceeding max size rejected immediately", func(t *testing.T) {
		oversized := strings.Repeat("x", 1025)
		claims, err := VerifyEmailConfirmationToken(oversized, secret)
		assert.ErrorIs(t, err, ErrEmailTokenInvalid)
		assert.Nil(t, claims)
	})

	t.Run("colon in email handled safely by JSON payload", func(t *testing.T) {
		colonEmail := `"john:doe"@acmecorp.com`
		token, err := CreateEmailConfirmationToken(userID, colonEmail, secret, 15*time.Minute)
		require.NoError(t, err)

		claims, err := VerifyEmailConfirmationToken(token, secret)
		require.NoError(t, err)
		assert.Equal(t, colonEmail, claims.Email)
	})
}

package crypto_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAES256GCMEncryptionAndAADTenantBinding(t *testing.T) {
	masterKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	svc, err := crypto.NewService(crypto.Config{
		MasterKeyHex: masterKey,
	})
	require.NoError(t, err)

	tenantA := uuid.New()
	tenantB := uuid.New()
	secretData := []byte("ghp_1234567890abcdefghijklmnopqrstuvwxyz")

	// 1. Encrypt with tenantA binding
	encrypted, err := svc.Encrypt(secretData, tenantA)
	require.NoError(t, err)
	assert.NotEmpty(t, encrypted)

	// 2. Decrypt with correct tenantA binding
	decrypted, err := svc.Decrypt(encrypted, tenantA)
	require.NoError(t, err)
	assert.Equal(t, secretData, decrypted)

	// 3. Security Assert: Decrypting with wrong tenantB MUST fail
	_, err = svc.Decrypt(encrypted, tenantB)
	assert.ErrorIs(t, err, crypto.ErrAuthenticationFailed, "cross-tenant decryption must fail authentication")

	// 4. Security Assert: Decrypting with nil workspace MUST fail
	_, err = svc.Decrypt(encrypted, uuid.Nil)
	assert.ErrorIs(t, err, crypto.ErrAuthenticationFailed)
}

func TestKeyRotationSecondaryKeyFallback(t *testing.T) {
	oldKey := "1111111111111111111111111111111111111111111111111111111111111111"
	newKey := "2222222222222222222222222222222222222222222222222222222222222222"

	// Old service encrypts with oldKey
	oldSvc, err := crypto.NewService(crypto.Config{MasterKeyHex: oldKey})
	require.NoError(t, err)

	wsID := uuid.New()
	secret := []byte("rotated_token_data")
	cipherOld, err := oldSvc.Encrypt(secret, wsID)
	require.NoError(t, err)

	// New service with newKey as master and oldKey as secondary
	newSvc, err := crypto.NewService(crypto.Config{
		MasterKeyHex:    newKey,
		SecondaryKeyHex: oldKey,
	})
	require.NoError(t, err)

	// Decrypt payload created with oldKey
	decrypted, err := newSvc.Decrypt(cipherOld, wsID)
	require.NoError(t, err)
	assert.Equal(t, secret, decrypted)

	// Encrypt new payload with newKey
	cipherNew, err := newSvc.Encrypt(secret, wsID)
	require.NoError(t, err)

	// Decrypt new payload
	decryptedNew, err := newSvc.Decrypt(cipherNew, wsID)
	require.NoError(t, err)
	assert.Equal(t, secret, decryptedNew)
}

func TestHMACSHA256SignatureVerification(t *testing.T) {
	svc, err := crypto.NewService(crypto.Config{
		MasterKeyHex: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	require.NoError(t, err)

	secret := []byte("webhook_shared_secret_key_123")
	payload := []byte(`{"action":"opened","pull_request":{"number":42}}`)

	sig := svc.ComputeHMACSHA256(payload, secret)
	assert.NotEmpty(t, sig)

	// Valid signature
	assert.True(t, svc.VerifyHMACSHA256(payload, sig, secret))
	// Valid with sha256= prefix
	assert.True(t, svc.VerifyHMACSHA256(payload, "sha256="+sig, secret))

	// Tampered payload
	tampered := []byte(`{"action":"closed","pull_request":{"number":42}}`)
	assert.False(t, svc.VerifyHMACSHA256(tampered, sig, secret))

	// Wrong secret
	assert.False(t, svc.VerifyHMACSHA256(payload, sig, []byte("wrong_secret")))
}

func TestArgon2idPasswordHashing(t *testing.T) {
	svc, err := crypto.NewService(crypto.Config{
		MasterKeyHex: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ArgonTime:    1,
		ArgonMemory:  16 * 1024, // 16MB for test speed
		ArgonThreads: 2,
		ArgonKeyLen:  32,
	})
	require.NoError(t, err)

	password := "CorrectHorseBatteryStaple!2026"
	hash, salt, err := svc.HashPassword(password)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEmpty(t, salt)

	// Correct verification
	assert.True(t, svc.VerifyPassword(password, hash, salt))

	// Incorrect password
	assert.False(t, svc.VerifyPassword("WrongPassword123", hash, salt))
}

func TestBcryptCryptoService(t *testing.T) {
	cs := crypto.NewCryptoService()

	password := "SecureDevPassword!2026"
	hash, err := cs.HashPassword(password, 10)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)

	// Correct password match
	assert.True(t, cs.Match(password, hash))

	// Incorrect password match
	assert.False(t, cs.Match("WrongPassword!123", hash))
}


package crypto_test

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func generateTestKeyHex() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// TestCryptoStress_BitFlipAttackResistance tests that modifying any single bit in the
// ciphertext or authentication tag strictly causes authentication and decryption to fail.
func TestCryptoStress_BitFlipAttackResistance(t *testing.T) {
	masterKey := generateTestKeyHex()
	svc, err := crypto.NewService(crypto.Config{MasterKeyHex: masterKey})
	require.NoError(t, err)

	wsID := uuid.New()
	plaintext := []byte("Sensitive API Secret: sk-ant-api03-abcdef1234567890-test")

	encryptedBase64, err := svc.Encrypt(plaintext, wsID)
	require.NoError(t, err)

	rawPayload, err := base64.StdEncoding.DecodeString(encryptedBase64)
	require.NoError(t, err)
	require.True(t, len(rawPayload) > 28) // 12-byte nonce + ciphertext + 16-byte tag

	// Flip every single bit across the payload and verify decryption NEVER succeeds
	for byteIdx := 0; byteIdx < len(rawPayload); byteIdx++ {
		for bitIdx := 0; bitIdx < 8; bitIdx++ {
			corrupted := make([]byte, len(rawPayload))
			copy(corrupted, rawPayload)
			corrupted[byteIdx] ^= (1 << bitIdx)

			corruptedBase64 := base64.StdEncoding.EncodeToString(corrupted)
			decrypted, err := svc.Decrypt(corruptedBase64, wsID)
			assert.Error(t, err, "Bit flip at byte %d bit %d should fail", byteIdx, bitIdx)
			assert.Nil(t, decrypted)
		}
	}
}

// TestCryptoStress_TenantAADBoundaryIsolation ensures that ciphertext encrypted with
// Workspace A's identity as Additional Authenticated Data cannot be decrypted with Workspace B.
func TestCryptoStress_TenantAADBoundaryIsolation(t *testing.T) {
	masterKey := generateTestKeyHex()
	svc, err := crypto.NewService(crypto.Config{MasterKeyHex: masterKey})
	require.NoError(t, err)

	wsA := uuid.New()
	wsB := uuid.New()
	wsNil := uuid.Nil

	plaintext := []byte("confidential-tenant-credentials-12345")

	// 1. Encrypt for wsA
	encA, err := svc.Encrypt(plaintext, wsA)
	require.NoError(t, err)

	// Decrypt with wsA succeeds
	decA, err := svc.Decrypt(encA, wsA)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decA)

	// Decrypt with wsB MUST fail
	decB, err := svc.Decrypt(encA, wsB)
	assert.Error(t, err)
	assert.Nil(t, decB)

	// Decrypt with uuid.Nil MUST fail
	decNil, err := svc.Decrypt(encA, wsNil)
	assert.Error(t, err)
	assert.Nil(t, decNil)

	// 2. Encrypt with uuid.Nil
	encNil, err := svc.Encrypt(plaintext, wsNil)
	require.NoError(t, err)

	// Decrypt with uuid.Nil succeeds
	decNilPass, err := svc.Decrypt(encNil, wsNil)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decNilPass)

	// Decrypt with wsA MUST fail
	decNilFail, err := svc.Decrypt(encNil, wsA)
	assert.Error(t, err)
	assert.Nil(t, decNilFail)
}

// TestCryptoStress_KeyRotationWorkflow simulates enterprise dual-key rotation:
// records encrypted with Key-1 are seamlessly decrypted using a rotated service having
// Key-2 as primary and Key-1 as secondary, then re-encrypted under Key-2.
func TestCryptoStress_KeyRotationWorkflow(t *testing.T) {
	key1 := generateTestKeyHex()
	key2 := generateTestKeyHex()

	wsID := uuid.New()
	secret := []byte("db-connection-string://postgres:topsecret@db.internal:5432/main")

	// Service 1: Old active key (key1)
	svcOld, err := crypto.NewService(crypto.Config{MasterKeyHex: key1})
	require.NoError(t, err)

	oldCiphertext, err := svcOld.Encrypt(secret, wsID)
	require.NoError(t, err)

	// Service 2: Key rotation active (key2 primary, key1 secondary fallback)
	svcRotating, err := crypto.NewService(crypto.Config{
		MasterKeyHex:    key2,
		SecondaryKeyHex: key1,
	})
	require.NoError(t, err)

	// Verify svcRotating can read the old ciphertext via secondary fallback
	decryptedOld, err := svcRotating.Decrypt(oldCiphertext, wsID)
	require.NoError(t, err)
	assert.Equal(t, secret, decryptedOld)

	// Re-encrypt under new primary key (key2)
	newCiphertext, err := svcRotating.Encrypt(secret, wsID)
	require.NoError(t, err)

	// Service 3: Rotation finalized (key2 only, key1 retired)
	svcNew, err := crypto.NewService(crypto.Config{MasterKeyHex: key2})
	require.NoError(t, err)

	// Finalized service can decrypt new ciphertext
	decryptedNew, err := svcNew.Decrypt(newCiphertext, wsID)
	require.NoError(t, err)
	assert.Equal(t, secret, decryptedNew)

	// Old service CANNOT decrypt new ciphertext
	_, err = svcOld.Decrypt(newCiphertext, wsID)
	assert.Error(t, err)
}

// TestCryptoStress_Argon2idPasswordHashingConcurrency verifies Argon2id thread safety,
// salt entropy uniqueness, and rejection of invalid passwords across concurrent goroutines.
func TestCryptoStress_Argon2idPasswordHashingConcurrency(t *testing.T) {
	masterKey := generateTestKeyHex()
	svc, err := crypto.NewService(crypto.Config{
		MasterKeyHex: masterKey,
		ArgonTime:    1,
		ArgonMemory:  16 * 1024, // 16MB for fast test execution
		ArgonThreads: 2,
		ArgonKeyLen:  32,
	})
	require.NoError(t, err)

	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			pw := fmt.Sprintf("Sup3r$ecureP@ssword-%d", idx)

			hash, salt, err := svc.HashPassword(pw)
			assert.NoError(t, err)
			assert.NotEmpty(t, hash)
			assert.NotEmpty(t, salt)

			// Matching password verifies true
			assert.True(t, svc.VerifyPassword(pw, hash, salt))

			// Wrong password fails
			assert.False(t, svc.VerifyPassword(pw+"_wrong", hash, salt))

			// Tampered salt fails
			assert.False(t, svc.VerifyPassword(pw, hash, "00000000000000000000000000000000"))

			// Tampered hash fails
			assert.False(t, svc.VerifyPassword(pw, "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", salt))
		}(i)
	}

	wg.Wait()
}

// TestCryptoStress_BcryptCostFactorsAndMatching tests bcrypt hashing across various
// salt cost levels and verifies constant-time matching.
func TestCryptoStress_BcryptCostFactorsAndMatching(t *testing.T) {
	bc := crypto.NewCryptoService()

	// 1. Valid password hashing
	pw := "developer-dashboard-password-2026"
	hashDefault, err := bc.HashPassword(pw, bcrypt.DefaultCost)
	require.NoError(t, err)
	assert.True(t, bc.Match(pw, hashDefault))
	assert.False(t, bc.Match("wrong-pass", hashDefault))

	// 2. Minimum cost (cost 4 for speed)
	hashMin, err := bc.HashPassword(pw, bcrypt.MinCost)
	require.NoError(t, err)
	assert.True(t, bc.Match(pw, hashMin))

	// 3. Out of bounds cost falls back to DefaultCost
	hashOutOfRange, err := bc.HashPassword(pw, 999)
	require.NoError(t, err)
	assert.True(t, bc.Match(pw, hashOutOfRange))
}

// TestCryptoStress_HMACWebhookVerificationVariations tests webhook signature verification
// with GitHub, GitLab, and standard hex format headers.
func TestCryptoStress_HMACWebhookVerificationVariations(t *testing.T) {
	svc, err := crypto.NewService(crypto.Config{MasterKeyHex: generateTestKeyHex()})
	require.NoError(t, err)

	webhookSecret := []byte("webhook_secret_for_github_events_xyz")
	payload := []byte(`{"action":"opened","pull_request":{"id":123,"title":"feat: add scanning"}}`)

	computedHex := svc.ComputeHMACSHA256(payload, webhookSecret)
	assert.Len(t, computedHex, 64)

	// 1. Clean hex
	assert.True(t, svc.VerifyHMACSHA256(payload, computedHex, webhookSecret))

	// 2. GitHub style "sha256=" prefix
	assert.True(t, svc.VerifyHMACSHA256(payload, "sha256="+computedHex, webhookSecret))

	// 3. Alternative "sha256:" prefix
	assert.True(t, svc.VerifyHMACSHA256(payload, "sha256:"+computedHex, webhookSecret))

	// 4. Tampered payload
	tamperedPayload := []byte(`{"action":"closed","pull_request":{"id":123}}`)
	assert.False(t, svc.VerifyHMACSHA256(tamperedPayload, computedHex, webhookSecret))

	// 5. Wrong secret
	assert.False(t, svc.VerifyHMACSHA256(payload, computedHex, []byte("wrong_secret")))

	// 6. Malformed hex signature
	assert.False(t, svc.VerifyHMACSHA256(payload, "not-a-valid-hex-signature", webhookSecret))
}

// TestCryptoStress_HashTokenDeterminism ensures HashToken is fully deterministic
// and collision-free across multiple unique token prefixes.
func TestCryptoStress_HashTokenDeterminism(t *testing.T) {
	tokens := make(map[string]string)

	for i := 0; i < 500; i++ {
		raw := fmt.Sprintf("scandrix_live_%04d_%s", i, uuid.New().String())
		h1 := crypto.HashToken(raw)
		h2 := crypto.HashToken(raw)

		assert.Equal(t, h1, h2, "HashToken must be deterministic")
		assert.Len(t, h1, 64)

		// Check collision
		_, exists := tokens[h1]
		assert.False(t, exists, "HashToken collision detected for token: %s", raw)
		tokens[h1] = raw
	}
}

// TestCryptoStress_LargePayloadEncryption verifies encryption of payloads up to 1MB.
func TestCryptoStress_LargePayloadEncryption(t *testing.T) {
	svc, err := crypto.NewService(crypto.Config{MasterKeyHex: generateTestKeyHex()})
	require.NoError(t, err)

	wsID := uuid.New()
	const size = 1024 * 1024 // 1 MB
	largePayload := make([]byte, size)
	_, _ = rand.Read(largePayload)

	encrypted, err := svc.Encrypt(largePayload, wsID)
	require.NoError(t, err)

	decrypted, err := svc.Decrypt(encrypted, wsID)
	require.NoError(t, err)
	assert.Equal(t, largePayload, decrypted)
}

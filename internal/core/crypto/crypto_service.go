package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidKeyLength     = errors.New("encryption key must be exactly 32 bytes for AES-256")
	ErrCiphertextTooShort   = errors.New("ciphertext payload too short or malformed")
	ErrAuthenticationFailed = errors.New("ciphertext authentication failed (tag mismatch or corrupted data)")
	ErrTenantMismatch       = errors.New("AAD workspace binding verification failed")
)

// Config defines cryptographic parameters and encryption master keys.
type Config struct {
	MasterKeyHex       string
	SecondaryKeyHex    string // Used for key rotation; decrypts old records during migration
	ArgonTime          uint32
	ArgonMemory        uint32
	ArgonThreads       uint8
	ArgonKeyLen        uint32
}

// Service provides enterprise-grade cryptographic operations.
type Service struct {
	primaryGCM   cipher.AEAD
	secondaryGCM cipher.AEAD
	cfg          Config
}

// NewService instantiates a crypto service from raw 32-byte hex keys.
func NewService(cfg Config) (*Service, error) {
	if cfg.MasterKeyHex == "" {
		return nil, errors.New("master encryption key cannot be empty")
	}
	masterKey, err := hex.DecodeString(cfg.MasterKeyHex)
	if err != nil || len(masterKey) != 32 {
		// Fallback: derive 32-byte key via SHA-256 if hex is not formatted
		h := sha256.Sum256([]byte(cfg.MasterKeyHex))
		masterKey = h[:]
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed creating primary AES cipher: %w", err)
	}
	primaryGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed creating primary GCM: %w", err)
	}

	var secondaryGCM cipher.AEAD
	if cfg.SecondaryKeyHex != "" {
		secKey, err := hex.DecodeString(cfg.SecondaryKeyHex)
		if err != nil || len(secKey) != 32 {
			h := sha256.Sum256([]byte(cfg.SecondaryKeyHex))
			secKey = h[:]
		}
		secBlock, err := aes.NewCipher(secKey)
		if err == nil {
			secondaryGCM, _ = cipher.NewGCM(secBlock)
		}
	}

	// Sane defaults for Argon2id if not provided
	if cfg.ArgonTime == 0 {
		cfg.ArgonTime = 3
	}
	if cfg.ArgonMemory == 0 {
		cfg.ArgonMemory = 64 * 1024 // 64MB
	}
	if cfg.ArgonThreads == 0 {
		cfg.ArgonThreads = 2
	}
	if cfg.ArgonKeyLen == 0 {
		cfg.ArgonKeyLen = 32
	}

	return &Service{
		primaryGCM:   primaryGCM,
		secondaryGCM: secondaryGCM,
		cfg:          cfg,
	}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM.
// If workspaceID is not uuid.Nil, it is passed as Additional Authenticated Data (AAD)
// to cryptographically bind the ciphertext to that tenant.
func (s *Service) Encrypt(plaintext []byte, workspaceID uuid.UUID) (string, error) {
	if s == nil || s.primaryGCM == nil {
		return "", errors.New("crypto service is not initialized")
	}
	nonce := make([]byte, s.primaryGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed generating random nonce: %w", err)
	}

	var aad []byte
	if workspaceID != uuid.Nil {
		aad = workspaceID[:]
	}

	ciphertext := s.primaryGCM.Seal(nil, nonce, plaintext, aad)
	// Combine nonce + ciphertext
	payload := make([]byte, len(nonce)+len(ciphertext))
	copy(payload, nonce)
	copy(payload[len(nonce):], ciphertext)

	return base64.StdEncoding.EncodeToString(payload), nil
}

// Decrypt authenticates and decrypts an AES-256-GCM payload.
// It verifies the AAD workspace binding and tries the primary key, then the secondary key if rotating.
func (s *Service) Decrypt(encoded string, workspaceID uuid.UUID) ([]byte, error) {
	if s == nil || s.primaryGCM == nil {
		return nil, errors.New("crypto service is not initialized")
	}
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed decoding base64 ciphertext: %w", err)
	}

	nonceSize := s.primaryGCM.NonceSize()
	if len(payload) < nonceSize {
		return nil, ErrCiphertextTooShort
	}

	nonce := payload[:nonceSize]
	ciphertext := payload[nonceSize:]

	var aad []byte
	if workspaceID != uuid.Nil {
		aad = workspaceID[:]
	}

	// Try primary key first
	plaintext, err := s.primaryGCM.Open(nil, nonce, ciphertext, aad)
	if err == nil {
		return plaintext, nil
	}

	// If failed and secondary key exists, attempt decryption with previous master key
	if s.secondaryGCM != nil {
		secPlaintext, secErr := s.secondaryGCM.Open(nil, nonce, ciphertext, aad)
		if secErr == nil {
			return secPlaintext, nil
		}
	}

	return nil, ErrAuthenticationFailed
}

// ComputeHMACSHA256 generates a hex-encoded HMAC-SHA256 signature for webhook payloads.
func (s *Service) ComputeHMACSHA256(message []byte, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(message)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyHMACSHA256 validates an incoming webhook signature using constant-time comparison.
func (s *Service) VerifyHMACSHA256(message []byte, signatureHex string, secret []byte) bool {
	expectedSig := s.ComputeHMACSHA256(message, secret)
	// Strip "sha256=" prefix if present (common in GitHub webhooks)
	cleanSignature := strings.TrimPrefix(signatureHex, "sha256=")
	cleanSignature = strings.TrimPrefix(cleanSignature, "sha256:")

	sigA, errA := hex.DecodeString(expectedSig)
	sigB, errB := hex.DecodeString(cleanSignature)
	if errA != nil || errB != nil {
		return false
	}
	return subtle.ConstantTimeCompare(sigA, sigB) == 1
}

// HashToken generates a deterministic SHA-256 hex digest for indexing API keys and secrets in the database.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// HashPassword generates an Argon2id password hash with a random 16-byte cryptographic salt.
func (s *Service) HashPassword(password string) (hashHex string, saltHex string, err error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", "", fmt.Errorf("entropy error reading salt: %w", err)
	}
	derivedKey := argon2.IDKey(
		[]byte(password),
		salt,
		s.cfg.ArgonTime,
		s.cfg.ArgonMemory,
		s.cfg.ArgonThreads,
		s.cfg.ArgonKeyLen,
	)
	return hex.EncodeToString(derivedKey), hex.EncodeToString(salt), nil
}

// VerifyPassword verifies a plaintext password against the stored Argon2id hash and salt.
func (s *Service) VerifyPassword(password string, storedHashHex string, storedSaltHex string) bool {
	salt, err := hex.DecodeString(storedSaltHex)
	if err != nil || len(salt) != 16 {
		return false
	}
	expectedHash, err := hex.DecodeString(storedHashHex)
	if err != nil {
		return false
	}
	computedKey := argon2.IDKey(
		[]byte(password),
		salt,
		s.cfg.ArgonTime,
		s.cfg.ArgonMemory,
		s.cfg.ArgonThreads,
		s.cfg.ArgonKeyLen,
	)
	return subtle.ConstantTimeCompare(computedKey, expectedHash) == 1
}

// CryptoService mirrors ScanDrix CryptoService using bcrypt for password hashing and matching.
type CryptoService struct{}

// NewCryptoService instantiates a new CryptoService.
func NewCryptoService() *CryptoService {
	return &CryptoService{}
}

// HashPassword hashes a password using bcrypt with the specified salt rounds.
func (c *CryptoService) HashPassword(password string, saltCost int) (string, error) {
	if saltCost < bcrypt.MinCost || saltCost > bcrypt.MaxCost {
		saltCost = bcrypt.DefaultCost
	}
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), saltCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// Match verifies that an entered plaintext password matches the stored bcrypt hash.
func (c *CryptoService) Match(enteredPassword, dbPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(dbPassword), []byte(enteredPassword))
	return err == nil
}


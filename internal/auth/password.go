package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultBcryptCost sets the baseline OWASP-recommended cost factor (12 salt rounds).
	DefaultBcryptCost = 12
	// MaxBcryptPasswordLength is the algorithmic maximum input length supported by bcrypt.
	MaxBcryptPasswordLength = 72

	// RFC 9106 / OWASP ASVS Recommended Argon2id Baseline Parameters
	DefaultArgon2Memory      uint32 = 64 * 1024 // 64 MiB (65,536 KiB)
	DefaultArgon2Iterations  uint32 = 3         // 3 passes over memory
	DefaultArgon2Parallelism uint8  = 4         // 4 threads / parallel lanes
	DefaultArgon2SaltLength  uint32 = 16        // 16 bytes CSPRNG salt
	DefaultArgon2KeyLength   uint32 = 32        // 32 bytes (256-bit derived key)

	AlgoArgon2id = "argon2id"
	AlgoBcrypt   = "bcrypt"
)

var (
	// ErrPasswordTooLong indicates the password exceeds the 72-byte limit of bcrypt.
	ErrPasswordTooLong = errors.New("password exceeds maximum allowed length of 72 bytes")
	// ErrInvalidHash indicates the stored password hash string is malformed or corrupted.
	ErrInvalidHash = errors.New("invalid or malformed password hash format")

	configuredBcryptCost atomic.Int32

	configuredArgon2Memory      atomic.Uint32
	configuredArgon2Iterations  atomic.Uint32
	configuredArgon2Parallelism atomic.Uint32
	configuredArgon2SaltLength  atomic.Uint32
	configuredArgon2KeyLength   atomic.Uint32

	activeHashAlgo atomic.Pointer[string]

	// Pre-computed dummy salt for constant-time anti-enumeration verification
	dummyArgon2Salt = []byte("scandrix_dummy_s")
	dummyArgon2Key  = make([]byte, 32)
	dummyBcryptHash = "$2a$12$e8Yd8wXFvB8J5F7c1dE4XuP8pZ.wNqQ2Yv5XGjE4yV4fOqJ8uB2m."
)

// Argon2Params encapsulates tunable key derivation parameters.
type Argon2Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

func init() {
	// Initialize Bcrypt cost from environment or default
	cost := int32(DefaultBcryptCost)
	if raw := os.Getenv("BCRYPT_COST"); raw != "" {
		if val, err := strconv.Atoi(raw); err == nil {
			if val >= bcrypt.MinCost && val <= bcrypt.MaxCost {
				cost = int32(val)
			}
		}
	}
	configuredBcryptCost.Store(cost)

	// Initialize Argon2id parameters from environment or OWASP defaults
	mem := DefaultArgon2Memory
	if raw := os.Getenv("ARGON2_MEMORY"); raw != "" {
		if val, err := strconv.ParseUint(raw, 10, 32); err == nil && val >= 8*1024 {
			mem = uint32(val)
		}
	}
	configuredArgon2Memory.Store(mem)

	iter := DefaultArgon2Iterations
	if raw := os.Getenv("ARGON2_ITERATIONS"); raw != "" {
		if val, err := strconv.ParseUint(raw, 10, 32); err == nil && val >= 1 {
			iter = uint32(val)
		}
	}
	configuredArgon2Iterations.Store(iter)

	par := DefaultArgon2Parallelism
	if raw := os.Getenv("ARGON2_PARALLELISM"); raw != "" {
		if val, err := strconv.ParseUint(raw, 10, 8); err == nil && val >= 1 {
			par = uint8(val)
		}
	}
	configuredArgon2Parallelism.Store(uint32(par))

	saltLen := DefaultArgon2SaltLength
	if raw := os.Getenv("ARGON2_SALT_LENGTH"); raw != "" {
		if val, err := strconv.ParseUint(raw, 10, 32); err == nil && val >= 16 {
			saltLen = uint32(val)
		}
	}
	configuredArgon2SaltLength.Store(saltLen)

	keyLen := DefaultArgon2KeyLength
	if raw := os.Getenv("ARGON2_KEY_LENGTH"); raw != "" {
		if val, err := strconv.ParseUint(raw, 10, 32); err == nil && val >= 16 {
			keyLen = uint32(val)
		}
	}
	configuredArgon2KeyLength.Store(keyLen)

	// Initialize primary algorithm (default to argon2id, allow override to bcrypt)
	algo := AlgoArgon2id
	if raw := os.Getenv("PASSWORD_HASH_ALGO"); strings.EqualFold(raw, AlgoBcrypt) {
		algo = AlgoBcrypt
	}
	activeHashAlgo.Store(&algo)
}

// GetHashAlgo returns the active password hashing algorithm name ("argon2id" or "bcrypt").
func GetHashAlgo() string {
	if ptr := activeHashAlgo.Load(); ptr != nil {
		return *ptr
	}
	return AlgoArgon2id
}

// SetHashAlgo sets the active password hashing algorithm.
func SetHashAlgo(algo string) {
	clean := strings.ToLower(strings.TrimSpace(algo))
	if clean != AlgoBcrypt {
		clean = AlgoArgon2id
	}
	activeHashAlgo.Store(&clean)
}

// GetArgon2Params returns the active runtime parameters for Argon2id hashing.
func GetArgon2Params() Argon2Params {
	return Argon2Params{
		Memory:      configuredArgon2Memory.Load(),
		Iterations:  configuredArgon2Iterations.Load(),
		Parallelism: uint8(configuredArgon2Parallelism.Load()),
		SaltLength:  configuredArgon2SaltLength.Load(),
		KeyLength:   configuredArgon2KeyLength.Load(),
	}
}

// SetArgon2Params updates the runtime parameters for Argon2id hashing.
func SetArgon2Params(p Argon2Params) {
	if p.Memory >= 8*1024 {
		configuredArgon2Memory.Store(p.Memory)
	}
	if p.Iterations >= 1 {
		configuredArgon2Iterations.Store(p.Iterations)
	}
	if p.Parallelism >= 1 {
		configuredArgon2Parallelism.Store(uint32(p.Parallelism))
	}
	if p.SaltLength >= 16 {
		configuredArgon2SaltLength.Store(p.SaltLength)
	}
	if p.KeyLength >= 16 {
		configuredArgon2KeyLength.Store(p.KeyLength)
	}
}

// GetBcryptCost returns the current active work factor for hashing new passwords with bcrypt.
func GetBcryptCost() int {
	return int(configuredBcryptCost.Load())
}

// SetBcryptCost dynamically sets the bcrypt work factor (clamped between bcrypt.MinCost and bcrypt.MaxCost).
func SetBcryptCost(cost int) {
	if cost < bcrypt.MinCost {
		cost = bcrypt.MinCost
	}
	if cost > bcrypt.MaxCost {
		cost = bcrypt.MaxCost
	}
	configuredBcryptCost.Store(int32(cost))
}

// HashPassword computes a secure hash of the plaintext password using the active hashing algorithm.
// Defaults to Argon2id with 64MB memory, 3 iterations, and 4 lanes (RFC 9106 / ASVS V2.4).
func HashPassword(password string) (string, error) {
	if len(password) == 0 {
		return "", errors.New("password cannot be empty")
	}

	if GetHashAlgo() == AlgoBcrypt {
		if len(password) > MaxBcryptPasswordLength {
			return "", ErrPasswordTooLong
		}
		hashBytes, err := bcrypt.GenerateFromPassword([]byte(password), GetBcryptCost())
		if err != nil {
			if errors.Is(err, bcrypt.ErrPasswordTooLong) {
				return "", ErrPasswordTooLong
			}
			return "", fmt.Errorf("failed to hash password with bcrypt: %w", err)
		}
		return string(hashBytes), nil
	}

	// Default: Argon2id
	params := GetArgon2Params()
	salt := make([]byte, params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate secure salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, params.Iterations, params.Memory, params.Parallelism, params.KeyLength)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Key := base64.RawStdEncoding.EncodeToString(key)

	// Standard PHC string format: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
	phc := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, params.Memory, params.Iterations, params.Parallelism, b64Salt, b64Key)

	return phc, nil
}

// VerifyPassword verifies the plaintext password against either an Argon2id or Bcrypt hash.
// Automatically detects hash format, executes constant-time byte comparisons, and fails securely.
func VerifyPassword(password, hash string) bool {
	if len(password) == 0 || len(hash) == 0 {
		return false
	}

	// 1. Argon2id PHC string verification ($argon2id$...)
	if strings.HasPrefix(hash, "$argon2id$") {
		return verifyArgon2id(password, hash)
	}

	// 2. Legacy Bcrypt verification ($2a$, $2b$, $2y$)
	if strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$") {
		err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
		if err != nil {
			if !errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
				slog.Error("Corrupted bcrypt password hash during verification", "error", err)
			}
			return false
		}
		return true
	}

	slog.Error("Unrecognized password hash format", "prefix", hash[:min(len(hash), 12)])
	return false
}

func verifyArgon2id(password, hash string) bool {
	parts := strings.Split(hash, "$")
	// Format: "" , "argon2id" , "v=19" , "m=65536,t=3,p=4" , salt , key
	if len(parts) != 6 || parts[1] != "argon2id" {
		slog.Error("Malformed Argon2id hash structure")
		return false
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		slog.Error("Unsupported or missing Argon2id version", "version", parts[2])
		return false
	}

	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		slog.Error("Failed parsing Argon2id parameters", "params", parts[3], "error", err)
		return false
	}

	salt, err := decodeBase64Flexible(parts[4])
	if err != nil || len(salt) == 0 {
		slog.Error("Failed decoding Argon2id salt", "error", err)
		return false
	}

	expectedKey, err := decodeBase64Flexible(parts[5])
	if err != nil || len(expectedKey) == 0 {
		slog.Error("Failed decoding Argon2id expected key", "error", err)
		return false
	}

	derivedKey := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(expectedKey)))

	return subtle.ConstantTimeCompare(derivedKey, expectedKey) == 1
}

func decodeBase64Flexible(s string) ([]byte, error) {
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

// NeedsRehash determines whether a stored password hash was computed with an older algorithm
// or lower cost/memory factor than currently configured, allowing transparent on-login upgrades.
func NeedsRehash(hash string) bool {
	if len(hash) == 0 {
		return false
	}

	activeAlgo := GetHashAlgo()

	// If system is configured to Argon2id:
	if activeAlgo == AlgoArgon2id {
		// Legacy bcrypt hashes MUST be upgraded to Argon2id
		if strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$") {
			return true
		}

		if strings.HasPrefix(hash, "$argon2id$") {
			parts := strings.Split(hash, "$")
			if len(parts) != 6 {
				return false
			}
			var memory, time uint32
			var threads uint8
			if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
				return false
			}
			cur := GetArgon2Params()
			// If stored parameters are weaker than current runtime configuration, upgrade
			return memory < cur.Memory || time < cur.Iterations || threads < cur.Parallelism
		}
		return false
	}

	// If system is configured to Bcrypt:
	if strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$") {
		cost, err := bcrypt.Cost([]byte(hash))
		if err != nil {
			return false
		}
		return cost < GetBcryptCost()
	}

	return false
}

// DummyVerify performs an authentic password verification against a deterministic dummy hash.
// Used when an account is not found during authentication to maintain strict constant-time
// latency parity, completely neutralizing timing side-channel attacks for account enumeration (CWE-208).
func DummyVerify(password string) bool {
	if GetHashAlgo() == AlgoBcrypt {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(password))
		return false
	}

	params := GetArgon2Params()
	derived := argon2.IDKey([]byte(password), dummyArgon2Salt, params.Iterations, params.Memory, params.Parallelism, 32)
	_ = subtle.ConstantTimeCompare(derived, dummyArgon2Key)
	return false
}

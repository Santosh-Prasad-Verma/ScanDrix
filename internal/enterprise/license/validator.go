package license

import (
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// LicenseManager verifies and enforces cryptographically signed enterprise licenses.
//
// A manager holds one primary verification key plus an optional ring of
// additional keys addressed by KeyID. That ring is what makes a signing-key
// rotation possible without invalidating licenses that are still in the
// field: new licenses are minted naming the new key, while licenses signed by
// the previous key continue to verify until they expire.
type LicenseManager struct {
	mu            sync.RWMutex
	pubKey        ed25519.PublicKey
	keys          map[string]ed25519.PublicKey // KeyID -> verification key
	activeLicense *LicensePayload
	expectedFP    string // hardware fingerprint this server requires; "" = no binding
}

// NewLicenseManager initializes the license manager with the vendor verification public key.
func NewLicenseManager(pubKey ed25519.PublicKey) *LicenseManager {
	return &LicenseManager{
		pubKey: pubKey,
		keys:   make(map[string]ed25519.PublicKey),
	}
}

// AddVerificationKey registers an additional verification key under a key ID,
// forming part of the rotation ring. An empty key ID is rejected because it is
// reserved to mean "the primary key", and a wrong-sized key is rejected rather
// than registered, so a misconfigured rotation fails loudly at boot instead of
// silently failing to verify later.
func (m *LicenseManager) AddVerificationKey(keyID string, pubKey ed25519.PublicKey) error {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return fmt.Errorf("verification key ID must not be empty")
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return fmt.Errorf("verification key %q is %d bytes, expected %d", keyID, len(pubKey), ed25519.PublicKeySize)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.keys == nil {
		m.keys = make(map[string]ed25519.PublicKey)
	}
	m.keys[keyID] = pubKey
	return nil
}

// VerificationKeyIDs returns the registered rotation-ring key IDs, sorted, for
// diagnostics. The primary key is not part of the ring and is not listed.
func (m *LicenseManager) VerificationKeyIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.keys))
	for id := range m.keys {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// SetExpectedFingerprint binds this server to a hardware or cluster identity.
// When set, LoadLicense rejects licenses whose HardwareFingerprint differs.
// When empty (the default) no binding is enforced, so unbound licenses and
// deployments without fingerprinting both keep working.
func (m *LicenseManager) SetExpectedFingerprint(fingerprint string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expectedFP = strings.TrimSpace(fingerprint)
}

// verificationMaterial resolves the key a payload must be verified with, plus
// the fingerprint this server requires. Callers must hold at least a read lock.
//
// Selecting a key by an unverified KeyID is safe: the worst an attacker can do
// is name a key that is not registered, which fails the lookup and then the
// signature check. No payload field other than KeyID influences the choice, and
// KeyID is never trusted for anything except key selection.
func (m *LicenseManager) verificationMaterial(keyID string) (ed25519.PublicKey, string) {
	if keyID != "" {
		return m.keys[keyID], m.expectedFP
	}
	return m.pubKey, m.expectedFP
}

// IssueLicense generates a cryptographically signed license token using the vendor private key.
func IssueLicense(payload LicensePayload, privKey ed25519.PrivateKey) (string, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nilStr(), fmt.Errorf("failed serializing license payload: %w", err)
	}

	sig := ed25519.Sign(privKey, payloadBytes)

	token := SignedLicenseToken{
		Payload:   base64.StdEncoding.EncodeToString(payloadBytes),
		Signature: base64.StdEncoding.EncodeToString(sig),
	}

	tokenBytes, err := json.Marshal(token)
	if err != nil {
		return nilStr(), fmt.Errorf("failed serializing license token: %w", err)
	}

	return base64.StdEncoding.EncodeToString(tokenBytes), nil
}

func nilStr() string {
	return ""
}

// LoadLicense decodes, cryptographically authenticates, and activates a license token.
func (m *LicenseManager) LoadLicense(tokenStr string) (*LicensePayload, error) {
	tokenBytes, err := base64.StdEncoding.DecodeString(tokenStr)
	if err != nil {
		return nil, fmt.Errorf("invalid license encoding: %w", err)
	}

	var token SignedLicenseToken
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return nil, fmt.Errorf("failed parsing license envelope: %w", err)
	}

	payloadBytes, err := base64.StdEncoding.DecodeString(token.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed decoding license payload: %w", err)
	}

	sigBytes, err := base64.StdEncoding.DecodeString(token.Signature)
	if err != nil {
		return nil, fmt.Errorf("failed decoding license signature: %w", err)
	}

	var payload LicensePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("failed unmarshaling license payload: %w", err)
	}

	// Resolve the verification key from the payload's KeyID, then verify the
	// signature before trusting any other payload field. The lock is released
	// before the (comparatively slow) signature check so a burst of concurrent
	// verifications never serializes behind one another.
	m.mu.RLock()
	verifyKey, expectedFP := m.verificationMaterial(payload.KeyID)
	m.mu.RUnlock()

	if len(verifyKey) != ed25519.PublicKeySize {
		if payload.KeyID != "" {
			return nil, fmt.Errorf("no verification key registered for license key ID %q", payload.KeyID)
		}
		return nil, fmt.Errorf("no license verification key is configured on this server")
	}

	// Cryptographic Ed25519 signature validation.
	if !ed25519.Verify(verifyKey, payloadBytes, sigBytes) {
		return nil, fmt.Errorf("tampered or invalid license: digital signature verification failed")
	}

	// Hardware binding, enforced only when this server declares the identity it
	// expects. Compared in constant time so a mismatch cannot be probed by
	// timing, and only after the signature is proven authentic.
	if expectedFP != "" && subtle.ConstantTimeCompare([]byte(payload.HardwareFingerprint), []byte(expectedFP)) != 1 {
		return nil, fmt.Errorf("license is bound to a different hardware fingerprint than this server expects")
	}

	// Expiry validation with the shared grace period.
	if time.Now().UTC().After(payload.ExpiresAt.Add(LicenseGracePeriod)) {
		return nil, fmt.Errorf("license expired on %s (grace period of %s exceeded)",
			payload.ExpiresAt.UTC().Format(time.RFC3339), LicenseGracePeriod)
	}

	m.mu.Lock()
	m.activeLicense = &payload
	m.mu.Unlock()

	return &payload, nil
}

// HasFeature returns whether the specified capability is entitled.
func (m *LicenseManager) HasFeature(flag FeatureFlag) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.activeLicense == nil {
		return false
	}

	// Enterprise tier unlocks all features unconditionally
	if m.activeLicense.Tier == TierEnterprise {
		return true
	}

	for _, feat := range m.activeLicense.Features {
		if feat == string(flag) {
			return true
		}
	}
	return false
}

// AssertFeature enforces that a feature is permitted, returning an error if gated.
func (m *LicenseManager) AssertFeature(flag FeatureFlag) error {
	if !m.HasFeature(flag) {
		return fmt.Errorf("feature '%s' is not entitled under current %s license", flag, m.GetTier())
	}
	return nil
}

// CheckSeatLimit validates active users against license quota.
func (m *LicenseManager) CheckSeatLimit(activeSeats int) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.activeLicense == nil {
		if activeSeats > 5 {
			return fmt.Errorf("unlicensed community seat limit (5) exceeded: %d", activeSeats)
		}
		return nil
	}

	if m.activeLicense.MaxSeats > 0 && activeSeats > m.activeLicense.MaxSeats {
		return fmt.Errorf("license seat quota (%d) exceeded: %d active seats", m.activeLicense.MaxSeats, activeSeats)
	}
	return nil
}

// CheckRepoLimit validates active repositories against license quota.
func (m *LicenseManager) CheckRepoLimit(activeRepos int) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.activeLicense == nil {
		if activeRepos > 3 {
			return fmt.Errorf("unlicensed community repo limit (3) exceeded: %d", activeRepos)
		}
		return nil
	}

	if m.activeLicense.MaxRepositories > 0 && activeRepos > m.activeLicense.MaxRepositories {
		return fmt.Errorf("license repository quota (%d) exceeded: %d active repos", m.activeLicense.MaxRepositories, activeRepos)
	}
	return nil
}

// GetTier returns the active plan tier.
func (m *LicenseManager) GetTier() LicenseTier {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.activeLicense == nil {
		return TierCommunity
	}
	return m.activeLicense.Tier
}

// GetActiveLicense returns the current license payload.
func (m *LicenseManager) GetActiveLicense() *LicensePayload {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.activeLicense == nil {
		return nil
	}
	cp := *m.activeLicense
	return &cp
}

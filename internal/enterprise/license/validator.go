package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// LicenseManager verifies and enforces cryptographically signed enterprise licenses.
type LicenseManager struct {
	mu            sync.RWMutex
	pubKey        ed25519.PublicKey
	activeLicense *LicensePayload
}

// NewLicenseManager initializes the license manager with the vendor verification public key.
func NewLicenseManager(pubKey ed25519.PublicKey) *LicenseManager {
	return &LicenseManager{
		pubKey: pubKey,
	}
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

	// Cryptographic Ed25519 signature validation
	if !ed25519.Verify(m.pubKey, payloadBytes, sigBytes) {
		return nil, fmt.Errorf("tampered or invalid license: digital signature verification failed")
	}

	var payload LicensePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("failed unmarshaling license payload: %w", err)
	}

	// Expiry validation with 72-hour grace period
	now := time.Now().UTC()
	if now.After(payload.ExpiresAt.Add(72 * time.Hour)) {
		return nil, fmt.Errorf("license expired on %s (grace period exceeded)", payload.ExpiresAt.Format(time.RFC3339))
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

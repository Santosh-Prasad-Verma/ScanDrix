// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package sso

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultTestSessionTTL is the maximum lifetime for an active SSO connection test (15 minutes).
	DefaultTestSessionTTL = 15 * time.Minute
)

// TestSessionStatus represents the lifecycle state of an SSO connection test.
type TestSessionStatus string

const (
	TestSessionStatusPending TestSessionStatus = "PENDING"
	TestSessionStatusSuccess TestSessionStatus = "SUCCESS"
	TestSessionStatusFailed  TestSessionStatus = "FAILED"
)

// Standard diagnostic error codes for SSO testing.
const (
	ErrCodeInvalidAssertion      = "INVALID_ASSERTION"
	ErrCodeSignatureFailed        = "SIGNATURE_VERIFICATION_FAILED"
	ErrCodeMissingEmailAttribute  = "MISSING_EMAIL_ATTRIBUTE"
	ErrCodeExpiredAssertion       = "EXPIRED_ASSERTION"
	ErrCodeAudienceMismatch       = "AUDIENCE_MISMATCH"
	ErrCodeDomainMismatch         = "DOMAIN_MISMATCH"
	ErrCodeSessionExpired         = "TEST_SESSION_EXPIRED"
	ErrCodeIdPConnectionTimeout   = "IDP_CONNECTION_TIMEOUT"
	ErrCodeInvalidOIDCIDToken     = "INVALID_OIDC_ID_TOKEN"
)

var (
	ErrTestSessionNotFound = errors.New("SSO test session not found or expired")
)

// ExtractedIdentityAttributes represents the identity claims received from the test assertion.
type ExtractedIdentityAttributes struct {
	Subject     string            `json:"subject"`
	Email       string            `json:"email"`
	DisplayName string            `json:"display_name,omitempty"`
	Issuer      string            `json:"issuer,omitempty"`
	Groups      []string          `json:"groups,omitempty"`
	RawClaims   map[string]string `json:"raw_claims,omitempty"`
}

// SSOTestSession stores the state and diagnostics of an isolated connection test.
type SSOTestSession struct {
	SessionID         string                       `json:"session_id"`
	WorkspaceID       uuid.UUID                    `json:"workspace_id"`
	ProviderType      SSOProviderType              `json:"provider_type"`
	Status            TestSessionStatus            `json:"status"`
	ConfigFingerprint string                       `json:"config_fingerprint"`
	Domains           []string                     `json:"domains"`
	CreatedBy         string                       `json:"created_by,omitempty"`
	CreatedAt         time.Time                    `json:"created_at"`
	ExpiresAt         time.Time                    `json:"expires_at"`
	TestedAt          *time.Time                   `json:"tested_at,omitempty"`
	Attributes        *ExtractedIdentityAttributes `json:"attributes,omitempty"`
	FailureCode       string                       `json:"failure_code,omitempty"`
	FailureMessage    string                       `json:"failure_message,omitempty"`
}

// SSOTestSessionWorkbench provides an isolated testing sandbox for validating IdP connections.
type SSOTestSessionWorkbench struct {
	mu          sync.RWMutex
	sessions    map[string]*SSOTestSession // sessionID -> session
	samlHandler *SAMLHandler
	oidcHandler *OIDCHandler
	ttl         time.Duration
}

// NewSSOTestSessionWorkbench initializes the SSO test session manager.
func NewSSOTestSessionWorkbench(saml *SAMLHandler, oidc *OIDCHandler) *SSOTestSessionWorkbench {
	return &SSOTestSessionWorkbench{
		sessions:    make(map[string]*SSOTestSession),
		samlHandler: saml,
		oidcHandler: oidc,
		ttl:         DefaultTestSessionTTL,
	}
}

// BuildConfigFingerprint generates a reproducible SHA-256 fingerprint of the SSO config under test.
func BuildConfigFingerprint(providerType SSOProviderType, endpoint string, domains []string) string {
	h := sha256.New()
	h.Write([]byte(string(providerType)))
	h.Write([]byte(strings.ToLower(strings.TrimSpace(endpoint))))
	for _, d := range domains {
		h.Write([]byte(NormalizeDomain(d)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// CreateSession initiates a new isolated test session.
func (w *SSOTestSessionWorkbench) CreateSession(
	_ context.Context,
	wsID uuid.UUID,
	providerType SSOProviderType,
	endpoint string,
	domains []string,
	createdBy string,
) (*SSOTestSession, error) {
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed generating test session id: %w", err)
	}
	sessionID := hex.EncodeToString(tokenBytes)

	now := time.Now().UTC()
	normalizedDomains := make([]string, len(domains))
	for i, d := range domains {
		normalizedDomains[i] = NormalizeDomain(d)
	}

	session := &SSOTestSession{
		SessionID:         sessionID,
		WorkspaceID:       wsID,
		ProviderType:      providerType,
		Status:            TestSessionStatusPending,
		ConfigFingerprint: BuildConfigFingerprint(providerType, endpoint, normalizedDomains),
		Domains:           normalizedDomains,
		CreatedBy:         createdBy,
		CreatedAt:         now,
		ExpiresAt:         now.Add(w.ttl),
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.sessions[sessionID] = session

	return session, nil
}

// GetSession retrieves an active test session, returning ErrTestSessionNotFound if missing or expired.
func (w *SSOTestSessionWorkbench) GetSession(_ context.Context, sessionID string) (*SSOTestSession, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	session, exists := w.sessions[sessionID]
	if !exists || session == nil {
		return nil, ErrTestSessionNotFound
	}

	if time.Now().UTC().After(session.ExpiresAt) {
		return nil, ErrTestSessionNotFound
	}

	return session, nil
}

// ValidateSAMLAssertion evaluates an incoming SAML response within the test sandbox.
func (w *SSOTestSessionWorkbench) ValidateSAMLAssertion(
	_ context.Context,
	sessionID string,
	samlXML []byte,
	expectedAudience string,
) (*SSOTestSession, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	session, exists := w.sessions[sessionID]
	if !exists || session == nil || time.Now().UTC().After(session.ExpiresAt) {
		return nil, ErrTestSessionNotFound
	}

	now := time.Now().UTC()
	session.TestedAt = &now

	if w.samlHandler == nil {
		session.Status = TestSessionStatusFailed
		session.FailureCode = ErrCodeInvalidAssertion
		session.FailureMessage = "SAML handler is not configured on the test workbench"
		return session, nil
	}

	var identity *FederatedIdentity
	var err error
	if w.samlHandler.HasIdPCertificate() {
		identity, err = w.samlHandler.ParseAndVerifyAssertion(samlXML, expectedAudience, now)
	} else {
		identity, err = w.samlHandler.ParseAssertionWithoutSignature(samlXML, expectedAudience, now)
	}

	if err != nil {
		session.Status = TestSessionStatusFailed
		errStr := strings.ToLower(err.Error())
		switch {
		case strings.Contains(errStr, "malformed") || strings.Contains(errStr, "missing nameid"):
			session.FailureCode = ErrCodeInvalidAssertion
		case strings.Contains(errStr, "expired") || strings.Contains(errStr, "not yet valid"):
			session.FailureCode = ErrCodeExpiredAssertion
		case strings.Contains(errStr, "audience mismatch"):
			session.FailureCode = ErrCodeAudienceMismatch
		default:
			session.FailureCode = ErrCodeSignatureFailed
		}
		session.FailureMessage = fmt.Sprintf("SAML verification failed: %s", err.Error())
		return session, nil
	}

	if identity == nil || identity.Email == "" || !strings.Contains(identity.Email, "@") {
		session.Status = TestSessionStatusFailed
		session.FailureCode = ErrCodeMissingEmailAttribute
		session.FailureMessage = "IdP SAML assertion did not contain a valid email address attribute (checked email, emailAddress, NameID)"
		return session, nil
	}

	// Validate allowed domain boundaries if configured
	if len(session.Domains) > 0 {
		userDomain := ""
		if atIdx := strings.LastIndex(identity.Email, "@"); atIdx != -1 {
			userDomain = NormalizeDomain(identity.Email[atIdx+1:])
		}
		domainMatched := false
		for _, d := range session.Domains {
			normD := NormalizeDomain(d)
			if userDomain == normD || strings.HasSuffix(userDomain, "."+normD) {
				domainMatched = true
				break
			}
		}
		if !domainMatched {
			session.Status = TestSessionStatusFailed
			session.FailureCode = ErrCodeDomainMismatch
			session.FailureMessage = fmt.Sprintf("User identity domain %q does not match configured test domains (%s)", userDomain, strings.Join(session.Domains, ", "))
			return session, nil
		}
	}

	displayName := strings.TrimSpace(identity.FirstName + " " + identity.LastName)
	if displayName == "" {
		displayName = identity.Email
	}

	session.Status = TestSessionStatusSuccess
	session.FailureCode = ""
	session.FailureMessage = ""
	session.Attributes = &ExtractedIdentityAttributes{
		Subject:     identity.ExternalID,
		Email:       identity.Email,
		DisplayName: displayName,
		Issuer:      string(identity.Provider),
		Groups:      identity.Groups,
		RawClaims:   identity.RawClaims,
	}

	return session, nil
}

// MarkFailed manually registers a test failure with diagnostics.
func (w *SSOTestSessionWorkbench) MarkFailed(sessionID string, code, message string) (*SSOTestSession, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	session, exists := w.sessions[sessionID]
	if !exists || session == nil || time.Now().UTC().After(session.ExpiresAt) {
		return nil, ErrTestSessionNotFound
	}

	now := time.Now().UTC()
	session.TestedAt = &now
	session.Status = TestSessionStatusFailed
	session.FailureCode = code
	session.FailureMessage = message

	return session, nil
}

// CleanupExpired purges test sessions that have exceeded their TTL.
func (w *SSOTestSessionWorkbench) CleanupExpired() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now().UTC()
	purged := 0
	for id, s := range w.sessions {
		if now.After(s.ExpiresAt) {
			delete(w.sessions, id)
			purged++
		}
	}
	return purged
}

// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package sso

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultDomainTokenTTL defines the lifespan for a pending domain verification token (24 hours).
	DefaultDomainTokenTTL = 24 * time.Hour
	// DefaultDomainVerifiedTTL defines how long a verified domain status remains active (30 days).
	DefaultDomainVerifiedTTL = 30 * 24 * time.Hour

	// TXTRecordPrefix specifies the DNS TXT record prefix expected during live DNS challenges.
	TXTRecordPrefix = "scandrix-domain-verification="
)

var (
	ErrInvalidDomain          = errors.New("invalid or empty domain name")
	ErrInvalidContactEmail    = errors.New("invalid contact email address")
	ErrDomainMismatch         = errors.New("contact email domain must match the domain being verified")
	ErrTokenNotFoundOrExpired = errors.New("domain verification token not found or expired")
	ErrDNSTXTRecordNotFound   = errors.New("matching DNS TXT record not found on domain")
)

// DNSResolver defines the contract for looking up TXT records from authoritative nameservers.
type DNSResolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// DefaultDNSResolver wraps net.DefaultResolver for production DNS lookups.
type DefaultDNSResolver struct {
	resolver *net.Resolver
}

func NewDefaultDNSResolver() *DefaultDNSResolver {
	return &DefaultDNSResolver{
		resolver: net.DefaultResolver,
	}
}

func (r *DefaultDNSResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return r.resolver.LookupTXT(ctx, name)
}

// DomainVerificationRecord stores domain verification metadata and state.
type DomainVerificationRecord struct {
	Domain             string     `json:"domain"`
	WorkspaceID        uuid.UUID  `json:"workspace_id"`
	Verified           bool       `json:"verified"`
	VerifiedByEmail    string     `json:"verified_by_email,omitempty"`
	VerifiedAt         *time.Time `json:"verified_at,omitempty"`
	Token              string     `json:"token,omitempty"`
	TXTRecordExpected  string     `json:"txt_record_expected"`
	TXTRecordHost      string     `json:"txt_record_host"`
	ExpiresAt          time.Time  `json:"expires_at"`
	IsSelfHostedBypass bool       `json:"is_self_hosted_bypass,omitempty"`
}

// DomainVerifierService manages enterprise SSO domain ownership challenges.
type DomainVerifierService struct {
	mu           sync.RWMutex
	dnsResolver  DNSResolver
	records      map[string]*DomainVerificationRecord // "wsID:domain" -> record
	tokenIndex   map[string]string                    // token -> "wsID:domain"
	cloudMode    bool
	tokenTTL     time.Duration
	verifiedTTL  time.Duration
}

// NewDomainVerifierService initializes the domain verifier service.
func NewDomainVerifierService(resolver DNSResolver, cloudMode bool) *DomainVerifierService {
	if resolver == nil {
		resolver = NewDefaultDNSResolver()
	}
	return &DomainVerifierService{
		dnsResolver: resolver,
		records:     make(map[string]*DomainVerificationRecord),
		tokenIndex:  make(map[string]string),
		cloudMode:   cloudMode,
		tokenTTL:    DefaultDomainTokenTTL,
		verifiedTTL: DefaultDomainVerifiedTTL,
	}
}

// NormalizeDomain cleans up raw domains, stripping protocols, ports, and whitespace.
func NormalizeDomain(raw string) string {
	d := strings.TrimSpace(strings.ToLower(raw))
	// Strip null bytes and control characters
	if idx := strings.IndexByte(d, 0); idx != -1 {
		d = d[:idx]
	}
	if strings.Contains(d, "://") {
		if u, err := url.Parse(d); err == nil && u.Host != "" {
			d = u.Host
		}
	}
	// Strip port if present
	if host, _, err := net.SplitHostPort(d); err == nil {
		d = host
	}
	// Strip paths if user typed domain.com/something
	if idx := strings.Index(d, "/"); idx != -1 {
		d = d[:idx]
	}
	// Strip trailing dots
	return strings.TrimRight(d, ".")
}

func (s *DomainVerifierService) recordKey(wsID uuid.UUID, domain string) string {
	return fmt.Sprintf("%s:%s", wsID.String(), domain)
}

// RequestVerification initiates a new verification challenge for a domain.
func (s *DomainVerifierService) RequestVerification(
	ctx context.Context,
	wsID uuid.UUID,
	rawDomain string,
	contactEmail string,
) (*DomainVerificationRecord, error) {
	domain := NormalizeDomain(rawDomain)
	if domain == "" || (!strings.Contains(domain, ".") && domain != "localhost") {
		return nil, ErrInvalidDomain
	}

	email := strings.TrimSpace(strings.ToLower(contactEmail))
	if email == "" || !strings.Contains(email, "@") {
		return nil, ErrInvalidContactEmail
	}

	// In cloud mode, require contact email to match the domain
	if s.cloudMode {
		emailDomain := email[strings.LastIndex(email, "@")+1:]
		if emailDomain != domain && !strings.HasSuffix(emailDomain, "."+domain) {
			return nil, ErrDomainMismatch
		}
	}

	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed generating verification token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	now := time.Now().UTC()
	record := &DomainVerificationRecord{
		Domain:             domain,
		WorkspaceID:        wsID,
		Verified:           false,
		VerifiedByEmail:    email,
		Token:              token,
		TXTRecordExpected:  TXTRecordPrefix + token,
		TXTRecordHost:      fmt.Sprintf("_scandrix-challenge.%s", domain),
		ExpiresAt:          now.Add(s.tokenTTL),
		IsSelfHostedBypass: false,
	}

	// In self-hosted mode without cloud enforcement, allow instant auto-verification
	if !s.cloudMode {
		record.Verified = true
		record.VerifiedAt = &now
		record.IsSelfHostedBypass = true
		record.ExpiresAt = now.Add(s.verifiedTTL)
	}

	key := s.recordKey(wsID, domain)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.records[key] = record
	if !record.Verified {
		s.tokenIndex[token] = key
	}

	return record, nil
}

// VerifyDNS attempts to verify domain ownership by checking DNS TXT records.
func (s *DomainVerifierService) VerifyDNS(
	ctx context.Context,
	wsID uuid.UUID,
	rawDomain string,
) (*DomainVerificationRecord, error) {
	domain := NormalizeDomain(rawDomain)
	key := s.recordKey(wsID, domain)

	s.mu.RLock()
	record, exists := s.records[key]
	s.mu.RUnlock()

	if !exists || record == nil {
		return nil, ErrTokenNotFoundOrExpired
	}

	if record.Verified {
		return record, nil
	}

	if time.Now().UTC().After(record.ExpiresAt) {
		return nil, ErrTokenNotFoundOrExpired
	}

	// Check both `_scandrix-challenge.<domain>` and `<domain>` root
	hostsToProbe := []string{
		record.TXTRecordHost,
		domain,
	}

	expectedVal := record.TXTRecordExpected
	verified := false

	for _, host := range hostsToProbe {
		records, err := s.dnsResolver.LookupTXT(ctx, host)
		if err != nil {
			continue
		}
		for _, txt := range records {
			if strings.TrimSpace(txt) == expectedVal {
				verified = true
				break
			}
		}
		if verified {
			break
		}
	}

	if !verified {
		return nil, ErrDNSTXTRecordNotFound
	}

	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	record.Verified = true
	record.VerifiedAt = &now
	record.ExpiresAt = now.Add(s.verifiedTTL)
	delete(s.tokenIndex, record.Token)

	return record, nil
}

// ConfirmToken verifies domain ownership via direct token presentation.
func (s *DomainVerifierService) ConfirmToken(
	_ context.Context,
	token string,
) (*DomainVerificationRecord, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrTokenNotFoundOrExpired
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key, exists := s.tokenIndex[token]
	if !exists {
		return nil, ErrTokenNotFoundOrExpired
	}

	record, found := s.records[key]
	if !found || record == nil || time.Now().UTC().After(record.ExpiresAt) {
		delete(s.tokenIndex, token)
		return nil, ErrTokenNotFoundOrExpired
	}

	now := time.Now().UTC()
	record.Verified = true
	record.VerifiedAt = &now
	record.ExpiresAt = now.Add(s.verifiedTTL)
	delete(s.tokenIndex, token)

	return record, nil
}

// GetDomainStatus returns the verification record for a given workspace and domain.
func (s *DomainVerifierService) GetDomainStatus(
	_ context.Context,
	wsID uuid.UUID,
	rawDomain string,
) (*DomainVerificationRecord, error) {
	domain := NormalizeDomain(rawDomain)
	key := s.recordKey(wsID, domain)

	s.mu.RLock()
	defer s.mu.RUnlock()

	record, exists := s.records[key]
	if !exists || record == nil {
		return nil, nil
	}

	if record.Verified && time.Now().UTC().After(record.ExpiresAt) {
		return nil, nil // expired verified state
	}

	return record, nil
}

// IsDomainVerified returns true if the domain is actively verified for the workspace.
func (s *DomainVerifierService) IsDomainVerified(
	ctx context.Context,
	wsID uuid.UUID,
	rawDomain string,
) bool {
	record, err := s.GetDomainStatus(ctx, wsID, rawDomain)
	if err != nil || record == nil {
		return false
	}
	return record.Verified
}

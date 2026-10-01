// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package sso_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/sso"
)

type mockDNSResolver struct {
	mu      sync.RWMutex
	records map[string][]string
}

func newMockDNSResolver() *mockDNSResolver {
	return &mockDNSResolver{records: make(map[string][]string)}
}

func (m *mockDNSResolver) setTXT(host string, txts []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records[host] = txts
}

func (m *mockDNSResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if txts, ok := m.records[name]; ok {
		return txts, nil
	}
	return nil, errors.New("host not found")
}

func TestDomainNormalization(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com"},
		{"EXAMPLE.COM", "example.com"},
		{"https://company.com/", "company.com"},
		{"http://sub.company.com:8080/path?q=1", "sub.company.com"},
		{"corp.internal.", "corp.internal"},
		{"  auth.domain.io  ", "auth.domain.io"},
	}

	for _, c := range cases {
		got := sso.NormalizeDomain(c.input)
		if got != c.expected {
			t.Errorf("NormalizeDomain(%q) = %q; want %q", c.input, got, c.expected)
		}
	}
}

func TestDomainVerification_CloudModeDNS(t *testing.T) {
	resolver := newMockDNSResolver()
	service := sso.NewDomainVerifierService(resolver, true) // cloud mode enabled
	wsID := uuid.New()
	ctx := context.Background()

	// 1. Invalid domain rejection
	if _, err := service.RequestVerification(ctx, wsID, "invalid", "admin@invalid.com"); err == nil {
		t.Fatal("Expected error for domain without dot")
	}

	// 2. Contact email mismatch rejection
	if _, err := service.RequestVerification(ctx, wsID, "acme.corp", "admin@other.com"); !errors.Is(err, sso.ErrDomainMismatch) {
		t.Fatalf("Expected ErrDomainMismatch, got %v", err)
	}

	// 3. Valid verification request
	req, err := service.RequestVerification(ctx, wsID, "acme.corp", "security@acme.corp")
	if err != nil {
		t.Fatalf("RequestVerification failed: %v", err)
	}
	if req.Verified {
		t.Fatal("Cloud mode should not auto-verify domain")
	}
	if !strings.HasPrefix(req.TXTRecordExpected, "scandrix-domain-verification=") {
		t.Fatalf("Unexpected TXT record format: %s", req.TXTRecordExpected)
	}

	// 4. Verify before publishing DNS should fail
	if _, err := service.VerifyDNS(ctx, wsID, "acme.corp"); !errors.Is(err, sso.ErrDNSTXTRecordNotFound) {
		t.Fatalf("Expected ErrDNSTXTRecordNotFound, got %v", err)
	}

	// 5. Publish correct TXT record in mock DNS
	resolver.setTXT(req.TXTRecordHost, []string{req.TXTRecordExpected})

	// 6. Verify after publishing DNS should succeed
	verifiedRec, err := service.VerifyDNS(ctx, wsID, "acme.corp")
	if err != nil {
		t.Fatalf("VerifyDNS failed after DNS publish: %v", err)
	}
	if !verifiedRec.Verified || verifiedRec.VerifiedAt == nil {
		t.Fatal("Domain should be marked verified with timestamp")
	}

	// 7. Check helper status method
	if !service.IsDomainVerified(ctx, wsID, "acme.corp") {
		t.Fatal("IsDomainVerified should return true")
	}
}

// TestDomainVerification_RequiresProofOfOwnership pins AUDIT_REMEDIATION.md F-11.
//
// The service previously auto-approved every domain when cloudMode was false,
// which is how it was constructed in production, so no domain ever required
// proof. Ownership must now come from a DNS TXT challenge or a confirmed token.
func TestDomainVerification_RequiresProofOfOwnership(t *testing.T) {
	for _, cloudMode := range []bool{false, true} {
		t.Run(fmt.Sprintf("cloudMode=%v", cloudMode), func(t *testing.T) {
			resolver := newMockDNSResolver()
			service := sso.NewDomainVerifierService(resolver, cloudMode)
			wsID := uuid.New()
			ctx := context.Background()

			rec, err := service.RequestVerification(ctx, wsID, "selfhosted.org", "it@selfhosted.org")
			if err != nil {
				t.Fatalf("RequestVerification failed: %v", err)
			}

			if rec.Verified {
				t.Fatalf("a fresh challenge must not be pre-verified: %+v", rec)
			}
			if rec.Token == "" {
				t.Fatalf("a challenge must carry a token to prove ownership")
			}
			if rec.TXTRecordExpected == "" {
				t.Fatalf("a challenge must state the expected TXT record")
			}
			if service.IsDomainVerified(ctx, wsID, "selfhosted.org") {
				t.Fatalf("IsDomainVerified must be false before the challenge is satisfied")
			}

			// Once the DNS challenge is satisfied, the domain is verified.
			resolver.setTXT(rec.TXTRecordHost, []string{rec.TXTRecordExpected})
			if _, err := service.VerifyDNS(ctx, wsID, "selfhosted.org"); err != nil {
				t.Fatalf("VerifyDNS failed: %v", err)
			}
			if !service.IsDomainVerified(ctx, wsID, "selfhosted.org") {
				t.Fatalf("the domain must verify once the TXT record matches")
			}
		})
	}
}

// TestDomainVerification_ContactEmailMustMatchDomainInCloudMode confirms the
// cloud-mode tightening is retained after the bypass was removed.
func TestDomainVerification_ContactEmailMustMatchDomainInCloudMode(t *testing.T) {
	resolver := newMockDNSResolver()
	cloud := sso.NewDomainVerifierService(resolver, true)
	wsID := uuid.New()
	ctx := context.Background()

	if _, err := cloud.RequestVerification(ctx, wsID, "corp.com", "attacker@evil.test"); err == nil {
		t.Fatalf("cloud mode must reject a contact email outside the domain being verified")
	}
}

func TestDomainVerification_DirectTokenConfirmation(t *testing.T) {
	resolver := newMockDNSResolver()
	service := sso.NewDomainVerifierService(resolver, true)
	wsID := uuid.New()
	ctx := context.Background()

	rec, err := service.RequestVerification(ctx, wsID, "startup.io", "ceo@startup.io")
	if err != nil {
		t.Fatalf("RequestVerification failed: %v", err)
	}

	// Confirm with token directly
	confirmed, err := service.ConfirmToken(ctx, rec.Token)
	if err != nil {
		t.Fatalf("ConfirmToken failed: %v", err)
	}
	if !confirmed.Verified {
		t.Fatal("Expected confirmed record to be verified")
	}

	// Re-confirming same token should fail (single-use)
	if _, err := service.ConfirmToken(ctx, rec.Token); !errors.Is(err, sso.ErrTokenNotFoundOrExpired) {
		t.Fatalf("Expected single-use token to fail on reuse, got %v", err)
	}
}

func TestDomainVerification_Concurrency(t *testing.T) {
	resolver := newMockDNSResolver()
	service := sso.NewDomainVerifierService(resolver, false)
	ctx := context.Background()

	var wg sync.WaitGroup
	workers := 50

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			ws := uuid.New()
			// The domain must be unique per worker. It used to be
			// "company-"+rune('a'+workerID%26), so with 50 workers several
			// shared a domain; each setTXT then overwrote the shared TXT
			// record and a peer failed to verify. That made this test flaky
			// and unrelated to the concurrency it claims to exercise.
			domain := fmt.Sprintf("company-%d.com", workerID)
			email := "user@" + domain

			// A fresh challenge must be unverified, and the service must stay
			// race-free under concurrent access (AUDIT_REMEDIATION.md F-11).
			rec, err := service.RequestVerification(ctx, ws, domain, email)
			if err != nil {
				t.Errorf("Worker %d failed: %v", workerID, err)
				return
			}
			if service.IsDomainVerified(ctx, ws, domain) {
				t.Errorf("Worker %d: domain must not be verified before the challenge is met", workerID)
			}

			// Satisfy the challenge; the per-workspace record must stay isolated
			// from every other worker's.
			resolver.setTXT(rec.TXTRecordHost, []string{rec.TXTRecordExpected})
			if _, err := service.VerifyDNS(ctx, ws, domain); err != nil {
				t.Errorf("Worker %d verify failed: %v", workerID, err)
				return
			}
			if !service.IsDomainVerified(ctx, ws, domain) {
				t.Errorf("Worker %d expected verified after the TXT record matched", workerID)
			}
		}(i)
	}

	wg.Wait()
}

// TestDomainVerification_QuotedAndSegmentedTXTRecords pins AUDIT_REMEDIATION.md
// F-11. The auto-approval bypass made VerifyDNS return before it performed a
// lookup, so these real-world DNS response shapes were never exercised. Now
// that ownership is genuinely checked, a correctly published record must verify
// whether or not the provider quotes it, and must not be fooled by unrelated
// records in the same answer.
func TestDomainVerification_QuotedAndSegmentedTXTRecords(t *testing.T) {
	tests := []struct {
		name  string
		build func(valid string) []string
	}{
		{"bare", func(v string) []string { return []string{v} }},
		{"quoted", func(v string) []string { return []string{`"` + v + `"`} }},
		{"padded", func(v string) []string { return []string{"  " + v + "  "} }},
		{"segmented", func(v string) []string { return []string{`"v=spf1 -all" "` + v + `"`} }},
		{"with noise", func(v string) []string {
			return []string{
				"v=spf1 include:_spf.google.com ~all",
				"scandrix-domain-verification=" + strings.Repeat("0", 48),
				v,
				"google-site-verification=abc123",
			}
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolver := newMockDNSResolver()
			svc := sso.NewDomainVerifierService(resolver, false)
			wsID := uuid.New()
			ctx := context.Background()

			rec, err := svc.RequestVerification(ctx, wsID, "quoted.example.com", "admin@quoted.example.com")
			if err != nil {
				t.Fatalf("RequestVerification failed: %v", err)
			}
			resolver.setTXT("quoted.example.com", tc.build(rec.TXTRecordExpected))

			if _, err := svc.VerifyDNS(ctx, wsID, "quoted.example.com"); err != nil {
				t.Fatalf("a correctly published record must verify: %v", err)
			}
			if !svc.IsDomainVerified(ctx, wsID, "quoted.example.com") {
				t.Fatal("domain should be verified after a matching TXT record")
			}
		})
	}
}

// TestDomainVerification_RejectsWrongToken guards the negative path: a domain
// whose TXT record carries somebody else's token must not verify.
func TestDomainVerification_RejectsWrongToken(t *testing.T) {
	resolver := newMockDNSResolver()
	svc := sso.NewDomainVerifierService(resolver, false)
	wsID := uuid.New()
	ctx := context.Background()

	if _, err := svc.RequestVerification(ctx, wsID, "target.example.com", "admin@target.example.com"); err != nil {
		t.Fatalf("RequestVerification failed: %v", err)
	}
	resolver.setTXT("target.example.com", []string{
		sso.TXTRecordPrefix + strings.Repeat("f", 48),
	})

	if _, err := svc.VerifyDNS(ctx, wsID, "target.example.com"); err == nil {
		t.Fatal("a TXT record carrying a different token must not verify the domain")
	}
	if svc.IsDomainVerified(ctx, wsID, "target.example.com") {
		t.Fatal("domain must remain unverified after a failed challenge")
	}
}

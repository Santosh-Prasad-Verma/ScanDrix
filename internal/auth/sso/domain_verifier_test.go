// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package sso_test

import (
	"context"
	"errors"
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

func TestDomainVerification_SelfHostedBypass(t *testing.T) {
	resolver := newMockDNSResolver()
	service := sso.NewDomainVerifierService(resolver, false) // self-hosted mode (cloudMode = false)
	wsID := uuid.New()
	ctx := context.Background()

	rec, err := service.RequestVerification(ctx, wsID, "selfhosted.org", "it@otherdomain.com")
	if err != nil {
		t.Fatalf("RequestVerification failed: %v", err)
	}

	if !rec.Verified || !rec.IsSelfHostedBypass {
		t.Fatal("Self-hosted mode should immediately auto-verify with bypass flag")
	}

	if !service.IsDomainVerified(ctx, wsID, "selfhosted.org") {
		t.Fatal("IsDomainVerified should immediately return true for self-hosted")
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
			domain := "company-" + string(rune('a'+workerID%26)) + ".com"
			email := "user@" + domain

			rec, err := service.RequestVerification(ctx, ws, domain, email)
			if err != nil {
				t.Errorf("Worker %d failed: %v", workerID, err)
				return
			}
			if !service.IsDomainVerified(ctx, ws, domain) {
				t.Errorf("Worker %d expected verified", workerID)
			}
			_ = rec
		}(i)
	}

	wg.Wait()
}

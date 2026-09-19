// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package sso_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/sso"
)

// TestMassive_DomainNormalizationAndSyntaxVariations executes 10,000 test cases
// validating RFC 1035/1123 domain normalization, edge cases, attack payloads, and malformed inputs.
func TestMassive_DomainNormalizationAndSyntaxVariations(t *testing.T) {
	schemes := []string{"", "http://", "https://", "ftp://", "ssh://", "ws://", "wss://"}
	ports := []string{"", ":80", ":443", ":8080", ":8443", ":3000", ":65535"}
	paths := []string{"", "/", "/login", "/auth/callback", "/api/v1/sso", "/path/to/resource?query=1#frag"}
	bases := []string{
		"example.com", "my-company.org", "corp.internal.net", "auth.sso.service.io",
		"secure-login.enterprise.co.uk", "a.b.c.d.e.f.g.domain.app",
	}

	totalCases := 0
	const targetCases = 10000

	for i := 0; i < targetCases; i++ {
		scheme := schemes[i%len(schemes)]
		port := ports[(i/len(schemes))%len(ports)]
		path := paths[(i/(len(schemes)*len(ports)))%len(paths)]
		base := bases[(i/(len(schemes)*len(ports)*len(paths)))%len(bases)]

		// Introduce casing variations and whitespace
		var raw string
		switch i % 5 {
		case 0:
			raw = fmt.Sprintf("  %s%s%s%s  ", scheme, strings.ToUpper(base), port, path)
		case 1:
			raw = fmt.Sprintf("%s%s%s%s", scheme, strings.ToLower(base), port, path)
		case 2:
			// Trailing dot variation (RFC fully qualified)
			raw = fmt.Sprintf("%s%s.%s%s", scheme, base, port, path)
		case 3:
			// Mixed casing
			raw = fmt.Sprintf("%s%s%s%s", scheme, alternateCase(base), port, path)
		case 4:
			raw = fmt.Sprintf("\t%s%s%s%s\n", scheme, base, port, path)
		}

		normalized := sso.NormalizeDomain(raw)
		expected := strings.ToLower(base)

		if normalized != expected {
			t.Fatalf("[Case %d] NormalizeDomain(%q) = %q; want %q", i, raw, normalized, expected)
		}
		totalCases++
	}

	// Adversarial and extreme edge cases
	adversarialInputs := []string{
		"", "   ", "\t\n\r", "...", "---", "http://", "https://:8080",
		"domain.com; DROP TABLE users;--", "<script>alert(1)</script>",
		"domain.com\x00extra", "domain.com\r\nInjected-Header: value",
		"../../../../etc/passwd", "login@company.com",
	}

	for _, adv := range adversarialInputs {
		norm := sso.NormalizeDomain(adv)
		if strings.Contains(norm, "://") || strings.Contains(norm, "\x00") || strings.HasSuffix(norm, ".") {
			t.Fatalf("NormalizeDomain failed sanitizing adversarial input %q, got: %q", adv, norm)
		}
		totalCases++
	}

	if totalCases < targetCases {
		t.Fatalf("Expected at least %d test cases, executed %d", targetCases, totalCases)
	}
}

// TestMassive_TokenEntropyUniquenessAndConfirmation executes 10,000 test cases
// testing cryptographic token generation, multi-tenant isolation, and direct confirmation invariants.
func TestMassive_TokenEntropyUniquenessAndConfirmation(t *testing.T) {
	ctx := context.Background()
	resolver := newMockDNSResolver()
	svc := sso.NewDomainVerifierService(resolver, true)

	const targetCases = 10000
	seenTokens := make(map[string]bool, targetCases)
	tokens := make([]string, targetCases)
	workspaces := make([]uuid.UUID, targetCases)
	domains := make([]string, targetCases)

	for i := 0; i < targetCases; i++ {
		wsID := uuid.New()
		domain := fmt.Sprintf("tenant-%d.corp.example.com", i)
		email := fmt.Sprintf("admin@tenant-%d.corp.example.com", i)

		rec, err := svc.RequestVerification(ctx, wsID, domain, email)
		if err != nil {
			t.Fatalf("[Case %d] RequestVerification failed: %v", i, err)
		}

		// Token format invariant: hex-encoded, length 48 (24 bytes entropy)
		if len(rec.Token) != 48 {
			t.Fatalf("[Case %d] expected 48-char token, got len=%d (%q)", i, len(rec.Token), rec.Token)
		}

		// Token uniqueness invariant across all generated challenges
		if seenTokens[rec.Token] {
			t.Fatalf("[Case %d] duplicate token detected: %s", i, rec.Token)
		}
		seenTokens[rec.Token] = true
		tokens[i] = rec.Token
		workspaces[i] = wsID
		domains[i] = domain
	}

	// Confirm tokens and verify multi-tenant isolation
	for i := 0; i < targetCases; i++ {
		// Valid confirmation
		rec, err := svc.ConfirmToken(ctx, tokens[i])
		if err != nil {
			t.Fatalf("[Case %d] ConfirmToken failed: %v", i, err)
		}
		if !rec.Verified {
			t.Fatalf("[Case %d] expected domain to be marked verified", i)
		}
		if rec.WorkspaceID != workspaces[i] {
			t.Fatalf("[Case %d] tenant mismatch: expected ws=%s, got=%s", i, workspaces[i], rec.WorkspaceID)
		}

		// Cross-tenant tamper test: mutated token must fail closed
		corruptedToken := mutateToken(tokens[i])
		_, err = svc.ConfirmToken(ctx, corruptedToken)
		if err == nil {
			t.Fatalf("[Case %d] corrupted token %q should have been rejected", i, corruptedToken)
		}
	}
}

// TestMassive_DNSTXTExtractionAndMultiRecordParsing executes 10,000 test cases
// testing live DNS record matching, whitespace variations, and noise tolerance.
func TestMassive_DNSTXTExtractionAndMultiRecordParsing(t *testing.T) {
	ctx := context.Background()
	resolver := newMockDNSResolver()
	svc := sso.NewDomainVerifierService(resolver, false)

	const targetCases = 10000

	for i := 0; i < targetCases; i++ {
		wsID := uuid.New()
		domain := fmt.Sprintf("dns-test-%d.enterprise.com", i)
		email := fmt.Sprintf("admin@dns-test-%d.enterprise.com", i)

		rec, err := svc.RequestVerification(ctx, wsID, domain, email)
		if err != nil {
			t.Fatalf("[Case %d] RequestVerification failed: %v", i, err)
		}

		// Build DNS TXT records with various noise and real-world records
		validRecord := fmt.Sprintf("%s%s", sso.TXTRecordPrefix, rec.Token)
		var txtRecords []string

		switch i % 4 {
		case 0:
			// Solitary valid record
			txtRecords = []string{validRecord}
		case 1:
			// Multi-record with SPF, DMARC, and Google verification
			txtRecords = []string{
				"v=spf1 include:_spf.google.com ~all",
				"google-site-verification=abcdef1234567890",
				validRecord,
				"v=DMARC1; p=reject; rua=mailto:dmarc@enterprise.com",
			}
		case 2:
			// Surrounding quotes (some DNS providers return quoted strings)
			txtRecords = []string{
				fmt.Sprintf("%q", validRecord),
			}
		case 3:
			// Valid record sandwiched between competing fake tokens
			txtRecords = []string{
				"scandrix-domain-verification=0000000000000000000000000000000000000000000000000000000000000000",
				validRecord,
				"scandrix-domain-verification=ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			}
		}

		resolver.setTXT(domain, txtRecords)

		// Execute VerifyDNS
		verifiedRec, err := svc.VerifyDNS(ctx, wsID, domain)
		if err != nil {
			t.Fatalf("[Case %d] VerifyDNS failed unexpectedly: %v (records: %v)", i, err, txtRecords)
		}
		if !verifiedRec.Verified {
			t.Fatalf("[Case %d] expected verified=true", i)
		}
	}
}

// TestMassive_SelfHostedAirGappedBypassAndPrivateNetworks executes 10,000 test cases
// verifying air-gapped deployments, private subnet rules, and loopback auto-verification.
func TestMassive_SelfHostedAirGappedBypassAndPrivateNetworks(t *testing.T) {
	ctx := context.Background()

	// In cloudMode=true, self-hosted bypass must NOT activate
	cloudSvc := sso.NewDomainVerifierService(newMockDNSResolver(), true)

	// In cloudMode=false, self-hosted bypass MUST activate for private hostnames
	selfHostedSvc := sso.NewDomainVerifierService(newMockDNSResolver(), false)

	const targetCases = 10000
	privateBases := []string{"localhost", "127.0.0.1", "corp.internal", "app.local", "dev.test", "10.0.1.5"}

	for i := 0; i < targetCases; i++ {
		wsID := uuid.New()
		base := privateBases[i%len(privateBases)]
		domain := fmt.Sprintf("node-%d.%s", i, base)
		if base == "localhost" || base == "127.0.0.1" || base == "10.0.1.5" {
			domain = base
		}
		email := fmt.Sprintf("admin@%s", domain)

		// 1. Cloud Mode: Must require DNS challenge
		cloudRec, err := cloudSvc.RequestVerification(ctx, wsID, domain, email)
		if err != nil {
			t.Fatalf("[Cloud Case %d] RequestVerification failed: %v", i, err)
		}
		if cloudRec.Verified {
			t.Fatalf("[Cloud Case %d] Cloud mode must not auto-verify private domain %s", i, domain)
		}

		// 2. Self-Hosted Mode: Must auto-verify private hostname
		selfHostedRec, err := selfHostedSvc.RequestVerification(ctx, wsID, domain, email)
		if err != nil {
			t.Fatalf("[SelfHosted Case %d] RequestVerification failed: %v", i, err)
		}
		if !selfHostedRec.Verified {
			t.Fatalf("[SelfHosted Case %d] Self-hosted mode should auto-verify %s", i, domain)
		}
		if !selfHostedRec.IsSelfHostedBypass {
			t.Fatalf("[SelfHosted Case %d] expected IsSelfHostedBypass=true", i)
		}
	}
}

func alternateCase(s string) string {
	b := strings.Builder{}
	for i, r := range s {
		if i%2 == 0 {
			b.WriteString(strings.ToUpper(string(r)))
		} else {
			b.WriteString(strings.ToLower(string(r)))
		}
	}
	return b.String()
}

func mutateToken(token string) string {
	if len(token) == 0 {
		return "invalid"
	}
	b := []byte(token)
	if b[0] == 'a' {
		b[0] = 'b'
	} else {
		b[0] = 'a'
	}
	return string(b)
}

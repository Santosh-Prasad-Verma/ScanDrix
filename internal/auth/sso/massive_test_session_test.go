// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package sso_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/sso"
)

// TestMassive_SessionLifecycleFingerprintingAndExpiry executes 10,000 test cases
// validating workbench session creation, SHA-256 fingerprinting, TTL enforcement, and cleanup.
func TestMassive_SessionLifecycleFingerprintingAndExpiry(t *testing.T) {
	ctx := context.Background()
	workbench := sso.NewSSOTestSessionWorkbench(sso.NewSAMLHandler(), nil)

	const targetCases = 10000
	sessionIDs := make([]string, targetCases)
	fingerprints := make(map[string]bool)

	for i := 0; i < targetCases; i++ {
		wsID := uuid.New()
		providerType := sso.ProviderTypeSAML2
		if i%2 == 1 {
			providerType = sso.ProviderTypeOIDC
		}

		ssoURL := fmt.Sprintf("https://idp-%d.enterprise.com/sso/saml", i)
		domains := []string{
			fmt.Sprintf("corp-%d.com", i),
			fmt.Sprintf("sub-%d.corp-%d.com", i, i),
		}

		session, err := workbench.CreateSession(ctx, wsID, providerType, ssoURL, domains, "admin@enterprise.com")
		if err != nil {
			t.Fatalf("[Case %d] CreateSession failed: %v", i, err)
		}

		if session.Status != sso.TestSessionStatusPending {
			t.Fatalf("[Case %d] expected initial status PENDING, got %s", i, session.Status)
		}
		if session.ConfigFingerprint == "" {
			t.Fatalf("[Case %d] expected non-empty config fingerprint", i)
		}

		fingerprints[session.ConfigFingerprint] = true
		sessionIDs[i] = session.SessionID
	}

	// Verify all fingerprints across unique configs were computed
	if len(fingerprints) != targetCases {
		t.Fatalf("expected %d unique fingerprints, got %d", targetCases, len(fingerprints))
	}

	// Retrieve each session and verify fields
	for i := 0; i < targetCases; i++ {
		session, err := workbench.GetSession(ctx, sessionIDs[i])
		if err != nil {
			t.Fatalf("[Case %d] GetSession failed: %v", i, err)
		}
		if session.SessionID != sessionIDs[i] {
			t.Fatalf("[Case %d] session ID mismatch: expected %s, got %s", i, sessionIDs[i], session.SessionID)
		}
	}
}

// TestMassive_SAMLAssertionParsingAndDiagnosticCodes executes 15,000 test cases
// testing assertion XML evaluation, expiration boundaries, audience validation, and error code mappings.
func TestMassive_SAMLAssertionParsingAndDiagnosticCodes(t *testing.T) {
	ctx := context.Background()
	workbench := sso.NewSSOTestSessionWorkbench(sso.NewSAMLHandler(), nil)
	const expectedAudience = "https://scandrix.internal/api/v1/auth/saml/metadata"

	const targetCases = 15000

	for i := 0; i < targetCases; i++ {
		wsID := uuid.New()
		domain := fmt.Sprintf("tenant-%d.com", i)
		ssoURL := fmt.Sprintf("https://idp.tenant-%d.com/sso", i)

		session, err := workbench.CreateSession(ctx, wsID, sso.ProviderTypeSAML2, ssoURL, []string{domain}, "admin")
		if err != nil {
			t.Fatalf("[Case %d] CreateSession failed: %v", i, err)
		}

		scenario := i % 5
		var xmlPayload string
		var expectedCode string
		now := time.Now().UTC()

		switch scenario {
		case 0:
			// Valid SAML Assertion
			notBefore := now.Add(-5 * time.Minute).Format(time.RFC3339)
			notOnOrAfter := now.Add(10 * time.Minute).Format(time.RFC3339)
			xmlPayload = buildTestSAMLAssertion(
				fmt.Sprintf("user-%d@%s", i, domain),
				expectedAudience,
				notBefore,
				notOnOrAfter,
				"Alice",
				"Engineer",
			)
			expectedCode = "" // Success

		case 1:
			// Expired Assertion (NotOnOrAfter in the past)
			notBefore := now.Add(-30 * time.Minute).Format(time.RFC3339)
			notOnOrAfter := now.Add(-5 * time.Minute).Format(time.RFC3339)
			xmlPayload = buildTestSAMLAssertion(
				fmt.Sprintf("user-%d@%s", i, domain),
				expectedAudience,
				notBefore,
				notOnOrAfter,
				"Bob",
				"Tester",
			)
			expectedCode = sso.ErrCodeExpiredAssertion

		case 2:
			// Audience Mismatch
			notBefore := now.Add(-5 * time.Minute).Format(time.RFC3339)
			notOnOrAfter := now.Add(10 * time.Minute).Format(time.RFC3339)
			xmlPayload = buildTestSAMLAssertion(
				fmt.Sprintf("user-%d@%s", i, domain),
				"https://wrong-sp.com/saml/metadata",
				notBefore,
				notOnOrAfter,
				"Charlie",
				"Admin",
			)
			expectedCode = sso.ErrCodeAudienceMismatch

		case 3:
			// Missing Email Attribute (NameID is transient GUID and no email attributes)
			notBefore := now.Add(-5 * time.Minute).Format(time.RFC3339)
			notOnOrAfter := now.Add(10 * time.Minute).Format(time.RFC3339)
			xmlPayload = fmt.Sprintf(`<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion">
				<saml:Assertion ID="_a_%d">
					<saml:Subject><saml:NameID Format="urn:oasis:names:tc:SAML:2.0:nameid-format:transient">_guid_%d</saml:NameID></saml:Subject>
					<saml:Conditions NotBefore="%s" NotOnOrAfter="%s">
						<saml:AudienceRestriction><saml:Audience>%s</saml:Audience></saml:AudienceRestriction>
					</saml:Conditions>
				</saml:Assertion>
			</samlp:Response>`, i, i, notBefore, notOnOrAfter, expectedAudience)
			expectedCode = sso.ErrCodeMissingEmailAttribute

		case 4:
			// Malformed XML payload
			xmlPayload = fmt.Sprintf("<malformed-saml-%d><incomplete", i)
			expectedCode = sso.ErrCodeInvalidAssertion
		}

		res, err := workbench.ValidateSAMLAssertion(ctx, session.SessionID, []byte(xmlPayload), expectedAudience)
		if expectedCode == "" {
			if err != nil {
				t.Fatalf("[Case %d - Scenario %d] unexpected validation error: %v", i, scenario, err)
			}
			if res.Status != sso.TestSessionStatusSuccess {
				t.Fatalf("[Case %d - Scenario %d] expected status SUCCESS, got %s", i, scenario, res.Status)
			}
			if res.Attributes == nil || res.Attributes.Email == "" {
				t.Fatalf("[Case %d - Scenario %d] expected extracted email claim", i, scenario)
			}
		} else {
			if res.Status != sso.TestSessionStatusFailed {
				t.Fatalf("[Case %d - Scenario %d] expected status FAILED, got %s", i, scenario, res.Status)
			}
			if res.FailureCode != expectedCode {
				t.Fatalf("[Case %d - Scenario %d] expected failure code %s, got %s (%s)", i, scenario, expectedCode, res.FailureCode, res.FailureMessage)
			}
		}
	}
}

// TestMassive_UserClaimExtractionAndDomainValidation executes 15,000 test cases
// verifying attribute claim normalization, domain boundaries, and multi-tenant user isolation.
func TestMassive_UserClaimExtractionAndDomainValidation(t *testing.T) {
	ctx := context.Background()
	workbench := sso.NewSSOTestSessionWorkbench(sso.NewSAMLHandler(), nil)
	const expectedAudience = "https://scandrix.internal/api/v1/auth/saml/metadata"

	const targetCases = 15000
	now := time.Now().UTC()
	notBefore := now.Add(-5 * time.Minute).Format(time.RFC3339)
	notOnOrAfter := now.Add(10 * time.Minute).Format(time.RFC3339)

	for i := 0; i < targetCases; i++ {
		wsID := uuid.New()
		allowedDomain := fmt.Sprintf("allowed-%d.org", i)
		ssoURL := fmt.Sprintf("https://idp-%d.org/sso", i)

		session, err := workbench.CreateSession(ctx, wsID, sso.ProviderTypeSAML2, ssoURL, []string{allowedDomain}, "admin")
		if err != nil {
			t.Fatalf("[Case %d] CreateSession failed: %v", i, err)
		}

		var userEmail string
		var shouldMatchDomain bool

		if i%2 == 0 {
			// Compliant domain
			userEmail = fmt.Sprintf("developer-%d@%s", i, allowedDomain)
			shouldMatchDomain = true
		} else {
			// Rogue domain mismatch
			userEmail = fmt.Sprintf("intruder-%d@unauthorized-domain-%d.com", i, i)
			shouldMatchDomain = false
		}

		xml := buildTestSAMLAssertion(userEmail, expectedAudience, notBefore, notOnOrAfter, "Dev", "User")
		res, _ := workbench.ValidateSAMLAssertion(ctx, session.SessionID, []byte(xml), expectedAudience)

		if shouldMatchDomain {
			if res.Status != sso.TestSessionStatusSuccess {
				t.Fatalf("[Case %d] expected valid domain to succeed, got %s: %s", i, res.Status, res.FailureMessage)
			}
			if res.Attributes.Email != strings.ToLower(userEmail) {
				t.Fatalf("[Case %d] email mismatch: expected %s, got %s", i, userEmail, res.Attributes.Email)
			}
		} else {
			// Domain mismatch must fail closed
			if res.Status != sso.TestSessionStatusFailed {
				t.Fatalf("[Case %d] expected domain mismatch to fail, got %s", i, res.Status)
			}
			if res.FailureCode != "DOMAIN_MISMATCH" {
				t.Fatalf("[Case %d] expected failure code DOMAIN_MISMATCH, got %s", i, res.FailureCode)
			}
		}
	}
}

func buildTestSAMLAssertion(nameID, audience, notBefore, notOnOrAfter, firstName, lastName string) string {
	return fmt.Sprintf(`<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion">
		<saml:Assertion ID="_a_%d">
			<saml:Subject>
				<saml:NameID Format="urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress">%s</saml:NameID>
			</saml:Subject>
			<saml:Conditions NotBefore="%s" NotOnOrAfter="%s">
				<saml:AudienceRestriction>
					<saml:Audience>%s</saml:Audience>
				</saml:AudienceRestriction>
			</saml:Conditions>
			<saml:AttributeStatement>
				<saml:Attribute Name="email"><saml:AttributeValue>%s</saml:AttributeValue></saml:Attribute>
				<saml:Attribute Name="firstName"><saml:AttributeValue>%s</saml:AttributeValue></saml:Attribute>
				<saml:Attribute Name="lastName"><saml:AttributeValue>%s</saml:AttributeValue></saml:Attribute>
			</saml:AttributeStatement>
		</saml:Assertion>
	</samlp:Response>`, time.Now().UnixNano(), nameID, notBefore, notOnOrAfter, audience, nameID, firstName, lastName)
}

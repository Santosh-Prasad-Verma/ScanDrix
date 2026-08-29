package sso_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/sso"
	"github.com/scandrix/backend/pkg/models"
)

func TestSSOSAMLAndOIDCFederation(t *testing.T) {
	now := time.Now().UTC()
	samlHandler := sso.NewSAMLHandler()
	oidcHandler := sso.NewOIDCHandler()
	provisioner := sso.NewJITProvisioner()

	// 1. SAML SP Metadata Generation
	entityID := "https://api.scandrix.io/auth/saml/metadata"
	acsURL := "https://api.scandrix.io/auth/saml/acs"
	dummyCert := "-----BEGIN CERTIFICATE-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA0\n-----END CERTIFICATE-----"

	metadataXML := samlHandler.GenerateSPMetadata(entityID, acsURL, dummyCert)
	if !strings.Contains(metadataXML, entityID) || !strings.Contains(metadataXML, acsURL) {
		t.Fatalf("metadata XML missing entityID or acsURL: %s", metadataXML)
	}

	// 2. SAML Assertion Verification & Extraction
	samlXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response xmlns="urn:oasis:names:tc:SAML:2.0:protocol">
  <Assertion xmlns="urn:oasis:names:tc:SAML:2.0:assertion">
    <Issuer>https://idp.okta.com/exk123</Issuer>
    <Subject>
      <NameID>alice@acme.com</NameID>
    </Subject>
    <Conditions NotBefore="%s" NotOnOrAfter="%s">
      <AudienceRestriction>
        <Audience>https://api.scandrix.io/auth/saml/metadata</Audience>
      </AudienceRestriction>
    </Conditions>
    <AttributeStatement>
      <Attribute Name="FirstName"><AttributeValue>Alice</AttributeValue></Attribute>
      <Attribute Name="LastName"><AttributeValue>Smith</AttributeValue></Attribute>
      <Attribute Name="Groups">
        <AttributeValue>Engineering</AttributeValue>
        <AttributeValue>Sec-Admins</AttributeValue>
      </Attribute>
    </AttributeStatement>
  </Assertion>
</Response>`, now.Add(-10*time.Minute).Format(time.RFC3339), now.Add(10*time.Minute).Format(time.RFC3339))

	samlIdent, err := samlHandler.ParseAndVerifyAssertion([]byte(samlXML), entityID, now)
	if err != nil {
		t.Fatalf("SAML assertion parsing failed: %v", err)
	}

	if samlIdent.Email != "alice@acme.com" || samlIdent.FirstName != "Alice" || samlIdent.LastName != "Smith" {
		t.Fatalf("unexpected SAML identity attributes: %+v", samlIdent)
	}
	if len(samlIdent.Groups) != 2 || samlIdent.Groups[1] != "Sec-Admins" {
		t.Fatalf("unexpected SAML groups: %+v", samlIdent.Groups)
	}

	// 3. OIDC ID Token Parsing & Verification
	claims := sso.OIDCClaims{
		Issuer:        "https://accounts.google.com",
		Subject:       "google_sub_123456",
		Audience:      "scandrix-client-id-abc",
		ExpiresAt:     now.Add(1 * time.Hour).Unix(),
		Email:         "bob@acme.com",
		EmailVerified: true,
		Name:          "Bob Builder",
		Groups:        []string{"Developers"},
		HD:            "acme.com",
	}

	headerJSON := `{"alg":"none","typ":"JWT"}`
	claimsJSON, _ := json.Marshal(claims)
	tokenStr := fmt.Sprintf("%s.%s.dummy_signature",
		base64.RawURLEncoding.EncodeToString([]byte(headerJSON)),
		base64.RawURLEncoding.EncodeToString(claimsJSON),
	)

	oidcIdent, err := oidcHandler.ParseAndVerifyIDToken(tokenStr, "https://accounts.google.com", "scandrix-client-id-abc", now)
	if err != nil {
		t.Fatalf("OIDC ID token parsing failed: %v", err)
	}

	if oidcIdent.Email != "bob@acme.com" || oidcIdent.FirstName != "Bob" || oidcIdent.LastName != "Builder" {
		t.Fatalf("unexpected OIDC identity attributes: %+v", oidcIdent)
	}

	// 4. JIT Provisioning & Domain Whitelisting
	idpConfig := sso.IdPConfiguration{
		ID:             uuid.New(),
		WorkspaceID:    uuid.New(),
		ProviderType:   sso.ProviderTypeSAML2,
		AllowedDomains: []string{"acme.com"},
		RoleMapping: map[string]string{
			"Sec-Admins": "ADMIN",
			"Developers": "MEMBER",
		},
	}

	// Valid domain with Admin group mapping
	userAlice, isNew, err := provisioner.ProvisionUser(context.Background(), idpConfig, samlIdent)
	if err != nil {
		t.Fatalf("provision user Alice failed: %v", err)
	}
	if !isNew || userAlice.Role != models.RoleAdmin {
		t.Fatalf("expected new user Alice with role ADMIN, got isNew=%v, role=%s", isNew, userAlice.Role)
	}

	// Valid domain with Member group mapping
	userBob, isNewBob, err := provisioner.ProvisionUser(context.Background(), idpConfig, oidcIdent)
	if err != nil {
		t.Fatalf("provision user Bob failed: %v", err)
	}
	if !isNewBob || userBob.Role != models.RoleMember {
		t.Fatalf("expected new user Bob with role MEMBER, got isNew=%v, role=%s", isNewBob, userBob.Role)
	}

	// Unauthorized domain rejected!
	evilIdent := &sso.FederatedIdentity{
		Email:  "mallory@attacker.org",
		Groups: []string{"Sec-Admins"},
	}
	_, _, err = provisioner.ProvisionUser(context.Background(), idpConfig, evilIdent)
	if err == nil {
		t.Fatal("security violation: expected unauthorized domain error for attacker.org, but provisioning succeeded")
	}
	if !strings.Contains(err.Error(), "not authorized") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

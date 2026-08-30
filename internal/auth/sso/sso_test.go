package sso_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
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
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed generating test RSA key: %v", err)
	}

	certTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &certTemplate, &certTemplate, &rsaKey.PublicKey, rsaKey)
	if err != nil {
		t.Fatalf("failed creating test certificate: %v", err)
	}
	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))

	if err := samlHandler.SetIdPCertificate(certPEM); err != nil {
		t.Fatalf("failed setting IdP cert: %v", err)
	}

	unsignedXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
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

	// Sign XML with RSA-SHA256
	h256 := sha256.Sum256([]byte(unsignedXML))
	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, h256[:])
	if err != nil {
		t.Fatalf("failed signing test XML: %v", err)
	}
	sigBase64 := base64.StdEncoding.EncodeToString(sigBytes)

	samlXML := strings.Replace(unsignedXML, "</Response>", fmt.Sprintf("<Signature xmlns=\"http://www.w3.org/2000/09/xmldsig#\"><SignedInfo><SignatureMethod Algorithm=\"http://www.w3.org/2001/04/xmldsig-more#rsa-sha256\"/></SignedInfo><SignatureValue>%s</SignatureValue></Signature></Response>", sigBase64), 1)

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

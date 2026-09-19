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
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
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

	// 2b. Security Verification: Rejection of XML Signature Wrapping (XSW) attacks (CWE-347)
	shadowSAML := strings.Replace(samlXML, "</Assertion>", "</Assertion><Assertion><Subject><NameID>attacker@evil.com</NameID></Subject></Assertion>", 1)
	_, errXSW := samlHandler.ParseAndVerifyAssertion([]byte(shadowSAML), entityID, now)
	if errXSW == nil || !strings.Contains(errXSW.Error(), "XML Signature Wrapping (XSW)") {
		t.Fatalf("expected XSW security rejection, got %v", errXSW)
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

	oidcHandler.SetSigningKey(&rsaKey.PublicKey)

	headerJSON := `{"alg":"RS256","typ":"JWT"}`
	claimsJSON, _ := json.Marshal(claims)
	contentToSign := fmt.Sprintf("%s.%s",
		base64.RawURLEncoding.EncodeToString([]byte(headerJSON)),
		base64.RawURLEncoding.EncodeToString(claimsJSON),
	)
	h256OIDC := sha256.Sum256([]byte(contentToSign))
	oidcSig, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, h256OIDC[:])
	if err != nil {
		t.Fatalf("failed signing test OIDC token: %v", err)
	}
	tokenStr := fmt.Sprintf("%s.%s", contentToSign, base64.RawURLEncoding.EncodeToString(oidcSig))

	oidcIdent, err := oidcHandler.ParseAndVerifyIDToken(tokenStr, "https://accounts.google.com", "scandrix-client-id-abc", now)
	if err != nil {
		t.Fatalf("OIDC ID token parsing failed: %v", err)
	}

	if oidcIdent.Email != "bob@acme.com" || oidcIdent.FirstName != "Bob" || oidcIdent.LastName != "Builder" {
		t.Fatalf("unexpected OIDC identity attributes: %+v", oidcIdent)
	}

	// 3b. Security Verification: Rejection of alg: none (CWE-327)
	noneHeader := `{"alg":"none","typ":"JWT"}`
	noneToken := fmt.Sprintf("%s.%s.",
		base64.RawURLEncoding.EncodeToString([]byte(noneHeader)),
		base64.RawURLEncoding.EncodeToString(claimsJSON),
	)
	_, errNone := oidcHandler.ParseAndVerifyIDToken(noneToken, "https://accounts.google.com", "scandrix-client-id-abc", now)
	if errNone == nil || !errors.Is(errNone, sso.ErrAlgorithmNone) {
		t.Fatalf("expected ErrAlgorithmNone on alg: none, got %v", errNone)
	}

	// 3c. Security Verification: Rejection of algorithm confusion (HS256 header with RSA key)
	confusionHeader := `{"alg":"HS256","typ":"JWT"}`
	confusionToken := fmt.Sprintf("%s.%s.bogus",
		base64.RawURLEncoding.EncodeToString([]byte(confusionHeader)),
		base64.RawURLEncoding.EncodeToString(claimsJSON),
	)
	_, errConfusion := oidcHandler.ParseAndVerifyIDToken(confusionToken, "https://accounts.google.com", "scandrix-client-id-abc", now)
	if errConfusion == nil || !errors.Is(errConfusion, sso.ErrAlgorithmMismatch) {
		t.Fatalf("expected ErrAlgorithmMismatch on algorithm confusion, got %v", errConfusion)
	}

	// 3d. Security Verification: Fail closed when no key is configured
	unconfiguredHandler := sso.NewOIDCHandler()
	_, errUnsigned := unconfiguredHandler.ParseAndVerifyIDToken(tokenStr, "https://accounts.google.com", "scandrix-client-id-abc", now)
	if errUnsigned == nil || !errors.Is(errUnsigned, sso.ErrUnsignedToken) {
		t.Fatalf("expected ErrUnsignedToken when key is unconfigured, got %v", errUnsigned)
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

	// 5. Role Hierarchy Precedence: Member listed AFTER Admin should not downgrade to Member
	multiGroupIdent := &sso.FederatedIdentity{
		Email:  "charlie@acme.com",
		Groups: []string{"Sec-Admins", "Developers"}, // Admin first, Member second
	}
	userCharlie, _, err := provisioner.ProvisionUser(context.Background(), idpConfig, multiGroupIdent)
	if err != nil {
		t.Fatalf("provision Charlie failed: %v", err)
	}
	if userCharlie.Role != models.RoleAdmin {
		t.Fatalf("expected Charlie to have RoleAdmin, got: %s", userCharlie.Role)
	}

	// Member first, Admin second -> should also be RoleAdmin
	multiGroupIdent2 := &sso.FederatedIdentity{
		Email:  "david@acme.com",
		Groups: []string{"Developers", "Sec-Admins"}, // Member first, Admin second
	}
	userDavid, _, err := provisioner.ProvisionUser(context.Background(), idpConfig, multiGroupIdent2)
	if err != nil {
		t.Fatalf("provision David failed: %v", err)
	}
	if userDavid.Role != models.RoleAdmin {
		t.Fatalf("expected David to have RoleAdmin, got: %s", userDavid.Role)
	}
}

func TestOIDCRemoteJWKSRotationAndDiscovery(t *testing.T) {
	ctx := context.Background()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed generating RSA key: %v", err)
	}

	kid := "key-rotation-2026"
	nStr := base64.RawURLEncoding.EncodeToString(rsaKey.PublicKey.N.Bytes())
	eBytes := big.NewInt(int64(rsaKey.PublicKey.E)).Bytes()
	eStr := base64.RawURLEncoding.EncodeToString(eBytes)

	// Mock IdP Server hosting discovery and JWKS endpoints
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(sso.OIDCProviderMetadata{
				Issuer:                "https://mock-idp.example.com",
				AuthorizationEndpoint: "https://mock-idp.example.com/oauth2/v1/authorize",
				TokenEndpoint:         "https://mock-idp.example.com/oauth2/v1/token",
				JwksURI:               "http://" + r.Host + "/keys",
			})
		case "/keys":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(sso.JSONWebKeySet{
				Keys: []sso.JSONWebKey{
					{
						Kty: "RSA",
						Kid: kid,
						Use: "sig",
						Alg: "RS256",
						N:   nStr,
						E:   eStr,
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	oidcHandler := sso.NewOIDCHandler()
	oidcHandler.SetHTTPClient(ts.Client())

	// 1. Test Discovery Endpoint
	meta, err := oidcHandler.DiscoverProvider(ctx, ts.URL)
	if err != nil {
		t.Fatalf("discovery failed: %v", err)
	}
	if meta.Issuer != "https://mock-idp.example.com" || !strings.HasSuffix(meta.JwksURI, "/keys") {
		t.Fatalf("unexpected provider metadata: %+v", meta)
	}

	// 2. Test Fetch and Cache JWKS
	if err := oidcHandler.FetchAndCacheJWKS(ctx, meta.JwksURI); err != nil {
		t.Fatalf("fetch JWKS failed: %v", err)
	}

	// 3. Verify Token using cached JWKS key matched by header kid (without explicit SetSigningKey)
	now := time.Now().UTC()
	claims := sso.OIDCClaims{
		Issuer:        "https://mock-idp.example.com",
		Subject:       "user-456",
		Audience:      "client-app-1",
		ExpiresAt:     now.Add(10 * time.Minute).Unix(),
		Email:         "charlie@example.com",
		EmailVerified: true,
	}
	headerJSON := fmt.Sprintf(`{"alg":"RS256","typ":"JWT","kid":"%s"}`, kid)
	claimsJSON, _ := json.Marshal(claims)
	content := fmt.Sprintf("%s.%s",
		base64.RawURLEncoding.EncodeToString([]byte(headerJSON)),
		base64.RawURLEncoding.EncodeToString(claimsJSON),
	)
	h256 := sha256.Sum256([]byte(content))
	sig, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, h256[:])
	if err != nil {
		t.Fatalf("signing failed: %v", err)
	}
	jwtToken := fmt.Sprintf("%s.%s", content, base64.RawURLEncoding.EncodeToString(sig))

	ident, err := oidcHandler.ParseAndVerifyIDToken(jwtToken, "https://mock-idp.example.com", "client-app-1", now)
	if err != nil {
		t.Fatalf("failed validating token via JWKS cache: %v", err)
	}
	if ident.Email != "charlie@example.com" || ident.ExternalID != "user-456" {
		t.Fatalf("unexpected identity from JWKS token: %+v", ident)
	}
}

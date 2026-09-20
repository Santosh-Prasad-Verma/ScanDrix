package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

func TestSSOOIDCClaimValidation(t *testing.T) {
	sso := NewSSOService(&OIDCProviderConfig{
		IssuerURL:    "https://auth.company.com",
		ClientID:     "scandrix-client-id",
		ClientSecret: "scandrix-secret",
		RedirectURI:  "https://app.scandrix.io/auth/callback",
	}, nil)

	nonce := GenerateSecureNonce()
	authURL := sso.GenerateOIDCAuthURL(nonce)
	if authURL == "" || len(nonce) == 0 {
		t.Fatal("failed generating OIDC auth URL")
	}
	if !strings.Contains(authURL, "client_id=scandrix-client-id") ||
		!strings.Contains(authURL, "state="+nonce) ||
		!strings.Contains(authURL, "scope=openid+email+profile") {
		t.Errorf("authURL missing expected encoded parameters: %s", authURL)
	}

	// 1. Valid claim payload with string aud
	validPayload := fmt.Sprintf(`{
		"iss": "https://auth.company.com",
		"sub": "usr_998877",
		"aud": "scandrix-client-id",
		"email": "sarah@acme.corp",
		"email_verified": true,
		"name": "Sarah Connor",
		"exp": %d
	}`, time.Now().Add(1*time.Hour).Unix())

	claims, err := sso.ValidateIDTokenPayload([]byte(validPayload), "scandrix-client-id")
	if err != nil {
		t.Fatalf("expected valid claims, got error: %v", err)
	}
	if claims.Email != "sarah@acme.corp" || claims.Name != "Sarah Connor" {
		t.Errorf("unexpected claim values: %+v", claims)
	}

	// 2. Valid claim payload with array aud per RFC 7519 §4.1.3
	arrayAudPayload := fmt.Sprintf(`{
		"iss": "https://auth.company.com",
		"sub": "usr_998877",
		"aud": ["other-service", "scandrix-client-id"],
		"email": "sarah@acme.corp",
		"email_verified": true,
		"name": "Sarah Connor",
		"exp": %d
	}`, time.Now().Add(1*time.Hour).Unix())

	arrayClaims, err := sso.ValidateIDTokenPayload([]byte(arrayAudPayload), "scandrix-client-id")
	if err != nil {
		t.Fatalf("expected valid claims for array aud, got error: %v", err)
	}
	if !arrayClaims.AudienceContains("scandrix-client-id") {
		t.Errorf("expected AudienceContains true for scandrix-client-id")
	}

	// 3. Rejection when email_verified is false
	unverifiedEmailPayload := fmt.Sprintf(`{
		"iss": "https://auth.company.com",
		"sub": "usr_998877",
		"aud": "scandrix-client-id",
		"email": "sarah@acme.corp",
		"email_verified": false,
		"exp": %d
	}`, time.Now().Add(1*time.Hour).Unix())

	_, err = sso.ValidateIDTokenPayload([]byte(unverifiedEmailPayload), "scandrix-client-id")
	if err == nil {
		t.Error("expected error for unverified email, got nil")
	} else if !strings.Contains(err.Error(), "email is not verified") {
		t.Errorf("expected 'email is not verified' error, got: %v", err)
	}

	// 4. Rejection on audience mismatch
	_, err = sso.ValidateIDTokenPayload([]byte(validPayload), "attacker-client-id")
	if err == nil {
		t.Error("expected error for audience mismatch, got nil")
	}

	// 5. Rejection on issuer mismatch
	mismatchedIssuerPayload := fmt.Sprintf(`{
		"iss": "https://evil.idp.com",
		"sub": "usr_998877",
		"aud": "scandrix-client-id",
		"email": "sarah@acme.corp",
		"email_verified": true,
		"exp": %d
	}`, time.Now().Add(1*time.Hour).Unix())

	_, err = sso.ValidateIDTokenPayload([]byte(mismatchedIssuerPayload), "scandrix-client-id")
	if err == nil {
		t.Error("expected error for issuer mismatch, got nil")
	}

	// 6. Expired claim payload (beyond clock skew)
	expiredPayload := fmt.Sprintf(`{
		"iss": "https://auth.company.com",
		"aud": "scandrix-client-id",
		"email": "sarah@acme.corp",
		"email_verified": true,
		"exp": %d
	}`, time.Now().Add(-1*time.Hour).Unix())

	_, err = sso.ValidateIDTokenPayload([]byte(expiredPayload), "scandrix-client-id")
	if err == nil {
		t.Error("expected error for expired token, got nil")
	}
}

func TestSSOSAMLAssertionParsing(t *testing.T) {
	sso := NewSSOService(nil, &SAMLProviderConfig{
		SPEntityID: "urn:scandrix:sp",
		ACSURL:     "https://app.scandrix.io/auth/saml/acs",
		IDPSSOURL:  "https://idp.okta.com/app/sso/saml",
	})

	// Test GenerateSAMLAuthnRequestWithID
	reqID, authnReq, err := sso.GenerateSAMLAuthnRequestWithID()
	if err != nil || authnReq == "" {
		t.Fatalf("failed generating SAML authn request with ID: %v", err)
	}
	if !strings.HasPrefix(reqID, "_") {
		t.Errorf("expected request ID to start with '_', got %s", reqID)
	}

	xmlBytes, err := base64.StdEncoding.DecodeString(authnReq)
	if err != nil {
		t.Fatalf("failed to decode generated authn request: %v", err)
	}
	var parsedReq samlAuthnRequestXML
	if err := xml.Unmarshal(xmlBytes, &parsedReq); err != nil {
		t.Fatalf("failed unmarshaling generated authn request XML: %v", err)
	}
	if parsedReq.ID != reqID || parsedReq.Issuer != "urn:scandrix:sp" {
		t.Errorf("parsed authn request mismatch: %+v", parsedReq)
	}

	// 1. Mock valid SAML Response with NameID as email
	rawXML := `<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol">
		<samlp:Status>
			<samlp:StatusCode Value="urn:oasis:names:tc:SAML:2.0:status:Success"/>
		</samlp:Status>
		<saml:Assertion xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion">
			<saml:Subject>
				<saml:NameID>dev-lead@enterprise.com</saml:NameID>
			</saml:Subject>
		</saml:Assertion>
	</samlp:Response>`

	b64XML := base64.StdEncoding.EncodeToString([]byte(rawXML))

	profile, err := sso.ParseSAMLResponse(b64XML)
	if err != nil {
		t.Fatalf("failed parsing SAML assertion: %v", err)
	}

	if profile.Email != "dev-lead@enterprise.com" {
		t.Errorf("expected email 'dev-lead@enterprise.com', got %s", profile.Email)
	}
	if profile.Role != models.RoleMember {
		t.Errorf("expected member role, got %s", profile.Role)
	}

	// 2. SAML Response with opaque NameID and email in AttributeStatement
	attrXML := `<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol">
		<samlp:Status>
			<samlp:StatusCode Value="urn:oasis:names:tc:SAML:2.0:status:Success"/>
		</samlp:Status>
		<saml:Assertion xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion">
			<saml:Subject>
				<saml:NameID>usr_opaque_uuid_883921</saml:NameID>
			</saml:Subject>
			<saml:AttributeStatement>
				<saml:Attribute Name="http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress">
					<saml:AttributeValue>arch-lead@enterprise.com</saml:AttributeValue>
				</saml:Attribute>
			</saml:AttributeStatement>
		</saml:Assertion>
	</samlp:Response>`

	profileAttr, err := sso.ParseSAMLResponse(base64.StdEncoding.EncodeToString([]byte(attrXML)))
	if err != nil {
		t.Fatalf("failed parsing SAML assertion with attribute email: %v", err)
	}
	if profileAttr.Email != "arch-lead@enterprise.com" {
		t.Errorf("expected email 'arch-lead@enterprise.com', got %s", profileAttr.Email)
	}

	// 3. Expired assertion conditions
	expiredCondXML := fmt.Sprintf(`<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol">
		<samlp:Status>
			<samlp:StatusCode Value="urn:oasis:names:tc:SAML:2.0:status:Success"/>
		</samlp:Status>
		<saml:Assertion xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion">
			<saml:Conditions NotBefore="%s" NotOnOrAfter="%s" />
			<saml:Subject>
				<saml:NameID>dev-lead@enterprise.com</saml:NameID>
			</saml:Subject>
		</saml:Assertion>
	</samlp:Response>`, time.Now().Add(-2*time.Hour).Format(time.RFC3339), time.Now().Add(-1*time.Hour).Format(time.RFC3339))

	_, err = sso.ParseSAMLResponse(base64.StdEncoding.EncodeToString([]byte(expiredCondXML)))
	if err == nil {
		t.Error("expected error for expired SAML assertion conditions, got nil")
	} else if !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected expired error, got: %v", err)
	}

	// 4. SAML Failure status
	failureXML := `<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol">
		<samlp:Status>
			<samlp:StatusCode Value="urn:oasis:names:tc:SAML:2.0:status:Responder"/>
		</samlp:Status>
	</samlp:Response>`

	_, err = sso.ParseSAMLResponse(base64.StdEncoding.EncodeToString([]byte(failureXML)))
	if err == nil {
		t.Error("expected error for non-Success SAML status, got nil")
	}
}

func TestSSOSAMLCryptographicVerification(t *testing.T) {
	now := time.Now().UTC()
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

	entityID := "urn:scandrix:sp"
	ssoService := NewSSOService(nil, &SAMLProviderConfig{
		SPEntityID:     entityID,
		ACSURL:         "https://app.scandrix.io/auth/saml/acs",
		IDPSSOURL:      "https://idp.okta.com/app/sso/saml",
		IDPCertificate: certPEM,
	})

	unsignedXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response xmlns="urn:oasis:names:tc:SAML:2.0:protocol">
  <Assertion xmlns="urn:oasis:names:tc:SAML:2.0:assertion">
    <Issuer>https://idp.okta.com/app</Issuer>
    <Subject>
      <NameID>crypto-user@enterprise.com</NameID>
    </Subject>
    <Conditions NotBefore="%s" NotOnOrAfter="%s">
      <AudienceRestriction>
        <Audience>urn:scandrix:sp</Audience>
      </AudienceRestriction>
    </Conditions>
  </Assertion>
</Response>`, now.Add(-5*time.Minute).Format(time.RFC3339), now.Add(5*time.Minute).Format(time.RFC3339))

	h256 := sha256.Sum256([]byte(unsignedXML))
	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, h256[:])
	if err != nil {
		t.Fatalf("failed signing test XML: %v", err)
	}
	sigBase64 := base64.StdEncoding.EncodeToString(sigBytes)

	samlXML := strings.Replace(unsignedXML, "</Response>", fmt.Sprintf("<Signature xmlns=\"http://www.w3.org/2000/09/xmldsig#\"><SignedInfo><SignatureMethod Algorithm=\"http://www.w3.org/2001/04/xmldsig-more#rsa-sha256\"/></SignedInfo><SignatureValue>%s</SignatureValue></Signature></Response>", sigBase64), 1)

	profile, err := ssoService.ParseSAMLResponse(base64.StdEncoding.EncodeToString([]byte(samlXML)))
	if err != nil {
		t.Fatalf("failed parsing cryptographically signed SAML response: %v", err)
	}
	if profile.Email != "crypto-user@enterprise.com" {
		t.Errorf("expected email 'crypto-user@enterprise.com', got %s", profile.Email)
	}

	// Test signature tampering rejection
	tamperedXML := strings.Replace(samlXML, "crypto-user@enterprise.com", "hacker@evil.com", 1)
	_, err = ssoService.ParseSAMLResponse(base64.StdEncoding.EncodeToString([]byte(tamperedXML)))
	if err == nil {
		t.Error("expected error for tampered SAML assertion signature, got nil")
	}
}

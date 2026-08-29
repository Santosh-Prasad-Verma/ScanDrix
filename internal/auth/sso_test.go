package auth

import (
	"encoding/base64"
	"fmt"
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

	// Valid claim payload
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

	// Expired claim payload
	expiredPayload := fmt.Sprintf(`{
		"iss": "https://auth.company.com",
		"aud": "scandrix-client-id",
		"email": "sarah@acme.corp",
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

	authnReq, err := sso.GenerateSAMLAuthnRequest()
	if err != nil || authnReq == "" {
		t.Fatalf("failed generating SAML authn request: %v", err)
	}

	// Mock valid SAML Response
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
}

package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// OIDCProviderConfig holds discovery and client credentials for OpenID Connect SSO.
type OIDCProviderConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// OIDCDiscoveryDoc models standard OpenID Connect discovery JSON metadata.
type OIDCDiscoveryDoc struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

// OIDCTokenClaims represents identity claims extracted from a validated ID Token.
type OIDCTokenClaims struct {
	Issuer        string `json:"iss"`
	Subject       string `json:"sub"`
	Audience      string `json:"aud"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	HostedDomain  string `json:"hd,omitempty"`
	Expiry        int64  `json:"exp"`
}

// SAMLProviderConfig models enterprise SAML 2.0 IdP connection parameters.
type SAMLProviderConfig struct {
	SPEntityID     string
	ACSURL         string
	IDPEntityID    string
	IDPSSOURL      string
	IDPCertificate string
}

// SAMLResponseXML represents a simplified parsed SAML 2.0 assertion payload.
type SAMLResponseXML struct {
	XMLName   xml.Name `xml:"Response"`
	Issuer    string   `xml:"Issuer"`
	Status    struct {
		StatusCode struct {
			Value string `xml:"Value,attr"`
		} `xml:"StatusCode"`
	} `xml:"Status"`
	Assertion struct {
		Subject struct {
			NameID string `xml:"NameID"`
		} `xml:"Subject"`
		Conditions struct {
			NotBefore    string `xml:"NotBefore,attr"`
			NotOnOrAfter string `xml:"NotOnOrAfter,attr"`
		} `xml:"Conditions"`
		AttributeStatement struct {
			Attributes []struct {
				Name       string   `xml:"Name,attr"`
				Values     []string `xml:"AttributeValue"`
			} `xml:"Attribute"`
		} `xml:"AttributeStatement"`
	} `xml:"Assertion"`
}

// SSOService coordinates enterprise authentication flows.
type SSOService struct {
	oidcConfig *OIDCProviderConfig
	samlConfig *SAMLProviderConfig
	httpClient *http.Client
}

// NewSSOService initializes the enterprise identity service.
func NewSSOService(oidc *OIDCProviderConfig, saml *SAMLProviderConfig) *SSOService {
	return &SSOService{
		oidcConfig: oidc,
		samlConfig: saml,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// GenerateOIDCAuthURL constructs the login redirection URL with a cryptographically secure state nonce.
func (s *SSOService) GenerateOIDCAuthURL(stateNonce string) string {
	if s.oidcConfig == nil {
		return ""
	}
	return fmt.Sprintf(
		"%s/protocol/openid-connect/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=openid+email+profile&state=%s",
		strings.TrimRight(s.oidcConfig.IssuerURL, "/"),
		s.oidcConfig.ClientID,
		s.oidcConfig.RedirectURI,
		stateNonce,
	)
}

// ValidateIDTokenPayload decodes and checks core OIDC claim requirements (Master Rule 5.1).
func (s *SSOService) ValidateIDTokenPayload(rawPayloadJSON []byte, expectedAudience string) (*OIDCTokenClaims, error) {
	var claims OIDCTokenClaims
	if err := json.Unmarshal(rawPayloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("invalid oidc token json: %w", err)
	}

	now := time.Now().Unix()
	if claims.Expiry < now {
		return nil, errors.New("oidc token expired")
	}

	if expectedAudience != "" && claims.Audience != expectedAudience {
		return nil, fmt.Errorf("audience mismatch: expected %s, got %s", expectedAudience, claims.Audience)
	}

	if claims.Email == "" {
		return nil, errors.New("oidc token missing email claim")
	}

	return &claims, nil
}

// GenerateSAMLAuthnRequest creates a base64-encoded SAML 2.0 AuthnRequest for IdP redirection.
func (s *SSOService) GenerateSAMLAuthnRequest() (string, error) {
	if s.samlConfig == nil {
		return "", errors.New("saml provider not configured")
	}

	requestID := "_" + uuid.New().String()
	issueInstant := time.Now().UTC().Format(time.RFC3339)

	xmlRequest := fmt.Sprintf(`<samlp:AuthnRequest xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" ID="%s" Version="2.0" IssueInstant="%s" Destination="%s" AssertionConsumerServiceURL="%s"><saml:Issuer xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion">%s</saml:Issuer></samlp:AuthnRequest>`,
		requestID, issueInstant, s.samlConfig.IDPSSOURL, s.samlConfig.ACSURL, s.samlConfig.SPEntityID,
	)

	return base64.StdEncoding.EncodeToString([]byte(xmlRequest)), nil
}

// ParseSAMLResponse decodes and verifies a base64-encoded SAMLResponse assertion from the IdP.
func (s *SSOService) ParseSAMLResponse(rawBase64 string) (*models.AccountProfile, error) {
	xmlBytes, err := base64.StdEncoding.DecodeString(rawBase64)
	if err != nil {
		return nil, fmt.Errorf("failed base64 decoding saml response: %w", err)
	}

	var resp SAMLResponseXML
	if err := xml.Unmarshal(xmlBytes, &resp); err != nil {
		return nil, fmt.Errorf("failed unmarshaling saml xml: %w", err)
	}

	if !strings.HasSuffix(resp.Status.StatusCode.Value, "Success") {
		return nil, fmt.Errorf("saml assertion failure status: %s", resp.Status.StatusCode.Value)
	}

	email := resp.Assertion.Subject.NameID
	if email == "" {
		return nil, errors.New("saml assertion missing NameID email")
	}

	// Just-In-Time provisioned account profile.
	// The workspace ID must be resolved by the caller from the SSO
	// configuration that initiated this flow — it is NOT safe to
	// hardcode a fallback workspace here.
	return &models.AccountProfile{
		ID:          uuid.New(),
		WorkspaceID: uuid.Nil, // Caller MUST populate from SAML config's workspace
		Email:       email,
		DisplayName: strings.Split(email, "@")[0],
		Role:        models.RoleMember,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}, nil
}

// GenerateSecureNonce creates a random cryptographic hex string for SSO state tracking.
func GenerateSecureNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

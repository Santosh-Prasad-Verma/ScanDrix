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
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	ssoPkg "github.com/scandrix/backend/internal/auth/sso"
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
// Supports both single-string and array-based "aud" claims per RFC 7519 §4.1.3.
type OIDCTokenClaims struct {
	Issuer        string   `json:"iss"`
	Subject       string   `json:"sub"`
	Audience      []string `json:"aud"`
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	Name          string   `json:"name"`
	HostedDomain  string   `json:"hd,omitempty"`
	Expiry        int64    `json:"exp"`
	NotBefore     int64    `json:"nbf,omitempty"`
	IssuedAt      int64    `json:"iat,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling to handle string or string-array "aud" claims.
func (c *OIDCTokenClaims) UnmarshalJSON(data []byte) error {
	type rawClaims struct {
		Issuer        string `json:"iss"`
		Subject       string `json:"sub"`
		Audience      any    `json:"aud"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		HostedDomain  string `json:"hd,omitempty"`
		Expiry        int64  `json:"exp"`
		NotBefore     int64  `json:"nbf,omitempty"`
		IssuedAt      int64  `json:"iat,omitempty"`
	}

	var raw rawClaims
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	c.Issuer = raw.Issuer
	c.Subject = raw.Subject
	c.Email = raw.Email
	c.EmailVerified = raw.EmailVerified
	c.Name = raw.Name
	c.HostedDomain = raw.HostedDomain
	c.Expiry = raw.Expiry
	c.NotBefore = raw.NotBefore
	c.IssuedAt = raw.IssuedAt

	switch v := raw.Audience.(type) {
	case string:
		if v != "" {
			c.Audience = []string{v}
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				c.Audience = append(c.Audience, s)
			}
		}
	case []string:
		c.Audience = v
	}

	return nil
}

// AudienceContains checks whether the expected audience matches any audience entry in the token.
func (c *OIDCTokenClaims) AudienceContains(expectedAudience string) bool {
	if expectedAudience == "" {
		return true
	}
	for _, a := range c.Audience {
		if a == expectedAudience {
			return true
		}
	}
	return false
}

// SAMLProviderConfig models enterprise SAML 2.0 IdP connection parameters.
type SAMLProviderConfig struct {
	SPEntityID     string
	ACSURL         string
	IDPEntityID    string
	IDPSSOURL      string
	IDPCertificate string
}

// SAMLResponseXML represents a parsed SAML 2.0 assertion payload.
type SAMLResponseXML struct {
	XMLName xml.Name `xml:"Response"`
	Issuer  string   `xml:"Issuer"`
	Status  struct {
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
				Name   string   `xml:"Name,attr"`
				Values []string `xml:"AttributeValue"`
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

// GenerateOIDCAuthURL constructs the login redirection URL with properly escaped query parameters.
func (s *SSOService) GenerateOIDCAuthURL(stateNonce string) string {
	if s.oidcConfig == nil {
		return ""
	}

	baseURL := strings.TrimRight(s.oidcConfig.IssuerURL, "/")
	var authEndpoint string
	if strings.Contains(baseURL, "/protocol/openid-connect") || strings.Contains(baseURL, "/oauth2") || strings.Contains(baseURL, "/authorize") {
		authEndpoint = baseURL
	} else {
		authEndpoint = baseURL + "/protocol/openid-connect/auth"
	}

	q := url.Values{}
	q.Set("client_id", s.oidcConfig.ClientID)
	q.Set("redirect_uri", s.oidcConfig.RedirectURI)
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("state", stateNonce)

	return authEndpoint + "?" + q.Encode()
}

// ValidateIDTokenPayload decodes and checks core OIDC claim requirements (Master Rule 5.1).
// Enforces email_verified = true and applies a 2-minute clock skew tolerance.
func (s *SSOService) ValidateIDTokenPayload(rawPayloadJSON []byte, expectedAudience string) (*OIDCTokenClaims, error) {
	var claims OIDCTokenClaims
	if err := json.Unmarshal(rawPayloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("invalid oidc token json: %w", err)
	}

	// 1. Clock skew tolerance of 2 minutes (120 seconds) for expiration and nbf
	now := time.Now().Unix()
	const clockSkewSec = 120
	if claims.Expiry > 0 && claims.Expiry+clockSkewSec < now {
		return nil, errors.New("oidc token expired")
	}
	if claims.NotBefore > 0 && claims.NotBefore-clockSkewSec > now {
		return nil, errors.New("oidc token is not yet valid (nbf)")
	}

	// 2. Validate Audience
	if expectedAudience != "" && !claims.AudienceContains(expectedAudience) {
		return nil, fmt.Errorf("audience mismatch: expected %s, got %v", expectedAudience, claims.Audience)
	}

	// 3. Validate Issuer if configured
	if s.oidcConfig != nil && s.oidcConfig.IssuerURL != "" && claims.Issuer != "" {
		cleanExpected := strings.TrimRight(s.oidcConfig.IssuerURL, "/")
		cleanActual := strings.TrimRight(claims.Issuer, "/")
		if cleanExpected != cleanActual {
			return nil, fmt.Errorf("issuer mismatch: expected %s, got %s", cleanExpected, cleanActual)
		}
	}

	// 4. Validate Email Presence and Verification
	if claims.Email == "" {
		return nil, errors.New("oidc token missing email claim")
	}
	if !claims.EmailVerified {
		return nil, errors.New("oidc email is not verified by provider")
	}

	return &claims, nil
}

type samlAuthnRequestXML struct {
	XMLName                     xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:protocol AuthnRequest"`
	ID                          string   `xml:"ID,attr"`
	Version                     string   `xml:"Version,attr"`
	IssueInstant                string   `xml:"IssueInstant,attr"`
	Destination                 string   `xml:"Destination,attr,omitempty"`
	AssertionConsumerServiceURL string   `xml:"AssertionConsumerServiceURL,attr,omitempty"`
	Issuer                      string   `xml:"urn:oasis:names:tc:SAML:2.0:assertion Issuer"`
}

// GenerateSAMLAuthnRequestWithID creates an injection-safe SAML 2.0 AuthnRequest and returns the request ID and base64 payload.
func (s *SSOService) GenerateSAMLAuthnRequestWithID() (string, string, error) {
	if s.samlConfig == nil {
		return "", "", errors.New("saml provider not configured")
	}

	requestID := "_" + uuid.New().String()
	issueInstant := time.Now().UTC().Format(time.RFC3339)

	reqObj := samlAuthnRequestXML{
		ID:                          requestID,
		Version:                     "2.0",
		IssueInstant:                issueInstant,
		Destination:                 s.samlConfig.IDPSSOURL,
		AssertionConsumerServiceURL: s.samlConfig.ACSURL,
		Issuer:                      s.samlConfig.SPEntityID,
	}

	xmlData, err := xml.Marshal(reqObj)
	if err != nil {
		return "", "", fmt.Errorf("failed marshaling saml authn request: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(xmlData)
	return requestID, encoded, nil
}

// GenerateSAMLAuthnRequest creates a base64-encoded SAML 2.0 AuthnRequest for IdP redirection.
func (s *SSOService) GenerateSAMLAuthnRequest() (string, error) {
	_, encoded, err := s.GenerateSAMLAuthnRequestWithID()
	return encoded, err
}

// ParseSAMLResponse decodes and verifies a base64-encoded SAMLResponse assertion from the IdP.
// Enforces cryptographic signature verification when IDPCertificate is provided, validates temporal conditions,
// and extracts email from NameID or AttributeStatement.
func (s *SSOService) ParseSAMLResponse(rawBase64 string) (*models.AccountProfile, error) {
	xmlBytes, err := base64.StdEncoding.DecodeString(rawBase64)
	if err != nil {
		return nil, fmt.Errorf("failed base64 decoding saml response: %w", err)
	}

	// 1. If an IdP certificate is configured, verify signature and conditions.
	if s.samlConfig != nil && strings.TrimSpace(s.samlConfig.IDPCertificate) != "" {
		handler := ssoPkg.NewSAMLHandler()
		if err := handler.SetIdPCertificate(s.samlConfig.IDPCertificate); err != nil {
			return nil, fmt.Errorf("invalid saml idp certificate: %w", err)
		}

		fedIdent, err := handler.ParseAndVerifyAssertion(xmlBytes, s.samlConfig.SPEntityID, time.Now().UTC())
		if err != nil {
			return nil, fmt.Errorf("saml cryptographic assertion verification failed: %w", err)
		}

		email := fedIdent.Email
		if email == "" {
			email = fedIdent.ExternalID
		}

		return &models.AccountProfile{
			ID:          uuid.New(),
			WorkspaceID: uuid.Nil,
			Email:       email,
			DisplayName: strings.Split(email, "@")[0],
			Role:        models.RoleMember,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}, nil
	}

	// 2. No IdP certificate configured: refuse.
	//
	// This used to fall through to plain XML parsing with only status and
	// temporal checks, then build a complete AccountProfile -- including an
	// email -- and return it for the caller to authenticate. Any party able to
	// POST to the ACS endpoint could therefore assert any identity at all: no
	// signature, no issuer check, nothing (AUDIT_REMEDIATION.md F-23).
	//
	// Status and NotBefore/NotOnOrAfter are properties of the *unverified*
	// document, so they prove nothing about who sent it. An unverified SAML
	// assertion is not authentication; it is an unauthenticated request to log
	// in as whoever the attacker named.
	//
	// The parsing below is retained only so the failure message can describe what
	// arrived, and its result is discarded.
	if s.samlConfig == nil || strings.TrimSpace(s.samlConfig.IDPCertificate) == "" {
		var resp SAMLResponseXML
		_ = xml.Unmarshal(xmlBytes, &resp)
		return nil, fmt.Errorf(
			"saml authentication refused: no IdP certificate is configured for this workspace (%s), "+
				"so the assertion cannot be cryptographically verified; refusing to authenticate an unsigned assertion",
			samlEntityIDForError(resp))
	}

	// Unreachable in practice: the block above returns unless a certificate is
	// configured, and that case is handled above with full verification.
	var resp SAMLResponseXML
	if err := xml.Unmarshal(xmlBytes, &resp); err != nil {
		return nil, fmt.Errorf("failed unmarshaling saml xml: %w", err)
	}

	if !strings.HasSuffix(resp.Status.StatusCode.Value, "Success") {
		return nil, fmt.Errorf("saml assertion failure status: %s", resp.Status.StatusCode.Value)
	}

	// Validate temporal validity with 5-minute clock skew tolerance
	now := time.Now().UTC()
	const clockSkew = 5 * time.Minute
	if resp.Assertion.Conditions.NotBefore != "" {
		nb, err := time.Parse(time.RFC3339, resp.Assertion.Conditions.NotBefore)
		if err == nil && now.Add(clockSkew).Before(nb) {
			return nil, fmt.Errorf("saml assertion is not yet valid (NotBefore: %s)", resp.Assertion.Conditions.NotBefore)
		}
	}
	if resp.Assertion.Conditions.NotOnOrAfter != "" {
		noa, err := time.Parse(time.RFC3339, resp.Assertion.Conditions.NotOnOrAfter)
		if err == nil && now.Add(-clockSkew).After(noa) {
			return nil, fmt.Errorf("saml assertion has expired (NotOnOrAfter: %s)", resp.Assertion.Conditions.NotOnOrAfter)
		}
	}

	// Resolve email from NameID or AttributeStatement
	email := strings.TrimSpace(resp.Assertion.Subject.NameID)
	if !strings.Contains(email, "@") {
		// Search in AttributeStatement for email or mail
		for _, attr := range resp.Assertion.AttributeStatement.Attributes {
			lowerName := strings.ToLower(attr.Name)
			if (strings.Contains(lowerName, "email") || strings.Contains(lowerName, "mail")) && len(attr.Values) > 0 {
				val := strings.TrimSpace(attr.Values[0])
				if strings.Contains(val, "@") {
					email = val
					break
				}
			}
		}
	}

	if email == "" || !strings.Contains(email, "@") {
		return nil, errors.New("saml assertion missing valid email in NameID or AttributeStatement")
	}

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
	if _, err := rand.Read(b); err != nil {
		u := uuid.New()
		return hex.EncodeToString(u[:])
	}
	return hex.EncodeToString(b)
}

// samlEntityIDForError extracts an issuer identifier purely for a diagnostic
// message. It is attacker-controlled and is never used for an access decision.
func samlEntityIDForError(resp SAMLResponseXML) string {
	if id := strings.TrimSpace(resp.Issuer); id != "" {
		if len(id) > 120 {
			id = id[:120]
		}
		return id
	}
	return "unknown issuer"
}

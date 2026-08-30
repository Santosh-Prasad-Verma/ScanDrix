package sso

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// SAMLHandler manages SAML 2.0 metadata generation and assertion parsing.
type SAMLHandler struct {
	idpCert *x509.Certificate
}

// NewSAMLHandler initializes the SAML handler.
func NewSAMLHandler() *SAMLHandler {
	return &SAMLHandler{}
}

// SetIdPCertificate loads an Identity Provider X.509 certificate for signature verification.
func (h *SAMLHandler) SetIdPCertificate(certPEM string) error {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		// Try raw base64 DER
		clean := strings.ReplaceAll(certPEM, "-----BEGIN CERTIFICATE-----", "")
		clean = strings.ReplaceAll(clean, "-----END CERTIFICATE-----", "")
		clean = strings.ReplaceAll(clean, "\n", "")
		clean = strings.ReplaceAll(clean, "\r", "")
		der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(clean))
		if err != nil {
			return fmt.Errorf("invalid PEM or DER certificate: %w", err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return fmt.Errorf("failed parsing x509 certificate: %w", err)
		}
		h.idpCert = cert
		return nil
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed parsing x509 certificate from PEM: %w", err)
	}
	h.idpCert = cert
	return nil
}

// GenerateSPMetadata produces standardized OASIS SAML 2.0 SP metadata XML.
func (h *SAMLHandler) GenerateSPMetadata(entityID, acsURL, certPEM string) string {
	cleanCert := strings.ReplaceAll(certPEM, "-----BEGIN CERTIFICATE-----", "")
	cleanCert = strings.ReplaceAll(cleanCert, "-----END CERTIFICATE-----", "")
	cleanCert = strings.ReplaceAll(cleanCert, "\n", "")
	cleanCert = strings.TrimSpace(cleanCert)

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" entityID="%s">
  <md:SPSSODescriptor AuthnRequestsSigned="true" WantAssertionsSigned="true" protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <md:KeyDescriptor use="signing">
      <ds:KeyInfo xmlns:ds="http://www.w3.org/2000/09/xmldsig#">
        <ds:X509Data>
          <ds:X509Certificate>%s</ds:X509Certificate>
        </ds:X509Data>
      </ds:KeyInfo>
    </md:KeyDescriptor>
    <md:NameIDFormat>urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress</md:NameIDFormat>
    <md:AssertionConsumerService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="%s" index="1" isDefault="true"/>
  </md:SPSSODescriptor>
</md:EntityDescriptor>`, entityID, cleanCert, acsURL)
}

// XML structures for assertion unmarshaling
type rawSAMLResponse struct {
	XMLName   xml.Name        `xml:"Response"`
	Assertion rawSAMLAssertion `xml:"Assertion"`
	Signature *rawXMLSignature `xml:"Signature"`
}

type rawSAMLAssertion struct {
	Issuer             string             `xml:"Issuer"`
	Subject            rawSAMLSubject     `xml:"Subject"`
	Conditions         rawSAMLConditions  `xml:"Conditions"`
	AttributeStatement rawSAMLAttributeSt `xml:"AttributeStatement"`
	Signature          *rawXMLSignature   `xml:"Signature"`
}

type rawXMLSignature struct {
	XMLName        xml.Name      `xml:"Signature"`
	SignedInfo     rawSignedInfo `xml:"SignedInfo"`
	SignatureValue string        `xml:"SignatureValue"`
	KeyInfo        rawKeyInfo    `xml:"KeyInfo"`
}

type rawSignedInfo struct {
	SignatureMethod rawAlgorithm `xml:"SignatureMethod"`
}

type rawAlgorithm struct {
	Algorithm string `xml:"Algorithm,attr"`
}

type rawKeyInfo struct {
	Certificate string `xml:"X509Data>X509Certificate"`
}

type rawSAMLSubject struct {
	NameID string `xml:"NameID"`
}

type rawSAMLConditions struct {
	NotBefore           string                   `xml:"NotBefore,attr"`
	NotOnOrAfter        string                   `xml:"NotOnOrAfter,attr"`
	AudienceRestriction rawSAMLAudienceRestrict `xml:"AudienceRestriction"`
}

type rawSAMLAudienceRestrict struct {
	Audience string `xml:"Audience"`
}

type rawSAMLAttributeSt struct {
	Attributes []rawSAMLAttribute `xml:"Attribute"`
}

type rawSAMLAttribute struct {
	Name   string   `xml:"Name,attr"`
	Values []string `xml:"AttributeValue"`
}

// VerifySignature validates XMLDSig cryptographic signature against IdP certificate.
func (h *SAMLHandler) VerifySignature(xmlData []byte, cert *x509.Certificate) error {
	if cert == nil {
		return errors.New("cannot verify signature: no IdP certificate configured")
	}

	rsaPub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("unsupported public key type: expected RSA")
	}

	var resp rawSAMLResponse
	if err := xml.Unmarshal(xmlData, &resp); err != nil {
		return fmt.Errorf("failed unmarshaling XML: %w", err)
	}

	sig := resp.Signature
	if sig == nil && resp.Assertion.Signature != nil {
		sig = resp.Assertion.Signature
	}

	if sig == nil {
		return errors.New("missing XMLDSig signature in SAML payload")
	}

	cleanSig := strings.ReplaceAll(sig.SignatureValue, "\n", "")
	cleanSig = strings.ReplaceAll(cleanSig, "\r", "")
	cleanSig = strings.TrimSpace(cleanSig)
	sigBytes, err := base64.StdEncoding.DecodeString(cleanSig)
	if err != nil {
		return fmt.Errorf("invalid base64 signature: %w", err)
	}

	// Compute digest over SignedInfo or XML content
	var digest []byte
	var hashFunc crypto.Hash
	if strings.Contains(sig.SignedInfo.SignatureMethod.Algorithm, "rsa-sha256") {
		h256 := sha256.Sum256(xmlData)
		digest = h256[:]
		hashFunc = crypto.SHA256
	} else {
		h1 := sha1.Sum(xmlData)
		digest = h1[:]
		hashFunc = crypto.SHA1
	}

	if err := rsa.VerifyPKCS1v15(rsaPub, hashFunc, digest, sigBytes); err != nil {
		return fmt.Errorf("cryptographic XMLDSig signature verification failed: %w", err)
	}

	return nil
}

// ParseAndVerifyAssertion parses SAML response XML, validates audience, temporal validity, and optional cryptographic signature.
func (h *SAMLHandler) ParseAndVerifyAssertion(xmlData []byte, expectedAudience string, now time.Time) (*FederatedIdentity, error) {
	var resp rawSAMLResponse
	decoder := xml.NewDecoder(bytes.NewReader(xmlData))
	if err := decoder.Decode(&resp); err != nil {
		return nil, fmt.Errorf("malformed SAML XML: %w", err)
	}

	assertion := resp.Assertion
	if assertion.Subject.NameID == "" {
		return nil, fmt.Errorf("SAML assertion missing NameID")
	}

	// 1. Validate Audience
	if expectedAudience != "" && assertion.Conditions.AudienceRestriction.Audience != expectedAudience {
		return nil, fmt.Errorf("audience mismatch: expected '%s', got '%s'", expectedAudience, assertion.Conditions.AudienceRestriction.Audience)
	}

	// 2. Validate Timestamps (with 5-minute clock skew tolerance)
	const clockSkew = 5 * time.Minute
	if assertion.Conditions.NotBefore != "" {
		nb, err := time.Parse(time.RFC3339, assertion.Conditions.NotBefore)
		if err == nil && now.Add(clockSkew).Before(nb) {
			return nil, fmt.Errorf("assertion is not yet valid (NotBefore: %s)", assertion.Conditions.NotBefore)
		}
	}
	if assertion.Conditions.NotOnOrAfter != "" {
		noa, err := time.Parse(time.RFC3339, assertion.Conditions.NotOnOrAfter)
		if err == nil && now.Add(-clockSkew).After(noa) {
			return nil, fmt.Errorf("assertion has expired (NotOnOrAfter: %s)", assertion.Conditions.NotOnOrAfter)
		}
	}

	// 3. Cryptographic Signature Verification (if IdP cert configured or present)
	if h.idpCert != nil {
		if err := h.VerifySignature(xmlData, h.idpCert); err != nil {
			return nil, err
		}
	}

	// 4. Extract attributes
	attrs := make(map[string]string)
	var groups []string
	firstName := ""
	lastName := ""

	for _, a := range assertion.AttributeStatement.Attributes {
		lowerName := strings.ToLower(a.Name)
		if len(a.Values) > 0 {
			attrs[lowerName] = a.Values[0]
			if strings.Contains(lowerName, "firstname") || strings.Contains(lowerName, "givenname") {
				firstName = a.Values[0]
			} else if strings.Contains(lowerName, "lastname") || strings.Contains(lowerName, "surname") {
				lastName = a.Values[0]
			} else if strings.Contains(lowerName, "group") || strings.Contains(lowerName, "role") {
				groups = append(groups, a.Values...)
			}
		}
	}

	return &FederatedIdentity{
		ExternalID: assertion.Subject.NameID,
		Email:      assertion.Subject.NameID,
		FirstName:  firstName,
		LastName:   lastName,
		Groups:     groups,
		MappedRole: models.RoleMember,
		Provider:   ProviderTypeSAML2,
		RawClaims:  attrs,
	}, nil
}

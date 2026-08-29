package sso

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// SAMLHandler manages SAML 2.0 metadata generation and assertion parsing.
type SAMLHandler struct{}

// NewSAMLHandler initializes the SAML handler.
func NewSAMLHandler() *SAMLHandler {
	return &SAMLHandler{}
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
}

type rawSAMLAssertion struct {
	Issuer             string             `xml:"Issuer"`
	Subject            rawSAMLSubject     `xml:"Subject"`
	Conditions         rawSAMLConditions  `xml:"Conditions"`
	AttributeStatement rawSAMLAttributeSt `xml:"AttributeStatement"`
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

// ParseAndVerifyAssertion parses SAML response XML and validates audience and temporal validity.
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

	// Validate Audience
	if expectedAudience != "" && assertion.Conditions.AudienceRestriction.Audience != expectedAudience {
		return nil, fmt.Errorf("audience mismatch: expected '%s', got '%s'", expectedAudience, assertion.Conditions.AudienceRestriction.Audience)
	}

	// Validate Timestamps (with 5-minute clock skew tolerance)
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

	// Extract attributes
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
		MappedRole: models.RoleMember, // Default until provisioner evaluates role mapping
		Provider:   ProviderTypeSAML2,
		RawClaims:  attrs,
	}, nil
}

package sso

import (
	"bytes"
	"crypto"
	"crypto/rsa"
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
	XMLName   xml.Name         `xml:"Response"`
	ID        string           `xml:"ID,attr"`
	Assertion rawSAMLAssertion `xml:"Assertion"`
	Signature *rawXMLSignature `xml:"Signature"`
}

type rawSAMLAssertion struct {
	ID                 string             `xml:"ID,attr"`
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
	CanonicalizationMethod rawAlgorithm  `xml:"CanonicalizationMethod"`
	SignatureMethod        rawAlgorithm  `xml:"SignatureMethod"`
	Reference              *rawReference `xml:"Reference"`
}

type rawReference struct {
	URI          string       `xml:"URI,attr"`
	DigestMethod rawAlgorithm `xml:"DigestMethod"`
	DigestValue  string       `xml:"DigestValue"`
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
	NotBefore           string                  `xml:"NotBefore,attr"`
	NotOnOrAfter        string                  `xml:"NotOnOrAfter,attr"`
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

// VerifySignature validates XMLDSig cryptographic signature against IdP certificate,
// enforcing strict defense against XML Signature Wrapping (XSW) attacks (CWE-347).
func (h *SAMLHandler) VerifySignature(xmlData []byte, cert *x509.Certificate) error {
	if cert == nil {
		return errors.New("cannot verify signature: no IdP certificate configured")
	}

	rsaPub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("unsupported public key type: expected RSA")
	}

	// 1. Detect and prevent XML Signature Wrapping (XSW) attacks (CWE-347)
	assertionCount := bytes.Count(xmlData, []byte("<Assertion")) +
		bytes.Count(xmlData, []byte("<saml:Assertion")) +
		bytes.Count(xmlData, []byte("<saml2:Assertion"))
	if assertionCount > 1 {
		return errors.New("XML Signature Wrapping (XSW) vulnerability detected: multiple assertions present in document (CWE-347)")
	}

	responseCount := bytes.Count(xmlData, []byte("<Response")) +
		bytes.Count(xmlData, []byte("<samlp:Response")) +
		bytes.Count(xmlData, []byte("<saml2p:Response"))
	if responseCount > 1 {
		return errors.New("XML Signature Wrapping (XSW) vulnerability detected: multiple response root elements present (CWE-347)")
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

	// 2. Algorithm enforcement (reject MD5/SHA-1)
	algo := strings.ToLower(sig.SignedInfo.SignatureMethod.Algorithm)
	var hashFunc crypto.Hash
	if strings.Contains(algo, "rsa-sha256") || strings.Contains(algo, "sha256") {
		hashFunc = crypto.SHA256
	} else if strings.Contains(algo, "rsa-sha384") || strings.Contains(algo, "sha384") {
		hashFunc = crypto.SHA384
	} else if strings.Contains(algo, "rsa-sha512") || strings.Contains(algo, "sha512") {
		hashFunc = crypto.SHA512
	} else {
		return fmt.Errorf("insecure or unsupported signature algorithm '%s': only RSA-SHA256/384/512 are accepted (CWE-327)", sig.SignedInfo.SignatureMethod.Algorithm)
	}

	// 3. If Reference URI is present, verify binding to Assertion or Response ID
	if sig.SignedInfo.Reference != nil && sig.SignedInfo.Reference.URI != "" {
		refURI := strings.TrimPrefix(sig.SignedInfo.Reference.URI, "#")
		if refURI != "" && refURI != resp.ID && refURI != resp.Assertion.ID {
			return fmt.Errorf("XMLDSig reference mismatch: signature references '%s', but assertion ID is '%s'", refURI, resp.Assertion.ID)
		}
	}

	// 4. In standard XMLDSig, signature is computed over canonicalized <SignedInfo>...</SignedInfo>
	if idxStart := bytes.Index(xmlData, []byte("<SignedInfo")); idxStart != -1 {
		if idxEnd := bytes.Index(xmlData[idxStart:], []byte("</SignedInfo>")); idxEnd != -1 {
			rawBlock := xmlData[idxStart : idxStart+idxEnd+len("</SignedInfo>")]
			
			// Build canonicalization variants to accommodate exc-c14n namespace propagation and whitespace rules
			signedInfoCandidates := [][]byte{
				rawBlock,
			}

			// Variant: inject XMLDSig default namespace if omitted by parent <Signature> inheritance
			if !bytes.Contains(rawBlock, []byte("xmlns")) {
				injected := bytes.Replace(rawBlock, []byte("<SignedInfo"), []byte(`<SignedInfo xmlns="http://www.w3.org/2000/09/xmldsig#"`), 1)
				signedInfoCandidates = append(signedInfoCandidates, injected)
			}

			// Variant: normalized line endings (LF only)
			if bytes.Contains(rawBlock, []byte("\r\n")) {
				lfNormalized := bytes.ReplaceAll(rawBlock, []byte("\r\n"), []byte("\n"))
				signedInfoCandidates = append(signedInfoCandidates, lfNormalized)
			}

			for _, candidate := range signedInfoCandidates {
				hasher := hashFunc.New()
				hasher.Write(candidate)
				digest := hasher.Sum(nil)
				if err := rsa.VerifyPKCS1v15(rsaPub, hashFunc, digest, sigBytes); err == nil {
					return nil
				}
			}
		}
	}

	// 5. Fallback to enveloped document digest (with <Signature> element stripped)
	dataForDigest := xmlData
	if idxStart := bytes.Index(xmlData, []byte("<Signature")); idxStart != -1 {
		if idxEnd := bytes.Index(xmlData[idxStart:], []byte("</Signature>")); idxEnd != -1 {
			dataForDigest = append(append([]byte{}, xmlData[:idxStart]...), xmlData[idxStart+idxEnd+len("</Signature>"):]...)
		}
	}

	hasher := hashFunc.New()
	hasher.Write(dataForDigest)
	digest := hasher.Sum(nil)

	if err := rsa.VerifyPKCS1v15(rsaPub, hashFunc, digest, sigBytes); err != nil {
		return fmt.Errorf("cryptographic XMLDSig signature verification failed: %w", err)
	}

	return nil
}

// HasIdPCertificate reports whether an IdP X.509 certificate has been loaded.
func (h *SAMLHandler) HasIdPCertificate() bool {
	return h.idpCert != nil
}

// ParseAssertionWithoutSignature parses SAML claims, audience, and temporal validity without cryptographic signature verification (used for sandbox diagnostic claim inspection).
func (h *SAMLHandler) ParseAssertionWithoutSignature(xmlData []byte, expectedAudience string, now time.Time) (*FederatedIdentity, error) {
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

	// 3. Extract attributes
	attrs := make(map[string]string)
	var groups []string
	firstName := ""
	lastName := ""
	email := assertion.Subject.NameID

	for _, a := range assertion.AttributeStatement.Attributes {
		lowerName := strings.ToLower(a.Name)
		if len(a.Values) > 0 {
			attrs[lowerName] = a.Values[0]
			if strings.Contains(lowerName, "email") || strings.Contains(lowerName, "mail") {
				email = a.Values[0]
			} else if strings.Contains(lowerName, "firstname") || strings.Contains(lowerName, "givenname") {
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
		Email:      email,
		FirstName:  firstName,
		LastName:   lastName,
		Groups:     groups,
		MappedRole: models.RoleMember,
		Provider:   ProviderTypeSAML2,
		RawClaims:  attrs,
	}, nil
}

// ParseAndVerifyAssertion parses SAML response XML, validates audience, temporal validity, and mandatory cryptographic signature.
func (h *SAMLHandler) ParseAndVerifyAssertion(xmlData []byte, expectedAudience string, now time.Time) (*FederatedIdentity, error) {
	identity, err := h.ParseAssertionWithoutSignature(xmlData, expectedAudience, now)
	if err != nil {
		return nil, err
	}

	// Cryptographic Signature Verification (IdP certificate is mandatory, Master Rule 5.1 & CWE-1390)
	if h.idpCert == nil {
		return nil, errors.New("cannot verify SAML assertion: IdP certificate is not configured (refusing unsigned authentication)")
	}
	if err := h.VerifySignature(xmlData, h.idpCert); err != nil {
		return nil, err
	}

	return identity, nil
}

// IdPMetadata represents parsed Identity Provider XML metadata.
type IdPMetadata struct {
	EntityID    string
	SSOURL      string
	Certificate string
}

// ParseIdPMetadataXML extracts EntityID, SingleSignOnService HTTP-POST/Redirect URL, and X.509 Certificate from IdP metadata XML.
func ParseIdPMetadataXML(metadataXML []byte) (*IdPMetadata, error) {
	var raw struct {
		EntityID         string `xml:"entityID,attr"`
		IDPSSODescriptor struct {
			KeyDescriptor []struct {
				Use     string `xml:"use,attr"`
				KeyInfo struct {
					X509Certificate string `xml:"X509Data>X509Certificate"`
				} `xml:"KeyInfo"`
			} `xml:"KeyDescriptor"`
			SingleSignOnService []struct {
				Binding  string `xml:"Binding,attr"`
				Location string `xml:"Location,attr"`
			} `xml:"SingleSignOnService"`
		} `xml:"IDPSSODescriptor"`
	}

	if err := xml.Unmarshal(metadataXML, &raw); err != nil {
		return nil, fmt.Errorf("failed parsing IdP metadata XML: %w", err)
	}

	meta := &IdPMetadata{EntityID: raw.EntityID}
	for _, sso := range raw.IDPSSODescriptor.SingleSignOnService {
		if strings.Contains(sso.Binding, "HTTP-POST") || strings.Contains(sso.Binding, "HTTP-Redirect") {
			meta.SSOURL = sso.Location
			if strings.Contains(sso.Binding, "HTTP-POST") {
				break
			}
		}
	}

	for _, kd := range raw.IDPSSODescriptor.KeyDescriptor {
		if kd.Use == "signing" || kd.Use == "" {
			cert := strings.TrimSpace(kd.KeyInfo.X509Certificate)
			if cert != "" {
				meta.Certificate = cert
				break
			}
		}
	}

	if meta.SSOURL == "" && len(raw.IDPSSODescriptor.SingleSignOnService) > 0 {
		meta.SSOURL = raw.IDPSSODescriptor.SingleSignOnService[0].Location
	}

	return meta, nil
}

// GenerateAuthnRequest constructs an RFC compliant SAML 2.0 AuthnRequest XML.
func (h *SAMLHandler) GenerateAuthnRequest(spEntityID, acsURL, idpSSOURL string) (string, string) {
	reqID := fmt.Sprintf("_%s", strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000"), ".", ""))
	issueInstant := time.Now().UTC().Format(time.RFC3339)

	xmlStr := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<samlp:AuthnRequest xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol"
                    xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"
                    ID="%s"
                    Version="2.0"
                    IssueInstant="%s"
                    Destination="%s"
                    ProtocolBinding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST"
                    AssertionConsumerServiceURL="%s">
    <saml:Issuer>%s</saml:Issuer>
    <samlp:NameIDPolicy Format="urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress" AllowCreate="true"/>
</samlp:AuthnRequest>`, reqID, issueInstant, idpSSOURL, acsURL, spEntityID)

	return xmlStr, reqID
}

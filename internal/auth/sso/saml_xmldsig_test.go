package sso

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
)

// buildSignedSAMLResponse produces a SAML Response whose assertion carries a
// genuine XMLDSig enveloped signature (Reference + DigestValue + canonicalised
// SignedInfo), signed with the returned key/certificate.
//
// The previous test fixture signed the whole unsigned document and embedded a
// <SignedInfo> with no <Reference> at all. That is not XMLDSig; it happened to
// satisfy the handler's fallback, so the suite passed without ever exercising
// real signature verification. AUDIT_REMEDIATION.md F-24.
func buildSignedSAMLResponse(t *testing.T, now time.Time, mutate func(assertion *etree.Element)) (xmlDoc string, certPEM string, certDER []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-idp"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))

	audience := "https://api.scandrix.io/auth/saml/metadata"
	raw := fmt.Sprintf(`<Response xmlns="urn:oasis:names:tc:SAML:2.0:protocol" ID="_resp123">
  <Assertion xmlns="urn:oasis:names:tc:SAML:2.0:assertion" ID="_assert456">
    <Issuer>https://idp.example.com/exk123</Issuer>
    <Subject><NameID>alice@acme.com</NameID></Subject>
    <Conditions NotBefore="%s" NotOnOrAfter="%s">
      <AudienceRestriction><Audience>%s</Audience></AudienceRestriction>
    </Conditions>
    <AttributeStatement>
      <Attribute Name="FirstName"><AttributeValue>Alice</AttributeValue></Attribute>
      <Attribute Name="LastName"><AttributeValue>Smith</AttributeValue></Attribute>
    </AttributeStatement>
  </Assertion>
</Response>`,
		now.Add(-10*time.Minute).Format(time.RFC3339),
		now.Add(10*time.Minute).Format(time.RFC3339),
		audience)

	doc := etree.NewDocument()
	if err := doc.ReadFromString(raw); err != nil {
		t.Fatalf("parse: %v", err)
	}
	assertion := doc.FindElement("//Assertion")
	if assertion == nil {
		t.Fatal("no Assertion element")
	}

	// Sign the assertion as an enveloped signature, exactly as an IdP does.
	signCtx, err := dsig.NewSigningContext(key, [][]byte{certDER})
	if err != nil {
		t.Fatalf("signing context: %v", err)
	}
	signCtx.IdAttribute = "ID"
	if err := signCtx.SetSignatureMethod(dsig.RSASHA256SignatureMethod); err != nil {
		t.Fatalf("set signature method: %v", err)
	}
	// SignEnveloped returns a COPY of the element with the signature appended;
	// it does not mutate in place, so the copy has to be spliced back in.
	signedAssertion, err := signCtx.SignEnveloped(assertion)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sigEl := signedAssertion.FindElement("./Signature")
	if sigEl == nil {
		t.Fatal("goxmldsig did not produce a Signature element")
	}
	// Drop <KeyInfo>. goxmldsig's signer left <X509Certificate> empty, and its
	// validator refuses a KeyInfo with a blank certificate rather than falling
	// back to the store. Many real IdPs omit KeyInfo too, in which case the
	// single configured IdP certificate is used.
	// Match on the local tag: etree's XPath does not resolve the "ds" prefix.
	for _, child := range sigEl.ChildElements() {
		if child.Tag == "KeyInfo" {
			sigEl.RemoveChild(child)
		}
	}
	if sigEl.FindElement(".//KeyInfo") != nil || hasLocalTag(sigEl, "KeyInfo") {
		t.Fatal("failed to remove KeyInfo from the fixture")
	}

	parent := assertion.Parent()
	if parent == nil {
		t.Fatal("assertion has no parent")
	}
	parent.RemoveChild(assertion)
	parent.AddChild(signedAssertion)
	assertion = signedAssertion

	if mutate != nil {
		mutate(assertion)
	}

	// Do NOT re-indent after signing: canonicalisation preserves whitespace
	// between elements, so pretty-printing SignedInfo after the signature was
	// computed changes the canonical bytes and the signature stops verifying.
	out, err := doc.WriteToString()
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return out, certPEM, der
}

// TestVerifySignatureAcceptsRealXMLDSig is the positive case: a properly signed
// assertion must verify. AUDIT_REMEDIATION.md F-24.
func TestVerifySignatureAcceptsRealXMLDSig(t *testing.T) {
	now := time.Now().UTC()
	xmlDoc, certPEM, _ := buildSignedSAMLResponse(t, now, nil)

	if !strings.Contains(xmlDoc, "DigestValue") {
		t.Fatal("fixture has no DigestValue; it is not a real signature")
	}
	if !strings.Contains(xmlDoc, `URI="#_assert456"`) {
		t.Fatal("fixture Reference does not bind to the assertion ID")
	}

	h := NewSAMLHandler()
	if err := h.SetIdPCertificate(certPEM); err != nil {
		t.Fatalf("set cert: %v", err)
	}
	if err := h.VerifySignature([]byte(xmlDoc), h.idpCert); err != nil {
		t.Fatalf("a genuine XMLDSig signature should verify, got: %v", err)
	}
}

// TestVerifySignatureRejectsTamperedContent is the case the old implementation
// could not catch. DigestValue covers the referenced element, so altering signed
// content must fail even though the SignatureValue is untouched and the old
// fallback would have hashed something different anyway.
func TestVerifySignatureRejectsTamperedContent(t *testing.T) {
	now := time.Now().UTC()
	xmlDoc, certPEM, _ := buildSignedSAMLResponse(t, now, func(a *etree.Element) {
		// Escalate a signed attribute after the digest was computed.
		if v := a.FindElement("//AttributeValue"); v != nil {
			v.SetText("Mallory")
		}
	})

	h := NewSAMLHandler()
	if err := h.SetIdPCertificate(certPEM); err != nil {
		t.Fatalf("set cert: %v", err)
	}
	err := h.VerifySignature([]byte(xmlDoc), h.idpCert)
	if err == nil {
		t.Fatal("tampered assertion content must not verify")
	}
	t.Logf("correctly rejected: %v", err)
}

// TestVerifySignatureRejectsWrongKey confirms the signature is actually bound
// to the certificate, not merely well-formed.
func TestVerifySignatureRejectsWrongKey(t *testing.T) {
	now := time.Now().UTC()
	xmlDoc, _, _ := buildSignedSAMLResponse(t, now, nil)

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "other"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &otherKey.PublicKey, otherKey)
	if err != nil {
		t.Fatalf("cert: %v", err)
	}
	otherPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))

	h := NewSAMLHandler()
	if err := h.SetIdPCertificate(otherPEM); err != nil {
		t.Fatalf("set cert: %v", err)
	}
	if err := h.VerifySignature([]byte(xmlDoc), h.idpCert); err == nil {
		t.Fatal("a signature must not verify against an unrelated certificate")
	}
}

// TestVerifySignatureRejectsSignatureWithNoReference pins the requirement that a
// <Reference> exists. Without one the signature is not bound to any content,
// which is exactly the gap the old implementation had.
func TestVerifySignatureRejectsSignatureWithNoReference(t *testing.T) {
	now := time.Now().UTC()
	xmlDoc, certPEM, _ := buildSignedSAMLResponse(t, now, nil)

	// Strip the whole Signature element: there is then nothing to verify.
	stripped := stripSignature(t, xmlDoc)
	h := NewSAMLHandler()
	if err := h.SetIdPCertificate(certPEM); err != nil {
		t.Fatalf("set cert: %v", err)
	}
	if err := h.VerifySignature([]byte(stripped), h.idpCert); err == nil {
		t.Fatal("payload with no signature must be rejected")
	}
}

func stripSignature(t *testing.T, xmlDoc string) string {
	t.Helper()
	doc := etree.NewDocument()
	if err := doc.ReadFromString(xmlDoc); err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Walk by local tag: the signature carries the "ds" prefix, which etree's
	// XPath does not resolve, so "//Signature" would match nothing.
	removed := 0
	var strip func(el *etree.Element)
	strip = func(el *etree.Element) {
		for _, child := range el.ChildElements() {
			if child.Tag == "Signature" {
				el.RemoveChild(child)
				removed++
				continue
			}
			strip(child)
		}
	}
	strip(doc.Root())
	if removed == 0 {
		t.Fatal("stripSignature removed nothing; XPath did not match the ds: prefixed element")
	}
	// Do NOT re-indent after signing: canonicalisation preserves whitespace
	// between elements, so pretty-printing SignedInfo after the signature was
	// computed changes the canonical bytes and the signature stops verifying.
	out, err := doc.WriteToString()
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return out
}

// hasLocalTag reports whether any descendant has the given local tag name,
// ignoring namespace prefixes.
func hasLocalTag(el *etree.Element, tag string) bool {
	for _, c := range el.ChildElements() {
		if c.Tag == tag || hasLocalTag(c, tag) {
			return true
		}
	}
	return false
}

package enterprise_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/audit"
	"github.com/scandrix/backend/internal/enterprise/classification"
	"github.com/scandrix/backend/internal/enterprise/license"
)

func TestPRClassifier(t *testing.T) {
	classifier := classification.NewPRClassifier()
	wsID := uuid.New()

	tests := []struct {
		title    string
		expected classification.PRCategory
	}{
		{"fix: resolve null pointer dereference in auth", classification.CategoryBugFix},
		{"feat(api): add multi-tenant workspace quotas", classification.CategoryFeature},
		{"sec: patch CVE-2026-1123 path traversal", classification.CategorySecurity},
		{"refactor: clean up error handlers", classification.CategoryRefactor},
		{"test: add e2e integration tests", classification.CategoryTest},
		{"chore(deps): bump rabbitmq client", classification.CategoryChore},
	}

	for _, tt := range tests {
		res := classifier.ClassifyPR(uuid.New(), wsID, 1, tt.title)
		if res.Category != tt.expected {
			t.Fatalf("expected category %s for %q, got %s", tt.expected, tt.title, res.Category)
		}
	}
}

func TestSIEMAuditStreamerAndFormatting(t *testing.T) {
	streamer := audit.NewSIEMAuditStreamer()
	wsID := uuid.New()

	// 1. Record Event 1
	e1 := streamer.RecordEvent(wsID, "usr-1", "alice@acme.com", "10.0.0.1", audit.ActionCreate, "CodeReviewSettings", "cfg-1", "", "{\"strict\":true}")
	if e1.PrevHash != "0000000000000000000000000000000000000000000000000000000000000000" {
		t.Fatalf("unexpected genesis hash: %s", e1.PrevHash)
	}

	// 2. Record Event 2 -> Must chain to e1.Hash
	e2 := streamer.RecordEvent(wsID, "usr-1", "alice@acme.com", "10.0.0.1", audit.ActionUpdate, "CodeReviewSettings", "cfg-1", "{\"strict\":true}", "{\"strict\":false}")
	if e2.PrevHash != e1.Hash {
		t.Fatalf("hash chain broken: expected prev hash %s, got %s", e1.Hash, e2.PrevHash)
	}

	// 3. Format CEF
	cef := audit.FormatCEF(e2)
	if !strings.HasPrefix(cef, "CEF:0|Scandrix|EnterprisePlatform|2.0|UPDATE|") || !strings.Contains(cef, "alice@acme.com") {
		t.Fatalf("invalid CEF log format: %s", cef)
	}

	// 4. Format RFC-5424 Syslog
	syslog := audit.FormatRFC5424(e2)
	if !strings.HasPrefix(syslog, "<134>1") || !strings.Contains(syslog, "scandrix-api") {
		t.Fatalf("invalid RFC-5424 log format: %s", syslog)
	}
}

func TestEd25519LicenseVerifier(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating keypair: %v", err)
	}

	payload := license.LicensePayload{
		LicenseID:       uuid.New(),
		CustomerName:    "Enterprise Bank Corp",
		CustomerID:      "enterprise-bank",
		Tier:            license.TierEnterprise,
		MaxSeats:        500,
		MaxRepositories: 0,
		Features: []string{
			string(license.FeatureSSOSAML),
			string(license.FeatureSCIM),
		},
		IssuedAt:  time.Now().UTC().Add(-1 * time.Hour),
		ExpiresAt: time.Now().UTC().Add(365 * 24 * time.Hour),
	}

	token, err := license.IssueLicense(payload, priv)
	if err != nil {
		t.Fatalf("failed issuing license: %v", err)
	}

	mgr := license.NewLicenseManager(pub)
	loaded, err := mgr.LoadLicense(token)
	if err != nil {
		t.Fatalf("license verification failed: %v", err)
	}

	if loaded.CustomerName != "Enterprise Bank Corp" || loaded.MaxSeats != 500 {
		t.Fatalf("verified payload corrupted: %+v", loaded)
	}
	if !mgr.HasFeature(license.FeatureSSOSAML) || !mgr.HasFeature(license.FeatureSCIM) {
		t.Fatalf("expected enterprise features enabled, got %+v", mgr.GetActiveLicense())
	}

	ent := mgr.Entitlement()
	if !ent.Valid || ent.Tier != license.TierEnterprise {
		t.Fatalf("expected valid enterprise entitlement, got %+v", ent)
	}
	if ent.Source != license.SourceSigned {
		t.Fatalf("expected signed source, got %q", ent.Source)
	}

	// A license signed by an untrusted key must not verify.
	otherPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating second keypair: %v", err)
	}
	foreign := license.NewLicenseManager(otherPub)
	if _, err := foreign.LoadLicense(token); err == nil {
		t.Fatal("expected license signed by an untrusted key to fail verification")
	}

	// A payload swapped under a valid signature must not verify: re-sign an
	// enlarged payload, then graft it under the original signature.
	tampered := payload
	tampered.MaxSeats = 99999
	tamperedBytes, err := json.Marshal(tampered)
	if err != nil {
		t.Fatalf("failed marshaling tampered payload: %v", err)
	}

	originalToken := mustIssue(t, payload, priv)
	originalRaw, err := base64.StdEncoding.DecodeString(originalToken)
	if err != nil {
		t.Fatalf("failed decoding original token: %v", err)
	}
	var originalEnvelope license.SignedLicenseToken
	if err := json.Unmarshal(originalRaw, &originalEnvelope); err != nil {
		t.Fatalf("failed unmarshaling original envelope: %v", err)
	}

	forged := license.SignedLicenseToken{
		Payload:   base64.StdEncoding.EncodeToString(tamperedBytes),
		Signature: originalEnvelope.Signature,
	}
	forgedRaw, err := json.Marshal(forged)
	if err != nil {
		t.Fatalf("failed marshaling forged envelope: %v", err)
	}
	forgedMgr := license.NewLicenseManager(pub)
	if _, err := forgedMgr.LoadLicense(base64.StdEncoding.EncodeToString(forgedRaw)); err == nil {
		t.Fatal("expected tampered license to fail verification")
	}
}

func mustIssue(t *testing.T, payload license.LicensePayload, priv ed25519.PrivateKey) string {
	t.Helper()
	token, err := license.IssueLicense(payload, priv)
	if err != nil {
		t.Fatalf("failed issuing license: %v", err)
	}
	return token
}

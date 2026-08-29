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

	claims := license.LicenseClaims{
		LicenseID:          uuid.New(),
		WorkspaceID:        uuid.New(),
		CustomerName:       "Enterprise Bank Corp",
		Tier:               "enterprise",
		MaxSeats:           500,
		AllowBYOK:          true,
		AllowAirGap:        true,
		AllowDORAAnalytics: true,
		IssuedAt:           time.Now().UTC().Add(-1 * time.Hour),
		ExpiresAt:          time.Now().UTC().Add(365 * 24 * time.Hour),
	}

	claimsJSON, _ := json.Marshal(claims)
	signature := ed25519.Sign(priv, claimsJSON)

	envelope := license.SignedLicenseEnvelope{
		PayloadB64:   base64.StdEncoding.EncodeToString(claimsJSON),
		SignatureB64: base64.StdEncoding.EncodeToString(signature),
	}
	envJSON, _ := json.Marshal(envelope)

	verifier := license.NewLicenseVerifier(pub)
	verifiedClaims, err := verifier.VerifyLicense(envJSON)
	if err != nil {
		t.Fatalf("license verification failed: %v", err)
	}

	if verifiedClaims.CustomerName != "Enterprise Bank Corp" || verifiedClaims.MaxSeats != 500 {
		t.Fatalf("verified claims corrupted: %+v", verifiedClaims)
	}
	if !verifiedClaims.AllowBYOK || !verifiedClaims.AllowAirGap {
		t.Fatalf("expected enterprise features enabled, got %+v", verifiedClaims)
	}

	// Tampered license should fail
	tamperedClaims := claims
	tamperedClaims.MaxSeats = 99999
	tamperedJSON, _ := json.Marshal(tamperedClaims)
	tamperedEnv := envelope
	tamperedEnv.PayloadB64 = base64.StdEncoding.EncodeToString(tamperedJSON)
	tamperedEnvJSON, _ := json.Marshal(tamperedEnv)

	_, errTampered := verifier.VerifyLicense(tamperedEnvJSON)
	if errTampered == nil {
		t.Fatal("expected tampered license to fail verification")
	}
}

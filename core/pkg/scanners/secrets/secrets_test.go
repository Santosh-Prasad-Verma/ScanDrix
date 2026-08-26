package secrets

import (
	"testing"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

func TestCalculateShannonEntropy(t *testing.T) {
	// Low entropy (repeating characters)
	lowEntropy := CalculateShannonEntropy("aaaaaaaaaaaaaaaa")
	if lowEntropy != 0.0 {
		t.Errorf("expected 0 entropy for identical chars, got %f", lowEntropy)
	}

	// High entropy (random base64 secret)
	highEntropy := CalculateShannonEntropy("4f9a7b2c8e1d6f3a5b0c9e8d7f6a5b4c")
	if highEntropy < 3.5 {
		t.Errorf("expected high entropy > 3.5, got %f", highEntropy)
	}
}

func TestDetectorScanContent(t *testing.T) {
	detector := NewDetector()

	content := []byte(`
package config

// Normal config
var ServerPort = 8080

// Leaked AWS Secret
var AWSAccessKey = "AKIAIOSFODNN7EXAMPLE"

// Leaked GitHub Token
var GitHubToken = "ghp_1234567890abcdefghijklmnopqrstuvwxyz"

// Leaked Stripe Live Key
var StripeSecret = "sk_live_51ABC1234567890defghijklmnop"

// Leaked Postgres Connection
var DatabaseURI = "postgresql://admin:super_secret_password_123@db.internal.corp:5432/prod_db"

// This is a dummy example key for testing (should be ignored)
var DummyKey = "AKIAEXAMPLEMOCKKEY12"
`)

	tenantID := uuid.New()
	projectID := uuid.New()
	scanID := uuid.New()

	findings, evidences := detector.ScanContent(tenantID, projectID, scanID, "config.go", content)

	if len(findings) < 4 {
		t.Fatalf("expected at least 4 secret findings, got %d", len(findings))
	}

	hasAWS := false
	hasGitHub := false
	hasStripe := false
	hasDB := false

	for _, f := range findings {
		if f.Category != domain.FindingCategorySecretLeak {
			t.Errorf("expected category SECRET_LEAK, got %s", f.Category)
		}
		if *f.CWEID != "CWE-798" {
			t.Errorf("expected CWE-798 for secret leak, got %s", *f.CWEID)
		}

		if f.Severity == domain.FindingSeverityCritical {
			hasAWS = true
		}
		if f.Title == "Exposed GitHub Personal Access Token in config.go" {
			hasGitHub = true
		}
		if f.Title == "Exposed Stripe Live API Key in config.go" {
			hasStripe = true
		}
		if f.Title == "Exposed Database Connection String with Credentials in config.go" {
			hasDB = true
		}
	}

	if !hasAWS {
		t.Errorf("expected AWS finding")
	}
	if !hasGitHub {
		t.Errorf("expected GitHub finding")
	}
	if !hasStripe {
		t.Errorf("expected Stripe finding")
	}
	if !hasDB {
		t.Errorf("expected DB URI finding")
	}
	if len(evidences) != len(findings) {
		t.Errorf("expected equal count of evidences and findings")
	}
}

func TestMaskSecret(t *testing.T) {
	secret := "AKIAIOSFODNN7EXAMPLE"
	masked := MaskSecret(secret)

	if masked != "AKIA...MPLE" {
		t.Errorf("expected AKIA...MPLE, got %s", masked)
	}

	shortSecret := "123"
	if MaskSecret(shortSecret) != "******" {
		t.Errorf("expected short secret to be fully masked")
	}
}

func TestValidateKeyFormat(t *testing.T) {
	if !ValidateKeyFormat("AWS", "AKIA1234567890ABCDEF") {
		t.Errorf("expected valid AWS key format")
	}
	if !ValidateKeyFormat("GITHUB", "ghp_1234567890abcdefghijklmnopqrstuvwxyz") {
		t.Errorf("expected valid GitHub token format")
	}
	if !ValidateKeyFormat("STRIPE", "sk_live_1234567890abcdefghijklmnop") {
		t.Errorf("expected valid Stripe key format")
	}
}

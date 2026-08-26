package secrets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

// SecretRule defines a high-precision secret detector rule.
type SecretRule struct {
	ID          string
	Name        string
	Severity    domain.FindingSeverity
	Pattern     *regexp.Regexp
	MinEntropy  float64
	Description string
}

// Detector scans commit history and files for exposed API keys, private keys, and passwords.
type Detector struct {
	rules []SecretRule
}

// NewDetector creates a secret detector initialized with industry-standard secret signatures.
func NewDetector() *Detector {
	return &Detector{
		rules: defaultSecretRules(),
	}
}

// CalculateShannonEntropy returns the Shannon entropy of a string (H = -sum(p * log2(p))).
func CalculateShannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	freq := make(map[rune]float64)
	for _, char := range s {
		freq[char]++
	}
	var entropy float64
	length := float64(len(s))
	for _, count := range freq {
		p := count / length
		entropy -= p * math.Log2(p)
	}
	return entropy
}

func defaultSecretRules() []SecretRule {
	return []SecretRule{
		{
			ID:          "CODEHOUND-SEC-AWS-001",
			Name:        "AWS Access Key ID",
			Severity:    domain.FindingSeverityCritical,
			Pattern:     regexp.MustCompile(`\b((?:AKIA|ASIA|ABIA|ACCA)[0-9A-Z]{16})\b`),
			MinEntropy:  3.0,
			Description: "Hardcoded AWS Access Key ID detected. Exposing IAM credentials can lead to full AWS infrastructure compromise.",
		},
		{
			ID:          "CODEHOUND-SEC-GITHUB-001",
			Name:        "GitHub Personal Access Token",
			Severity:    domain.FindingSeverityCritical,
			Pattern:     regexp.MustCompile(`\b(ghp_[0-9a-zA-Z]{36}|github_pat_[0-9a-zA-Z_]{82}|gho_[0-9a-zA-Z]{36})\b`),
			MinEntropy:  4.0,
			Description: "Exposed GitHub Personal Access Token (PAT). Grants direct repository and organization write access.",
		},
		{
			ID:          "CODEHOUND-SEC-STRIPE-001",
			Name:        "Stripe Live API Key",
			Severity:    domain.FindingSeverityCritical,
			Pattern:     regexp.MustCompile(`\b((?:sk|rk)_live_[0-9a-zA-Z]{24,34})\b`),
			MinEntropy:  3.5,
			Description: "Exposed Stripe Live Secret Key. Allows unauthorized financial charges, refund manipulation, and customer data export.",
		},
		{
			ID:          "CODEHOUND-SEC-OPENAI-001",
			Name:        "OpenAI / OpenRouter API Key",
			Severity:    domain.FindingSeverityHigh,
			Pattern:     regexp.MustCompile(`\b(sk-[a-zA-Z0-9]{48,}|sk-or-v1-[a-fA-F0-9]{64})\b`),
			MinEntropy:  4.0,
			Description: "Exposed LLM provider API token. Subject to immediate credit draining and unauthorized model queries.",
		},
		{
			ID:          "CODEHOUND-SEC-PRIVKEY-001",
			Name:        "Asymmetric Private Key",
			Severity:    domain.FindingSeverityCritical,
			Pattern:     regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY-----`),
			MinEntropy:  0.0,
			Description: "Unencrypted cryptographic private key embedded in source code repository.",
		},
		{
			ID:          "CODEHOUND-SEC-DBURI-001",
			Name:        "Database Connection String with Credentials",
			Severity:    domain.FindingSeverityHigh,
			Pattern:     regexp.MustCompile(`(?i)\b((?:postgres|postgresql|mysql|mongodb(?:\+srv)?|redis|cockroachdb)://[^:\s@]+:[^@\s]+@[^\s/]+)`),
			MinEntropy:  3.0,
			Description: "Plaintext database connection string containing embedded username and password.",
		},
		{
			ID:          "CODEHOUND-SEC-SLACK-001",
			Name:        "Slack Token / Webhook URL",
			Severity:    domain.FindingSeverityHigh,
			Pattern:     regexp.MustCompile(`\b(xox[baprs]-[0-9a-zA-Z]{10,48}|https://hooks\.slack\.com/services/T[0-9a-zA-Z_]+/B[0-9a-zA-Z_]+/[0-9a-zA-Z_]+)\b`),
			MinEntropy:  3.2,
			Description: "Slack API bot/user token or incoming webhook URL exposed in source code.",
		},
		{
			ID:          "CODEHOUND-SEC-GENERIC-001",
			Name:        "High-Entropy Secret in Variable Assignment",
			Severity:    domain.FindingSeverityMedium,
			Pattern:     regexp.MustCompile(`(?i)(?:api_key|secret_key|auth_token|client_secret|access_token|password|bearer)\s*[:=]\s*["']([a-zA-Z0-9_\-\.]{24,})["']`),
			MinEntropy:  4.5,
			Description: "Generic high-entropy secret detected in variable assignment string literal.",
		},
	}
}

// MaskSecret returns a masked preview of a secret (e.g. "AKIA...39FA").
func MaskSecret(secret string) string {
	if len(secret) <= 8 {
		return "******"
	}
	return secret[:4] + "..." + secret[len(secret)-4:]
}

// ScanContent scans a file content for secrets and returns findings and evidence.
func (d *Detector) ScanContent(tenantID, projectID, scanID uuid.UUID, filePath string, content []byte) ([]domain.Finding, []domain.Evidence) {
	var findings []domain.Finding
	var evidences []domain.Evidence

	lines := strings.Split(string(content), "\n")

	for i, line := range lines {
		lineNum := i + 1

		// Ignore explicit mock comments
		lowerLine := strings.ToLower(line)
		trimmed := strings.TrimSpace(lowerLine)
		if (strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "/*")) &&
			(strings.Contains(trimmed, "dummy") || strings.Contains(trimmed, "mock") || strings.Contains(trimmed, "fake")) {
			continue
		}

		for _, rule := range d.rules {
			matches := rule.Pattern.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				secretMatch := match[0]
				if len(match) > 1 && match[1] != "" {
					secretMatch = match[1]
				}

				// Check Shannon entropy threshold
				if rule.MinEntropy > 0 {
					entropy := CalculateShannonEntropy(secretMatch)
					if entropy < rule.MinEntropy {
						continue
					}
				}

				findingID := uuid.New()
				maskedSecret := MaskSecret(secretMatch)

				// Deterministic canonical key
				hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%s", rule.ID, filePath, lineNum, maskedSecret)))
				canonicalKey := fmt.Sprintf("SECRET-%s-%s", rule.ID, hex.EncodeToString(hash[:8]))

				cwe := "CWE-798"
				finding := domain.Finding{
					ID:              findingID,
					TenantID:        tenantID,
					ProjectID:       projectID,
					CanonicalKey:    canonicalKey,
					Category:        domain.FindingCategorySecretLeak,
					Severity:        rule.Severity,
					State:           domain.FindingStateVerifiedProven,
					Confidence:      0.98,
					Title:           fmt.Sprintf("Exposed %s in %s", rule.Name, filePath),
					Description:     fmt.Sprintf("%s Masked value: `%s`", rule.Description, maskedSecret),
					PrimaryFile:     filePath,
					PrimaryLine:     lineNum,
					CWEID:           &cwe,
					FirstSeenScanID: scanID,
					LastSeenScanID:  scanID,
				}

				evidence := domain.Evidence{
					ID:             uuid.New(),
					FindingID:      findingID,
					EvidenceType:   "SECRET_ENTROPY_REGEX",
					SourceAnalyzer: "CODEHOUND_GITLEAKS_CORE",
					Strength:       "CRITICAL",
					Summary:        fmt.Sprintf("Secret pattern %s verified on line %d (Entropy: %.2f)", rule.Name, lineNum, CalculateShannonEntropy(secretMatch)),
					Payload: map[string]any{
						"rule_id":       rule.ID,
						"secret_type":   rule.Name,
						"masked_value":  maskedSecret,
						"line_number":   lineNum,
						"entropy_score": CalculateShannonEntropy(secretMatch),
					},
				}

				findings = append(findings, finding)
				evidences = append(evidences, evidence)
			}
		}
	}

	return findings, evidences
}

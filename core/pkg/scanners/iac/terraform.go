package iac

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

// TerraformRule defines a security check for Terraform IaC files.
type TerraformRule struct {
	ID          string
	Title       string
	Severity    domain.FindingSeverity
	Pattern     *regexp.Regexp
	Description string
	Remediation string
}

// TerraformScanner inspects Terraform .tf files for security misconfigurations.
type TerraformScanner struct {
	rules []TerraformRule
}

// NewTerraformScanner initializes Terraform security rules.
func NewTerraformScanner() *TerraformScanner {
	return &TerraformScanner{
		rules: []TerraformRule{
			{
				ID:          "CODEHOUND-IAC-TF-S3-001",
				Title:       "Publicly Accessible S3 Bucket ACL",
				Severity:    domain.FindingSeverityCritical,
				Pattern:     regexp.MustCompile(`(?i)(acl\s*=\s*["'](?:public-read|public-read-write)["'])`),
				Description: "S3 bucket configured with public ACL grants worldwide read/write access to sensitive data.",
				Remediation: "Set acl = 'private' and enable AWS S3 Public Access Block.",
			},
			{
				ID:          "CODEHOUND-IAC-TF-SG-001",
				Title:       "Open Ingress to Sensitive Port (0.0.0.0/0)",
				Severity:    domain.FindingSeverityHigh,
				Pattern:     regexp.MustCompile(`(?i)(cidr_blocks\s*=\s*\[\s*["']0\.0\.0\.0/0["']\s*\])`),
				Description: "Security group allows unrestricted ingress traffic from any IP address on the internet.",
				Remediation: "Restrict ingress cidr_blocks to specific VPC CIDR blocks, VPN subnets, or bastion host IPs.",
			},
			{
				ID:          "CODEHOUND-IAC-TF-ENC-001",
				Title:       "Unencrypted Storage Volume / Database",
				Severity:    domain.FindingSeverityHigh,
				Pattern:     regexp.MustCompile(`(?i)(storage_encrypted\s*=\s*false|encrypted\s*=\s*false)`),
				Description: "AWS RDS database or EBS volume created without AES-256 / KMS storage encryption at rest.",
				Remediation: "Set storage_encrypted = true (or encrypted = true) with KMS Key ID.",
			},
		},
	}
}

// ScanTerraform scans .tf file content for misconfigurations.
func (s *TerraformScanner) ScanTerraform(tenantID, projectID, scanID uuid.UUID, filePath string, content []byte) ([]domain.Finding, []domain.Evidence) {
	var findings []domain.Finding
	var evidences []domain.Evidence

	lines := strings.Split(string(content), "\n")

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		for _, rule := range s.rules {
			if rule.Pattern.MatchString(trimmed) {
				findingID := uuid.New()
				hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", rule.ID, filePath, lineNum)))
				canonicalKey := fmt.Sprintf("IAC-%s-%s", rule.ID, hex.EncodeToString(hash[:8]))

				cwe := "CWE-1188"
				finding := domain.Finding{
					ID:              findingID,
					TenantID:        tenantID,
					ProjectID:       projectID,
					CanonicalKey:    canonicalKey,
					Category:        domain.FindingCategoryIaCMisconfig,
					Severity:        rule.Severity,
					State:           domain.FindingStateVerifiedProven,
					Confidence:      0.98,
					Title:           fmt.Sprintf("[%s] %s", rule.ID, rule.Title),
					Description:     fmt.Sprintf("%s Remediation: %s", rule.Description, rule.Remediation),
					PrimaryFile:     filePath,
					PrimaryLine:     lineNum,
					CWEID:           &cwe,
					FirstSeenScanID: scanID,
					LastSeenScanID:  scanID,
				}

				evidence := domain.Evidence{
					ID:             uuid.New(),
					FindingID:      findingID,
					EvidenceType:   "IAC_TERRAFORM_MISCONFIG",
					SourceAnalyzer: "CODEHOUND_TFSEC_IAC",
					Strength:       "CRITICAL",
					Summary:        fmt.Sprintf("Terraform rule %s triggered on line %d: %s", rule.ID, lineNum, rule.Title),
					Payload: map[string]any{
						"rule_id":     rule.ID,
						"line_number": lineNum,
						"code_sample": trimmed,
						"remediation": rule.Remediation,
					},
				}

				findings = append(findings, finding)
				evidences = append(evidences, evidence)
			}
		}
	}

	return findings, evidences
}

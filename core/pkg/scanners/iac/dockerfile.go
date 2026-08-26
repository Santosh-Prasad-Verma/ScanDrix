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

// DockerfileRule defines a Dockerfile security check.
type DockerfileRule struct {
	ID          string
	Title       string
	Severity    domain.FindingSeverity
	Pattern     *regexp.Regexp
	Description string
	Remediation string
}

// DockerfileScanner checks container images for security anti-patterns.
type DockerfileScanner struct {
	rules []DockerfileRule
}

// NewDockerfileScanner initializes Dockerfile security rules.
func NewDockerfileScanner() *DockerfileScanner {
	return &DockerfileScanner{
		rules: []DockerfileRule{
			{
				ID:          "CODEHOUND-IAC-DOCKER-001",
				Title:       "Insecure Curl/Wget Piping Directly to Shell",
				Severity:    domain.FindingSeverityCritical,
				Pattern:     regexp.MustCompile(`(?i)(?:curl|wget).*?\|\s*(?:sh|bash|sudo\s+sh|sudo\s+bash)`),
				Description: "Downloading and executing remote scripts via shell piping bypasses integrity verification.",
				Remediation: "Download the script, verify its cryptographic checksum (SHA-256), and execute it safely.",
			},
			{
				ID:          "CODEHOUND-IAC-DOCKER-002",
				Title:       "Exposing Insecure Remote Management Port (SSH Port 22)",
				Severity:    domain.FindingSeverityHigh,
				Pattern:     regexp.MustCompile(`(?i)EXPOSE\s+.*?\b22\b`),
				Description: "Exposing SSH port 22 in application container violates single-responsibility principle.",
				Remediation: "Remove SSH server from container image. Use docker exec or kubernetes exec instead.",
			},
			{
				ID:          "CODEHOUND-IAC-DOCKER-003",
				Title:       "Use of Mutable Image Tag ':latest'",
				Severity:    domain.FindingSeverityMedium,
				Pattern:     regexp.MustCompile(`(?i)^FROM\s+[^\s:]+:latest`),
				Description: "Using ':latest' base image introduces non-deterministic builds and untested vulnerabilities.",
				Remediation: "Pin the base image to an immutable semantic version or SHA-256 digest (e.g. alpine:3.19).",
			},
			{
				ID:          "CODEHOUND-IAC-DOCKER-004",
				Title:       "Missing Non-Root USER Instruction",
				Severity:    domain.FindingSeverityHigh,
				Pattern:     nil, // Handled via structural file analysis
				Description: "Container executes by default as root user (UID 0), increasing container escape impact.",
				Remediation: "Add 'USER nonroot' or 'USER 10001:10001' before ENTRYPOINT/CMD.",
			},
		},
	}
}

// ScanDockerfile scans a Dockerfile content for misconfigurations.
func (s *DockerfileScanner) ScanDockerfile(tenantID, projectID, scanID uuid.UUID, filePath string, content []byte) ([]domain.Finding, []domain.Evidence) {
	var findings []domain.Finding
	var evidences []domain.Evidence

	lines := strings.Split(string(content), "\n")
	hasUserDirective := false

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(strings.ToUpper(trimmed), "USER ") {
			hasUserDirective = true
		}

		for _, rule := range s.rules {
			if rule.Pattern != nil && rule.Pattern.MatchString(trimmed) {
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
					Confidence:      0.95,
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
					EvidenceType:   "IAC_DOCKER_MISCONFIG",
					SourceAnalyzer: "CODEHOUND_CHECKOV_IAC",
					Strength:       "HIGH",
					Summary:        fmt.Sprintf("Dockerfile rule %s triggered on line %d: %s", rule.ID, lineNum, rule.Title),
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

	// Check if missing USER directive
	if !hasUserDirective && len(lines) > 0 {
		rule := s.rules[3] // Missing Non-Root USER Instruction
		findingID := uuid.New()
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:1", rule.ID, filePath)))
		canonicalKey := fmt.Sprintf("IAC-%s-%s", rule.ID, hex.EncodeToString(hash[:8]))

		cwe := "CWE-250"
		finding := domain.Finding{
			ID:              findingID,
			TenantID:        tenantID,
			ProjectID:       projectID,
			CanonicalKey:    canonicalKey,
			Category:        domain.FindingCategoryIaCMisconfig,
			Severity:        rule.Severity,
			State:           domain.FindingStateVerifiedProven,
			Confidence:      0.90,
			Title:           fmt.Sprintf("[%s] %s in %s", rule.ID, rule.Title, filePath),
			Description:     fmt.Sprintf("%s Remediation: %s", rule.Description, rule.Remediation),
			PrimaryFile:     filePath,
			PrimaryLine:     1,
			CWEID:           &cwe,
			FirstSeenScanID: scanID,
			LastSeenScanID:  scanID,
		}

		evidence := domain.Evidence{
			ID:             uuid.New(),
			FindingID:      findingID,
			EvidenceType:   "IAC_DOCKER_MISCONFIG",
			SourceAnalyzer: "CODEHOUND_CHECKOV_IAC",
			Strength:       "HIGH",
			Summary:        "Dockerfile does not specify a non-root USER instruction.",
			Payload: map[string]any{
				"rule_id":     rule.ID,
				"line_number": 1,
				"remediation": rule.Remediation,
			},
		}

		findings = append(findings, finding)
		evidences = append(evidences, evidence)
	}

	return findings, evidences
}

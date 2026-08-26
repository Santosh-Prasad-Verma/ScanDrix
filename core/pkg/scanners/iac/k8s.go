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

// K8sRule defines a security check for Kubernetes manifests.
type K8sRule struct {
	ID          string
	Title       string
	Severity    domain.FindingSeverity
	Pattern     *regexp.Regexp
	Description string
	Remediation string
}

// K8sScanner inspects Kubernetes YAML manifests for container security risks.
type K8sScanner struct {
	rules []K8sRule
}

// NewK8sScanner initializes Kubernetes manifest security rules.
func NewK8sScanner() *K8sScanner {
	return &K8sScanner{
		rules: []K8sRule{
			{
				ID:          "CODEHOUND-IAC-K8S-001",
				Title:       "Privileged Container Execution",
				Severity:    domain.FindingSeverityCritical,
				Pattern:     regexp.MustCompile(`(?i)(privileged\s*:\s*true)`),
				Description: "Container executes in privileged mode, granting full access to host devices and root capabilities.",
				Remediation: "Set securityContext.privileged = false and add only specific necessary Linux capabilities.",
			},
			{
				ID:          "CODEHOUND-IAC-K8S-002",
				Title:       "Shared Host PID / Network Namespace",
				Severity:    domain.FindingSeverityHigh,
				Pattern:     regexp.MustCompile(`(?i)(hostPID\s*:\s*true|hostNetwork\s*:\s*true|hostIPC\s*:\s*true)`),
				Description: "Pod shares the host process ID or network namespace, breaking container process isolation.",
				Remediation: "Remove hostPID: true, hostNetwork: true, and hostIPC: true from pod spec.",
			},
			{
				ID:          "CODEHOUND-IAC-K8S-003",
				Title:       "HostPath Volume Mount",
				Severity:    domain.FindingSeverityHigh,
				Pattern:     regexp.MustCompile(`(?i)(hostPath\s*:)`),
				Description: "Mounting the underlying host filesystem allows malicious containers to access node credentials or escape.",
				Remediation: "Use PersistentVolumeClaims (PVCs) or ConfigMaps/Secrets rather than hostPath mounts.",
			},
		},
	}
}

// ScanK8sManifest scans a Kubernetes YAML file for security misconfigurations.
func (s *K8sScanner) ScanK8sManifest(tenantID, projectID, scanID uuid.UUID, filePath string, content []byte) ([]domain.Finding, []domain.Evidence) {
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

				cwe := "CWE-250"
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
					EvidenceType:   "IAC_K8S_MISCONFIG",
					SourceAnalyzer: "CODEHOUND_CHECKOV_K8S",
					Strength:       "CRITICAL",
					Summary:        fmt.Sprintf("Kubernetes rule %s triggered on line %d: %s", rule.ID, lineNum, rule.Title),
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

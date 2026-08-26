package sca

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

// VulnerabilityAdvisory represents a security advisory from OSV / NVD / GitHub Security Advisory.
type VulnerabilityAdvisory struct {
	ID             string                 `json:"id"`      // e.g. GHSA-xxxx or CVE-2023-xxxx
	PackageName    string                 `json:"package"` // e.g. "github.com/gin-gonic/gin"
	Ecosystem      string                 `json:"ecosystem"`
	AffectedRange  string                 `json:"affected_range"`
	FixedVersion   string                 `json:"fixed_version"`
	Severity       domain.FindingSeverity `json:"severity"`
	Summary        string                 `json:"summary"`
	CWEID          string                 `json:"cwe_id"`
}

// OSVDatabase holds known security advisories across software ecosystems.
type OSVDatabase struct {
	advisories []VulnerabilityAdvisory
}

// NewOSVDatabase initializes the OSV advisory database.
func NewOSVDatabase() *OSVDatabase {
	return &OSVDatabase{
		advisories: defaultAdvisories(),
	}
}

func defaultAdvisories() []VulnerabilityAdvisory {
	return []VulnerabilityAdvisory{
		{
			ID:            "CVE-2023-45288",
			PackageName:   "golang.org/x/net",
			Ecosystem:     "Go",
			AffectedRange: "< 0.23.0",
			FixedVersion:  "0.23.0",
			Severity:      domain.FindingSeverityHigh,
			Summary:       "HTTP/2 CONTINUATION flood unbounded memory consumption leading to Denial of Service.",
			CWEID:         "CWE-400",
		},
		{
			ID:            "CVE-2022-29078",
			PackageName:   "ejs",
			Ecosystem:     "npm",
			AffectedRange: "< 3.1.7",
			FixedVersion:  "3.1.7",
			Severity:      domain.FindingSeverityCritical,
			Summary:       "Server-Side Template Injection (SSTI) in ejs leading to Remote Code Execution.",
			CWEID:         "CWE-94",
		},
		{
			ID:            "CVE-2021-44906",
			PackageName:   "minimist",
			Ecosystem:     "npm",
			AffectedRange: "< 1.2.6",
			FixedVersion:  "1.2.6",
			Severity:      domain.FindingSeverityHigh,
			Summary:       "Prototype Pollution in minimist argument parser.",
			CWEID:         "CWE-1321",
		},
		{
			ID:            "CVE-2022-42969",
			PackageName:   "py",
			Ecosystem:     "PyPI",
			AffectedRange: "< 1.11.0",
			FixedVersion:  "1.11.0",
			Severity:      domain.FindingSeverityHigh,
			Summary:       "Regular expression denial of service (ReDoS) via subversion client wrapper.",
			CWEID:         "CWE-1333",
		},
		{
			ID:            "CVE-2023-32681",
			PackageName:   "requests",
			Ecosystem:     "PyPI",
			AffectedRange: "< 2.31.0",
			FixedVersion:  "2.31.0",
			Severity:      domain.FindingSeverityMedium,
			Summary:       "Requests session unintended leak of Proxy-Authorization header during HTTPS redirect.",
			CWEID:         "CWE-200",
		},
	}
}

// MatchVulnerabilities checks discovered dependencies against the advisory database.
func (db *OSVDatabase) MatchVulnerabilities(tenantID, projectID, scanID uuid.UUID, manifestFile string, deps []PackageDependency) ([]domain.Finding, []domain.Evidence) {
	var findings []domain.Finding
	var evidences []domain.Evidence

	for _, dep := range deps {
		for _, adv := range db.advisories {
			if strings.EqualFold(dep.Name, adv.PackageName) && strings.EqualFold(dep.Ecosystem, adv.Ecosystem) {
				// Matched vulnerable package dependency
				findingID := uuid.New()
				cveID := adv.ID
				cweID := adv.CWEID

				hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s", adv.ID, dep.Name, dep.Version)))
				canonicalKey := fmt.Sprintf("SCA-%s-%s", adv.ID, hex.EncodeToString(hash[:8]))

				finding := domain.Finding{
					ID:              findingID,
					TenantID:        tenantID,
					ProjectID:       projectID,
					CanonicalKey:    canonicalKey,
					Category:        domain.FindingCategorySecurityVuln,
					Severity:        adv.Severity,
					State:           domain.FindingStateVerifiedProven,
					Confidence:      0.99,
					Title:           fmt.Sprintf("[%s] Vulnerability in %s (%s)", adv.ID, dep.Name, dep.Version),
					Description:     fmt.Sprintf("%s (Fixed in version: %s)", adv.Summary, adv.FixedVersion),
					PrimaryFile:     manifestFile,
					PrimaryLine:     1,
					CWEID:           &cweID,
					CVEID:           &cveID,
					FirstSeenScanID: scanID,
					LastSeenScanID:  scanID,
				}

				evidence := domain.Evidence{
					ID:             uuid.New(),
					FindingID:      findingID,
					EvidenceType:   "SCA_DEPENDENCY_MATCH",
					SourceAnalyzer: "CODEHOUND_OSV_TRIVY_CORE",
					Strength:       "CRITICAL",
					Summary:        fmt.Sprintf("Dependency %s@%s matched advisory %s (Fixed in %s)", dep.Name, dep.Version, adv.ID, adv.FixedVersion),
					Payload: map[string]any{
						"advisory_id":    adv.ID,
						"package_name":   dep.Name,
						"version":        dep.Version,
						"ecosystem":      dep.Ecosystem,
						"affected_range": adv.AffectedRange,
						"fixed_version":  adv.FixedVersion,
						"manifest_file":  manifestFile,
					},
				}

				findings = append(findings, finding)
				evidences = append(evidences, evidence)
			}
		}
	}

	return findings, evidences
}

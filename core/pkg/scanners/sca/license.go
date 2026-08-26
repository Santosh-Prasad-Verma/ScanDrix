package sca

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

// LicenseCategory classifies legal obligations of open source software licenses.
type LicenseCategory string

const (
	LicenseCategoryPermissive    LicenseCategory = "PERMISSIVE"
	LicenseCategoryWeakCopyleft  LicenseCategory = "WEAK_COPYLEFT"
	LicenseCategoryStrongCopyleft LicenseCategory = "STRONG_COPYLEFT"
	LicenseCategoryUnlicensed    LicenseCategory = "UNLICENSED"
)

// LicenseInfo contains policy details regarding a software license.
type LicenseInfo struct {
	SPDXID      string
	Name        string
	Category    LicenseCategory
	IsCopyleft  bool
	Description string
}

// LicenseChecker evaluates license compatibility for proprietary projects.
type LicenseChecker struct {
	knownLicenses map[string]LicenseInfo
}

// NewLicenseChecker initializes standard SPDX license categorization.
func NewLicenseChecker() *LicenseChecker {
	return &LicenseChecker{
		knownLicenses: defaultLicenses(),
	}
}

func defaultLicenses() map[string]LicenseInfo {
	return map[string]LicenseInfo{
		"MIT": {
			SPDXID:      "MIT",
			Name:        "MIT License",
			Category:    LicenseCategoryPermissive,
			IsCopyleft:  false,
			Description: "Permissive license allowing commercial reuse with attribution.",
		},
		"Apache-2.0": {
			SPDXID:      "Apache-2.0",
			Name:        "Apache License 2.0",
			Category:    LicenseCategoryPermissive,
			IsCopyleft:  false,
			Description: "Permissive license with explicit patent grant and trademark protection.",
		},
		"BSD-3-Clause": {
			SPDXID:      "BSD-3-Clause",
			Name:        "BSD 3-Clause 'New' or 'Revised' License",
			Category:    LicenseCategoryPermissive,
			IsCopyleft:  false,
			Description: "Permissive license with endorsement prohibition.",
		},
		"ISC": {
			SPDXID:      "ISC",
			Name:        "ISC License",
			Category:    LicenseCategoryPermissive,
			IsCopyleft:  false,
			Description: "Functionally equivalent to simplified BSD license.",
		},
		"MPL-2.0": {
			SPDXID:      "MPL-2.0",
			Name:        "Mozilla Public License 2.0",
			Category:    LicenseCategoryWeakCopyleft,
			IsCopyleft:  true,
			Description: "File-level copyleft license. Modified files must be open sourced.",
		},
		"LGPL-3.0": {
			SPDXID:      "LGPL-3.0-only",
			Name:        "GNU Lesser General Public License v3.0",
			Category:    LicenseCategoryWeakCopyleft,
			IsCopyleft:  true,
			Description: "Weak copyleft license requiring dynamic linking to avoid viral licensing.",
		},
		"GPL-3.0": {
			SPDXID:      "GPL-3.0-only",
			Name:        "GNU General Public License v3.0",
			Category:    LicenseCategoryStrongCopyleft,
			IsCopyleft:  true,
			Description: "Strong viral copyleft license. Incorporating requires releasing entire application source code under GPL.",
		},
		"AGPL-3.0": {
			SPDXID:      "AGPL-3.0-only",
			Name:        "GNU Affero General Public License v3.0",
			Category:    LicenseCategoryStrongCopyleft,
			IsCopyleft:  true,
			Description: "Network-triggered viral copyleft license. SaaS usage mandates open sourcing server backend.",
		},
	}
}

// CheckLicenseCompliance scans package licenses and flags copyleft risks in proprietary projects.
func (lc *LicenseChecker) CheckLicenseCompliance(tenantID, projectID, scanID uuid.UUID, manifestFile, rawLicense string) ([]domain.Finding, []domain.Evidence) {
	var findings []domain.Finding
	var evidences []domain.Evidence

	cleanLicense := strings.TrimSpace(rawLicense)
	if cleanLicense == "" {
		return nil, nil
	}

	for spdxKey, info := range lc.knownLicenses {
		if strings.EqualFold(spdxKey, cleanLicense) || strings.Contains(strings.ToUpper(cleanLicense), strings.ToUpper(spdxKey)) {
			if info.Category == LicenseCategoryStrongCopyleft {
				findingID := uuid.New()

				hash := sha256.Sum256([]byte(fmt.Sprintf("LICENSE:%s:%s", manifestFile, info.SPDXID)))
				canonicalKey := fmt.Sprintf("LIC-%s-%s", info.SPDXID, hex.EncodeToString(hash[:8]))

				finding := domain.Finding{
					ID:              findingID,
					TenantID:        tenantID,
					ProjectID:       projectID,
					CanonicalKey:    canonicalKey,
					Category:        domain.FindingCategoryLicenseConflict,
					Severity:        domain.FindingSeverityHigh,
					State:           domain.FindingStateVerifiedProven,
					Confidence:      0.95,
					Title:           fmt.Sprintf("Strong Copyleft License (%s) Detected in %s", info.SPDXID, manifestFile),
					Description:     fmt.Sprintf("%s May legally obligate disclosing proprietary source code under %s.", info.Description, info.Name),
					PrimaryFile:     manifestFile,
					PrimaryLine:     1,
					FirstSeenScanID: scanID,
					LastSeenScanID:  scanID,
				}

				evidence := domain.Evidence{
					ID:             uuid.New(),
					FindingID:      findingID,
					EvidenceType:   "LICENSE_POLICY_VIOLATION",
					SourceAnalyzer: "CODEHOUND_SCA_LICENSE",
					Strength:       "HIGH",
					Summary:        fmt.Sprintf("Manifest %s declared license %s categorized as %s", manifestFile, info.SPDXID, info.Category),
					Payload: map[string]any{
						"license_spdx":    info.SPDXID,
						"license_name":    info.Name,
						"category":        string(info.Category),
						"is_copyleft":     info.IsCopyleft,
						"manifest_file":   manifestFile,
					},
				}

				findings = append(findings, finding)
				evidences = append(evidences, evidence)
			}
			break
		}
	}

	return findings, evidences
}

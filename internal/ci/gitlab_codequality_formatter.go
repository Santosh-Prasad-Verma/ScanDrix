// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package ci

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// GitLabCodeQualityFinding implements GitLab Code Quality JSON artifact specification.
type GitLabCodeQualityFinding struct {
	Description string                     `json:"description"`
	CheckName   string                     `json:"check_name"`
	Fingerprint string                     `json:"fingerprint"`
	Severity    string                     `json:"severity"` // "info", "minor", "major", "critical", "blocker"
	Location    GitLabCodeQualityLocation `json:"location"`
}

type GitLabCodeQualityLocation struct {
	Path  string                 `json:"path"`
	Lines GitLabCodeQualityLines `json:"lines"`
}

type GitLabCodeQualityLines struct {
	Begin int `json:"begin"`
}

// FormatGitLabCodeQuality produces GitLab Code Quality array JSON.
func FormatGitLabCodeQuality(findings []CIFinding) ([]byte, error) {
	items := make([]GitLabCodeQualityFinding, 0, len(findings))

	for _, f := range findings {
		ruleID := f.RuleID
		if ruleID == "" {
			ruleID = "SD-" + f.Category
		}

		severity := "minor"
		switch f.Severity {
		case FailCritical:
			severity = "critical"
		case FailError:
			severity = "major"
		case FailWarning:
			severity = "minor"
		case FailInfo:
			severity = "info"
		}

		// Compute deterministic fingerprint
		hashData := fmt.Sprintf("%s:%d:%s:%s", f.FilePath, f.StartLine, ruleID, f.Message)
		sum := sha256.Sum256([]byte(hashData))
		fingerprint := hex.EncodeToString(sum[:])

		line := f.StartLine
		if line <= 0 {
			line = 1
		}

		item := GitLabCodeQualityFinding{
			Description: f.Message,
			CheckName:   ruleID,
			Fingerprint: fingerprint,
			Severity:    severity,
			Location: GitLabCodeQualityLocation{
				Path: f.FilePath,
				Lines: GitLabCodeQualityLines{
					Begin: line,
				},
			},
		}
		items = append(items, item)
	}

	return json.MarshalIndent(items, "", "  ")
}

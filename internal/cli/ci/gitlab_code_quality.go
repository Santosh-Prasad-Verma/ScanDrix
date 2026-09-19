// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ci

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scandrix/backend/pkg/models"
)

// GitLabCodeQualityFinding implements GitLab Code Quality artifact schema specification.
type GitLabCodeQualityFinding struct {
	Description string                `json:"description"`
	CheckName   string                `json:"check_name"`
	Fingerprint string                `json:"fingerprint"`
	Severity    string                `json:"severity"` // info, minor, major, critical, blocker
	Location    GitLabQualityLocation `json:"location"`
}

type GitLabQualityLocation struct {
	Path  string            `json:"path"`
	Lines GitLabQualityLines `json:"lines"`
}

type GitLabQualityLines struct {
	Begin int `json:"begin"`
}

// ConvertToGitLabCodeQuality transforms review findings into GitLab Code Quality JSON format.
func ConvertToGitLabCodeQuality(findings []models.CodeFinding) []GitLabCodeQualityFinding {
	var results []GitLabCodeQualityFinding

	for _, f := range findings {
		sev := mapGitLabSeverity(string(f.Severity))
		fingerprint := generateFingerprint(f.FilePath, f.StartLine, f.Title)

		checkName := f.Category
		if checkName == "" {
			checkName = "scandrix-review"
		}

		desc := f.Title
		if f.Description != "" {
			desc = fmt.Sprintf("%s: %s", f.Title, f.Description)
		}

		results = append(results, GitLabCodeQualityFinding{
			Description: desc,
			CheckName:   checkName,
			Fingerprint: fingerprint,
			Severity:    sev,
			Location: GitLabQualityLocation{
				Path: f.FilePath,
				Lines: GitLabQualityLines{
					Begin: f.StartLine,
				},
			},
		})
	}

	if results == nil {
		results = []GitLabCodeQualityFinding{}
	}
	return results
}

// FormatGitLabCodeQualityJSON outputs formatted JSON for GitLab Code Quality artifacts.
func FormatGitLabCodeQualityJSON(findings []models.CodeFinding) ([]byte, error) {
	ql := ConvertToGitLabCodeQuality(findings)
	return json.MarshalIndent(ql, "", "  ")
}

func mapGitLabSeverity(sev string) string {
	switch strings.ToLower(sev) {
	case "critical":
		return "blocker"
	case "high", "error":
		return "critical"
	case "medium", "warning":
		return "major"
	case "low":
		return "minor"
	default:
		return "info"
	}
}

func generateFingerprint(path string, line int, title string) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s:%d:%s", path, line, title)))
	return hex.EncodeToString(h.Sum(nil))
}

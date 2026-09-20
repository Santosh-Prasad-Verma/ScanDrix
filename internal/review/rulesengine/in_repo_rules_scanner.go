// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package rulesengine

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

var frontmatterRegex = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n(.*)$`)

// InRepoRulesScanner discovers and parses custom rules stored within repository files.
type InRepoRulesScanner struct{}

// NewInRepoRulesScanner constructs an in-repo rules scanner.
func NewInRepoRulesScanner() *InRepoRulesScanner {
	return &InRepoRulesScanner{}
}

// IsRuleFilePath checks if a path matches `.scandrix/rules/` or `.drixy/rules/`.
func (s *InRepoRulesScanner) IsRuleFilePath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	clean = strings.TrimPrefix(strings.TrimPrefix(clean, "./"), "/")
	return strings.HasPrefix(clean, ".scandrix/rules/") || strings.HasPrefix(clean, ".drixy/rules/")
}

type ruleFrontmatter struct {
	Title           string            `yaml:"title"`
	Slug            string            `yaml:"slug"`
	Severity        string            `yaml:"severity"`
	Scope           string            `yaml:"scope"`
	Paths           []string          `yaml:"paths"`
	Languages       []string          `yaml:"languages"`
	RemediationHint string            `yaml:"remediation_hint"`
	Detector        *detectorRawInput `yaml:"detector"`
}

type detectorRawInput struct {
	Type            string `yaml:"type" json:"type"`
	Pattern         string `yaml:"pattern" json:"pattern"`
	NegativePattern string `yaml:"negative_pattern" json:"negative_pattern"`
	Flags           string `yaml:"flags" json:"flags"`
	Reason          string `yaml:"reason" json:"reason"`
}

// ParseRuleFile parses a markdown rule with frontmatter or a JSON rule file.
func (s *InRepoRulesScanner) ParseRuleFile(filePath string, raw []byte) (*DrixyRule, error) {
	clean := strings.ToLower(filepath.ToSlash(filePath))
	filename := filepath.Base(filePath)

	if strings.HasSuffix(clean, ".json") {
		type jsonRule struct {
			Title           string            `json:"title"`
			Slug            string            `json:"slug"`
			Description     string            `json:"description"`
			Severity        string            `json:"severity"`
			Scope           string            `json:"scope"`
			Paths           []string          `json:"paths"`
			Languages       []string          `json:"languages"`
			RemediationHint string            `json:"remediation_hint"`
			Detector        *detectorRawInput `json:"detector"`
		}
		var jr jsonRule
		if err := json.Unmarshal(raw, &jr); err != nil {
			return nil, fmt.Errorf("failed to parse JSON rule %s: %w", filePath, err)
		}

		slug := jr.Slug
		if slug == "" {
			slug = strings.TrimSuffix(filename, filepath.Ext(filename))
		}

		sev := parseSeverity(jr.Severity)
		scope := parseScope(jr.Scope)

		rule := &DrixyRule{
			ID:              uuid.New(),
			Slug:            slug,
			Title:           jr.Title,
			Description:     jr.Description,
			Severity:        sev,
			Scope:           scope,
			PathGlobs:       jr.Paths,
			LanguageFilters: jr.Languages,
			RemediationHint: jr.RemediationHint,
			Status:          StatusActive,
			Origin:          OriginRepoSync,
			Inheritable:     false,
			CreatedAt:       time.Now().UTC(),
			UpdatedAt:       time.Now().UTC(),
		}

		if jr.Detector != nil && jr.Detector.Pattern != "" {
			rule.Detector = &CompiledRuleDetector{
				Type:            DetectorType(jr.Detector.Type),
				Pattern:         jr.Detector.Pattern,
				NegativePattern: jr.Detector.NegativePattern,
				Flags:           jr.Detector.Flags,
				Reason:          jr.Detector.Reason,
			}
			_ = CompileRuleDetector(rule.Detector)
		}
		return rule, nil
	}

	// Markdown with frontmatter
	content := string(raw)
	matches := frontmatterRegex.FindStringSubmatch(content)
	if len(matches) < 3 {
		// No frontmatter: entire content is description, filename is title
		slug := strings.TrimSuffix(filename, filepath.Ext(filename))
		return &DrixyRule{
			ID:          uuid.New(),
			Slug:        slug,
			Title:       slug,
			Description: strings.TrimSpace(content),
			Severity:    models.SeverityMedium,
			Scope:       ScopeFile,
			Status:      StatusActive,
			Origin:      OriginRepoSync,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}, nil
	}

	fmYaml := matches[1]
	bodyText := strings.TrimSpace(matches[2])

	var fm ruleFrontmatter
	if err := yaml.Unmarshal([]byte(fmYaml), &fm); err != nil {
		return nil, fmt.Errorf("invalid YAML frontmatter in %s: %w", filePath, err)
	}

	slug := fm.Slug
	if slug == "" {
		slug = strings.TrimSuffix(filename, filepath.Ext(filename))
	}
	title := fm.Title
	if title == "" {
		title = slug
	}

	rule := &DrixyRule{
		ID:              uuid.New(),
		Slug:            slug,
		Title:           title,
		Description:     bodyText,
		Severity:        parseSeverity(fm.Severity),
		Scope:           parseScope(fm.Scope),
		PathGlobs:       fm.Paths,
		LanguageFilters: fm.Languages,
		RemediationHint: fm.RemediationHint,
		Status:          StatusActive,
		Origin:          OriginRepoSync,
		Inheritable:     false,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	if fm.Detector != nil && fm.Detector.Pattern != "" {
		dType := DetectorRegex
		if fm.Detector.Type != "" {
			dType = DetectorType(fm.Detector.Type)
		}
		rule.Detector = &CompiledRuleDetector{
			Type:            dType,
			Pattern:         fm.Detector.Pattern,
			NegativePattern: fm.Detector.NegativePattern,
			Flags:           fm.Detector.Flags,
			Reason:          fm.Detector.Reason,
		}
		_ = CompileRuleDetector(rule.Detector)
	}

	return rule, nil
}

// ScanPatches scans pull request diff patches for new or modified rule files.
func (s *InRepoRulesScanner) ScanPatches(patches []*diff.FilePatch) ([]*DrixyRule, error) {
	var rules []*DrixyRule

	for _, patch := range patches {
		targetPath := patch.NewPath
		if targetPath == "" {
			targetPath = patch.OldPath
		}

		if s.IsRuleFilePath(targetPath) {
			var sb strings.Builder
			for _, h := range patch.Hunks {
				for _, l := range h.Lines {
					if l.Type != diff.LineDeletion {
						sb.WriteString(l.Content)
						sb.WriteString("\n")
					}
				}
			}
			content := sb.String()
			if strings.TrimSpace(content) == "" {
				continue
			}

			rule, err := s.ParseRuleFile(targetPath, []byte(content))
			if err == nil && rule != nil {
				rules = append(rules, rule)
			}
		}
	}

	return rules, nil
}

func parseSeverity(s string) models.FindingSeverity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return models.SeverityCritical
	case "high":
		return models.SeverityHigh
	case "medium":
		return models.SeverityMedium
	case "low":
		return models.SeverityLow
	case "info":
		return models.SeverityInfo
	default:
		return models.SeverityMedium
	}
}

func parseScope(s string) DrixyRuleScope {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pr", "pull_request":
		return ScopePullRequest
	case "directory":
		return ScopeDirectory
	case "commit":
		return ScopeCommit
	default:
		return ScopeFile
	}
}

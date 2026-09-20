// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package aiengine

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ReferenceKind classifies an identified external entity in PR descriptions.
type ReferenceKind string

const (
	RefKindJira     ReferenceKind = "JIRA"
	RefKindLinear   ReferenceKind = "LINEAR"
	RefKindGitHub   ReferenceKind = "GITHUB_ISSUE"
	RefKindGitLab   ReferenceKind = "GITLAB_ISSUE"
	RefKindCVE      ReferenceKind = "CVE"
	RefKindCommit   ReferenceKind = "COMMIT_SHA"
	RefKindWebURL   ReferenceKind = "URL"
)

// DetectedReference captures an external ticket, issue, CVE, or commit citation.
type DetectedReference struct {
	Kind     ReferenceKind `json:"kind"`
	Key      string        `json:"key"`
	RawMatch string        `json:"raw_match"`
	URL      string        `json:"url,omitempty"`
}

// ReferenceDetector extracts project management and security citations from text.
type ReferenceDetector struct {
	jiraRegex   *regexp.Regexp
	linearRegex *regexp.Regexp
	ghIssueReg  *regexp.Regexp
	cveRegex    *regexp.Regexp
	commitRegex *regexp.Regexp
	urlRegex    *regexp.Regexp
}

// NewReferenceDetector constructs a reference detector.
func NewReferenceDetector() *ReferenceDetector {
	return &ReferenceDetector{
		// JIRA: e.g. PROJ-123, SEC-999, SCANDRIX-404
		jiraRegex: regexp.MustCompile(`\b([A-Z][A-Z0-9_]{1,10}-\d+)\b`),
		// Linear: e.g. ENG-123, LIN-456
		linearRegex: regexp.MustCompile(`\b(ENG|LIN)-\d+\b`),
		// GitHub/GitLab issue: e.g. #123, GH-123, fixes #456
		ghIssueReg: regexp.MustCompile(`(?:^|\s)(?:#|GH-)(\d+)\b`),
		// CVE: e.g. CVE-2026-12345
		cveRegex: regexp.MustCompile(`\b(CVE-\d{4}-\d{4,8})\b`),
		// Commit SHA: 7 to 40 hex characters
		commitRegex: regexp.MustCompile(`\b([0-9a-f]{7,40})\b`),
		// HTTP/HTTPS URLs
		urlRegex: regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `]+`),
	}
}

// DetectReferences scans PR title, description, and commit messages for external citations.
func (d *ReferenceDetector) DetectReferences(text string) []DetectedReference {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	seen := make(map[string]struct{})
	var refs []DetectedReference

	// 1. CVE Citations
	for _, match := range d.cveRegex.FindAllStringSubmatch(text, -1) {
		key := strings.ToUpper(match[1])
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			refs = append(refs, DetectedReference{
				Kind:     RefKindCVE,
				Key:      key,
				RawMatch: match[0],
				URL:      "https://nvd.nist.gov/vuln/detail/" + key,
			})
		}
	}

	// 2. Linear Tickets
	for _, match := range d.linearRegex.FindAllStringSubmatch(text, -1) {
		key := strings.ToUpper(match[0])
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			refs = append(refs, DetectedReference{
				Kind:     RefKindLinear,
				Key:      key,
				RawMatch: match[0],
			})
		}
	}

	// 3. Jira Tickets
	for _, match := range d.jiraRegex.FindAllStringSubmatch(text, -1) {
		key := strings.ToUpper(match[1])
		// Avoid classifying CVEs as Jira tickets
		if strings.HasPrefix(key, "CVE-") {
			continue
		}
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			refs = append(refs, DetectedReference{
				Kind:     RefKindJira,
				Key:      key,
				RawMatch: match[0],
			})
		}
	}

	// 4. GitHub Issues
	for _, match := range d.ghIssueReg.FindAllStringSubmatch(text, -1) {
		num := match[1]
		key := "#" + num
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			refs = append(refs, DetectedReference{
				Kind:     RefKindGitHub,
				Key:      num,
				RawMatch: strings.TrimSpace(match[0]),
			})
		}
	}

	// 5. URLs
	for _, match := range d.urlRegex.FindAllString(text, -1) {
		cleanURL := strings.TrimRight(match, ".,;:)")
		if _, exists := seen[cleanURL]; !exists {
			seen[cleanURL] = struct{}{}
			refs = append(refs, DetectedReference{
				Kind:     RefKindWebURL,
				Key:      cleanURL,
				RawMatch: match,
				URL:      cleanURL,
			})
		}
	}

	// Sort references by kind then key for deterministic output
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Kind == refs[j].Kind {
			return refs[i].Key < refs[j].Key
		}
		return refs[i].Kind < refs[j].Kind
	})

	return refs
}

// FormatContextMarkdown generates a formatted citation list for the agent review prompt.
func (d *ReferenceDetector) FormatContextMarkdown(refs []DetectedReference) string {
	if len(refs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### Linked Tickets & Context References\n")
	for _, r := range refs {
		switch r.Kind {
		case RefKindCVE:
			sb.WriteString(fmt.Sprintf("- 🛡️ **Security Advisory:** [%s](%s)\n", r.Key, r.URL))
		case RefKindLinear:
			sb.WriteString(fmt.Sprintf("- 📐 **Linear Issue:** `%s`\n", r.Key))
		case RefKindJira:
			sb.WriteString(fmt.Sprintf("- 🎫 **Jira Ticket:** `%s`\n", r.Key))
		case RefKindGitHub:
			sb.WriteString(fmt.Sprintf("- 🐙 **Issue:** `#%s`\n", r.Key))
		case RefKindWebURL:
			sb.WriteString(fmt.Sprintf("- 🔗 **External Reference:** %s\n", r.URL))
		}
	}

	return sb.String()
}

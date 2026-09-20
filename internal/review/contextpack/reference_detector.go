// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package contextpack

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	// Jira and Linear issue key pattern: 2-10 uppercase alphanumeric chars, hyphen, digits
	jiraTicketRegex = regexp.MustCompile(`\b([A-Z][A-Z0-9]{1,9}-\d+)\b`)

	// GitHub issue reference pattern: #123
	githubIssueRegex = regexp.MustCompile(`(?:^|[\s,;:(])#(\d+)\b`)

	// Git commit SHA pattern: 7 to 40 hex chars
	gitSHARegex = regexp.MustCompile(`\b([0-9a-f]{7,40})\b`)

	// Markdown documentation link pattern
	docPathRegex = regexp.MustCompile(`\b((?:docs/|architecture/|design/)[a-zA-Z0-9_\-./]+\.md)\b`)
)

// DetectedReferences aggregates all extracted external and repository markers.
type DetectedReferences struct {
	TicketKeys   []string `json:"ticket_keys"`
	IssueNumbers []int    `json:"issue_numbers"`
	CommitSHAs   []string `json:"commit_shas"`
	DocPaths     []string `json:"doc_paths"`
}

// ReferenceDetector parses unstructured text to extract ticket IDs, issue numbers, and doc links.
type ReferenceDetector struct{}

// NewReferenceDetector constructs a new reference detector.
func NewReferenceDetector() *ReferenceDetector {
	return &ReferenceDetector{}
}

// DetectReferences scans title, description, and commit messages to extract external entity references.
func (d *ReferenceDetector) DetectReferences(texts ...string) DetectedReferences {
	ticketSet := make(map[string]struct{})
	issueSet := make(map[int]struct{})
	shaSet := make(map[string]struct{})
	docSet := make(map[string]struct{})

	for _, text := range texts {
		if strings.TrimSpace(text) == "" {
			continue
		}

		// 1. Detect Jira/Linear ticket keys
		for _, m := range jiraTicketRegex.FindAllStringSubmatch(text, -1) {
			if len(m) > 1 {
				key := strings.ToUpper(strings.TrimSpace(m[1]))
				// Filter out common false positives like UTF-8, SHA-256, HTTP-2, ISO-8601
				if !isCommonFalsePositive(key) {
					ticketSet[key] = struct{}{}
				}
			}
		}

		// 2. Detect GitHub issue numbers
		for _, m := range githubIssueRegex.FindAllStringSubmatch(text, -1) {
			if len(m) > 1 {
				if num, err := strconv.Atoi(m[1]); err == nil && num > 0 {
					issueSet[num] = struct{}{}
				}
			}
		}

		// 3. Detect Markdown documentation paths
		for _, m := range docPathRegex.FindAllStringSubmatch(text, -1) {
			if len(m) > 1 {
				docSet[strings.TrimSpace(m[1])] = struct{}{}
			}
		}

		// 4. Detect Git commit SHAs
		for _, m := range gitSHARegex.FindAllStringSubmatch(text, -1) {
			if len(m) > 1 {
				sha := strings.ToLower(strings.TrimSpace(m[1]))
				// Avoid all-numeric strings that are likely numbers/timestamps
				if !isAllDigits(sha) {
					shaSet[sha] = struct{}{}
				}
			}
		}
	}

	tickets := make([]string, 0, len(ticketSet))
	for k := range ticketSet {
		tickets = append(tickets, k)
	}
	sort.Strings(tickets)

	issues := make([]int, 0, len(issueSet))
	for num := range issueSet {
		issues = append(issues, num)
	}
	sort.Ints(issues)

	shas := make([]string, 0, len(shaSet))
	for s := range shaSet {
		shas = append(shas, s)
	}
	sort.Strings(shas)

	docs := make([]string, 0, len(docSet))
	for p := range docSet {
		docs = append(docs, p)
	}
	sort.Strings(docs)

	return DetectedReferences{
		TicketKeys:   tickets,
		IssueNumbers: issues,
		CommitSHAs:   shas,
		DocPaths:     docs,
	}
}

func isCommonFalsePositive(key string) bool {
	upper := strings.ToUpper(key)
	switch {
	case strings.HasPrefix(upper, "UTF-"):
		return true
	case strings.HasPrefix(upper, "SHA-"):
		return true
	case strings.HasPrefix(upper, "HTTP-"):
		return true
	case strings.HasPrefix(upper, "ISO-"):
		return true
	case strings.HasPrefix(upper, "RFC-"):
		return true
	case strings.HasPrefix(upper, "TLS-"):
		return true
	case strings.HasPrefix(upper, "SSL-"):
		return true
	case strings.HasPrefix(upper, "IPV-"):
		return true
	case strings.HasPrefix(upper, "AMD-") || strings.HasPrefix(upper, "ARM-") || strings.HasPrefix(upper, "X86-"):
		return true
	}
	return false
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package trace

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
)

var (
	adrTitleRegex  = regexp.MustCompile(`(?i)^#\s*(?:(?:ADR[-_]?\s*(\d+))|(\d+))?[:\s]*(.*)$`)
	statusRegex    = regexp.MustCompile(`(?i)(?:status|state):\s*(accepted|proposed|deprecated|superseded)`)
	scopeRegex     = regexp.MustCompile(`(?i)(?:scope|applies_to|paths):\s*(.*)`)
	supersedesRegex = regexp.MustCompile(`(?i)supersedes:\s*([a-zA-Z0-9_\-]+)`)
)

// InRepoADRParser parses in-repo ADR markdown and JSON files into TraceDecisions.
type InRepoADRParser struct{}

// NewInRepoADRParser constructs a new in-repo ADR parser.
func NewInRepoADRParser() *InRepoADRParser {
	return &InRepoADRParser{}
}

// ScanPatchesForADRs scans pull request diff patches for newly added or modified ADR files.
func (p *InRepoADRParser) ScanPatchesForADRs(patches []*diff.FilePatch, orgID, repoID string) []*TraceDecision {
	var results []*TraceDecision

	for _, patch := range patches {
		path := patch.NewPath
		if path == "" {
			path = patch.OldPath
		}
		if !isADRDocumentPath(path) {
			continue
		}

		var sb strings.Builder
		for _, hunk := range patch.Hunks {
			for _, line := range hunk.Lines {
				if line.Type != diff.LineDeletion {
					sb.WriteString(line.Content)
					sb.WriteString("\n")
				}
			}
		}

		content := sb.String()
		if strings.TrimSpace(content) == "" {
			continue
		}

		cleanPath := filepathToSlash(path)
		if strings.HasSuffix(strings.ToLower(cleanPath), ".json") {
			decisions, err := p.parseJSONADR(content, cleanPath, orgID, repoID)
			if err == nil {
				results = append(results, decisions...)
			}
		} else {
			decision, err := p.parseMarkdownADR(content, cleanPath, orgID, repoID)
			if err == nil && decision != nil {
				results = append(results, decision)
			}
		}
	}

	return results
}

func (p *InRepoADRParser) parseJSONADR(raw, path, orgID, repoID string) ([]*TraceDecision, error) {
	// First try array
	var list []*TraceDecision
	if err := json.Unmarshal([]byte(raw), &list); err == nil {
		for _, d := range list {
			d.OrgID = orgID
			d.RepoID = repoID
			d.SourceFilePath = path
			d.Origin = OriginInRepoADR
			if d.ID == uuid.Nil {
				d.ID = uuid.New()
			}
		}
		return list, nil
	}

	// Try single object
	var single TraceDecision
	if err := json.Unmarshal([]byte(raw), &single); err != nil {
		return nil, err
	}

	single.OrgID = orgID
	single.RepoID = repoID
	single.SourceFilePath = path
	single.Origin = OriginInRepoADR
	if single.ID == uuid.Nil {
		single.ID = uuid.New()
	}
	return []*TraceDecision{&single}, nil
}

func (p *InRepoADRParser) parseMarkdownADR(content, path, orgID, repoID string) (*TraceDecision, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return nil, nil
	}

	baseName := filepath.Base(path)
	decisionKey := strings.TrimSuffix(baseName, filepath.Ext(baseName))

	title := decisionKey
	status := StatusAccepted
	decision := ""
	rationale := ""
	var scope []string
	supersededBy := ""

	currentSection := ""
	var sectionBuffer strings.Builder

	flushSection := func() {
		text := strings.TrimSpace(sectionBuffer.String())
		switch currentSection {
		case "context":
			rationale = text
		case "decision":
			decision = text
		case "consequences":
			if rationale != "" {
				rationale = rationale + "\n\n*Consequences*: " + text
			} else {
				rationale = text
			}
		}
		sectionBuffer.Reset()
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Title extraction
		if strings.HasPrefix(trimmed, "# ") && title == decisionKey {
			m := adrTitleRegex.FindStringSubmatch(trimmed)
			if len(m) > 0 {
				extractedTitle := strings.TrimSpace(m[len(m)-1])
				if extractedTitle != "" {
					title = extractedTitle
				}
			}
			continue
		}

		// Section headers
		if strings.HasPrefix(trimmed, "## ") {
			flushSection()
			header := strings.ToLower(strings.TrimPrefix(trimmed, "## "))
			switch {
			case strings.HasPrefix(header, "status"):
				currentSection = "status"
				if sm := statusRegex.FindStringSubmatch(header); len(sm) > 1 {
					status = DecisionStatus(strings.ToLower(sm[1]))
				}
			case strings.HasPrefix(header, "scope") || strings.HasPrefix(header, "applies_to") || strings.HasPrefix(header, "paths"):
				currentSection = "scope"
				if scm := scopeRegex.FindStringSubmatch(trimmed); len(scm) > 1 {
					parts := strings.Split(scm[1], ",")
					for _, pt := range parts {
						cleanPt := strings.Trim(strings.TrimSpace(pt), "`\"'")
						if cleanPt != "" {
							scope = append(scope, cleanPt)
						}
					}
				}
			case strings.HasPrefix(header, "context"):
				currentSection = "context"
			case strings.HasPrefix(header, "decision"):
				currentSection = "decision"
			case strings.HasPrefix(header, "consequences"):
				currentSection = "consequences"
			default:
				currentSection = header
			}
			continue
		}

		// Inline metadata lines
		if sm := statusRegex.FindStringSubmatch(trimmed); len(sm) > 1 {
			status = DecisionStatus(strings.ToLower(sm[1]))
			continue
		}
		if scm := scopeRegex.FindStringSubmatch(trimmed); len(scm) > 1 {
			rawScope := scm[1]
			parts := strings.Split(rawScope, ",")
			for _, pt := range parts {
				cleanPt := strings.Trim(strings.TrimSpace(pt), "`\"'")
				if cleanPt != "" {
					scope = append(scope, cleanPt)
				}
			}
			continue
		}
		if sup := supersedesRegex.FindStringSubmatch(trimmed); len(sup) > 1 {
			supersededBy = strings.ToUpper(sup[1])
			continue
		}

		sectionBuffer.WriteString(line)
		sectionBuffer.WriteString("\n")
	}

	flushSection()

	if decision == "" {
		decision = title
	}

	return &TraceDecision{
		ID:             uuid.New(),
		OrgID:          orgID,
		RepoID:         repoID,
		DecisionKey:    decisionKey,
		Title:          title,
		Decision:       decision,
		Rationale:      rationale,
		Type:           DecisionArchitectural,
		Status:         status,
		Origin:         OriginInRepoADR,
		Scope:          scope,
		SupersededBy:   supersededBy,
		SourceFilePath: path,
		Confidence:     1.0,
		Pinned:         true,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}, nil
}

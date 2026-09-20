// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package trace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

// ADRViolationFinding records an identified conflict between a code change and an Architectural Decision Record.
type ADRViolationFinding struct {
	DecisionKey string                 `json:"decision_key"`
	DecisionTitle string               `json:"decision_title"`
	FilePath    string                 `json:"file_path"`
	LineNumber  int                    `json:"line_number"`
	Violation   string                 `json:"violation"`
	Severity    models.FindingSeverity `json:"severity"`
	Remediation string                 `json:"remediation"`
}

// ADRInvalidationReport summarizes architectural audit results for a pull request.
type ADRInvalidationReport struct {
	EvaluatedDecisionsCount int                    `json:"evaluated_decisions_count"`
	Violations              []ADRViolationFinding  `json:"violations"`
	SupersededDecisions     []string               `json:"superseded_decisions"`
	Findings                []models.CodeFinding   `json:"findings"`
}

// TraceInvalidator evaluates PR diffs against architectural constraints and manages supersession lifecycles.
type TraceInvalidator struct {
	store ITraceStore
}

// NewTraceInvalidator constructs a new invalidator.
func NewTraceInvalidator(store ITraceStore) *TraceInvalidator {
	return &TraceInvalidator{
		store: store,
	}
}

// AuditPullRequest inspects pull request diffs for potential ADR violations and supersessions.
func (inv *TraceInvalidator) AuditPullRequest(
	ctx context.Context,
	reviewID uuid.UUID,
	workspaceID uuid.UUID,
	orgID, repoID string,
	patches []*diff.FilePatch,
) (*ADRInvalidationReport, error) {
	report := &ADRInvalidationReport{}

	if inv.store == nil || len(patches) == 0 {
		return report, nil
	}

	decisions, err := inv.store.ListDecisionsForRepo(ctx, orgID, repoID)
	if err != nil {
		return report, err
	}

	report.EvaluatedDecisionsCount = len(decisions)
	if len(decisions) == 0 {
		return report, nil
	}

	// 1. Detect In-PR supersessions or deprecations from ADR files
	for _, patch := range patches {
		path := patch.NewPath
		if path == "" {
			path = patch.OldPath
		}
		if isADRDocumentPath(path) {
			inv.detectADRSupersessionFromPatch(ctx, orgID, repoID, patch, report)
		}
	}

	// 2. Detect code modifications violating architectural decision scope and constraints
	for _, d := range decisions {
		if !d.IsActive() {
			continue
		}

		for _, patch := range patches {
			path := patch.NewPath
			if path == "" {
				path = patch.OldPath
			}
			if !d.MatchesPath(path) {
				continue
			}

			// Check added lines for explicit anti-pattern triggers mentioned in the decision
			for _, hunk := range patch.Hunks {
				for _, line := range hunk.Lines {
					if line.Type != diff.LineAddition {
						continue
					}

					violationMsg, isViolated := evaluateDecisionConflict(d, line.Content)
					if isViolated {
						lineNo := line.NewLineNo
						if lineNo <= 0 {
							lineNo = 1
						}

						hash := sha256.Sum256([]byte(fmt.Sprintf("adr:%s:%s:%d", d.DecisionKey, path, lineNo)))
						fp := hex.EncodeToString(hash[:16])

						vf := ADRViolationFinding{
							DecisionKey:   d.DecisionKey,
							DecisionTitle: d.Title,
							FilePath:      path,
							LineNumber:    lineNo,
							Violation:     violationMsg,
							Severity:      models.SeverityHigh,
							Remediation:   fmt.Sprintf("Reconcile with Architectural Decision [%s: %s]: %s", d.DecisionKey, d.Title, d.Decision),
						}
						report.Violations = append(report.Violations, vf)

						finding := models.CodeFinding{
							ID:            uuid.New(),
							ReviewID:      reviewID,
							WorkspaceID:   workspaceID,
							FilePath:      path,
							StartLine:     lineNo,
							EndLine:       lineNo,
							Severity:      models.SeverityHigh,
							Category:      "architecture",
							Title:         fmt.Sprintf("Architecture Invariant Violation [%s]", d.DecisionKey),
							Description:   fmt.Sprintf("**Decision**: %s\n\n**Violation**: %s\n\n*Rationale*: %s", d.Decision, violationMsg, d.Rationale),
							Remediation:   vf.Remediation,
							SuggestedDiff: vf.Remediation,
							Fingerprint:   fp,
							CreatedAt:     time.Now().UTC(),
						}
						report.Findings = append(report.Findings, finding)
					}
				}
			}
		}
	}

	return report, nil
}

func isADRDocumentPath(path string) bool {
	clean := strings.ToLower(filepathToSlash(path))
	return strings.HasPrefix(clean, "docs/adr/") || strings.HasPrefix(clean, ".scandrix/trace/") || strings.Contains(clean, "/adr/")
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

func (inv *TraceInvalidator) detectADRSupersessionFromPatch(
	ctx context.Context,
	orgID, repoID string,
	patch *diff.FilePatch,
	report *ADRInvalidationReport,
) {
	for _, hunk := range patch.Hunks {
		for _, line := range hunk.Lines {
			if line.Type == diff.LineAddition {
				content := strings.ToLower(line.Content)
				// Look for `supersedes: ADR-0001` or `superseded by ADR-0002`
				if strings.Contains(content, "supersedes") || strings.Contains(content, "superseded by") {
					parts := strings.Fields(content)
					for i, p := range parts {
						if (p == "supersedes" || p == "supersedes:") && i+1 < len(parts) {
							targetKey := strings.Trim(parts[i+1], "[],`\":.")
							if targetKey != "" {
								targetKey = strings.ToUpper(targetKey)
								_ = inv.MarkSuperseded(ctx, orgID, repoID, targetKey, patch.NewPath)
								report.SupersededDecisions = append(report.SupersededDecisions, targetKey)
							}
						}
					}
				}
			}
		}
	}
}

// MarkSuperseded marks a decision as superseded by a newer decision.
func (inv *TraceInvalidator) MarkSuperseded(ctx context.Context, orgID, repoID, decisionKey, supersedingKey string) error {
	d, err := inv.store.GetDecision(ctx, orgID, repoID, decisionKey)
	if err != nil {
		return err
	}

	d.Status = StatusSuperseded
	d.SupersededBy = supersedingKey
	return inv.store.SaveDecision(ctx, d)
}

func evaluateDecisionConflict(d *TraceDecision, lineContent string) (string, bool) {
	lowerLine := strings.ToLower(lineContent)
	lowerDecision := strings.ToLower(d.Decision)

	// Heuristic 1: If decision prohibits raw SQL and line uses string formatting in query
	if (strings.Contains(lowerDecision, "parameterized") || strings.Contains(lowerDecision, "sql")) && (strings.Contains(lowerDecision, "concat") || strings.Contains(lowerDecision, "raw sql") || strings.Contains(lowerDecision, "injection")) {
		if strings.Contains(lowerLine, "fmt.sprintf") && (strings.Contains(lowerLine, "select") || strings.Contains(lowerLine, "insert") || strings.Contains(lowerLine, "update") || strings.Contains(lowerLine, "delete")) {
			return "Raw SQL string concatenation detected in scope governed by parameterized query ADR", true
		}
	}

	// Heuristic 2: If decision mandates pgx/sqlx and line introduces gorm or raw database/sql
	if strings.Contains(lowerDecision, "pgx") && strings.Contains(lowerDecision, "without gorm") {
		if strings.Contains(lowerLine, "gorm.io/gorm") || strings.Contains(lowerLine, "gorm.open") {
			return "GORM invocation detected in scope explicitly restricted to native pgx driver", true
		}
	}

	// Heuristic 3: If decision mandates strict timeout bounds
	if strings.Contains(lowerDecision, "bounded timeout") || strings.Contains(lowerDecision, "context with timeout") {
		if strings.Contains(lowerLine, "http.get(") || strings.Contains(lowerLine, "http.post(") {
			return "Default HTTP client without context timeout used in bounded timeout scope", true
		}
	}

	return "", false
}

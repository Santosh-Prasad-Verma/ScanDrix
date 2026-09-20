// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package rulesengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

// AtomEvaluationReport records outcome of atomic rule evaluation across diff hunks.
type AtomEvaluationReport struct {
	TotalAtomsEvaluated   int                 `json:"total_atoms_evaluated"`
	MechanicalViolations  int                 `json:"mechanical_violations"`
	SemanticPendingAtoms  int                 `json:"semantic_pending_atoms"`
	ComplianceScore       float64             `json:"compliance_score"` // 0.0 (all failed) to 1.0 (all passed)
	Findings              []models.CodeFinding `json:"findings"`
	ViolatedAtomIDs       []string            `json:"violated_atom_ids"`
}

// AtomEvaluator executes atomic invariants against pull request diffs.
type AtomEvaluator struct{}

// NewAtomEvaluator constructs an atom evaluator.
func NewAtomEvaluator() *AtomEvaluator {
	return &AtomEvaluator{}
}

// EvaluateAtoms checks mechanical atoms against diff line additions.
func (e *AtomEvaluator) EvaluateAtoms(
	ctx context.Context,
	reviewID uuid.UUID,
	workspaceID uuid.UUID,
	atoms *DrixyRuleAtoms,
	patches []*diff.FilePatch,
) *AtomEvaluationReport {
	report := &AtomEvaluationReport{
		ComplianceScore: 1.0,
	}

	if atoms == nil || len(atoms.Items) == 0 || len(patches) == 0 {
		return report
	}

	report.TotalAtomsEvaluated = len(atoms.Items)
	violatedAtoms := make(map[string]bool)

	for _, atom := range atoms.Items {
		// If semantic (no detector), tally as pending deliberation
		if atom.Detector == nil || atom.Detector.Pattern == "" {
			report.SemanticPendingAtoms++
			continue
		}

		if atom.Detector.CompiledRegex == nil {
			if err := CompileRuleDetector(atom.Detector); err != nil {
				report.SemanticPendingAtoms++
				continue
			}
		}

		// Evaluate mechanical regex against added lines in matching patches
		for _, patch := range patches {
			targetPath := patch.NewPath
			if targetPath == "" {
				targetPath = patch.OldPath
			}
			if targetPath == "" {
				continue
			}

			// Path glob filter check
			if len(atom.PathGlobs) > 0 {
				matched := false
				for _, glob := range atom.PathGlobs {
					if matchPathGlob(glob, targetPath) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}

			for _, hunk := range patch.Hunks {
				for _, line := range hunk.Lines {
					if line.Type != diff.LineAddition {
						continue
					}

					content := line.Content
					if atom.Detector.CompiledRegex.MatchString(content) {
						if atom.Detector.CompiledNegative != nil && atom.Detector.CompiledNegative.MatchString(content) {
							// Negative pattern matched -> skip false positive
							continue
						}

						lineNo := line.NewLineNo
						if lineNo <= 0 {
							lineNo = 1
						}

						hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", atom.ID, targetPath, lineNo)))
						fp := hex.EncodeToString(hash[:16])

						desc := atom.Spec
						if atom.Detector.Reason != "" {
							desc = fmt.Sprintf("%s\n\n*Violation Detected*: %s", desc, atom.Detector.Reason)
						}
						desc = fmt.Sprintf("%s\n\n*(Governed by rule requirement: %s)*", desc, atom.Title)

						remediation := atom.Remediation
						if remediation == "" {
							remediation = fmt.Sprintf("Refactor code to conform with %s", atom.Title)
						}

						finding := models.CodeFinding{
							ID:            uuid.New(),
							ReviewID:      reviewID,
							WorkspaceID:   workspaceID,
							FilePath:      targetPath,
							StartLine:     lineNo,
							EndLine:       lineNo,
							Severity:      atom.Severity,
							Category:      atom.Category,
							Title:         atom.Title,
							Description:   desc,
							Remediation:   remediation,
							SuggestedDiff: remediation,
							Fingerprint:   fp,
							CreatedAt:     time.Now().UTC(),
						}

						report.Findings = append(report.Findings, finding)
						report.MechanicalViolations++
						violatedAtoms[atom.ID] = true
					}
				}
			}
		}
	}

	for id := range violatedAtoms {
		report.ViolatedAtomIDs = append(report.ViolatedAtomIDs, id)
	}

	if report.TotalAtomsEvaluated > 0 {
		passedAtoms := report.TotalAtomsEvaluated - len(violatedAtoms)
		report.ComplianceScore = float64(passedAtoms) / float64(report.TotalAtomsEvaluated)
	}

	return report
}

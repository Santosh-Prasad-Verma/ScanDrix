// Package orchestrator coordinates specialized review agents and synthesizes multi-perspective findings.
package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"github.com/scandrix/backend/pkg/models"
)

// ArbiterVerdict represents the result of the Skeptical Arbiter evaluation.
type ArbiterVerdict string

const (
	ArbiterConfirmed  ArbiterVerdict = "CONFIRMED"
	ArbiterDowngraded ArbiterVerdict = "DOWNGRADED"
	ArbiterEnhanced   ArbiterVerdict = "ENHANCED"
	ArbiterDisputed   ArbiterVerdict = "DISPUTED"
)

// ArbiterEvaluation models the arbiter's judgment on a single finding.
type ArbiterEvaluation struct {
	FindingID        string                 `json:"finding_id"`
	Verdict          ArbiterVerdict         `json:"verdict"`
	AdjustedSeverity models.FindingSeverity `json:"adjusted_severity,omitempty"`
	AdjustedBlocking *bool                  `json:"adjusted_blocking,omitempty"`
	Reasoning        string                 `json:"reasoning"`
}

// SkepticalArbiter cross-examines findings to eliminate false positives and calibrate severities.
type SkepticalArbiter interface {
	EvaluateFindings(ctx context.Context, findings []AgentFinding, changedFiles []ChangedFile) ([]ArbiterEvaluation, error)
}

// HeuristicSkepticalArbiter evaluates findings deterministically using AST, diff boundaries,
// and anti-speculation heuristics to refute false positives without additional LLM latency.
type HeuristicSkepticalArbiter struct {
	speculativeRegex *regexp.Regexp
	paramQueryRegex  *regexp.Regexp
	nilCheckRegex    *regexp.Regexp
}

// NewHeuristicSkepticalArbiter constructs a concrete deterministic arbiter.
func NewHeuristicSkepticalArbiter() *HeuristicSkepticalArbiter {
	return &HeuristicSkepticalArbiter{
		speculativeRegex: regexp.MustCompile(`(?i)\b(might want to consider|could potentially|in case you ever|just a thought|not strictly necessary|may want to verify if|consider checking whether|nitpick|cosmetic)\b`),
		paramQueryRegex:  regexp.MustCompile(`(\$\d+|\?|:[a-zA-Z_]\w*|@p\d+)`),
		nilCheckRegex:    regexp.MustCompile(`(?i)(if\s+[a-zA-Z0-9_.]+\s*==\s*nil|if\s+err\s*!=\s*nil|return|guard\s+let)`),
	}
}

// EvaluateFindings runs heuristic cross-examination across all findings.
func (h *HeuristicSkepticalArbiter) EvaluateFindings(
	ctx context.Context,
	findings []AgentFinding,
	changedFiles []ChangedFile,
) ([]ArbiterEvaluation, error) {
	fileMap := make(map[string]ChangedFile, len(changedFiles))
	for _, f := range changedFiles {
		clean := strings.TrimPrefix(strings.TrimPrefix(f.Filename, "./"), "/")
		fileMap[clean] = f
	}

	evals := make([]ArbiterEvaluation, 0, len(findings))
	blockingFalse := false
	blockingTrue := true

	for _, f := range findings {
		fID := f.ID.String()

		// 1. If finding is tied to an explicit custom DrixyRule, respect the configured rule definition.
		if f.RuleID != nil {
			evals = append(evals, ArbiterEvaluation{
				FindingID: fID,
				Verdict:   ArbiterConfirmed,
				Reasoning: "Validated by custom Drixy rule definition",
			})
			continue
		}

		// 2. Hallucination check: missing file or completely empty core text
		cleanPath := strings.TrimPrefix(strings.TrimPrefix(f.FilePath, "./"), "/")
		cf, fileExists := fileMap[cleanPath]
		if !fileExists && len(fileMap) > 0 {
			evals = append(evals, ArbiterEvaluation{
				FindingID: fID,
				Verdict:   ArbiterDisputed,
				Reasoning: fmt.Sprintf("Referenced file %s not found in PR changed files", f.FilePath),
			})
			continue
		}

		if strings.TrimSpace(f.Title) == "" || strings.TrimSpace(f.Description) == "" {
			evals = append(evals, ArbiterEvaluation{
				FindingID: fID,
				Verdict:   ArbiterDisputed,
				Reasoning: "Discarded non-actionable finding with empty title or description",
			})
			continue
		}

		// 3. Multi-agent consensus enhancement: if 2 or more distinct agents flagged this issue
		if len(f.ContributingAgents) >= 2 {
			evals = append(evals, ArbiterEvaluation{
				FindingID:        fID,
				Verdict:          ArbiterEnhanced,
				AdjustedBlocking: &blockingTrue,
				Reasoning:        fmt.Sprintf("Confirmed by multi-agent consensus (%s)", strings.Join(f.ContributingAgents, ", ")),
			})
			continue
		}

		// 4. False-Positive SQL Injection filter: if flagged as SQL injection but query uses parameterized placeholders
		if strings.EqualFold(f.Category, "security") || strings.Contains(strings.ToLower(f.Title), "sql injection") {
			if f.ExistingCode != "" && h.paramQueryRegex.MatchString(f.ExistingCode) {
				evals = append(evals, ArbiterEvaluation{
					FindingID: fID,
					Verdict:   ArbiterDisputed,
					Reasoning: "SQL injection refutation: code snippet uses parameterized query placeholders",
				})
				continue
			}
		}

		// 5. Speculative tone check: downgrade speculative or cosmetic suggestions masquerading as Critical/High
		textToExamine := f.Title + " " + f.Description
		if h.speculativeRegex.MatchString(textToExamine) {
			if f.Severity == models.SeverityCritical || f.Severity == models.SeverityHigh {
				evals = append(evals, ArbiterEvaluation{
					FindingID:        fID,
					Verdict:          ArbiterDowngraded,
					AdjustedSeverity: models.SeverityLow,
					AdjustedBlocking: &blockingFalse,
					Reasoning:        "Speculative/advisory wording detected; downgraded from blocking severity",
				})
				continue
			}
			if f.Severity == models.SeverityMedium {
				evals = append(evals, ArbiterEvaluation{
					FindingID:        fID,
					Verdict:          ArbiterDowngraded,
					AdjustedSeverity: models.SeverityInfo,
					AdjustedBlocking: &blockingFalse,
					Reasoning:        "Speculative/advisory wording detected; downgraded to info",
				})
				continue
			}
		}

		// 6. Check if code already contains defensive guard statements in the patch hunk
		if cf.Patch != "" && (strings.Contains(strings.ToLower(f.Title), "nil dereference") || strings.Contains(strings.ToLower(f.Title), "null pointer")) {
			if f.ExistingCode != "" && h.nilCheckRegex.MatchString(f.ExistingCode) {
				evals = append(evals, ArbiterEvaluation{
					FindingID: fID,
					Verdict:   ArbiterDisputed,
					Reasoning: "Nil dereference refutation: defensive guard check already present in context",
				})
				continue
			}
		}

		// 7. Verified Committable Diff bonus: if suggested diff is concrete, confirm
		if f.SuggestedDiff != "" && strings.Contains(f.SuggestedDiff, "@@") {
			evals = append(evals, ArbiterEvaluation{
				FindingID: fID,
				Verdict:   ArbiterConfirmed,
				Reasoning: "Verified actionable suggestion with committable unified diff",
			})
			continue
		}

		// Default: Confirmed
		evals = append(evals, ArbiterEvaluation{
			FindingID: fID,
			Verdict:   ArbiterConfirmed,
			Reasoning: "Verified under standard heuristic checks",
		})
	}

	return evals, nil
}

// ComputeFindingFingerprint calculates a deterministic SHA-256 fingerprint for grouping.
// Buckets line numbers into 5-line windows to align near-duplicate findings on adjacent lines.
func ComputeFindingFingerprint(filePath string, startLine, endLine int, category, title string) string {
	cleanPath := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(filePath, "./"), "/"))
	normCategory := strings.ToLower(strings.TrimSpace(category))
	normTitle := strings.ToLower(strings.Join(strings.Fields(title), " "))

	// Bucket line numbers in 5-line increments (e.g. lines 12-14 and 10-15 share bucket 10)
	bucketStart := (startLine / 5) * 5
	bucketEnd := ((endLine + 4) / 5) * 5

	raw := fmt.Sprintf("%s:%d-%d:%s:%s", cleanPath, bucketStart, bucketEnd, normCategory, normTitle)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:16])
}

// SpecialistDomainAuthority returns an authority multiplier for agent personas over given categories.
func SpecialistDomainAuthority(agentName, category string) float64 {
	agent := strings.ToLower(agentName)
	cat := strings.ToLower(category)

	switch agent {
	case "security":
		if strings.Contains(cat, "security") || strings.Contains(cat, "vulnerability") ||
			strings.Contains(cat, "auth") || strings.Contains(cat, "injection") || strings.Contains(cat, "crypto") {
			return 1.6
		}
	case "drixy_rules":
		if strings.Contains(cat, "rule") || strings.Contains(cat, "convention") || strings.Contains(cat, "standard") {
			return 1.6
		}
	case "performance":
		if strings.Contains(cat, "performance") || strings.Contains(cat, "memory") ||
			strings.Contains(cat, "complexity") || strings.Contains(cat, "database") || strings.Contains(cat, "allocation") {
			return 1.5
		}
	case "bug":
		if strings.Contains(cat, "correctness") || strings.Contains(cat, "bug") ||
			strings.Contains(cat, "logic") || strings.Contains(cat, "nil") || strings.Contains(cat, "race") {
			return 1.5
		}
	case "architecture":
		if strings.Contains(cat, "architecture") || strings.Contains(cat, "coupling") ||
			strings.Contains(cat, "modularity") || strings.Contains(cat, "design") {
			return 1.4
		}
	case "business_logic":
		if strings.Contains(cat, "business") || strings.Contains(cat, "domain") || strings.Contains(cat, "workflow") {
			return 1.5
		}
	}
	return 1.0
}

// ReconcileFindings reconciles multi-agent outputs, resolves fingerprint collisions,
// merges contributing agent attributions, and arbitrates conflicting findings.
func ReconcileFindings(findings []AgentFinding) []AgentFinding {
	if len(findings) <= 1 {
		for i := range findings {
			if findings[i].Fingerprint == "" {
				findings[i].Fingerprint = ComputeFindingFingerprint(
					findings[i].FilePath, findings[i].StartLine, findings[i].EndLine,
					findings[i].Category, findings[i].Title,
				)
			}
			if len(findings[i].ContributingAgents) == 0 && findings[i].AgentName != "" {
				findings[i].ContributingAgents = []string{findings[i].AgentName}
			}
		}
		return findings
	}

	// Group findings by file and fingerprint
	type cluster struct {
		findings []AgentFinding
	}

	clusters := make([]*cluster, 0)
	clusterIndex := make(map[string]*cluster)

	for _, f := range findings {
		if f.Fingerprint == "" {
			f.Fingerprint = ComputeFindingFingerprint(f.FilePath, f.StartLine, f.EndLine, f.Category, f.Title)
		}
		if len(f.ContributingAgents) == 0 && f.AgentName != "" {
			f.ContributingAgents = []string{f.AgentName}
		}

		cleanPath := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(f.FilePath, "./"), "/"))
		key := fmt.Sprintf("%s:%s", cleanPath, f.Fingerprint)

		// Check exact fingerprint match
		if c, ok := clusterIndex[key]; ok {
			c.findings = append(c.findings, f)
			continue
		}

		// Check line-range overlap (+/- 3 lines in same file)
		matched := false
		for _, c := range clusters {
			if len(c.findings) == 0 {
				continue
			}
			primary := c.findings[0]
			cClean := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(primary.FilePath, "./"), "/"))
			if cClean == cleanPath {
				// Check line span overlap
				startOverlap := max(1, primary.StartLine-3)
				endOverlap := primary.EndLine + 3
				if f.StartLine <= endOverlap && f.EndLine >= startOverlap {
					c.findings = append(c.findings, f)
					matched = true
					break
				}
			}
		}

		if !matched {
			newC := &cluster{findings: []AgentFinding{f}}
			clusters = append(clusters, newC)
			clusterIndex[key] = newC
		}
	}

	reconciled := make([]AgentFinding, 0, len(clusters))
	for _, c := range clusters {
		if len(c.findings) == 1 {
			reconciled = append(reconciled, c.findings[0])
			continue
		}

		// Resolve multi-agent collision
		primary := c.findings[0]
		primaryScore := float64(severityWeight(primary.Severity))*SpecialistDomainAuthority(primary.AgentName, primary.Category) +
			float64(confidenceWeight(primary.Confidence))

		agentSet := make(map[string]struct{})
		for _, a := range primary.ContributingAgents {
			agentSet[a] = struct{}{}
		}
		anyBlocking := primary.Blocking

		for _, other := range c.findings[1:] {
			for _, a := range other.ContributingAgents {
				agentSet[a] = struct{}{}
			}
			if other.Blocking {
				anyBlocking = true
			}

			otherScore := float64(severityWeight(other.Severity))*SpecialistDomainAuthority(other.AgentName, other.Category) +
				float64(confidenceWeight(other.Confidence))

			if otherScore > primaryScore {
				// Other specialist has stronger domain authority or severity
				if primary.SuggestedDiff != "" && other.SuggestedDiff == "" {
					other.SuggestedDiff = primary.SuggestedDiff
				}
				primary = other
				primaryScore = otherScore
			} else {
				if primary.SuggestedDiff == "" && other.SuggestedDiff != "" {
					primary.SuggestedDiff = other.SuggestedDiff
				}
			}
		}

		// Populate merged contributing agents
		contributors := make([]string, 0, len(agentSet))
		for a := range agentSet {
			contributors = append(contributors, a)
		}
		sort.Strings(contributors)
		primary.ContributingAgents = contributors
		primary.Blocking = anyBlocking

		// Multi-agent consensus elevates confidence to HIGH
		if len(contributors) >= 2 {
			primary.Confidence = "HIGH"
			primary.DisputeStatus = "RESOLVED"
		}

		reconciled = append(reconciled, primary)
	}

	return reconciled
}

// DeliberationSynthesizer consolidates, arbitrates, prioritizes, and decides final PR verdicts.
type DeliberationSynthesizer struct {
	arbiter   SkepticalArbiter
	deduper   *SemanticDeduplicator
	validator *DiffBoundaryValidator
}

// NewDeliberationSynthesizer constructs a deliberation synthesizer.
func NewDeliberationSynthesizer(arbiter SkepticalArbiter, deduper *SemanticDeduplicator, validator *DiffBoundaryValidator) *DeliberationSynthesizer {
	if arbiter == nil {
		arbiter = NewHeuristicSkepticalArbiter()
	}
	if deduper == nil {
		deduper = NewSemanticDeduplicator(nil)
	}
	if validator == nil {
		validator = NewDiffBoundaryValidator()
	}
	return &DeliberationSynthesizer{
		arbiter:   arbiter,
		deduper:   deduper,
		validator: validator,
	}
}

// SynthesizeReview executes the full deliberation pipeline from raw agent outputs to final review.
func (s *DeliberationSynthesizer) SynthesizeReview(
	ctx context.Context,
	input ReviewAgentInput,
	agentOutputs []ReviewAgentOutput,
) (*OrchestratorOutput, error) {
	// 1. Aggregate all findings from all specialist outputs
	var rawFindings []AgentFinding
	personaSet := make(map[string]struct{})
	totalTokens := 0
	var allWarnings []ReviewWarning

	for _, out := range agentOutputs {
		if out.AgentName != "" {
			personaSet[out.AgentName] = struct{}{}
		}
		rawFindings = append(rawFindings, out.Findings...)
		allWarnings = append(allWarnings, out.Warnings...)
		totalTokens += out.TokensConsumed
	}

	preSynthesisCount := len(rawFindings)

	// 2. Validate against diff boundaries (clip/discard out-of-hunk line numbers)
	validFindings, _ := s.validator.FilterFindings(rawFindings, input.ChangedFiles)

	// 3. Multi-agent reconciliation and fingerprint collision merging
	reconciledFindings := ReconcileFindings(validFindings)

	// 4. Apply Skeptical Arbiter cross-examination
	var postArbiterFindings []AgentFinding
	removedByArbiter := 0
	downgradedCount := 0
	enhancedCount := 0

	if s.arbiter != nil && len(reconciledFindings) > 0 {
		evals, err := s.arbiter.EvaluateFindings(ctx, reconciledFindings, input.ChangedFiles)
		if err == nil {
			evalMap := make(map[string]ArbiterEvaluation, len(evals))
			for _, ev := range evals {
				evalMap[ev.FindingID] = ev
			}

			for _, f := range reconciledFindings {
				if ev, ok := evalMap[f.ID.String()]; ok {
					f.ArbiterVerdict = string(ev.Verdict)
					switch ev.Verdict {
					case ArbiterDisputed:
						removedByArbiter++
						continue // Dropped! False positive eliminated
					case ArbiterDowngraded:
						downgradedCount++
						if ev.AdjustedSeverity != "" {
							f.Severity = ev.AdjustedSeverity
						}
						if ev.AdjustedBlocking != nil {
							f.Blocking = *ev.AdjustedBlocking
						} else if f.Severity != models.SeverityCritical && f.Severity != models.SeverityHigh {
							f.Blocking = false
						}
					case ArbiterEnhanced:
						enhancedCount++
						if ev.AdjustedSeverity != "" {
							f.Severity = ev.AdjustedSeverity
						}
						if ev.AdjustedBlocking != nil {
							f.Blocking = *ev.AdjustedBlocking
						} else {
							f.Blocking = true
						}
					case ArbiterConfirmed:
						// Keep as-is
					}
				}
				postArbiterFindings = append(postArbiterFindings, f)
			}
		} else {
			postArbiterFindings = reconciledFindings
		}
	} else {
		postArbiterFindings = reconciledFindings
	}

	postArbiterCount := len(postArbiterFindings)

	// 5. Semantic Deduplication across personas
	dedupedFindings, _ := s.deduper.DeduplicateFindings(postArbiterFindings)
	postDedupCount := len(dedupedFindings)

	// 6. Final Quality Gate Filter
	var qualifiedFindings []AgentFinding
	removedByQualityGate := 0
	for _, f := range dedupedFindings {
		if strings.TrimSpace(f.Title) == "" || strings.TrimSpace(f.Description) == "" {
			removedByQualityGate++
			continue
		}
		// Discard INFO findings with LOW confidence
		if f.Severity == models.SeverityInfo && strings.EqualFold(f.Confidence, "LOW") {
			removedByQualityGate++
			continue
		}
		qualifiedFindings = append(qualifiedFindings, f)
	}

	// 7. Prioritize and Sort Findings
	sortFindings(qualifiedFindings)

	// 8. Determine Final Review Verdict
	verdict := VerdictApprove
	blockingCount := 0
	for _, f := range qualifiedFindings {
		if f.Blocking {
			blockingCount++
		}
	}

	if blockingCount > 0 {
		verdict = VerdictRequestChanges
	} else if len(qualifiedFindings) > 0 {
		verdict = VerdictCommentOnly
	}

	// Check FailOnSeverity threshold if configured
	if input.ReviewOptions.FailOnSeverity != "" {
		failWeight := severityWeight(input.ReviewOptions.FailOnSeverity)
		for _, f := range qualifiedFindings {
			if severityWeight(f.Severity) >= failWeight {
				verdict = VerdictRequestChanges
				break
			}
		}
	}

	// Build executive summary markdown
	personas := make([]string, 0, len(personaSet))
	for p := range personaSet {
		personas = append(personas, p)
	}
	sort.Strings(personas)

	delibMeta := DeliberationMetadata{
		PersonasConsulted:            personas,
		FindingsPreSynthesis:         preSynthesisCount,
		FindingsPostArbiter:          postArbiterCount,
		FindingsPostDedup:            postDedupCount,
		FindingsRemovedByArbiter:     removedByArbiter,
		FindingsDowngraded:           downgradedCount,
		FindingsEnhanced:             enhancedCount,
		FindingsRemovedByQualityGate: removedByQualityGate,
	}

	summary := s.buildExecutiveSummary(verdict, qualifiedFindings, delibMeta, input)

	return &OrchestratorOutput{
		Verdict:              verdict,
		Summary:              summary,
		Findings:             qualifiedFindings,
		AgentOutputs:         agentOutputs,
		Warnings:             dedupWarnings(allWarnings),
		DeliberationMetadata: delibMeta,
		TotalTokensConsumed:  totalTokens,
	}, nil
}

// sortFindings applies the 3-tier prioritization sort: Blocking -> Severity -> Confidence
func sortFindings(findings []AgentFinding) {
	sort.SliceStable(findings, func(i, j int) bool {
		// 1. Primary: Blocking findings first
		if findings[i].Blocking != findings[j].Blocking {
			return findings[i].Blocking
		}

		// 2. Secondary: Higher severity first
		wI := severityWeight(findings[i].Severity)
		wJ := severityWeight(findings[j].Severity)
		if wI != wJ {
			return wI > wJ
		}

		// 3. Tertiary: Higher confidence first
		cI := confidenceWeight(findings[i].Confidence)
		cJ := confidenceWeight(findings[j].Confidence)
		return cI > cJ
	})
}

func (s *DeliberationSynthesizer) buildExecutiveSummary(
	verdict ReviewVerdict,
	findings []AgentFinding,
	meta DeliberationMetadata,
	input ReviewAgentInput,
) string {
	var sb strings.Builder

	// Header & Verdict Badge
	switch verdict {
	case VerdictApprove:
		sb.WriteString("## 🚀 ScanDrix AI Review: APPROVED\n\n")
		sb.WriteString("No blocking security vulnerabilities, performance regressions, or custom rule violations were identified in this pull request.\n\n")
	case VerdictCommentOnly:
		sb.WriteString("## 💬 ScanDrix AI Review: COMMENT ONLY\n\n")
		sb.WriteString(fmt.Sprintf("ScanDrix identified **%d** non-blocking recommendation(s) and advisory improvement(s).\n\n", len(findings)))
	case VerdictRequestChanges:
		sb.WriteString("## 🛑 ScanDrix AI Review: CHANGES REQUESTED\n\n")
		sb.WriteString(fmt.Sprintf("ScanDrix identified **%d** critical issue(s) that require attention before merging.\n\n", len(findings)))
	}

	// Deliberation Metrics Bar
	sb.WriteString("### 🧠 Multi-Agent Deliberation Summary\n")
	sb.WriteString(fmt.Sprintf("- **Personas Consulted**: `%s`\n", strings.Join(meta.PersonasConsulted, "`, `")))
	sb.WriteString(fmt.Sprintf("- **Raw Findings**: %d initial -> %d after Skeptical Arbiter (%d disputed false positives removed) -> **%d final** verified findings.\n\n",
		meta.FindingsPreSynthesis, meta.FindingsPostArbiter, meta.FindingsRemovedByArbiter, len(findings)))

	if len(findings) > 0 {
		sb.WriteString("### 📋 Prioritized Findings Matrix\n\n")
		sb.WriteString("| Severity | File | Line | Title | Category | Blocking |\n")
		sb.WriteString("|---|---|---|---|---|---|\n")

		for _, f := range findings {
			blockingEmoji := "No"
			if f.Blocking {
				blockingEmoji = "⚠️ Yes"
			}
			sb.WriteString(fmt.Sprintf("| **%s** | `%s` | `%d-%d` | %s | `%s` | %s |\n",
				f.Severity, f.FilePath, f.StartLine, f.EndLine, f.Title, f.Category, blockingEmoji))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("---\n*Generated by ScanDrix Enterprise Code Review Engine (`scandrix.dev`)*\n")
	return sb.String()
}

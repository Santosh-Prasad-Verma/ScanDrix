// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package rulesengine

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

const (
	// MaxPatternLen caps the length of mechanical regex detector patterns to avoid complex backtracking.
	MaxPatternLen = 200
)

var (
	// nestedQuantifierRegex detects nested quantifiers in regex patterns that cause catastrophic ReDoS backtracking.
	nestedQuantifierRegex = regexp.MustCompile(`(?:\([^()]*[+*}][^()]*\)|\[[^\]]*[+*}][^\]]*\])\s*(?:[*+]|\{\d+,?\d*\})`)
)

// IsDetectorRegexSafe validates that a regex pattern is safe from catastrophic ReDoS backtracking.
func IsDetectorRegexSafe(pattern string) bool {
	if pattern == "" || len(pattern) > MaxPatternLen {
		return false
	}
	if nestedQuantifierRegex.MatchString(pattern) {
		return false
	}
	return true
}

// CompileGateResult records outcome and audit trail of compile-time validation.
type CompileGateResult struct {
	Passed        bool
	Detector      *CompiledRuleDetector
	DeclineReason string
}

// RuleAtomCompiler decomposes long composite rules into atomic testable invariants
// and compiles deterministic T0 regex detectors guarded by rigorous safety gates.
type RuleAtomCompiler struct {
	modelName string
}

// NewRuleAtomCompiler constructs an atom compiler.
func NewRuleAtomCompiler(modelName string) *RuleAtomCompiler {
	if modelName == "" {
		modelName = "scandrix-compiler-v1"
	}
	return &RuleAtomCompiler{
		modelName: modelName,
	}
}

// DecomposeRule breaks down a composite DrixyRule into structured atomic requirements.
func (c *RuleAtomCompiler) DecomposeRule(rule *DrixyRule, examples []DrixyRuleExample) *DrixyRuleAtoms {
	if rule == nil {
		return nil
	}

	rawText := strings.TrimSpace(rule.Description)
	if rawText == "" {
		rawText = strings.TrimSpace(rule.Title)
	}

	atomCandidates := c.parseStructuralCandidates(rule, rawText)
	if len(atomCandidates) == 0 {
		// Fallback: single atom from rule itself
		atomCandidates = append(atomCandidates, candidateRequirement{
			Title:          rule.Title,
			Spec:           rawText,
			Severity:       rule.Severity,
			NormativeLevel: NormativeMust,
		})
	}

	slug := rule.Slug
	if slug == "" {
		slug = rule.ID.String()
	}

	items := make([]*DrixyRuleAtom, 0, len(atomCandidates))
	for i, cand := range atomCandidates {
		atomID := fmt.Sprintf("%s-atom-%d", slug, i+1)
		atom := &DrixyRuleAtom{
			ID:             atomID,
			ParentRuleID:   rule.ID,
			ParentSlug:     slug,
			Index:          i + 1,
			Title:          cand.Title,
			Spec:           cand.Spec,
			Severity:       cand.Severity,
			Category:       "drixy_rules",
			NormativeLevel: cand.NormativeLevel,
			PathGlobs:      rule.PathGlobs,
			Remediation:    cand.Remediation,
			Examples:       cand.Examples,
		}

		if len(atom.Examples) == 0 && len(examples) > 0 {
			atom.Examples = examples
		}

		// Attempt mechanical compile gate
		gateRes := c.EvaluateCompileGate(atom, cand.CandidatePattern, cand.CandidateNegative)
		if gateRes.Passed && gateRes.Detector != nil {
			atom.Detector = gateRes.Detector
		} else {
			atom.DeclineReason = gateRes.DeclineReason
		}

		items = append(items, atom)
	}

	sourceHash := ComputeAtomsSourceHash(rawText, examples)

	return &DrixyRuleAtoms{
		Items:       items,
		SourceHash:  sourceHash,
		GeneratedAt: time.Now().UTC(),
		Model:       c.modelName,
	}
}

// EvaluateCompileGate executes the compile-time gate against atom examples:
// 1. Pattern syntax validity
// 2. ReDoS safety guard
// 3. Recall check: must match at least one line of an incorrect example (if any provided)
// 4. Precision check: must NOT match any line of a correct example
func (c *RuleAtomCompiler) EvaluateCompileGate(atom *DrixyRuleAtom, pattern, negativePattern string) CompileGateResult {
	if pattern == "" {
		pattern, negativePattern = c.SynthesizeHeuristicPattern(atom.Spec)
	}

	if pattern == "" {
		return CompileGateResult{
			Passed:        false,
			DeclineReason: "not-mechanical",
		}
	}

	if !IsDetectorRegexSafe(pattern) {
		return CompileGateResult{
			Passed:        false,
			DeclineReason: "unsafe-regex",
		}
	}

	rx, err := regexp.Compile(pattern)
	if err != nil {
		return CompileGateResult{
			Passed:        false,
			DeclineReason: "invalid-regex",
		}
	}

	var negRx *regexp.Regexp
	if negativePattern != "" {
		if !IsDetectorRegexSafe(negativePattern) {
			return CompileGateResult{
				Passed:        false,
				DeclineReason: "unsafe-regex",
			}
		}
		negRx, err = regexp.Compile(negativePattern)
		if err != nil {
			return CompileGateResult{
				Passed:        false,
				DeclineReason: "invalid-regex",
			}
		}
	}

	// Examples verification gate
	var badExamples []DrixyRuleExample
	var goodExamples []DrixyRuleExample

	for _, ex := range atom.Examples {
		if strings.TrimSpace(ex.Snippet) == "" {
			continue
		}
		if ex.IsCorrect {
			goodExamples = append(goodExamples, ex)
		} else {
			badExamples = append(badExamples, ex)
		}
	}

	// Gate 1: Recall check (must match at least one bad example line)
	if len(badExamples) > 0 {
		matchedAnyBad := false
		for _, bad := range badExamples {
			lines := strings.Split(bad.Snippet, "\n")
			for _, line := range lines {
				content := strings.TrimRight(line, "\r\n")
				if rx.MatchString(content) {
					if negRx == nil || !negRx.MatchString(content) {
						matchedAnyBad = true
						break
					}
				}
			}
			if matchedAnyBad {
				break
			}
		}
		if !matchedAnyBad {
			return CompileGateResult{
				Passed:        false,
				DeclineReason: "missed-incorrect-example",
			}
		}
	}

	// Gate 2: Precision check (must NOT flag any good example line)
	if len(goodExamples) > 0 {
		for _, good := range goodExamples {
			lines := strings.Split(good.Snippet, "\n")
			for _, line := range lines {
				content := strings.TrimRight(line, "\r\n")
				if rx.MatchString(content) {
					if negRx == nil || !negRx.MatchString(content) {
						return CompileGateResult{
							Passed:        false,
							DeclineReason: "flagged-correct-example",
						}
					}
				}
			}
		}
	}

	detector := &CompiledRuleDetector{
		Type:             DetectorRegex,
		Pattern:          pattern,
		CompiledRegex:    rx,
		NegativePattern:  negativePattern,
		CompiledNegative: negRx,
		Reason:           fmt.Sprintf("Compiled deterministic detector for atom %s", atom.ID),
		CompiledBy:       c.modelName,
	}

	return CompileGateResult{
		Passed:   true,
		Detector: detector,
	}
}

// SynthesizeHeuristicPattern detects standard mechanical anti-patterns in invariant prose.
func (c *RuleAtomCompiler) SynthesizeHeuristicPattern(text string) (string, string) {
	lower := strings.ToLower(text)

	// 1. Debug prints
	if strings.Contains(lower, "fmt.print") || strings.Contains(lower, "console.log") || strings.Contains(lower, "no debug print") {
		return `(?i)\b(?:fmt\.Print(?:ln|f)?|console\.(?:log|debug|info))\s*\(`, `(?i)(?:_test\.go|\.test\.ts|\.spec\.ts)`
	}

	// 2. Unsafe package import
	if strings.Contains(lower, "unsafe") && (strings.Contains(lower, "import") || strings.Contains(lower, "package")) {
		return `(?m)^\s*import\s+(?:.*[^\w])?"unsafe"`, ""
	}

	// 3. Raw SQL concat
	if (strings.Contains(lower, "sql") || strings.Contains(lower, "query")) && (strings.Contains(lower, "concatenat") || strings.Contains(lower, "injection") || strings.Contains(lower, "sprintf")) {
		return `(?i)(?:db\.(?:Query|Exec|Raw))\s*\(\s*(?:fmt\.Sprintf\s*\(|["'].*\+\s*[a-zA-Z0-9_\.]+|["'].*\+.*["'])`, `(?i)(?:WHERE\s+1=1|\$1|\?)`
	}

	// 4. Hardcoded Secrets
	if strings.Contains(lower, "secret") || strings.Contains(lower, "api key") || strings.Contains(lower, "credential") || strings.Contains(lower, "token") {
		return `(?i)(?:sk_live_|ghp_|AKIA[0-9A-Z]{16}|bearer\s+[a-zA-Z0-9_\-\.]{30,})`, `(?i)(?:placeholder|example|mock|test_secret)`
	}

	// 5. Bare os.Exit in libraries
	if strings.Contains(lower, "os.exit") && !strings.Contains(lower, "main") {
		return `\bos\.Exit\s*\(`, `(?i)(?:main\.go|main\(\))`
	}

	// 6. Unchecked err
	if strings.Contains(lower, "blank identifier") || strings.Contains(lower, "discard error") || strings.Contains(lower, "_ = err") {
		return `\b_\s*=\s*err\b`, ""
	}

	return "", ""
}

type candidateRequirement struct {
	Title             string
	Spec              string
	Severity          models.FindingSeverity
	NormativeLevel    RFC2119Level
	Remediation       string
	CandidatePattern  string
	CandidateNegative string
	Examples          []DrixyRuleExample
}

// parseStructuralCandidates extracts distinct requirement clauses from markdown lists, headings, and RFC2119 imperatives.
func (c *RuleAtomCompiler) parseStructuralCandidates(rule *DrixyRule, raw string) []candidateRequirement {
	lines := strings.Split(raw, "\n")
	var candidates []candidateRequirement

	var currentTitle string
	var currentLines []string
	var currentLevel RFC2119Level = NormativeMust

	flushCandidate := func() {
		if len(currentLines) == 0 && currentTitle == "" {
			return
		}
		spec := strings.TrimSpace(strings.Join(currentLines, "\n"))
		title := currentTitle
		if title == "" {
			if len(currentLines) > 0 {
				title = strings.TrimLeft(currentLines[0], "-*1234567890. #")
				if len(title) > 60 {
					title = title[:57] + "..."
				}
			} else {
				title = "Rule Requirement"
			}
		}

		sev := rule.Severity
		if currentLevel == NormativeShould || currentLevel == NormativeShouldNot {
			if sev == models.SeverityCritical {
				sev = models.SeverityHigh
			}
		} else if currentLevel == NormativeMay {
			sev = models.SeverityLow
		}

		candidates = append(candidates, candidateRequirement{
			Title:          strings.TrimSpace(title),
			Spec:           spec,
			Severity:       sev,
			NormativeLevel: currentLevel,
		})

		currentTitle = ""
		currentLines = nil
		currentLevel = NormativeMust
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Heading division (e.g. `### Requirement 1: ...`)
		if strings.HasPrefix(trimmed, "#") {
			flushCandidate()
			currentTitle = strings.TrimLeft(trimmed, "# ")
			currentLevel = detectNormativeLevel(trimmed)
			continue
		}

		// Bullet point or numbered list item
		if isListItem(trimmed) {
			if len(currentLines) > 0 {
				flushCandidate()
			}
			clean := strings.TrimLeft(trimmed, "-*1234567890. ")
			currentTitle = clean
			if len(currentTitle) > 60 {
				currentTitle = currentTitle[:57] + "..."
			}
			currentLevel = detectNormativeLevel(clean)
			currentLines = append(currentLines, clean)
			continue
		}

		currentLines = append(currentLines, trimmed)
	}

	flushCandidate()
	return candidates
}

func isListItem(s string) bool {
	if strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "* ") {
		return true
	}
	if len(s) >= 3 && s[0] >= '0' && s[0] <= '9' && (s[1] == '.' || (s[1] >= '0' && s[1] <= '9' && s[2] == '.')) {
		return true
	}
	return false
}

func detectNormativeLevel(s string) RFC2119Level {
	upper := strings.ToUpper(s)
	switch {
	case strings.Contains(upper, "MUST NOT"):
		return NormativeMustNot
	case strings.Contains(upper, "SHALL NOT"):
		return NormativeShallNot
	case strings.Contains(upper, "SHOULD NOT"):
		return NormativeShouldNot
	case strings.Contains(upper, "NEVER"):
		return NormativeNever
	case strings.Contains(upper, "MUST"):
		return NormativeMust
	case strings.Contains(upper, "SHALL"):
		return NormativeShall
	case strings.Contains(upper, "REQUIRED"):
		return NormativeRequired
	case strings.Contains(upper, "ALWAYS"):
		return NormativeAlways
	case strings.Contains(upper, "SHOULD"):
		return NormativeShould
	case strings.Contains(upper, "MAY"):
		return NormativeMay
	default:
		return NormativeMust
	}
}

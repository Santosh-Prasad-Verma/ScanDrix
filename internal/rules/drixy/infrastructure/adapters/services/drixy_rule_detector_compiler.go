// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rule_detector_compiler.go
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// DrixyRuleDetectorCompiler compiles mechanical rules into deterministic T0 regex detectors.
type DrixyRuleDetectorCompiler struct {
	rulesService contracts.IDrixyRulesService
	llmGateway   *llm.Gateway
}

// NewDrixyRuleDetectorCompiler constructs a new compiler.
func NewDrixyRuleDetectorCompiler(service contracts.IDrixyRulesService, gw *llm.Gateway) *DrixyRuleDetectorCompiler {
	return &DrixyRuleDetectorCompiler{
		rulesService: service,
		llmGateway:   gw,
	}
}

// CompileAndSave attempts to compile a rule into a deterministic T0 detector and persist it.
func (c *DrixyRuleDetectorCompiler) CompileAndSave(
	ctx context.Context,
	organizationID string,
	teamID string,
	ruleUUID string,
	rule *interfaces.DrixyRule,
) (contracts.CompileResult, error) {
	if rule == nil || rule.Rule == "" {
		return contracts.CompileResult{Compiled: false, DeclineReason: "empty_rule"}, nil
	}

	// 1. Check if pattern already provided or extract mechanical pattern
	pattern, reason := c.extractDeterministicPattern(rule)
	if pattern == "" {
		return contracts.CompileResult{Compiled: false, DeclineReason: reason}, nil
	}

	// 2. Validate regex compilation & length safety guard (ReDoS protection)
	if len(pattern) > 1000 {
		return contracts.CompileResult{Compiled: false, DeclineReason: "pattern_exceeds_safe_length"}, nil
	}

	compiledRegex, err := regexp.Compile(pattern)
	if err != nil {
		return contracts.CompileResult{Compiled: false, DeclineReason: "invalid_regex"}, nil
	}

	// 3. Verification Gate: test against bad/good examples if present
	if len(rule.Examples) > 0 {
		gatePassed := true
		for _, ex := range rule.Examples {
			matched := compiledRegex.MatchString(ex.Snippet)
			// A violation detector should match incorrect snippets and not match correct ones
			if !ex.IsCorrect && !matched {
				gatePassed = false
				break
			}
			if ex.IsCorrect && matched {
				gatePassed = false
				break
			}
		}
		if !gatePassed {
			return contracts.CompileResult{Compiled: false, DeclineReason: "failed_example_gate"}, nil
		}
	}

	detector := &interfaces.DrixyRuleDetector{
		Type:       "regex",
		Pattern:    pattern,
		CompiledBy: "scandrix-t0-compiler",
		Reason:     "deterministic-mechanical-check",
	}

	// 4. Persist detector onto rule
	if c.rulesService != nil && organizationID != "" && ruleUUID != "" {
		_, _ = c.rulesService.UpdateRuleDetector(ctx, organizationID, ruleUUID, detector)
	}

	return contracts.CompileResult{
		Compiled: true,
		Detector: detector,
	}, nil
}

func (c *DrixyRuleDetectorCompiler) extractDeterministicPattern(rule *interfaces.DrixyRule) (string, string) {
	// If rule explicitly specifies a regex detector
	if rule.Detector != nil && rule.Detector.Pattern != "" {
		return rule.Detector.Pattern, ""
	}

	text := strings.ToLower(rule.Rule + " " + rule.Title)

	// Mechanical detection rules:
	if strings.Contains(text, "console.log") {
		return `console\.log\(`, ""
	}
	if strings.Contains(text, "eval(") || strings.Contains(text, "avoid eval") {
		return `\beval\s*\(`, ""
	}
	if strings.Contains(text, "debugger") {
		return `\bdebugger\b`, ""
	}
	if strings.Contains(text, "http://") && strings.Contains(text, "https") {
		return `http://[a-zA-Z0-9]`, ""
	}
	if strings.Contains(text, "todo") && strings.Contains(text, "forbidden") {
		return `(?i)\bTODO\b`, ""
	}
	if strings.Contains(text, "fixme") && strings.Contains(text, "forbidden") {
		return `(?i)\bFIXME\b`, ""
	}
	if strings.Contains(text, "any") && strings.Contains(text, "typescript") && strings.Contains(text, "disallow") {
		return `:\s*any\b`, ""
	}

	return "", fmt.Sprintf("rule is semantic (requires LLM judgment)")
}

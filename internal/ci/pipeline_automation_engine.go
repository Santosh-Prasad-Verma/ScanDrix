// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package ci

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// PipelineTriggerConfig controls when ScanDrix automated review executes in CI pipelines.
type PipelineTriggerConfig struct {
	TargetBranches []string `json:"target_branches"` // e.g. ["main", "master", "release/*"]
	IgnoreBranches []string `json:"ignore_branches"` // e.g. ["dependabot/*", "renovate/*"]
	IgnorePaths    []string `json:"ignore_paths"`    // e.g. ["docs/**", "**/*.md", ".github/**"]
	SkipDraftPRs   bool     `json:"skip_draft_prs"`
	EnableAutoFix  bool     `json:"enable_auto_fix"`
}

// DefaultPipelineTriggerConfig returns standard enterprise triggers.
func DefaultPipelineTriggerConfig() PipelineTriggerConfig {
	return PipelineTriggerConfig{
		TargetBranches: []string{"main", "master", "release/*", "develop"},
		IgnoreBranches: []string{"dependabot/*", "renovate/*"},
		IgnorePaths:    []string{"docs/**", "**/*.md", "**/*.png", "**/*.svg"},
		SkipDraftPRs:   true,
		EnableAutoFix:  true,
	}
}

// PipelineAutomationEngine evaluates trigger conditions and generates auto-fix commits.
type PipelineAutomationEngine struct {
	config PipelineTriggerConfig
}

// NewPipelineAutomationEngine creates a new pipeline trigger and auto-fix engine.
func NewPipelineAutomationEngine(cfg PipelineTriggerConfig) *PipelineAutomationEngine {
	return &PipelineAutomationEngine{
		config: cfg,
	}
}

// ShouldTriggerReview evaluates whether an automated review should execute for the current CI run.
func (e *PipelineAutomationEngine) ShouldTriggerReview(
	ctx context.Context,
	env CIEnvironment,
	commitMsg string,
	changedFiles []string,
	isDraft bool,
) (bool, string) {
	// 1. Commit message skip flags
	lowerMsg := strings.ToLower(commitMsg)
	if strings.Contains(lowerMsg, "[skip scandrix]") ||
		strings.Contains(lowerMsg, "[scandrix skip]") ||
		strings.Contains(lowerMsg, "[skip ci]") {
		return false, "Skipped by commit message directive"
	}

	// 2. Draft PR evaluation
	if e.config.SkipDraftPRs && isDraft {
		return false, "Skipped because pull request is in draft status"
	}

	// 3. Target Branch check (if base branch specified)
	targetBranch := env.BaseRef
	if targetBranch != "" && len(e.config.TargetBranches) > 0 {
		matched := false
		for _, pat := range e.config.TargetBranches {
			if matchBranch(pat, targetBranch) {
				matched = true
				break
			}
		}
		if !matched {
			return false, fmt.Sprintf("Target branch '%s' does not match configured target branches", targetBranch)
		}
	}

	// 4. Ignored Branch check (head branch)
	headBranch := env.HeadRef
	if headBranch != "" {
		for _, pat := range e.config.IgnoreBranches {
			if matchBranch(pat, headBranch) {
				return false, fmt.Sprintf("Head branch '%s' is explicitly ignored by policy", headBranch)
			}
		}
	}

	// 5. Changed files path filter
	if len(changedFiles) > 0 && len(e.config.IgnorePaths) > 0 {
		allIgnored := true
		for _, file := range changedFiles {
			fileIgnored := false
			for _, pat := range e.config.IgnorePaths {
				if matchGlob(pat, file) {
					fileIgnored = true
					break
				}
			}
			if !fileIgnored {
				allIgnored = false
				break
			}
		}
		if allIgnored {
			return false, "All changed files match ignored path patterns"
		}
	}

	return true, "Eligible for automated review"
}

// GenerateAutoFixPatch synthesizes suggested code replacements into a unified patch.
func (e *PipelineAutomationEngine) GenerateAutoFixPatch(findings []CIFinding) (string, int) {
	var sb bytes.Buffer
	fixCount := 0

	for _, f := range findings {
		if strings.TrimSpace(f.Suggestion) == "" {
			continue
		}

		fixCount++
		sb.WriteString(fmt.Sprintf("--- a/%s\n", f.FilePath))
		sb.WriteString(fmt.Sprintf("+++ b/%s\n", f.FilePath))
		sb.WriteString(fmt.Sprintf("@@ -%d,1 +%d,1 @@\n", f.StartLine, f.StartLine))

		// If suggestion already has diff markers (+, -), output as-is
		lines := strings.Split(f.Suggestion, "\n")
		hasMarkers := false
		for _, l := range lines {
			if strings.HasPrefix(l, "+") || strings.HasPrefix(l, "-") {
				hasMarkers = true
				break
			}
		}

		if hasMarkers {
			sb.WriteString(f.Suggestion)
			sb.WriteString("\n")
		} else {
			// Wrap in addition marker
			for _, l := range lines {
				sb.WriteString("+" + l + "\n")
			}
		}
	}

	return sb.String(), fixCount
}

func matchBranch(pattern, branch string) bool {
	cleanBranch := strings.TrimPrefix(branch, "refs/heads/")
	if pattern == cleanBranch || pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(cleanBranch, prefix+"/")
	}
	matched, _ := filepath.Match(pattern, cleanBranch)
	return matched
}

func matchGlob(pattern, path string) bool {
	cleanPat := strings.ToLower(pattern)
	cleanPath := strings.ToLower(path)

	if matched, _ := filepath.Match(cleanPat, cleanPath); matched {
		return true
	}
	if matched, _ := filepath.Match(cleanPat, filepath.Base(cleanPath)); matched {
		return true
	}
	if strings.HasSuffix(cleanPat, "/**") {
		prefix := strings.TrimSuffix(cleanPat, "/**")
		if strings.HasPrefix(cleanPath, prefix+"/") || cleanPath == prefix {
			return true
		}
	}
	if strings.HasPrefix(cleanPat, "**/") {
		subPat := strings.TrimPrefix(cleanPat, "**/")
		if matched, _ := filepath.Match(subPat, filepath.Base(cleanPath)); matched {
			return true
		}
		if matched, _ := filepath.Match(subPat, cleanPath); matched {
			return true
		}
	}
	return false
}

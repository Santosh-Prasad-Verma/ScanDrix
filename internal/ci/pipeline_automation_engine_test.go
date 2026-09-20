// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package ci

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPipelineAutomationEngine_TriggerDecisions(t *testing.T) {
	cfg := PipelineTriggerConfig{
		TargetBranches: []string{"main", "release/*"},
		IgnoreBranches: []string{"dependabot/*"},
		IgnorePaths:    []string{"docs/**", "**/*.md"},
		SkipDraftPRs:   true,
		EnableAutoFix:  true,
	}

	engine := NewPipelineAutomationEngine(cfg)
	ctx := context.Background()

	// 1. Valid PR to main
	envValid := CIEnvironment{
		BaseRef: "main",
		HeadRef: "feature/auth",
	}
	shouldRun, reason := engine.ShouldTriggerReview(ctx, envValid, "feat: implement auth", []string{"internal/auth.go"}, false)
	assert.True(t, shouldRun)
	assert.Contains(t, reason, "Eligible")

	// 2. Commit message skip
	shouldRun, reason = engine.ShouldTriggerReview(ctx, envValid, "fix typo [skip scandrix]", []string{"internal/auth.go"}, false)
	assert.False(t, shouldRun)
	assert.Contains(t, reason, "commit message directive")

	// 3. Draft PR skip
	shouldRun, reason = engine.ShouldTriggerReview(ctx, envValid, "WIP work", []string{"internal/auth.go"}, true)
	assert.False(t, shouldRun)
	assert.Contains(t, reason, "draft status")

	// 4. Ignored branch (dependabot)
	envDep := CIEnvironment{
		BaseRef: "main",
		HeadRef: "dependabot/npm_and_yarn/foo-1.0.0",
	}
	shouldRun, reason = engine.ShouldTriggerReview(ctx, envDep, "bump dep", []string{"package.json"}, false)
	assert.False(t, shouldRun)
	assert.Contains(t, reason, "explicitly ignored")

	// 5. Ignored paths (only documentation files)
	shouldRun, reason = engine.ShouldTriggerReview(ctx, envValid, "docs update", []string{"docs/readme.md", "README.md"}, false)
	assert.False(t, shouldRun)
	assert.Contains(t, reason, "ignored path patterns")
}

func TestPipelineAutomationEngine_GenerateAutoFixPatch(t *testing.T) {
	engine := NewPipelineAutomationEngine(DefaultPipelineTriggerConfig())

	findings := []CIFinding{
		{
			FilePath:   "internal/db.go",
			StartLine:  10,
			Suggestion: "- query := fmt.Sprintf(\"SELECT * FROM users WHERE id = %s\", id)\n+ query := \"SELECT * FROM users WHERE id = $1\"",
		},
		{
			FilePath:   "internal/worker.go",
			StartLine:  25,
			Suggestion: "time.Sleep(100 * time.Millisecond)",
		},
	}

	patch, count := engine.GenerateAutoFixPatch(findings)
	assert.Equal(t, 2, count)
	assert.Contains(t, patch, "--- a/internal/db.go")
	assert.Contains(t, patch, "+++ b/internal/db.go")
	assert.Contains(t, patch, "SELECT * FROM users WHERE id = $1")
	assert.Contains(t, patch, "--- a/internal/worker.go")
}

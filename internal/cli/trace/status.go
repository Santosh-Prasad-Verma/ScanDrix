// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scandrix/backend/internal/cli/services/git"
)

// TraceStatusReport encapsulates overall Trace health, hook installation, and record metrics.
type TraceStatusReport struct {
	GitRepoDetected   bool   `json:"git_repo_detected"`
	GitRoot           string `json:"git_root,omitempty"`
	CurrentBranch     string `json:"current_branch,omitempty"`
	SessionsCount     int    `json:"sessions_count"`
	TotalTurnsCount   int    `json:"total_turns_count"`
	LocalDecisions    int    `json:"local_decisions"`
	BranchDecisions   int    `json:"branch_decisions"`
	ClaudeHooksActive bool   `json:"claude_hooks_active"`
	CursorHooksActive bool   `json:"cursor_hooks_active"`
	CodexHooksActive  bool   `json:"codex_hooks_active"`
	GitHooksActive    bool   `json:"git_hooks_active"`
	StoreDir          string `json:"store_dir"`
}

// GetTraceStatus compiles a comprehensive status report of the Trace subsystem.
func GetTraceStatus(ctx context.Context, gitRoot string) (*TraceStatusReport, error) {
	if gitRoot == "" {
		gitRoot = "."
	}

	gitSvc := git.NewService(gitRoot)
	branch, err := gitSvc.GetCurrentBranch(ctx)
	isRepo := err == nil && branch != ""

	rep := &TraceStatusReport{
		GitRepoDetected: isRepo,
		GitRoot:         gitRoot,
		CurrentBranch:   branch,
		StoreDir:        RepoStoreDir(gitRoot),
	}

	// Count sessions and turns
	sessions, _ := ListSessions(gitRoot)
	rep.SessionsCount = len(sessions)
	for _, s := range sessions {
		rep.TotalTurnsCount += s.TurnCount
	}

	// Count local decisions
	if decFiles, err := os.ReadDir(LocalDecisionsDir(gitRoot)); err == nil {
		rep.LocalDecisions = len(decFiles)
	}

	// Count branch decisions if trace ref exists
	if isRepo && branch != "" {
		if rec, err := ReadBranchRecord(ctx, gitRoot, branch, ""); err == nil && rec != nil {
			rep.BranchDecisions = len(rec.Decisions)
		}
	}

	// Check Claude hooks (.claude/settings.json)
	if data, err := os.ReadFile(filepath.Join(gitRoot, ".claude", "settings.json")); err == nil {
		if strings.Contains(string(data), "scandrix trace hooks claude-code") {
			rep.ClaudeHooksActive = true
		}
	}

	// Check Cursor hooks (.cursor/hooks.json)
	if data, err := os.ReadFile(filepath.Join(gitRoot, ".cursor", "hooks.json")); err == nil {
		if strings.Contains(string(data), "scandrix trace hooks cursor") {
			rep.CursorHooksActive = true
		}
	}

	// Check Codex hooks (~/.codex/config.toml)
	if home, err := os.UserHomeDir(); err == nil {
		if data, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml")); err == nil {
			if strings.Contains(string(data), "scandrix") && strings.Contains(string(data), "hooks") {
				rep.CodexHooksActive = true
			}
		}
	}

	// Check Git hooks (prepare-commit-msg, pre-push)
	if isRepo {
		if hooksDir, err := gitSvc.GetHooksDir(ctx); err == nil {
			prepData, _ := os.ReadFile(filepath.Join(hooksDir, "prepare-commit-msg"))
			pushData, _ := os.ReadFile(filepath.Join(hooksDir, "pre-push"))
			if strings.Contains(string(prepData), GitTraceMarkerStart) || strings.Contains(string(pushData), GitTraceMarkerStart) {
				rep.GitHooksActive = true
			}
		}
	}

	return rep, nil
}

// FormatStatusReport renders the status report as human-readable CLI text.
func FormatStatusReport(rep *TraceStatusReport) string {
	var sb strings.Builder
	sb.WriteString("ScanDrix Trace Subsystem Status\n")
	sb.WriteString("---------------------------------------------------------------\n")
	sb.WriteString(fmt.Sprintf("  Repository:        %s (branch: %s)\n", rep.GitRoot, rep.CurrentBranch))
	sb.WriteString(fmt.Sprintf("  Sessions Captured: %d sessions (%d turns)\n", rep.SessionsCount, rep.TotalTurnsCount))
	sb.WriteString(fmt.Sprintf("  Decisions:         %d local, %d on %s\n", rep.LocalDecisions, rep.BranchDecisions, TraceBranch))
	sb.WriteString("\n  Assistant Capture Hooks:\n")

	claudeStatus := "not installed"
	if rep.ClaudeHooksActive {
		claudeStatus = "active (in .claude/settings.json)"
	}
	sb.WriteString(fmt.Sprintf("    • Claude Code:   %s\n", claudeStatus))

	cursorStatus := "not installed"
	if rep.CursorHooksActive {
		cursorStatus = "active (in .cursor/hooks.json)"
	}
	sb.WriteString(fmt.Sprintf("    • Cursor:        %s\n", cursorStatus))

	codexStatus := "not installed"
	if rep.CodexHooksActive {
		codexStatus = "active (in ~/.codex/config.toml)"
	}
	sb.WriteString(fmt.Sprintf("    • Codex CLI:     %s\n", codexStatus))

	gitStatus := "not installed"
	if rep.GitHooksActive {
		gitStatus = "active (prepare-commit-msg, pre-push)"
	}
	sb.WriteString(fmt.Sprintf("    • Git Hooks:     %s\n", gitStatus))

	sb.WriteString(fmt.Sprintf("\n  Local Storage:     %s\n", rep.StoreDir))
	sb.WriteString("  (All traces and decisions live cleanly outside the repository tree)\n")
	return sb.String()
}

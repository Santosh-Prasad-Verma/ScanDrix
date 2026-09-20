// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package hooks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scandrix/backend/internal/cli/services/git"
)

const (
	HookMarkerStart  = "# scandrix-hook-start"
	HookMarkerEnd    = "# scandrix-hook-end"
	LegacyMarker     = "# Installed by: scandrix hook install"
	TraceMarkerStart = "# scandrix-trace-start"
	TraceMarkerEnd   = "# scandrix-trace-end"
)

type HookStatusInfo struct {
	GitRepoDetected   bool `json:"git_repo_detected"`
	PreCommitActive   bool `json:"pre_commit_active"`
	PrePushActive     bool `json:"pre_push_active"`
	PreCommitIsCustom bool `json:"pre_commit_is_custom"`
	PrePushIsCustom   bool `json:"pre_push_is_custom"`
}

func getHooksDir(workDir string) (string, error) {
	if workDir == "" {
		workDir = "."
	}
	svc := git.NewService(workDir)
	if dir, err := svc.GetHooksDir(context.Background()); err == nil && dir != "" {
		_ = os.MkdirAll(dir, 0755)
		return dir, nil
	}

	gitDir := filepath.Join(workDir, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return "", fmt.Errorf("not a git repository (missing .git directory in %s)", workDir)
	}
	if info.IsDir() {
		dir := filepath.Join(gitDir, "hooks")
		_ = os.MkdirAll(dir, 0755)
		return dir, nil
	}

	// Linked git worktree: .git is a file containing "gitdir: <path>"
	if data, rErr := os.ReadFile(gitDir); rErr == nil {
		text := strings.TrimSpace(string(data))
		if strings.HasPrefix(text, "gitdir:") {
			target := strings.TrimSpace(strings.TrimPrefix(text, "gitdir:"))
			if !filepath.IsAbs(target) {
				target = filepath.Join(workDir, target)
			}
			parent := filepath.Dir(target)
			if filepath.Base(parent) == "worktrees" {
				commonHooks := filepath.Join(filepath.Dir(parent), "hooks")
				_ = os.MkdirAll(commonHooks, 0755)
				return commonHooks, nil
			}
			commonHooks := filepath.Join(target, "hooks")
			_ = os.MkdirAll(commonHooks, 0755)
			return commonHooks, nil
		}
	}

	return "", fmt.Errorf("not a git repository (%s)", workDir)
}

// ExtractTraceBlock extracts any active trace hook block to preserve during updates.
func ExtractTraceBlock(content string) string {
	if content == "" {
		return ""
	}
	start := strings.Index(content, TraceMarkerStart)
	if start == -1 {
		start = strings.Index(content, "# trace-start")
	}
	if start == -1 {
		return ""
	}
	end := strings.Index(content, TraceMarkerEnd)
	if end == -1 {
		end = strings.Index(content, "# trace-end")
	}
	if end != -1 && end > start {
		remainder := content[end:]
		nl := strings.Index(remainder, "\n")
		if nl != -1 {
			return strings.TrimSpace(content[start : end+nl])
		}
		return strings.TrimSpace(content[start:])
	}
	return ""
}

// generatePreCommitBlock returns the delimited ScanDrix pre-commit snippet.
func generatePreCommitBlock(failOnSeverity string, fast bool) string {
	fastFlag := ""
	if fast {
		fastFlag = "--fast "
	}
	return fmt.Sprintf(`%s
# ScanDrix Pre-Commit Review Guard
if [ -z "$SCANDRIX_SKIP_HOOK" ] && command -v scandrix >/dev/null 2>&1; then
    scandrix review --staged %s--fail-on %s --format terminal --quiet
fi
%s`, HookMarkerStart, fastFlag, strings.ToLower(failOnSeverity), HookMarkerEnd)
}

// generatePrePushBlock returns the delimited ScanDrix pre-push snippet with multi-ref support.
func generatePrePushBlock(failOnSeverity string, fast bool) string {
	fastFlag := ""
	if fast {
		fastFlag = "--fast "
	}
	return fmt.Sprintf(`%s
# ScanDrix Pre-Push Review Guard
if [ -z "$SCANDRIX_SKIP_HOOK" ] && command -v scandrix >/dev/null 2>&1; then
    remote="$1"
    current_branch="$(git symbolic-ref --short HEAD 2>/dev/null || echo '')"

    while read -r local_ref local_sha remote_ref remote_sha; do
        # Skip branch deletions
        if [ "$local_sha" = "0000000000000000000000000000000000000000" ]; then
            continue
        fi

        # Skip new remote branch with no previous state to diff against
        if [ "$remote_sha" = "0000000000000000000000000000000000000000" ]; then
            continue
        fi

        branch_name="${local_ref#refs/heads/}"
        if [ "$branch_name" != "$current_branch" ]; then
            continue
        fi

        if ! scandrix review --branch "${remote}/${branch_name}" %s--fail-on %s --format terminal --quiet; then
            echo "❌ ScanDrix review blocked push due to security/quality violations."
            exit 1
        fi
    done
fi
%s`, HookMarkerStart, fastFlag, strings.ToLower(failOnSeverity), HookMarkerEnd)
}

// injectHookBlock merges ScanDrix snippet into existing hook content non-destructively.
func injectHookBlock(existingContent, newBlock string) string {
	content := strings.TrimSpace(existingContent)
	if content == "" {
		return "#!/usr/bin/env bash\n\n" + newBlock + "\n"
	}

	// Preserve trace block if present
	traceBlock := ExtractTraceBlock(content)

	// 1. If existing content has legacy marker, strip it
	if strings.Contains(content, LegacyMarker) {
		lines := strings.Split(content, "\n")
		var cleaned []string
		for _, l := range lines {
			if !strings.Contains(l, LegacyMarker) && !strings.Contains(l, "ScanDrix Pre-") {
				cleaned = append(cleaned, l)
			}
		}
		content = strings.Join(cleaned, "\n")
	}

	// 2. Replace existing delimited block if present
	startIdx := strings.Index(content, HookMarkerStart)
	endIdx := strings.Index(content, HookMarkerEnd)
	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		before := content[:startIdx]
		after := content[endIdx+len(HookMarkerEnd):]
		merged := strings.TrimSpace(before) + "\n\n" + newBlock + "\n" + strings.TrimSpace(after)
		if traceBlock != "" && !strings.Contains(merged, traceBlock) {
			merged = merged + "\n\n" + traceBlock + "\n"
		}
		return merged + "\n"
	}

	// 3. Append to existing script
	if !strings.HasPrefix(content, "#!") {
		content = "#!/usr/bin/env bash\n\n" + content
	}

	merged := content + "\n\n" + newBlock + "\n"
	if traceBlock != "" && !strings.Contains(merged, traceBlock) {
		merged = merged + "\n\n" + traceBlock + "\n"
	}
	return merged
}

// removeHookBlock strips ScanDrix block from existing hook content.
func removeHookBlock(content string) (string, bool) {
	startIdx := strings.Index(content, HookMarkerStart)
	endIdx := strings.Index(content, HookMarkerEnd)

	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		before := strings.TrimSpace(content[:startIdx])
		after := strings.TrimSpace(content[endIdx+len(HookMarkerEnd):])

		var remainder []string
		if before != "" && before != "#!/usr/bin/env bash" && before != "#!/bin/sh" && before != "#!/bin/bash" {
			remainder = append(remainder, before)
		}
		if after != "" {
			remainder = append(remainder, after)
		}

		if len(remainder) == 0 {
			return "", true // File should be removed completely
		}
		return "#!/usr/bin/env bash\n\n" + strings.Join(remainder, "\n\n") + "\n", false
	}

	// Fallback check for legacy marker
	if strings.Contains(content, LegacyMarker) {
		return "", true
	}

	return content, false
}

// Install installs pre-commit and/or pre-push hooks non-destructively.
func Install(workDir string, preCommit, prePush bool, failOnSeverity string, fast ...bool) error {
	hooksDir, err := getHooksDir(workDir)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("failed creating hooks directory: %w", err)
	}

	if failOnSeverity == "" {
		failOnSeverity = "critical"
	}

	isFast := true
	if len(fast) > 0 {
		isFast = fast[0]
	}

	if preCommit {
		hookPath := filepath.Join(hooksDir, "pre-commit")
		var existing string
		if data, err := os.ReadFile(hookPath); err == nil {
			existing = string(data)
		}
		updated := injectHookBlock(existing, generatePreCommitBlock(failOnSeverity, isFast))
		if err := os.WriteFile(hookPath, []byte(updated), 0755); err != nil {
			return fmt.Errorf("failed writing pre-commit hook: %w", err)
		}
	}

	if prePush {
		hookPath := filepath.Join(hooksDir, "pre-push")
		var existing string
		if data, err := os.ReadFile(hookPath); err == nil {
			existing = string(data)
		}
		updated := injectHookBlock(existing, generatePrePushBlock(failOnSeverity, isFast))
		if err := os.WriteFile(hookPath, []byte(updated), 0755); err != nil {
			return fmt.Errorf("failed writing pre-push hook: %w", err)
		}
	}

	return nil
}

// Uninstall cleanly removes ScanDrix blocks from git hooks while preserving user scripts.
func Uninstall(workDir string) error {
	hooksDir, err := getHooksDir(workDir)
	if err != nil {
		return err
	}

	for _, name := range []string{"pre-commit", "pre-push"} {
		hookPath := filepath.Join(hooksDir, name)
		if content, err := os.ReadFile(hookPath); err == nil {
			cleaned, removeEntireFile := removeHookBlock(string(content))
			if removeEntireFile {
				_ = os.Remove(hookPath)
			} else {
				_ = os.WriteFile(hookPath, []byte(cleaned), 0755)
			}
		}
	}

	return nil
}

// Status inspects the current state of git hooks.
func Status(workDir string) (*HookStatusInfo, error) {
	hooksDir, err := getHooksDir(workDir)
	if err != nil {
		return &HookStatusInfo{GitRepoDetected: false}, nil
	}

	status := &HookStatusInfo{GitRepoDetected: true}

	preCommitPath := filepath.Join(hooksDir, "pre-commit")
	if content, err := os.ReadFile(preCommitPath); err == nil {
		str := string(content)
		if strings.Contains(str, HookMarkerStart) || strings.Contains(str, LegacyMarker) {
			status.PreCommitActive = true
		}
		// If there are other lines outside the marker, mark custom as true
		cleaned, _ := removeHookBlock(str)
		if strings.TrimSpace(cleaned) != "" {
			status.PreCommitIsCustom = true
		}
	}

	prePushPath := filepath.Join(hooksDir, "pre-push")
	if content, err := os.ReadFile(prePushPath); err == nil {
		str := string(content)
		if strings.Contains(str, HookMarkerStart) || strings.Contains(str, LegacyMarker) {
			status.PrePushActive = true
		}
		cleaned, _ := removeHookBlock(str)
		if strings.TrimSpace(cleaned) != "" {
			status.PrePushIsCustom = true
		}
	}

	return status, nil
}

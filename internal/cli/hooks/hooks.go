// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	HookMarkerStart = "# scandrix-hook-start"
	HookMarkerEnd   = "# scandrix-hook-end"
	LegacyMarker    = "# Installed by: scandrix hook install"
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
	gitDir := filepath.Join(workDir, ".git")
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("not a git repository (missing .git directory in %s)", workDir)
	}
	return filepath.Join(gitDir, "hooks"), nil
}

// generatePreCommitBlock returns the delimited ScanDrix pre-commit snippet.
func generatePreCommitBlock(failOnSeverity string) string {
	return fmt.Sprintf(`%s
# ScanDrix Pre-Commit Review Guard
if [ -z "$SCANDRIX_SKIP_HOOK" ] && command -v scandrix >/dev/null 2>&1; then
    scandrix review --staged --fail-on-severity %s
fi
%s`, HookMarkerStart, failOnSeverity, HookMarkerEnd)
}

// generatePrePushBlock returns the delimited ScanDrix pre-push snippet with multi-ref support.
func generatePrePushBlock(failOnSeverity string) string {
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

        if ! scandrix review --branch "${remote}/${branch_name}" --fail-on-severity %s; then
            echo "❌ ScanDrix review blocked push due to security/quality violations."
            exit 1
        fi
    done
fi
%s`, HookMarkerStart, failOnSeverity, HookMarkerEnd)
}

// injectHookBlock merges ScanDrix snippet into existing hook content non-destructively.
func injectHookBlock(existingContent, newBlock string) string {
	content := strings.TrimSpace(existingContent)
	if content == "" {
		return "#!/usr/bin/env bash\n\n" + newBlock + "\n"
	}

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
		return strings.TrimSpace(before) + "\n\n" + newBlock + "\n" + strings.TrimSpace(after) + "\n"
	}

	// 3. Append to existing script
	if !strings.HasPrefix(content, "#!") {
		content = "#!/usr/bin/env bash\n\n" + content
	}

	return content + "\n\n" + newBlock + "\n"
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
func Install(workDir string, preCommit, prePush bool, failOnSeverity string) error {
	hooksDir, err := getHooksDir(workDir)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("failed creating hooks directory: %w", err)
	}

	if failOnSeverity == "" {
		failOnSeverity = "HIGH"
	}

	if preCommit {
		hookPath := filepath.Join(hooksDir, "pre-commit")
		var existing string
		if data, err := os.ReadFile(hookPath); err == nil {
			existing = string(data)
		}
		updated := injectHookBlock(existing, generatePreCommitBlock(failOnSeverity))
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
		updated := injectHookBlock(existing, generatePrePushBlock(failOnSeverity))
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

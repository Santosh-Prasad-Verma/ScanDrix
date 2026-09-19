// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	TraceHookMarker    = "# scandrix-trace-start"
	TraceHookEndMarker = "# scandrix-trace-end"
	ReviewHookMarker   = "# scandrix-review-start"
	ReviewHookEndMarker = "# scandrix-review-end"
)

var (
	LegacyHookMarkers = []string{
		"# legacy-trace-start",
		"# legacy-review-start",
		"# Installed by: scandrix hook install",
	}
	LegacyHookEndMarkers = []string{
		"# legacy-trace-end",
		"# legacy-review-end",
	}

	PrepareCommitMsgScript = `# scandrix-trace-start
# ScanDrix Trace Commit-Msg Hook
if [ -z "$SCANDRIX_SKIP_TRACE" ] && command -v scandrix >/dev/null 2>&1; then
    scandrix trace commit-trailer "$1" "$2" 2>/dev/null || true
fi
# scandrix-trace-end`

	PrePushScript = `# scandrix-review-start
# ScanDrix Pre-Push Review Guard
if [ -z "$SCANDRIX_SKIP_HOOK" ] && command -v scandrix >/dev/null 2>&1; then
    remote="$1"
    current_branch="$(git symbolic-ref --short HEAD 2>/dev/null || echo '')"
    while read -r local_ref local_sha remote_ref remote_sha; do
        if [ "$local_sha" = "0000000000000000000000000000000000000000" ]; then continue; fi
        if [ "$remote_sha" = "0000000000000000000000000000000000000000" ]; then continue; fi
        branch_name="${local_ref#refs/heads/}"
        if [ "$branch_name" != "$current_branch" ]; then continue; fi
        if ! scandrix review --branch "${remote}/${branch_name}" --fail-on high --format terminal --quiet; then
            echo "❌ ScanDrix review blocked push due to quality or security policy violations."
            exit 1
        fi
    done
fi
# scandrix-review-end`

	PreCommitScript = `# scandrix-review-start
# ScanDrix Pre-Commit Review Guard
if [ -z "$SCANDRIX_SKIP_HOOK" ] && command -v scandrix >/dev/null 2>&1; then
    if ! scandrix review --staged --fail-on high --format terminal --quiet; then
        echo "❌ ScanDrix review blocked commit due to staged policy violations."
        exit 1
    fi
fi
# scandrix-review-end`
)

// HookInstallResult reports which hooks were installed or already active.
type HookInstallResult struct {
	Installed        []string `json:"installed"`
	AlreadyInstalled []string `json:"alreadyInstalled"`
}

// HookUninstallResult reports which hooks were cleanly stripped.
type HookUninstallResult struct {
	Removed []string `json:"removed"`
}

// HooksService manages lifecycle and non-destructive injection of Git hooks.
type HooksService struct {
	gitService *Service
}

// NewHooksService creates a hooks manager tied to a GitService.
func NewHooksService(gitSvc *Service) *HooksService {
	if gitSvc == nil {
		gitSvc = DefaultService()
	}
	return &HooksService{gitService: gitSvc}
}

// ResolveHooksDir determines the active hooks directory (respecting core.hooksPath).
func (h *HooksService) ResolveHooksDir(ctx context.Context) (string, error) {
	hooksPath, err := h.gitService.runGit(ctx, "config", "--get", "core.hooksPath")
	if err == nil && strings.TrimSpace(hooksPath) != "" {
		p := strings.TrimSpace(hooksPath)
		if filepath.IsAbs(p) {
			return p, nil
		}
		root, err := h.gitService.GetGitRoot(ctx)
		if err != nil {
			return "", err
		}
		return filepath.Join(root, p), nil
	}

	commonDir, err := h.gitService.runGit(ctx, "rev-parse", "--git-common-dir")
	if err == nil && strings.TrimSpace(commonDir) != "" {
		p := strings.TrimSpace(commonDir)
		if filepath.IsAbs(p) {
			return filepath.Join(p, "hooks"), nil
		}
		root, err := h.gitService.GetGitRoot(ctx)
		if err != nil {
			return "", err
		}
		return filepath.Join(root, p, "hooks"), nil
	}

	gitDir, err := h.gitService.runGit(ctx, "rev-parse", "--git-dir")
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}

	p := strings.TrimSpace(gitDir)
	if filepath.IsAbs(p) {
		return filepath.Join(p, "hooks"), nil
	}
	root, err := h.gitService.GetGitRoot(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, p, "hooks"), nil
}

// Install installs prepare-commit-msg, pre-push, and pre-commit hooks non-destructively.
func (h *HooksService) Install(ctx context.Context) (*HookInstallResult, error) {
	hooksDir, err := h.ResolveHooksDir(ctx)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return nil, fmt.Errorf("failed creating hooks directory: %w", err)
	}

	result := &HookInstallResult{
		Installed:        make([]string, 0),
		AlreadyInstalled: make([]string, 0),
	}

	// 1. Remove legacy post-commit if present
	_ = h.removeLegacyHook(hooksDir, "post-commit")

	// 2. Install prepare-commit-msg
	prepRes, err := h.installHookFile(hooksDir, "prepare-commit-msg", PrepareCommitMsgScript, TraceHookMarker, TraceHookEndMarker)
	if err != nil {
		return nil, err
	}
	if prepRes {
		result.AlreadyInstalled = append(result.AlreadyInstalled, "prepare-commit-msg")
	} else {
		result.Installed = append(result.Installed, "prepare-commit-msg")
	}

	// 3. Install pre-push
	pushRes, err := h.installHookFile(hooksDir, "pre-push", PrePushScript, ReviewHookMarker, ReviewHookEndMarker)
	if err != nil {
		return nil, err
	}
	if pushRes {
		result.AlreadyInstalled = append(result.AlreadyInstalled, "pre-push")
	} else {
		result.Installed = append(result.Installed, "pre-push")
	}

	// 4. Install pre-commit
	commitRes, err := h.installHookFile(hooksDir, "pre-commit", PreCommitScript, ReviewHookMarker, ReviewHookEndMarker)
	if err != nil {
		return nil, err
	}
	if commitRes {
		result.AlreadyInstalled = append(result.AlreadyInstalled, "pre-commit")
	} else {
		result.Installed = append(result.Installed, "pre-commit")
	}

	return result, nil
}

// Uninstall cleanly removes ScanDrix blocks from git hooks while preserving user custom scripts.
func (h *HooksService) Uninstall(ctx context.Context) (*HookUninstallResult, error) {
	hooksDir, err := h.ResolveHooksDir(ctx)
	if err != nil {
		return nil, err
	}

	result := &HookUninstallResult{
		Removed: make([]string, 0),
	}

	for _, name := range []string{"prepare-commit-msg", "pre-push", "pre-commit", "post-commit"} {
		if removed, _ := h.removeHookFile(hooksDir, name); removed {
			result.Removed = append(result.Removed, name)
		}
	}

	return result, nil
}

func (h *HooksService) installHookFile(hooksDir, hookName, script, marker, endMarker string) (alreadyInstalled bool, err error) {
	hookPath := filepath.Join(hooksDir, hookName)
	var existing string
	if data, err := os.ReadFile(hookPath); err == nil {
		existing = string(data)
	}

	withoutLegacy := StripBlocks(existing, LegacyHookMarkers, LegacyHookEndMarkers)
	blocks := ExtractBlocks(existing, marker, endMarker)
	if withoutLegacy == existing && len(blocks) == 1 && strings.TrimSpace(blocks[0]) == strings.TrimSpace(script) {
		return true, nil
	}

	withoutBlock := StripBlocks(withoutLegacy, []string{marker}, []string{endMarker})
	content := appendHookBlock(withoutBlock, script)

	if content == existing {
		return true, nil
	}

	if err := os.WriteFile(hookPath, []byte(content), 0755); err != nil {
		return false, err
	}

	return false, nil
}

func (h *HooksService) removeLegacyHook(hooksDir, hookName string) error {
	hookPath := filepath.Join(hooksDir, hookName)
	data, err := os.ReadFile(hookPath)
	if err != nil {
		return nil
	}
	remaining := StripBlocks(string(data), LegacyHookMarkers, LegacyHookEndMarkers)
	if remaining == string(data) {
		return nil
	}
	if strings.TrimSpace(remaining) == "#!/bin/sh" || strings.TrimSpace(remaining) == "#!/usr/bin/env bash" || strings.TrimSpace(remaining) == "" {
		return os.Remove(hookPath)
	}
	return os.WriteFile(hookPath, []byte(remaining), 0755)
}

func (h *HooksService) removeHookFile(hooksDir, hookName string) (bool, error) {
	hookPath := filepath.Join(hooksDir, hookName)
	data, err := os.ReadFile(hookPath)
	if err != nil {
		return false, nil
	}
	content := string(data)
	allMarkers := append([]string{TraceHookMarker, ReviewHookMarker}, LegacyHookMarkers...)
	allEndMarkers := append([]string{TraceHookEndMarker, ReviewHookEndMarker}, LegacyHookEndMarkers...)

	found := false
	for _, m := range allMarkers {
		if strings.Contains(content, m) {
			found = true
			break
		}
	}
	if !found {
		return false, nil
	}

	cleaned := StripBlocks(content, allMarkers, allEndMarkers)
	trimmed := strings.TrimSpace(cleaned)
	if trimmed == "" || trimmed == "#!/bin/sh" || trimmed == "#!/usr/bin/env bash" {
		_ = os.Remove(hookPath)
		return true, nil
	}

	_ = os.WriteFile(hookPath, []byte(cleaned), 0755)
	return true, nil
}

func appendHookBlock(existing, script string) string {
	base := strings.TrimRight(existing, " \t\r\n")
	if base == "" {
		return fmt.Sprintf("#!/bin/sh\n\n%s\n", script)
	}
	return fmt.Sprintf("%s\n\n%s\n", base, script)
}

// StripBlocks removes marked sections from hook content cleanly.
func StripBlocks(content string, markers, endMarkers []string) string {
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	for {
		startIdx := -1
		for i, line := range lines {
			t := strings.TrimSpace(line)
			for _, m := range markers {
				if t == m {
					startIdx = i
					break
				}
			}
			if startIdx != -1 {
				break
			}
		}
		if startIdx == -1 {
			break
		}

		endIdx := -1
		for i := startIdx + 1; i < len(lines); i++ {
			t := strings.TrimSpace(lines[i])
			for _, em := range endMarkers {
				if t == em {
					endIdx = i
					break
				}
			}
			if endIdx != -1 {
				break
			}
		}

		if endIdx == -1 {
			// Remove from startIdx to end of lines
			lines = lines[:startIdx]
			break
		} else {
			lines = append(lines[:startIdx], lines[endIdx+1:]...)
		}
	}

	res := strings.Join(lines, "\n")
	// Clean up multi-blank lines
	for strings.Contains(res, "\n\n\n") {
		res = strings.ReplaceAll(res, "\n\n\n", "\n\n")
	}
	return strings.TrimRight(res, " \t\r\n") + "\n"
}

// ExtractBlocks finds and returns all blocks enclosed by marker and endMarker.
func ExtractBlocks(content, marker, endMarker string) []string {
	var blocks []string
	lines := strings.Split(content, "\n")

	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != marker {
			continue
		}
		end := -1
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == endMarker {
				end = j
				break
			}
		}
		if end != -1 {
			blocks = append(blocks, strings.Join(lines[i:end+1], "\n"))
			i = end
		}
	}

	return blocks
}

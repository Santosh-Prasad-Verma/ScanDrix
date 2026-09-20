// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/cli/services/git"
)

const (
	GitTraceMarkerStart = "# scandrix-trace-start"
	GitTraceMarkerEnd   = "# scandrix-trace-end"
)

// InstallClaudeHooks configures Claude Code hooks in .claude/settings.json.
func InstallClaudeHooks(gitRoot string) (bool, error) {
	claudeDir := filepath.Join(gitRoot, ".claude")
	_ = os.MkdirAll(claudeDir, 0755)
	settingsPath := filepath.Join(claudeDir, "settings.json")

	var settings map[string]any
	if data, err := os.ReadFile(settingsPath); err == nil {
		_ = json.Unmarshal(data, &settings)
	}
	if settings == nil {
		settings = make(map[string]any)
	}

	hooksMap, _ := settings["hooks"].(map[string]any)
	if hooksMap == nil {
		hooksMap = make(map[string]any)
	}

	changed := false
	hookEvents := []string{
		"session-start",
		"session-end",
		"stop",
		"user-prompt-submit",
		"subagent-start",
		"subagent-stop",
	}

	for _, ev := range hookEvents {
		expectedCmd := fmt.Sprintf("scandrix trace hooks claude-code %s", ev)
		if cur, _ := hooksMap[ev].(string); cur != expectedCmd {
			hooksMap[ev] = expectedCmd
			changed = true
		}
	}

	if changed {
		settings["hooks"] = hooksMap
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return false, err
		}
		return true, os.WriteFile(settingsPath, append(data, '\n'), 0644)
	}
	return false, nil
}

// RemoveClaudeHooks cleans up ScanDrix hooks from .claude/settings.json.
func RemoveClaudeHooks(gitRoot string) (bool, error) {
	settingsPath := filepath.Join(gitRoot, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return false, nil
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return false, nil
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok || hooksMap == nil {
		return false, nil
	}

	changed := false
	for k, v := range hooksMap {
		if s, ok := v.(string); ok && strings.Contains(s, "scandrix trace hooks") {
			delete(hooksMap, k)
			changed = true
		}
	}

	if changed {
		settings["hooks"] = hooksMap
		out, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return false, err
		}
		return true, os.WriteFile(settingsPath, append(out, '\n'), 0644)
	}
	return false, nil
}

// InstallCursorHooks configures Cursor hooks in .cursor/hooks.json.
func InstallCursorHooks(gitRoot string) (bool, error) {
	cursorDir := filepath.Join(gitRoot, ".cursor")
	_ = os.MkdirAll(cursorDir, 0755)
	hooksPath := filepath.Join(cursorDir, "hooks.json")

	var conf map[string]any
	if data, err := os.ReadFile(hooksPath); err == nil {
		_ = json.Unmarshal(data, &conf)
	}
	if conf == nil {
		conf = make(map[string]any)
	}
	conf["version"] = 1

	hooksMap, _ := conf["hooks"].(map[string]any)
	if hooksMap == nil {
		hooksMap = make(map[string]any)
	}

	changed := false
	events := []string{"sessionStart", "sessionEnd", "stop", "beforeSubmitPrompt", "subagentStart", "subagentStop"}

	for _, ev := range events {
		expectedCmd := fmt.Sprintf("scandrix trace hooks cursor %s", ev)
		var list []any
		if rawList, ok := hooksMap[ev].([]any); ok {
			list = rawList
		}

		alreadyConfigured := false
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				if c, _ := m["command"].(string); c == expectedCmd {
					alreadyConfigured = true
					break
				}
			}
		}

		if !alreadyConfigured {
			list = append(list, map[string]any{"command": expectedCmd})
			hooksMap[ev] = list
			changed = true
		}
	}

	if changed {
		conf["hooks"] = hooksMap
		data, err := json.MarshalIndent(conf, "", "  ")
		if err != nil {
			return false, err
		}
		return true, os.WriteFile(hooksPath, append(data, '\n'), 0644)
	}
	return false, nil
}

// RemoveCursorHooks removes ScanDrix hooks from .cursor/hooks.json.
func RemoveCursorHooks(gitRoot string) (bool, error) {
	hooksPath := filepath.Join(gitRoot, ".cursor", "hooks.json")
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		return false, nil
	}

	var conf map[string]any
	if err := json.Unmarshal(data, &conf); err != nil {
		return false, nil
	}

	hooksMap, ok := conf["hooks"].(map[string]any)
	if !ok || hooksMap == nil {
		return false, nil
	}

	changed := false
	for ev, rawList := range hooksMap {
		if list, ok := rawList.([]any); ok {
			var clean []any
			for _, item := range list {
				if m, ok := item.(map[string]any); ok {
					if c, _ := m["command"].(string); strings.Contains(c, "scandrix trace hooks") {
						changed = true
						continue
					}
				}
				clean = append(clean, item)
			}
			hooksMap[ev] = clean
		}
	}

	if changed {
		conf["hooks"] = hooksMap
		out, err := json.MarshalIndent(conf, "", "  ")
		if err != nil {
			return false, err
		}
		return true, os.WriteFile(hooksPath, append(out, '\n'), 0644)
	}
	return false, nil
}

// InstallGitTraceHooks adds prepare-commit-msg and pre-push hooks to git hooks directory.
func InstallGitTraceHooks(ctx context.Context, gitRoot string) ([]string, error) {
	gitSvc := git.NewService(gitRoot)
	hooksDir, err := gitSvc.GetHooksDir(ctx)
	if err != nil {
		return nil, err
	}
	_ = os.MkdirAll(hooksDir, 0755)

	var installed []string

	// 1. prepare-commit-msg: adds ScanDrix-Trace trailer
	prepareMsgScript := fmt.Sprintf(`%s
# ScanDrix Trace commit trailer linker
if command -v scandrix >/dev/null 2>&1; then
    if ! grep -q '^ScanDrix-Trace:' "$1" 2>/dev/null; then
        SCANDRIX_TRAILER="$(scandrix trace commit-trailer 2>/dev/null)"
        if [ -n "$SCANDRIX_TRAILER" ]; then
            printf '\n%%s\n' "$SCANDRIX_TRAILER" >> "$1"
        fi
    fi
fi
%s`, GitTraceMarkerStart, GitTraceMarkerEnd)

	prepPath := filepath.Join(hooksDir, "prepare-commit-msg")
	if updated := injectTraceScript(prepPath, prepareMsgScript); updated {
		installed = append(installed, "prepare-commit-msg")
	}

	// 2. pre-push: detached background distillation
	prePushScript := fmt.Sprintf(`%s
# ScanDrix Trace background distillation on push
if [ -z "$SCANDRIX_TRACE_SKIP" ] && command -v scandrix >/dev/null 2>&1; then
    remote="$1"
    while read -r local_ref local_sha remote_ref remote_sha; do
        case "$remote_ref" in
            refs/heads/scandrix/trace/v1) continue ;;
            refs/heads/*) ;;
            *) continue ;;
        esac

        if [ "$local_sha" = "0000000000000000000000000000000000000000" ]; then
            continue
        fi

        branch_name="${remote_ref#refs/heads/}"
        scandrix trace distill --branch "$branch_name" --head "$local_sha" --remote "$remote" >/dev/null 2>&1 </dev/null &
    done
fi
%s`, GitTraceMarkerStart, GitTraceMarkerEnd)

	pushPath := filepath.Join(hooksDir, "pre-push")
	if updated := injectTraceScript(pushPath, prePushScript); updated {
		installed = append(installed, "pre-push")
	}

	return installed, nil
}

// RemoveGitTraceHooks strips scandrix trace blocks from git hooks.
func RemoveGitTraceHooks(ctx context.Context, gitRoot string) ([]string, error) {
	gitSvc := git.NewService(gitRoot)
	hooksDir, err := gitSvc.GetHooksDir(ctx)
	if err != nil {
		return nil, err
	}

	var removed []string
	for _, hookName := range []string{"prepare-commit-msg", "pre-push"} {
		p := filepath.Join(hooksDir, hookName)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}

		content := string(data)
		startIdx := strings.Index(content, GitTraceMarkerStart)
		endIdx := strings.Index(content, GitTraceMarkerEnd)
		if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
			before := strings.TrimSpace(content[:startIdx])
			after := strings.TrimSpace(content[endIdx+len(GitTraceMarkerEnd):])
			var newContent string
			if before != "" && after != "" {
				newContent = before + "\n\n" + after + "\n"
			} else if before != "" {
				newContent = before + "\n"
			} else if after != "" {
				newContent = after + "\n"
			}
			_ = os.WriteFile(p, []byte(newContent), 0755)
			removed = append(removed, hookName)
		}
	}
	return removed, nil
}

func injectTraceScript(hookPath, scriptBlock string) bool {
	existing := ""
	if data, err := os.ReadFile(hookPath); err == nil {
		existing = string(data)
	}

	startIdx := strings.Index(existing, GitTraceMarkerStart)
	endIdx := strings.Index(existing, GitTraceMarkerEnd)

	var newContent string
	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		// Replace block
		before := strings.TrimSpace(existing[:startIdx])
		after := strings.TrimSpace(existing[endIdx+len(GitTraceMarkerEnd):])
		newContent = before + "\n\n" + scriptBlock + "\n" + after + "\n"
	} else if strings.TrimSpace(existing) == "" {
		newContent = "#!/usr/bin/env bash\n\n" + scriptBlock + "\n"
	} else {
		newContent = strings.TrimSpace(existing) + "\n\n" + scriptBlock + "\n"
	}

	if err := os.WriteFile(hookPath, []byte(newContent), 0755); err == nil {
		return true
	}
	return false
}

const CodexHookMarker = "scandrix trace hooks codex"

// ResolveCodexConfigPath resolves the path to ~/.codex/config.toml.
func ResolveCodexConfigPath(rawPath string) string {
	if strings.TrimSpace(rawPath) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		return filepath.Join(home, ".codex", "config.toml")
	}

	if strings.HasPrefix(rawPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		return filepath.Join(home, rawPath[2:])
	}

	return filepath.Clean(rawPath)
}

// InstallCodexHooks configures Codex hooks in ~/.codex/config.toml.
func InstallCodexHooks(configPath string) (bool, error) {
	resolved := ResolveCodexConfigPath(configPath)
	data, _ := os.ReadFile(resolved)
	content := string(data)

	markerCount := strings.Count(content, CodexHookMarker)
	if markerCount == 1 {
		return false, nil
	}
	if markerCount > 1 {
		_, _ = RemoveCodexHooks(resolved)
		data, _ = os.ReadFile(resolved)
		content = string(data)
	}

	hookBlock := fmt.Sprintf("\n[[hooks]]\nevent = \"AfterAgent\"\ncommand = \"%s AfterAgent\"\n", CodexHookMarker)

	var nextContent string
	if strings.TrimSpace(content) == "" {
		nextContent = strings.TrimSpace(hookBlock) + "\n"
	} else {
		nextContent = strings.TrimRight(content, " \t\r\n") + "\n" + hookBlock
	}

	dir := filepath.Dir(resolved)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false, err
	}
	return true, os.WriteFile(resolved, []byte(nextContent), 0644)
}

// RemoveCodexHooks removes ScanDrix hooks from ~/.codex/config.toml.
func RemoveCodexHooks(configPath string) (bool, error) {
	resolved := ResolveCodexConfigPath(configPath)
	data, err := os.ReadFile(resolved)
	if err != nil {
		return false, nil
	}

	content := string(data)
	if !strings.Contains(content, CodexHookMarker) {
		return false, nil
	}

	lines := strings.Split(content, "\n")
	var resultLines []string
	i := 0

	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "[[hooks]]" {
			blockLines := getTomlBlock(lines, i)
			hasMarker := false
			for _, bl := range blockLines {
				if strings.Contains(bl, CodexHookMarker) {
					hasMarker = true
					break
				}
			}
			if hasMarker {
				i += len(blockLines)
				continue
			}
		}
		resultLines = append(resultLines, line)
		i++
	}

	nextContent := strings.Join(resultLines, "\n")
	multiNewlines := regexp.MustCompile(`\n{3,}`)
	nextContent = multiNewlines.ReplaceAllString(nextContent, "\n\n")
	nextContent = strings.TrimLeft(nextContent, "\n")
	nextContent = strings.TrimRight(nextContent, " \t\r\n")
	if nextContent != "" {
		nextContent += "\n"
	}

	return true, os.WriteFile(resolved, []byte(nextContent), 0644)
}

func getTomlBlock(lines []string, startIndex int) []string {
	block := []string{lines[startIndex]}
	for i := startIndex + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "[[") || (strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "[[")) {
			break
		}
		block = append(block, lines[i])
	}
	return block
}


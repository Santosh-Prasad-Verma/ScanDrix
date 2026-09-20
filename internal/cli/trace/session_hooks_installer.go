package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	SessionHooksPrefix = "scandrix trace hooks"
)

// SessionHookResult represents the outcome of installing session hooks.
type SessionHookResult struct {
	SettingsPath string `json:"settings_path"`
	Changed      bool   `json:"changed"`
}

// SessionHookRemovalResult represents the outcome of removing session hooks.
type SessionHookRemovalResult struct {
	SettingsPath string `json:"settings_path"`
	Removed      bool   `json:"removed"`
}

// ClaudeCommandHook represents an individual command hook inside Claude Code settings.
type ClaudeCommandHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// ClaudeMatcherBlock represents a matcher group containing hooks.
type ClaudeMatcherBlock struct {
	Matcher string              `json:"matcher"`
	Hooks   []ClaudeCommandHook `json:"hooks"`
}

// CursorHookEntry represents a single hook command in Cursor hooks.json.
type CursorHookEntry struct {
	Command string `json:"command"`
}

// CursorHooksConfig represents .cursor/hooks.json structure.
type CursorHooksConfig struct {
	Version int                          `json:"version"`
	Hooks   map[string][]CursorHookEntry `json:"hooks"`
}

// isSessionsHookCommand determines if a command belongs to ScanDrix session hooks
// or legacy decisions command that should be upgraded.
func isSessionsHookCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	return strings.Contains(command, SessionHooksPrefix) ||
		strings.HasPrefix(trimmed, "scandrix decisions ")
}

// readJSONMap safely reads a JSON file into a map[string]any.
func readJSONMap(filePath string) (map[string]any, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]any), nil
		}
		return nil, err
	}
	var res map[string]any
	if err := json.Unmarshal(data, &res); err != nil {
		// If JSON is malformed, start fresh
		return make(map[string]any), nil
	}
	if res == nil {
		res = make(map[string]any)
	}
	return res, nil
}

// upsertClaudeHook updates or appends a hook in Claude Code settings.
func upsertClaudeHook(hooks map[string]any, eventKey, matcherName, command string) bool {
	existing, ok := hooks[eventKey]
	var matchers []any
	if ok {
		if arr, isArr := existing.([]any); isArr {
			matchers = arr
		}
	}

	for i, m := range matchers {
		mMap, isMap := m.(map[string]any)
		if !isMap {
			continue
		}
		curMatcher, _ := mMap["matcher"].(string)
		if curMatcher != matcherName {
			continue
		}

		var hookList []any
		if rawHooks, hasHooks := mMap["hooks"].([]any); hasHooks {
			hookList = rawHooks
		}

		var managed []map[string]any
		for _, h := range hookList {
			if hMap, isH := h.(map[string]any); isH {
				if t, _ := hMap["type"].(string); t == "command" {
					if cmd, _ := hMap["command"].(string); isSessionsHookCommand(cmd) {
						managed = append(managed, hMap)
					}
				}
			}
		}

		if len(managed) == 1 {
			if cmd, _ := managed[0]["command"].(string); cmd == command {
				return false
			}
		}

		var filteredHooks []any
		for _, h := range hookList {
			hMap, isH := h.(map[string]any)
			if !isH {
				filteredHooks = append(filteredHooks, h)
				continue
			}
			t, _ := hMap["type"].(string)
			cmd, _ := hMap["command"].(string)
			if t == "command" && isSessionsHookCommand(cmd) {
				continue
			}
			filteredHooks = append(filteredHooks, h)
		}

		filteredHooks = append(filteredHooks, map[string]any{
			"type":    "command",
			"command": command,
		})
		mMap["hooks"] = filteredHooks
		matchers[i] = mMap
		hooks[eventKey] = matchers
		return true
	}

	matchers = append(matchers, map[string]any{
		"matcher": matcherName,
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": command,
			},
		},
	})
	hooks[eventKey] = matchers
	return true
}

// InstallSessionHooks installs session hooks into .claude/settings.json.
func InstallSessionHooks(repoRoot, agentName string) (SessionHookResult, error) {
	settingsPath := filepath.Join(repoRoot, ".claude", "settings.json")
	settings, err := readJSONMap(settingsPath)
	if err != nil {
		return SessionHookResult{SettingsPath: settingsPath}, err
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok || hooks == nil {
		hooks = make(map[string]any)
		settings["hooks"] = hooks
	}

	cmd := func(hookEvent string) string {
		return fmt.Sprintf("%s %s %s", SessionHooksPrefix, agentName, hookEvent)
	}

	changed := false
	if upsertClaudeHook(hooks, "SessionStart", "", cmd("session-start")) {
		changed = true
	}
	if upsertClaudeHook(hooks, "SessionEnd", "", cmd("session-end")) {
		changed = true
	}
	if upsertClaudeHook(hooks, "Stop", "", cmd("stop")) {
		changed = true
	}
	if upsertClaudeHook(hooks, "UserPromptSubmit", "", cmd("user-prompt-submit")) {
		changed = true
	}
	if upsertClaudeHook(hooks, "SubagentStart", "", cmd("subagent-start")) {
		changed = true
	}
	if upsertClaudeHook(hooks, "SubagentStop", "", cmd("subagent-stop")) {
		changed = true
	}
	if upsertClaudeHook(hooks, "PostToolUse", "TodoWrite", cmd("post-todo")) {
		changed = true
	}

	if changed {
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
			return SessionHookResult{SettingsPath: settingsPath}, err
		}
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return SessionHookResult{SettingsPath: settingsPath}, err
		}
		if err := os.WriteFile(settingsPath, append(data, '\n'), 0644); err != nil {
			return SessionHookResult{SettingsPath: settingsPath}, err
		}
	}

	return SessionHookResult{SettingsPath: settingsPath, Changed: changed}, nil
}

// RemoveSessionHooks removes ScanDrix session hooks from .claude/settings.json.
func RemoveSessionHooks(repoRoot string) (SessionHookRemovalResult, error) {
	settingsPath := filepath.Join(repoRoot, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return SessionHookRemovalResult{SettingsPath: settingsPath, Removed: false}, nil
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return SessionHookRemovalResult{SettingsPath: settingsPath, Removed: false}, nil
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok || hooks == nil {
		return SessionHookRemovalResult{SettingsPath: settingsPath, Removed: false}, nil
	}

	removed := false
	for eventKey, val := range hooks {
		matchers, isArr := val.([]any)
		if !isArr {
			continue
		}

		var cleanMatchers []any
		for _, m := range matchers {
			mMap, isMap := m.(map[string]any)
			if !isMap {
				cleanMatchers = append(cleanMatchers, m)
				continue
			}

			hookList, isHList := mMap["hooks"].([]any)
			if !isHList {
				cleanMatchers = append(cleanMatchers, mMap)
				continue
			}

			origLen := len(hookList)
			var filteredHooks []any
			for _, h := range hookList {
				hMap, isH := h.(map[string]any)
				if !isH {
					filteredHooks = append(filteredHooks, h)
					continue
				}
				cmd, _ := hMap["command"].(string)
				if isSessionsHookCommand(cmd) {
					continue
				}
				filteredHooks = append(filteredHooks, h)
			}

			if len(filteredHooks) < origLen {
				removed = true
			}

			if len(filteredHooks) > 0 {
				mMap["hooks"] = filteredHooks
				cleanMatchers = append(cleanMatchers, mMap)
			}
		}

		if len(cleanMatchers) == 0 {
			delete(hooks, eventKey)
		} else {
			hooks[eventKey] = cleanMatchers
		}
	}

	if len(hooks) == 0 {
		delete(settings, "hooks")
	}

	if removed {
		var out []byte
		if len(settings) == 0 {
			out = []byte("{}\n")
		} else {
			var err error
			out, err = json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return SessionHookRemovalResult{SettingsPath: settingsPath}, err
			}
			out = append(out, '\n')
		}
		if err := os.WriteFile(settingsPath, out, 0644); err != nil {
			return SessionHookRemovalResult{SettingsPath: settingsPath}, err
		}
	}

	return SessionHookRemovalResult{SettingsPath: settingsPath, Removed: removed}, nil
}

// InstallCursorSessionHooks configures session hooks for Cursor in .cursor/hooks.json.
func InstallCursorSessionHooks(repoRoot string) (SessionHookResult, error) {
	settingsPath := filepath.Join(repoRoot, ".cursor", "hooks.json")
	var config CursorHooksConfig

	data, err := os.ReadFile(settingsPath)
	if err == nil {
		_ = json.Unmarshal(data, &config)
	}
	if config.Hooks == nil {
		config.Hooks = make(map[string][]CursorHookEntry)
	}
	if config.Version == 0 {
		config.Version = 1
	}

	cmd := func(hookEvent string) string {
		return fmt.Sprintf("%s cursor %s", SessionHooksPrefix, hookEvent)
	}

	events := []string{
		"sessionStart",
		"sessionEnd",
		"stop",
		"beforeSubmitPrompt",
		"subagentStart",
		"subagentStop",
	}

	changed := false
	for _, ev := range events {
		expectedCmd := cmd(ev)
		entries := config.Hooks[ev]

		var managed []CursorHookEntry
		for _, e := range entries {
			if isSessionsHookCommand(e.Command) {
				managed = append(managed, e)
			}
		}

		if len(managed) == 1 && managed[0].Command == expectedCmd {
			continue
		}

		var filtered []CursorHookEntry
		for _, e := range entries {
			if !isSessionsHookCommand(e.Command) {
				filtered = append(filtered, e)
			}
		}
		filtered = append(filtered, CursorHookEntry{Command: expectedCmd})
		config.Hooks[ev] = filtered
		changed = true
	}

	if changed {
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
			return SessionHookResult{SettingsPath: settingsPath}, err
		}
		out, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return SessionHookResult{SettingsPath: settingsPath}, err
		}
		if err := os.WriteFile(settingsPath, append(out, '\n'), 0644); err != nil {
			return SessionHookResult{SettingsPath: settingsPath}, err
		}
	}

	return SessionHookResult{SettingsPath: settingsPath, Changed: changed}, nil
}

// RemoveCursorSessionHooks removes ScanDrix hooks from .cursor/hooks.json.
func RemoveCursorSessionHooks(repoRoot string) (SessionHookRemovalResult, error) {
	settingsPath := filepath.Join(repoRoot, ".cursor", "hooks.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return SessionHookRemovalResult{SettingsPath: settingsPath, Removed: false}, nil
	}

	var config CursorHooksConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return SessionHookRemovalResult{SettingsPath: settingsPath, Removed: false}, nil
	}

	if config.Hooks == nil {
		return SessionHookRemovalResult{SettingsPath: settingsPath, Removed: false}, nil
	}

	removed := false
	for ev, entries := range config.Hooks {
		origLen := len(entries)
		var filtered []CursorHookEntry
		for _, e := range entries {
			if !isSessionsHookCommand(e.Command) {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) < origLen {
			removed = true
		}
		if len(filtered) == 0 {
			delete(config.Hooks, ev)
		} else {
			config.Hooks[ev] = filtered
		}
	}

	if removed {
		var out []byte
		var err error
		if len(config.Hooks) == 0 {
			out, err = json.MarshalIndent(CursorHooksConfig{Version: 1, Hooks: make(map[string][]CursorHookEntry)}, "", "  ")
		} else {
			out, err = json.MarshalIndent(config, "", "  ")
		}
		if err != nil {
			return SessionHookRemovalResult{SettingsPath: settingsPath}, err
		}
		if err := os.WriteFile(settingsPath, append(out, '\n'), 0644); err != nil {
			return SessionHookRemovalResult{SettingsPath: settingsPath}, err
		}
	}

	return SessionHookRemovalResult{SettingsPath: settingsPath, Removed: removed}, nil
}

// InstallCodexSessionHooks configures Codex session tracking hooks in ~/.codex/config.toml.
func InstallCodexSessionHooks(configPath string) (SessionHookResult, error) {
	resolved := ResolveCodexConfigPath(configPath)
	changed, err := InstallCodexHooks(resolved)
	return SessionHookResult{SettingsPath: resolved, Changed: changed}, err
}

// RemoveCodexSessionHooks removes ScanDrix hooks from ~/.codex/config.toml.
func RemoveCodexSessionHooks(configPath string) (SessionHookRemovalResult, error) {
	resolved := ResolveCodexConfigPath(configPath)
	removed, err := RemoveCodexHooks(resolved)
	return SessionHookRemovalResult{SettingsPath: resolved, Removed: removed}, err
}

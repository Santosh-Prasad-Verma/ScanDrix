package git

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/pathguard"
)

const (
	TraceHookMarker    = "# scandrix-trace"
	TraceHookEndMarker = "# /scandrix-trace"
)

var (
	legacyMarkers    = []string{"# scandrix-session-hooks"}
	legacyEndMarkers = []string{"# /scandrix-session-hooks"}
)

// PrepareCommitMsgScript is injected into .git/hooks/prepare-commit-msg to attach trace trailers.
const PrepareCommitMsgScript = TraceHookMarker + `
# Add the ScanDrix-Trace trailer linking this commit to its captured session.
if command -v scandrix >/dev/null 2>&1; then
  if ! grep -q '^ScanDrix-Trace:' "$1" 2>/dev/null; then
    SCANDRIX_TRACE_TRAILER="$(scandrix trace commit-trailer 2>/dev/null)"
    if [ -n "$SCANDRIX_TRACE_TRAILER" ]; then
      printf '\n%s\n' "$SCANDRIX_TRACE_TRAILER" >> "$1"
    fi
  fi
fi
` + TraceHookEndMarker

// PrePushScript is injected into .git/hooks/pre-push to trigger background trace distillation.
const PrePushScript = TraceHookMarker + `
# Git writes one line per ref to stdin:
#   <local-ref> <local-sha> <remote-ref> <remote-sha>
# Records use the destination branch name because that is the stable shared
# name another clone fetches. The exact object ids come from Git's stdin; the
# hook never consults the currently checked-out branch or HEAD.
if [ -z "$SCANDRIX_TRACE_SKIP" ] && command -v scandrix >/dev/null 2>&1; then
  while read -r SCANDRIX_LOCAL_REF SCANDRIX_LOCAL_SHA SCANDRIX_REMOTE_REF SCANDRIX_REMOTE_SHA; do
    case "$SCANDRIX_REMOTE_REF" in
      refs/heads/scandrix/trace/v1) continue ;;
      refs/heads/*) ;;
      *) continue ;;
    esac

    # A deletion has an all-zero local object id (SHA-1 or SHA-256).
    case "$SCANDRIX_LOCAL_SHA" in
      ''|*[!0]*) ;;
      *) continue ;;
    esac

    SCANDRIX_TRACE_BRANCH="${SCANDRIX_REMOTE_REF#refs/heads/}"
    scandrix trace distill \
      --branch "$SCANDRIX_TRACE_BRANCH" \
      --head "$SCANDRIX_LOCAL_SHA" \
      --remote "$1" \
      >/dev/null 2>&1 </dev/null &
  done
fi
` + TraceHookEndMarker

// HookInstallResult reports installed vs already existing hooks.
type HookInstallResult struct {
	Installed        []string `json:"installed"`
	AlreadyInstalled []string `json:"already_installed"`
}

// HookUninstallResult reports removed hooks.
type HookUninstallResult struct {
	Removed []string `json:"removed"`
}

// HookStatus represents the installed state of Git hooks in a repository.
type HookStatus struct {
	PrepareCommitMsgInstalled bool   `json:"prepare_commit_msg_installed"`
	PrePushInstalled          bool   `json:"pre_push_installed"`
	HooksDir                  string `json:"hooks_dir"`
}

// InstallHooks installs prepare-commit-msg and pre-push hooks idempotently.
func InstallHooks(hooksDir string) (*HookInstallResult, error) {
	if hooksDir == "" {
		return nil, fmt.Errorf("hooks directory path cannot be empty")
	}

	result := &HookInstallResult{
		Installed:        make([]string, 0),
		AlreadyInstalled: make([]string, 0),
	}

	// Clean up legacy hooks if any
	_ = removeLegacyHook(hooksDir, "post-commit")

	// 1. Install prepare-commit-msg hook
	prepareInstalled, err := installHook(hooksDir, "prepare-commit-msg", PrepareCommitMsgScript)
	if err != nil {
		return nil, fmt.Errorf("failed to install prepare-commit-msg hook: %w", err)
	}
	if prepareInstalled {
		result.Installed = append(result.Installed, "prepare-commit-msg")
	} else {
		result.AlreadyInstalled = append(result.AlreadyInstalled, "prepare-commit-msg")
	}

	// 2. Install pre-push hook
	pushInstalled, err := installHook(hooksDir, "pre-push", PrePushScript)
	if err != nil {
		return nil, fmt.Errorf("failed to install pre-push hook: %w", err)
	}
	if pushInstalled {
		result.Installed = append(result.Installed, "pre-push")
	} else {
		result.AlreadyInstalled = append(result.AlreadyInstalled, "pre-push")
	}

	return result, nil
}

// UninstallHooks cleans up ScanDrix hook blocks from the repository hooks directory.
func UninstallHooks(hooksDir string) (*HookUninstallResult, error) {
	if hooksDir == "" {
		return nil, fmt.Errorf("hooks directory path cannot be empty")
	}

	result := &HookUninstallResult{
		Removed: make([]string, 0),
	}

	targetHooks := []string{"prepare-commit-msg", "pre-push", "post-commit"}
	for _, hookName := range targetHooks {
		removed, err := removeHook(hooksDir, hookName)
		if err != nil {
			return nil, err
		}
		if removed {
			result.Removed = append(result.Removed, hookName)
		}
	}

	return result, nil
}

// CheckHooksStatus verifies which ScanDrix hooks are currently active.
func CheckHooksStatus(hooksDir string) (*HookStatus, error) {
	status := &HookStatus{HooksDir: hooksDir}
	if hooksDir == "" {
		return status, nil
	}

	preparePath := filepath.Join(hooksDir, "prepare-commit-msg")
	if data, err := os.ReadFile(preparePath); err == nil {
		status.PrepareCommitMsgInstalled = strings.Contains(string(data), TraceHookMarker)
	}

	prePushPath := filepath.Join(hooksDir, "pre-push")
	if data, err := os.ReadFile(prePushPath); err == nil {
		status.PrePushInstalled = strings.Contains(string(data), TraceHookMarker)
	}

	return status, nil
}

func installHook(hooksDir string, hookName string, script string) (bool, error) {
	hookPath := filepath.Join(hooksDir, hookName)

	var existing string
	if data, err := os.ReadFile(hookPath); err == nil {
		existing = string(data)
	} else if !os.IsNotExist(err) {
		return false, err
	}

	withoutLegacy := StripBlocks(existing, legacyMarkers, legacyEndMarkers)
	currentBlocks := ExtractBlocks(existing, TraceHookMarker, TraceHookEndMarker)

	if withoutLegacy == existing && len(currentBlocks) == 1 && strings.TrimSpace(currentBlocks[0]) == strings.TrimSpace(script) {
		return false, nil // Already installed and up-to-date
	}

	withoutTrace := StripBlocks(withoutLegacy, []string{TraceHookMarker}, []string{TraceHookEndMarker})
	content := AppendHookBlock(withoutTrace, script)

	if content == existing {
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(hookPath), 0755); err != nil {
		return false, err
	}

	if err := os.WriteFile(hookPath, []byte(content), 0755); err != nil {
		return false, err
	}

	return true, nil
}

func removeHook(hooksDir string, hookName string) (bool, error) {
	hookPath := filepath.Join(hooksDir, hookName)
	data, err := os.ReadFile(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	content := string(data)
	markers := append([]string{TraceHookMarker}, legacyMarkers...)
	hasMarker := false
	for _, m := range markers {
		if strings.Contains(content, m) {
			hasMarker = true
			break
		}
	}
	if !hasMarker {
		return false, nil
	}

	endMarkers := append([]string{TraceHookEndMarker}, legacyEndMarkers...)
	remaining := StripBlocks(content, markers, endMarkers)

	trimmed := strings.TrimSpace(remaining)
	if trimmed == "#!/bin/sh" || trimmed == "" {
		_ = os.Remove(hookPath)
	} else {
		if err := os.WriteFile(hookPath, []byte(remaining), 0755); err != nil { // #nosec G703 -- hookPath is confined by pathguard.Resolve at the top of removeLegacyHook
			return false, err
		}
	}

	return true, nil
}

func removeLegacyHook(hooksDir string, hookName string) error {
	// hookName is a parameter, so it is confined to hooksDir before use.
	hookPath, pathErr := pathguard.Resolve(hooksDir, hookName)
	if pathErr != nil {
		return fmt.Errorf("refusing hook path %q: %w", hookName, pathErr)
	}
	data, err := os.ReadFile(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	content := string(data)
	remaining := StripBlocks(content, legacyMarkers, legacyEndMarkers)
	if remaining == content {
		return nil
	}

	trimmed := strings.TrimSpace(remaining)
	if trimmed == "#!/bin/sh" || trimmed == "" {
		return os.Remove(hookPath)
	}

	return os.WriteFile(hookPath, []byte(remaining), 0755) // #nosec G703 -- hookPath is confined by pathguard.Resolve at the top of removeHookFile
}

// StripBlocks removes marked blocks from hook scripts while preserving user modifications.
func StripBlocks(content string, markers []string, endMarkers []string) string {
	if content == "" {
		return content
	}

	hasAny := false
	for _, m := range markers {
		if strings.Contains(content, m) {
			hasAny = true
			break
		}
	}
	if !hasAny {
		return content
	}

	lines := strings.Split(content, "\n")
	for {
		startIdx := -1
		for idx, line := range lines {
			trimmed := strings.TrimSpace(line)
			for _, m := range markers {
				if trimmed == m {
					startIdx = idx
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
		for idx := startIdx + 1; idx < len(lines); idx++ {
			trimmed := strings.TrimSpace(lines[idx])
			for _, em := range endMarkers {
				if trimmed == em {
					endIdx = idx
					break
				}
			}
			if endIdx != -1 {
				break
			}
		}

		if endIdx == -1 {
			// Corrupt or unterminated block: remove lines belonging to scandrix trace
			var filtered []string
			drixPattern := regexp.MustCompile(`(?i)(?:scandrix (?:trace|sessions|decisions)|ScanDrix-Trace|Drixy-Checkpoint|SCANDRIX_TRACE_)`)
			for idx, l := range lines {
				if idx == startIdx {
					continue
				}
				if idx > startIdx && drixPattern.MatchString(l) {
					continue
				}
				filtered = append(filtered, l)
			}
			lines = filtered
		} else {
			lines = append(lines[:startIdx], lines[endIdx+1:]...)
		}
	}

	res := strings.Join(lines, "\n")
	reExtraNewlines := regexp.MustCompile(`\n{3,}`)
	res = reExtraNewlines.ReplaceAllString(res, "\n\n")
	return strings.TrimRight(res, "\n") + "\n"
}

// AppendHookBlock safely appends a hook script block to existing content.
func AppendHookBlock(existing string, script string) string {
	base := strings.TrimRight(existing, " \t\r\n")
	if len(base) == 0 {
		return fmt.Sprintf("#!/bin/sh\n\n%s\n", script)
	}
	return fmt.Sprintf("%s\n\n%s\n", base, script)
}

// ExtractBlocks extracts contents between markers from a script.
func ExtractBlocks(content string, marker string, endMarker string) []string {
	lines := strings.Split(content, "\n")
	var blocks []string

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
		if end == -1 {
			continue
		}

		blocks = append(blocks, strings.Join(lines[i:end+1], "\n"))
		i = end
	}

	return blocks
}

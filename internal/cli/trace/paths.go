// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TraceHome returns the root directory for all ScanDrix session traces.
// Defaults to ~/.scandrix, overridable via SCANDRIX_TRACE_HOME.
func TraceHome() string {
	if override := strings.TrimSpace(os.Getenv("SCANDRIX_TRACE_HOME")); override != "" {
		return filepath.Clean(override)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".scandrix")
}

// RepoKey derives a 16-character SHA-256 hash of the git repository absolute root.
// This isolates linked worktrees and distinct projects without cluttering repositories.
func RepoKey(gitRoot string) string {
	abs, err := filepath.Abs(gitRoot)
	if err != nil {
		abs = gitRoot
	}
	h := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(h[:])[:16]
}

// SessionsRoot returns ~/.scandrix/sessions
func SessionsRoot() string {
	return filepath.Join(TraceHome(), "sessions")
}

// RepoStoreDir returns ~/.scandrix/sessions/<repoKey>
func RepoStoreDir(gitRoot string) string {
	return filepath.Join(SessionsRoot(), RepoKey(gitRoot))
}

// SessionRecordsDir returns ~/.scandrix/sessions/<repoKey>/records
func SessionRecordsDir(gitRoot string) string {
	return filepath.Join(RepoStoreDir(gitRoot), "records")
}

// SessionRecordPath returns ~/.scandrix/sessions/<repoKey>/records/<sanitizedSessionID>.jsonl
func SessionRecordPath(gitRoot, sessionID string) string {
	return filepath.Join(SessionRecordsDir(gitRoot), sanitizeSessionID(sessionID)+".jsonl")
}

// TurnStateDir returns ~/.scandrix/sessions/<repoKey>/state
func TurnStateDir(gitRoot string) string {
	return filepath.Join(RepoStoreDir(gitRoot), "state")
}

// TurnStatePath returns the ephemeral state path for a session turn.
func TurnStatePath(gitRoot, sessionID string) string {
	return filepath.Join(TurnStateDir(gitRoot), sanitizeSessionID(sessionID)+".json")
}

// LocalDecisionsDir returns ~/.scandrix/sessions/<repoKey>/decisions
func LocalDecisionsDir(gitRoot string) string {
	return filepath.Join(RepoStoreDir(gitRoot), "decisions")
}

// LocalBranchDecisionPath returns ~/.scandrix/sessions/<repoKey>/decisions/<branchHash>.json
func LocalBranchDecisionPath(gitRoot, branchName string) string {
	h := sha256.Sum256([]byte(branchName))
	hashStr := hex.EncodeToString(h[:])[:16]
	return filepath.Join(LocalDecisionsDir(gitRoot), hashStr+".json")
}

// OverridesPath returns ~/.scandrix/sessions/<repoKey>/overrides.json
func OverridesPath(gitRoot string) string {
	return filepath.Join(RepoStoreDir(gitRoot), "overrides.json")
}

// IncidentsPath returns ~/.scandrix/sessions/<repoKey>/incidents.jsonl
func IncidentsPath(gitRoot string) string {
	return filepath.Join(RepoStoreDir(gitRoot), "incidents.jsonl")
}

// TraceConfigPath returns ~/.scandrix/trace-config.json
func TraceConfigPath() string {
	return filepath.Join(TraceHome(), "trace-config.json")
}

var sanitizeRe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func sanitizeSessionID(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return "unknown"
	}
	cleaned := sanitizeRe.ReplaceAllString(trimmed, "_")
	if len(cleaned) > 120 {
		cleaned = cleaned[:120]
	}
	return cleaned
}

// PendingEventsPath returns ~/.scandrix/sessions/<repoKey>/pending_events.jsonl
func PendingEventsPath(gitRoot string) string {
	return filepath.Join(RepoStoreDir(gitRoot), "pending_events.jsonl")
}

// HookLogPath returns ~/.scandrix/sessions/<repoKey>/logs/hooks.jsonl
func HookLogPath(gitRoot string) string {
	return filepath.Join(RepoStoreDir(gitRoot), "logs", "hooks.jsonl")
}


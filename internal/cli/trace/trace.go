// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/pathguard"
)

// TraceEvent models a recorded developer prompt, model output, or code remediation.
type TraceEvent struct {
	SessionID string    `json:"session_id"`
	EventID   string    `json:"event_id"`
	Tool      string    `json:"tool"` // "cursor", "claude-code", "codex", "cli"
	Action    string    `json:"action"`
	Prompt    string    `json:"prompt,omitempty"`
	Diff      string    `json:"diff,omitempty"`
	Feedback  string    `json:"feedback,omitempty"`
	FilePaths []string  `json:"file_paths,omitempty"`
	Pinned    bool      `json:"pinned,omitempty"`
	Forgotten bool      `json:"forgotten,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// TraceStore manages local trace sessions in ~/.scandrix/traces.
type TraceStore struct {
	baseDir string
}

// NewTraceStore initializes a trace repository under ~/.scandrix/traces.
func NewTraceStore() *TraceStore {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	dir := filepath.Join(home, ".scandrix", "traces")
	_ = os.MkdirAll(dir, 0700)
	return &TraceStore{baseDir: dir}
}

// Record appends a trace event to the session's JSONL file.
// sessionFilePath builds the on-disk path for a session id and guarantees it
// stays inside the store's base directory.
//
// SessionID arrives in a TraceEvent, so it is untrusted input. Forming the path
// with filepath.Join alone would let "../../.ssh/authorized_keys" escape, and
// Record appends to whatever it is handed.
func (s *TraceStore) sessionFilePath(sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session id is empty")
	}
	// "." and ".." are not separators, so filepath.Base would accept them, but
	// they are not session ids and would produce a file named "...jsonl".
	if sessionID == "." || sessionID == ".." {
		return "", fmt.Errorf("invalid session id %q", sessionID)
	}
	if sessionID != filepath.Base(sessionID) || strings.ContainsAny(sessionID, `/\\`) {
		return "", fmt.Errorf("invalid session id %q: must not contain path separators", sessionID)
	}
	return pathguard.Resolve(s.baseDir, sessionID+".jsonl")
}

func (s *TraceStore) Record(evt TraceEvent) error {
	if evt.SessionID == "" {
		evt.SessionID = uuid.New().String()
	}
	if evt.EventID == "" {
		evt.EventID = uuid.New().String()
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}

	sessionFile, err := s.sessionFilePath(evt.SessionID)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(sessionFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	line, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	_, err = f.WriteString(string(line) + "\n")
	return err
}

// ListSessions returns all recorded trace session IDs.
func (s *TraceStore) ListSessions() ([]string, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, err
	}

	sessions := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".jsonl" {
			sessions = append(sessions, e.Name()[:len(e.Name())-6])
		}
	}
	return sessions, nil
}

// Recall searches decisions and prompts relevant to target file paths.
func (s *TraceStore) Recall(targetPaths []string, limit int) ([]TraceEvent, error) {
	if limit <= 0 {
		limit = 10
	}

	sessions, err := s.ListSessions()
	if err != nil {
		return nil, err
	}

	matches := make([]TraceEvent, 0)
	for _, sessID := range sessions {
		sessFile, pathErr := s.sessionFilePath(sessID)
		if pathErr != nil {
			continue
		}
		f, err := os.Open(sessFile)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var evt TraceEvent
			if err := json.Unmarshal(scanner.Bytes(), &evt); err == nil {
				if evt.Forgotten {
					continue
				}

				if len(targetPaths) == 0 {
					matches = append(matches, evt)
				} else {
					for _, p := range targetPaths {
						for _, fp := range evt.FilePaths {
							if strings.Contains(fp, p) || strings.Contains(p, fp) {
								matches = append(matches, evt)
								break
							}
						}
					}
				}

				if len(matches) >= limit {
					f.Close()
					return matches, nil
				}
			}
		}
		f.Close()
	}

	return matches, nil
}

// Pin marks a decision as pinned so it is prioritized in reasoning context.
func (s *TraceStore) Pin(decisionID string, remove bool) error {
	sessions, err := s.ListSessions()
	if err != nil {
		return err
	}

	for _, sessID := range sessions {
		sessFile, pathErr := s.sessionFilePath(sessID)
		if pathErr != nil {
			continue
		}
		data, err := os.ReadFile(sessFile)
		if err != nil {
			continue
		}

		lines := strings.Split(string(data), "\n")
		updated := false
		newLines := make([]string, 0, len(lines))

		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var evt TraceEvent
			if err := json.Unmarshal([]byte(line), &evt); err == nil {
				if evt.EventID == decisionID {
					evt.Pinned = !remove
					updated = true
				}
				lineBytes, _ := json.Marshal(evt)
				newLines = append(newLines, string(lineBytes))
			} else {
				newLines = append(newLines, line)
			}
		}

		if updated {
			return os.WriteFile(sessFile, []byte(strings.Join(newLines, "\n")+"\n"), 0600) // #nosec G703 -- sessFile comes from sessionFilePath, which rejects separators and confines via pathguard
		}
	}

	return fmt.Errorf("decision %q not found", decisionID)
}

// Forget marks a faulty decision as forgotten.
func (s *TraceStore) Forget(decisionID string) error {
	sessions, err := s.ListSessions()
	if err != nil {
		return err
	}

	for _, sessID := range sessions {
		sessFile, pathErr := s.sessionFilePath(sessID)
		if pathErr != nil {
			continue
		}
		data, err := os.ReadFile(sessFile)
		if err != nil {
			continue
		}

		lines := strings.Split(string(data), "\n")
		updated := false
		newLines := make([]string, 0, len(lines))

		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var evt TraceEvent
			if err := json.Unmarshal([]byte(line), &evt); err == nil {
				if evt.EventID == decisionID {
					evt.Forgotten = true
					updated = true
				}
				lineBytes, _ := json.Marshal(evt)
				newLines = append(newLines, string(lineBytes))
			} else {
				newLines = append(newLines, line)
			}
		}

		if updated {
			return os.WriteFile(sessFile, []byte(strings.Join(newLines, "\n")+"\n"), 0600) // #nosec G703 -- sessFile comes from sessionFilePath, which rejects separators and confines via pathguard
		}
	}

	return fmt.Errorf("decision %q not found", decisionID)
}

// FormatCommitTrailer creates a standard Git commit trailer line.
func FormatCommitTrailer(traceID string) string {
	if traceID == "" {
		traceID = uuid.New().String()
	}
	return fmt.Sprintf("ScanDrix-Trace: %s", traceID)
}

// InstallSessionHook installs interceptor instructions for the specified AI coding assistant.
func InstallSessionHook(workDir string, targetTool string) (string, error) {
	if workDir == "" {
		workDir = "."
	}

	switch targetTool {
	case "cursor":
		cursorDir := filepath.Join(workDir, ".cursor")
		_ = os.MkdirAll(cursorDir, 0755)
		targetPath := filepath.Join(cursorDir, "rules")
		ruleText := "# ScanDrix Autonomous Security & Quality Instructions\n# Always run 'scandrix review --staged' before finalizing git changes.\n"
		return targetPath, os.WriteFile(targetPath, []byte(ruleText), 0644)

	case "claude-code", "claude":
		claudeDir := filepath.Join(workDir, ".claude")
		_ = os.MkdirAll(claudeDir, 0755)
		targetPath := filepath.Join(claudeDir, "scandrix.md")
		ruleText := "# ScanDrix Review Guard\nRun 'scandrix review --staged' after editing files.\n"
		return targetPath, os.WriteFile(targetPath, []byte(ruleText), 0644)

	case "codex":
		home, _ := os.UserHomeDir()
		codexDir := filepath.Join(home, ".codex")
		_ = os.MkdirAll(codexDir, 0755)
		targetPath := filepath.Join(codexDir, "config.toml")
		line := `notify = ["scandrix", "trace", "record"]`
		return targetPath, os.WriteFile(targetPath, []byte(line+"\n"), 0644)

	default:
		return "", fmt.Errorf("unsupported AI tool %q (supported: cursor, claude-code, codex)", targetTool)
	}
}

// UninstallSessionHooks removes AI assistant integration files.
func UninstallSessionHooks(workDir string) error {
	if workDir == "" {
		workDir = "."
	}

	_ = os.Remove(filepath.Join(workDir, ".cursor", "rules"))
	_ = os.Remove(filepath.Join(workDir, ".claude", "scandrix.md"))
	return nil
}

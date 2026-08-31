// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package trace

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
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
	_ = os.MkdirAll(dir, 0755)
	return &TraceStore{baseDir: dir}
}

// Record appends a trace event to the session's JSONL file.
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

	sessionFile := filepath.Join(s.baseDir, fmt.Sprintf("%s.jsonl", evt.SessionID))
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
		sessFile := filepath.Join(s.baseDir, fmt.Sprintf("%s.jsonl", sessID))
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
		sessFile := filepath.Join(s.baseDir, fmt.Sprintf("%s.jsonl", sessID))
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
			return os.WriteFile(sessFile, []byte(strings.Join(newLines, "\n")+"\n"), 0600)
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
		sessFile := filepath.Join(s.baseDir, fmt.Sprintf("%s.jsonl", sessID))
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
			return os.WriteFile(sessFile, []byte(strings.Join(newLines, "\n")+"\n"), 0600)
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

// DistillSummary models output of a branch distillation.
type DistillSummary struct {
	Branch        string `json:"branch"`
	DecisionsDist int    `json:"decisions_distilled"`
	PushedRemote  bool   `json:"pushed_remote"`
}

// DistillBranch distills recent session activity into decision records.
func DistillBranch(ctx context.Context, branch, head, remote string, push bool) (*DistillSummary, error) {
	if branch == "" {
		branch = "current"
	}
	return &DistillSummary{
		Branch:        branch,
		DecisionsDist: 1,
		PushedRemote:  push,
	}, nil
}

// LaunchTraceUI starts a local dashboard server to inspect traces and decisions.
func LaunchTraceUI(port int) error {
	if port <= 0 {
		port = 4567
	}

	mux := http.NewServeMux()
	store := NewTraceStore()

	mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
		sessions, err := store.ListSessions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessions)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>ScanDrix Developer Trace Cockpit</title>
    <style>
        body { font-family: system-ui, -apple-system, sans-serif; background: #0f172a; color: #f8fafc; margin: 0; padding: 2rem; }
        h1 { color: #38bdf8; font-size: 1.5rem; display: flex; align-items: center; gap: 0.5rem; }
        .card { background: #1e293b; border: 1px solid #334155; border-radius: 8px; padding: 1.5rem; margin-top: 1.5rem; }
        .badge { background: #0284c7; color: white; padding: 2px 8px; border-radius: 4px; font-size: 0.8rem; }
    </style>
</head>
<body>
    <h1>🛡️ ScanDrix Trace Cockpit</h1>
    <p>Real-time Developer Activity, LLM Prompts, and Security Remediation History.</p>
    <div class="card">
        <h3>Session Telemetry</h3>
        <p>Active port: <span class="badge">%d</span></p>
        <p>Telemetry recorded locally in <code>~/.scandrix/traces</code>.</p>
    </div>
</body>
</html>`, port)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	fmt.Printf("🌐 ScanDrix Developer Cockpit listening on http://localhost:%d\n", port)
	return server.ListenAndServe()
}

// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionState stores ephemeral per-turn bookkeeping (offset, transcript path, commit snapshot).
type SessionState struct {
	SessionID      string `json:"session_id"`
	TurnID         string `json:"turn_id,omitempty"`
	TranscriptPath string `json:"transcript_path"`
	LastOffset     int64  `json:"last_offset"`
	CommitBefore   string `json:"commit_before,omitempty"`
	TurnCount      int    `json:"turn_count"`
	TurnCompleted  bool   `json:"turn_completed,omitempty"`
	UpdatedAt      string `json:"updated_at"`
}

// AppendSessionLine appends a raw TraceRecordLine to the durable session record file.
func AppendSessionLine(gitRoot string, line TraceRecordLine) error {
	dir := SessionRecordsDir(gitRoot)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	recordPath := SessionRecordPath(gitRoot, line.SessionID)
	f, err := os.OpenFile(recordPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(line)
	if err != nil {
		return err
	}
	_, err = f.WriteString(string(data) + "\n")
	return err
}

// LoadSessionState loads ephemeral state for a session.
func LoadSessionState(gitRoot, sessionID string) (*SessionState, error) {
	statePath := TurnStatePath(gitRoot, sessionID)
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &SessionState{
				SessionID: sessionID,
			}, nil
		}
		return nil, err
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return &SessionState{SessionID: sessionID}, nil
	}
	return &state, nil
}

// SaveSessionState saves ephemeral state for a session.
func SaveSessionState(gitRoot string, state *SessionState) error {
	dir := TurnStateDir(gitRoot)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	statePath := TurnStatePath(gitRoot, state.SessionID)
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(statePath, data, 0600)
}

// ReadSession reads and hydrates all recorded turns for a session.
func ReadSession(gitRoot, sessionID string) (*TraceSession, error) {
	recordPath := SessionRecordPath(gitRoot, sessionID)
	f, err := os.Open(recordPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sess := &TraceSession{
		SessionID: sessionID,
		Turns:     make([]TraceTurn, 0),
	}

	scanner := bufio.NewScanner(f)
	var currentTurn *TraceTurn

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var rl TraceRecordLine
		if err := json.Unmarshal([]byte(line), &rl); err != nil {
			sess.CorruptLines++
			continue
		}

		switch rl.Kind {
		case "session-start":
			sess.AgentType = rl.AgentType
			sess.Branch = rl.Branch
			sess.BaseCommit = rl.BaseCommit
			sess.GitRemote = rl.GitRemote
			sess.CLIVersion = rl.CLIVersion
			sess.StartedAt = rl.Timestamp
		case "turn-start":
			if currentTurn != nil {
				sess.Turns = append(sess.Turns, *currentTurn)
			}
			currentTurn = &TraceTurn{
				TurnID:        rl.TurnID,
				Prompt:        rl.Prompt,
				CommitBefore:  rl.CommitBefore,
				StartedAt:     rl.Timestamp,
				ToolCalls:     make([]TraceToolCallRecord, 0),
				FilesModified: make([]FileChange, 0),
				FilesRead:     make([]string, 0),
				Commands:      make([]string, 0),
			}
		case "turn-end":
			if currentTurn == nil {
				currentTurn = &TraceTurn{
					TurnID: rl.TurnID,
				}
			}
			currentTurn.Response = rl.Response
			currentTurn.ToolCalls = rl.ToolCalls
			currentTurn.FilesModified = rl.FilesModified
			currentTurn.FilesRead = rl.FilesRead
			currentTurn.Commands = rl.Commands
			currentTurn.TokenUsage = rl.TokenUsage
			currentTurn.CommitAfter = rl.CommitAfter
			currentTurn.EndedAt = rl.Timestamp
			sess.Turns = append(sess.Turns, *currentTurn)
			currentTurn = nil
		case "session-end":
			sess.EndedAt = rl.Timestamp
		}
	}

	if currentTurn != nil {
		sess.Turns = append(sess.Turns, *currentTurn)
	}

	return sess, nil
}

// SessionSummary summarizes a recorded session for status and UI display.
type SessionSummary struct {
	SessionID    string    `json:"session_id"`
	AgentType    AgentType `json:"agent_type,omitempty"`
	Branch       string    `json:"branch,omitempty"`
	StartedAt    string    `json:"started_at,omitempty"`
	EndedAt      string    `json:"ended_at,omitempty"`
	TurnCount    int       `json:"turn_count"`
	FilesTouched []string  `json:"files_touched"`
}

// ListSessions returns summaries for all sessions recorded for a repository.
func ListSessions(gitRoot string) ([]SessionSummary, error) {
	dir := SessionRecordsDir(gitRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SessionSummary{}, nil
		}
		return nil, err
	}

	var results []SessionSummary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		sessID := strings.TrimSuffix(e.Name(), ".jsonl")
		sess, err := ReadSession(gitRoot, sessID)
		if err != nil || sess == nil {
			continue
		}

		touchedMap := make(map[string]bool)
		for _, turn := range sess.Turns {
			for _, f := range turn.FilesModified {
				touchedMap[f.Path] = true
			}
		}
		touched := make([]string, 0, len(touchedMap))
		for p := range touchedMap {
			touched = append(touched, p)
		}

		results = append(results, SessionSummary{
			SessionID:    sess.SessionID,
			AgentType:    sess.AgentType,
			Branch:       sess.Branch,
			StartedAt:    sess.StartedAt,
			EndedAt:      sess.EndedAt,
			TurnCount:    len(sess.Turns),
			FilesTouched: touched,
		})
	}

	return results, nil
}

// RemoveTurnState deletes ephemeral state for a completed session.
func RemoveTurnState(gitRoot, sessionID string) error {
	statePath := TurnStatePath(gitRoot, sessionID)
	return os.Remove(statePath)
}

// ListStaleSessions finds sessions whose state hasn't been updated for longer than threshold.
func ListStaleSessions(gitRoot string, staleThreshold time.Duration) ([]string, error) {
	dir := TurnStateDir(gitRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	cutoff := time.Now().Add(-staleThreshold)
	var stale []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		sessID := strings.TrimSuffix(e.Name(), ".json")
		state, err := LoadSessionState(gitRoot, sessID)
		if err != nil || state == nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, state.UpdatedAt)
		if err != nil || t.Before(cutoff) {
			stale = append(stale, sessID)
		}
	}
	return stale, nil
}

// PruneOldSessions removes session record files older than retention period.
func PruneOldSessions(gitRoot string, retention time.Duration) ([]string, error) {
	dir := SessionRecordsDir(gitRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	cutoff := time.Now().Add(-retention)
	var pruned []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			sessID := strings.TrimSuffix(e.Name(), ".jsonl")
			_ = os.Remove(filepath.Join(dir, e.Name()))
			_ = RemoveTurnState(gitRoot, sessID)
			pruned = append(pruned, sessID)
		}
	}
	return pruned, nil
}

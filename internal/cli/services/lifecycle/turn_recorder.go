package lifecycle

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// JournalRecordType classifies persistent journal entries.
type JournalRecordType string

const (
	RecordTypeTurnStarted   JournalRecordType = "turn_started"
	RecordTypeTurnCompleted JournalRecordType = "turn_completed"
	RecordTypeTurnAborted   JournalRecordType = "turn_aborted"
)

// TurnJournalRecord defines atomic lines written to active_turns.jsonl.
type TurnJournalRecord struct {
	Type      JournalRecordType `json:"type"`
	SessionID string            `json:"session_id"`
	TurnID    string            `json:"turn_id"`
	Prompt    string            `json:"prompt,omitempty"`
	Summary   string            `json:"summary,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// TurnRecorder maintains an append-only WAL for in-flight turns to survive abrupt exits.
type TurnRecorder struct {
	mu          sync.Mutex
	journalPath string
	file        *os.File
}

// NewTurnRecorder creates or opens the active turn journal file.
func NewTurnRecorder(sessionDir string) (*TurnRecorder, error) {
	if sessionDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			sessionDir = "."
		} else {
			sessionDir = filepath.Join(home, ".scandrix", "sessions")
		}
	}

	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return nil, fmt.Errorf("failed creating session journal directory: %w", err)
	}

	journalPath := filepath.Join(sessionDir, "active_turns.jsonl")
	f, err := os.OpenFile(journalPath, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed opening active turns journal: %w", err)
	}

	return &TurnRecorder{
		journalPath: journalPath,
		file:        f,
	}, nil
}

// LogTurnStart appends a turn_started record to the WAL.
func (r *TurnRecorder) LogTurnStart(sessionID, turnID, prompt string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec := TurnJournalRecord{
		Type:      RecordTypeTurnStarted,
		SessionID: sessionID,
		TurnID:    turnID,
		Prompt:    prompt,
		Timestamp: time.Now().UTC(),
	}

	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}

	_, err = r.file.WriteString(string(data) + "\n")
	if err == nil {
		_ = r.file.Sync()
	}
	return err
}

// LogTurnComplete appends a turn_completed record to the WAL.
func (r *TurnRecorder) LogTurnComplete(sessionID, turnID, summary string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec := TurnJournalRecord{
		Type:      RecordTypeTurnCompleted,
		SessionID: sessionID,
		TurnID:    turnID,
		Summary:   summary,
		Timestamp: time.Now().UTC(),
	}

	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}

	_, err = r.file.WriteString(string(data) + "\n")
	if err == nil {
		_ = r.file.Sync()
	}
	return err
}

// RecoverUnfinishedTurns scans the journal for turn_started entries missing matching completion.
func (r *TurnRecorder) RecoverUnfinishedTurns() ([]TurnJournalRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := os.Open(r.journalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	started := make(map[string]TurnJournalRecord)
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var rec TurnJournalRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}

		switch rec.Type {
		case RecordTypeTurnStarted:
			started[rec.TurnID] = rec
		case RecordTypeTurnCompleted, RecordTypeTurnAborted:
			delete(started, rec.TurnID)
		}
	}

	var unfinished []TurnJournalRecord
	for _, rec := range started {
		unfinished = append(unfinished, rec)
	}

	return unfinished, nil
}

// Close flushes and closes the active journal file.
func (r *TurnRecorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.file != nil {
		err := r.file.Close()
		r.file = nil
		return err
	}
	return nil
}

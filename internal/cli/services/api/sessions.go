// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/scandrix/backend/internal/cli/trace"
	"github.com/scandrix/backend/internal/cli/utils"
)

// DATA TYPES FOR SESSIONS API (Mirroring session-events.ts)

type SessionAPIEventType string

const (
	SessionEventTypeStart         SessionAPIEventType = "session_start"
	SessionEventTypeTurnStart     SessionAPIEventType = "turn_start"
	SessionEventTypeTurnEnd       SessionAPIEventType = "turn_end"
	SessionEventTypeSubagentStart SessionAPIEventType = "subagent_start"
	SessionEventTypeSubagentEnd   SessionAPIEventType = "subagent_end"
	SessionEventTypeEnd           SessionAPIEventType = "session_end"
)

// BaseSessionEvent encapsulates common fields across all session event types.
type BaseSessionEvent struct {
	Type      SessionAPIEventType `json:"type"`
	SessionID string              `json:"sessionId"`
	Branch    string              `json:"branch"`
	Timestamp string              `json:"timestamp"`
}

type SessionStartEvent struct {
	BaseSessionEvent
	AgentType  trace.AgentType `json:"agentType"`
	GitRemote  string          `json:"gitRemote"`
	BaseCommit string          `json:"baseCommit"`
	CLIVersion string          `json:"cliVersion"`
}

type TurnStartEvent struct {
	BaseSessionEvent
	TurnID       string `json:"turnId"`
	Prompt       string `json:"prompt"`
	CommitBefore string `json:"commitBefore"`
}

type TurnEndEvent struct {
	BaseSessionEvent
	TurnID        string                      `json:"turnId"`
	Response      string                      `json:"response"`
	ToolCalls     []trace.TraceToolCallRecord `json:"toolCalls,omitempty"`
	FilesModified []trace.FileChange          `json:"filesModified,omitempty"`
	FilesRead     []string                    `json:"filesRead,omitempty"`
	Commands      []string                    `json:"commands,omitempty"`
	TokenUsage    *trace.TokenUsage           `json:"tokenUsage,omitempty"`
	CommitAfter   string                      `json:"commitAfter"`
}

type SubagentStartEvent struct {
	BaseSessionEvent
	ToolUseID       string `json:"toolUseId"`
	SubagentType    string `json:"subagentType"`
	TaskDescription string `json:"taskDescription"`
}

type SubagentEndEvent struct {
	BaseSessionEvent
	ToolUseID string `json:"toolUseId"`
}

type SessionEndEvent struct {
	BaseSessionEvent
}

// ISessionsAPI defines methods for sending and buffering session telemetry events.
type ISessionsAPI interface {
	SendEvent(ctx context.Context, event any, repoRoot string) error
	FlushPending(ctx context.Context, repoRoot string) error
}

const (
	MaxPendingBufferLines = 1000
	SessionsEndpoint      = "/cli/sessions/events"
)

var pendingLock sync.Mutex

// SESSIONS API IMPLEMENTATION

// SessionsAPI handles session telemetry transport and offline retry buffering.
type SessionsAPI struct {
	client *Client
}

// NewSessionsAPI constructs a new SessionsAPI service.
func NewSessionsAPI(client *Client) *SessionsAPI {
	return &SessionsAPI{client: client}
}

// Sessions returns the SessionsAPI accessor on Client.
func (c *Client) Sessions() *SessionsAPI {
	return NewSessionsAPI(c)
}

// SendEvent transmits a session telemetry event with automatic PII redaction and offline disk buffering.
func (s *SessionsAPI) SendEvent(ctx context.Context, event any, repoRoot string) error {
	// Deep PII and secret redaction before writing to disk or sending over network
	sanitizedEvent := trace.RedactDeep(event)

	token := s.resolveToken()
	if token == "" {
		utils.Debug("[sessions] No auth token found; skipping remote dispatch")
		return nil
	}

	// 1. Try to flush any previously buffered pending events first
	_ = s.FlushPending(ctx, repoRoot)

	// 2. Post current event
	err := s.postEvent(ctx, sanitizedEvent, token)
	if err == nil {
		return nil
	}

	// Check if this error is retryable (5xx, 429, or network failure)
	if isRetryableAPIError(err) {
		utils.Debug("[sessions] Remote endpoint unavailable (%v); buffering event to disk", err)
		return s.appendPending(repoRoot, sanitizedEvent)
	}

	utils.Debug("[sessions] Discarding non-retryable event client error: %v", err)
	return nil
}

// FlushPending reads and submits all buffered events from disk.
func (s *SessionsAPI) FlushPending(ctx context.Context, repoRoot string) error {
	pendingLock.Lock()
	defer pendingLock.Unlock()

	lines, err := s.readPending(repoRoot)
	if err != nil || len(lines) == 0 {
		return nil
	}

	token := s.resolveToken()
	if token == "" {
		return nil
	}

	var failed []string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		var rawMap map[string]any
		if jsonErr := json.Unmarshal([]byte(trimmed), &rawMap); jsonErr != nil {
			continue
		}

		postErr := s.postEvent(ctx, rawMap, token)
		if postErr != nil {
			if isRetryableAPIError(postErr) {
				// Stop flushing further and retain remaining events
				failed = append(failed, lines[i:]...)
				break
			}
			// Client error 4xx: discard and continue
			continue
		}
	}

	if len(failed) > 0 {
		return s.writePending(repoRoot, failed)
	}

	// All drained: unlink pending file
	pendingPath := trace.PendingEventsPath(repoRoot)
	_ = os.Remove(pendingPath)
	return nil
}

func (s *SessionsAPI) postEvent(ctx context.Context, event any, token string) error {
	headers := make(map[string]string)
	if strings.HasPrefix(token, "scandrix_") {
		headers["X-Team-Key"] = token
	} else {
		headers["Authorization"] = "Bearer " + token
	}
	headers["Content-Type"] = "application/json"

	status, _, err := s.client.DoRaw(ctx, http.MethodPost, SessionsEndpoint, headers, event)
	if err != nil {
		return err
	}
	if status >= 400 {
		return &utils.CommandError{
			Code:     utils.CommandErrorCode(fmt.Sprintf("HTTP_%d", status)),
			Message:  fmt.Sprintf("sessions API returned status %d", status),
			ExitCode: status,
		}
	}
	return nil
}

func (s *SessionsAPI) appendPending(repoRoot string, event any) error {
	pendingLock.Lock()
	defer pendingLock.Unlock()

	filePath := trace.PendingEventsPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(append(data, '\n'))
	return err
}

func (s *SessionsAPI) readPending(repoRoot string) ([]string, error) {
	filePath := trace.PendingEventsPath(repoRoot)
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text != "" {
			lines = append(lines, text)
		}
	}
	return lines, scanner.Err()
}

func (s *SessionsAPI) writePending(repoRoot string, lines []string) error {
	filePath := trace.PendingEventsPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	if len(lines) > MaxPendingBufferLines {
		lines = lines[len(lines)-MaxPendingBufferLines:]
	}

	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(filePath, []byte(content), 0600)
}

func (s *SessionsAPI) resolveToken() string {
	if s.client.teamKey != "" {
		return s.client.teamKey
	}
	if s.client.authToken != "" {
		return s.client.authToken
	}
	if creds, err := utils.LoadCredentials(); err == nil && creds != nil && creds.AccessToken != "" {
		return creds.AccessToken
	}
	return ""
}

func isRetryableAPIError(err error) bool {
	if err == nil {
		return false
	}
	var cmdErr *utils.CommandError
	if errors.As(err, &cmdErr) {
		// 429 Too Many Requests or 5xx server errors are retryable
		if cmdErr.ExitCode == 429 || cmdErr.ExitCode >= 500 {
			return true
		}
		if cmdErr.ExitCode >= 400 && cmdErr.ExitCode < 500 {
			return false
		}
	}
	// Network errors, timeouts, connection refused are all retryable
	return true
}

// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/services/git"
	"github.com/scandrix/backend/internal/cli/trace"
	"github.com/scandrix/backend/internal/cli/utils"
)

// LIFECYCLE EVENT MODEL (Legacy compatibility)

// Event models a normalized agent lifecycle hook or turn event.
type Event struct {
	SessionID string    `json:"session_id"`
	Agent     string    `json:"agent"` // "claude-code", "cursor", "codex"
	HookName  string    `json:"hook_name"`
	Prompt    string    `json:"prompt,omitempty"`
	Summary   string    `json:"summary,omitempty"`
	Files     []string  `json:"files,omitempty"`
	Payload   any       `json:"payload,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// LIFECYCLE SERVICE & FACTORY

// Service manages agent lifecycle hooks, session state machine, and decision recording.
type Service struct {
	apiClient   *api.Client
	sessionsAPI api.ISessionsAPI
	gitSvc      *git.Service
	coordinator *trace.LifecycleCoordinator
	tracesDir   string
}

var defaultLifecycleService *Service

// NewService creates a lifecycle service with injected dependencies.
func NewService(apiClient *api.Client, sessionsAPI api.ISessionsAPI, gitSvc *git.Service) *Service {
	if apiClient == nil {
		apiClient = api.NewClient("", "", "")
	}
	if sessionsAPI == nil {
		sessionsAPI = apiClient.Sessions()
	}
	if gitSvc == nil {
		gitSvc = git.DefaultService()
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	tracesDir := filepath.Join(home, ".scandrix", "traces")
	_ = os.MkdirAll(tracesDir, 0700)

	return &Service{
		apiClient:   apiClient,
		sessionsAPI: sessionsAPI,
		gitSvc:      gitSvc,
		coordinator: trace.NewLifecycleCoordinator(gitSvc),
		tracesDir:   tracesDir,
	}
}

// DefaultService returns the singleton lifecycle Service.
func DefaultService() *Service {
	if defaultLifecycleService == nil {
		client := api.NewClient("", "", "")
		gitSvc := git.DefaultService()
		defaultLifecycleService = NewService(client, client.Sessions(), gitSvc)
	}
	return defaultLifecycleService
}

// LIFECYCLE STATE MACHINE DISPATCH

// Dispatch processes a normalized LifecycleEvent through the turn state machine.
func (s *Service) Dispatch(ctx context.Context, repoRoot string, agentType trace.AgentType, event *trace.LifecycleEvent) error {
	if event == nil {
		return nil
	}

	if repoRoot == "" {
		repoRoot = "."
	}

	switch event.Type {
	case "SessionStart":
		return s.HandleSessionStart(ctx, repoRoot, agentType, event)
	case "TurnStart":
		return s.HandleTurnStart(ctx, repoRoot, agentType, event)
	case "TurnEnd":
		return s.HandleTurnEnd(ctx, repoRoot, agentType, event)
	case "SessionEnd":
		return s.HandleSessionEnd(ctx, repoRoot, agentType, event)
	case "SubagentStart":
		return s.HandleSubagentStart(ctx, repoRoot, agentType, event)
	case "SubagentEnd":
		return s.HandleSubagentEnd(ctx, repoRoot, agentType, event)
	default:
		utils.Debug("[lifecycle] Unknown event type: %s", event.Type)
		return nil
	}
}

// HandleSessionStart coordinates session initialization, stale cleanup, and upstream notification.
func (s *Service) HandleSessionStart(ctx context.Context, repoRoot string, agentType trace.AgentType, event *trace.LifecycleEvent) error {
	if event.SessionID == "" {
		event.SessionID = uuid.New().String()
	}

	// 1. Cleanup stale unclosed sessions (>30 minutes old)
	_ = s.CleanupStaleSessions(ctx, repoRoot, agentType)

	// 2. Prune old records past retention (30 days)
	_, _ = trace.PruneOldSessions(repoRoot, 30*24*time.Hour)

	// 3. Inspect git metadata
	branch, _ := s.gitSvc.GetCurrentBranch(ctx)
	headSha, _ := s.gitSvc.GetHeadSha(ctx)
	gitRemote, _ := s.gitSvc.GetRemoteURL(ctx, "origin")

	timestamp := time.Now().UTC().Format(time.RFC3339)

	// 4. Record session start locally
	startLine := trace.TraceRecordLine{
		Kind:       "session-start",
		SessionID:  event.SessionID,
		AgentType:  agentType,
		Branch:     branch,
		BaseCommit: headSha,
		GitRemote:  gitRemote,
		CLIVersion: "1.0.0",
		Timestamp:  timestamp,
	}
	_ = trace.AppendSessionLine(repoRoot, startLine)

	// Save initial session state
	state := &trace.SessionState{
		SessionID:      event.SessionID,
		TranscriptPath: event.SessionRef,
		TurnCount:      0,
	}
	_ = trace.SaveSessionState(repoRoot, state)

	// 5. Fire-and-forget upstream event
	remoteEvt := api.SessionStartEvent{
		BaseSessionEvent: api.BaseSessionEvent{
			Type:      api.SessionEventTypeStart,
			SessionID: event.SessionID,
			Branch:    branch,
			Timestamp: timestamp,
		},
		AgentType:  agentType,
		GitRemote:  gitRemote,
		BaseCommit: headSha,
		CLIVersion: "1.0.0",
	}
	_ = s.sessionsAPI.SendEvent(ctx, remoteEvt, repoRoot)

	return nil
}

// HandleTurnStart registers a new user prompt turn and captures snapshot diff baseline.
func (s *Service) HandleTurnStart(ctx context.Context, repoRoot string, agentType trace.AgentType, event *trace.LifecycleEvent) error {
	if event.SessionID == "" {
		event.SessionID = uuid.New().String()
	}

	branch, _ := s.gitSvc.GetCurrentBranch(ctx)
	headSha, _ := s.gitSvc.GetHeadSha(ctx)

	state, err := trace.LoadSessionState(repoRoot, event.SessionID)
	if err != nil || state == nil {
		state = &trace.SessionState{SessionID: event.SessionID}
	}
	if event.SessionRef != "" {
		state.TranscriptPath = event.SessionRef
	}

	state.TurnCount++
	turnID := fmt.Sprintf("turn_%d_%s", state.TurnCount, uuid.New().String()[:8])
	state.TurnID = turnID
	state.CommitBefore = headSha
	state.TurnCompleted = false

	timestamp := time.Now().UTC().Format(time.RFC3339)
	scrubbedPrompt := trace.Redact(event.Prompt)

	// Append turn-start to local session log
	line := trace.TraceRecordLine{
		Kind:         "turn-start",
		SessionID:    event.SessionID,
		TurnID:       turnID,
		Prompt:       scrubbedPrompt,
		CommitBefore: headSha,
		Timestamp:    timestamp,
	}
	_ = trace.AppendSessionLine(repoRoot, line)
	_ = trace.SaveSessionState(repoRoot, state)

	// Dispatch upstream turn-start event
	remoteEvt := api.TurnStartEvent{
		BaseSessionEvent: api.BaseSessionEvent{
			Type:      api.SessionEventTypeTurnStart,
			SessionID: event.SessionID,
			Branch:    branch,
			Timestamp: timestamp,
		},
		TurnID:       turnID,
		Prompt:       scrubbedPrompt,
		CommitBefore: headSha,
	}
	_ = s.sessionsAPI.SendEvent(ctx, remoteEvt, repoRoot)

	return nil
}

// HandleTurnEnd stabilizes the transcript, parses modified files and tools, and dispatches turn completion.
func (s *Service) HandleTurnEnd(ctx context.Context, repoRoot string, agentType trace.AgentType, event *trace.LifecycleEvent) error {
	if event.SessionID == "" {
		event.SessionID = uuid.New().String()
	}

	state, _ := trace.LoadSessionState(repoRoot, event.SessionID)

	// Dedup check: if this turn already completed (e.g. stop + post-tool duplicate hooks), skip
	if state != nil && state.TurnCompleted {
		utils.Debug("[lifecycle] Turn %s already marked completed; skipping duplicate turn_end", state.TurnID)
		return nil
	}

	turnID := ""
	transcriptPath := ""
	var transcriptOffset int64

	if state != nil {
		turnID = state.TurnID
		transcriptPath = state.TranscriptPath
		transcriptOffset = state.LastOffset
	}
	if turnID == "" {
		turnID = fmt.Sprintf("turn_%d", time.Now().UnixNano())
	}
	if transcriptPath == "" && event.SessionRef != "" {
		transcriptPath = event.SessionRef
	}

	var toolCalls []trace.TraceToolCallRecord
	var filesModified []trace.FileChange
	var filesRead []string
	var commands []string
	var tokenUsage *trace.TokenUsage
	var response string

	if transcriptPath != "" {
		// Wait for transcript file to stabilize
		_ = trace.WaitForTranscriptFlush(transcriptPath, 3*time.Second)

		parsed, err := trace.ParseTranscript(transcriptPath, transcriptOffset)
		if err == nil && parsed != nil {
			if state != nil {
				state.LastOffset = parsed.NextOffset
			}
			toolCalls = parsed.ToolCalls
			for _, f := range parsed.ModifiedFiles {
				rel := toRepoRelative(repoRoot, f)
				filesModified = append(filesModified, trace.FileChange{Path: rel})
			}
			for _, r := range parsed.FilesRead {
				filesRead = append(filesRead, toRepoRelative(repoRoot, r))
			}
			for _, c := range parsed.Commands {
				commands = append(commands, trace.Redact(c))
			}
			tokenUsage = &parsed.TokenUsage
			response = trace.Redact(parsed.Summary)
		}
	}

	branch, _ := s.gitSvc.GetCurrentBranch(ctx)
	headSha, _ := s.gitSvc.GetHeadSha(ctx)

	// If TurnStart was never triggered, synthesize TurnStart event upstream first
	if state == nil || state.TurnCount == 0 {
		syntheticTurnStart := api.TurnStartEvent{
			BaseSessionEvent: api.BaseSessionEvent{
				Type:      api.SessionEventTypeTurnStart,
				SessionID: event.SessionID,
				Branch:    branch,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			},
			TurnID:       turnID,
			Prompt:       "",
			CommitBefore: headSha,
		}
		_ = s.sessionsAPI.SendEvent(ctx, syntheticTurnStart, repoRoot)
	}

	// Mark turn as completed in state
	if state == nil {
		state = &trace.SessionState{SessionID: event.SessionID}
	}
	state.TurnID = turnID
	state.TranscriptPath = transcriptPath
	state.TurnCompleted = true
	_ = trace.SaveSessionState(repoRoot, state)

	timestamp := time.Now().UTC().Format(time.RFC3339)

	// Append turn-end to local session log
	commitBefore := ""
	if state != nil {
		commitBefore = state.CommitBefore
	}
	endLine := trace.TraceRecordLine{
		Kind:          "turn-end",
		SessionID:     event.SessionID,
		TurnID:        turnID,
		Response:      response,
		ToolCalls:     toolCalls,
		FilesModified: filesModified,
		FilesRead:     filesRead,
		Commands:      commands,
		TokenUsage:    tokenUsage,
		CommitBefore:  commitBefore,
		CommitAfter:   headSha,
		Timestamp:     timestamp,
	}
	_ = trace.AppendSessionLine(repoRoot, endLine)

	// Dispatch upstream turn-end event
	remoteEvt := api.TurnEndEvent{
		BaseSessionEvent: api.BaseSessionEvent{
			Type:      api.SessionEventTypeTurnEnd,
			SessionID: event.SessionID,
			Branch:    branch,
			Timestamp: timestamp,
		},
		TurnID:        turnID,
		Response:      response,
		ToolCalls:     toolCalls,
		FilesModified: filesModified,
		FilesRead:     filesRead,
		Commands:      commands,
		TokenUsage:    tokenUsage,
		CommitAfter:   headSha,
	}
	_ = s.sessionsAPI.SendEvent(ctx, remoteEvt, repoRoot)

	return nil
}

// HandleSubagentStart transmits task spawning metadata for subagents.
func (s *Service) HandleSubagentStart(ctx context.Context, repoRoot string, agentType trace.AgentType, event *trace.LifecycleEvent) error {
	if event.ToolUseID == "" {
		return nil
	}

	branch, _ := s.gitSvc.GetCurrentBranch(ctx)
	timestamp := time.Now().UTC().Format(time.RFC3339)

	remoteEvt := api.SubagentStartEvent{
		BaseSessionEvent: api.BaseSessionEvent{
			Type:      api.SessionEventTypeSubagentStart,
			SessionID: event.SessionID,
			Branch:    branch,
			Timestamp: timestamp,
		},
		ToolUseID:       event.ToolUseID,
		SubagentType:    event.SubagentType,
		TaskDescription: trace.Redact(event.TaskDescription),
	}
	_ = s.sessionsAPI.SendEvent(ctx, remoteEvt, repoRoot)

	return nil
}

// HandleSubagentEnd transmits completion telemetry for subagents.
func (s *Service) HandleSubagentEnd(ctx context.Context, repoRoot string, agentType trace.AgentType, event *trace.LifecycleEvent) error {
	if event.ToolUseID == "" {
		return nil
	}

	branch, _ := s.gitSvc.GetCurrentBranch(ctx)
	timestamp := time.Now().UTC().Format(time.RFC3339)

	remoteEvt := api.SubagentEndEvent{
		BaseSessionEvent: api.BaseSessionEvent{
			Type:      api.SessionEventTypeSubagentEnd,
			SessionID: event.SessionID,
			Branch:    branch,
			Timestamp: timestamp,
		},
		ToolUseID: event.ToolUseID,
	}
	_ = s.sessionsAPI.SendEvent(ctx, remoteEvt, repoRoot)

	return nil
}

// HandleSessionEnd closes an active session and removes ephemeral turn state.
func (s *Service) HandleSessionEnd(ctx context.Context, repoRoot string, agentType trace.AgentType, event *trace.LifecycleEvent) error {
	if event.SessionID == "" {
		return nil
	}

	branch, _ := s.gitSvc.GetCurrentBranch(ctx)
	timestamp := time.Now().UTC().Format(time.RFC3339)

	// Append session-end to local session log
	endLine := trace.TraceRecordLine{
		Kind:      "session-end",
		SessionID: event.SessionID,
		Timestamp: timestamp,
	}
	_ = trace.AppendSessionLine(repoRoot, endLine)

	// Dispatch upstream session-end event
	remoteEvt := api.SessionEndEvent{
		BaseSessionEvent: api.BaseSessionEvent{
			Type:      api.SessionEventTypeEnd,
			SessionID: event.SessionID,
			Branch:    branch,
			Timestamp: timestamp,
		},
	}
	_ = s.sessionsAPI.SendEvent(ctx, remoteEvt, repoRoot)

	// Clean up ephemeral turn state; the durable session log remains in records/
	_ = trace.RemoveTurnState(repoRoot, event.SessionID)

	return nil
}

// CleanupStaleSessions closes abandoned sessions that have been inactive > threshold.
func (s *Service) CleanupStaleSessions(ctx context.Context, repoRoot string, agentType trace.AgentType) error {
	staleIDs, err := trace.ListStaleSessions(repoRoot, 30*time.Minute)
	if err != nil || len(staleIDs) == 0 {
		return nil
	}

	branch, _ := s.gitSvc.GetCurrentBranch(ctx)
	timestamp := time.Now().UTC().Format(time.RFC3339)

	for _, id := range staleIDs {
		utils.Debug("[lifecycle] Closing stale session: %s", id)
		remoteEvt := api.SessionEndEvent{
			BaseSessionEvent: api.BaseSessionEvent{
				Type:      api.SessionEventTypeEnd,
				SessionID: id,
				Branch:    branch,
				Timestamp: timestamp,
			},
		}
		_ = s.sessionsAPI.SendEvent(ctx, remoteEvt, repoRoot)
		_ = trace.RemoveTurnState(repoRoot, id)
	}

	return nil
}

// LEGACY COMPATIBILITY: EVENT RECORDING

// RecordEvent writes an event to the local session log and forwards to decision capture.
func (s *Service) RecordEvent(ctx context.Context, evt Event) error {
	if evt.SessionID == "" {
		evt.SessionID = uuid.New().String()
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}

	sessionFile := filepath.Join(s.tracesDir, fmt.Sprintf("%s.jsonl", evt.SessionID))
	f, err := os.OpenFile(sessionFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err == nil {
		line, _ := json.Marshal(evt)
		_, _ = f.WriteString(string(line) + "\n")
		_ = f.Close()
	}

	captureReq := api.DecisionCapturePayload{
		Agent:     evt.Agent,
		Event:     evt.HookName,
		Summary:   evt.Summary,
		Payload:   evt.Payload,
		Timestamp: evt.Timestamp,
	}
	_ = s.apiClient.CaptureDecision(ctx, captureReq)

	return nil
}

// HOOK SCAFFOLDING & REMOVAL

// EnableHooks installs lifecycle hooks for requested agents (claude, cursor, codex).
func (s *Service) EnableHooks(workDir string, agents []string, codexConfigPath string) ([]string, error) {
	if workDir == "" {
		workDir = "."
	}

	var installed []string
	for _, a := range agents {
		agent := strings.TrimSpace(strings.ToLower(a))
		switch agent {
		case "cursor":
			if err := s.installCursorHook(workDir); err == nil {
				installed = append(installed, "cursor")
			}
		case "claude", "claude-code":
			if err := s.installClaudeCodeHook(workDir); err == nil {
				installed = append(installed, "claude-code")
			}
		case "codex":
			if err := s.installCodexHook(codexConfigPath); err == nil {
				installed = append(installed, "codex")
			}
		}
	}
	return installed, nil
}

// DisableHooks removes installed lifecycle hooks for all agents.
func (s *Service) DisableHooks(workDir string) error {
	if workDir == "" {
		workDir = "."
	}

	cursorFile := filepath.Join(workDir, ".cursor", "rules", "scandrix.mdc")
	_ = os.Remove(cursorFile)

	claudeFile := filepath.Join(workDir, ".claude", "settings.json")
	_ = os.Remove(claudeFile)

	return nil
}

func (s *Service) installCursorHook(workDir string) error {
	dir := filepath.Join(workDir, ".cursor", "rules")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	content := `---
description: ScanDrix Automated Review & Architecture Guard
globs: *
alwaysApply: true
---

Before completing code changes, run: scandrix review --prompt-only --staged
Report findings and remediations before committing.
`
	return os.WriteFile(filepath.Join(dir, "scandrix.mdc"), []byte(content), 0644)
}

func (s *Service) installClaudeCodeHook(workDir string) error {
	dir := filepath.Join(workDir, ".claude")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	settingsPath := filepath.Join(dir, "settings.json")
	settings := map[string]any{
		"hooks": map[string]any{
			"stop": "scandrix decisions capture --capture-agent claude-code --event stop",
		},
	}
	data, _ := json.MarshalIndent(settings, "", "  ")
	return os.WriteFile(settingsPath, data, 0644)
}

func (s *Service) installCodexHook(configPath string) error {
	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".codex", "config.toml")
	}
	_ = os.MkdirAll(filepath.Dir(configPath), 0755)

	notifyLine := `notify = ["scandrix", "decisions", "capture", "--capture-agent", "codex", "--event", "stop"]`
	existing, _ := os.ReadFile(configPath)
	if strings.Contains(string(existing), "scandrix decisions capture") {
		return nil
	}

	f, err := os.OpenFile(configPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString("\n" + notifyLine + "\n")
	return err
}

func toRepoRelative(repoRoot, filePath string) string {
	if !filepath.IsAbs(filePath) {
		return filePath
	}
	rel, err := filepath.Rel(repoRoot, filePath)
	if err != nil {
		return filePath
	}
	return rel
}

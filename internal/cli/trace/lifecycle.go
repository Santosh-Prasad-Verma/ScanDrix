// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/cli/services/git"
)

// LifecycleCoordinator orchestrates recording of assistant events into the durable store.
type LifecycleCoordinator struct {
	gitSvc *git.Service
}

func NewLifecycleCoordinator(gitSvc *git.Service) *LifecycleCoordinator {
	if gitSvc == nil {
		gitSvc = git.DefaultService()
	}
	return &LifecycleCoordinator{gitSvc: gitSvc}
}

// Dispatch processes a normalized LifecycleEvent and appends it to the session record.
func (lc *LifecycleCoordinator) Dispatch(ctx context.Context, gitRoot string, agentType AgentType, evt *LifecycleEvent) error {
	if evt == nil {
		return nil
	}

	sessionID := evt.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	state, err := LoadSessionState(gitRoot, sessionID)
	if err != nil || state == nil {
		state = &SessionState{SessionID: sessionID}
	}

	if evt.SessionRef != "" {
		state.TranscriptPath = evt.SessionRef
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)

	switch evt.Type {
	case "SessionStart":
		branch, _ := lc.gitSvc.GetCurrentBranch(ctx)
		headSha, _ := lc.gitSvc.GetHeadSha(ctx)
		remoteURL, _ := lc.gitSvc.GetRemoteURL(ctx, "origin")

		line := TraceRecordLine{
			Kind:       "session-start",
			SessionID:  sessionID,
			AgentType:  agentType,
			Branch:     branch,
			BaseCommit: headSha,
			GitRemote:  remoteURL,
			CLIVersion: "1.0.0",
			Timestamp:  nowStr,
		}
		_ = AppendSessionLine(gitRoot, line)
		_ = SaveSessionState(gitRoot, state)

	case "TurnStart":
		headSha, _ := lc.gitSvc.GetHeadSha(ctx)
		state.CommitBefore = headSha
		state.TurnCount++

		turnID := fmt.Sprintf("turn_%d_%s", state.TurnCount, uuid.New().String()[:8])
		prompt := Redact(evt.Prompt)

		line := TraceRecordLine{
			Kind:         "turn-start",
			SessionID:    sessionID,
			TurnID:       turnID,
			Prompt:       prompt,
			CommitBefore: headSha,
			Timestamp:    nowStr,
		}
		_ = AppendSessionLine(gitRoot, line)
		_ = SaveSessionState(gitRoot, state)

	case "TurnEnd":
		headSha, _ := lc.gitSvc.GetHeadSha(ctx)

		var toolCalls []TraceToolCallRecord
		var filesModified []FileChange
		var filesRead []string
		var commands []string
		var tokenUsage *TokenUsage
		var response string

		if state.TranscriptPath != "" {
			parsed, pErr := ParseTranscript(state.TranscriptPath, state.LastOffset)
			if pErr == nil && parsed != nil {
				state.LastOffset = parsed.NextOffset
				toolCalls = parsed.ToolCalls
				for _, f := range parsed.ModifiedFiles {
					filesModified = append(filesModified, FileChange{Path: f})
				}
				filesRead = parsed.FilesRead
				commands = parsed.Commands
				tokenUsage = &parsed.TokenUsage
				response = Redact(parsed.Summary)
			}
		}

		turnID := fmt.Sprintf("turn_%d", state.TurnCount)
		line := TraceRecordLine{
			Kind:          "turn-end",
			SessionID:     sessionID,
			TurnID:        turnID,
			Response:      response,
			ToolCalls:     toolCalls,
			FilesModified: filesModified,
			FilesRead:     filesRead,
			Commands:      commands,
			TokenUsage:    tokenUsage,
			CommitBefore:  state.CommitBefore,
			CommitAfter:   headSha,
			Timestamp:     nowStr,
		}
		_ = AppendSessionLine(gitRoot, line)
		_ = SaveSessionState(gitRoot, state)

	case "SessionEnd":
		line := TraceRecordLine{
			Kind:      "session-end",
			SessionID: sessionID,
			Timestamp: nowStr,
		}
		_ = AppendSessionLine(gitRoot, line)
	}

	return nil
}

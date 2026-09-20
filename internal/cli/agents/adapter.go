package agents

import (
	"time"

	"github.com/scandrix/backend/internal/cli/trace"
)

// AgentAdapter normalizes IDE AI agent hooks and event payloads into canonical ScanDrix LifecycleEvents.
type AgentAdapter interface {
	AgentType() trace.AgentType
	ParseHookEvent(hookName string, payload []byte) (*trace.LifecycleEvent, error)
	ReadTranscript(transcriptPath string, fromOffset int64) (*trace.TranscriptParseResult, error)
	ExtractPrompts(transcriptPath string) ([]string, error)
	ExtractSummary(transcriptPath string) (string, error)
	ExtractModifiedFiles(transcriptPath string) ([]string, error)
	CalculateTokenUsage(transcriptPath string) (*trace.TokenUsage, error)
	WaitForTranscriptFlush(transcriptPath string, timeout time.Duration) bool
}

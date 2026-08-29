package operations

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// CodeManager coordinates commit statuses, discussions, and repository operations.
type CodeManager struct {
	mu       sync.RWMutex
	statuses map[string]CommitStatusCheck // SHA -> Status
	threads  map[string][]GitChatResponse // ThreadID -> messages
}

func NewCodeManager() *CodeManager {
	return &CodeManager{
		statuses: make(map[string]CommitStatusCheck),
		threads:  make(map[string][]GitChatResponse),
	}
}

// SetCommitStatus updates the CI/CD commit check status on GitHub/GitLab.
func (m *CodeManager) SetCommitStatus(ctx context.Context, check CommitStatusCheck) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if strings.TrimSpace(check.SHA) == "" {
		return fmt.Errorf("commit SHA cannot be empty")
	}

	check.CreatedAt = time.Now().UTC()
	m.statuses[check.SHA] = check
	return nil
}

// GetCommitStatus retrieves the current recorded status check for a commit SHA.
func (m *CodeManager) GetCommitStatus(sha string) (CommitStatusCheck, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status, exists := m.statuses[sha]
	return status, exists
}

// RecordThreadReply archives bot replies within a PR discussion thread.
func (m *CodeManager) RecordThreadReply(threadID string, reply GitChatResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.threads[threadID] = append(m.threads[threadID], reply)
}

// GetThreadReplies returns all responses recorded for an interactive discussion.
func (m *CodeManager) GetThreadReplies(threadID string) []GitChatResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.threads[threadID]
}

// BuildSummaryComment formats an enterprise PR summary with markdown watermarking and metrics.
func BuildSummaryComment(workspaceID uuid.UUID, prNumber int, criticalCount, highCount, totalFiles int) string {
	statusBanner := "### ✅ Scandrix Automated Code Assurance Passed"
	if criticalCount > 0 {
		statusBanner = "### 🚨 Scandrix Security Alert: Critical Vulnerabilities Detected"
	} else if highCount > 0 {
		statusBanner = "### ⚠️ Scandrix Code Review: Action Required"
	}

	return fmt.Sprintf(`%s

| Metric | Status |
|---|---|
| **Files Analyzed** | %d |
| **Critical Issues** | %d |
| **High Severity** | %d |
| **Assurance Engine** | AST + Multi-Agent Consensus |

> Reply with `+"`@scandrix review`"+` to trigger a re-scan, or ask questions directly in comment threads.

%s`, statusBanner, totalFiles, criticalCount, highCount, WatermarkCommentMarker)
}

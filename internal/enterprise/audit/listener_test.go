package audit

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockHandler struct {
	category   AuditEventCategory
	callCount  int64
	handledEvs []EnterpriseLogEvent
}

func (m *mockHandler) Category() AuditEventCategory {
	return m.category
}

func (m *mockHandler) HandleEvent(ctx context.Context, event EnterpriseLogEvent) error {
	atomic.AddInt64(&m.callCount, 1)
	m.handledEvs = append(m.handledEvs, event)
	return nil
}

func TestAuditLogListener_EmitAndProcess(t *testing.T) {
	repo := NewMemoryAuditRepository("listener-test-key")
	siem := NewSIEMAuditStreamer()

	cfg := DefaultAuditListenerConfig()
	cfg.WorkerCount = 2
	listener := NewAuditLogListener(repo, siem, cfg)
	defer listener.Close()

	handler := &mockHandler{category: CategoryCodeReviewConfig}
	listener.RegisterHandler(handler)

	orgID := uuid.New()
	wsID := uuid.New()

	ev := EnterpriseLogEvent{
		Category: CategoryCodeReviewConfig,
		Action:   "UPDATE",
		Actor: ActorContext{
			UserID: "usr-admin",
			Email:  "admin@scandrix.dev",
		},
		Target: TargetContext{
			OrganizationID: orgID,
			WorkspaceID:    wsID,
			TargetEntityID: "cfg-101",
		},
		Changes: []FieldChange{
			{Field: "auto_approve", OldValue: false, NewValue: true},
		},
	}

	err := listener.Emit(ev)
	require.NoError(t, err)

	// Allow workers to process
	time.Sleep(150 * time.Millisecond)

	// Handler was called
	assert.Equal(t, int64(1), atomic.LoadInt64(&handler.callCount))

	// Event was committed to repo with hash
	logs, total, err := repo.QueryLogs(context.Background(), AuditLogFilter{OrganizationID: &orgID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "cfg-101", logs[0].Target.TargetEntityID)
	assert.NotEmpty(t, logs[0].Hash)

	// Verify chain integrity
	valid, _, err := repo.VerifyChainIntegrity(context.Background(), orgID)
	require.NoError(t, err)
	assert.True(t, valid)
}

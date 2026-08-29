package audit_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/audit"
)

func TestAuditLoggerAndCEF(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := audit.NewAuditLogger(buf)

	wsID := uuid.New()
	event := audit.AuditEvent{
		WorkspaceID: wsID,
		ActorID:     "usr_12345",
		ActorEmail:  "ciso@enterprise.com",
		IPAddress:   "192.168.1.100",
		Action:      audit.ActionRuleCreated,
		TargetType:  "Rule",
		TargetID:    "rule_sec_01",
		Metadata:    map[string]any{"severity": "CRITICAL"},
	}

	if err := logger.Record(context.Background(), event); err != nil {
		t.Fatalf("failed recording audit event: %v", err)
	}

	// Verify JSON output was written to sink
	output := buf.String()
	if !strings.Contains(output, "rule.created") || !strings.Contains(output, "ciso@enterprise.com") {
		t.Fatalf("unexpected sink output: %s", output)
	}

	// Verify CEF formatting
	cef := event.ToCEF()
	if !strings.HasPrefix(cef, "CEF:0|Scandrix|CoreEngine") {
		t.Errorf("invalid CEF header: %s", cef)
	}
	if !strings.Contains(cef, "suser=ciso@enterprise.com") {
		t.Errorf("CEF missing actor email: %s", cef)
	}

	// Verify buffer flush
	flushed := logger.Flush()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 flushed event, got %d", len(flushed))
	}
	if len(logger.Flush()) != 0 {
		t.Error("expected buffer to be empty after flush")
	}
}

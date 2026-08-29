package finetuning_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/finetuning"
)

func TestFineTuningDatasetBuilder(t *testing.T) {
	builder := finetuning.NewFineTuningDatasetBuilder()
	wsID := uuid.New()

	// 1. Accepted sample
	builder.RecordSample(finetuning.ReviewSuggestionTrainingSample{
		ID:                 uuid.New(),
		WorkspaceID:        wsID,
		SystemPrompt:       "You are an expert security code reviewer.",
		InputCodeDiff:      "- query := fmt.Sprintf(\"SELECT * FROM users WHERE id = '%s'\", id)\n+ query := \"SELECT * FROM users WHERE id = $1\"",
		ExpectedSuggestion: "Use parameterized query placeholders to prevent SQL injection (CWE-89).",
		Status:             finetuning.FeedbackAccepted,
		CapturedAt:         time.Now().UTC(),
	})

	// 2. Rejected sample (developer flagged as false positive)
	builder.RecordSample(finetuning.ReviewSuggestionTrainingSample{
		ID:                 uuid.New(),
		WorkspaceID:        wsID,
		SystemPrompt:       "You are an expert security code reviewer.",
		InputCodeDiff:      "+ log.Printf(\"Processed payment for order %s\", orderID)",
		ExpectedSuggestion: "Sensitive data exposure in logs",
		Status:             finetuning.FeedbackRejected,
		CapturedAt:         time.Now().UTC(),
	})

	var buf bytes.Buffer
	count, err := builder.ExportJSONL(&buf)
	if err != nil {
		t.Fatalf("failed exporting fine-tuning JSONL: %v", err)
	}

	// Should export exactly 1 sample (accepted only)
	if count != 1 {
		t.Fatalf("expected exactly 1 exported sample, got %d", count)
	}

	line := strings.TrimSpace(buf.String())
	var entry finetuning.ChatFineTuningEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("exported line is not valid JSON: %v", err)
	}

	if len(entry.Messages) != 3 {
		t.Fatalf("expected 3 chat messages (system, user, assistant), got %d", len(entry.Messages))
	}
	if entry.Messages[0].Role != "system" || entry.Messages[1].Role != "user" || entry.Messages[2].Role != "assistant" {
		t.Fatalf("unexpected message roles: %+v", entry.Messages)
	}
}

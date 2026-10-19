package finetuning_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/finetuning"
)

type mockFinetuningRepo struct {
	samples []finetuning.ReviewSuggestionTrainingSample
}

func (m *mockFinetuningRepo) GetFineTuningSamples(ctx context.Context, workspaceID uuid.UUID, limit int) ([]finetuning.ReviewSuggestionTrainingSample, error) {
	return m.samples, nil
}

func TestExportDataset_ChatFormat(t *testing.T) {
	wsID := uuid.New()
	repo := &mockFinetuningRepo{
		samples: []finetuning.ReviewSuggestionTrainingSample{
			{
				ID:                 uuid.New(),
				WorkspaceID:        wsID,
				SystemPrompt:       "You are ScanDrix",
				InputCodeDiff:      "+ auth_token = req.headers['x-token']",
				ExpectedSuggestion: "Validate auth_token is not null before parsing",
				Status:             finetuning.FeedbackAccepted,
				CapturedAt:         time.Now(),
			},
			{
				ID:                 uuid.New(),
				WorkspaceID:        wsID,
				SystemPrompt:       "You are ScanDrix",
				InputCodeDiff:      "+ debug_print()",
				ExpectedSuggestion: "Remove debug print",
				Status:             finetuning.FeedbackRejected,
				CapturedAt:         time.Now(),
			},
		},
	}

	buf := new(bytes.Buffer)
	count, err := finetuning.ExportDataset(context.Background(), repo, finetuning.ExportOptions{
		WorkspaceID: wsID,
		Format:      finetuning.FormatChat,
	}, buf)

	if err != nil {
		t.Fatalf("unexpected error exporting chat dataset: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 accepted sample exported, got %d", count)
	}

	line := buf.String()
	if !strings.Contains(line, "Validate auth_token") || strings.Contains(line, "Remove debug print") {
		t.Errorf("expected only accepted suggestion in chat format: %s", line)
	}
}

func TestExportDataset_DPOFormat(t *testing.T) {
	wsID := uuid.New()
	repo := &mockFinetuningRepo{
		samples: []finetuning.ReviewSuggestionTrainingSample{
			{
				ID:                 uuid.New(),
				WorkspaceID:        wsID,
				InputCodeDiff:      "+ fmt.Println(secret)",
				ExpectedSuggestion: "Chosen: Use structured logger and redact secrets",
				Status:             finetuning.FeedbackAccepted,
			},
			{
				ID:                 uuid.New(),
				WorkspaceID:        wsID,
				InputCodeDiff:      "+ fmt.Println(secret)",
				ExpectedSuggestion: "Rejected: Ignore secret logging",
				Status:             finetuning.FeedbackRejected,
			},
		},
	}

	buf := new(bytes.Buffer)
	count, err := finetuning.ExportDataset(context.Background(), repo, finetuning.ExportOptions{
		WorkspaceID: wsID,
		Format:      finetuning.FormatDPO,
	}, buf)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 DPO pair, got %d", count)
	}

	var dpo finetuning.DPOFineTuningEntry
	if err := json.Unmarshal(buf.Bytes(), &dpo); err != nil {
		t.Fatalf("failed decoding DPO jsonl line: %v", err)
	}

	if !strings.Contains(dpo.Chosen, "Chosen") || !strings.Contains(dpo.Rejected, "Rejected") {
		t.Errorf("unexpected DPO chosen/rejected values: %+v", dpo)
	}
}

package clireview_test

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/clireview"
	"github.com/stretchr/testify/assert"
)

type mockCaptureRepo struct {
	capture   *clireview.CliSessionCapture
	completed []clireview.CliSessionClassifiedDecision
	source    string
	status    string
}

func (m *mockCaptureRepo) Create(ctx context.Context, capture *clireview.CliSessionCapture) (*clireview.CliSessionCapture, error) {
	m.capture = capture
	return capture, nil
}

func (m *mockCaptureRepo) FindByDedupKey(ctx context.Context, dedupKey string) (*clireview.CliSessionCapture, error) {
	return m.capture, nil
}

func (m *mockCaptureRepo) FindByCaptureID(ctx context.Context, captureID string) (*clireview.CliSessionCapture, error) {
	return m.capture, nil
}

func (m *mockCaptureRepo) MarkProcessing(ctx context.Context, captureID string) error {
	m.status = "processing"
	return nil
}

func (m *mockCaptureRepo) MarkCompleted(ctx context.Context, captureID string, decisions []clireview.CliSessionClassifiedDecision, source string) error {
	m.completed = decisions
	m.source = source
	m.status = "completed"
	return nil
}

func (m *mockCaptureRepo) MarkFailed(ctx context.Context, captureID string, errorMessage string) error {
	m.status = "failed"
	return nil
}

func (m *mockCaptureRepo) MarkSkipped(ctx context.Context, captureID string, reason string) error {
	m.status = "skipped"
	return nil
}

type mockCaptureLLMClient struct {
	decisions []clireview.CliSessionClassifiedDecision
	err       error
	callCount int
}

func (m *mockCaptureLLMClient) ExtractDecisions(ctx context.Context, systemPrompt, userPayload string) ([]clireview.CliSessionClassifiedDecision, error) {
	m.callCount++
	if m.err != nil {
		return nil, m.err
	}
	return m.decisions, nil
}

func TestClassifyCliSessionCaptureUseCase_ExtractWithLLM(t *testing.T) {
	capture := &clireview.CliSessionCapture{
		OrganizationID: "org-123",
		Summary:        "Designed the audit log",
		Signals: &clireview.CliSessionSignals{
			Prompt:           "Design the audit log",
			AssistantMessage: "I chose event sourcing.",
			ModifiedFiles:    []string{"src/audit/store.ts", "src/audit/replay.ts"},
			ToolUses: []clireview.CliSessionToolUse{
				{Tool: "Edit", FilePath: "src/audit/store.ts"},
			},
		},
	}

	modelDecisions := []clireview.CliSessionClassifiedDecision{
		{
			Type:       clireview.DecisionArchitecturalDetail,
			Origin:     clireview.OriginHuman,
			Decision:   "Use event sourcing for the audit log",
			Rationale:  "Full auditability of every state change",
			Confidence: 0.9,
			Evidence:   []string{"src/audit/store.ts", "src/audit/replay.ts"},
		},
		{
			Type:       clireview.DecisionTooling,
			Decision:   "Adopt pnpm as the package manager",
			Confidence: 0.4,
		},
	}

	t.Run("maps the model decisions[] byte-for-byte to CliSessionClassifiedDecision[]", func(t *testing.T) {
		repo := &mockCaptureRepo{}
		llm := &mockCaptureLLMClient{decisions: modelDecisions}
		uc := clireview.NewClassifyCliSessionCaptureUseCase(repo, llm)

		decisions, err := uc.ExtractWithLLM(context.Background(), capture)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := []clireview.CliSessionClassifiedDecision{
			{
				Type:                 clireview.DecisionArchitecturalDetail,
				Origin:               clireview.OriginHuman,
				Decision:             "Use event sourcing for the audit log",
				Rationale:            "Full auditability of every state change",
				Confidence:           0.9,
				Evidence:             []string{"src/audit/store.ts", "src/audit/replay.ts"},
				AutoPromoteCandidate: true,
			},
			{
				Type:                 clireview.DecisionTooling,
				Origin:               "",
				Decision:             "Adopt pnpm as the package manager",
				Rationale:            "",
				Confidence:           0.4,
				Evidence:             []string{},
				AutoPromoteCandidate: false,
			},
		}

		assert.Equal(t, expected, decisions)
	})

	t.Run("routes through exactly one LLM call path", func(t *testing.T) {
		repo := &mockCaptureRepo{}
		llm := &mockCaptureLLMClient{decisions: modelDecisions}
		uc := clireview.NewClassifyCliSessionCaptureUseCase(repo, llm)

		_, err := uc.ExtractWithLLM(context.Background(), capture)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if llm.callCount != 1 {
			t.Errorf("expected exactly 1 LLM call, got %d", llm.callCount)
		}
	})

	t.Run("empty decisions -> empty mapping (no throw)", func(t *testing.T) {
		repo := &mockCaptureRepo{}
		llm := &mockCaptureLLMClient{decisions: []clireview.CliSessionClassifiedDecision{}}
		uc := clireview.NewClassifyCliSessionCaptureUseCase(repo, llm)

		decisions, err := uc.ExtractWithLLM(context.Background(), capture)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(decisions) != 0 {
			t.Errorf("expected empty decisions, got %+v", decisions)
		}
	})
}

package usecases

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/pkg/models"
)

type mockHandler struct {
	called bool
}

func (m *mockHandler) CanHandle(params contracts.WebhookEventParams) bool {
	return true
}

func (m *mockHandler) Handle(ctx context.Context, params contracts.WebhookEventParams) error {
	m.called = true
	return nil
}

type mockJobQueue struct {
	jobs []WebhookJob
}

func (m *mockJobQueue) Enqueue(ctx context.Context, job WebhookJob) error {
	m.jobs = append(m.jobs, job)
	return nil
}

func TestReceiveWebhookUseCase(t *testing.T) {
	ghHandler := &mockHandler{}
	handlers := map[models.SCMProvider]contracts.IWebhookEventHandler{
		models.SCMProviderGitHub: ghHandler,
	}

	uc := NewReceiveWebhookUseCase(handlers, nil)

	err := uc.Execute(context.Background(), contracts.WebhookEventParams{
		PlatformType: models.SCMProviderGitHub,
		Event:        "pull_request",
		RawPayload:   json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ghHandler.called {
		t.Errorf("expected GitHub handler to be called")
	}

	// Unknown platform should not error, but skip gracefully
	err = uc.Execute(context.Background(), contracts.WebhookEventParams{
		PlatformType: models.SCMProviderForgejo,
		Event:        "pull_request",
		RawPayload:   json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("unexpected error on unhandled platform: %v", err)
	}
}

func TestEnqueueWebhookUseCase(t *testing.T) {
	queue := &mockJobQueue{}
	uc := NewEnqueueWebhookUseCase(queue, nil)

	err := uc.Execute(context.Background(), EnqueueWebhookInput{
		PlatformType: "github",
		Event:        "pull_request",
		Payload:      json.RawMessage(`{"test":true}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(queue.jobs) != 1 {
		t.Fatalf("expected 1 job enqueued, got %d", len(queue.jobs))
	}
	job := queue.jobs[0]
	if job.PlatformType != models.SCMProviderGitHub || job.WorkflowType != "WEBHOOK_PROCESSING" {
		t.Errorf("unexpected job details: %+v", job)
	}
	if job.CorrelationID == "" {
		t.Errorf("expected generated correlation ID")
	}

	// Test aliases
	aliasTests := []struct {
		input    string
		expected models.SCMProvider
	}{
		{"azure-repos", models.SCMProviderAzureDevOps},
		{"azuredevops", models.SCMProviderAzureDevOps},
		{"gitlab", models.SCMProviderGitLab},
		{"bitbucket", models.SCMProviderBitbucket},
		{"forgejo", models.SCMProviderForgejo},
		{"gitea", models.SCMProviderForgejo},
	}

	for _, tc := range aliasTests {
		p, err := NormalizePlatformType(tc.input)
		if err != nil || p != tc.expected {
			t.Errorf("NormalizePlatformType(%q) = %v, %v; expected %v", tc.input, p, err, tc.expected)
		}
	}
}

package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// EnqueueWebhookInput represents parameters for queueing an asynchronous raw webhook.
type EnqueueWebhookInput struct {
	PlatformType  string            `json:"platformType"`
	Event         string            `json:"event"`
	Action        string            `json:"action,omitempty"`
	Payload       json.RawMessage   `json:"payload"`
	Headers       map[string]string `json:"headers,omitempty"`
	CorrelationID string            `json:"correlationId,omitempty"`
}

// WebhookJob represents a queued webhook processing task.
type WebhookJob struct {
	CorrelationID string             `json:"correlationId"`
	WorkflowType  string             `json:"workflowType"` // "WEBHOOK_PROCESSING"
	HandlerType   string             `json:"handlerType"`  // "WEBHOOK_RAW"
	PlatformType  models.SCMProvider `json:"platformType"`
	Event         string             `json:"event"`
	Action        string             `json:"action,omitempty"`
	Payload       json.RawMessage    `json:"payload"`
	Headers       map[string]string  `json:"headers,omitempty"`
	Status        string             `json:"status"` // "PENDING"
	Priority      int                `json:"priority"`
	RetryCount    int                `json:"retryCount"`
	MaxRetries    int                `json:"maxRetries"`
}

// IWebhookJobQueue contract for publishing raw webhooks to asynchronous workers.
type IWebhookJobQueue interface {
	Enqueue(ctx context.Context, job WebhookJob) error
}

// EnqueueWebhookUseCase validates and asynchronously enqueues raw incoming webhooks.
type EnqueueWebhookUseCase struct {
	queue  IWebhookJobQueue
	logger *slog.Logger
}

// NewEnqueueWebhookUseCase creates a new EnqueueWebhookUseCase.
func NewEnqueueWebhookUseCase(queue IWebhookJobQueue, logger *slog.Logger) *EnqueueWebhookUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &EnqueueWebhookUseCase{
		queue:  queue,
		logger: logger,
	}
}

// NormalizePlatformType standardizes provider aliases across various webhook sources.
func NormalizePlatformType(input string) (models.SCMProvider, error) {
	norm := strings.ToUpper(strings.TrimSpace(input))
	norm = strings.ReplaceAll(norm, "-", "_")
	norm = strings.ReplaceAll(norm, " ", "_")

	switch norm {
	case "GITHUB":
		return models.SCMProviderGitHub, nil
	case "GITLAB":
		return models.SCMProviderGitLab, nil
	case "BITBUCKET":
		return models.SCMProviderBitbucket, nil
	case "AZURE_REPOS", "AZUREDEVOPS", "AZURE_DEVOPS", "AZURE_REPOSITORIES":
		return models.SCMProviderAzureDevOps, nil
	case "FORGEJO", "GITEA":
		return models.SCMProviderForgejo, nil
	default:
		return "", fmt.Errorf("unsupported platform type: %s", input)
	}
}

// Execute validates platform and enqueues the raw webhook job.
func (uc *EnqueueWebhookUseCase) Execute(ctx context.Context, input EnqueueWebhookInput) error {
	provider, err := NormalizePlatformType(input.PlatformType)
	if err != nil {
		uc.logger.ErrorContext(ctx, "Unsupported platform type in webhook enqueue",
			slog.String("platform", input.PlatformType),
			slog.Any("error", err),
		)
		return err
	}

	correlationID := input.CorrelationID
	if correlationID == "" {
		correlationID = uuid.New().String()
	}

	job := WebhookJob{
		CorrelationID: correlationID,
		WorkflowType:  "WEBHOOK_PROCESSING",
		HandlerType:   "WEBHOOK_RAW",
		PlatformType:  provider,
		Event:         input.Event,
		Action:        input.Action,
		Payload:       input.Payload,
		Headers:       input.Headers,
		Status:        "PENDING",
		Priority:      0,
		RetryCount:    0,
		MaxRetries:    3,
	}

	if err := uc.queue.Enqueue(ctx, job); err != nil {
		uc.logger.ErrorContext(ctx, "Failed to enqueue webhook job",
			slog.String("correlationId", correlationID),
			slog.String("platform", string(provider)),
			slog.String("event", input.Event),
			slog.Any("error", err),
		)
		return fmt.Errorf("failed to enqueue webhook job: %w", err)
	}

	uc.logger.InfoContext(ctx, "Successfully enqueued raw webhook job",
		slog.String("correlationId", correlationID),
		slog.String("platform", string(provider)),
		slog.String("event", input.Event),
	)

	return nil
}

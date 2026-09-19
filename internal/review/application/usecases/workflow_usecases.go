package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// WorkflowJobPublisher abstracts enqueuing background asynchronous jobs.
type WorkflowJobPublisher interface {
	PublishJob(ctx context.Context, eventType string, payload []byte) error
}

// AstGraphUpdatePayload mirrors input payload for AST graph incremental ingestion.
type AstGraphUpdatePayload struct {
	WorkspaceID  uuid.UUID `json:"workspaceId"`
	RepositoryID uuid.UUID `json:"repositoryId"`
	CommitSHA    string    `json:"commitSha"`
	BaseSHA      string    `json:"baseSha,omitempty"`
	Branch       string    `json:"branch"`
	EnqueuedAt   time.Time `json:"enqueuedAt"`
}

// ImplementationCheckPayload defines verification job input.
type ImplementationCheckPayload struct {
	WorkspaceID    uuid.UUID `json:"workspaceId"`
	RepositoryID   uuid.UUID `json:"repositoryId"`
	PullNumber     int       `json:"pullNumber"`
	HeadSHA        string    `json:"headSha"`
	CorrelationID  string    `json:"correlationId"`
	EnqueuedAt     time.Time `json:"enqueuedAt"`
}

// WorkflowUseCases coordinates background job scheduling for reviews.
type WorkflowUseCases struct {
	publisher WorkflowJobPublisher
}

// NewWorkflowUseCases creates a new workflow use cases coordinator.
func NewWorkflowUseCases(publisher WorkflowJobPublisher) *WorkflowUseCases {
	return &WorkflowUseCases{publisher: publisher}
}

// EnqueueAstGraphUpdateOnMerged triggers an incremental AST graph re-index when a PR merges.
func (u *WorkflowUseCases) EnqueueAstGraphUpdateOnMerged(ctx context.Context, workspaceID, repoID uuid.UUID, branch, headSHA, baseSHA string) error {
	payload := AstGraphUpdatePayload{
		WorkspaceID:  workspaceID,
		RepositoryID: repoID,
		CommitSHA:    headSHA,
		BaseSHA:      baseSHA,
		Branch:       branch,
		EnqueuedAt:   time.Now().UTC(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal ast update payload: %w", err)
	}

	return u.publisher.PublishJob(ctx, "workflow.ast_graph.update", data)
}

// EnqueueImplementationCheck schedules verification of delivered suggestions after PR updates.
func (u *WorkflowUseCases) EnqueueImplementationCheck(ctx context.Context, workspaceID, repoID uuid.UUID, prNumber int, headSHA string) (string, error) {
	correlationID := fmt.Sprintf("impl-check-%s-%d-%s", repoID, prNumber, headSHA[:min(7, len(headSHA))])
	payload := ImplementationCheckPayload{
		WorkspaceID:   workspaceID,
		RepositoryID:  repoID,
		PullNumber:    prNumber,
		HeadSHA:       headSHA,
		CorrelationID: correlationID,
		EnqueuedAt:    time.Now().UTC(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal implementation check payload: %w", err)
	}

	err = u.publisher.PublishJob(ctx, "workflow.suggestion.check_implementation", data)
	if err != nil {
		return "", err
	}

	return correlationID, nil
}

// Default Outbox Job Publisher implementation for PostgreSQL transactional outbox.
type OutboxJobPublisher struct {
	saveOutbox func(ctx context.Context, rec models.OutboxRecord) error
}

// NewOutboxJobPublisher constructs an outbox publisher.
func NewOutboxJobPublisher(saveFunc func(ctx context.Context, rec models.OutboxRecord) error) *OutboxJobPublisher {
	return &OutboxJobPublisher{saveOutbox: saveFunc}
}

// PublishJob persists the job record to the transactional outbox table.
func (p *OutboxJobPublisher) PublishJob(ctx context.Context, eventType string, payload []byte) error {
	rec := models.OutboxRecord{
		ID:         uuid.New(),
		EventType:  eventType,
		Payload:    payload,
		Status:     models.OutboxPending,
		RetryCount: 0,
		CreatedAt:  time.Now().UTC(),
	}
	return p.saveOutbox(ctx, rec)
}

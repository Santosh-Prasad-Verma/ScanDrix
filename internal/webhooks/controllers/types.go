package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/scandrix/backend/internal/webhooks/ingestion"
	"github.com/scandrix/backend/pkg/models"
)


// WebhookEnqueueService encapsulates normalized event creation, deduplication,
// and transactional outbox persistence across all provider controllers.
type WebhookEnqueueService struct {
	repo     *database.Repository
	outbox   *relay.OutboxStore
	verifier *ingestion.WebhookVerifier
	parser   *ingestion.WebhookParser
	resolver ingestion.SecretResolver
}

// NewWebhookEnqueueService creates a new shared enqueue service for webhook controllers.
func NewWebhookEnqueueService(
	repo *database.Repository,
	outbox *relay.OutboxStore,
	resolver ingestion.SecretResolver,
) *WebhookEnqueueService {
	return &WebhookEnqueueService{
		repo:     repo,
		outbox:   outbox,
		verifier: ingestion.NewWebhookVerifier(),
		parser:   ingestion.NewWebhookParser(),
		resolver: resolver,
	}
}

// Verifier returns the cryptographic verifier.
func (s *WebhookEnqueueService) Verifier() *ingestion.WebhookVerifier {
	return s.verifier
}

// Resolver returns the secret resolver.
func (s *WebhookEnqueueService) Resolver() ingestion.SecretResolver {
	return s.resolver
}

// ReadPayload safely reads the HTTP request body with a 10MB limit.
func ReadPayload(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// EnqueueOptions carries optional metadata for enqueuing an event.
type EnqueueOptions struct {
	DeliveryID string
	SkipActionFilter bool
}

// EnqueueEvent processes, normalizes, and writes an inbound webhook event to the transactional outbox.
func (s *WebhookEnqueueService) EnqueueEvent(
	ctx context.Context,
	provider models.SCMProvider,
	eventType string,
	body []byte,
	opts ...EnqueueOptions,
) (*ingestion.NormalizedWebhookEvent, error) {
	event, err := s.parser.Parse(provider, eventType, body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s webhook: %w", provider, err)
	}

	var opt EnqueueOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	// Delivery / Dedup ID
	if opt.DeliveryID != "" {
		if parsedUUID, err := uuid.Parse(opt.DeliveryID); err == nil {
			event.ID = parsedUUID
		} else {
			event.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(string(provider)+":"+opt.DeliveryID))
		}
	} else if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}

	event.TaskID = event.ID
	event.EventID = event.ID

	// Resolve workspace ID if repo namespace is known
	if s.repo != nil && event.WorkspaceID == uuid.Nil && event.RepoNamespace != "" {
		if wsID, err := s.repo.GetWorkspaceIDByRepoNamespace(ctx, string(provider), event.RepoNamespace); err == nil && wsID != uuid.Nil {
			event.WorkspaceID = wsID
		}
	}

	// Write to Transactional Outbox
	eventBytes, _ := json.Marshal(event)
	if s.repo != nil {
		eventTypePrefix := "scm.pull_request."
		if strings.HasPrefix(string(event.Action), "INSTALLATION_") || strings.HasPrefix(string(event.Action), "REPOS_") {
			eventTypePrefix = "scm.lifecycle."
		}
		outboxEvent := &models.OutboxRecord{
			ID:          event.ID,
			WorkspaceID: event.WorkspaceID,
			EventType:   eventTypePrefix + strings.ToLower(string(event.Action)),
			Payload:     eventBytes,
			Status:      models.OutboxPending,
			CreatedAt:   time.Now().UTC(),
		}

		var initialReview *models.PullRequestReview
		if event.PullRequestNumber > 0 && (event.Action == ingestion.ActionOpened || event.Action == ingestion.ActionSynchronize || event.Action == ingestion.ActionReopened) {
			initialReview = &models.PullRequestReview{
				ID:             event.ID,
				WorkspaceID:    event.WorkspaceID,
				RepositoryID:   uuid.Nil,
				PullNumber:     event.PullRequestNumber,
				Title:          fmt.Sprintf("PR #%d: %s", event.PullRequestNumber, event.RepoNamespace),
				HeadSHA:        event.HeadSHA,
				BaseSHA:        event.BaseSHA,
				AuthorUsername: event.Sender,
				State:          models.ReviewStateQueued,
				FindingsCount:  0,
				CreatedAt:      time.Now().UTC(),
			}
		}

		if err := s.repo.IngestWebhookEventTx(ctx, initialReview, outboxEvent); err != nil {
			slog.Error("Failed inserting transactional webhook outbox event", "event_id", event.ID, "error", err)
			return nil, err
		}

		// Emit sandbox.invalidate event if PR closed or force-pushed/synchronized
		if event.PullRequestNumber > 0 && (event.Action == ingestion.ActionClosed || event.Action == ingestion.ActionSynchronize) {
			reason := contracts.ReasonPRClosed
			if event.Action == ingestion.ActionSynchronize {
				reason = contracts.ReasonForcePushed
			}
			orgID := event.WorkspaceID.String()
			if event.WorkspaceID != uuid.Nil {
				if prKey, err := contracts.BuildPrKey(orgID, event.RepoNamespace, event.PullRequestNumber); err == nil {
					invalPayload, _ := json.Marshal(contracts.SandboxInvalidatePayload{
						PrKey:  prKey,
						Reason: reason,
					})
					_ = s.repo.InsertOutboxEvent(ctx, &models.OutboxRecord{
						ID:          uuid.New(),
						WorkspaceID: event.WorkspaceID,
						EventType:   contracts.RoutingKeySandboxInvalidate,
						Payload:     invalPayload,
						Status:      models.OutboxPending,
					})
					slog.Info("Emitted sandbox invalidation event", "pr_key", prKey, "reason", reason)
				}
			}
		}
	} else if s.outbox != nil {

		_ = s.outbox.Insert(ctx, relay.OutboxMessage{
			ID:          event.ID,
			WorkspaceID: event.WorkspaceID,
			Topic:       "scandrix.webhooks.received",
			Payload:     eventBytes,
			CreatedAt:   time.Now().UTC(),
		})
	}

	return event, nil
}

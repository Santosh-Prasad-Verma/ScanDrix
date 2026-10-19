package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

// WebhookHealthRepository defines the data access contract for webhook delivery and outbox metrics (Clean Architecture).
type WebhookHealthRepository interface {
	GetOutboxMetrics(ctx context.Context, wsID uuid.UUID) (totalDelivered, pending, retrying, dlq int64, lastEventAt *time.Time, err error)
	RetryDeadLetterOutboxEvents(ctx context.Context, limit int) (int, error)
	ListOutboxEvents(ctx context.Context, wsID uuid.UUID, limit int, status string) ([]models.OutboxRecord, error)
	RetrySingleOutboxEvent(ctx context.Context, wsID, eventID uuid.UUID) (bool, error)
}

// WebhookHealthController inspects webhook delivery reliability and outbox message backlog.
type WebhookHealthController struct {
	repo WebhookHealthRepository
}

// NewWebhookHealthController initializes the webhook health controller with repository dependency.
func NewWebhookHealthController(repo WebhookHealthRepository) *WebhookHealthController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &WebhookHealthController{repo: repo}
}

// Routes mounts health and queue observability endpoints.
func (c *WebhookHealthController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/webhooks", c.handleGetWebhookHealth)
	r.Get("/outbox-lag", c.handleGetOutboxLag)
	r.Post("/retry-dlq", c.handleRetryDLQ)
	r.Get("/outbox-events", c.handleListOutboxEvents)
	r.Post("/outbox-events/{id}/retry", c.handleRetrySingleEvent)

	return r
}

func (c *WebhookHealthController) handleGetWebhookHealth(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var delivered, pending, retrying, dlq int64
	var lastEventAt *time.Time
	if c.repo != nil {
		var err error
		delivered, pending, retrying, dlq, lastEventAt, err = c.repo.GetOutboxMetrics(r.Context(), wsID)
		if err != nil {
			http.Error(w, `{"error":"failed querying webhook health"}`, http.StatusInternalServerError)
			return
		}
	}

	total := delivered + pending + retrying + dlq
	var successRate *float64
	var unavailable []string
	if total > 0 {
		sr := float64(delivered) / float64(total) * 100.0
		successRate = &sr
	} else {
		unavailable = append(unavailable, "success_rate:insufficient_data")
	}

	if lastEventAt == nil {
		unavailable = append(unavailable, "last_event_at:no_data_source")
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.WebhookHealthResponse{
		TotalDelivered: delivered,
		SuccessRate:    successRate,
		RecentFailures: int(dlq),
		LastEventAt:    lastEventAt,
		Unavailable:    unavailable,
	})
}

func (c *WebhookHealthController) handleGetOutboxLag(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var pending, retrying, dlq int64
	var lastEventAt *time.Time
	if c.repo != nil {
		var err error
		_, pending, retrying, dlq, lastEventAt, err = c.repo.GetOutboxMetrics(r.Context(), wsID)
		if err != nil {
			http.Error(w, `{"error":"failed querying outbox lag"}`, http.StatusInternalServerError)
			return
		}
	}

	oldest := time.Now().UTC()
	if lastEventAt != nil {
		oldest = *lastEventAt
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.OutboxLagResponse{
		PendingCount:    int(pending),
		RetryingCount:   int(retrying),
		DeadLetterCount: int(dlq),
		OldestPendingAt: oldest,
	})
}

func (c *WebhookHealthController) handleRetryDLQ(w http.ResponseWriter, r *http.Request) {
	requeued := 0
	if c.repo != nil {
		if count, err := c.repo.RetryDeadLetterOutboxEvents(r.Context(), 100); err == nil {
			requeued = count
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":            "RETRY_INITIATED",
		"requeued_messages": requeued,
	})
}

func (c *WebhookHealthController) handleListOutboxEvents(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 && val <= 200 {
			limit = val
		}
	}
	status := r.URL.Query().Get("status")

	var records []models.OutboxRecord
	if c.repo != nil {
		var err error
		records, err = c.repo.ListOutboxEvents(r.Context(), wsID, limit, status)
		if err != nil {
			http.Error(w, `{"error":"failed querying outbox events"}`, http.StatusInternalServerError)
			return
		}
	}
	if records == nil {
		records = []models.OutboxRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(records)
}

func (c *WebhookHealthController) handleRetrySingleEvent(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	eventIDStr := chi.URLParam(r, "id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid event id"}`, http.StatusBadRequest)
		return
	}

	success := true
	if c.repo != nil {
		var err error
		success, err = c.repo.RetrySingleOutboxEvent(r.Context(), wsID, eventID)
		if err != nil {
			http.Error(w, `{"error":"failed scheduling event redelivery"}`, http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":   "REPLAY_SCHEDULED",
		"event_id": eventID.String(),
		"success":  success,
	})
}

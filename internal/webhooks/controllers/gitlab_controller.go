package controllers

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/webhooks/ingestion"
	"github.com/scandrix/backend/pkg/models"
)

// GitLabController handles GitLab webhook deliveries.
type GitLabController struct {
	enqueueSvc *WebhookEnqueueService
}

// NewGitLabController creates a new GitLab webhook controller.
func NewGitLabController(enqueueSvc *WebhookEnqueueService) *GitLabController {
	return &GitLabController{enqueueSvc: enqueueSvc}
}

// RegisterRoutes registers the controller endpoints on a Chi router.
func (c *GitLabController) RegisterRoutes(r chi.Router) {
	r.Post("/webhook", c.HandleWebhook)
}

// HandleWebhook processes an inbound GitLab webhook delivery.
func (c *GitLabController) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	event := r.Header.Get("X-Gitlab-Event")
	if event == "" {
		http.Error(w, "missing X-Gitlab-Event header", http.StatusBadRequest)
		return
	}

	body, err := ReadPayload(w, r)
	if err != nil {
		http.Error(w, "failed to read payload", http.StatusBadRequest)
		return
	}

	// 1. Filter unsupported events before enqueuing
	supportedEvents := map[string]bool{
		"Merge Request Hook": true,
		"Note Hook":          true,
		"Push Hook":          true,
	}
	if !supportedEvents[event] {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Webhook ignored (event not supported)"))
		return
	}

	// 2. Secret & Token verification
	repoNamespace, _ := ingestion.ExtractRepoNamespace(body)
	secret, _ := c.enqueueSvc.Resolver().ResolveSecret(models.ProviderGitLab, repoNamespace)
	token := r.Header.Get("X-Gitlab-Token")
	if !c.enqueueSvc.Verifier().VerifyGitLab(token, secret) {
		slog.Warn("GitLab webhook token verification failed", "repo", repoNamespace)
		http.Error(w, "invalid webhook token", http.StatusUnauthorized)
		return
	}

	// 3. Enqueue to outbox / database
	deliveryID := r.Header.Get("X-Gitlab-Event-UUID")
	_, err = c.enqueueSvc.EnqueueEvent(r.Context(), models.ProviderGitLab, event, body, EnqueueOptions{
		DeliveryID: deliveryID,
	})
	if err != nil {
		slog.Error("Error enqueuing GitLab webhook", "error", err, "event", event)
		http.Error(w, "Failed to process webhook", http.StatusInternalServerError)
		return
	}

	slog.Info("GitLab webhook enqueued", "event", event, "repo", repoNamespace, "delivery_id", deliveryID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Webhook received"))
}

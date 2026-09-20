package controllers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/webhooks/ingestion"
	"github.com/scandrix/backend/pkg/models"
)

// GitHubController handles GitHub webhook deliveries.
type GitHubController struct {
	enqueueSvc *WebhookEnqueueService
}

// NewGitHubController creates a new GitHub webhook controller.
func NewGitHubController(enqueueSvc *WebhookEnqueueService) *GitHubController {
	return &GitHubController{enqueueSvc: enqueueSvc}
}

// RegisterRoutes registers the controller endpoints on a Chi router.
func (c *GitHubController) RegisterRoutes(r chi.Router) {
	r.Post("/webhook", c.HandleWebhook)
}

// HandleWebhook processes an inbound GitHub webhook delivery.
func (c *GitHubController) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	event := r.Header.Get("X-GitHub-Event")
	if event == "" {
		http.Error(w, "missing X-GitHub-Event header", http.StatusBadRequest)
		return
	}

	body, err := ReadPayload(w, r)
	if err != nil {
		http.Error(w, "failed to read payload", http.StatusBadRequest)
		return
	}

	// 1. Filter unsupported events before enqueuing
	supportedEvents := map[string]bool{
		"pull_request":                true,
		"issue_comment":               true,
		"pull_request_review_comment": true,
		"push":                        true,
		"installation":                true,
		"installation_repositories":   true,
	}
	if !supportedEvents[event] {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Webhook ignored (event not supported)"))
		return
	}

	// 2. For pull_request events, filter unsupported actions
	var raw struct {
		Action     string `json:"action"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	_ = json.Unmarshal(body, &raw)

	if event == "pull_request" {
		allowedActions := map[string]bool{
			"opened":           true,
			"synchronize":      true,
			"closed":           true,
			"reopened":         true,
			"ready_for_review": true,
		}
		if !allowedActions[raw.Action] {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Webhook ignored (action not supported)"))
			return
		}
	}

	// 3. Signature verification
	repoNamespace := raw.Repository.FullName
	if repoNamespace == "" {
		repoNamespace, _ = ingestion.ExtractRepoNamespace(body)
	}

	secret, _ := c.enqueueSvc.Resolver().ResolveSecret(models.ProviderGitHub, repoNamespace)
	sig := r.Header.Get("X-Hub-Signature-256")
	if !c.enqueueSvc.Verifier().VerifyGitHub(sig, body, secret) {
		slog.Warn("GitHub webhook signature verification failed", "repo", repoNamespace)
		http.Error(w, "invalid webhook signature", http.StatusUnauthorized)
		return
	}

	// 4. Enqueue to outbox / database
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	_, err = c.enqueueSvc.EnqueueEvent(r.Context(), models.ProviderGitHub, event, body, EnqueueOptions{
		DeliveryID: deliveryID,
	})
	if err != nil {
		slog.Error("Error enqueuing GitHub webhook", "error", err, "event", event)
		http.Error(w, "Failed to process webhook", http.StatusInternalServerError)
		return
	}

	slog.Info("GitHub webhook enqueued", "event", event, "repo", repoNamespace, "delivery_id", deliveryID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Webhook received"))
}

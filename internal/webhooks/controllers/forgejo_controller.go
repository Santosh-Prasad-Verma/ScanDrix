package controllers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/webhooks/ingestion"
	"github.com/scandrix/backend/pkg/models"
)

// ForgejoController handles Forgejo and Gitea webhooks.
type ForgejoController struct {
	enqueueSvc *WebhookEnqueueService
}

// NewForgejoController creates a new Forgejo webhook controller.
func NewForgejoController(enqueueSvc *WebhookEnqueueService) *ForgejoController {
	return &ForgejoController{enqueueSvc: enqueueSvc}
}

// RegisterRoutes registers the controller endpoints on a Chi router.
func (c *ForgejoController) RegisterRoutes(r chi.Router) {
	r.Post("/webhook", c.HandleWebhook)
}

// HandleWebhook processes an inbound Forgejo/Gitea webhook delivery.
func (c *ForgejoController) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// Forgejo uses X-Forgejo-Event header, but also supports X-Gitea-Event,
	// X-GitHub-Event and X-Gogs-Event for compatibility
	event := r.Header.Get("X-Forgejo-Event")
	if event == "" {
		event = r.Header.Get("X-Gitea-Event")
	}
	if event == "" {
		event = r.Header.Get("X-GitHub-Event")
	}
	if event == "" {
		event = r.Header.Get("X-Gogs-Event")
	}

	if event == "" {
		http.Error(w, "missing event header (X-Forgejo-Event/X-Gitea-Event)", http.StatusBadRequest)
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
			"opened":       true,
			"synchronized": true,
			"reopened":     true,
			"closed":       true,
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

	secret, _ := c.enqueueSvc.Resolver().ResolveSecret(models.ProviderForgejo, repoNamespace)
	sig := r.Header.Get("X-Gitea-Signature")
	if sig == "" {
		sig = r.Header.Get("X-Hub-Signature-256")
	}

	if secret != "" && sig != "" {
		if !c.enqueueSvc.Verifier().VerifyForgejo(sig, body, secret) {
			slog.Warn("Forgejo webhook signature verification failed", "repo", repoNamespace)
			http.Error(w, "invalid webhook signature", http.StatusUnauthorized)
			return
		}
	}

	// 4. Enqueue to outbox / database
	deliveryID := r.Header.Get("X-Delivery")
	if deliveryID == "" {
		deliveryID = r.Header.Get("X-Request-Id")
	}
	_, err = c.enqueueSvc.EnqueueEvent(r.Context(), models.ProviderForgejo, event, body, EnqueueOptions{
		DeliveryID: deliveryID,
	})
	if err != nil {
		slog.Error("Error enqueuing Forgejo webhook", "error", err, "event", event)
		http.Error(w, "Failed to process webhook", http.StatusInternalServerError)
		return
	}

	slog.Info("Forgejo webhook enqueued", "event", event, "repo", repoNamespace)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Webhook received"))
}

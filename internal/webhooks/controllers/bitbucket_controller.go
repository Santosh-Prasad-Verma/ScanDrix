package controllers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/webhooks/ingestion"
	"github.com/scandrix/backend/pkg/models"
)

// BitbucketController handles Bitbucket Cloud and Data Center webhooks.
type BitbucketController struct {
	enqueueSvc *WebhookEnqueueService
}

// NewBitbucketController creates a new Bitbucket webhook controller.
func NewBitbucketController(enqueueSvc *WebhookEnqueueService) *BitbucketController {
	return &BitbucketController{enqueueSvc: enqueueSvc}
}

// RegisterRoutes registers the controller endpoints on a Chi router.
func (c *BitbucketController) RegisterRoutes(r chi.Router) {
	r.Post("/webhook", c.HandleWebhook)
}

// HandleWebhook processes an inbound Bitbucket webhook delivery.
func (c *BitbucketController) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	event := r.Header.Get("X-Event-Key")
	if event == "" {
		http.Error(w, "missing X-Event-Key header", http.StatusBadRequest)
		return
	}

	body, err := ReadPayload(w, r)
	if err != nil {
		http.Error(w, "failed to read payload", http.StatusBadRequest)
		return
	}

	// 1. Filter unsupported events before enqueuing
	supportedEvents := map[string]bool{
		// Cloud
		"pullrequest:created":         true,
		"pullrequest:updated":         true,
		"pullrequest:fulfilled":       true,
		"pullrequest:rejected":        true,
		"pullrequest:comment_created": true,

		// Data Center / Server
		"pr:opened":           true,
		"pr:modified":         true,
		"pr:reviewer:updated": true,
		"pr:comment:added":    true,
		"pr:merged":           true,
		"pr:declined":         true,
	}
	if !supportedEvents[event] {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Webhook ignored (event not supported)"))
		return
	}

	// 2. Signature verification (if secret configured) against raw wire body
	repoNamespace, _ := ingestion.ExtractRepoNamespace(body)
	secret, _ := c.enqueueSvc.Resolver().ResolveSecret(models.ProviderBitbucket, repoNamespace)
	if secret != "" {
		sig := r.Header.Get("X-Hub-Signature")
		if !c.enqueueSvc.Verifier().VerifyBitbucket(sig, body, secret) {
			slog.Warn("Bitbucket webhook signature verification failed", "repo", repoNamespace)
			http.Error(w, "invalid webhook signature", http.StatusUnauthorized)
			return
		}
	}

	// 3. Normalize Bitbucket Data Center payload (`pullRequest` -> `pullrequest`)
	isDataCenter := strings.HasPrefix(event, "pr:")
	normalizedBody := body
	if isDataCenter {
		var genericMap map[string]any
		if err := json.Unmarshal(body, &genericMap); err == nil {
			if prVal, exists := genericMap["pullRequest"]; exists && genericMap["pullrequest"] == nil {
				genericMap["pullrequest"] = prVal
				if updatedBytes, err := json.Marshal(genericMap); err == nil {
					normalizedBody = updatedBytes
				}
			}
		}
	}

	// 4. Enqueue to outbox / database
	deliveryID := r.Header.Get("X-Hook-UUID")
	if deliveryID == "" {
		deliveryID = r.Header.Get("X-Request-Id")
	}
	_, err = c.enqueueSvc.EnqueueEvent(r.Context(), models.ProviderBitbucket, event, normalizedBody, EnqueueOptions{
		DeliveryID: deliveryID,
	})
	if err != nil {
		slog.Error("Error enqueuing Bitbucket webhook", "error", err, "event", event)
		http.Error(w, "Failed to process webhook", http.StatusInternalServerError)
		return
	}

	slog.Info("Bitbucket webhook enqueued", "event", event, "repo", repoNamespace, "is_data_center", isDataCenter)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Webhook received"))
}

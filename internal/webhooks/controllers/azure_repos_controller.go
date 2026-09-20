package controllers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/pkg/models"
)

// AzureReposController handles Azure DevOps / Azure Repos webhooks.
type AzureReposController struct {
	enqueueSvc           *WebhookEnqueueService
	codeManagementSecret string
	codeManagementToken  string
}

// NewAzureReposController creates a new Azure Repos webhook controller.
func NewAzureReposController(
	enqueueSvc *WebhookEnqueueService,
	codeManagementSecret string,
	codeManagementToken string,
) *AzureReposController {
	return &AzureReposController{
		enqueueSvc:           enqueueSvc,
		codeManagementSecret: codeManagementSecret,
		codeManagementToken:  codeManagementToken,
	}
}

// RegisterRoutes registers the controller endpoints on a Chi router.
func (c *AzureReposController) RegisterRoutes(r chi.Router) {
	r.Post("/webhook", c.HandleWebhook)
}

// HandleWebhook processes an inbound Azure DevOps webhook delivery.
func (c *AzureReposController) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// 1. Verify encrypted token from query param (AES-256-CBC) or header auth
	encryptedToken := r.URL.Query().Get("token")
	authenticated := false

	if encryptedToken != "" && c.codeManagementSecret != "" && c.codeManagementToken != "" {
		authenticated = c.enqueueSvc.Verifier().VerifyAzureWebhookToken(
			encryptedToken,
			c.codeManagementSecret,
			c.codeManagementToken,
		)
	}

	if !authenticated {
		// Fallback to Authorization header check
		authHeader := r.Header.Get("Authorization")
		secret, _ := c.enqueueSvc.Resolver().ResolveSecret(models.ProviderAzure, "")
		if secret != "" && authHeader != "" {
			authenticated = c.enqueueSvc.Verifier().VerifyAzureDevOps(authHeader, secret)
		}
	}

	if !authenticated {
		slog.Warn("Webhook Azure DevOps Not Token Valid")
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	body, err := ReadPayload(w, r)
	if err != nil {
		http.Error(w, "failed to read payload", http.StatusBadRequest)
		return
	}

	var payload struct {
		EventType string `json:"eventType"`
		Resource  struct {
			PullRequestID int `json:"pullRequestId"`
			Repository    struct {
				Name    string `json:"name"`
				Project struct {
					Name string `json:"name"`
				} `json:"project"`
			} `json:"repository"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.EventType == "" {
		slog.Info("Webhook Azure DevOps received without eventType")
		http.Error(w, "Unrecognized event", http.StatusBadRequest)
		return
	}

	// 2. Filter unsupported events before enqueuing
	supportedEvents := map[string]bool{
		"git.pullrequest.created":                   true,
		"git.pullrequest.updated":                   true,
		"git.pullrequest.merge.attempted":           true,
		"ms.vss-code.git-pullrequest-comment-event": true,
	}
	if !supportedEvents[payload.EventType] {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Webhook ignored (event not supported)"))
		return
	}

	// 3. Enqueue to outbox / database
	deliveryID := r.Header.Get("X-Vss-Subscriptionid")
	if deliveryID == "" {
		deliveryID = r.Header.Get("X-Request-Id")
	}
	_, err = c.enqueueSvc.EnqueueEvent(r.Context(), models.ProviderAzure, payload.EventType, body, EnqueueOptions{
		DeliveryID: deliveryID,
	})
	if err != nil {
		slog.Error("Error enqueuing Azure DevOps webhook", "error", err, "event", payload.EventType)
		http.Error(w, "Failed to process webhook", http.StatusInternalServerError)
		return
	}

	repoName := payload.Resource.Repository.Project.Name + "/" + payload.Resource.Repository.Name
	slog.Info("Azure DevOps webhook enqueued", "event", payload.EventType, "repo", repoName, "pr", payload.Resource.PullRequestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Webhook received"))
}

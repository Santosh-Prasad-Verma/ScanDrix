package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

// IngestionHandler validates and enqueues external Git provider webhook notifications.
type IngestionHandler struct {
	repo         *database.Repository
	githubSecret string
	gitlabSecret string
}

// NewIngestionHandler initializes the webhook receiver.
func NewIngestionHandler(repo *database.Repository, githubSecret, gitlabSecret string) *IngestionHandler {
	return &IngestionHandler{
		repo:         repo,
		githubSecret: githubSecret,
		gitlabSecret: gitlabSecret,
	}
}

// GitHubWebhookPayload models relevant fields from GitHub pull_request events.
type GitHubWebhookPayload struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		Title string `json:"title"`
		Head  struct {
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			SHA string `json:"sha"`
		} `json:"base"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
		ID       int64  `json:"id"`
	} `json:"repository"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
}

// HandleGitHub processes incoming GitHub App / Webhook payloads.
func (h *IngestionHandler) HandleGitHub(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}

	// Verify HMAC-SHA256 signature (Master Rule 5.5)
	if h.githubSecret != "" {
		sigHeader := r.Header.Get("X-Hub-Signature-256")
		if !verifyGitHubSignature(h.githubSecret, body, sigHeader) {
			http.Error(w, `{"error":"invalid webhook signature"}`, http.StatusUnauthorized)
			return
		}
	}

	eventType := r.Header.Get("X-GitHub-Event")
	if eventType != "pull_request" {
		// Acknowledge non-PR events (ping, push, check_run) without queuing review
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ignored_event"}`))
		return
	}

	var payload GitHubWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, `{"error":"malformed payload"}`, http.StatusBadRequest)
		return
	}

	// Only trigger reviews on actionable lifecycle events
	if payload.Action != "opened" && payload.Action != "synchronize" && payload.Action != "reopened" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ignored_action"}`))
		return
	}

	// Persist to transactional outbox for reliable asynchronous delivery
	// Default workspace fallback for demo/installation
	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	outboxEvent := &models.OutboxRecord{
		WorkspaceID: workspaceID,
		EventType:   "github.pull_request." + payload.Action,
		Payload:     body,
	}

	if err := h.repo.InsertOutboxEvent(r.Context(), outboxEvent); err != nil {
		http.Error(w, `{"error":"failed to record outbox event"}`, http.StatusInternalServerError)
		return
	}

	// Return 202 Accepted in <15ms
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"enqueued","event_id":"` + outboxEvent.ID.String() + `"}`))
}

func verifyGitHubSignature(secret string, body []byte, signatureHeader string) bool {
	if !strings.HasPrefix(signatureHeader, "sha256=") {
		return false
	}
	expectedHash := strings.TrimPrefix(signatureHeader, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	actualHash := hex.EncodeToString(mac.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) == 1
}

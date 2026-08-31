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
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/queue/consumer"
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
	} `json:"repository"`
}

// HandleGitHub processes incoming GitHub webhook payloads with fail-closed HMAC-SHA256 signature verification.
func (h *IngestionHandler) HandleGitHub(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}

	// Verify HMAC-SHA256 signature (Master Rule 5.5) - Fail Closed
	if h.githubSecret == "" {
		http.Error(w, `{"error":"unauthorized: github webhook secret is not configured"}`, http.StatusUnauthorized)
		return
	}
	sigHeader := r.Header.Get("X-Hub-Signature-256")
	if !verifyGitHubSignature(h.githubSecret, body, sigHeader) {
		http.Error(w, `{"error":"invalid webhook signature"}`, http.StatusUnauthorized)
		return
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

	// Resolve workspace from tracked_repositories instead of hardcoded fallback
	var workspaceID uuid.UUID
	if h.repo != nil {
		var err error
		workspaceID, err = h.repo.GetWorkspaceIDByRepoNamespace(r.Context(), "github", payload.Repository.FullName)
		if err != nil {
			http.Error(w, `{"error":"repository not tracked: no workspace mapping found"}`, http.StatusNotFound)
			return
		}
	} else {
		http.Error(w, `{"error":"internal: repository store unavailable"}`, http.StatusInternalServerError)
		return
	}
	taskID := uuid.New()
	eventID := uuid.New()

	reviewTask := consumer.ReviewTaskPayload{
		TaskID:            taskID,
		EventID:           eventID,
		WorkspaceID:       workspaceID,
		Provider:          models.ProviderGitHub,
		RepoNamespace:     payload.Repository.FullName,
		PullRequestNumber: payload.Number,
		HeadSHA:           payload.PullRequest.Head.SHA,
		BaseSHA:           payload.PullRequest.Base.SHA,
		Sender:            payload.PullRequest.User.Login,
		AttemptCount:      0,
		EnqueuedAt:        time.Now().UTC(),
	}

	taskBytes, err := json.Marshal(reviewTask)
	if err != nil {
		http.Error(w, `{"error":"failed to serialize task payload"}`, http.StatusInternalServerError)
		return
	}

	outboxEvent := &models.OutboxRecord{
		ID:          eventID,
		WorkspaceID: workspaceID,
		EventType:   "github.pull_request." + payload.Action,
		Payload:     taskBytes,
	}

	if h.repo != nil {
		if err := h.repo.InsertOutboxEvent(r.Context(), outboxEvent); err != nil {
			http.Error(w, `{"error":"failed to record outbox event"}`, http.StatusInternalServerError)
			return
		}
	}

	// Return 202 Accepted in <15ms
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"enqueued","event_id":"` + outboxEvent.ID.String() + `","task_id":"` + taskID.String() + `"}`))
}

// GitLabWebhookPayload models incoming GitLab Merge Request Hook payloads.
type GitLabWebhookPayload struct {
	ObjectKind string `json:"object_kind"`
	Project    struct {
		ID                int64  `json:"id"`
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
	ObjectAttributes struct {
		IID          int    `json:"iid"`
		Title        string `json:"title"`
		Action       string `json:"action"` // open, update, reopen, close, merge
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
		LastCommit   struct {
			ID string `json:"id"`
		} `json:"last_commit"`
	} `json:"object_attributes"`
	User struct {
		Username string `json:"username"`
	} `json:"user"`
}

// HandleGitLab processes incoming GitLab webhook payloads with token verification.
func (h *IngestionHandler) HandleGitLab(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}

	// Verify X-Gitlab-Token header (Master Rule 5.5) - Fail Closed
	if h.gitlabSecret == "" {
		http.Error(w, `{"error":"unauthorized: gitlab webhook secret is not configured"}`, http.StatusUnauthorized)
		return
	}
	tokenHeader := r.Header.Get("X-Gitlab-Token")
	if !verifyGitLabToken(h.gitlabSecret, tokenHeader) {
		http.Error(w, `{"error":"invalid webhook token"}`, http.StatusUnauthorized)
		return
	}

	eventType := r.Header.Get("X-Gitlab-Event")
	if eventType != "Merge Request Hook" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ignored_event"}`))
		return
	}

	var payload GitLabWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, `{"error":"malformed payload"}`, http.StatusBadRequest)
		return
	}

	action := payload.ObjectAttributes.Action
	if action != "open" && action != "update" && action != "reopen" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ignored_action"}`))
		return
	}

	// Resolve workspace from tracked_repositories instead of hardcoded fallback
	var workspaceID uuid.UUID
	if h.repo != nil {
		var err error
		workspaceID, err = h.repo.GetWorkspaceIDByRepoNamespace(r.Context(), "gitlab", payload.Project.PathWithNamespace)
		if err != nil {
			http.Error(w, `{"error":"repository not tracked: no workspace mapping found"}`, http.StatusNotFound)
			return
		}
	} else {
		http.Error(w, `{"error":"internal: repository store unavailable"}`, http.StatusInternalServerError)
		return
	}
	taskID := uuid.New()
	eventID := uuid.New()

	reviewTask := consumer.ReviewTaskPayload{
		TaskID:            taskID,
		EventID:           eventID,
		WorkspaceID:       workspaceID,
		Provider:          models.ProviderGitLab,
		RepoNamespace:     payload.Project.PathWithNamespace,
		PullRequestNumber: payload.ObjectAttributes.IID,
		HeadSHA:           payload.ObjectAttributes.LastCommit.ID,
		BaseSHA:           payload.ObjectAttributes.TargetBranch,
		Sender:            payload.User.Username,
		AttemptCount:      0,
		EnqueuedAt:        time.Now().UTC(),
	}

	taskBytes, err := json.Marshal(reviewTask)
	if err != nil {
		http.Error(w, `{"error":"failed to serialize task payload"}`, http.StatusInternalServerError)
		return
	}

	outboxEvent := &models.OutboxRecord{
		ID:          eventID,
		WorkspaceID: workspaceID,
		EventType:   "gitlab.merge_request." + action,
		Payload:     taskBytes,
	}

	if h.repo != nil {
		if err := h.repo.InsertOutboxEvent(r.Context(), outboxEvent); err != nil {
			http.Error(w, `{"error":"failed to record outbox event"}`, http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"enqueued","event_id":"` + outboxEvent.ID.String() + `","task_id":"` + taskID.String() + `"}`))
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

func verifyGitLabToken(secret, tokenHeader string) bool {
	if secret == "" || tokenHeader == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(tokenHeader), []byte(secret)) == 1
}

package ingestion

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/pkg/models"
)

// IngestionHandler handles inbound webhook HTTP requests from Git providers.
type IngestionHandler struct {
	verifier *WebhookVerifier
	parser   *WebhookParser
	resolver SecretResolver
	outbox   *relay.OutboxStore
}

// NewIngestionHandler initializes the HTTP receiver.
func NewIngestionHandler(resolver SecretResolver, outbox *relay.OutboxStore) *IngestionHandler {
	return &IngestionHandler{
		verifier: NewWebhookVerifier(),
		parser:   NewWebhookParser(),
		resolver: resolver,
		outbox:   outbox,
	}
}

// ServeHTTP routes and processes inbound webhook payloads.
func (h *IngestionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Limit max payload to 10MB
	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "request payload too large or unreadable", http.StatusBadRequest)
		return
	}

	// 2. Identify provider from header or path
	provider, eventType := detectProvider(r)
	if provider == "" {
		http.Error(w, "unknown SCM webhook provider", http.StatusBadRequest)
		return
	}

	// 3. Resolve secret and verify signature
	repo, _ := ExtractRepoNamespace(body)
	secret, _ := h.resolver.ResolveSecret(provider, repo)

	if !h.verifyRequest(provider, r, body, secret) {
		http.Error(w, "invalid webhook signature or unauthorized token", http.StatusUnauthorized)
		return
	}

	// 4. Parse into normalized event
	event, err := h.parser.Parse(provider, eventType, body)
	if err != nil {
		http.Error(w, "failed to parse webhook payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Replay / Delivery Deduplication: Extract unique delivery IDs from SCM providers
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	if deliveryID == "" {
		deliveryID = r.Header.Get("X-Gitlab-Event-UUID")
	}
	if deliveryID == "" {
		deliveryID = r.Header.Get("X-Hook-UUID")
	}
	if deliveryID == "" {
		deliveryID = r.Header.Get("X-Delivery")
	}
	if deliveryID == "" {
		deliveryID = r.Header.Get("X-Request-Id")
	}
	if deliveryID != "" {
		if parsedUUID, err := uuid.Parse(deliveryID); err == nil {
			event.ID = parsedUUID
		} else {
			event.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(string(provider)+":"+deliveryID))
		}
	}

	// Ignore events that don't require review
	if event.Action == ActionIgnored || event.Action == ActionClosed {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ignored", "reason": "action does not trigger review"})
		return
	}

	// 5. Transactional Outbox Write
	eventBytes, _ := json.Marshal(event)
	if h.outbox != nil {
		_ = h.outbox.Insert(r.Context(), relay.OutboxMessage{
			ID:          event.ID,
			WorkspaceID: event.WorkspaceID,
			Topic:       "scandrix.webhooks.received",
			Payload:     eventBytes,
			CreatedAt:   time.Now().UTC(),
		})
	}

	// 6. Return 202 Accepted immediately
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(IngestionResult{
		Status:     "accepted",
		EventID:    event.ID,
		Message:    "webhook successfully queued for code review analysis",
		ReceivedAt: event.ReceivedAt,
	})
}

func detectProvider(r *http.Request) (models.SCMProvider, string) {
	if r.Header.Get("X-GitHub-Event") != "" {
		return models.ProviderGitHub, r.Header.Get("X-GitHub-Event")
	}
	if r.Header.Get("X-Gitlab-Event") != "" {
		return models.ProviderGitLab, r.Header.Get("X-Gitlab-Event")
	}
	if r.Header.Get("X-Event-Key") != "" {
		return models.ProviderBitbucket, r.Header.Get("X-Event-Key")
	}
	if r.Header.Get("X-Gitea-Event") != "" {
		return models.ProviderForgejo, r.Header.Get("X-Gitea-Event")
	}
	if r.Header.Get("X-Vss-Subscriptionid") != "" || r.URL.Query().Get("provider") == "azure" {
		return models.ProviderAzure, "git.pullrequest.created"
	}

	// Path fallback
	path := r.URL.Path
	if len(path) > 0 {
		return models.ProviderGitHub, "pull_request"
	}
	return "", ""
}

func (h *IngestionHandler) verifyRequest(provider models.SCMProvider, r *http.Request, body []byte, secret string) bool {
	if secret == "" {
		return false
	}

	switch provider {
	case models.ProviderGitHub:
		sig := r.Header.Get("X-Hub-Signature-256")
		return h.verifier.VerifyGitHub(sig, body, secret)
	case models.ProviderGitLab:
		tok := r.Header.Get("X-Gitlab-Token")
		return h.verifier.VerifyGitLab(tok, secret)
	case models.ProviderBitbucket:
		sig := r.Header.Get("X-Hub-Signature")
		return h.verifier.VerifyBitbucket(sig, body, secret)
	case models.ProviderForgejo:
		sig := r.Header.Get("X-Gitea-Signature")
		return h.verifier.VerifyForgejo(sig, body, secret)
	case models.ProviderAzure:
		auth := r.Header.Get("Authorization")
		return h.verifier.VerifyAzureDevOps(auth, secret)
	default:
		return false
	}
}

// IngestionResultUUID Helper constructor
func NewEventID() uuid.UUID {
	return uuid.New()
}

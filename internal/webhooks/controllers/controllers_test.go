package controllers_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/internal/webhooks/controllers"
	"github.com/scandrix/backend/internal/webhooks/ingestion"
)

func setupTestRouter() (*chi.Mux, string, string, string) {
	secret := "test-secret-key-123"
	codeSecret := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	codePlainToken := "azure-webhook-plain-token"

	resolver := ingestion.NewStaticSecretResolver(map[string]string{
		"github:acme/repo":    secret,
		"github":              secret,
		"gitlab":              secret,
		"bitbucket":           secret,
		"azure":               secret,
		"forgejo":             secret,
	})
	outbox := relay.NewOutboxStore()
	enqueueSvc := controllers.NewWebhookEnqueueService(nil, outbox, resolver)

	r := chi.NewRouter()

	ghCtrl := controllers.NewGitHubController(enqueueSvc)
	glCtrl := controllers.NewGitLabController(enqueueSvc)
	bbCtrl := controllers.NewBitbucketController(enqueueSvc)
	azCtrl := controllers.NewAzureReposController(enqueueSvc, codeSecret, codePlainToken)
	fjCtrl := controllers.NewForgejoController(enqueueSvc)
	blCtrl := controllers.NewBillingController(nil, secret)
	hlCtrl := controllers.NewHealthController(nil, nil)

	r.Post("/github/webhook", ghCtrl.HandleWebhook)
	r.Post("/gitlab/webhook", glCtrl.HandleWebhook)
	r.Post("/bitbucket/webhook", bbCtrl.HandleWebhook)
	r.Post("/azure-repos/webhook", azCtrl.HandleWebhook)
	r.Post("/forgejo/webhook", fjCtrl.HandleWebhook)
	r.Route("/billing/webhook", func(br chi.Router) {
		blCtrl.RegisterRoutes(br)
	})
	r.Route("/health", func(hr chi.Router) {
		hlCtrl.RegisterRoutes(hr)
	})

	return r, secret, codeSecret, codePlainToken
}

func TestGitHubController(t *testing.T) {
	r, secret, _, _ := setupTestRouter()

	body := []byte(`{"action":"opened","pull_request":{"number":1,"title":"test"},"repository":{"full_name":"acme/repo"}}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// 1. Valid pull request opened
	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", validSig)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "Webhook received" {
		t.Fatalf("expected 200 Webhook received, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Unsupported event -> 200 ignored
	req = httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "star")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "Webhook ignored (event not supported)" {
		t.Fatalf("expected ignored event, got %d: %s", w.Code, w.Body.String())
	}

	// 3. Unsupported PR action (e.g. labeled) -> 200 ignored
	labeledBody := []byte(`{"action":"labeled","pull_request":{"number":1},"repository":{"full_name":"acme/repo"}}`)
	req = httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(labeledBody))
	req.Header.Set("X-GitHub-Event", "pull_request")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "Webhook ignored (action not supported)" {
		t.Fatalf("expected ignored action, got %d: %s", w.Code, w.Body.String())
	}

	// 4. Invalid signature -> 401
	req = httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestGitLabController(t *testing.T) {
	r, secret, _, _ := setupTestRouter()

	body := []byte(`{"event_type":"merge_request","project":{"path_with_namespace":"acme/repo"},"object_attributes":{"id":10,"iid":1,"title":"test MR"}}`)

	// 1. Valid merge request
	req := httptest.NewRequest(http.MethodPost, "/gitlab/webhook", bytes.NewReader(body))
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	req.Header.Set("X-Gitlab-Token", secret)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "Webhook received" {
		t.Fatalf("expected 200 Webhook received, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Unsupported event
	req = httptest.NewRequest(http.MethodPost, "/gitlab/webhook", bytes.NewReader(body))
	req.Header.Set("X-Gitlab-Event", "Pipeline Hook")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "Webhook ignored (event not supported)" {
		t.Fatalf("expected ignored event, got %d: %s", w.Code, w.Body.String())
	}

	// 3. Invalid token
	req = httptest.NewRequest(http.MethodPost, "/gitlab/webhook", bytes.NewReader(body))
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	req.Header.Set("X-Gitlab-Token", "wrong-token")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestBitbucketController(t *testing.T) {
	r, secret, _, _ := setupTestRouter()

	// 1. Cloud event
	body := []byte(`{"repository":{"full_name":"acme/repo"},"actor":{"username":"dev"},"pullrequest":{"id":1,"title":"BB test","source":{"commit":{"hash":"abc"}},"destination":{"commit":{"hash":"def"}}}}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/bitbucket/webhook", bytes.NewReader(body))
	req.Header.Set("X-Event-Key", "pullrequest:created")
	req.Header.Set("X-Hub-Signature", validSig)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "Webhook received" {
		t.Fatalf("expected 200 Webhook received, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Data Center event with pullRequest uppercase normalization
	dcBody := []byte(`{"repository":{"full_name":"acme/repo"},"actor":{"username":"dev"},"pullRequest":{"id":2,"title":"BB DC test","source":{"commit":{"hash":"abc"}},"destination":{"commit":{"hash":"def"}}}}`)
	mac2 := hmac.New(sha256.New, []byte(secret))
	mac2.Write(dcBody)
	validSig2 := "sha256=" + hex.EncodeToString(mac2.Sum(nil))

	req = httptest.NewRequest(http.MethodPost, "/bitbucket/webhook", bytes.NewReader(dcBody))
	req.Header.Set("X-Event-Key", "pr:opened")
	req.Header.Set("X-Hub-Signature", validSig2)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "Webhook received" {
		t.Fatalf("expected 200 Webhook received for Data Center, got %d: %s", w.Code, w.Body.String())
	}

	// 3. Unsupported event
	req = httptest.NewRequest(http.MethodPost, "/bitbucket/webhook", bytes.NewReader(body))
	req.Header.Set("X-Event-Key", "repo:push")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "Webhook ignored (event not supported)" {
		t.Fatalf("expected ignored event, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAzureReposController(t *testing.T) {
	r, _, codeSecret, codePlainToken := setupTestRouter()

	body := []byte(`{"eventType":"git.pullrequest.created","resource":{"pullRequestId":42,"title":"Azure PR","repository":{"name":"test-repo","project":{"name":"test-proj"}},"createdBy":{"displayName":"developer"},"lastMergeSourceCommit":{"commitId":"c0ffee"}}}`)

	encToken, err := ingestion.GenerateAzureWebhookToken(codePlainToken, codeSecret)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 1. Valid encrypted token in query
	req := httptest.NewRequest(http.MethodPost, "/azure-repos/webhook?token="+encToken, bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "Webhook received" {
		t.Fatalf("expected 200 Webhook received, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Missing/invalid token -> 403
	req = httptest.NewRequest(http.MethodPost, "/azure-repos/webhook", bytes.NewReader(body))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}

	// 3. Header fallback
	req = httptest.NewRequest(http.MethodPost, "/azure-repos/webhook", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-secret-key-123")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with header auth, got %d", w.Code)
	}
}

func TestForgejoController(t *testing.T) {
	r, secret, _, _ := setupTestRouter()

	body := []byte(`{"action":"opened","pull_request":{"number":10,"title":"Forgejo PR"},"repository":{"full_name":"acme/repo"}}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	headers := []string{"X-Forgejo-Event", "X-Gitea-Event", "X-GitHub-Event", "X-Gogs-Event"}
	for _, h := range headers {
		req := httptest.NewRequest(http.MethodPost, "/forgejo/webhook", bytes.NewReader(body))
		req.Header.Set(h, "pull_request")
		req.Header.Set("X-Gitea-Signature", validSig)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK || w.Body.String() != "Webhook received" {
			t.Fatalf("failed for header %s: got %d: %s", h, w.Code, w.Body.String())
		}
	}
}

func TestBillingController(t *testing.T) {
	r, secret, _, _ := setupTestRouter()

	makeSignedReq := func(path string, payload any) *http.Request {
		b, _ := json.Marshal(payload)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(b)
		sig := hex.EncodeToString(mac.Sum(nil))

		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
		req.Header.Set("x-scandrix-signature", sig)
		return req
	}

	// 1. payment-failed
	pfReq := makeSignedReq("/billing/webhook/payment-failed", map[string]any{
		"organizationId": "org-123",
		"amount":         299.00,
		"currency":       "USD",
		"failureReason":  "Insufficient funds",
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, pfReq)
	if w.Code != http.StatusOK || w.Body.String() != "ok" {
		t.Fatalf("expected 200 ok for payment-failed, got %d: %s", w.Code, w.Body.String())
	}

	// 2. trial-expiring
	teReq := makeSignedReq("/billing/webhook/trial-expiring", map[string]any{
		"organizationId": "org-123",
		"daysRemaining":  3,
	})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, teReq)
	if w.Code != http.StatusOK || w.Body.String() != "ok" {
		t.Fatalf("expected 200 ok for trial-expiring, got %d: %s", w.Code, w.Body.String())
	}

	// 3. plan-changed
	pcReq := makeSignedReq("/billing/webhook/plan-changed", map[string]any{
		"organizationId": "org-123",
		"planType":       "ENTERPRISE",
	})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, pcReq)
	if w.Code != http.StatusOK || w.Body.String() != "ok" {
		t.Fatalf("expected 200 ok for plan-changed, got %d: %s", w.Code, w.Body.String())
	}

	// 4. missing org ID -> 400
	badReq := makeSignedReq("/billing/webhook/plan-changed", map[string]any{
		"planType": "ENTERPRISE",
	})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, badReq)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}

	// 5. invalid signature -> 401
	rawBody := []byte(`{"organizationId":"org-1"}`)
	invalidSigReq := httptest.NewRequest(http.MethodPost, "/billing/webhook/plan-changed", bytes.NewReader(rawBody))
	invalidSigReq.Header.Set("x-scandrix-signature", "invalid-sig")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, invalidSigReq)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", w.Code)
	}
}

func TestHealthController(t *testing.T) {
	r, _, _, _ := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /health, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /health/live, got %d", w.Code)
	}
}

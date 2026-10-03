package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/auth"
)

// TestRegisterTurnstileFailsClosed pins AUDIT_REMEDIATION.md F-20.
//
// The guard used to be `if secret != "" && token != ""`. Once a secret was
// configured, an attacker who simply omitted turnstile_token skipped bot
// verification completely -- protection that is only on when the client
// cooperates is not protection.
//
// The handler is wired with a nil repository on purpose. The Turnstile check
// runs BEFORE the repository availability check, so a 400 here proves the
// challenge gate fired; a 503 would mean it was skipped and the request
// proceeded to the account-creation path.
func TestRegisterTurnstileFailsClosed(t *testing.T) {
	ctrl := NewAuthController(auth.NewAuthenticator("test-jwt-secret-key-123456789012"), nil)
	ctrl.SetRequireEmailVerification(false)
	ctrl.SetTurnstileSecretKey("test-turnstile-secret-placeholder")

	post := func(t *testing.T, token string) *httptest.ResponseRecorder {
		t.Helper()
		body := map[string]any{
			"email":    "turnstile-probe@example.com",
			"password": "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8",
			"name":     "Turnstile Probe",
		}
		if token != "" {
			body["turnstile_token"] = token
		}
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ctrl.Routes().ServeHTTP(w, req)
		return w
	}

	t.Run("missing token is refused", func(t *testing.T) {
		w := post(t, "")
		if w.Code == http.StatusServiceUnavailable {
			t.Fatalf("request reached the repository: the challenge gate was skipped (503)")
		}
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 when turnstile_token is omitted, got %d: %s", w.Code, w.Body.String())
		}
		if !bytes.Contains(w.Body.Bytes(), []byte("challenge is required")) {
			t.Fatalf("expected an explicit 'challenge is required' error, got: %s", w.Body.String())
		}
	})

	t.Run("whitespace token is refused", func(t *testing.T) {
		w := post(t, "   ")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for a whitespace-only token, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unconfigured secret does not gate registration", func(t *testing.T) {
		ctrl.SetTurnstileSecretKey("")
		w := post(t, "")
		// With no secret configured there is no challenge to enforce, so the
		// request must get past the gate. A nil repository then yields 503,
		// which is the expected outcome here.
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected the request to pass the (disabled) gate and reach the repo (503), got %d: %s",
				w.Code, w.Body.String())
		}
	})
}

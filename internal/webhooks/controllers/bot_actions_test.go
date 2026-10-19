package controllers_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/webhooks/controllers"
)

func generateSlackHeaders(secret string, body []byte) (string, string) {
	ts := fmt.Sprintf("%d", time.Now().UTC().Unix())
	base := fmt.Sprintf("v0:%s:%s", ts, string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(base))
	sig := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return sig, ts
}

func TestSlackAction_ApprovePR(t *testing.T) {
	secret := "test_slack_signing_secret_123"
	ctrl := controllers.NewBotActionsController(nil, secret, "")

	payloadJSON := `{
		"type": "block_actions",
		"user": {"id": "U123", "username": "alice"},
		"actions": [
			{
				"action_id": "approve_pr",
				"value": "{\"repo\":\"acme/platform\",\"pr_number\":42,\"action\":\"approve_pr\"}"
			}
		]
	}`

	form := url.Values{}
	form.Set("payload", payloadJSON)
	rawBody := form.Encode()

	sig, ts := generateSlackHeaders(secret, []byte(rawBody))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/slack/actions", bytes.NewBufferString(rawBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Signature", sig)
	req.Header.Set("X-Slack-Request-Timestamp", ts)

	rr := httptest.NewRecorder()
	ctrl.HandleSlackActions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, "APPROVED") || !strings.Contains(body, "alice") {
		t.Errorf("unexpected Slack response body: %s", body)
	}
}

func TestSlackAction_InvalidSignature(t *testing.T) {
	secret := "test_slack_signing_secret_123"
	ctrl := controllers.NewBotActionsController(nil, secret, "")

	rawBody := "payload={}"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/slack/actions", bytes.NewBufferString(rawBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Signature", "v0=bad_signature")
	req.Header.Set("X-Slack-Request-Timestamp", fmt.Sprintf("%d", time.Now().UTC().Unix()))

	rr := httptest.NewRecorder()
	ctrl.HandleSlackActions(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for bad signature, got %d", rr.Code)
	}
}

func TestSlackAction_RequestChanges(t *testing.T) {
	secret := "test_slack_signing_secret_123"
	ctrl := controllers.NewBotActionsController(nil, secret, "")

	payloadJSON := `{
		"type": "block_actions",
		"user": {"id": "U456", "username": "bob"},
		"actions": [
			{
				"action_id": "request_changes",
				"value": "{\"repo\":\"acme/frontend\",\"pr_number\":99,\"action\":\"request_changes\"}"
			}
		]
	}`

	form := url.Values{}
	form.Set("payload", payloadJSON)
	rawBody := form.Encode()

	sig, ts := generateSlackHeaders(secret, []byte(rawBody))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/slack/actions", bytes.NewBufferString(rawBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Signature", sig)
	req.Header.Set("X-Slack-Request-Timestamp", ts)

	rr := httptest.NewRecorder()
	ctrl.HandleSlackActions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, "CHANGES REQUESTED") || !strings.Contains(body, "bob") {
		t.Errorf("expected CHANGES REQUESTED response, got: %s", body)
	}
}

func TestTeamsAction_ApprovePR(t *testing.T) {
	secret := "teams_shared_token_999"
	ctrl := controllers.NewBotActionsController(nil, "", secret)

	teamsPayload := map[string]any{
		"type": "invoke",
		"name": "adaptiveCard/action",
		"from": map[string]any{
			"id":   "teams-user-1",
			"name": "Carol Danvers",
		},
		"value": map[string]any{
			"action":    "approve_pr",
			"repo":      "acme/core-api",
			"pr_number": 15,
		},
	}
	bodyBytes, _ := json.Marshal(teamsPayload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/teams/actions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+secret)

	rr := httptest.NewRecorder()
	ctrl.HandleTeamsActions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, "APPROVED") || !strings.Contains(body, "Carol Danvers") {
		t.Errorf("expected Adaptive Card response with approval, got: %s", body)
	}
}

func TestTeamsAction_Unauthorized(t *testing.T) {
	secret := "teams_shared_token_999"
	ctrl := controllers.NewBotActionsController(nil, "", secret)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/teams/actions", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer invalid_token")

	rr := httptest.NewRecorder()
	ctrl.HandleTeamsActions(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rr.Code)
	}
}

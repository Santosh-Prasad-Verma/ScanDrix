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

	"github.com/scandrix/backend/internal/webhooks/controllers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPMWebhooksController_Jira_SecretVerification(t *testing.T) {
	secret := "test-jira-secret-xyz"
	ctrl := controllers.NewPMWebhooksController(nil, secret, "")

	payload := []byte(`{
		"webhookEvent": "jira:issue_updated",
		"issue": {
			"key": "SCA-101",
			"fields": {
				"status": {
					"name": "Done",
					"statusCategory": { "key": "done", "name": "Done" }
				}
			}
		}
	}`)

	// 1. Missing secret -> 401 Unauthorized
	req, _ := http.NewRequest(http.MethodPost, "/jira", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	ctrl.HandleJiraWebhook(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	// 2. Query param secret -> 200 OK
	reqWithQuery, _ := http.NewRequest(http.MethodPost, "/jira?secret="+secret, bytes.NewReader(payload))
	rr2 := httptest.NewRecorder()
	ctrl.HandleJiraWebhook(rr2, reqWithQuery)
	assert.Equal(t, http.StatusOK, rr2.Code)

	var resp map[string]any
	err := json.Unmarshal(rr2.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "processed", resp["status"])
	assert.Equal(t, "jira", resp["platform"])
	assert.Equal(t, "SCA-101", resp["ticket_key"])
	assert.Equal(t, "RESOLVED", resp["target_status"])

	// 3. Header secret -> 200 OK
	reqWithHeader, _ := http.NewRequest(http.MethodPost, "/jira", bytes.NewReader(payload))
	reqWithHeader.Header.Set("X-Atlassian-Webhook-Identifier", secret)
	rr3 := httptest.NewRecorder()
	ctrl.HandleJiraWebhook(rr3, reqWithHeader)
	assert.Equal(t, http.StatusOK, rr3.Code)
}

func TestPMWebhooksController_Jira_WontFix_Dismissed(t *testing.T) {
	ctrl := controllers.NewPMWebhooksController(nil, "", "")

	payload := []byte(`{
		"webhookEvent": "jira:issue_updated",
		"issue": {
			"key": "SCA-404",
			"fields": {
				"status": {
					"name": "Closed",
					"statusCategory": { "key": "done", "name": "Done" }
				},
				"resolution": {
					"name": "Won't Fix"
				}
			}
		}
	}`)

	req, _ := http.NewRequest(http.MethodPost, "/jira", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	ctrl.HandleJiraWebhook(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp map[string]any
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "SCA-404", resp["ticket_key"])
	assert.Equal(t, "DISMISSED", resp["target_status"])
}

func TestPMWebhooksController_Linear_HMAC_Verification(t *testing.T) {
	secret := "test-linear-secret-abc"
	ctrl := controllers.NewPMWebhooksController(nil, "", secret)

	payload := []byte(`{
		"action": "update",
		"type": "Issue",
		"data": {
			"id": "lin_issue_123",
			"identifier": "ENG-77",
			"state": {
				"name": "Done",
				"type": "completed"
			}
		}
	}`)

	// 1. Missing header -> 401 Unauthorized
	req, _ := http.NewRequest(http.MethodPost, "/linear", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	ctrl.HandleLinearWebhook(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	// 2. Invalid signature -> 401 Unauthorized
	reqBadSig, _ := http.NewRequest(http.MethodPost, "/linear", bytes.NewReader(payload))
	reqBadSig.Header.Set("Linear-Signature", "deadbeefbadcafe")
	rr2 := httptest.NewRecorder()
	ctrl.HandleLinearWebhook(rr2, reqBadSig)
	assert.Equal(t, http.StatusUnauthorized, rr2.Code)

	// 3. Valid HMAC-SHA256 signature -> 200 OK
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSig := hex.EncodeToString(mac.Sum(nil))

	reqValid, _ := http.NewRequest(http.MethodPost, "/linear", bytes.NewReader(payload))
	reqValid.Header.Set("Linear-Signature", validSig)
	rr3 := httptest.NewRecorder()
	ctrl.HandleLinearWebhook(rr3, reqValid)
	assert.Equal(t, http.StatusOK, rr3.Code)

	var resp map[string]any
	err := json.Unmarshal(rr3.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "processed", resp["status"])
	assert.Equal(t, "linear", resp["platform"])
	assert.Equal(t, "ENG-77", resp["ticket_key"])
	assert.Equal(t, "RESOLVED", resp["target_status"])
}

func TestPMWebhooksController_Linear_Canceled(t *testing.T) {
	ctrl := controllers.NewPMWebhooksController(nil, "", "")

	payload := []byte(`{
		"action": "update",
		"type": "Issue",
		"data": {
			"identifier": "ENG-99",
			"state": {
				"name": "Canceled",
				"type": "canceled"
			}
		}
	}`)

	req, _ := http.NewRequest(http.MethodPost, "/linear", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	ctrl.HandleLinearWebhook(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp map[string]any
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "ENG-99", resp["ticket_key"])
	assert.Equal(t, "DISMISSED", resp["target_status"])
}

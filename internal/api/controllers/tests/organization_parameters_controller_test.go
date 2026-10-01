// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/llm/byok"
	orgparamusecases "github.com/scandrix/backend/internal/organization/application/usecases/organizationparameters"
	"github.com/scandrix/backend/pkg/models"
)

type mockOrgParamRepo struct {
	params     map[string]*models.OrganizationParameter
	workspaces []models.Workspace
}

func newMockOrgParamRepo(wsID uuid.UUID) *mockOrgParamRepo {
	return &mockOrgParamRepo{
		params: make(map[string]*models.OrganizationParameter),
		workspaces: []models.Workspace{
			{ID: wsID, Name: "Acme Corp"},
		},
	}
}

func (m *mockOrgParamRepo) ListWorkspacesForUser(ctx context.Context, email string) ([]models.Workspace, error) {
	return m.workspaces, nil
}

func (m *mockOrgParamRepo) GetOrganizationParameter(ctx context.Context, wsID uuid.UUID, key string) (*models.OrganizationParameter, error) {
	if p, ok := m.params[key]; ok {
		return p, nil
	}
	return nil, nil
}

func (m *mockOrgParamRepo) SetOrganizationParameter(ctx context.Context, wsID uuid.UUID, key string, val []byte, desc string) error {
	m.params[key] = &models.OrganizationParameter{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		ConfigKey:   key,
		ConfigValue: val,
		Description: desc,
		IsActive:    true,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	return nil
}

func (m *mockOrgParamRepo) DeleteOrganizationParameter(ctx context.Context, wsID uuid.UUID, key string) error {
	delete(m.params, key)
	return nil
}

func (m *mockOrgParamRepo) ListOrganizationParameters(ctx context.Context, wsID uuid.UUID) ([]models.OrganizationParameter, error) {
	var list []models.OrganizationParameter
	for _, p := range m.params {
		list = append(list, *p)
	}
	return list, nil
}

func TestOrgParams_FindByKey_DefaultsAndMasking(t *testing.T) {
	wsID := uuid.New()
	repo := newMockOrgParamRepo(wsID)
	ctrl := controllers.NewOrganizationParametersController(repo)
	routes := ctrl.Routes()

	// 1. Missing key -> 400
	req := httptest.NewRequest(http.MethodGet, "/find-by-key", nil)
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing key, got %d", w.Code)
	}

	// 2. Query empty byok_config -> returns default template
	req = httptest.NewRequest(http.MethodGet, "/find-by-key?key=byok_config", nil)
	req = req.WithContext(auth.WithWorkspaceContext(req.Context(), wsID))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for byok_config, got %d", w.Code)
	}

	var resp struct {
		StatusCode int `json:"statusCode"`
		Data       struct {
			ConfigKey   string          `json:"configKey"`
			ConfigValue json.RawMessage `json:"configValue"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed decoding response: %v", err)
	}
	if resp.Data.ConfigKey != "byok_config" {
		t.Fatalf("expected configKey byok_config, got %s", resp.Data.ConfigKey)
	}

	// 3. Seed real BYOK config with plaintext secret in DB and verify it is masked
	secretKey := "sk-ant-api03-verylongsecretkeyhere-AA"
	rawConfig := fmt.Sprintf(`{
		"version": 2,
		"credentials": [
			{"id": "cred-1", "provider": "anthropic", "apiKey": "%s"}
		],
		"models": [
			{"id": "model-1", "credentialId": "cred-1", "model": "claude-3-7-sonnet"}
		],
		"routing": {"defaultModelId": "model-1"}
	}`, secretKey)

	_ = repo.SetOrganizationParameter(context.Background(), wsID, "byok_config", []byte(rawConfig), "Production BYOK")

	req = httptest.NewRequest(http.MethodGet, "/find-by-key?key=byok_config", nil)
	req = req.WithContext(auth.WithWorkspaceContext(req.Context(), wsID))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	bodyStr := w.Body.String()
	if bytes.Contains(w.Body.Bytes(), []byte(secretKey)) {
		t.Fatalf("CRITICAL SECURITY LEAK: Plaintext API key was returned in response: %s", bodyStr)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("sk...-AA")) {
		t.Fatalf("Expected masked key format sk...-AA, got response: %s", bodyStr)
	}
}

func TestOrgParams_CreateOrUpdate_ReferentialIntegrity(t *testing.T) {
	wsID := uuid.New()
	repo := newMockOrgParamRepo(wsID)
	ctrl := controllers.NewOrganizationParametersController(repo)
	routes := ctrl.Routes()

	// 1. Submit BYOKConfig with a dangling credential reference -> must reject with 400
	invalidPayload := map[string]any{
		"key": "byok_config",
		"configValue": map[string]any{
			"version": 2,
			"credentials": []map[string]any{
				{"id": "cred-valid", "provider": "openai", "apiKey": "sk-real-key-12345"},
			},
			"models": []map[string]any{
				{"id": "model-1", "credentialId": "cred-ghost-missing", "model": "gpt-4o"},
			},
			"routing": map[string]any{
				"defaultModelId": "model-1",
			},
		},
	}
	b, _ := json.Marshal(invalidPayload)
	req := httptest.NewRequest(http.MethodPost, "/create-or-update", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithWorkspaceContext(req.Context(), wsID))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for dangling credential reference, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Submit valid BYOKConfig -> must succeed and encrypt key in repo
	validPayload := map[string]any{
		"key": "byok_config",
		"configValue": map[string]any{
			"version": 2,
			"credentials": []map[string]any{
				{"id": "cred-1", "provider": "openai", "apiKey": "sk-proj-supersecret123"},
			},
			"models": []map[string]any{
				{"id": "model-1", "credentialId": "cred-1", "model": "gpt-4o"},
			},
			"routing": map[string]any{
				"defaultModelId": "model-1",
			},
		},
	}
	bValid, _ := json.Marshal(validPayload)
	reqValid := httptest.NewRequest(http.MethodPost, "/create-or-update", bytes.NewReader(bValid))
	reqValid.Header.Set("Content-Type", "application/json")
	reqValid = reqValid.WithContext(auth.WithWorkspaceContext(reqValid.Context(), wsID))
	wValid := httptest.NewRecorder()
	routes.ServeHTTP(wValid, reqValid)

	if wValid.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid config, got %d: %s", wValid.Code, wValid.Body.String())
	}

	stored, _ := repo.GetOrganizationParameter(context.Background(), wsID, "byok_config")
	if stored == nil {
		t.Fatalf("Expected byok_config to be saved in repository")
	}

	var storedCfg byok.BYOKConfig
	_ = json.Unmarshal(stored.ConfigValue, &storedCfg)
	if storedCfg.Credentials[0].APIKey == "sk-proj-supersecret123" {
		t.Fatalf("Expected stored API key to be encrypted, but found plaintext in DB")
	}
}

func TestOrgParams_DeleteBYOK_ReferentialIntegrityGuard(t *testing.T) {
	wsID := uuid.New()
	repo := newMockOrgParamRepo(wsID)
	ctrl := controllers.NewOrganizationParametersController(repo)
	routes := ctrl.Routes()

	// Seed multi-model config where model-1 is default routing
	cfg := byok.BYOKConfig{
		Version: 2,
		Credentials: []byok.BYOKCredential{
			{ID: "c1", Provider: "openai", APIKey: "enc-key"},
		},
		Models: []byok.BYOKModelConfig{
			{ID: "model-1", CredentialID: "c1", Model: "gpt-4o"},
			{ID: "model-2", CredentialID: "c1", Model: "gpt-4o-mini"},
		},
		Routing: byok.BYOKRouting{
			DefaultModelID: "model-1",
		},
	}
	cfgBytes, _ := json.Marshal(cfg)
	_ = repo.SetOrganizationParameter(context.Background(), wsID, "byok_config", cfgBytes, "")

	// 1. Attempt to delete model-1 which is referenced by DefaultModelID -> must fail 400
	delReq := httptest.NewRequest(http.MethodDelete, "/delete-byok-config?modelId=model-1", nil)
	delReq = delReq.WithContext(auth.WithWorkspaceContext(delReq.Context(), wsID))
	wDel := httptest.NewRecorder()
	routes.ServeHTTP(wDel, delReq)

	if wDel.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request when deleting routed model, got %d: %s", wDel.Code, wDel.Body.String())
	}

	// 2. Delete model-2 which is unreferenced -> must succeed
	delReq2 := httptest.NewRequest(http.MethodDelete, "/delete-byok-config?modelId=model-2", nil)
	delReq2 = delReq2.WithContext(auth.WithWorkspaceContext(delReq2.Context(), wsID))
	wDel2 := httptest.NewRecorder()
	routes.ServeHTTP(wDel2, delReq2)

	if wDel2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK when deleting unreferenced model, got %d: %s", wDel2.Code, wDel2.Body.String())
	}

	// 3. Now delete model-1 (now the last remaining model) -> triggers full BYOK teardown
	delReq3 := httptest.NewRequest(http.MethodDelete, "/delete-byok-config?modelId=model-1", nil)
	delReq3 = delReq3.WithContext(auth.WithWorkspaceContext(delReq3.Context(), wsID))
	wDel3 := httptest.NewRecorder()
	routes.ServeHTTP(wDel3, delReq3)

	if wDel3.Code != http.StatusOK {
		t.Fatalf("expected 200 OK when tearing down last model, got %d: %s", wDel3.Code, wDel3.Body.String())
	}

	// Verify param was deleted from DB
	param, _ := repo.GetOrganizationParameter(context.Background(), wsID, "byok_config")
	if param != nil {
		t.Fatalf("Expected byok_config parameter to be completely deleted after last model removal")
	}
}

func TestOrgParams_TestBYOK_Validation(t *testing.T) {
	wsID := uuid.New()
	ctrl := controllers.NewOrganizationParametersController(nil)
	routes := ctrl.Routes()

	// 1. Unknown provider -> 400
	reqBody := map[string]any{
		"provider": "non_existent_provider_xyz",
		"model":    "unknown-model",
	}
	b, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/test-byok", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown provider, got %d", w.Code)
	}

	// 2. With no probe wired the endpoint must refuse rather than report a
	//    fabricated success.
	reqValidBody := map[string]any{
		"provider": "openai", "model": "gpt-4o", "temperature": 0.3,
	}
	bValid, _ := json.Marshal(reqValidBody)
	reqValid := httptest.NewRequest(http.MethodPost, "/test-byok", bytes.NewReader(bValid))
	reqValid.Header.Set("Content-Type", "application/json")
	wValid := httptest.NewRecorder()
	routes.ServeHTTP(wValid, reqValid)

	if wValid.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when no probe is wired, got %d: %s", wValid.Code, wValid.Body.String())
	}
	_ = wsID
}

// stubRoundTripper answers provider requests without opening a socket, so the
// probe path is exercised end to end while the SSRF guard still runs for real.
type stubRoundTripper struct {
	status int
}

func (s stubRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: s.status,
		Body:       io.NopCloser(strings.NewReader(`{"error":"upstream"}`)),
		Header:     make(http.Header),
		Request:    r,
	}, nil
}

// TestOrgParams_TestBYOK_PerformsRealProbe asserts the endpoint reflects an
// actual provider response. It previously answered success with a fixed 85 ms
// latency without ever using the API key, so an invalid credential looked valid.
func TestOrgParams_TestBYOK_PerformsRealProbe(t *testing.T) {
	cases := []struct {
		name           string
		providerStatus int
		wantStatus     int
		wantOK         bool
		wantCode       string
	}{
		{"provider accepts the key", http.StatusOK, http.StatusOK, true, "ok"},
		{"provider rejects the key", http.StatusUnauthorized, http.StatusBadRequest, false, "auth"},
		{"model not found", http.StatusNotFound, http.StatusBadRequest, false, "not_found"},
		{"rate limited", http.StatusTooManyRequests, http.StatusBadRequest, false, "rate_limit"},
		{"provider is down", http.StatusBadGateway, http.StatusBadGateway, false, "server_error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uc := orgparamusecases.NewTestBYOKModelUseCase().WithHTTPClient(
				&http.Client{Transport: stubRoundTripper{status: tc.providerStatus}})

			ctrl := controllers.NewOrganizationParametersController(nil).
				WithUseCases(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, uc)
			routes := ctrl.Routes()

			body, _ := json.Marshal(map[string]any{
				"provider": "openai",
				"apiKey":   "sk-test-not-a-real-key",
				// A public-looking host that does not resolve, so the SSRF guard
				// allows it and the stub transport answers.
				"baseURL": "https://provider.invalid/v1",
				"model":   "gpt-4o",
			})
			req := httptest.NewRequest(http.MethodPost, "/test-byok", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			routes.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected %d, got %d: %s", tc.wantStatus, rec.Code, rec.Body.String())
			}

			var res struct {
				Data struct {
					OK         bool   `json:"ok"`
					Code       string `json:"code"`
					HTTPStatus int    `json:"httpStatus"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
				t.Fatalf("failed decoding: %v", err)
			}
			if res.Data.OK != tc.wantOK {
				t.Fatalf("expected ok=%v, got %v", tc.wantOK, res.Data.OK)
			}
			if res.Data.Code != tc.wantCode {
				t.Fatalf("expected code=%q, got %q", tc.wantCode, res.Data.Code)
			}
			if res.Data.HTTPStatus != tc.providerStatus {
				t.Fatalf("expected the real provider status %d, got %d",
					tc.providerStatus, res.Data.HTTPStatus)
			}
		})
	}
}

// TestOrgParams_TestBYOK_RejectsPrivateEndpoints asserts the SSRF guard is
// enforced through the endpoint: a base URL pointing at the loopback interface
// must be refused rather than probed.
func TestOrgParams_TestBYOK_RejectsPrivateEndpoints(t *testing.T) {
	uc := orgparamusecases.NewTestBYOKModelUseCase().WithHTTPClient(
		&http.Client{Transport: stubRoundTripper{status: http.StatusOK}})

	ctrl := controllers.NewOrganizationParametersController(nil).
		WithUseCases(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, uc)
	routes := ctrl.Routes()

	for _, base := range []string{
		"http://127.0.0.1:8080",
		"http://localhost:9090",
		"http://169.254.169.254/latest/meta-data",
	} {
		body, _ := json.Marshal(map[string]any{
			"provider": "openai", "apiKey": "sk-x", "baseURL": base, "model": "gpt-4o",
		})
		req := httptest.NewRequest(http.MethodPost, "/test-byok", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("base %s: expected 400, got %d: %s", base, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), `"ok":true`) {
			t.Fatalf("base %s: a blocked endpoint must never report success", base)
		}
	}
}

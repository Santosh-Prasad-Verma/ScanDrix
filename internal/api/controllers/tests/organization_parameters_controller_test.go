// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/llm/byok"
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

func (m *mockOrgParamRepo) ListWorkspaces(ctx context.Context) ([]models.Workspace, error) {
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

	// 2. Valid provider and tuning -> 200
	reqValidBody := map[string]any{
		"provider":    "openai",
		"model":       "gpt-4o",
		"temperature": 0.3,
	}
	bValid, _ := json.Marshal(reqValidBody)
	reqValid := httptest.NewRequest(http.MethodPost, "/test-byok", bytes.NewReader(bValid))
	reqValid.Header.Set("Content-Type", "application/json")
	wValid := httptest.NewRecorder()
	routes.ServeHTTP(wValid, reqValid)

	if wValid.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid test-byok, got %d: %s", wValid.Code, wValid.Body.String())
	}
	_ = wsID
}

func TestOrgParams_ListProvidersAndModels(t *testing.T) {
	ctrl := controllers.NewOrganizationParametersController(nil)
	routes := ctrl.Routes()

	// 1. GET /list-providers
	req := httptest.NewRequest(http.MethodGet, "/list-providers", nil)
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /list-providers, got %d", w.Code)
	}

	var provResp struct {
		StatusCode int `json:"statusCode"`
		Data       []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&provResp); err != nil || len(provResp.Data) == 0 {
		t.Fatalf("failed reading providers: %v (len: %d)", err, len(provResp.Data))
	}

	foundOpenAI := false
	for _, p := range provResp.Data {
		if p.ID == "openai" {
			foundOpenAI = true
			break
		}
	}
	if !foundOpenAI {
		t.Fatalf("Expected openai to be registered in provider list")
	}

	// 2. GET /list-models?provider=openai
	reqModels := httptest.NewRequest(http.MethodGet, "/list-models?provider=openai", nil)
	wModels := httptest.NewRecorder()
	routes.ServeHTTP(wModels, reqModels)
	if wModels.Code != http.StatusOK {
		t.Fatalf("expected 200 for /list-models, got %d", wModels.Code)
	}

	// 3. GET /model-capabilities?provider=openai&model=gpt-4o
	reqCaps := httptest.NewRequest(http.MethodGet, "/model-capabilities?provider=openai&model=gpt-4o", nil)
	wCaps := httptest.NewRecorder()
	routes.ServeHTTP(wCaps, reqCaps)
	if wCaps.Code != http.StatusOK {
		t.Fatalf("expected 200 for /model-capabilities, got %d", wCaps.Code)
	}
}

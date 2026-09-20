package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	centdomain "github.com/scandrix/backend/internal/centralizedconfig/domain"
	centinfra "github.com/scandrix/backend/internal/centralizedconfig/infrastructure"
)

func setupCentralizedConfigControllerTest() (*controllers.ParametersController, *centinfra.MemoryTreeProvider, *centinfra.DefaultConfigStorage) {
	storage := centinfra.NewDefaultConfigStorage(nil)
	tree := centinfra.NewMemoryTreeProvider()
	prSvc := centinfra.NewPRService(nil)
	svc := centinfra.NewService(prSvc, tree, storage)

	ctrl := controllers.NewParametersController(nil).
		WithCentralizedConfig(svc, prSvc)

	return ctrl, tree, storage
}

func TestCentralizedConfigRoutes_FullLifecycle(t *testing.T) {
	ctrl, tree, storage := setupCentralizedConfigControllerTest()
	router := ctrl.CentralizedConfigRoutes()

	wsID := uuid.New()
	ctx := auth.WithWorkspaceContext(context.Background(), wsID)

	// Seed repository tree with a valid .scandrix/config.yml and rule
	tree.SetTree(wsID.String(), "", "repo-central", []centdomain.TreeItem{
		{
			Path: ".scandrix/config.yml",
			Type: "blob",
		},
		{
			Path: ".scandrix/rules/security.yml",
			Type: "blob",
		},
	})
	tree.SetFileContent(wsID.String(), "", "repo-central", ".scandrix/config.yml", []byte(`
ignorePaths:
  - "vendor/**"
reviewOptions:
  security: true
  performance: true
`))
	tree.SetFileContent(wsID.String(), "", "repo-central", ".scandrix/rules/security.yml", []byte(`
title: "No Plaintext Passwords"
severity: "critical"
enabled: true
rule: "Ensure no hardcoded passwords exist"
`))

	// Configure initial code review parameter with central repo
	_ = storage.SaveCodeReviewParameter(ctx, wsID.String(), "", "repo-central", nil, map[string]any{
		"enabled": true,
		"repository": map[string]any{
			"id":   "repo-central",
			"name": "central-governance-repo",
		},
	})

	// 1. GET /status -> returns active rules and status
	reqStatus, _ := http.NewRequestWithContext(ctx, "GET", "/status", nil)
	recStatus := httptest.NewRecorder()
	router.ServeHTTP(recStatus, reqStatus)

	if recStatus.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for status, got %d: %s", recStatus.Code, recStatus.Body.String())
	}

	var statusResp map[string]any
	if err := json.Unmarshal(recStatus.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("failed decoding status response: %v", err)
	}
	if statusResp["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", statusResp["enabled"])
	}

	// 2. POST /init -> initializes central repo
	initBody := bytes.NewBufferString(`{
		"repository_id": "repo-central",
		"default_branch": "main"
	}`)
	reqInit, _ := http.NewRequestWithContext(ctx, "POST", "/init", initBody)
	recInit := httptest.NewRecorder()
	router.ServeHTTP(recInit, reqInit)

	if recInit.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for init, got %d: %s", recInit.Code, recInit.Body.String())
	}

	// 3. POST /sync -> synchronizes repository rules
	syncBody := bytes.NewBufferString(`{
		"repository_id": "repo-central"
	}`)
	reqSync, _ := http.NewRequestWithContext(ctx, "POST", "/sync", syncBody)
	recSync := httptest.NewRecorder()
	router.ServeHTTP(recSync, reqSync)

	if recSync.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for sync, got %d: %s", recSync.Code, recSync.Body.String())
	}

	// 4. GET /download -> downloads configuration files list
	reqDownload, _ := http.NewRequestWithContext(ctx, "GET", "/download", nil)
	recDownload := httptest.NewRecorder()
	router.ServeHTTP(recDownload, reqDownload)

	if recDownload.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for download, got %d: %s", recDownload.Code, recDownload.Body.String())
	}

	// 5. GET /download?zip=true -> streams zip archive
	reqZip, _ := http.NewRequestWithContext(ctx, "GET", "/download?zip=true", nil)
	recZip := httptest.NewRecorder()
	router.ServeHTTP(recZip, reqZip)

	if recZip.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for zip download, got %d: %s", recZip.Code, recZip.Body.String())
	}
	if ct := recZip.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("expected Content-Type application/zip, got %s", ct)
	}

	// 6. POST /disable -> disables and clears active PR tracking
	reqDisable, _ := http.NewRequestWithContext(ctx, "POST", "/disable", nil)
	recDisable := httptest.NewRecorder()
	router.ServeHTTP(recDisable, reqDisable)

	if recDisable.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for disable, got %d: %s", recDisable.Code, recDisable.Body.String())
	}
}

// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: cli_drixy_rules_controller_test.go
// ═══════════════════════════════════════════════════════════════

package controllers_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/rules/drixy/application/usecases"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/repositories"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
	"github.com/scandrix/backend/pkg/models"
)

type mockTeamCLIKeyRepo struct {
	keys map[string]*models.TeamCLIKey
}

func (m *mockTeamCLIKeyRepo) GetAPIKeyByHash(ctx context.Context, keyHash string) (*models.TeamCLIKey, error) {
	if k, ok := m.keys[keyHash]; ok {
		return k, nil
	}
	return nil, nil
}

func TestCliDrixyRulesController(t *testing.T) {
	orgID := uuid.New().String()
	teamID := uuid.New()
	wsID := uuid.MustParse(orgID)

	plainKey := "scandrix_test_cli_key_12345"
	h := sha256.New()
	h.Write([]byte(plainKey))
	hashStr := hex.EncodeToString(h.Sum(nil))

	configJSON, _ := json.Marshal(map[string]any{
		"capabilities": []string{controllers.CapabilityDrixyRulesManage},
	})

	keyRepo := &mockTeamCLIKeyRepo{
		keys: map[string]*models.TeamCLIKey{
			hashStr: {
				ID:          uuid.New(),
				WorkspaceID: &wsID,
				TeamID:      &teamID,
				Name:        "Test Key",
				Active:      true,
				Config:      configJSON,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		},
	}

	memRepo := repositories.NewPostgresDrixyRulesRepository(nil)
	ruleLikeSvc := services.NewRuleLikeService(repositories.NewPostgresRuleLikeRepository(nil))
	drixySvc := services.NewDrixyRulesService(memRepo, ruleLikeSvc)

	createUC := usecases.NewCreateOrUpdateDrixyRuleUseCase(drixySvc, nil, nil, nil)
	findUC := usecases.NewFindRulesInOrganizationByFilterDrixyRulesUseCase(drixySvc, nil)

	cliCtrl := controllers.NewCliDrixyRulesController(keyRepo, createUC, findUC)
	r := chi.NewRouter()
	r.Mount("/cli/drixy-rules", cliCtrl.Routes())

	t.Run("Create Rule via X-Team-Key", func(t *testing.T) {
		body := map[string]any{
			"title":        "No eval() in production",
			"rule":         "Avoid using eval() because it introduces code injection vulnerabilities.",
			"repositoryId": "repo-123",
			"severity":     "HIGH",
			"scope":        "file",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/cli/drixy-rules", bytes.NewReader(bodyBytes))
		req.Header.Set("x-team-key", plainKey)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}

		var created interfaces.DrixyRule
		if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
			t.Fatalf("failed decoding created rule: %v", err)
		}
		if created.Title != "No eval() in production" {
			t.Errorf("expected title to match, got %s", created.Title)
		}
		if created.UUID == "" {
			t.Errorf("expected generated UUID, got empty")
		}

		// Now List Rules
		listReq := httptest.NewRequest(http.MethodGet, "/cli/drixy-rules?repositoryId=repo-123", nil)
		listReq.Header.Set("x-team-key", plainKey)
		listW := httptest.NewRecorder()

		r.ServeHTTP(listW, listReq)
		if listW.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", listW.Code, listW.Body.String())
		}

		var listResp []*interfaces.DrixyRules
		_ = json.NewDecoder(listW.Body).Decode(&listResp)
		if len(listResp) == 0 {
			t.Fatalf("expected at least 1 rule collection")
		}

		// Now Update Rule
		updateBody := map[string]any{
			"uuid":         created.UUID,
			"title":        "Updated: No eval() anywhere",
			"rule":         "Strictly avoid eval().",
			"repositoryId": "repo-123",
			"severity":     "CRITICAL",
			"scope":        "file",
		}
		upBytes, _ := json.Marshal(updateBody)
		upReq := httptest.NewRequest(http.MethodPatch, "/cli/drixy-rules/"+created.UUID, bytes.NewReader(upBytes))
		upReq.Header.Set("x-team-key", plainKey)
		upW := httptest.NewRecorder()

		r.ServeHTTP(upW, upReq)
		if upW.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", upW.Code, upW.Body.String())
		}
	})

	t.Run("Create Rule forbids user-supplied UUID", func(t *testing.T) {
		body := map[string]any{
			"uuid":         "fake-client-uuid",
			"title":        "Invalid rule",
			"rule":         "Invalid",
			"repositoryId": "repo-123",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/cli/drixy-rules", bytes.NewReader(bodyBytes))
		req.Header.Set("x-team-key", plainKey)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d", w.Code)
		}
	})

	t.Run("Unauthorized when key is missing or invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/cli/drixy-rules", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", w.Code)
		}
	})
}

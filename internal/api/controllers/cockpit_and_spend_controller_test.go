package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
)

type mockCockpitRepo struct{}

func (m *mockCockpitRepo) CountActiveReviews(ctx context.Context, wsID uuid.UUID) (int, error) {
	return 3, nil
}

func (m *mockCockpitRepo) ListTrackedRepositories(ctx context.Context, wsID uuid.UUID) ([]models.TrackedRepository, error) {
	return []models.TrackedRepository{
		{ID: uuid.New(), NamespacePath: "acme/api", IsActive: true},
	}, nil
}

type mockSpendRepo struct {
	limit float64
}

func (m *mockSpendRepo) GetSpendLimitStatus(ctx context.Context, wsID uuid.UUID) (*models.SpendLimitEvaluation, error) {
	return &models.SpendLimitEvaluation{
		SpentUSD:            45.50,
		LimitUSD:            m.limit,
		PercentageUsed:      18.2,
		IsOverLimit:         false,
		MonthlyBudgetUSD:    m.limit,
		AlertThresholdPct:   85.0,
		NotificationEnabled: true,
	}, nil
}

func (m *mockSpendRepo) GetWorkspaceUsage(ctx context.Context, wsID uuid.UUID, since time.Time) (int64, int64, float64, error) {
	return 100000, 25000, 45.50, nil
}

func (m *mockSpendRepo) UpdateSpendLimit(ctx context.Context, wsID uuid.UUID, limitUSD float64) error {
	m.limit = limitUSD
	return nil
}

type mockMessagesRepo struct {
	settings *models.PullRequestMessageSettings
}

func (m *mockMessagesRepo) GetPullRequestMessages(ctx context.Context, wsID uuid.UUID, repoID *uuid.UUID, directoryID string) (*models.PullRequestMessageSettings, error) {
	if m.settings != nil {
		return m.settings, nil
	}
	return &models.PullRequestMessageSettings{
		WorkspaceID:  wsID,
		ConfigLevel:  "global",
		StartReviewMessage: models.MessageContentWithStatus{
			Content: "Custom ScanDrix started review...",
			Status:  models.PRMessageStatusActive,
		},
		HideComments:         false,
		SuggestionCopyPrompt: true,
	}, nil
}

func (m *mockMessagesRepo) SavePullRequestMessages(ctx context.Context, wsID uuid.UUID, settings *models.PullRequestMessageSettings) error {
	m.settings = settings
	return nil
}

func TestCockpitController(t *testing.T) {
	wsID := uuid.New()
	ctrl := controllers.NewCockpitController(&mockCockpitRepo{})
	r := chi.NewRouter()

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})

	r.Mount("/cockpit", ctrl.Routes())
	r.Mount("/code-health", ctrl.CodeHealthRoutes())
	r.Mount("/productivity", ctrl.ProductivityRoutes())

	// 1. /cockpit/health
	req := httptest.NewRequest("GET", "/cockpit/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "healthy")

	// 2. /cockpit/overview
	req = httptest.NewRequest("GET", "/cockpit/overview", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "activeRepositories")

	// 3. /code-health/overview
	req = httptest.NewRequest("GET", "/code-health/overview", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "healthScore")

	// 4. /productivity/overview
	req = httptest.NewRequest("GET", "/productivity/overview", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "cycleTimeHours")
}

func TestSpendLimitController(t *testing.T) {
	wsID := uuid.New()
	spendRepo := &mockSpendRepo{limit: 250.0}
	ctrl := controllers.NewSpendLimitController(spendRepo)
	r := chi.NewRouter()

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Mount("/spend-limit", ctrl.Routes())

	// 1. GET /spend-limit/status
	req := httptest.NewRequest("GET", "/spend-limit/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "spent_usd")

	// 2. GET /spend-limit
	req = httptest.NewRequest("GET", "/spend-limit", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "modelPrices")

	// 3. POST /spend-limit
	payload := []byte(`{"limitUsd": 500.0, "alertThresholdPct": 80.0, "notificationEnabled": true}`)
	req = httptest.NewRequest("POST", "/spend-limit", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "500")
}

func TestPullRequestMessagesController(t *testing.T) {
	wsID := uuid.New()
	msgRepo := &mockMessagesRepo{}
	ctrl := controllers.NewPullRequestMessagesController(msgRepo)
	r := chi.NewRouter()

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Mount("/pull-request-messages", ctrl.Routes())

	// 1. GET /pull-request-messages
	req := httptest.NewRequest("GET", "/pull-request-messages", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Custom ScanDrix started review...")

	// 2. POST /pull-request-messages
	body := map[string]any{
		"configLevel": "global",
		"startReviewMessage": map[string]any{
			"content": "Updated review message",
			"status":  "ACTIVE",
		},
	}
	bodyBytes, _ := json.Marshal(body)
	req = httptest.NewRequest("POST", "/pull-request-messages", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Updated review message")
}

func TestSystemController(t *testing.T) {
	ctrl := controllers.NewSystemController()
	r := chi.NewRouter()
	r.Mount("/system", ctrl.Routes())

	req := httptest.NewRequest("GET", "/system/info", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "scandrix-api")

	req = httptest.NewRequest("GET", "/system/version", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "2.5.0")
}

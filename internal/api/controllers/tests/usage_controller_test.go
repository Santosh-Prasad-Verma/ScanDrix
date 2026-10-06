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
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

func TestUsageControllerEndpoints(t *testing.T) {
	ctrl := controllers.NewUsageController(nil)
	wsID := uuid.New()

	router := ctrl.Routes()

	// 1. Test Get Usage
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for get usage, got: %d", rec.Code)
	}

	var usageResp dtos.TokenUsageResponse
	if err := json.NewDecoder(rec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("failed decoding usage response: %v", err)
	}

	// 2. Test Get Live Quota
	req = httptest.NewRequest(http.MethodGet, "/quota", nil)
	req = req.WithContext(ctx)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for get quota, got: %d", rec.Code)
	}

	var quotaResp dtos.LiveQuotaResponse
	if err := json.NewDecoder(rec.Body).Decode(&quotaResp); err != nil {
		t.Fatalf("failed decoding live quota response: %v", err)
	}
	if quotaResp.MonthlyTokenLimit != 500_000 {
		t.Fatalf("expected 500k default monthly token limit, got: %d", quotaResp.MonthlyTokenLimit)
	}

	// 3. Test Get Usage History
	req = httptest.NewRequest(http.MethodGet, "/history?days=14", nil)
	req = req.WithContext(ctx)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for usage history, got: %d", rec.Code)
	}

	// 4. Test Update Spend Limit
	body, _ := json.Marshal(dtos.UpdateSpendLimitRequest{
		MonthlySpendLimitUSD: 100.0,
	})
	req = httptest.NewRequest(http.MethodPut, "/spend-limit", bytes.NewReader(body))
	// Raising a spend limit is owner/admin only (auth.RoleGuard); the shared ctx
	// above carries no role, so attach an owner for this privileged call.
	spendCtx := auth.WithAccountContext(ctx, &models.AccountProfile{WorkspaceID: wsID, Role: models.RoleOwner})
	req = req.WithContext(spendCtx)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for update spend limit, got: %d", rec.Code)
	}
}

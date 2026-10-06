package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/analytics/pricing"
	analyticsRepo "github.com/scandrix/backend/internal/analytics/repository"
	"github.com/scandrix/backend/internal/analytics/spendlimit"
	"github.com/scandrix/backend/internal/analytics/usage"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestAnalytics() (*UsageController, *SpendLimitController, uuid.UUID) {
	wsID := uuid.New()
	cat := pricing.NewTokenPricingCatalog(pricing.WithOnlineFetching(false))
	resolver := pricing.NewPricingResolver(cat)
	calc := pricing.NewModelCostCalculator(resolver)

	tokenRepo := analyticsRepo.NewTokenUsageRepository(resolver)
	tokenSvc := analyticsRepo.NewTokenUsageService(tokenRepo)

	summaryUC := usage.NewBuildUsageSummaryUseCase(tokenSvc, resolver, nil)
	estimateUC := usage.NewCostEstimateUseCase(tokenSvc, nil, calc)
	devUC := usage.NewTokensByDeveloperUseCase(tokenSvc, nil, nil)
	monthlyUC := usage.NewMonthlySpendUseCase(tokenSvc, calc)

	cfgSvc := spendlimit.NewSpendLimitConfigService(nil, monthlyUC, resolver)
	configureUC := spendlimit.NewConfigureSpendLimitUseCase(cfgSvc)
	getConfigUC := spendlimit.NewGetSpendLimitConfigUseCase(cfgSvc, nil, nil, resolver)

	usageCtrl := NewUsageController(nil).WithAnalyticsServices(
		tokenSvc,
		summaryUC,
		estimateUC,
		devUC,
		cat,
		cfgSvc,
	)

	spendCtrl := NewSpendLimitController(nil).WithSpendLimitServices(
		cfgSvc,
		configureUC,
		getConfigUC,
	)

	return usageCtrl, spendCtrl, wsID
}

// withWorkspaceContext attaches a workspace and an owner profile. Spend-limit
// and other privileged routes are guarded by auth.RoleGuard, which denies any
// request without a role, so tests that drive those routes need one.
func withWorkspaceContext(r *http.Request, wsID uuid.UUID) *http.Request {
	ctx := context.WithValue(r.Context(), auth.WorkspaceContextKey, wsID)
	ctx = auth.WithAccountContext(ctx, &models.AccountProfile{WorkspaceID: wsID, Role: models.RoleOwner})
	return r.WithContext(ctx)
}

func TestUsageController_TokenAnalyticsEndpoints(t *testing.T) {
	usageCtrl, _, wsID := setupTestAnalytics()
	router := usageCtrl.Routes()

	// 1. GET /tokens/summary
	req := httptest.NewRequest(http.MethodGet, "/tokens/summary?organizationId="+wsID.String(), nil)
	req = withWorkspaceContext(req, wsID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var summary usage.UsageSummaryReportContract
	err := json.Unmarshal(rec.Body.Bytes(), &summary)
	require.NoError(t, err)

	// 2. GET /tokens/overview
	req = httptest.NewRequest(http.MethodGet, "/tokens/overview?organizationId="+wsID.String(), nil)
	req = withWorkspaceContext(req, wsID)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var overview usage.UsageOverviewReportContract
	err = json.Unmarshal(rec.Body.Bytes(), &overview)
	require.NoError(t, err)

	// 3. GET /tokens/estimate
	req = httptest.NewRequest(http.MethodGet, "/tokens/estimate?organizationId="+wsID.String(), nil)
	req = withWorkspaceContext(req, wsID)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var estimate usage.CostEstimateContract
	err = json.Unmarshal(rec.Body.Bytes(), &estimate)
	require.NoError(t, err)
	assert.Equal(t, 14, estimate.PeriodDays)
	assert.Equal(t, 30, estimate.ProjectionDays)

	// 4. GET /tokens/developer
	req = httptest.NewRequest(http.MethodGet, "/tokens/developer?organizationId="+wsID.String(), nil)
	req = withWorkspaceContext(req, wsID)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// 5. GET /tokens/pricing
	req = httptest.NewRequest(http.MethodGet, "/tokens/pricing?model=claude-3-5-sonnet", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var priceInfo pricing.ModelPricingInfo
	err = json.Unmarshal(rec.Body.Bytes(), &priceInfo)
	require.NoError(t, err)
	assert.Equal(t, "claude-3-5-sonnet", priceInfo.ID)
	assert.True(t, priceInfo.Pricing.Input.Default > 0)
}

func TestSpendLimitController_Endpoints(t *testing.T) {
	_, spendCtrl, wsID := setupTestAnalytics()
	router := spendCtrl.Routes()

	// 1. GET /status (nil when no limit enabled)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req = withWorkspaceContext(req, wsID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. GET /
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req = withWorkspaceContext(req, wsID)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var view spendlimit.SpendLimitConfigView
	err := json.Unmarshal(rec.Body.Bytes(), &view)
	require.NoError(t, err)
	assert.False(t, view.Enabled)

	// 3. POST / (fail on negative limit)
	invalidBody := []byte(`{"enabled":true,"monthlyLimitUsd":-50}`)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(invalidBody))
	req = withWorkspaceContext(req, wsID)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 4. POST / (configure valid limit)
	validBody := []byte(`{"enabled":true,"monthlyLimitUsd":200,"models":["gpt-4o"]}`)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(validBody))
	req = withWorkspaceContext(req, wsID)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var configured spendlimit.SpendLimitConfig
	err = json.Unmarshal(rec.Body.Bytes(), &configured)
	require.NoError(t, err)
	assert.True(t, configured.Enabled)
	assert.Equal(t, 200.0, configured.MonthlyLimitUSD)
}

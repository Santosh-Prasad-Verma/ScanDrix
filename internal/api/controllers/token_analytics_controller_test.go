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

type mockOrgParamsService struct {
	data map[string]any
}

func newMockOrgParamsService() *mockOrgParamsService {
	return &mockOrgParamsService{data: make(map[string]any)}
}

func (m *mockOrgParamsService) Get(ctx context.Context, orgID, teamID, key string) (any, error) {
	val, ok := m.data[orgID+":"+teamID+":"+key]
	if !ok {
		return nil, nil
	}
	return val, nil
}

func (m *mockOrgParamsService) Set(ctx context.Context, orgID, teamID, key string, val any) error {
	m.data[orgID+":"+teamID+":"+key] = val
	return nil
}

func (m *mockOrgParamsService) ListConfigured(ctx context.Context, key string) ([]spendlimit.OrgParameterRecord, error) {
	var res []spendlimit.OrgParameterRecord
	for _, v := range m.data {
		res = append(res, spendlimit.OrgParameterRecord{
			ConfigKey:   key,
			ConfigValue: v,
		})
	}
	return res, nil
}

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

	mockOrgParams := newMockOrgParamsService()
	cfgSvc := spendlimit.NewSpendLimitConfigService(mockOrgParams, monthlyUC, resolver)
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

func TestUsageController_TenantIsolationNegativeTests(t *testing.T) {
	usageCtrl, _, wsID := setupTestAnalytics()
	router := usageCtrl.Routes()
	otherOrgID := uuid.New().String()

	// 1. Unauthenticated request (no workspace context) -> 401 Unauthorized
	unauthReq := httptest.NewRequest(http.MethodGet, "/tokens/summary", nil)
	unauthRec := httptest.NewRecorder()
	router.ServeHTTP(unauthRec, unauthReq)
	assert.Equal(t, http.StatusUnauthorized, unauthRec.Code)

	// 2. Cross-tenant attempt via organizationId query param -> 403 Forbidden
	endpoints := []string{
		"/tokens/summary?organizationId=" + otherOrgID,
		"/tokens/overview?organizationId=" + otherOrgID,
		"/tokens/estimate?organizationId=" + otherOrgID,
		"/tokens/developer?organizationId=" + otherOrgID,
		"/tokens/daily?organizationId=" + otherOrgID,
		"/tokens/pr?organizationId=" + otherOrgID,
		"/tokens/review?organizationId=" + otherOrgID,
		"/tokens/area?organizationId=" + otherOrgID,
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(http.MethodGet, ep, nil)
		req = withWorkspaceContext(req, wsID)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code, "endpoint %s must return 403 Forbidden on mismatched tenant", ep)
	}
}

func TestSpendLimitController_TenantIsolationAndPersistenceNegativeTests(t *testing.T) {
	_, spendCtrl, wsID := setupTestAnalytics()
	router := spendCtrl.Routes()
	otherOrgID := uuid.New().String()

	// 1. Cross-tenant query parameter on GET / -> 403 Forbidden
	req := httptest.NewRequest(http.MethodGet, "/?organizationId="+otherOrgID, nil)
	req = withWorkspaceContext(req, wsID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// 2. Cross-tenant body organizationId on POST / -> 403 Forbidden
	body := []byte(`{"organizationId":"` + otherOrgID + `","enabled":true,"monthlyLimitUsd":150}`)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req = withWorkspaceContext(req, wsID)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// 3. Controller with nil persistence service -> 503 Service Unavailable (not fake success)
	cat := pricing.NewTokenPricingCatalog(pricing.WithOnlineFetching(false))
	resolver := pricing.NewPricingResolver(cat)
	calc := pricing.NewModelCostCalculator(resolver)
	tokenRepo := analyticsRepo.NewTokenUsageRepository(resolver)
	tokenSvc := analyticsRepo.NewTokenUsageService(tokenRepo)
	monthlyUC := usage.NewMonthlySpendUseCase(tokenSvc, calc)

	cfgSvcNilPersistence := spendlimit.NewSpendLimitConfigService(nil, monthlyUC, resolver)
	configureUCNil := spendlimit.NewConfigureSpendLimitUseCase(cfgSvcNilPersistence)
	getConfigUCNil := spendlimit.NewGetSpendLimitConfigUseCase(cfgSvcNilPersistence, nil, nil, resolver)

	spendCtrlNil := NewSpendLimitController(nil).WithSpendLimitServices(
		cfgSvcNilPersistence,
		configureUCNil,
		getConfigUCNil,
	)
	routerNil := spendCtrlNil.Routes()

	// Test POST / fails with 503 instead of pretending to succeed
	validBody := []byte(`{"enabled":true,"monthlyLimitUsd":200,"models":["gpt-4o"]}`)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(validBody))
	req = withWorkspaceContext(req, wsID)
	rec = httptest.NewRecorder()
	routerNil.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "write without persistence must fail with 503")

	// Test GET / on controller without persistence or use case fails with 503 instead of fabricating default $250
	spendCtrlNoRepo := NewSpendLimitController(nil)
	routerNoRepo := spendCtrlNoRepo.Routes()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req = withWorkspaceContext(req, wsID)
	rec = httptest.NewRecorder()
	routerNoRepo.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "read without persistence must fail with 503")
}

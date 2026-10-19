package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/analytics/warehouse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupWarehouseRouter() (*WarehouseController, *chi.Mux) {
	ctrl := NewWarehouseController(nil)
	r := chi.NewRouter()
	r.Route("/api/v1/analytics/warehouse", ctrl.RegisterRoutes)
	return ctrl, r
}

func TestWarehouseController_CRUD_And_Sync(t *testing.T) {
	_, r := setupWarehouseRouter()
	wsID := uuid.New()

	// 1. GET /config
	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/warehouse/"+wsID.String()+"/config", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var config warehouse.WarehouseConfig
	err := json.Unmarshal(rec.Body.Bytes(), &config)
	require.NoError(t, err)
	assert.Equal(t, wsID, config.WorkspaceID)

	// 2. PUT /config
	updatePayload := warehouse.WarehouseConfig{
		Type:               warehouse.WarehouseTypeSnowflake,
		Enabled:            true,
		AutoSync:           true,
		SnowflakeAccount:   "acme-prod.snowflakecomputing.com",
		SnowflakeWarehouse: "INGEST_WH",
		SnowflakeDatabase:  "CORP_DW",
		SnowflakeSchema:    "PUBLIC",
		SnowflakeUsername:  "INGEST_USER",
		SnowflakePassword:  "Secret123$",
	}
	bodyBytes, _ := json.Marshal(updatePayload)
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/analytics/warehouse/"+wsID.String()+"/config", bytes.NewBuffer(bodyBytes))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	r.ServeHTTP(putRec, putReq)

	assert.Equal(t, http.StatusOK, putRec.Code)
	var saved warehouse.WarehouseConfig
	err = json.Unmarshal(putRec.Body.Bytes(), &saved)
	require.NoError(t, err)
	assert.True(t, saved.Enabled)
	assert.Equal(t, "••••••••••••••••", saved.SnowflakePassword)

	// 3. POST /test
	testReq := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/warehouse/"+wsID.String()+"/test", bytes.NewBuffer(bodyBytes))
	testReq.Header.Set("Content-Type", "application/json")
	testRec := httptest.NewRecorder()
	r.ServeHTTP(testRec, testReq)

	assert.Equal(t, http.StatusOK, testRec.Code)
	var testResult warehouse.ConnectionTestResult
	err = json.Unmarshal(testRec.Body.Bytes(), &testResult)
	require.NoError(t, err)
	assert.True(t, testResult.Success)

	// 4. POST /sync?dry_run=true
	syncReq := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/warehouse/"+wsID.String()+"/sync?dry_run=true", nil)
	syncRec := httptest.NewRecorder()
	r.ServeHTTP(syncRec, syncReq)

	assert.Equal(t, http.StatusOK, syncRec.Code)
	var run warehouse.IngestionRun
	err = json.Unmarshal(syncRec.Body.Bytes(), &run)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", run.Status)
	assert.True(t, run.DryRun)
	assert.Greater(t, run.TotalRecords, 0)

	// 5. GET /history
	histReq := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/warehouse/"+wsID.String()+"/history", nil)
	histRec := httptest.NewRecorder()
	r.ServeHTTP(histRec, histReq)

	assert.Equal(t, http.StatusOK, histRec.Code)
	var runs []warehouse.IngestionRun
	err = json.Unmarshal(histRec.Body.Bytes(), &runs)
	require.NoError(t, err)
	assert.Len(t, runs, 1)

	// 6. GET /schema-ddl?type=snowflake
	ddlReq := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/warehouse/"+wsID.String()+"/schema-ddl?type=snowflake", nil)
	ddlRec := httptest.NewRecorder()
	r.ServeHTTP(ddlRec, ddlReq)

	assert.Equal(t, http.StatusOK, ddlRec.Code)
	assert.Contains(t, ddlRec.Body.String(), "CREATE TABLE IF NOT EXISTS SCANDRIX_PRS")
}

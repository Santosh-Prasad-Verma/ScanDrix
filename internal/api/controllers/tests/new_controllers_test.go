// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - New Controllers Parity Unit Tests
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemControllerVersionStatus(t *testing.T) {
	ctrl := controllers.NewSystemController()
	router := ctrl.Routes()

	t.Run("cloud mode returns unknown=true reason=cloud", func(t *testing.T) {
		os.Setenv("SCANDRIX_CLOUD_MODE", "true")
		defer os.Unsetenv("SCANDRIX_CLOUD_MODE")

		req := httptest.NewRequest(http.MethodGet, "/version-status", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var res controllers.VersionStatus
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		assert.True(t, res.Unknown)
		assert.Equal(t, "cloud", res.Reason)
	})

	t.Run("no semver returns unknown=true reason=no-version", func(t *testing.T) {
		os.Setenv("SCANDRIX_CLOUD_MODE", "false")
		os.Setenv("RELEASE_VERSION", "dev-master")
		defer os.Unsetenv("RELEASE_VERSION")

		req := httptest.NewRequest(http.MethodGet, "/version-status", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var res controllers.VersionStatus
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		assert.True(t, res.Unknown)
		assert.Equal(t, "no-version", res.Reason)
	})
}

func TestSkillsController(t *testing.T) {
	ctrl := controllers.NewSkillsController()
	router := ctrl.Routes()

	t.Run("list skills returns catalog", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var list []map[string]any
		err := json.NewDecoder(rec.Body).Decode(&list)
		require.NoError(t, err)
		assert.NotEmpty(t, list)
	})

	t.Run("get bundled skill meta and instructions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/scandrix-review/meta", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var meta controllers.SkillMetaResponse
		err := json.NewDecoder(rec.Body).Decode(&meta)
		require.NoError(t, err)
		assert.Equal(t, "scandrix-review", meta.Name)

		reqInst := httptest.NewRequest(http.MethodGet, "/scandrix-review/instructions", nil)
		recInst := httptest.NewRecorder()
		router.ServeHTTP(recInst, reqInst)

		assert.Equal(t, http.StatusOK, recInst.Code)
		var inst controllers.SkillInstructionsResponse
		err = json.NewDecoder(recInst.Body).Decode(&inst)
		require.NoError(t, err)
		assert.Contains(t, inst.Instructions, "scandrix review")
	})

	t.Run("unknown skill returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/non-existent-skill-99999/meta", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestUserLogController(t *testing.T) {
	ctrl := controllers.NewUserLogController(nil)
	router := ctrl.Routes()

	t.Run("post status-change succeeds with 204", func(t *testing.T) {
		body := bytes.NewBufferString(`{"userId":"user-123","email":"dev@scandrix.internal","status":"ACTIVE"}`)
		req := httptest.NewRequest(http.MethodPost, "/status-change", body)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("get code-review-settings returns empty array when repo is nil", func(t *testing.T) {
		wsID := uuid.New()
		ctx := context.WithValue(context.Background(), auth.WorkspaceContextKey, wsID)
		req := httptest.NewRequest(http.MethodGet, "/code-review-settings", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var res map[string]any
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		assert.Equal(t, float64(0), res["total"])
	})
}

func TestLicenseControllerExtended(t *testing.T) {
	ctrl := controllers.NewLicenseController(nil)
	router := ctrl.Routes()
	wsID := uuid.New()
	ctx := auth.WithWorkspaceContext(context.Background(), wsID)

	t.Run("get status returns valid community license", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/status", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var res map[string]any
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		assert.Equal(t, true, res["valid"])
		assert.Equal(t, "COMMUNITY", res["plan"])
	})

	t.Run("get org-status returns active subscription", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/org-status", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var res map[string]any
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		assert.Equal(t, true, res["valid"])
		assert.Equal(t, "active", res["subscriptionStatus"])
	})

	t.Run("assign license partitions successful users", func(t *testing.T) {
		body := bytes.NewBufferString(`{"teamId":"team-1","users":[{"gitId":"1001","gitTool":"github","licenseStatus":"active"}]}`)
		req := httptest.NewRequest(http.MethodPost, "/assign", body).WithContext(ctx)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var res map[string]any
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		assert.NotEmpty(t, res["successful"])
	})

	t.Run("removable seats and prune seats", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/removable-seats", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)

		pruneBody := bytes.NewBufferString(`{"gitIds":["1001","1002"]}`)
		reqPrune := httptest.NewRequest(http.MethodPost, "/prune-seats", pruneBody).WithContext(ctx)
		recPrune := httptest.NewRecorder()
		router.ServeHTTP(recPrune, reqPrune)
		assert.Equal(t, http.StatusOK, recPrune.Code)
	})

	t.Run("trial extension returns honest unconfigured response when webhook missing", func(t *testing.T) {
		body := bytes.NewBufferString(`{"teamId":"team-alpha","teamSize":15,"message":"Need 5 more trial days"}`)
		req := httptest.NewRequest(http.MethodPost, "/trial-extension-request", body).WithContext(ctx)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var res map[string]any
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		assert.Equal(t, false, res["success"])
		assert.Contains(t, res["message"], "not configured")
	})
}

func TestUserController(t *testing.T) {
	ctrl := controllers.NewUserController(nil, nil)
	router := ctrl.Routes()

	t.Run("check email returns json boolean", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/email?email=test@scandrix.internal", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("get invite returns invite status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/invite?userId=user-456", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var res map[string]any
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		assert.Equal(t, "user-456", res["userId"])
	})

	t.Run("join organization completes", func(t *testing.T) {
		body := bytes.NewBufferString(`{"organizationId":"org-789"}`)
		req := httptest.NewRequest(http.MethodPost, "/join-organization", body)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var res map[string]any
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		assert.Equal(t, true, res["success"])
	})
}

func TestWorkflowQueueController(t *testing.T) {
	ctrl := controllers.NewWorkflowQueueController(nil)
	router := ctrl.Routes()

	t.Run("get metrics returns queue summary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var res map[string]any
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		assert.Equal(t, float64(200), res["status"])
		assert.NotNil(t, res["data"])
	})

	t.Run("get job status returns status object", func(t *testing.T) {
		jobID := uuid.New().String()
		req := httptest.NewRequest(http.MethodGet, "/jobs/"+jobID, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var res map[string]any
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		assert.Equal(t, float64(200), res["status"])
	})
}

func TestSSOConfigController(t *testing.T) {
	ctrl := controllers.NewSSOConfigController(nil)
	router := ctrl.Routes()
	wsID := uuid.New()
	ctx := auth.WithWorkspaceContext(context.Background(), wsID)

	t.Run("get and post sso-config", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var res map[string]any
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		assert.Equal(t, wsID.String(), res["organizationId"])

		postBody := bytes.NewBufferString(`{"protocol":"SAML","active":true,"domains":["company.com"]}`)
		reqPost := httptest.NewRequest(http.MethodPost, "/", postBody).WithContext(ctx)
		recPost := httptest.NewRecorder()
		router.ServeHTTP(recPost, reqPost)

		assert.Equal(t, http.StatusOK, recPost.Code)
	})
}

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
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemControllerVersionStatus(t *testing.T) {
	ctrl := controllers.NewSystemController(nil)
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

// inviteUserRepo serves a single account so the invite endpoint has a real
// source of truth to report.
type inviteUserRepo struct {
	record *database.UserRecord
}

func (m *inviteUserRepo) GetUserByID(_ context.Context, id uuid.UUID) (*database.UserRecord, error) {
	if m.record == nil || m.record.UUID != id {
		return nil, nil
	}
	return m.record, nil
}
func (m *inviteUserRepo) GetUserByEmail(_ context.Context, _ string) (*database.UserRecord, error) {
	return nil, nil
}
func (m *inviteUserRepo) UpdateUserStatus(_ context.Context, _ uuid.UUID, _ string) error { return nil }
func (m *inviteUserRepo) UpdateUserPassword(_ context.Context, _ string, _ string) error  { return nil }

func TestUserController(t *testing.T) {
	ctrl := controllers.NewUserController(nil, nil)
	router := ctrl.Routes()

	// With no AuthController wired the availability check cannot be performed.
	// It used to answer {"exists": email != ""}, claiming every address was
	// taken; it now reports the check as unavailable.
	t.Run("check email reports unavailable without an auth controller", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/email?email=test@scandrix.internal", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.NotContains(t, rec.Body.String(), "true")
	})

	// The invite endpoint previously answered {"valid":true,"status":"pending"}
	// for any userId, telling callers an invitation existed when none did.
	//
	// It is now authenticated and ignores the userId parameter entirely, so it
	// reports only the calling account. Requiring a session alone would still
	// have let one user probe another's status by passing a different userId
	// (AUDIT_REMEDIATION.md F-49), so the tests below pass a *different* id in
	// the query string and expect the session's own account to be reported.
	t.Run("get invite reports the caller's own stored status", func(t *testing.T) {
		callerID := uuid.New()
		otherID := uuid.New()
		repo := &inviteUserRepo{record: &database.UserRecord{
			UUID:   callerID,
			Email:  "invitee@scandrix.internal",
			Status: "pending",
		}}
		authSvc := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
		inviteRouter := controllers.NewUserController(
			controllers.NewAuthController(authSvc, nil), repo,
		).Routes()
		bearer := func(t *testing.T, userID uuid.UUID) string {
			t.Helper()
			tok, _, err := authSvc.GenerateTokenPairWithEmail(
				userID, uuid.New(), models.RoleMember, "invitee@scandrix.internal")
			require.NoError(t, err)
			return tok
		}

		// A real signed token, not a hand-built context: the route sits behind
		// the actual authentication middleware, and a test that injected the
		// profile directly would pass even if the middleware were broken.
		callerToken := bearer(t, callerID)
		withCaller := func(path string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Authorization", "Bearer "+callerToken)
			rec := httptest.NewRecorder()
			inviteRouter.ServeHTTP(rec, req)
			return rec
		}

		// The query parameter names somebody else; the response must describe
		// the caller's own record, and the repo here only ever returns the
		// caller's record regardless, so the assertion is on the reported id.
		rec := withCaller("/invite?userId=" + otherID.String())
		assert.Equal(t, http.StatusOK, rec.Code)
		var res map[string]any
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&res))
		assert.Equal(t, true, res["valid"])
		assert.Equal(t, "pending", res["status"])
		assert.Equal(t, callerID.String(), res["userId"], "the response must describe the caller, not the supplied userId")

		// A known account that is not awaiting acceptance is not a valid invite.
		repo.record = &database.UserRecord{UUID: callerID, Email: "a@b.test", Status: "active"}
		rec = withCaller("/invite?userId=" + otherID.String())
		require.Equal(t, http.StatusOK, rec.Code)
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&res))
		assert.Equal(t, false, res["valid"])
		assert.Equal(t, "active", res["status"])
	})

	// The userId parameter used to be parsed and rejected when malformed. It is
	// now ignored altogether, so a junk value changes nothing: the handler
	// reports the caller's own account. This sub-test now pins that the
	// parameter can no longer influence the response at all.
	t.Run("get invite ignores a malformed userId", func(t *testing.T) {
		callerID := uuid.New()
		repo := &inviteUserRepo{record: &database.UserRecord{
			UUID:   callerID,
			Email:  "invitee@scandrix.internal",
			Status: "pending",
		}}
		authSvc := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
		inviteRouter := controllers.NewUserController(
			controllers.NewAuthController(authSvc, nil), repo,
		).Routes()
		tok, _, err := authSvc.GenerateTokenPairWithEmail(
			callerID, uuid.New(), models.RoleMember, "invitee@scandrix.internal")
		require.NoError(t, err)

		for _, path := range []string{
			"/invite?userId=user-456",
			"/invite?userId=" + uuid.NewString(),
			"/invite",
		} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Authorization", "Bearer "+tok)
			rec := httptest.NewRecorder()
			inviteRouter.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code, "path %s", path)
			var res map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&res))
			assert.Equal(t, callerID.String(), res["userId"], "path %s", path)
			assert.Equal(t, "pending", res["status"], "path %s", path)
		}
	})

	t.Run("get invite returns 404 for an unknown account", func(t *testing.T) {
		callerID := uuid.New()
		authSvc := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
		inviteRouter := controllers.NewUserController(
			controllers.NewAuthController(authSvc, nil), &inviteUserRepo{},
		).Routes()
		tok, _, err := authSvc.GenerateTokenPairWithEmail(
			callerID, uuid.New(), models.RoleMember, "ghost@scandrix.internal")
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/invite?userId="+uuid.New().String(), nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		inviteRouter.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		var res map[string]any
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&res))
		assert.Equal(t, false, res["valid"])
	})

	// This endpoint was reachable without authentication and performed a real
	// cross-tenant write: it granted organization membership, repointed the
	// caller's workspace, and could delete the workspace it orphaned
	// (AUDIT_REMEDIATION.md F-02). It must now refuse anonymous callers.
	//
	// This controller is built with no AuthController, so the shared middleware
	// fails closed with 503 (the authenticator is unavailable, so nobody can
	// be authenticated). The 401 case, with a real authenticator wired, is
	// covered in user_controller_security_test.go.
	t.Run("join organization requires authentication", func(t *testing.T) {
		body := bytes.NewBufferString(`{"organizationId":"org-789"}`)
		req := httptest.NewRequest(http.MethodPost, "/join-organization", body)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.NotContains(t, rec.Body.String(), `"success":true`)
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

	t.Run("get returns an explicit empty state", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var res map[string]any
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		assert.Equal(t, wsID.String(), res["organizationId"])
		// The endpoint no longer fabricates a configuration: an unconfigured
		// workspace is reported as such.
		assert.Equal(t, "not_configured", res["status"])
		assert.Equal(t, false, res["active"])
	})

	t.Run("post rejects an incomplete active config", func(t *testing.T) {
		// Activating SAML without an issuer, entry point and IdP certificate
		// would leave the workspace unable to verify a single assertion, so it
		// is refused rather than stored.
		postBody := bytes.NewBufferString(`{"protocol":"SAML","active":true,"domains":["company.com"]}`)
		reqPost := httptest.NewRequest(http.MethodPost, "/", postBody).WithContext(ctx)
		recPost := httptest.NewRecorder()
		router.ServeHTTP(recPost, reqPost)

		assert.Equal(t, http.StatusBadRequest, recPost.Code)
	})

	t.Run("post rejects an unsupported protocol", func(t *testing.T) {
		postBody := bytes.NewBufferString(`{"protocol":"LDAP","active":false}`)
		reqPost := httptest.NewRequest(http.MethodPost, "/", postBody).WithContext(ctx)
		recPost := httptest.NewRecorder()
		router.ServeHTTP(recPost, reqPost)

		assert.Equal(t, http.StatusBadRequest, recPost.Code)
	})

	t.Run("post without persistence reports unavailable", func(t *testing.T) {
		// A valid config on a deployment with no SSO store must say so instead
		// of pretending the save succeeded.
		postBody := bytes.NewBufferString(`{"protocol":"SAML","active":false,"domains":["company.com"]}`)
		reqPost := httptest.NewRequest(http.MethodPost, "/", postBody).WithContext(ctx)
		recPost := httptest.NewRecorder()
		router.ServeHTTP(recPost, reqPost)

		assert.Equal(t, http.StatusServiceUnavailable, recPost.Code)
	})
}

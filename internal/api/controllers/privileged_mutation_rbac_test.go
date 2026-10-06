package controllers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/require"
)

// TestPrivilegedMutationsRequireOwnerOrAdmin pins the policy for mutations that
// grant or revoke access, move money, or hand out credentials.
//
// auth.RoleGuard implements a hierarchical rank (OWNER > ADMIN > MEMBER >
// VIEWER) and had its own unit test, but no production route used it: these
// handlers only checked that a workspace was present in the context. A VIEWER
// could therefore assign licenses, revoke another user's repository access and
// raise spend limits.
//
// The guard is middleware, so the denial is asserted at the router rather than
// in the handler. Requests that reach the handler would need a wired repository;
// the assertion here is that they never get there, which is also why the
// handlers are safe to drive with a nil repository.
func TestPrivilegedMutationsRequireOwnerOrAdmin(t *testing.T) {
	ws := uuid.New()

	license := controllers.NewLicenseController(nil).Routes()
	permissions := controllers.NewPermissionsController(nil).Routes()
	spend := controllers.NewSpendLimitController(nil).Routes()
	usage := controllers.NewUsageController(nil).Routes()
	team := controllers.NewTeamController(nil).Routes()

	cases := []struct {
		name    string
		router  http.Handler
		method  string
		path    string
		payload string
	}{
		{"license activate", license, "POST", "/activate", `{}`},
		{"license assign", license, "POST", "/assign", `{"users":[]}`},
		{"license prune seats", license, "POST", "/prune-seats", `{}`},
		{"license trial extension", license, "POST", "/trial-extension-request", `{}`},
		{"permissions assign repos", permissions, "POST", "/users/" + uuid.NewString() + "/repositories", `{"repositoryIds":[]}`},
		{"permissions revoke repo", permissions, "DELETE", "/users/" + uuid.NewString() + "/repositories/" + uuid.NewString(), ``},
		{"permissions assign repos legacy", permissions, "POST", "/assign-repos", `{}`},
		{"spend limit configure", spend, "POST", "/", `{"monthlyLimitUsd":1}`},
		{"usage spend limit update", usage, "PUT", "/spend-limit", `{"limitUsd":1}`},
		{"team remove member", team, "DELETE", "/" + uuid.NewString() + "/members/" + uuid.NewString(), ``},
		{"team create cli key", team, "POST", "/" + uuid.NewString() + "/cli-keys", `{}`},
		{"team update cli key", team, "PATCH", "/" + uuid.NewString() + "/cli-keys/" + uuid.NewString(), `{}`},
		{"team revoke cli key", team, "DELETE", "/" + uuid.NewString() + "/cli-keys/" + uuid.NewString(), ``},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, role := range []models.UserRole{models.RoleViewer, models.RoleMember} {
				ctx := context.WithValue(context.Background(), auth.WorkspaceContextKey, ws)
				ctx = auth.WithAccountContext(ctx, &models.AccountProfile{WorkspaceID: ws, Role: role})
				r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.payload)).WithContext(ctx)
				w := httptest.NewRecorder()
				tc.router.ServeHTTP(w, r)
				require.Equal(t, http.StatusForbidden, w.Code, "%s must be denied for %s", tc.name, role)
			}

			// A workspace-only context carries no role, so it is denied too rather
			// than treated as implicitly privileged.
			ctx := context.WithValue(context.Background(), auth.WorkspaceContextKey, ws)
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.payload)).WithContext(ctx)
			w := httptest.NewRecorder()
			tc.router.ServeHTTP(w, r)
			require.Equal(t, http.StatusForbidden, w.Code, "%s must be denied without an account profile", tc.name)
		})
	}
}

// TestOwnerAndAdminPassTheGuard guards against over-tightening: the middleware
// must not reject the roles the policy allows. These requests proceed past the
// guard and fail later on the nil repository instead, which is how we know the
// guard let them through.
func TestOwnerAndAdminPassTheGuard(t *testing.T) {
	ws := uuid.New()
	spend := controllers.NewSpendLimitController(nil).Routes()

	for _, role := range []models.UserRole{models.RoleOwner, models.RoleAdmin} {
		ctx := context.WithValue(context.Background(), auth.WorkspaceContextKey, ws)
		ctx = auth.WithAccountContext(ctx, &models.AccountProfile{WorkspaceID: ws, Role: role})
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"monthlyLimitUsd":1}`)).WithContext(ctx)
		w := httptest.NewRecorder()
		spend.ServeHTTP(w, r)
		require.NotEqual(t, http.StatusForbidden, w.Code, "%s must be allowed by the guard", role)
	}
}

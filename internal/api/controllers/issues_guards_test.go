package controllers_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/issues"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/require"
)

type failedIssueLookup struct {
	controllers.IssuesRepository
	err error
}

func (s failedIssueLookup) GetTrackedIssue(context.Context, uuid.UUID, uuid.UUID) (*issues.TrackedIssue, error) {
	return nil, s.err
}

func TestIssueUpdateDistinguishesMissingResourceFromOutage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"missing", pgx.ErrNoRows, 404},
		{"outage", errors.New("private database diagnostic"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := uuid.New()
			ctx := context.WithValue(context.Background(), auth.WorkspaceContextKey, ws)
			ctx = auth.WithAccountContext(ctx, &models.AccountProfile{WorkspaceID: ws, Role: models.RoleMember})
			r := httptest.NewRequest("PATCH", "/"+uuid.NewString(), strings.NewReader(`{"status":"RESOLVED"}`)).WithContext(ctx)
			w := httptest.NewRecorder()
			controllers.NewIssuesController(failedIssueLookup{err: tc.err}).Routes().ServeHTTP(w, r)
			require.Equal(t, tc.status, w.Code)
			require.NotContains(t, w.Body.String(), "private database diagnostic")
		})
	}
}

func TestIssueCountsUnavailableWithoutDataSource(t *testing.T) {
	ctx := context.WithValue(context.Background(), auth.WorkspaceContextKey, uuid.New())
	w := httptest.NewRecorder()
	controllers.NewIssuesController(nil).Routes().ServeHTTP(w, httptest.NewRequest("GET", "/count", nil).WithContext(ctx))
	require.Equal(t, 503, w.Code)
}

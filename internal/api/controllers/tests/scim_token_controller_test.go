// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package controllers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/enterprise/scim"
)

func doSCIMTokenRequest(t *testing.T, ctrl *controllers.SCIMTokenController, method string, withWorkspace bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/token", nil)
	if withWorkspace {
		req = req.WithContext(auth.WithWorkspaceContext(req.Context(), uuid.New()))
	}
	rec := httptest.NewRecorder()
	ctrl.Routes().ServeHTTP(rec, req)
	return rec
}

// A deployment with no SCIM persistence must say so, not serve a fabricated
// "enabled" answer. 503 is the honest status.
func TestSCIMTokenRoutesReportUnavailableWithoutPersistence(t *testing.T) {
	ctrl := controllers.NewSCIMTokenController(nil)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			rec := doSCIMTokenRequest(t, ctrl, method, true)

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected 503 when SCIM is not configured, got %d (%s)", rec.Code, rec.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed decoding response: %v", err)
			}
			msg, _ := body["error"].(string)
			if !strings.Contains(msg, "not configured") {
				t.Fatalf("expected the error to state the deployment is unconfigured, got %q", msg)
			}
			if _, present := body["enabled"]; present {
				t.Fatal("an unconfigured deployment must not report an enabled state")
			}
		})
	}
}

// Session authentication is not enough: a request with no tenant must be
// rejected before any token state is read or written.
func TestSCIMTokenRoutesRejectMissingWorkspaceContext(t *testing.T) {
	// A service with no repository is non-nil, so the unavailable branch is
	// passed and the tenancy check is the one under test.
	ctrl := controllers.NewSCIMTokenController(scim.NewSCIMService())

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			rec := doSCIMTokenRequest(t, ctrl, method, false)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 without a workspace context, got %d (%s)", rec.Code, rec.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed decoding response: %v", err)
			}
			if _, leaked := body["token"]; leaked {
				t.Fatal("an unauthorized response must not contain a token field")
			}
		})
	}
}

// A uuid.Nil workspace is not a tenant. It must fail closed exactly like a
// missing context rather than reaching the persistence layer.
func TestSCIMTokenRoutesRejectNilWorkspace(t *testing.T) {
	ctrl := controllers.NewSCIMTokenController(scim.NewSCIMService())

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/token", nil)
			req = req.WithContext(auth.WithWorkspaceContext(req.Context(), uuid.Nil))
			rec := httptest.NewRecorder()
			ctrl.Routes().ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 for a nil workspace, got %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

// When persistence is present but unusable, the controller must surface a 500
// rather than reporting that SCIM is disabled -- "we could not read the token"
// and "there is no token" are different answers.
func TestSCIMTokenRoutesSurfacePersistenceFailureAs500(t *testing.T) {
	ctrl := controllers.NewSCIMTokenController(scim.NewSCIMService())

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			rec := doSCIMTokenRequest(t, ctrl, method, true)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("expected 500 when the token store is unusable, got %d (%s)", rec.Code, rec.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed decoding response: %v", err)
			}
			if _, reported := body["enabled"]; reported {
				t.Fatal("a failed read must not be reported as a disabled/enabled state")
			}
		})
	}
}

// The status endpoint exists so the UI can decide whether to show SCIM. It must
// never carry a token, and it must state plainly that the token is not
// retrievable, so the UI does not offer a reveal affordance that cannot work.
func TestSCIMTokenStatusNeverReturnsAToken(t *testing.T) {
	ctrl := controllers.NewSCIMTokenController(scim.NewSCIMService())

	rec := doSCIMTokenRequest(t, ctrl, http.MethodGet, true)

	// Unusable persistence here, so assert the shape of the refusal rather than
	// a success payload; the guarantee under test is that no token escapes.
	if strings.Contains(rec.Body.String(), "scim_") {
		t.Fatalf("the status endpoint must never carry a provisioning token: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "token_recoverable\": true") {
		t.Fatalf("the status endpoint must not claim the token is recoverable: %s", rec.Body.String())
	}
}

// Issuing a token against an unusable store must fail loudly and return nothing
// that looks like a credential.
func TestSCIMTokenIssueDoesNotReturnATokenOnFailure(t *testing.T) {
	ctrl := controllers.NewSCIMTokenController(scim.NewSCIMService())

	rec := doSCIMTokenRequest(t, ctrl, http.MethodPost, true)

	if rec.Code == http.StatusCreated {
		t.Fatalf("issuing against an unusable store must not report 201: %s", rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "scim_") {
		t.Fatalf("a failed issue must not leak a token-shaped value: %s", body)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected a JSON content type, got %q", rec.Header().Get("Content-Type"))
	}
}

// Revocation must not report success when nothing was actually revoked.
func TestSCIMTokenRevokeDoesNotReportSuccessOnFailure(t *testing.T) {
	ctrl := controllers.NewSCIMTokenController(scim.NewSCIMService())

	rec := doSCIMTokenRequest(t, ctrl, http.MethodDelete, true)

	if rec.Code == http.StatusOK {
		t.Fatalf("revoking against an unusable store must not report 200: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Fatalf("a failed revoke must not report SCIM as disabled: %s", rec.Body.String())
	}
}

// The controller must be usable without a service and must not panic on any
// route combination.
func TestSCIMTokenControllerIsSafeWithNilService(t *testing.T) {
	ctrl := controllers.NewSCIMTokenController(nil)
	if ctrl == nil {
		t.Fatal("NewSCIMTokenController must return a controller even with no service")
	}
	rec := doSCIMTokenRequest(t, ctrl, http.MethodGet, false)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 before the tenancy check for an unconfigured deployment, got %d", rec.Code)
	}
}

// A service constructed without a repository must not be mistaken for a
// configured one by the controller.
func TestSCIMServiceWithoutRepositoryIsNotConfigured(t *testing.T) {
	svc := scim.NewSCIMService()
	if svc == nil {
		t.Fatal("NewSCIMService() must return a non-nil service")
	}
	if _, _, err := svc.IssueToken(context.Background(), uuid.New()); err == nil {
		t.Fatal("issuing a token without persistence must fail")
	}
	if err := svc.RevokeToken(context.Background(), uuid.New()); err == nil {
		t.Fatal("revoking a token without persistence must fail")
	}
	if enabled, err := svc.TokenState(context.Background(), uuid.New()); err == nil {
		t.Fatalf("reading token state without persistence must fail, got enabled=%v", enabled)
	}
}

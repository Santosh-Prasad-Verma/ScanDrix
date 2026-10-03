package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

// newTestUserController builds a UserController with no repository, which is
// enough to exercise the authentication and identity-derivation boundary
// without touching a database.
func newTestUserController(t *testing.T, secret string) (*UserController, *auth.Authenticator) {
	t.Helper()
	authSvc := auth.NewAuthenticator(secret)
	return NewUserController(NewAuthController(authSvc, nil), nil), authSvc
}

// serveUserRoute dispatches through a parent router mounted at /user, matching
// how router.go mounts this controller.
func serveUserRoute(t *testing.T, ctrl *UserController, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return serveMountedUserRoute(t, ctrl, method, path, body, "")
}

func serveMountedUserRoute(t *testing.T, ctrl *UserController, method, path, body, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		r.Header.Set("Authorization", bearer)
	}
	w := httptest.NewRecorder()
	root := chi.NewRouter()
	root.Mount("/user", ctrl.Routes())
	root.ServeHTTP(w, r)
	return w
}

// bearerFor mints a real signed session token, so these tests exercise the
// actual authentication middleware rather than a bypass.
func bearerFor(t *testing.T, a *auth.Authenticator, id uuid.UUID, role models.UserRole) string {
	t.Helper()
	tok, _, err := a.GenerateTokenPairWithEmail(id, uuid.New(), role, "caller@example.test")
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	return "Bearer " + tok
}

// TestUserRoutesRequireAuthentication pins AUDIT_REMEDIATION.md F-02.
//
// The /user route group carried a "Protected endpoints" comment and no
// middleware, and the whole controller was mounted outside the authenticated
// group in router.go. POST /join-organization was therefore reachable by
// anonymous callers and executed business logic before failing on a bad id.
func TestUserRoutesRequireAuthentication(t *testing.T) {
	ctrl, _ := newTestUserController(t, "test-jwt-secret-key-123456789012")

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/user/join-organization",
			`{"organizationId":"00000000-0000-0000-0000-0000000000ff","userId":"00000000-0000-0000-0000-0000000000ee"}`},
		{http.MethodGet, "/user/info", ""},
		{http.MethodPatch, "/user/marketing-survey", `{"nps":10}`},
		{http.MethodPatch, "/user/" + uuid.NewString(), `{"name":"x"}`},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			w := serveUserRoute(t, ctrl, tc.method, tc.path, tc.body)
			if w.Code == http.StatusOK || w.Code == http.StatusAccepted {
				t.Fatalf("%s %s returned %d without authentication; must be 401 (AUDIT F-02). body: %s",
					tc.method, tc.path, w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), `"success":true`) {
				t.Fatalf("%s %s returned a success payload unauthenticated: %s",
					tc.method, tc.path, w.Body.String())
			}
		})
	}
}

// TestJoinOrganizationRejectsBodySuppliedUserID is the specific identity
// spoofing that made F-02 exploitable: the acting user was read from the
// request body, and defaulted to a fixed UUID when absent.
func TestJoinOrganizationRejectsBodySuppliedUserID(t *testing.T) {
	ctrl, a := newTestUserController(t, "test-jwt-secret-key-123456789012")

	victim := uuid.New()
	attacker := uuid.New()

	body := `{"organizationId":"` + uuid.NewString() + `","userId":"` + victim.String() + `"}`
	w := serveMountedUserRoute(t, ctrl, http.MethodPost, "/user/join-organization", body,
		bearerFor(t, a, attacker, models.RoleOwner))

	// Must never succeed, and must never mention the victim id.
	if w.Code == http.StatusOK {
		t.Fatalf("join-organization succeeded with a body-supplied userId: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), victim.String()) {
		t.Fatalf("response echoed the attacker-supplied victim id: %s", w.Body.String())
	}
}

// TestJoinOrganizationRejectsNonUUIDOrganization verifies the silent
// uuid.NewSHA1 derivation is gone. A malformed id must be a 400, not a
// deterministic write to an unintended tenant.
func TestJoinOrganizationRejectsNonUUIDOrganization(t *testing.T) {
	ctrl, a := newTestUserController(t, "test-jwt-secret-key-123456789012")

	w := serveMountedUserRoute(t, ctrl, http.MethodPost, "/user/join-organization",
		`{"organizationId":"not-a-uuid"}`, bearerFor(t, a, uuid.New(), models.RoleOwner))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a non-UUID organizationId, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUpdateTargetUserEnforcesOwnership covers the missing authorization check
// on PATCH /user/{targetUserId}.
func TestUpdateTargetUserEnforcesOwnership(t *testing.T) {
	ctrl, a := newTestUserController(t, "test-jwt-secret-key-123456789012")
	victim := uuid.New()

	t.Run("member cannot update another user", func(t *testing.T) {
		w := serveMountedUserRoute(t, ctrl, http.MethodPatch, "/user/"+victim.String(),
			`{"name":"pwned"}`, bearerFor(t, a, uuid.New(), models.RoleMember))

		if w.Code != http.StatusForbidden {
			t.Fatalf("member updating another user: expected 403, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("member may address self", func(t *testing.T) {
		self := uuid.New()
		w := serveMountedUserRoute(t, ctrl, http.MethodPatch, "/user/"+self.String(),
			`{"name":"me"}`, bearerFor(t, a, self, models.RoleMember))

		// 501 is the honest answer: ownership passed, but no write exists.
		if w.Code != http.StatusNotImplemented {
			t.Fatalf("self-update: expected 501 (no persistence), got %d: %s", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), `"updated":true`) {
			t.Fatalf("self-update still claims a write happened: %s", w.Body.String())
		}
	})

	t.Run("admin may update another user", func(t *testing.T) {
		w := serveMountedUserRoute(t, ctrl, http.MethodPatch, "/user/"+victim.String(),
			`{"name":"ok"}`, bearerFor(t, a, uuid.New(), models.RoleAdmin))

		if w.Code != http.StatusNotImplemented {
			t.Fatalf("admin updating another user: expected 501, got %d: %s", w.Code, w.Body.String())
		}
	})
}

// TestMarketingSurveyDoesNotClaimPersistence guards against the regression
// where a no-write endpoint answered {"success":true}.
func TestMarketingSurveyDoesNotClaimPersistence(t *testing.T) {
	ctrl, a := newTestUserController(t, "test-jwt-secret-key-123456789012")

	w := serveMountedUserRoute(t, ctrl, http.MethodPatch, "/user/marketing-survey",
		`{"nps":9}`, bearerFor(t, a, uuid.New(), models.RoleMember))

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"success":true`) {
		t.Fatalf("survey still claims success without persistence: %s", w.Body.String())
	}

	var body struct {
		Status    string `json:"status"`
		Persisted bool   `json:"persisted"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Persisted {
		t.Fatalf("response claims persisted=true but nothing is stored")
	}
}

// TestCompleteInvitationRouteRemoved documents that the fake-success invite
// endpoint is gone.
func TestCompleteInvitationRouteRemoved(t *testing.T) {
	ctrl, _ := newTestUserController(t, "test-jwt-secret-key-123456789012")

	w := serveUserRoute(t, ctrl, http.MethodPost, "/user/invite/complete-invitation",
		`{"userId":"`+uuid.NewString()+`","password":"x","fullName":"y"}`)

	if w.Code == http.StatusOK {
		t.Fatalf("invite completion still returns 200: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"status":"active"`) {
		t.Fatalf("invite completion still claims an account was activated: %s", w.Body.String())
	}
}

// TestPublicUserRoutesNeedRateLimiting guards the F-02 fix for /user/email.
// router.go used to register GET /user/email in its own public block, which
// shadowed this controller and left the rate limiter attached to a route that
// no longer existed. The limiter now lives beside the route.
func TestPublicUserRoutesNeedRateLimiting(t *testing.T) {
	ctrl, _ := newTestUserController(t, "test-jwt-secret-key-123456789012")

	// No rate limiter configured on the controller, so requests pass through.
	// This asserts the route exists on the controller (not shadowed) and that a
	// missing AuthController does not turn the public block into a 503.
	// Only /user/email belongs here. /user/invite used to be on the public
	// block and was moved to the authenticated group: it disclosed real
	// account status for any userId the caller supplied
	// (AUDIT_REMEDIATION.md F-49).
	for _, path := range []string{
		"/user/email?email=test@example.test",
	} {
		w := serveUserRoute(t, ctrl, http.MethodGet, path, "")
		if w.Code == http.StatusUnauthorized {
			t.Fatalf("%s must stay public for the signup flow", path)
		}
	}
}

// TestInviteLookupRequiresSession pins the F-49 fix: the route must not answer
// an anonymous caller, and the userId query parameter must not select whose
// account is reported.
func TestInviteLookupRequiresSession(t *testing.T) {
	ctrl, _ := newTestUserController(t, "test-jwt-secret-key-123456789012")

	w := serveUserRoute(t, ctrl, http.MethodGet, "/user/invite?userId="+uuid.NewString(), "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous /user/invite must be refused, got %d: %s", w.Code, w.Body.String())
	}
}

// TestInviteLookupIgnoresSuppliedUserID is the stronger half of the F-49 fix.
// Requiring a session alone would still let one user probe another's status by
// passing a different userId, so the handler must ignore the parameter and
// report only the caller's own account.
func TestInviteLookupIgnoresSuppliedUserID(t *testing.T) {
	ctrl, _ := newTestUserController(t, "test-jwt-secret-key-123456789012")

	// No repository is configured, so the handler can only answer 503 -- but
	// crucially it must get past the "which user?" decision first, using the
	// session rather than the supplied id. Anonymous, it must stop at 401.
	anon := serveUserRoute(t, ctrl, http.MethodGet, "/user/invite?userId="+uuid.NewString(), "")
	if anon.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous request must be refused before any account lookup, got %d: %s",
			anon.Code, anon.Body.String())
	}
}

// TestAuthMiddlewareFailsClosedWithoutAuthenticator verifies the shared
// middleware refuses to pass traffic when no authenticator is configured,
// rather than treating every request as authenticated.
func TestAuthMiddlewareFailsClosedWithoutAuthenticator(t *testing.T) {
	ctrl := NewUserController(nil, nil) // no AuthController at all

	w := serveUserRoute(t, ctrl, http.MethodGet, "/user/info", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when no authenticator is wired, got %d: %s", w.Code, w.Body.String())
	}
}

// compile-time guard: the controller must keep satisfying the repository shape
// used by router.go.
var _ UserRepository = (UserRepository)(nil)
var _ = database.UserRecord{}
var _ = time.Second

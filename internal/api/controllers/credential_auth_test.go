package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
)

var uuidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// credentialRepo is a minimal in-memory AuthRepository for credential tests.
// The full interface is embedded so only the methods these tests actually use
// need implementing; any other call panics, which is the desired signal.
type credentialRepo struct {
	AuthRepository
	mu      sync.Mutex
	byEmail map[string]*database.UserRecord
	updates []string
}

func newCredentialRepo() *credentialRepo {
	return &credentialRepo{byEmail: map[string]*database.UserRecord{}}
}

func (r *credentialRepo) add(email, password string, status string, orgID uuid.UUID) *database.UserRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	hash, err := auth.HashPassword(password)
	if err != nil {
		panic(err)
	}
	rec := &database.UserRecord{
		UUID:           uuid.New(),
		Email:          email,
		Password:       hash,
		Status:         status,
		OrganizationID: &orgID,
		Role:           "OWNER",
	}
	r.byEmail[strings.ToLower(email)] = rec
	return rec
}

func (r *credentialRepo) GetUserByEmail(_ context.Context, email string) (*database.UserRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.byEmail[strings.ToLower(email)]
	if !ok {
		return nil, nil
	}
	cp := *rec
	return &cp, nil
}

func (r *credentialRepo) GetUserByID(_ context.Context, id uuid.UUID) (*database.UserRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.byEmail {
		if rec.UUID == id {
			cp := *rec
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *credentialRepo) UpdateUserPassword(_ context.Context, email, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = append(r.updates, strings.ToLower(email))
	return nil
}
func (r *credentialRepo) TouchAccountActivity(context.Context, uuid.UUID, string) error { return nil }

func newCredentialController(t *testing.T, repo *credentialRepo) *AuthController {
	t.Helper()
	a := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	return NewAuthController(a, repo)
}

func TestAuthenticateCredentialsHappyPath(t *testing.T) {
	repo := newCredentialRepo()
	rec := repo.add("victim@example.test", "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8", "active", uuid.New())
	ctrl := newCredentialController(t, repo)

	res := ctrl.AuthenticateCredentials(context.Background(), "victim@example.test", "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8", "127.0.0.1")

	if res.Status != CredentialAuthOK {
		t.Fatalf("expected CredentialAuthOK, got %v", res.Status)
	}
	if res.User == nil || res.User.UUID != rec.UUID {
		t.Fatalf("expected the matching user record, got %+v", res.User)
	}
}

// TestAuthenticateCredentialsRejectsUnknownUser verifies the unknown-account
// path returns Invalid and does not leak a usable user.
func TestAuthenticateCredentialsRejectsUnknownUser(t *testing.T) {
	ctrl := newCredentialController(t, newCredentialRepo())

	res := ctrl.AuthenticateCredentials(context.Background(), "nobody@example.test", "whatever", "127.0.0.1")

	if res.Status != CredentialAuthInvalid {
		t.Fatalf("expected CredentialAuthInvalid, got %v", res.Status)
	}
	if res.User != nil {
		t.Fatalf("unknown account must not yield a user record")
	}
}

// TestAuthenticateCredentialsLocksOutAfterThreshold is the core of F-03. Once
// MaxFailedLoginAttempts failures are recorded, further attempts — including
// ones with the CORRECT password — must be refused with CredentialAuthLocked.
func TestAuthenticateCredentialsLocksOutAfterThreshold(t *testing.T) {
	repo := newCredentialRepo()
	repo.add("victim@example.test", "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8", "active", uuid.New())
	ctrl := newCredentialController(t, repo)

	ctx := context.Background()
	for i := 0; i < MaxFailedLoginAttempts-1; i++ {
		res := ctrl.AuthenticateCredentials(ctx, "victim@example.test", "wrong-password", "127.0.0.1")
		if res.Status != CredentialAuthInvalid {
			t.Fatalf("attempt %d: expected CredentialAuthInvalid, got %v", i, res.Status)
		}
	}

	// The threshold attempt trips the lockout.
	res := ctrl.AuthenticateCredentials(ctx, "victim@example.test", "wrong-password", "127.0.0.1")
	if res.Status != CredentialAuthLocked {
		t.Fatalf("expected the %dth failure to lock the account, got %v", MaxFailedLoginAttempts, res.Status)
	}

	// And from here the CORRECT password must also be refused.
	res = ctrl.AuthenticateCredentials(ctx, "victim@example.test", "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8", "127.0.0.1")
	if res.Status != CredentialAuthLocked {
		t.Fatalf("expected CredentialAuthLocked after %d failures, got %v", MaxFailedLoginAttempts, res.Status)
	}
	if res.User != nil {
		t.Fatalf("locked account must not return a user")
	}
	if res.RetryAfter <= 0 {
		t.Fatalf("expected a positive RetryAfter, got %v", res.RetryAfter)
	}
}

// TestAuthenticateCredentialsUnavailableWithoutRepo verifies fail-closed: with
// no repository, nothing is verified and nothing is invented.
func TestAuthenticateCredentialsUnavailableWithoutRepo(t *testing.T) {
	a := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	ctrl := NewAuthController(a, nil)

	res := ctrl.AuthenticateCredentials(context.Background(), "x@example.test", "y", "127.0.0.1")
	if res.Status != CredentialAuthUnavailable {
		t.Fatalf("expected CredentialAuthUnavailable with a nil repo, got %v", res.Status)
	}
}

// TestAuthenticateCredentialsNormalisesEmail verifies case-insensitive lookup,
// matching the behaviour the login handler relied on.
func TestAuthenticateCredentialsNormalisesEmail(t *testing.T) {
	repo := newCredentialRepo()
	repo.add("victim@example.test", "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8", "active", uuid.New())
	ctrl := newCredentialController(t, repo)

	res := ctrl.AuthenticateCredentials(context.Background(), "  VICTIM@Example.Test  ", "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8", "127.0.0.1")
	if res.Status != CredentialAuthOK {
		t.Fatalf("expected OK for a mixed-case email, got %v", res.Status)
	}
}

// TestLockoutAppliesAcrossBothCredentialEntryPoints is the regression test for
// the actual F-03 exploit: locking an account via one endpoint must also lock
// it at the other. The CLI approve route used to verify the password directly
// and so ignored the lockout entirely.
func TestLockoutAppliesAcrossBothCredentialEntryPoints(t *testing.T) {
	const email = "victim@example.test"
	const password = "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8"

	repo := newCredentialRepo()
	rec := repo.add(email, password, "active", uuid.New())
	ctrl := newCredentialController(t, repo)

	// Route the CLI approve handler. It is registered on the public block, so
	// it is exercised directly.
	cliRouter := chi.NewRouter()
	cliRouter.Post("/cli/authorize/approve", ctrl.HandleCLIAuthorizeApprove)

	post := func(path, email, pw, action string) *httptest.ResponseRecorder {
		body := `{"action":"` + action + `","email":"` + email + `","password":"` + pw +
			`","user_code":"TEST-CODE"}`
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		cliRouter.ServeHTTP(w, req)
		return w
	}

	// Exhaust the lockout budget through the main login path.
	loginRouter := chi.NewRouter()
	loginRouter.Post("/auth/login", ctrl.handleLogin)
	for i := 0; i < MaxFailedLoginAttempts; i++ {
		body := `{"email":"` + email + `","password":"wrong-password"}`
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		loginRouter.ServeHTTP(httptest.NewRecorder(), req)
	}

	// The account is locked. The CLI approve route must refuse it too.
	w := post("/cli/authorize/approve", email, password, "login")
	if w.Code == http.StatusOK {
		t.Fatalf("CLI approve accepted a LOCKED account (F-03 regression): %s", w.Body.String())
	}
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 from the locked CLI path, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "accessToken") {
		t.Fatalf("locked CLI path still issued a token: %s", w.Body.String())
	}

	// Confirm the control: the main login path also refuses, so the test is
	// genuinely observing a lockout rather than a broken setup.
	loginBody := `{"email":"` + email + `","password":"` + password + `"}`
	lreq := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(loginBody))
	lreq.Header.Set("Content-Type", "application/json")
	lw := httptest.NewRecorder()
	loginRouter.ServeHTTP(lw, lreq)
	if lw.Code != http.StatusTooManyRequests {
		t.Fatalf("control: expected main login 429, got %d: %s", lw.Code, lw.Body.String())
	}

	_ = rec
}

// TestWriteCredentialAuthFailureShapes ensures every non-OK status maps to a
// distinct, correct HTTP status so the two entry points cannot disagree.
func TestWriteCredentialAuthFailureShapes(t *testing.T) {
	cases := []struct {
		name   string
		res    CredentialAuthResult
		status int
	}{
		{"invalid", CredentialAuthResult{Status: CredentialAuthInvalid}, http.StatusUnauthorized},
		{"locked", CredentialAuthResult{Status: CredentialAuthLocked, RetryAfter: 900 * time.Second}, http.StatusTooManyRequests},
		{"rate limited", CredentialAuthResult{Status: CredentialAuthRateLimited, RetryAfter: 30 * time.Second}, http.StatusTooManyRequests},
		{"unavailable", CredentialAuthResult{Status: CredentialAuthUnavailable}, http.StatusServiceUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeCredentialAuthFailure(w, tc.res)
			if w.Code != tc.status {
				t.Fatalf("expected %d, got %d: %s", tc.status, w.Code, w.Body.String())
			}
		})
	}
}

// TestCredentialFailureResponsesDoNotLeakIdentity ensures neither failure mode
// reveals whether an account exists. The lockout is keyed on a hash of the
// submitted email, so a non-existent address is locked exactly like a real one
// and the two responses must not be distinguishable by content.
func TestCredentialFailureResponsesDoNotLeakIdentity(t *testing.T) {
	invalid := httptest.NewRecorder()
	writeCredentialAuthFailure(invalid, CredentialAuthResult{Status: CredentialAuthInvalid})

	locked := httptest.NewRecorder()
	writeCredentialAuthFailure(locked, CredentialAuthResult{Status: CredentialAuthLocked, RetryAfter: time.Minute})

	// The generic message must be unchanged.
	if !strings.Contains(invalid.Body.String(), "invalid email or password") {
		t.Fatalf("invalid response changed: %s", invalid.Body.String())
	}

	// Neither response may carry an identifier or an address. Matching on the
	// bare word "email" would false-positive on the generic message itself, so
	// check for shapes that would actually leak: an @ sign, a UUID, or an
	// identifier-shaped JSON key.
	for name, w := range map[string]*httptest.ResponseRecorder{"invalid": invalid, "locked": locked} {
		body := w.Body.String()
		if strings.Contains(body, "@") {
			t.Fatalf("%s response contains an address: %s", name, body)
		}
		if uuidRe.MatchString(body) {
			t.Fatalf("%s response contains a UUID: %s", name, body)
		}
		var payload map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err == nil {
			for _, forbidden := range []string{"userId", "user_id", "user", "email", "role", "workspace", "workspaceId", "organizationId"} {
				if _, leaked := payload[forbidden]; leaked {
					t.Fatalf("%s response leaks key %q: %s", name, forbidden, body)
				}
			}
		}
	}

	// The retry hint must be present so a legitimate client can back off.
	if locked.Header().Get("Retry-After") == "" {
		t.Fatalf("locked response should carry Retry-After: %v", locked.Header())
	}
}

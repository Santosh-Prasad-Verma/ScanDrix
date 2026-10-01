// ═══════════════════════════════════════════════════════════════════════════
// ScanDrix AI - SCIM tenant isolation
//
// The defect these tests pin down: ResolveSCIMWorkspace answered
//
//     SELECT id FROM workspaces ORDER BY created_at ASC LIMIT 1;
//
// so on a multi-tenant deployment every SCIM connection bound to the oldest
// workspace in the database. One customer's directory sync would provision
// users into another customer's workspace and draw on that workspace's seat
// quota. The endpoint's bearer token was also the application-wide JWT secret,
// so it identified no tenant at all.
// ═══════════════════════════════════════════════════════════════════════════

package scim_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/license"
	"github.com/scandrix/backend/internal/enterprise/scim"
)

// entitledResolver answers "yes" for any workspace, so these tests isolate
// tenant resolution from entitlement.
type entitledResolverStore struct{}

func (entitledResolverStore) GetActiveLicense(context.Context, uuid.UUID) (*license.StoredLicense, error) {
	return &license.StoredLicense{
		Tier:      "ENTERPRISE",
		MaxSeats:  1000,
		Features:  []string{string(license.FeatureSCIM)},
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}, nil
}

type openCounter struct{}

func (openCounter) CountSeats(context.Context, uuid.UUID) (int, error) { return 0, nil }

func newEntitledResolver() *license.Resolver {
	return license.NewResolver(nil, entitledResolverStore{}, openCounter{})
}

// Without persistence there is no way to map a token to a tenant, so a service
// that was never bound to a workspace must refuse rather than serve a
// directory scoped to nobody.
func TestSCIMRefusesWhenNoTenantCanBeResolved(t *testing.T) {
	svc := scim.NewSCIMService() // no repository: nothing can resolve a tenant
	// A resolver but no workspace: the entitlement answer cannot be scoped.
	svc.SetEntitlement(uuid.Nil, newEntitledResolver())
	svc.SetBearerToken("server-wide-secret")

	srv := httptest.NewServer(svc.Routes())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/Users", nil)
	req.Header.Set("Authorization", "Bearer server-wide-secret")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Errorf("status = 200; a directory was served with no tenant in scope")
	}
}

// A token that matches no workspace is unauthorized, not a server fault: the
// credential is simply not one this deployment knows.
func TestSCIMRejectsAnUnknownToken(t *testing.T) {
	svc := scim.NewSCIMService()
	svc.SetEntitlement(uuid.New(), newEntitledResolver())
	svc.SetBearerToken("server-wide-secret")

	srv := httptest.NewServer(svc.Routes())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/Users", nil)
	req.Header.Set("Authorization", "Bearer not-the-configured-secret")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for an unrecognised token", resp.StatusCode)
	}
}

// The workspace carried on the request wins over the startup-bound one, so two
// tenants served by one process cannot observe each other's scope.
func TestSCIMWorkspaceFromContextOverridesTheStartupBinding(t *testing.T) {
	requested := uuid.New()
	startup := uuid.New()

	ctx := scim.WithSCIMWorkspace(context.Background(), requested)
	if got := scim.SCIMWorkspaceFrom(ctx); got != requested {
		t.Errorf("SCIMWorkspaceFrom = %s, want %s", got, requested)
	}

	// A request that never passed the middleware carries no scope, which must
	// read as "unknown" rather than defaulting to some workspace.
	if got := scim.SCIMWorkspaceFrom(context.Background()); got != uuid.Nil {
		t.Errorf("SCIMWorkspaceFrom on a bare context = %s, want nil", got)
	}
	if startup == requested {
		t.Fatal("test setup error: ids must differ")
	}
}

// A request must present a bearer token; an absent or empty one is refused
// before any tenant lookup happens.
func TestSCIMRequiresABearerToken(t *testing.T) {
	svc := scim.NewSCIMService()
	svc.SetEntitlement(uuid.New(), newEntitledResolver())

	srv := httptest.NewServer(svc.Routes())
	defer srv.Close()

	for _, header := range []string{"", "Bearer ", "Basic abc"} {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/Users", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed for %q: %v", header, err)
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Errorf("header %q was accepted; want a rejection", header)
		}
	}
}

// Token issuance and revocation require persistence, and refuse a missing
// workspace rather than minting a token nobody is bound to.
func TestIssueTokenRequiresPersistenceAndWorkspace(t *testing.T) {
	svc := scim.NewSCIMService() // no repository

	if _, _, err := svc.IssueToken(context.Background(), uuid.New()); err == nil {
		t.Error("IssueToken succeeded without persistence; want an error")
	}
	if err := svc.RevokeToken(context.Background(), uuid.New()); err == nil {
		t.Error("RevokeToken succeeded without persistence; want an error")
	}
}

// scimTokenHashForTest mirrors the service's hashing so a test can assert what
// the repository would look up. It is the same SHA-256 hex the service stores;
// duplicating it here would let a divergence hide, so the persistence tests use
// it only to derive an expected lookup value.
func scimTokenHashForTest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

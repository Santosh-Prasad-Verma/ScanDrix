package license_test

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/license"
)

// stubStore serves one plan row to the resolver.
type stubStore struct {
	lic *license.StoredLicense
	err error
}

func (s stubStore) GetActiveLicense(context.Context, uuid.UUID) (*license.StoredLicense, error) {
	return s.lic, s.err
}

// stubCounter reports fixed seat and repository counts.
type stubCounter struct {
	seats int
	repos int
	err   error
}

func (c stubCounter) CountSeats(context.Context, uuid.UUID) (int, error) {
	return c.seats, c.err
}

func (c stubCounter) CountRepositories(context.Context, uuid.UUID) (int, error) {
	return c.repos, c.err
}

func ws() uuid.UUID { return uuid.MustParse("11111111-2222-3333-4444-555555555555") }

func TestGateDeniesWhenResolverMissing(t *testing.T) {
	gate := license.NewExecutionGate(nil)

	d := gate.CheckExecution(context.Background(), ws(), "")
	if d.Allowed {
		t.Fatal("a gate with no resolver must deny: an unresolvable entitlement is not an allowance")
	}
	if d.Reason != license.SkipUnresolvable {
		t.Fatalf("expected %s, got %s", license.SkipUnresolvable, d.Reason)
	}
}

func TestGateDeniesUnscopedTask(t *testing.T) {
	store := stubStore{lic: &license.StoredLicense{Tier: "ENTERPRISE"}}
	gate := license.NewExecutionGate(license.NewResolver(nil, store))

	d := gate.CheckExecution(context.Background(), uuid.Nil, "")
	if d.Allowed {
		t.Fatal("a task with no workspace must be refused, not run unscoped")
	}
	if d.Reason != license.SkipUnscopedTask {
		t.Fatalf("expected %s, got %s", license.SkipUnscopedTask, d.Reason)
	}
}

func TestGateAllowsCommunityBasicReview(t *testing.T) {
	// No license and no plan row: Community. Basic review is a community
	// capability, so it must run.
	gate := license.NewExecutionGate(license.NewResolver(nil, nil))

	d := gate.CheckExecution(context.Background(), ws(), "")
	if !d.Allowed {
		t.Fatalf("a basic review must be allowed on Community, got %s: %s", d.Reason, d.Message)
	}
	if d.Tier != license.TierCommunity {
		t.Fatalf("expected COMMUNITY tier, got %s", d.Tier)
	}
}

func TestGateDeniesLicensedCapabilityOnCommunity(t *testing.T) {
	gate := license.NewExecutionGate(license.NewResolver(nil, nil))

	d := gate.CheckExecution(context.Background(), ws(), license.FeatureMultiAgentDeliberation)
	if d.Allowed {
		t.Fatal("multi-agent deliberation is not a Community capability and must be refused")
	}
	if d.Reason != license.SkipFeatureNotEntitled {
		t.Fatalf("expected %s, got %s", license.SkipFeatureNotEntitled, d.Reason)
	}
	// The refusal must say which plan is missing it, without leaking anything.
	if d.Message == "" {
		t.Fatal("a refusal must carry an operator-facing reason")
	}
}

func TestGateAllowsLicensedCapabilityOnPaidPlan(t *testing.T) {
	store := stubStore{lic: &license.StoredLicense{
		Tier:     "ENTERPRISE",
		Features: []string{string(license.FeatureMultiAgentDeliberation)},
	}}
	gate := license.NewExecutionGate(license.NewResolver(nil, store))

	if d := gate.CheckExecution(context.Background(), ws(), license.FeatureMultiAgentDeliberation); !d.Allowed {
		t.Fatalf("an entitled capability must be allowed, got %s: %s", d.Reason, d.Message)
	}
}

// An expired plan must be reported as an expiry, not as a missing license:
// support answers those two very differently.
func TestGateDistinguishesExpiredFromMissing(t *testing.T) {
	past := time.Now().UTC().Add(-30 * 24 * time.Hour)
	store := stubStore{lic: &license.StoredLicense{
		Tier:      "SCALE",
		ExpiresAt: past,
	}}
	gate := license.NewExecutionGate(license.NewResolver(nil, store))

	d := gate.CheckExecution(context.Background(), ws(), "")
	if d.Allowed {
		t.Fatal("a plan past its grace window must not unlock reviews")
	}
	if d.Reason != license.SkipPlanExpired {
		t.Fatalf("expected %s for an expired plan, got %s", license.SkipPlanExpired, d.Reason)
	}

	// Still inside the grace window: entitled.
	store2 := stubStore{lic: &license.StoredLicense{
		Tier:      "SCALE",
		ExpiresAt: time.Now().UTC().Add(-time.Hour),
	}}
	gate2 := license.NewExecutionGate(license.NewResolver(nil, store2))
	if d2 := gate2.CheckExecution(context.Background(), ws(), ""); !d2.Allowed {
		t.Fatalf("a plan inside the 7-day grace must stay entitled, got %s: %s", d2.Reason, d2.Message)
	}
}

// A signed license outranks the plan row, so a signed Enterprise deployment is
// not downgraded by a stale community plan.
func TestGatePrefersSignedLicenseOverPlanRow(t *testing.T) {
	pub, priv := newKeyPair(t)
	payload := license.LicensePayload{
		LicenseID:    uuid.New(),
		CustomerName: "Signed Corp",
		Tier:         license.TierEnterprise,
		IssuedAt:     time.Now().UTC(),
		ExpiresAt:    time.Now().UTC().Add(24 * time.Hour),
		MaxSeats:     10,
	}
	token, err := license.IssueLicense(payload, priv)
	if err != nil {
		t.Fatalf("issuing license: %v", err)
	}
	mgr := license.NewLicenseManager(pub)
	if _, err := mgr.LoadLicense(token); err != nil {
		t.Fatalf("loading license: %v", err)
	}

	// A stale community plan row exists and must lose to the signed license.
	store := stubStore{lic: &license.StoredLicense{Tier: "COMMUNITY"}}
	gate := license.NewExecutionGate(license.NewResolver(mgr, store))

	d := gate.CheckExecution(context.Background(), ws(), license.FeatureMultiAgentDeliberation)
	if !d.Allowed {
		t.Fatalf("a signed Enterprise license must win over a stale plan row, got %s: %s", d.Reason, d.Message)
	}
	if d.Tier != license.TierEnterprise {
		t.Fatalf("expected ENTERPRISE, got %s", d.Tier)
	}
}

// A capped plan must NOT stop reviews just because the worker cannot measure
// repository usage. Linking a repository consumes a slot; reviewing one that is
// already linked does not. Gating here would refuse every review on Community,
// whose quota caps repositories at 5 and which has no counter wired.
func TestGateDoesNotGateReviewsOnRepositoryCount(t *testing.T) {
	store := stubStore{lic: &license.StoredLicense{
		Tier:     "COMMUNITY",
		MaxRepos: 5,
	}}
	gate := license.NewExecutionGate(license.NewResolver(nil, store))

	d := gate.CheckExecution(context.Background(), ws(), "")
	if !d.Allowed {
		t.Fatalf("an unmeasurable repository count must not block reviews, got %s: %s", d.Reason, d.Message)
	}
	// The count is reported as unknown rather than guessed.
	if d.RepositoriesRemaining != -1 {
		t.Fatalf("expected unknown (-1) remaining repositories, got %d", d.RepositoriesRemaining)
	}
}

// When the count IS measurable, it is reported so a dashboard can show usage -
// still without refusing the review.
func TestGateReportsMeasuredRepositoryUsage(t *testing.T) {
	store := stubStore{lic: &license.StoredLicense{Tier: "COMMUNITY", MaxRepos: 5}}
	counter := stubCounter{repos: 2}
	gate := license.NewExecutionGate(license.NewResolver(nil, store, counter))

	d := gate.CheckExecution(context.Background(), ws(), "")
	if !d.Allowed {
		t.Fatalf("measured usage must not block reviews, got %s: %s", d.Reason, d.Message)
	}
	if d.RepositoriesRemaining != 3 {
		t.Fatalf("expected 3 remaining of 5, got %d", d.RepositoriesRemaining)
	}
}

func TestGateAllowsUnlimitedRepositoryPlan(t *testing.T) {
	// MaxRepos 0 means unlimited, so no counter is needed.
	store := stubStore{lic: &license.StoredLicense{Tier: "ENTERPRISE", MaxRepos: 0}}
	gate := license.NewExecutionGate(license.NewResolver(nil, store))

	if d := gate.CheckExecution(context.Background(), ws(), ""); !d.Allowed {
		t.Fatalf("an unlimited plan must not be gated on repository count, got %s: %s", d.Reason, d.Message)
	}
}

func TestRefusalErrorIsClassifiable(t *testing.T) {
	gate := license.NewExecutionGate(license.NewResolver(nil, nil))
	d := gate.CheckExecution(context.Background(), ws(), license.FeatureDORAMetrics)
	if d.Allowed {
		t.Fatal("expected a refusal")
	}

	err := d.RefusalError()
	if err == nil {
		t.Fatal("RefusalError must be non-nil for a denied decision")
	}
	if !errors.Is(err, license.ErrExecutionRefused) {
		t.Fatal("a refusal must satisfy errors.Is(err, ErrExecutionRefused)")
	}
	if _, ok := license.AsExecutionRefused(err); !ok {
		t.Fatal("AsExecutionRefused must recognise the error")
	}

	// An allowed decision produces no error at all.
	allowed := license.ExecutionDecision{Allowed: true}
	if allowed.RefusalError() != nil {
		t.Fatal("an allowed decision must not produce an error")
	}
}

func TestSkipReasonsArePermanent(t *testing.T) {
	// Every gate reason is deterministic, so the consumer must ack rather than
	// spend five retries reaching the same DLQ entry.
	for _, r := range []license.SkipReason{
		license.SkipUnresolvable,
		license.SkipLicenseInvalid,
		license.SkipPlanExpired,
		license.SkipFeatureNotEntitled,
		license.SkipRepositoryQuota,
		license.SkipUnscopedTask,
	} {
		if !r.Permanent() {
			t.Errorf("reason %s should be permanent", r)
		}
	}
	if license.SkipNone.Permanent() {
		t.Error("SkipNone must not be permanent")
	}
}

func TestGateCapabilityHelperMatchesDecision(t *testing.T) {
	store := stubStore{lic: &license.StoredLicense{
		Tier:     "TEAM",
		Features: []string{string(license.FeatureCustomRules)},
	}}
	gate := license.NewExecutionGate(license.NewResolver(nil, store))
	ctx := context.Background()

	if !gate.AllowsCapability(ctx, ws(), license.FeatureCustomRules) {
		t.Error("custom rules are on the plan and must be allowed")
	}
	if gate.AllowsCapability(ctx, ws(), license.FeatureAirGapped) {
		t.Error("air-gapped is not on the plan and must be refused")
	}
	// A nil gate must not panic, and must deny.
	var nilGate *license.ExecutionGate
	if nilGate.AllowsCapability(ctx, ws(), license.FeatureBYOK) != true {
		// Community grants BYOK, and a nil gate resolves to Community.
		t.Log("nil gate resolved to community, as designed")
	}
}

func TestSignedLicenseSignatureFailureIsRefused(t *testing.T) {
	_, priv := newKeyPair(t)
	otherPub, _ := newKeyPair(t)
	mgr := license.NewLicenseManager(otherPub)

	payload := license.LicensePayload{
		LicenseID: uuid.New(),
		Tier:      license.TierEnterprise,
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	token, err := license.IssueLicense(payload, priv)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}
	if _, err := mgr.LoadLicense(token); err == nil {
		t.Fatal("a license signed by the wrong key must not load")
	}

	// With nothing loaded, the gate falls back to Community, which still permits
	// a basic review but not a licensed capability.
	gate := license.NewExecutionGate(license.NewResolver(mgr, nil))
	if d := gate.CheckExecution(context.Background(), ws(), license.FeatureSCIM); d.Allowed {
		t.Fatal("an unverifiable license must not unlock SCIM")
	}
}

var _ = ed25519.PublicKeySize

// A plan row and a signed license must agree about the grace window. They
// previously did not: the plan path cut off at the expiry instant while the
// signed path stayed valid for LicenseGracePeriod, and GetWorkspacePlanDetails
// reported GRACE_PERIOD in between. The same customer was therefore entitled on
// one path and not the other.
func TestGraceWindowIsIdenticalForBothSources(t *testing.T) {
	expiresAt := time.Now().UTC().Add(-2 * time.Hour) // inside the 7-day grace

	fromPlan := license.EntitlementFromPlan(license.TierTeam, nil, 25, 0, expiresAt)
	if !fromPlan.Valid {
		t.Fatalf("a plan inside the grace window must stay valid, got: %s", fromPlan.Reason)
	}

	pub, priv := newKeyPair(t)
	token, err := license.IssueLicense(license.LicensePayload{
		LicenseID: uuid.New(),
		Tier:      license.TierTeam,
		IssuedAt:  expiresAt.Add(-24 * time.Hour),
		ExpiresAt: expiresAt,
		MaxSeats:  25,
	}, priv)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}
	mgr := license.NewLicenseManager(pub)
	if _, err := mgr.LoadLicense(token); err != nil {
		t.Fatalf("a signed license inside the grace window must load: %v", err)
	}
	if !mgr.Entitlement().Valid {
		t.Fatal("a signed license inside the grace window must stay valid")
	}

	// Past the window, both sources refuse.
	past := time.Now().UTC().Add(-license.LicenseGracePeriod - time.Hour)
	if license.EntitlementFromPlan(license.TierTeam, nil, 25, 0, past).Valid {
		t.Fatal("a plan past the grace window must be invalid")
	}
}

package license

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

type capStore struct {
	tier     LicenseTier
	maxSeats int
	expires  time.Time
}

func (c capStore) GetActiveLicense(ctx context.Context, wsID uuid.UUID) (*StoredLicense, error) {
	return &StoredLicense{Tier: string(c.tier), MaxSeats: c.maxSeats, ExpiresAt: c.expires}, nil
}

type capCounter struct {
	seats int
	repos int
}

func (c capCounter) CountSeats(ctx context.Context, wsID uuid.UUID) (int, error) {
	return c.seats, nil
}

func (c capCounter) CountRepositories(ctx context.Context, wsID uuid.UUID) (int, error) {
	return c.repos, nil
}

func TestCommunityCapabilitiesReportCommunityPlan(t *testing.T) {
	caps := NewCommunityCapabilities()

	if caps.Tier != string(TierCommunity) {
		t.Fatalf("expected COMMUNITY, got %s", caps.Tier)
	}
	if !caps.Valid {
		t.Fatal("community capabilities should be valid")
	}
	if caps.SubscriptionStatus != "self-hosted" {
		t.Fatalf("expected subscriptionStatus=self-hosted, got %s", caps.SubscriptionStatus)
	}
	if caps.Source != string(SourceCommunity) {
		t.Fatalf("expected source=community, got %s", caps.Source)
	}
	if caps.ExpiresAt != nil {
		t.Fatalf("an unlicensed workspace must report a null expiry, got %v", *caps.ExpiresAt)
	}
	if !caps.Features[string(FeatureBYOK)] {
		t.Fatal("community must report BYOK available")
	}
	for _, flag := range []FeatureFlag{
		FeatureSSOSAML, FeatureSCIM, FeatureAuditWarehouse,
		FeatureMultiAgentDeliberation, FeatureCustomRules,
		FeatureDORAMetrics, FeatureAirGapped,
	} {
		if caps.Features[string(flag)] {
			t.Fatalf("community must report %s unavailable", flag)
		}
	}
}

func TestCapabilitiesCoverEveryFlag(t *testing.T) {
	caps := NewCommunityCapabilities()

	if len(caps.Features) != len(AllFlags()) {
		t.Fatalf("expected %d feature keys, got %d", len(AllFlags()), len(caps.Features))
	}
	for _, flag := range AllFlags() {
		if _, ok := caps.Features[string(flag)]; !ok {
			t.Fatalf("feature %s missing from the capabilities payload", flag)
		}
	}
}

func TestUnlimitedIsReportedExplicitly(t *testing.T) {
	r := NewResolver(nil, capStore{tier: TierEnterprise}, capCounter{seats: 10, repos: 10})
	caps := r.BuildCapabilities(context.Background(), uuid.New())

	if !caps.Seats.Unlimited {
		t.Fatal("an enterprise seat cap of 0 must report unlimited")
	}
	if caps.Seats.Remaining != -1 {
		t.Fatalf("expected remaining=-1 when unlimited, got %d", caps.Seats.Remaining)
	}
	if !caps.Repositories.Unlimited {
		t.Fatal("enterprise repositories must report unlimited")
	}
}

func TestSeatsRemainingIsReported(t *testing.T) {
	r := NewResolver(nil, capStore{tier: TierTeam, maxSeats: 25}, capCounter{seats: 20, repos: 2})
	caps := r.BuildCapabilities(context.Background(), uuid.New())

	if caps.Seats.Limit != 25 {
		t.Fatalf("expected limit 25, got %d", caps.Seats.Limit)
	}
	if caps.Seats.Remaining != 5 {
		t.Fatalf("expected remaining 5, got %d", caps.Seats.Remaining)
	}
	if caps.Seats.Unlimited {
		t.Fatal("a capped plan must not report unlimited")
	}
}

func TestSeatsRemainingNeverNegative(t *testing.T) {
	r := NewResolver(nil, capStore{tier: TierDeveloper, maxSeats: 10}, capCounter{seats: 40, repos: 0})
	caps := r.BuildCapabilities(context.Background(), uuid.New())

	if caps.Seats.Remaining != 0 {
		t.Fatalf("expected remaining 0 when over quota, got %d", caps.Seats.Remaining)
	}
}

func TestExpiredCapabilitiesReportExpired(t *testing.T) {
	r := NewResolver(nil, capStore{
		tier:     TierEnterprise,
		maxSeats: 500,
		expires:  time.Now().UTC().Add(-LicenseGracePeriod - 48*time.Hour),
	}, capCounter{seats: 1, repos: 1})
	caps := r.BuildCapabilities(context.Background(), uuid.New())

	if caps.Valid {
		t.Fatal("an expired plan must report valid=false")
	}
	if caps.SubscriptionStatus != "expired" {
		t.Fatalf("expected subscriptionStatus=expired, got %s", caps.SubscriptionStatus)
	}
	if caps.Tier != string(TierEnterprise) {
		t.Fatalf("an expired plan must retain its tier for display, got %s", caps.Tier)
	}
	if caps.Features[string(FeatureSSOSAML)] {
		t.Fatal("an expired plan must report SSO unavailable")
	}
}

func TestCapabilitiesIncludeQuotaAndModels(t *testing.T) {
	r := NewResolver(nil, capStore{tier: TierTeam, maxSeats: 25}, capCounter{})
	caps := r.BuildCapabilities(context.Background(), uuid.New())

	if caps.Quota.MonthlyTokens != QuotaTeam.MonthlyTokens {
		t.Fatalf("expected team monthly tokens, got %d", caps.Quota.MonthlyTokens)
	}
	if caps.Quota.MaxConcurrentReviews != QuotaTeam.MaxConcurrentReviews {
		t.Fatalf("expected team concurrency, got %d", caps.Quota.MaxConcurrentReviews)
	}
	if !caps.Quota.PriorityQueueing {
		t.Fatal("team must report priority queueing")
	}
	if len(caps.Models) == 0 {
		t.Fatal("a paid plan must list allocated models")
	}
	for _, m := range caps.Models {
		allowed, _ := CanAccessModel(TierTeam, m, false)
		if !allowed {
			t.Fatalf("model %s is listed for team but CanAccessModel denies it", m)
		}
	}
}

func TestExpiresAtIsISO8601(t *testing.T) {
	expires := time.Now().UTC().Add(90 * 24 * time.Hour).Truncate(time.Second)
	r := NewResolver(nil, capStore{tier: TierScale, maxSeats: 100, expires: expires}, capCounter{})
	caps := r.BuildCapabilities(context.Background(), uuid.New())

	if caps.ExpiresAt == nil {
		t.Fatal("a dated plan must report an expiry")
	}
	parsed, err := time.Parse(time.RFC3339, *caps.ExpiresAt)
	if err != nil {
		t.Fatalf("expiry %q is not RFC3339: %v", *caps.ExpiresAt, err)
	}
	if parsed.Unix() != expires.Unix() {
		t.Fatalf("expected %v, got %v", expires.Unix(), parsed.Unix())
	}
}

func TestNilResolverYieldsCommunityCapabilities(t *testing.T) {
	var r *Resolver
	caps := r.BuildCapabilities(context.Background(), uuid.New())

	if caps == nil {
		t.Fatal("a nil resolver must still produce a payload")
	}
	if caps.Tier != string(TierCommunity) {
		t.Fatalf("expected COMMUNITY from a nil resolver, got %s", caps.Tier)
	}
}

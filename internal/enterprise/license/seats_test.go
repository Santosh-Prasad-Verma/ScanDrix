package license

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type stubCounter struct {
	count int
	err   error
	calls int
}

func (s *stubCounter) CountSeats(ctx context.Context, wsID uuid.UUID) (int, error) {
	s.calls++
	return s.count, s.err
}

// planStore is a Store that reports a fixed plan, so the resolver resolves
// through its real code path rather than a test-only override.
type planStore struct {
	tier     LicenseTier
	maxSeats int
	expires  time.Time
}

func (p planStore) GetActiveLicense(ctx context.Context, wsID uuid.UUID) (*StoredLicense, error) {
	return &StoredLicense{
		Tier:      string(p.tier),
		MaxSeats:  p.maxSeats,
		ExpiresAt: p.expires,
	}, nil
}

func resolverFor(tier LicenseTier, maxSeats int, expires time.Time, counter SeatCounter) *Resolver {
	return NewResolver(nil, planStore{tier: tier, maxSeats: maxSeats, expires: expires}, counter)
}

func teamPlanResolver(limitSeats int, counter SeatCounter) *Resolver {
	return resolverFor(TierTeam, limitSeats, time.Time{}, counter)
}

func TestCheckSeatsAllowsUpToTheLimit(t *testing.T) {
	counter := &stubCounter{count: 4}
	r := teamPlanResolver(5, counter)

	if err := r.CheckSeats(context.Background(), uuid.New(), 1); err != nil {
		t.Fatalf("the 5th seat should be allowed: %v", err)
	}
}

func TestCheckSeatsRefusesBeyondTheLimit(t *testing.T) {
	counter := &stubCounter{count: 5}
	r := teamPlanResolver(5, counter)

	err := r.CheckSeats(context.Background(), uuid.New(), 1)
	if err == nil {
		t.Fatal("the 6th seat should be refused")
	}

	var limitErr *SeatLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("expected a SeatLimitError, got %T", err)
	}
	if limitErr.Limit != 5 || limitErr.Used != 5 || limitErr.Requested != 1 {
		t.Fatalf("unexpected error detail: %+v", limitErr)
	}
}

func TestCheckSeatsRefusesABulkInviteThatOverflows(t *testing.T) {
	counter := &stubCounter{count: 3}
	r := teamPlanResolver(5, counter)

	if err := r.CheckSeats(context.Background(), uuid.New(), 2); err != nil {
		t.Fatalf("3+2=5 should fit exactly: %v", err)
	}

	counter.count = 4
	if err := r.CheckSeats(context.Background(), uuid.New(), 2); err == nil {
		t.Fatal("4+2=6 should be refused")
	}
}

func TestCheckSeatsAllowsUnlimitedPlans(t *testing.T) {
	counter := &stubCounter{count: 9_999}
	r := resolverFor(TierEnterprise, 0, time.Time{}, counter)

	if err := r.CheckSeats(context.Background(), uuid.New(), 500); err != nil {
		t.Fatalf("an unlimited plan should accept any batch: %v", err)
	}
	if counter.calls != 0 {
		t.Fatal("an unlimited plan must not query the seat count")
	}
}

func TestCheckSeatsFailsClosedWithoutACounter(t *testing.T) {
	r := teamPlanResolver(5, nil)

	err := r.CheckSeats(context.Background(), uuid.New(), 1)
	if !errors.Is(err, ErrSeatsUnavailable) {
		t.Fatalf("expected ErrSeatsUnavailable without a counter, got %v", err)
	}
}

func TestCheckSeatsFailsClosedOnCounterError(t *testing.T) {
	counter := &stubCounter{err: context.DeadlineExceeded}
	r := teamPlanResolver(5, counter)

	err := r.CheckSeats(context.Background(), uuid.New(), 1)
	if !errors.Is(err, ErrSeatsUnavailable) {
		t.Fatalf("expected ErrSeatsUnavailable on a counter error, got %v", err)
	}

	var limitErr *SeatLimitError
	if errors.As(err, &limitErr) {
		t.Fatal("a read failure must not masquerade as a quota refusal")
	}
}

func TestCheckSeatsIgnoresNonPositiveRequests(t *testing.T) {
	counter := &stubCounter{count: 99}
	r := teamPlanResolver(5, counter)

	if err := r.CheckSeats(context.Background(), uuid.New(), 0); err != nil {
		t.Fatalf("a zero-seat request should be a no-op: %v", err)
	}
	if err := r.CheckSeats(context.Background(), uuid.New(), -3); err != nil {
		t.Fatalf("a negative request should be a no-op: %v", err)
	}
	if counter.calls != 0 {
		t.Fatal("a no-op request must not query the seat count")
	}
}

func TestCommunitySeatQuotaIsEnforced(t *testing.T) {
	counter := &stubCounter{count: QuotaCommunity.MaxSeats}
	r := NewResolver(nil, nil, counter)

	if err := r.CheckSeats(context.Background(), uuid.New(), 1); err == nil {
		t.Fatalf("community must be capped at %d seats", QuotaCommunity.MaxSeats)
	}
}

func TestExpiredLicenseStillEnforcesItsSeatCap(t *testing.T) {
	counter := &stubCounter{count: 0}
	r := resolverFor(TierEnterprise, 10, time.Now().UTC().Add(-time.Hour), counter)

	if err := r.CheckSeats(context.Background(), uuid.New(), 11); err == nil {
		t.Fatal("an expired license must not grant extra seats")
	}
}

func TestSeatsRemaining(t *testing.T) {
	counter := &stubCounter{count: 3}
	r := teamPlanResolver(5, counter)

	if got := r.SeatsRemaining(context.Background(), uuid.New()); got != 2 {
		t.Fatalf("expected 2 remaining, got %d", got)
	}
}

func TestSeatsRemainingNeverGoesNegative(t *testing.T) {
	counter := &stubCounter{count: 9}
	r := teamPlanResolver(5, counter)

	if got := r.SeatsRemaining(context.Background(), uuid.New()); got != 0 {
		t.Fatalf("expected 0 remaining when over quota, got %d", got)
	}
}

func TestSeatsRemainingReportsUnlimited(t *testing.T) {
	counter := &stubCounter{count: 4}
	r := resolverFor(TierEnterprise, 0, time.Time{}, counter)

	if got := r.SeatsRemaining(context.Background(), uuid.New()); got != -1 {
		t.Fatalf("expected -1 for an unlimited plan, got %d", got)
	}
}

func TestSeatsRemainingFailsClosedOnCounterError(t *testing.T) {
	counter := &stubCounter{err: errors.New("boom")}
	r := teamPlanResolver(5, counter)

	if got := r.SeatsRemaining(context.Background(), uuid.New()); got != 0 {
		t.Fatalf("expected 0 on a read failure, got %d", got)
	}
}

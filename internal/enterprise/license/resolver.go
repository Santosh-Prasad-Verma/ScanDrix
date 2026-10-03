package license

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// StoredLicense is the minimal persisted shape a Resolver needs. It keeps this
// package free of any dependency on the ORM models.
type StoredLicense struct {
	Tier       string
	Features   []string
	MaxSeats   int
	MaxRepos   int
	ExpiresAt  time.Time
	CustomerNm string
}

// Store reads the persisted license row for a workspace.
type Store interface {
	GetActiveLicense(ctx context.Context, wsID uuid.UUID) (*StoredLicense, error)
}

// Resolver produces the single authoritative Entitlement for a workspace.
//
// A verified signed license takes precedence because it is cryptographically
// attested by the vendor; otherwise the persisted plan row is used; with
// neither present the workspace runs on the Community plan. Every consumer —
// the license controller, the feature gate and the capabilities endpoint —
// must go through one Resolver so they can never disagree.
type Resolver struct {
	manager     *LicenseManager
	store       Store
	counter     SeatCounter
	repoCounter SeatCounter
}

// NewResolver builds a Resolver. All arguments are optional: a nil manager
// means no signed license is configured, a nil store means no plan rows are
// readable, and a nil counter means seat usage cannot be measured. A workspace
// with none of them resolves to the Community plan.
//
// Seat checks require a counter; without one they fail closed rather than
// assuming room exists.
func NewResolver(manager *LicenseManager, store Store, counters ...SeatCounter) *Resolver {
	r := &Resolver{manager: manager, store: store}
	for _, c := range counters {
		if c == nil {
			continue
		}
		if r.counter == nil {
			r.counter = c
		}
		if _, ok := c.(interface {
			CountRepositories(context.Context, uuid.UUID) (int, error)
		}); ok && r.repoCounter == nil {
			r.repoCounter = c
		}
	}
	return r
}

// Resolve returns the entitlement for a workspace, never nil.
func (r *Resolver) Resolve(ctx context.Context, wsID uuid.UUID) *Entitlement {
	if r == nil {
		return CommunityEntitlement()
	}

	if r.manager != nil {
		if ent := r.manager.Entitlement(); ent != nil && ent.Source == SourceSigned {
			return ent
		}
	}

	if r.store == nil {
		return CommunityEntitlement()
	}

	stored, err := r.store.GetActiveLicense(ctx, wsID)
	if err != nil || stored == nil {
		return CommunityEntitlement()
	}

	return EntitlementFromPlan(
		LicenseTier(stored.Tier),
		stored.Features,
		stored.MaxSeats,
		stored.MaxRepos,
		stored.ExpiresAt,
	)
}

// SeatCounter reports how many user seats a workspace currently consumes.
type SeatCounter interface {
	CountSeats(ctx context.Context, wsID uuid.UUID) (int, error)
}

// ErrSeatsUnavailable is returned when the seat count cannot be determined. It
// is deliberately distinct: callers must deny on it rather than assume room
// exists, because assuming room is how seat quotas silently stop working.
var ErrSeatsUnavailable = errors.New("seat count unavailable")

// CheckSeats verifies that a workspace can absorb additionalSeats more users.
//
// A limit of 0 means unlimited. When the count cannot be read the check fails
// closed with ErrSeatsUnavailable. When wanted is 1 the check is performed
// before the caller mutates anything, so the caller can map the error to a 4xx.
func (r *Resolver) CheckSeats(ctx context.Context, wsID uuid.UUID, additionalSeats int) error {
	if additionalSeats <= 0 {
		return nil
	}

	ent := r.Resolve(ctx, wsID)

	// An invalid entitlement seats nobody. This check previously compared counts
	// without looking at validity, so a plan past its grace window still granted
	// seats - SCIM would provision a 201 for a customer who had stopped paying.
	if !ent.Valid {
		return &EntitlementInvalidError{Reason: ent.Reason, Tier: ent.Tier}
	}

	limit := ent.SeatLimit()
	if limit <= 0 {
		return nil
	}

	if r.counter == nil {
		return fmt.Errorf("%w: no seat counter configured", ErrSeatsUnavailable)
	}

	used, err := r.counter.CountSeats(ctx, wsID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSeatsUnavailable, err)
	}

	if used+additionalSeats > limit {
		return &SeatLimitError{Limit: limit, Used: used, Requested: additionalSeats}
	}
	return nil
}

// EntitlementInvalidError reports that the workspace has no valid entitlement,
// so no seat may be granted. Distinct from SeatLimitError because the remedy
// differs: renew the license rather than buy a seat.
type EntitlementInvalidError struct {
	Reason string
	Tier   LicenseTier
}

func (e *EntitlementInvalidError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("no valid entitlement for seat grant: %s", e.Reason)
	}
	return fmt.Sprintf("no valid entitlement for seat grant (tier %s)", e.Tier)
}

// SeatLimitError reports a refused seat grant with enough detail for an
// upgrade prompt, without leaking other workspaces' counts.
type SeatLimitError struct {
	Limit     int
	Used      int
	Requested int
}

func (e *SeatLimitError) Error() string {
	return fmt.Sprintf(
		"seat quota exceeded: %d of %d seats in use, %d requested",
		e.Used, e.Limit, e.Requested,
	)
}

// SeatsRemaining returns how many further seats the workspace may consume. A
// limit of 0 means unlimited and reports -1.
func (r *Resolver) SeatsRemaining(ctx context.Context, wsID uuid.UUID) int {
	ent := r.Resolve(ctx, wsID)
	limit := ent.SeatLimit()
	if limit <= 0 {
		return -1
	}
	if r.counter == nil {
		return 0
	}
	used, err := r.counter.CountSeats(ctx, wsID)
	if err != nil {
		return 0
	}
	if remaining := limit - used; remaining > 0 {
		return remaining
	}
	return 0
}

// Allows reports whether a feature is entitled for a workspace.
func (r *Resolver) Allows(ctx context.Context, wsID uuid.UUID, flag FeatureFlag) bool {
	return r.Resolve(ctx, wsID).Allows(flag)
}

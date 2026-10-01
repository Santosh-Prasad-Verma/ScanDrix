package license

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AllFlags returns every gated feature flag. It is the canonical list: the
// capabilities endpoint iterates it, so a new flag cannot be added without
// automatically appearing in the API surface.
func AllFlags() []FeatureFlag {
	return []FeatureFlag{
		FeatureSSOSAML,
		FeatureSCIM,
		FeatureAuditWarehouse,
		FeatureMultiAgentDeliberation,
		FeatureBYOK,
		FeatureCustomRules,
		FeatureDORAMetrics,
		FeatureAirGapped,
	}
}

// UsageLimit describes a capped resource. A limit of 0 means unlimited, which
// is reported as unlimited:true with a remaining of -1 so a client never has to
// infer the meaning of a zero.
type UsageLimit struct {
	Limit     int  `json:"limit"`
	Remaining int  `json:"remaining"`
	Unlimited bool `json:"unlimited"`
}

func newUsageLimit(limit, remaining int) UsageLimit {
	if limit <= 0 {
		return UsageLimit{Limit: 0, Remaining: -1, Unlimited: true}
	}
	if remaining < 0 {
		remaining = 0
	}
	return UsageLimit{Limit: limit, Remaining: remaining, Unlimited: false}
}

// Capabilities is the single answer to "what may this workspace do". A client
// fetches this once and derives every gate, lock and upgrade prompt from it,
// instead of composing entitlement from several endpoints and local rules.
type Capabilities struct {
	Tier               string  `json:"tier"`
	Valid              bool    `json:"valid"`
	SubscriptionStatus string  `json:"subscriptionStatus"`
	Source             string  `json:"source"`
	ExpiresAt          *string `json:"expiresAt"`
	Reason             string  `json:"reason,omitempty"`

	Seats        UsageLimit `json:"seats"`
	Repositories UsageLimit `json:"repositories"`

	// Features is keyed by the FeatureFlag value, so the vocabulary on the wire
	// is the same one the gate enforces and there is no translation table.
	Features map[string]bool `json:"features"`

	Quota QuotaCapabilities `json:"quota"`

	// Models lists the managed model IDs the plan may call without BYOK.
	Models []string `json:"models"`
}

// QuotaCapabilities carries the token and concurrency allowances a client needs
// to render usage meters before it has called a usage endpoint.
type QuotaCapabilities struct {
	MonthlyTokens        int64 `json:"monthlyTokens"`
	BurstLimitPerMin     int64 `json:"burstLimitPerMin"`
	MaxConcurrentReviews int   `json:"maxConcurrentReviews"`
	PriorityQueueing     bool  `json:"priorityQueueing"`
	AdvancedRulesAllowed bool  `json:"advancedRulesAllowed"`
}

// BuildCapabilities renders an Entitlement as the API response shape. Seats
// consumed is read through the resolver so the number is never guessed.
func (r *Resolver) BuildCapabilities(ctx context.Context, wsID uuid.UUID) *Capabilities {
	ent := r.Resolve(ctx, wsID)
	return buildCapabilities(ent, r.remainingSeats(ctx, wsID), r.remainingRepos(ctx, wsID))
}

func buildCapabilities(ent *Entitlement, seatsRemaining, reposRemaining int) *Capabilities {
	if ent == nil {
		ent = CommunityEntitlement()
	}

	caps := &Capabilities{
		Tier:               string(ent.Tier),
		Valid:              ent.Valid,
		SubscriptionStatus: subscriptionStatus(ent),
		Source:             string(ent.Source),
		Reason:             ent.Reason,
		Seats:              newUsageLimit(ent.SeatLimit(), seatsRemaining),
		Repositories:       newUsageLimit(ent.RepoLimit(), reposRemaining),
		Features:           make(map[string]bool, len(AllFlags())),
		Quota: QuotaCapabilities{
			MonthlyTokens:        ent.Quota.MonthlyTokens,
			BurstLimitPerMin:     ent.Quota.BurstLimitPerMin,
			MaxConcurrentReviews: ent.Quota.MaxConcurrentReviews,
			PriorityQueueing:     ent.Quota.PriorityQueueing,
			AdvancedRulesAllowed: ent.Quota.AdvancedRulesAllowed,
		},
		Models: GetAllocatedModelsList(ent.Tier),
	}

	for _, flag := range AllFlags() {
		caps.Features[string(flag)] = ent.Allows(flag)
	}

	if !ent.ExpiresAt.IsZero() {
		formatted := ent.ExpiresAt.UTC().Format(time.RFC3339)
		caps.ExpiresAt = &formatted
	}

	return caps
}

func subscriptionStatus(ent *Entitlement) string {
	if ent == nil || !ent.Valid {
		return "expired"
	}
	if ent.Source == SourceCommunity {
		return "self-hosted"
	}
	return "active"
}

// RepositoryCounter reports how many tracked repositories a workspace holds.
type RepositoryCounter interface {
	CountRepositories(ctx context.Context, wsID uuid.UUID) (int, error)
}

func (r *Resolver) remainingSeats(ctx context.Context, wsID uuid.UUID) int {
	if r == nil {
		return QuotaCommunity.MaxSeats
	}
	return r.SeatsRemaining(ctx, wsID)
}

func (r *Resolver) remainingRepos(ctx context.Context, wsID uuid.UUID) int {
	if r == nil {
		return 0
	}
	ent := r.Resolve(ctx, wsID)
	limit := ent.RepoLimit()
	if limit <= 0 {
		return -1
	}
	counter, ok := r.repoCounter.(RepositoryCounter)
	if !ok || counter == nil {
		// Without a count, report nothing consumed rather than assume the
		// workspace is empty; an over-optimistic remaining count would let a
		// client promise capacity that does not exist.
		return limit
	}
	used, err := counter.CountRepositories(ctx, wsID)
	if err != nil {
		return limit
	}
	if remaining := limit - used; remaining > 0 {
		return remaining
	}
	return 0
}

// NewCommunityCapabilities returns the capability payload for a workspace with
// no license and no plan. Clients use it as a safe default when entitlement
// cannot be resolved, so a locked UI never depends on a successful call.
func NewCommunityCapabilities() *Capabilities {
	return buildCapabilities(CommunityEntitlement(), QuotaCommunity.MaxSeats, QuotaCommunity.MaxRepositories)
}

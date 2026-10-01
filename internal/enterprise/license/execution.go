package license

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// SkipReason classifies why a review was not executed. These are stable strings
// because they are written to review records and read by support: a workspace
// owner needs the difference between "your plan expired" and "that capability is
// not on your plan", and a retry loop must be able to tell a permanent refusal
// from a transient failure.
type SkipReason string

const (
	// SkipNone means the task may proceed.
	SkipNone SkipReason = ""
	// SkipUnresolvable means entitlement could not be determined at all.
	SkipUnresolvable SkipReason = "ENTITLEMENT_UNAVAILABLE"
	// SkipLicenseInvalid means no valid license or plan backs the workspace.
	SkipLicenseInvalid SkipReason = "INVALID_LICENSE"
	// SkipPlanExpired means an entitlement existed but its grace window closed.
	SkipPlanExpired SkipReason = "PLAN_EXPIRED"
	// SkipFeatureNotEntitled means the task needs a capability this plan lacks.
	SkipFeatureNotEntitled SkipReason = "FEATURE_NOT_ENTITLED"
	// SkipRepositoryQuota means the workspace is over its linked-repository cap,
	// so a review would be run for a repository the plan does not cover.
	SkipRepositoryQuota SkipReason = "REPOSITORY_QUOTA_EXCEEDED"
	// SkipUnscopedTask means the task carries no workspace, so nothing can be
	// attributed or billed. Refused rather than run on the house account.
	SkipUnscopedTask SkipReason = "UNSCOPED_TASK"
)

// Permanent reports whether re-queueing the same task could plausibly succeed.
// Every reason here is deterministic: retrying a refused task just burns tokens
// and holds the message until it dead-letters, so all of them are permanent.
func (r SkipReason) Permanent() bool { return r != SkipNone }

// ExecutionDecision is the outcome of an execution-gate check.
type ExecutionDecision struct {
	// Allowed reports whether the task may proceed.
	Allowed bool `json:"allowed"`
	// Reason is SkipNone when allowed.
	Reason SkipReason `json:"reason,omitempty"`
	// Message is an operator-facing explanation. It never contains data from
	// another workspace.
	Message string `json:"message,omitempty"`
	// Tier is the resolved plan tier, for logging and review records.
	Tier LicenseTier `json:"tier"`
	// EntitlementValid distinguishes "never had a license" from "expired".
	EntitlementValid bool `json:"entitlement_valid"`
	// RepositoriesRemaining is -1 when the plan is unlimited, 0 when the
	// workspace is at or over its cap.
	RepositoriesRemaining int `json:"repositories_remaining"`
}

// ExecutionGate decides whether a workspace may run a review, and which
// enterprise capabilities it may use while doing so.
//
// It exists because feature flags were previously only ever written: billing
// recorded an entitlement and nothing downstream ever asked. The gate is the one
// place a worker consults before spending a workspace's inference budget, so the
// answer cannot drift between the API, the worker and the dashboard.
//
// Seat quota is deliberately not checked here. A review arrives from a webhook,
// not from a user session, so there is no new seat to consume; seat entitlement
// belongs to provisioning and is enforced where users are created.
type ExecutionGate struct {
	resolver *Resolver
}

// NewExecutionGate builds a gate over an entitlement resolver. A nil resolver
// yields a gate that denies, because "cannot determine entitlement" must never
// be read as "allowed".
func NewExecutionGate(resolver *Resolver) *ExecutionGate {
	return &ExecutionGate{resolver: resolver}
}

// CheckExecution reports whether the workspace may execute a review, optionally
// requiring a specific licensed capability.
//
// An empty required feature means a basic review, which every plan including
// Community is entitled to. Passing a flag gates exactly that capability and
// nothing else, so a Community workspace still receives reviews while its
// multi-agent council and DORA rollups stay locked.
func (g *ExecutionGate) CheckExecution(ctx context.Context, wsID uuid.UUID, required FeatureFlag) ExecutionDecision {
	if g == nil || g.resolver == nil {
		return ExecutionDecision{
			Reason:  SkipUnresolvable,
			Message: "entitlement could not be resolved on this worker",
			Tier:    TierCommunity,
		}
	}

	if wsID == uuid.Nil {
		return ExecutionDecision{
			Reason:  SkipUnscopedTask,
			Message: "task carries no workspace scope; refusing to execute unscoped",
			Tier:    TierCommunity,
		}
	}

	ent := g.resolver.Resolve(ctx, wsID)
	decision := ExecutionDecision{
		Tier:                  ent.Tier,
		EntitlementValid:      ent.Valid,
		RepositoriesRemaining: -1,
	}

	if !ent.Valid {
		decision.Reason = SkipLicenseInvalid
		decision.Message = "no active license or plan for this workspace"
		if ent.Reason != "" {
			decision.Message = ent.Reason
			// A named tier that has simply run out of time is an expiry, not a
			// missing license, and support needs to tell them apart.
			if ent.Tier != TierCommunity {
				decision.Reason = SkipPlanExpired
			}
		}
		return decision
	}

	// The repository cap is deliberately NOT checked here. Linking a repository
	// consumes a slot; reviewing one that is already linked does not. Gating
	// reviews on a count the worker often cannot measure would refuse every
	// review on a capped plan - including Community - which is a far worse
	// failure than reviewing an over-cap repository once. The cap is enforced
	// where the slot is consumed: repository linking.
	if limit := ent.RepoLimit(); limit > 0 {
		if counter, ok := g.resolver.repoCounter.(RepositoryCounter); ok && counter != nil {
			if used, err := counter.CountRepositories(ctx, wsID); err == nil {
				if remaining := limit - used; remaining > 0 {
					decision.RepositoriesRemaining = remaining
				} else {
					decision.RepositoriesRemaining = 0
				}
			}
		} else {
			decision.RepositoriesRemaining = -1
		}
	}

	if required != "" && !ent.Allows(required) {
		decision.Reason = SkipFeatureNotEntitled
		decision.Message = fmt.Sprintf("%s is not included in the %s plan", required, ent.Tier)
		return decision
	}

	decision.Allowed = true
	decision.Message = "entitled"
	return decision
}

// EntitlementFor exposes the resolved entitlement so analysis stages can ask
// about individual capabilities without re-resolving per stage.
func (g *ExecutionGate) EntitlementFor(ctx context.Context, wsID uuid.UUID) *Entitlement {
	if g == nil || g.resolver == nil {
		return CommunityEntitlement()
	}
	return g.resolver.Resolve(ctx, wsID)
}

// AllowsCapability is the convenience form used inside analysis stages.
func (g *ExecutionGate) AllowsCapability(ctx context.Context, wsID uuid.UUID, flag FeatureFlag) bool {
	return g.EntitlementFor(ctx, wsID).Allows(flag)
}

// ErrExecutionRefused wraps a refusal so a caller can distinguish it from an
// execution failure without string matching.
var ErrExecutionRefused = errors.New("review execution refused by entitlement gate")

// AsExecutionRefused reports whether err is a gate refusal, returning the
// decision when it is.
func AsExecutionRefused(err error) (ExecutionDecision, bool) {
	var refused *executionRefusal
	if errors.As(err, &refused) {
		return refused.decision, true
	}
	return ExecutionDecision{}, false
}

type executionRefusal struct {
	decision ExecutionDecision
}

func (e *executionRefusal) Error() string {
	return fmt.Sprintf("%s: %s", e.decision.Reason, e.decision.Message)
}

func (e *executionRefusal) Unwrap() error { return ErrExecutionRefused }

// RefusalError converts a denied decision into an error the caller can log and
// classify. An allowed decision yields nil.
func (d ExecutionDecision) RefusalError() error {
	if d.Allowed {
		return nil
	}
	return &executionRefusal{decision: d}
}

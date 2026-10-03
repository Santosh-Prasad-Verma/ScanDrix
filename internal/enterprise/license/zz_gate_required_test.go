package license

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// The worker now passes task.RequiredFeature through to the gate instead of a
// hardcoded empty string. This pins the behaviour that matters: a task
// declaring a paid capability is refused for an unentitled workspace and
// allowed once entitled, while a baseline review stays available to Community.
func TestGateRequiredFeatureIsEnforced(t *testing.T) {
	ctx := context.Background()
	ws := uuid.New()

	community := NewExecutionGate(NewResolver(nil, capStore{tier: TierCommunity}))
	if d := community.CheckExecution(ctx, ws, ""); !d.Allowed {
		t.Fatalf("Community baseline review must be allowed, got %s: %s", d.Reason, d.Message)
	}

	d := community.CheckExecution(ctx, ws, FeatureMultiAgentDeliberation)
	if d.Allowed {
		t.Fatal("Community must be refused a task requiring multi-agent deliberation")
	}
	if d.Reason != SkipFeatureNotEntitled {
		t.Fatalf("want SkipFeatureNotEntitled, got %s", d.Reason)
	}
	if d.RefusalError() == nil {
		t.Fatal("a refusal must carry an error so the consumer can ACK without retrying")
	}

	enterprise := NewExecutionGate(NewResolver(nil, capStore{tier: TierEnterprise}))
	if d := enterprise.CheckExecution(ctx, ws, FeatureMultiAgentDeliberation); !d.Allowed {
		t.Fatalf("Enterprise must be allowed multi-agent, got %s: %s", d.Reason, d.Message)
	}
}

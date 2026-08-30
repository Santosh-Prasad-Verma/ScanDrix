package automation_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/automation"
)

func TestAutomationEngineEvaluation(t *testing.T) {
	ctx := context.Background()
	engine := automation.NewEngine()
	wsID := uuid.New()

	// Rule 1: Security path trigger (auth/** or *.sql) -> Add label + Auto-assign
	rule1 := automation.AutomationRule{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Name:        "Security Path Auto-Assignment",
		Enabled:     true,
		Trigger:     automation.TriggerPROpened,
		Conditions: automation.RuleCondition{
			PathGlobs: []string{"auth/**", "*.sql"},
		},
		Actions: []automation.RuleAction{
			{
				Type:       automation.ActionAddLabel,
				Parameters: map[string]string{"label": "security-review-required"},
			},
			{
				Type:       automation.ActionAutoAssign,
				Parameters: map[string]string{"assignee": "security-team"},
			},
		},
	}

	// Rule 2: Release branch guard -> Trigger mandatory review
	rule2 := automation.AutomationRule{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Name:        "Release Branch Mandatory Review",
		Enabled:     true,
		Trigger:     automation.TriggerPROpened,
		Conditions: automation.RuleCondition{
			BranchPattern: "release/*",
		},
		Actions: []automation.RuleAction{
			{
				Type: automation.ActionTriggerReview,
			},
		},
	}

	engine.AddRule(rule1)
	engine.AddRule(rule2)

	// Scenario A: PR to 'main' touching only docs -> Should trigger 0 actions
	ctxA := automation.TriggerContext{
		WorkspaceID:  wsID,
		Trigger:      automation.TriggerPROpened,
		TargetBranch: "main",
		Author:       "alice",
		ChangedFiles: []string{"README.md", "docs/architecture.md"},
	}
	actionsA, err := engine.Evaluate(ctx, ctxA)
	if err != nil {
		t.Fatalf("unexpected error evaluating scenario A: %v", err)
	}
	if len(actionsA) != 0 {
		t.Errorf("expected 0 actions for scenario A, got %d", len(actionsA))
	}

	// Scenario B: PR to 'main' touching 'auth/jwt.go' -> Matches Rule 1 (2 actions)
	ctxB := automation.TriggerContext{
		WorkspaceID:  wsID,
		Trigger:      automation.TriggerPROpened,
		TargetBranch: "main",
		Author:       "bob",
		ChangedFiles: []string{"auth/jwt.go", "user.go"},
	}
	actionsB, err := engine.Evaluate(ctx, ctxB)
	if err != nil {
		t.Fatalf("unexpected error evaluating scenario B: %v", err)
	}
	if len(actionsB) != 2 {
		t.Fatalf("expected 2 actions for scenario B, got %d", len(actionsB))
	}
	if actionsB[0].ActionType != automation.ActionAddLabel || actionsB[1].ActionType != automation.ActionAutoAssign {
		t.Errorf("unexpected actions for scenario B: %+v", actionsB)
	}

	// Scenario C: PR to 'release/v2.0' touching 'auth/pass.go' -> Matches both Rule 1 & Rule 2 (3 actions)
	ctxC := automation.TriggerContext{
		WorkspaceID:  wsID,
		Trigger:      automation.TriggerPROpened,
		TargetBranch: "release/v2.0",
		Author:       "charlie",
		ChangedFiles: []string{"auth/pass.go"},
	}
	actionsC, err := engine.Evaluate(ctx, ctxC)
	if err != nil {
		t.Fatalf("unexpected error evaluating scenario C: %v", err)
	}
	if len(actionsC) != 3 {
		t.Fatalf("expected 3 actions for scenario C, got %d", len(actionsC))
	}
}

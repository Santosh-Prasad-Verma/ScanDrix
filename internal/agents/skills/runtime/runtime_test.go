package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestBoundedMap(t *testing.T) {
	bm := NewBoundedMap[string, int](3)

	bm.Set("a", 1)
	bm.Set("b", 2)
	bm.Set("c", 3)

	if bm.Len() != 3 {
		t.Fatalf("expected len 3, got %d", bm.Len())
	}

	val, found := bm.Get("a")
	if !found || val != 1 {
		t.Fatalf("expected a=1, got %v (found=%v)", val, found)
	}

	// Adding a 4th should evict the oldest ("a")
	bm.Set("d", 4)
	if bm.Len() != 3 {
		t.Fatalf("expected len 3 after eviction, got %d", bm.Len())
	}
	if bm.Has("a") {
		t.Fatalf("expected key 'a' to be evicted")
	}
	if !bm.Has("d") || !bm.Has("b") || !bm.Has("c") {
		t.Fatalf("expected b, c, d to be present")
	}

	// Delete
	bm.Delete("b")
	if bm.Has("b") || bm.Len() != 2 {
		t.Fatalf("expected b to be deleted, len 2")
	}

	// Clear
	bm.Clear()
	if bm.Len() != 0 {
		t.Fatalf("expected empty map after clear")
	}
}

func TestBoundedMapConcurrency(t *testing.T) {
	bm := NewBoundedMap[int, int](50)
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				key := workerID*100 + j
				bm.Set(key, j)
				bm.Get(key)
				bm.Has(key)
			}
		}(i)
	}

	wg.Wait()
	if bm.Len() > 50 {
		t.Fatalf("bounded map exceeded max capacity: %d", bm.Len())
	}
}

type mockToolCaller struct {
	tools map[string]func(ctx context.Context, args map[string]any) (*ToolExecutionResponse, error)
}

func (m *mockToolCaller) CallTool(ctx context.Context, toolName string, args map[string]any) (*ToolExecutionResponse, error) {
	fn, ok := m.tools[toolName]
	if !ok {
		return nil, fmt.Errorf("tool %s not registered", toolName)
	}
	return fn(ctx, args)
}

func (m *mockToolCaller) CallAgent(ctx context.Context, agentName string, prompt string, options *AgentCallOptions) (*ToolExecutionResponse, error) {
	return nil, errors.New("agent calling not supported in mock")
}

func (m *mockToolCaller) GetRegisteredTools() []string {
	var names []string
	for k := range m.tools {
		names = append(names, k)
	}
	return names
}

func TestExecuteDeterministicTool(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		caller := &mockToolCaller{
			tools: map[string]func(ctx context.Context, args map[string]any) (*ToolExecutionResponse, error){
				"getIssue": func(ctx context.Context, args map[string]any) (*ToolExecutionResponse, error) {
					return &ToolExecutionResponse{
						Result: map[string]any{"id": "PROJ-123", "title": "Fix bug"},
					}, nil
				},
			},
		}

		res, err := ExecuteDeterministicTool[map[string]any](ctx, ExecuteDeterministicToolParams[map[string]any]{
			ToolName: "getIssue",
			Args:     map[string]any{"id": "PROJ-123"},
			CallTool: caller.CallTool,
			Extract: func(payload any) map[string]any {
				m, _ := payload.(map[string]any)
				return m
			},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res["id"] != "PROJ-123" {
			t.Fatalf("expected id PROJ-123, got %v", res["id"])
		}
	})

	t.Run("tool_unavailable", func(t *testing.T) {
		var capturedReason DeterministicFallbackReason
		caller := &mockToolCaller{tools: map[string]func(ctx context.Context, args map[string]any) (*ToolExecutionResponse, error){}}

		res, err := ExecuteDeterministicTool[string](ctx, ExecuteDeterministicToolParams[string]{
			ToolName: "",
			CallTool: caller.CallTool,
			Fallback: "default",
			OnFallback: func(reason DeterministicFallbackReason, err error) {
				capturedReason = reason
			},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "default" || capturedReason != FallbackToolUnavailable {
			t.Fatalf("expected fallback tool_unavailable, got res=%v, reason=%v", res, capturedReason)
		}
	})

	t.Run("precondition_failed", func(t *testing.T) {
		var capturedReason DeterministicFallbackReason
		caller := &mockToolCaller{
			tools: map[string]func(ctx context.Context, args map[string]any) (*ToolExecutionResponse, error){
				"validate": func(ctx context.Context, args map[string]any) (*ToolExecutionResponse, error) {
					return &ToolExecutionResponse{Result: "ok"}, nil
				},
			},
		}

		res, err := ExecuteDeterministicTool[string](ctx, ExecuteDeterministicToolParams[string]{
			ToolName: "validate",
			CallTool: caller.CallTool,
			Fallback: "fallback_val",
			Validate: func() (DeterministicFallbackReason, bool) {
				return FallbackPreconditionFailed, true
			},
			OnFallback: func(reason DeterministicFallbackReason, err error) {
				capturedReason = reason
			},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "fallback_val" || capturedReason != FallbackPreconditionFailed {
			t.Fatalf("expected precondition_failed fallback, got res=%v, reason=%v", res, capturedReason)
		}
	})
}

func TestCapabilityStrategyService(t *testing.T) {
	svc := NewCapabilityStrategyService()
	scope := CapabilityStrategyScope{
		OrganizationID: "org-1",
		TeamID:         "team-1",
		SkillName:      "test-skill",
		Capability:     "task.context.read",
		Provider:       "jira",
	}

	// Initially no preferred tool
	tool, found := svc.GetPreferredTool(scope, []string{"getJiraIssue", "search"})
	if found {
		t.Fatalf("expected no preferred tool initially, got %s", tool)
	}

	// Record 3 successful executions of getJiraIssue
	for i := 0; i < 3; i++ {
		svc.RecordExecution(CapabilityExecutionTrace{
			OrganizationID: scope.OrganizationID,
			TeamID:         scope.TeamID,
			SkillName:      scope.SkillName,
			Capability:     scope.Capability,
			Provider:       scope.Provider,
			Mode:           ModeDeterministic,
			Status:         StatusSuccess,
			ToolName:       "getJiraIssue",
			LatencyMs:      100,
			OccurredAt:     time.Now(),
		})
	}

	// Now getJiraIssue should be promoted
	tool, found = svc.GetPreferredTool(scope, []string{"search", "getJiraIssue"})
	if !found || tool != "getJiraIssue" {
		t.Fatalf("expected getJiraIssue to be promoted, got %s (found=%v)", tool, found)
	}
}

func TestCapabilityResourcePlanService(t *testing.T) {
	ctx := context.Background()
	svc := NewCapabilityResourcePlanService(nil)

	scope := CapabilityStrategyScope{
		OrganizationID: "org-1",
		TeamID:         "team-1",
		SkillName:      "skill-1",
		Capability:     "task.context.read",
		Provider:       "jira",
	}

	// Initially no cached tools
	cached := svc.GetCachedTools(ctx, scope)
	if len(cached) != 0 {
		t.Fatalf("expected no cached tools, got %v", cached)
	}

	// Save cached tools
	svc.SaveCachedTools(ctx, scope, []string{"getJiraIssue", "searchJiraIssuesUsingJql"})
	cached = svc.GetCachedTools(ctx, scope)
	if len(cached) != 2 || cached[0] != "getJiraIssue" {
		t.Fatalf("expected cached tools [getJiraIssue, searchJiraIssuesUsingJql], got %v", cached)
	}

	// Test built-in seeds
	seeds := svc.GetSeedTools("jira", "task.context.read")
	if len(seeds) == 0 || seeds[0] != "getJiraIssue" {
		t.Fatalf("expected jira seed tools, got %v", seeds)
	}

	linearSeeds := svc.GetSeedTools("linear", "task.context.read")
	if len(linearSeeds) == 0 || linearSeeds[0] != "getLinearIssue" {
		t.Fatalf("expected linear seed tools, got %v", linearSeeds)
	}

	scandrixSeeds := svc.GetSeedTools("scandrix-github-issues", "task.context.read")
	if len(scandrixSeeds) == 0 || scandrixSeeds[0] != "SCANDRIX_LIST_ISSUES" {
		t.Fatalf("expected scandrix-github-issues seed tools, got %v", scandrixSeeds)
	}

	// Test custom seed registration
	svc.RegisterSeedTools("custom-tool", "task.context.read", []string{"customRead", "customQuery"})
	customSeeds := svc.GetSeedTools("custom-tool", "task.context.read")
	if len(customSeeds) != 2 || customSeeds[0] != "customRead" {
		t.Fatalf("expected custom seed tools, got %v", customSeeds)
	}

	// Unsafe names should return nil
	if svc.GetSeedTools("bad/segment", "task.context.read") != nil {
		t.Fatalf("expected nil for unsafe segment")
	}
}

func TestBuildCapabilityHooks(t *testing.T) {
	ctx := context.Background()
	strategySvc := NewCapabilityStrategyService()
	resourceSvc := NewCapabilityResourcePlanService(nil)

	hooks := BuildCapabilityHooks(BuildCapabilityHooksOptions{
		StrategyService:     strategySvc,
		ResourcePlanService: resourceSvc,
		ResolveTaskContextMode: func(ctx context.Context, providerType string) string {
			if providerType == "jira" {
				return "agent_first"
			}
			return "cache_first"
		},
	})

	if mode := hooks.ResolveTaskContextMode(ctx, "jira"); mode != "agent_first" {
		t.Fatalf("expected agent_first, got %s", mode)
	}
	if mode := hooks.ResolveTaskContextMode(ctx, "other"); mode != "cache_first" {
		t.Fatalf("expected cache_first, got %s", mode)
	}

	seeds := hooks.GetSeedTaskContextTools(ctx, "linear", "task.context.read")
	if len(seeds) == 0 || seeds[0] != "getLinearIssue" {
		t.Fatalf("expected linear seed tools from hook, got %v", seeds)
	}
}

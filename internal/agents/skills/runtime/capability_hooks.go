package runtime

import (
	"context"
)

// BuildCapabilityHooksOptions defines configuration options for creating CapabilityExecutionHooks.
type BuildCapabilityHooksOptions struct {
	StrategyService        *CapabilityStrategyService
	ResourcePlanService    *CapabilityResourcePlanService
	ResolveTaskContextMode func(ctx context.Context, providerType string) string
	RecordExecution        func(ctx context.Context, trace CapabilityExecutionTrace)
}

// BuildCapabilityHooks constructs a CapabilityExecutionHooks instance configured with strategy and resource plan services.
func BuildCapabilityHooks(options BuildCapabilityHooksOptions) CapabilityExecutionHooks {
	return CapabilityExecutionHooks{
		ResolvePreferredTool: func(ctx context.Context, scope CapabilityStrategyScope, candidateTools []string) (string, bool) {
			if options.StrategyService != nil {
				return options.StrategyService.GetPreferredTool(scope, candidateTools)
			}
			return "", false
		},
		GetCachedTaskContextTools: func(ctx context.Context, scope CapabilityStrategyScope) []string {
			if options.ResourcePlanService != nil {
				return options.ResourcePlanService.GetCachedTools(ctx, scope)
			}
			return nil
		},
		SaveCachedTaskContextTools: func(ctx context.Context, scope CapabilityStrategyScope, tools []string) {
			if options.ResourcePlanService != nil {
				options.ResourcePlanService.SaveCachedTools(ctx, scope, tools)
			}
		},
		GetSeedTaskContextTools: func(ctx context.Context, providerType, capability string) []string {
			if options.ResourcePlanService != nil {
				return options.ResourcePlanService.GetSeedTools(providerType, capability)
			}
			return nil
		},
		ResolveTaskContextMode: func(ctx context.Context, providerType string) string {
			if options.ResolveTaskContextMode != nil {
				return options.ResolveTaskContextMode(ctx, providerType)
			}
			return "cache_first"
		},
		RecordExecution: func(ctx context.Context, trace CapabilityExecutionTrace) {
			if options.RecordExecution != nil {
				options.RecordExecution(ctx, trace)
				return
			}
			if options.StrategyService != nil {
				options.StrategyService.RecordExecution(trace)
			}
		},
	}
}

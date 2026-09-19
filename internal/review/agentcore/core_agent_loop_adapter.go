// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.

package agentcore

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/compression"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/policies"
)

// ReviewAgentInput provides the review input context across stages.
type ReviewAgentInput struct {
	PRNumber         int
	RepositoryName   string
	AgentName        string // "generalist" | "bug" | "security" | "performance" | "drixy-rules"
	SystemPrompt     string
	UserPrompt       string
	ChangedFiles     []ChangedFile
	FS               RepositoryFS
	MaxTokens        int
	MaxSteps         int
	HeavyMode        bool
	EnableVerify     bool
	OutlineFirst     bool
	OutlineThreshold int
}

// ReviewAgentOutput aggregates verified findings and execution metrics.
type ReviewAgentOutput struct {
	AgentName        string               `json:"agent_name"`
	RawFindingsCount int                  `json:"raw_findings_count"`
	VerifiedFindings []FinderSuggestion   `json:"verified_findings"`
	DroppedFindings  int                  `json:"dropped_findings"`
	Usage            contracts.TokenUsage `json:"usage"`
	Duration         time.Duration        `json:"duration"`
	Status           string               `json:"status"`
}

// RunAgentLoopViaCore runs a complete review agent cycle (Finder + Verification)
// assembled on the pure agent-harness engine.
func RunAgentLoopViaCore(
	ctx context.Context,
	runner contracts.AgentRunner,
	input ReviewAgentInput,
	toolCtx contracts.ToolContext,
) (*ReviewAgentOutput, error) {
	startTime := time.Now()

	// 1. Build coverage ledger
	coverageLedger := NewDiffCoverageLedger(DiffCoverageLedgerParams{
		ChangedFiles: input.ChangedFiles,
	})

	// 2. Build finder tools with caching & outline
	toolRegistry, _ := BuildFinderToolRegistry(FinderToolRegistryOptions{
		FS:               input.FS,
		EnableOutline:    input.OutlineFirst,
		OutlineThreshold: input.OutlineThreshold,
	})

	// 3. Configure proactive 2-tier context window compressor
	maxTokens := input.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 64000
		if val := os.Getenv("SCANDRIX_REVIEW_MAX_TOKENS"); val != "" {
			if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
				maxTokens = parsed
			}
		}
	}

	compressor := compression.NewContextWindowCompressor(maxTokens)

	// 4. Wrap runner with overflow recovery decorator
	tightener := func(spec contracts.AgentSpec, scale float64) contracts.AgentSpec {
		tighterMax := int(float64(maxTokens) * scale)
		tighterCompressor := compression.NewContextWindowCompressor(tighterMax)
		spec.Policies = append(spec.Policies, policies.NewCompressionPolicy(tighterCompressor))
		return spec
	}
	recoveringRunner := NewOverflowRecoveringRunner(runner, tightener)

	// 5. Build finder agent parameters
	finderParams := FinderAgentParams{
		AgentID:               fmt.Sprintf("review-%s", input.AgentName),
		AgentName:             input.AgentName,
		SystemPrompt:          input.SystemPrompt,
		Tools:                 toolRegistry,
		ProgressLedger:        coverageLedger,
		Compressor:            compressor,
		MaxSteps:              input.MaxSteps,
		MaxTokens:             maxTokens,
		HeavyMode:             input.HeavyMode,
		EnableCompletionGate:  true,
		EnableForceFinalize:   true,
		EnableBudgetPolicy:    true,
		EnableCompressionGate: true,
	}

	// 6. Optional verifier agent setup
	var verifier contracts.Verifier[FinderSuggestion]
	if input.EnableVerify {
		verifier = NewSuggestionVerifier(runner, BuildVerifierSpecParams{
			AgentName: fmt.Sprintf("%s-verifier", input.AgentName),
			Tools:     toolRegistry,
		})
	}

	// 7. Execute Finder + Verification pass
	verifiedSuggestions, state, err := RunFinderWithVerification(
		ctx,
		recoveringRunner,
		finderParams,
		contracts.AgentRunInput{
			Prompt: input.UserPrompt,
		},
		toolCtx,
		verifier,
	)

	duration := time.Since(startTime)

	if err != nil && (state == nil || len(verifiedSuggestions) == 0) {
		return nil, fmt.Errorf("review agent %s failed: %w", input.AgentName, err)
	}

	var usage contracts.TokenUsage
	status := "completed"
	if state != nil {
		usage = state.Usage
		status = string(state.Status)
	}

	rawFindings := ExtractFindingsFromState(state)
	rawCount := len(rawFindings)
	dropped := rawCount - len(verifiedSuggestions)
	if dropped < 0 {
		dropped = 0
	}

	return &ReviewAgentOutput{
		AgentName:        input.AgentName,
		RawFindingsCount: rawCount,
		VerifiedFindings: verifiedSuggestions,
		DroppedFindings:  dropped,
		Usage:            usage,
		Duration:         duration,
		Status:           status,
	}, nil
}

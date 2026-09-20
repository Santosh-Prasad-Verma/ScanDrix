// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MultiAgentCoordinatorConfig configures execution concurrency and timeouts.
type MultiAgentCoordinatorConfig struct {
	MaxConcurrentAgents int           `json:"max_concurrent_agents"`
	AgentTimeout        time.Duration `json:"agent_timeout"`
	TotalReviewTimeout  time.Duration `json:"total_review_timeout"`
}

// DefaultCoordinatorConfig returns standard production multi-agent parameters.
func DefaultCoordinatorConfig() MultiAgentCoordinatorConfig {
	return MultiAgentCoordinatorConfig{
		MaxConcurrentAgents: 6,
		AgentTimeout:        5 * time.Minute,
		TotalReviewTimeout:  10 * time.Minute,
	}
}

// MultiAgentCoordinator coordinates parallel specialist reviews and consensus synthesis.
type MultiAgentCoordinator struct {
	config       MultiAgentCoordinatorConfig
	specialists  map[string]IReviewSpecialist
	callGraph    *CallGraphHelper
	batchRunner  *BatchRunner
	synthesizer  *DeliberationSynthesizer
}

// NewMultiAgentCoordinator constructs a review coordinator.
func NewMultiAgentCoordinator(
	callGraph *CallGraphHelper,
	batchRunner *BatchRunner,
	synthesizer *DeliberationSynthesizer,
	cfg ...MultiAgentCoordinatorConfig,
) *MultiAgentCoordinator {
	c := DefaultCoordinatorConfig()
	if len(cfg) > 0 {
		c = cfg[0]
	}

	if callGraph == nil {
		callGraph = NewCallGraphHelper()
	}
	if batchRunner == nil {
		batchRunner = NewBatchRunner()
	}
	if synthesizer == nil {
		synthesizer = NewDeliberationSynthesizer(nil, nil, nil)
	}

	return &MultiAgentCoordinator{
		config:      c,
		specialists: make(map[string]IReviewSpecialist),
		callGraph:   callGraph,
		batchRunner: batchRunner,
		synthesizer: synthesizer,
	}
}

// RegisterSpecialist registers an available review specialist persona.
func (c *MultiAgentCoordinator) RegisterSpecialist(s IReviewSpecialist) {
	c.specialists[s.Name()] = s
}

// ResolveEnabledSpecialists selects the active specialists based on ReviewOptions.
func (c *MultiAgentCoordinator) ResolveEnabledSpecialists(opts ReviewOptions) []IReviewSpecialist {
	var active []IReviewSpecialist

	if opts.Security {
		if s, ok := c.specialists["security"]; ok {
			active = append(active, s)
		}
	}
	if opts.Performance {
		if s, ok := c.specialists["performance"]; ok {
			active = append(active, s)
		}
	}
	if opts.Bug {
		if s, ok := c.specialists["bug"]; ok {
			active = append(active, s)
		}
	}
	if opts.Architecture {
		if s, ok := c.specialists["architecture"]; ok {
			active = append(active, s)
		}
	}
	if opts.DrixyRules {
		if s, ok := c.specialists["drixy_rules"]; ok {
			active = append(active, s)
		}
	}
	if opts.BusinessLogic {
		if s, ok := c.specialists["business_logic"]; ok {
			active = append(active, s)
		}
	}

	return active
}

// CoordinateReview executes the end-to-end multi-agent review lifecycle.
func (c *MultiAgentCoordinator) CoordinateReview(
	ctx context.Context,
	input ReviewAgentInput,
) (*OrchestratorOutput, error) {
	startTime := time.Now()

	reviewCtx, cancel := context.WithTimeout(ctx, c.config.TotalReviewTimeout)
	defer cancel()

	// 1. Build Cross-File Call Graph Context
	graph := c.callGraph.BuildCallGraph(input.ChangedFiles)
	callGraphPrompt := c.callGraph.AssembleCallGraphPrompt(graph)
	if callGraphPrompt != "" {
		if input.ExternalContext != "" {
			input.ExternalContext = input.ExternalContext + "\n\n" + callGraphPrompt
		} else {
			input.ExternalContext = callGraphPrompt
		}
	}

	// 2. Resolve Active Specialists
	activeSpecialists := c.ResolveEnabledSpecialists(input.ReviewOptions)
	if len(activeSpecialists) == 0 {
		return &OrchestratorOutput{
			Verdict:         VerdictApprove,
			Summary:         "No review categories enabled; review skipped.",
			TotalDurationMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	// 3. Fan-out execution across specialists with bounded concurrency
	var (
		mu           sync.Mutex
		agentOutputs []ReviewAgentOutput
		failures     []OrchestratorAgentFailure
		incomplete   []OrchestratorAgentIncomplete
		wg           sync.WaitGroup
		sem          = make(chan struct{}, c.config.MaxConcurrentAgents)
	)

	for _, spec := range activeSpecialists {
		wg.Add(1)
		go func(s IReviewSpecialist) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			agentStart := time.Now()
			agentCtx, agentCancel := context.WithTimeout(reviewCtx, c.config.AgentTimeout)
			defer agentCancel()

			// Check if diff is large enough to warrant chunked batch review
			totalTokens := EstimateTotalTokens(input.ChangedFiles)
			budget := input.ReviewOptions.MaxTokens
			if budget <= 0 {
				budget = 32000
			}

			var out *ReviewAgentOutput
			var err error

			if totalTokens > budget && len(input.ChangedFiles) > 1 {
				// Execute via chunked BatchRunner
				out, err = c.batchRunner.RunChunkedReview(agentCtx, input, func(bCtx context.Context, bInput ReviewAgentInput) (*ReviewAgentOutput, error) {
					return s.Review(bCtx, bInput)
				})
			} else {
				// Execute directly
				out, err = s.Review(agentCtx, input)
			}

			durationMs := time.Since(agentStart).Milliseconds()

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				failures = append(failures, OrchestratorAgentFailure{
					AgentName:  s.Name(),
					Category:   s.Category(),
					Error:      err.Error(),
					DurationMs: durationMs,
				})
				return
			}

			if out != nil {
				if out.FinishReason == "timeout" || out.FinishReason == "max-steps" {
					incomplete = append(incomplete, OrchestratorAgentIncomplete{
						AgentName:        s.Name(),
						Category:         s.Category(),
						FinishReason:     out.FinishReason,
						SuggestionsFound: len(out.Findings),
						DurationMs:       durationMs,
					})
				}
				agentOutputs = append(agentOutputs, *out)
			}
		}(spec)
	}

	wg.Wait()

	// 4. Synthesize Review via Consensus & Skeptical Arbiter
	res, err := c.synthesizer.SynthesizeReview(reviewCtx, input, agentOutputs)
	if err != nil {
		return nil, fmt.Errorf("review synthesis failed: %w", err)
	}

	res.Failures = failures
	res.Incomplete = incomplete
	res.TotalDurationMs = time.Since(startTime).Milliseconds()

	return res, nil
}

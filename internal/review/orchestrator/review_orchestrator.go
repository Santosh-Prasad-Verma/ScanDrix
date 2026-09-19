// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package orchestrator

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// OrchestratorStepBudgets defines standard step allocations per mode.
var (
	FastModeMaxSteps = map[string]int{
		"generalist":     4,
		"bug":            4,
		"security":       3,
		"performance":    3,
		"drixy_rules":    4,
		"architecture":   4,
		"business_logic": 4,
	}

	NormalModeMaxSteps = map[string]int{
		"generalist":     20,
		"bug":            20,
		"security":       12,
		"performance":    12,
		"drixy_rules":    20,
		"architecture":   15,
		"business_logic": 20,
	}

	DeepModeMaxSteps = 100
)

const (
	BaselineFilesCount = 8
	StepsPerExtraFile  = 0.5
	AdaptiveCapSteps   = 100
)

// ReviewOrchestratorConfig configures concurrency, budgets, and timeouts.
type ReviewOrchestratorConfig struct {
	MaxConcurrentAgents int           `json:"max_concurrent_agents"`
	AgentTimeout        time.Duration `json:"agent_timeout"`
	TotalReviewTimeout  time.Duration `json:"total_review_timeout"`
}

// DefaultReviewOrchestratorConfig returns enterprise defaults for review orchestration.
func DefaultReviewOrchestratorConfig() ReviewOrchestratorConfig {
	return ReviewOrchestratorConfig{
		MaxConcurrentAgents: 6,
		AgentTimeout:        5 * time.Minute,
		TotalReviewTimeout:  10 * time.Minute,
	}
}

// ReviewOrchestratorService coordinates specialized review personas, manages mode budgets,
// strips file content bodies before dispatching to preserve context, and runs deliberation synthesis.
type ReviewOrchestratorService struct {
	config      ReviewOrchestratorConfig
	specialists map[string]IReviewSpecialist
	callGraph   *CallGraphHelper
	batchRunner *BatchRunner
	synthesizer *DeliberationSynthesizer
	mu          sync.RWMutex
}

// NewReviewOrchestratorService constructs a review orchestrator service.
func NewReviewOrchestratorService(
	callGraph *CallGraphHelper,
	batchRunner *BatchRunner,
	synthesizer *DeliberationSynthesizer,
	cfg ...ReviewOrchestratorConfig,
) *ReviewOrchestratorService {
	c := DefaultReviewOrchestratorConfig()
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

	return &ReviewOrchestratorService{
		config:      c,
		specialists: make(map[string]IReviewSpecialist),
		callGraph:   callGraph,
		batchRunner: batchRunner,
		synthesizer: synthesizer,
	}
}

// RegisterSpecialist adds an agent specialist persona.
func (s *ReviewOrchestratorService) RegisterSpecialist(specialist IReviewSpecialist) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.specialists[specialist.Name()] = specialist
}

// GetSpecialist retrieves a registered specialist by name.
func (s *ReviewOrchestratorService) GetSpecialist(name string) (IReviewSpecialist, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	spec, ok := s.specialists[name]
	return spec, ok
}

// GetMaxStepsForAgent determines the step budget based on review mode and PR size.
func (s *ReviewOrchestratorService) GetMaxStepsForAgent(agentName string, mode ReviewMode, changedFilesCount int) int {
	if mode == ReviewModeDeep {
		return DeepModeMaxSteps
	}

	if mode == ReviewModeFast {
		if steps, ok := FastModeMaxSteps[agentName]; ok {
			return steps
		}
		return 4
	}

	base := 20
	if steps, ok := NormalModeMaxSteps[agentName]; ok {
		base = steps
	}

	// Adaptive step budget by PR size.
	// Beyond baseline (8 files), grant 0.5 steps per extra file, up to 100 steps.
	if changedFilesCount <= BaselineFilesCount {
		return base
	}

	extra := int(math.Round(float64(changedFilesCount-BaselineFilesCount) * StepsPerExtraFile))
	total := base + extra
	if total > AdaptiveCapSteps {
		return AdaptiveCapSteps
	}
	return total
}

// ResolveAgentsForInput determines which agents should execute based on reviewMode and enabled categories.
func (s *ReviewOrchestratorService) ResolveAgentsForInput(input ReviewAgentInput) []IReviewSpecialist {
	s.mu.RLock()
	defer s.mu.RUnlock()

	opts := input.ReviewOptions
	mode := opts.ReviewMode
	if mode == "" {
		mode = ReviewModeNormal
	}

	var active []IReviewSpecialist

	if mode == ReviewModeDeep {
		// Deep mode: dispatch discrete specialized agents in parallel
		if opts.Bug {
			if a, ok := s.specialists["bug"]; ok {
				active = append(active, a)
			}
		}
		if opts.Security {
			if a, ok := s.specialists["security"]; ok {
				active = append(active, a)
			}
		}
		if opts.Performance {
			if a, ok := s.specialists["performance"]; ok {
				active = append(active, a)
			}
		}
		if opts.Architecture {
			if a, ok := s.specialists["architecture"]; ok {
				active = append(active, a)
			}
		}
		if opts.BusinessLogic {
			if a, ok := s.specialists["business_logic"]; ok {
				active = append(active, a)
			}
		}
	} else {
		// Fast or Normal mode: prefer Generalist agent for unified evaluation
		if gen, ok := s.specialists["generalist"]; ok {
			active = append(active, gen)
		} else {
			// Fallback: run enabled specialists individually
			if opts.Bug {
				if a, ok := s.specialists["bug"]; ok {
					active = append(active, a)
				}
			}
			if opts.Security {
				if a, ok := s.specialists["security"]; ok {
					active = append(active, a)
				}
			}
			if opts.Performance {
				if a, ok := s.specialists["performance"]; ok {
					active = append(active, a)
				}
			}
			if opts.Architecture {
				if a, ok := s.specialists["architecture"]; ok {
					active = append(active, a)
				}
			}
			if opts.BusinessLogic {
				if a, ok := s.specialists["business_logic"]; ok {
					active = append(active, a)
				}
			}
		}
	}

	// Always append drixy_rules agent if enabled and custom rules are provided
	if opts.DrixyRules && len(input.DrixyRules) > 0 {
		if rAgent, ok := s.specialists["drixy_rules"]; ok {
			active = append(active, rAgent)
		}
	}

	return active
}

// StripFileContents strips file bodies from ChangedFiles before dispatching to agents.
// Agents access full source on demand via readFile in the sandbox to conserve context.
func StripFileContents(files []ChangedFile) []ChangedFile {
	stripped := make([]ChangedFile, len(files))
	for i, f := range files {
		fCopy := f
		fCopy.Content = ""
		fCopy.OldContent = ""
		stripped[i] = fCopy
	}
	return stripped
}

// Execute orchestrates the full code review process across specialized agents.
func (s *ReviewOrchestratorService) Execute(
	ctx context.Context,
	input ReviewAgentInput,
) (*OrchestratorOutput, error) {
	startTime := time.Now()

	reviewCtx, cancel := context.WithTimeout(ctx, s.config.TotalReviewTimeout)
	defer cancel()

	// 1. Build Cross-File Call Graph Context
	graph := s.callGraph.BuildCallGraph(input.ChangedFiles)
	callGraphPrompt := s.callGraph.AssembleCallGraphPrompt(graph)
	if callGraphPrompt != "" {
		if input.ExternalContext != "" {
			input.ExternalContext = input.ExternalContext + "\n\n" + callGraphPrompt
		} else {
			input.ExternalContext = callGraphPrompt
		}
	}

	// 2. Resolve Active Agents
	agents := s.ResolveAgentsForInput(input)
	if len(agents) == 0 {
		return &OrchestratorOutput{
			Verdict:         VerdictApprove,
			Summary:         "No review categories enabled; review skipped.",
			TotalDurationMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	// 3. Strip full file contents before dispatching to agents
	agentInputWithoutContent := input
	agentInputWithoutContent.ChangedFiles = StripFileContents(input.ChangedFiles)

	// 4. Concurrent Fan-Out Execution with Semaphore Pool
	var (
		mu           sync.Mutex
		agentOutputs []ReviewAgentOutput
		failures     []OrchestratorAgentFailure
		incomplete   []OrchestratorAgentIncomplete
		wg           sync.WaitGroup
		sem          = make(chan struct{}, s.config.MaxConcurrentAgents)
	)

	filesCount := len(input.ChangedFiles)
	mode := input.ReviewOptions.ReviewMode
	if mode == "" {
		mode = ReviewModeNormal
	}

	for _, agent := range agents {
		wg.Add(1)
		go func(a IReviewSpecialist) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			agentStart := time.Now()
			agentCtx, agentCancel := context.WithTimeout(reviewCtx, s.config.AgentTimeout)
			defer agentCancel()

			// Calculate step budget for this agent
			maxSteps := s.GetMaxStepsForAgent(a.Name(), mode, filesCount)

			// Setup per-agent input
			agentSpecificInput := agentInputWithoutContent
			agentSpecificInput.ReviewOptions.MaxSteps = maxSteps

			// Execute agent review (direct or chunked batch if diff exceeds budget)
			totalTokens := EstimateTotalTokens(input.ChangedFiles)
			budget := input.ReviewOptions.MaxTokens
			if budget <= 0 {
				budget = 32000
			}

			var out *ReviewAgentOutput
			var err error

			if totalTokens > budget && len(input.ChangedFiles) > 1 {
				out, err = s.batchRunner.RunChunkedReview(agentCtx, agentSpecificInput, func(bCtx context.Context, bInput ReviewAgentInput) (*ReviewAgentOutput, error) {
					return a.Review(bCtx, bInput)
				})
			} else {
				out, err = a.Review(agentCtx, agentSpecificInput)
			}

			durationMs := time.Since(agentStart).Milliseconds()

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				failures = append(failures, OrchestratorAgentFailure{
					AgentName:  a.Name(),
					Category:   a.Category(),
					Error:      err.Error(),
					DurationMs: durationMs,
				})
				return
			}

			if out != nil {
				if out.FinishReason == "timeout" || out.FinishReason == "max-steps" {
					incomplete = append(incomplete, OrchestratorAgentIncomplete{
						AgentName:        a.Name(),
						Category:         a.Category(),
						FinishReason:     out.FinishReason,
						SuggestionsFound: len(out.Findings),
						DurationMs:       durationMs,
					})
				}
				agentOutputs = append(agentOutputs, *out)
			}
		}(agent)
	}

	wg.Wait()

	// 5. Deliberation Synthesis across all agent outputs
	// Note: We supply the original input (with full file content if present) to synthesizer
	// so diff boundary validator and skeptical arbiter can verify lines against real patch hunks.
	res, err := s.synthesizer.SynthesizeReview(reviewCtx, input, agentOutputs)
	if err != nil {
		return nil, fmt.Errorf("review orchestration synthesis failed: %w", err)
	}

	res.Failures = failures
	res.Incomplete = incomplete
	res.TotalDurationMs = time.Since(startTime).Milliseconds()

	return res, nil
}

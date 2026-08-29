package advanced

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SweepResult reports progress from a detector reconciliation pass.
type SweepResult struct {
	TotalProcessed int           `json:"total_processed"`
	TotalCompiled  int           `json:"total_compiled"`
	TotalErrored   int           `json:"total_errored"`
	Duration       time.Duration `json:"duration"`
}

// RuleDetectorSweeper reconciles uncompiled rules across organizations.
type RuleDetectorSweeper struct {
	mu             sync.RWMutex
	compiler       *RuleCompiler
	maxRulesPerRun int
	activeRules    map[uuid.UUID][]EnterpriseRuleDefinition // workspaceID -> rules
	compiledCache  map[string]*CompiledDetector             // ruleKey -> detector
}

func NewRuleDetectorSweeper(compiler *RuleCompiler, maxRulesPerRun int) *RuleDetectorSweeper {
	if maxRulesPerRun <= 0 {
		maxRulesPerRun = 2000
	}
	return &RuleDetectorSweeper{
		compiler:       compiler,
		maxRulesPerRun: maxRulesPerRun,
		activeRules:    make(map[uuid.UUID][]EnterpriseRuleDefinition),
		compiledCache:  make(map[string]*CompiledDetector),
	}
}

// RegisterWorkspaceRules registers rule sets for an organization or workspace.
func (s *RuleDetectorSweeper) RegisterWorkspaceRules(workspaceID uuid.UUID, rules []EnterpriseRuleDefinition) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeRules[workspaceID] = rules
}

// RunSweep executes the T0 continuous sweep to compile missing detectors up to the budget cap.
func (s *RuleDetectorSweeper) RunSweep(ctx context.Context, onlyMissing bool) SweepResult {
	start := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	res := SweepResult{}
	budget := s.maxRulesPerRun

	for _, rules := range s.activeRules {
		if budget <= 0 {
			break
		}

		for _, rule := range rules {
			if budget <= 0 {
				break
			}

			res.TotalProcessed++
			budget--

			_, alreadyCompiled := s.compiledCache[rule.RuleKey]
			if onlyMissing && alreadyCompiled {
				continue // Steady-state skip
			}

			detector, err := s.compiler.CompileRule(rule)
			if err != nil {
				res.TotalErrored++
			} else {
				s.compiledCache[rule.RuleKey] = detector
				res.TotalCompiled++
			}
		}
	}

	res.Duration = time.Since(start)
	return res
}

// GetCompiledDetectors returns all active compiled detectors.
func (s *RuleDetectorSweeper) GetCompiledDetectors() []*CompiledDetector {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*CompiledDetector
	for _, d := range s.compiledCache {
		list = append(list, d)
	}
	return list
}

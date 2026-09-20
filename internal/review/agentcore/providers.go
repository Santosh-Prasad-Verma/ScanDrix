package agentcore

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// ReviewAgentCategory defines the specialized review focus.
type ReviewAgentCategory string

const (
	CategoryBug         ReviewAgentCategory = "bug"
	CategorySecurity    ReviewAgentCategory = "security"
	CategoryPerformance ReviewAgentCategory = "performance"
	CategoryRules       ReviewAgentCategory = "custom_rule"
	CategoryGeneralist  ReviewAgentCategory = "architecture"
)

// SpecializedAgentIdentity describes the role and expertise of a specialized agent.
type SpecializedAgentIdentity struct {
	Name        string
	Category    ReviewAgentCategory
	Description string
	Goal        string
	Expertise   []string
}

// SpecializedAgentProvider defines the contract for domain-specific review agents.
type SpecializedAgentProvider interface {
	Identity() SpecializedAgentIdentity
	Category() ReviewAgentCategory
	SystemPrompt(customRules []string) string
	Execute(ctx context.Context, runner contracts.AgentRunner, input ReviewAgentInput, toolCtx contracts.ToolContext) (*ReviewAgentOutput, error)
}

// BaseReviewAgentProvider provides shared execution plumbing for all specialized agents.
type BaseReviewAgentProvider struct {
	category    ReviewAgentCategory
	name        string
	description string
	goal        string
	expertise   []string
}

func (p *BaseReviewAgentProvider) Identity() SpecializedAgentIdentity {
	return SpecializedAgentIdentity{
		Name:        p.name,
		Category:    p.category,
		Description: p.description,
		Goal:        p.goal,
		Expertise:   p.expertise,
	}
}

func (p *BaseReviewAgentProvider) Category() ReviewAgentCategory {
	return p.category
}

func (p *BaseReviewAgentProvider) Execute(
	ctx context.Context,
	runner contracts.AgentRunner,
	input ReviewAgentInput,
	toolCtx contracts.ToolContext,
) (*ReviewAgentOutput, error) {
	providerPrompt := p.SystemPrompt(nil)
	if input.SystemPrompt == "" {
		input.SystemPrompt = providerPrompt
	} else {
		input.SystemPrompt = providerPrompt + "\n\n" + input.SystemPrompt
	}
	input.AgentName = string(p.category)

	return RunAgentLoopViaCore(ctx, runner, input, toolCtx)
}

func (p *BaseReviewAgentProvider) SystemPrompt(customRules []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("You are %s, an elite enterprise code review agent.\n", p.name))
	b.WriteString(fmt.Sprintf("Goal: %s\n\n", p.goal))
	b.WriteString("Expertise areas:\n")
	for _, e := range p.expertise {
		b.WriteString(fmt.Sprintf("- %s\n", e))
	}
	if len(customRules) > 0 {
		b.WriteString("\nOrganizational & Team Rules to enforce strictly:\n")
		for _, r := range customRules {
			b.WriteString(fmt.Sprintf("- %s\n", r))
		}
	}
	return b.String()
}

// 1. BUG AGENT PROVIDER (ScanDrix bug-agent)

type BugAgentProvider struct {
	BaseReviewAgentProvider
}

func NewBugAgentProvider() *BugAgentProvider {
	return &BugAgentProvider{
		BaseReviewAgentProvider: BaseReviewAgentProvider{
			category:    CategoryBug,
			name:        "scandrix-bug-review-agent",
			description: "Senior software engineer specialized in finding bugs, logic errors, edge cases, error handling issues, data flow problems, and race conditions in code changes.",
			goal:        "Find real, impactful bugs in the code changes by investigating the codebase before making any suggestion.",
			expertise: []string{
				"Bug detection and logic analysis",
				"Edge case identification and boundary conditions",
				"Error handling and panic prevention",
				"Data flow and state mutation consistency",
				"Race conditions and concurrency hazards",
				"Null / Nil pointer safety",
			},
		},
	}
}

// 2. SECURITY AGENT PROVIDER (ScanDrix security-agent)

type SecurityAgentProvider struct {
	BaseReviewAgentProvider
}

func NewSecurityAgentProvider() *SecurityAgentProvider {
	return &SecurityAgentProvider{
		BaseReviewAgentProvider: BaseReviewAgentProvider{
			category:    CategorySecurity,
			name:        "scandrix-security-review-agent",
			description: "Principal application security engineer specialized in identifying OWASP Top 10 vulnerabilities, unauthorized access risks, injection vectors, and data privacy leaks.",
			goal:        "Identify exploitable security vulnerabilities, injection flaws, and authorization bypasses backed by concrete repository evidence.",
			expertise: []string{
				"OWASP Top 10 web vulnerabilities",
				"SQL, Command, and Template Injection",
				"Authentication and Authorization bypass (RBAC / IDOR)",
				"Server-Side Request Forgery (SSRF)",
				"Cryptographic misuse and insecure randomness",
				"Secrets, tokens, and PII leakage",
			},
		},
	}
}

// 3. PERFORMANCE AGENT PROVIDER (ScanDrix performance-agent)

type PerformanceAgentProvider struct {
	BaseReviewAgentProvider
}

func NewPerformanceAgentProvider() *PerformanceAgentProvider {
	return &PerformanceAgentProvider{
		BaseReviewAgentProvider: BaseReviewAgentProvider{
			category:    CategoryPerformance,
			name:        "scandrix-performance-review-agent",
			description: "Systems and performance engineer specialized in detecting computational inefficiencies, memory leaks, unindexed database queries, and scaling bottlenecks.",
			goal:        "Detect latency spikes, redundant computation, memory growth, and database query anti-patterns across altered code paths.",
			expertise: []string{
				"Algorithmic complexity (O(N^2) loops and redundant iterations)",
				"Database query optimization (N+1 queries, unindexed scans)",
				"Memory allocations, buffer growth, and resource leakage",
				"Goroutine and connection pool starvation",
				"Cache invalidation and contention issues",
			},
		},
	}
}

// 4. DRIXY RULES AGENT PROVIDER (ScanDrix rules-agent)

type DrixyRulesAgentProvider struct {
	BaseReviewAgentProvider
}

func NewDrixyRulesAgentProvider() *DrixyRulesAgentProvider {
	return &DrixyRulesAgentProvider{
		BaseReviewAgentProvider: BaseReviewAgentProvider{
			category:    CategoryRules,
			name:        "scandrix-rules-review-agent",
			description: "Rule compliance auditor specialized in enforcing organization-wide standards, architectural boundaries, and custom repository guidelines.",
			goal:        "Verify full conformance with custom team guidelines and flag non-compliant code constructs verbatim.",
			expertise: []string{
				"Custom organization rules verification",
				"Architectural layering and dependency boundaries",
				"Coding style and convention enforcement",
				"API versioning and contract compatibility",
			},
		},
	}
}

// 5. GENERALIST AGENT PROVIDER (ScanDrix generalist-agent)

type GeneralistAgentProvider struct {
	BaseReviewAgentProvider
}

func NewGeneralistAgentProvider() *GeneralistAgentProvider {
	return &GeneralistAgentProvider{
		BaseReviewAgentProvider: BaseReviewAgentProvider{
			category:    CategoryGeneralist,
			name:        "scandrix-generalist-review-agent",
			description: "Staff engineer specialized in overarching architecture, maintainability, readability, test coverage, and idiomatic conventions.",
			goal:        "Provide holistic review balance across code structure, testing adequacy, and clarity.",
			expertise: []string{
				"Clean architecture and modularity",
				"Test coverage and testability of changes",
				"Idiomatic language practices",
				"Readability, documentation, and maintainability",
			},
		},
	}
}

// 6. MULTI-AGENT REVIEW ORCHESTRATOR

// MultiAgentReviewConfig configures parallel review dispatch with zero hardcoded values.
type MultiAgentReviewConfig struct {
	EnableBugAgent         bool
	EnableSecurityAgent    bool
	EnablePerformanceAgent bool
	EnableRulesAgent       bool
	EnableGeneralistAgent  bool
	MaxConcurrentAgents    int
	DedupSimilarityFloor   float64
}

// DefaultMultiAgentReviewConfig loads settings from environment variables or production defaults.
func DefaultMultiAgentReviewConfig() MultiAgentReviewConfig {
	cfg := MultiAgentReviewConfig{
		EnableBugAgent:         true,
		EnableSecurityAgent:    true,
		EnablePerformanceAgent: true,
		EnableRulesAgent:       true,
		EnableGeneralistAgent:  true,
		MaxConcurrentAgents:    3,
		DedupSimilarityFloor:   0.30,
	}

	if val := os.Getenv("SCANDRIX_ENABLE_BUG_AGENT"); val != "" {
		cfg.EnableBugAgent = strings.ToLower(val) == "true" || val == "1"
	}
	if val := os.Getenv("SCANDRIX_ENABLE_SECURITY_AGENT"); val != "" {
		cfg.EnableSecurityAgent = strings.ToLower(val) == "true" || val == "1"
	}
	if val := os.Getenv("SCANDRIX_ENABLE_PERFORMANCE_AGENT"); val != "" {
		cfg.EnablePerformanceAgent = strings.ToLower(val) == "true" || val == "1"
	}
	if val := os.Getenv("SCANDRIX_ENABLE_RULES_AGENT"); val != "" {
		cfg.EnableRulesAgent = strings.ToLower(val) == "true" || val == "1"
	}
	if val := os.Getenv("SCANDRIX_ENABLE_GENERALIST_AGENT"); val != "" {
		cfg.EnableGeneralistAgent = strings.ToLower(val) == "true" || val == "1"
	}
	if val := os.Getenv("SCANDRIX_MAX_CONCURRENT_AGENTS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			cfg.MaxConcurrentAgents = parsed
		}
	}
	if val := os.Getenv("SCANDRIX_DEDUP_SIMILARITY_FLOOR"); val != "" {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil && parsed > 0 {
			cfg.DedupSimilarityFloor = parsed
		}
	}

	return cfg
}

// MultiAgentReviewOrchestrator coordinates concurrent specialized review passes.
type MultiAgentReviewOrchestrator struct {
	config    MultiAgentReviewConfig
	providers []SpecializedAgentProvider
}

// NewMultiAgentReviewOrchestrator creates the multi-agent orchestrator.
func NewMultiAgentReviewOrchestrator(cfg ...MultiAgentReviewConfig) *MultiAgentReviewOrchestrator {
	c := DefaultMultiAgentReviewConfig()
	if len(cfg) > 0 {
		c = cfg[0]
	}

	var activeProviders []SpecializedAgentProvider
	if c.EnableBugAgent {
		activeProviders = append(activeProviders, NewBugAgentProvider())
	}
	if c.EnableSecurityAgent {
		activeProviders = append(activeProviders, NewSecurityAgentProvider())
	}
	if c.EnablePerformanceAgent {
		activeProviders = append(activeProviders, NewPerformanceAgentProvider())
	}
	if c.EnableRulesAgent {
		activeProviders = append(activeProviders, NewDrixyRulesAgentProvider())
	}
	if c.EnableGeneralistAgent {
		activeProviders = append(activeProviders, NewGeneralistAgentProvider())
	}

	return &MultiAgentReviewOrchestrator{
		config:    c,
		providers: activeProviders,
	}
}

// ExecuteMultiAgentReview runs all enabled specialized agents in parallel and merges findings.
func (o *MultiAgentReviewOrchestrator) ExecuteMultiAgentReview(
	ctx context.Context,
	runner contracts.AgentRunner,
	baseInput ReviewAgentInput,
	toolCtx contracts.ToolContext,
) ([]FinderSuggestion, error) {
	if len(o.providers) == 0 {
		return []FinderSuggestion{}, nil
	}

	concurrency := o.config.MaxConcurrentAgents
	if concurrency <= 0 {
		concurrency = 3
	}
	sem := make(chan struct{}, concurrency)

	var mu sync.Mutex
	var aggregatedFindings []FinderSuggestion
	var wg sync.WaitGroup

	for _, p := range o.providers {
		wg.Add(1)
		go func(prov SpecializedAgentProvider) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			// Copy input per agent
			input := baseInput
			out, err := prov.Execute(ctx, runner, input, toolCtx)
			if err == nil && out != nil && len(out.VerifiedFindings) > 0 {
				mu.Lock()
				aggregatedFindings = append(aggregatedFindings, out.VerifiedFindings...)
				mu.Unlock()
			}
		}(p)
	}

	wg.Wait()

	// Near-duplicate collapse via Jaccard similarity across multi-agent findings
	deduped := CollapseNearDuplicates(aggregatedFindings, o.config.DedupSimilarityFloor)
	return deduped, nil
}

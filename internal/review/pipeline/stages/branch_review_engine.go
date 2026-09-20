package stages

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// BranchReviewRuleMap maps source branch patterns to target branch patterns and their boolean decision.
type BranchReviewRuleMap map[string]map[string]bool

// BranchReviewConfig encapsulates compiled branch review rules.
type BranchReviewConfig struct {
	ReviewRules BranchReviewRuleMap `json:"reviewRules"`
}

var validBranchRuleRegex = regexp.MustCompile(`^[a-zA-Z0-9/*\-_!=:]+$`)

// ProcessBranchExpression converts a comma-separated branch rules expression into a BranchReviewConfig.
func ProcessBranchExpression(expression string) BranchReviewConfig {
	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return BranchReviewConfig{ReviewRules: make(BranchReviewRuleMap)}
	}

	rules := strings.Split(trimmed, ",")
	reviewRules := make(BranchReviewRuleMap)

	for _, raw := range rules {
		rule := strings.TrimSpace(raw)
		if rule == "" {
			continue
		}

		if reviewRules["*"] == nil {
			reviewRules["*"] = make(map[string]bool)
		}

		switch {
		case strings.HasPrefix(rule, "!=="):
			pattern := rule[3:]
			reviewRules["*"]["!"+pattern] = false
		case strings.HasPrefix(rule, "!"):
			pattern := rule[1:]
			reviewRules["*"]["!"+pattern] = false
		case strings.HasPrefix(rule, "="):
			pattern := rule[1:]
			reviewRules["*"][pattern] = true
		case strings.HasPrefix(rule, "contains:"):
			if reviewRules[rule] == nil {
				reviewRules[rule] = make(map[string]bool)
			}
			reviewRules[rule]["*"] = true
		default:
			reviewRules["*"][rule] = true
		}
	}

	return BranchReviewConfig{ReviewRules: reviewRules}
}

// ValidateBranchExpression validates branch rule syntax, rejecting double wildcards and invalid characters.
func ValidateBranchExpression(expression string) (bool, []string) {
	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return true, nil
	}

	rawRules := strings.Split(trimmed, ",")
	var errors []string
	seen := make(map[string]struct{})

	for idx, raw := range rawRules {
		rule := strings.TrimSpace(raw)
		if rule == "" {
			continue
		}

		ruleNum := idx + 1
		if _, exists := seen[rule]; exists {
			errors = append(errors, fmt.Sprintf("Rule %d: duplicate rule '%s' found", ruleNum, rule))
		}
		seen[rule] = struct{}{}

		if len(rule) > 100 {
			errors = append(errors, fmt.Sprintf("Rule %d exceeds 100 characters", ruleNum))
			continue
		}

		if rule == "!" || rule == "=" || rule == "contains:" || rule == "!==" {
			errors = append(errors, fmt.Sprintf("Rule %d is invalid: '%s' cannot be empty", ruleNum, rule))
			continue
		}

		if !validBranchRuleRegex.MatchString(rule) {
			errors = append(errors, fmt.Sprintf("Rule %d contains invalid characters: '%s'", ruleNum, rule))
			continue
		}

		if strings.Contains(rule, "**") {
			errors = append(errors, fmt.Sprintf("Rule %d is invalid: '**' is not allowed", ruleNum))
			continue
		}
	}

	return len(errors) == 0, errors
}

// ConvertConfigToBranchExpression formats a BranchReviewConfig back into an expression string.
func ConvertConfigToBranchExpression(config BranchReviewConfig) string {
	if config.ReviewRules == nil {
		return ""
	}

	var rules []string
	for sourcePattern, targets := range config.ReviewRules {
		for targetPattern, shouldReview := range targets {
			if sourcePattern == "*" {
				if strings.HasPrefix(targetPattern, "!") {
					rules = append(rules, targetPattern)
				} else if shouldReview {
					rules = append(rules, "="+targetPattern)
				} else {
					rules = append(rules, "!=="+strings.TrimPrefix(targetPattern, "!"))
				}
			} else {
				rules = append(rules, sourcePattern)
			}
		}
	}

	sort.Strings(rules)
	return strings.Join(rules, ", ")
}

type specificityResult struct {
	result      bool
	specificity int
}

// ShouldReviewBranches determines whether a pull request matches configured source/target branch patterns.
func ShouldReviewBranches(sourceBranch, targetBranch string, config BranchReviewConfig) bool {
	if sourceBranch == "" || targetBranch == "" || config.ReviewRules == nil || len(config.ReviewRules) == 0 {
		return false
	}

	var results []specificityResult

	for sourcePattern, targets := range config.ReviewRules {
		if matchesBranchPattern(sourceBranch, sourcePattern) {
			for targetPattern, shouldReview := range targets {
				if matchesBranchPattern(targetBranch, targetPattern) {
					spec := calculateBranchSpecificity(sourcePattern, targetPattern)
					results = append(results, specificityResult{
						result:      shouldReview,
						specificity: spec,
					})
				}
			}
		}
	}

	if len(results) == 0 {
		return false
	}

	// Sort by descending specificity
	sort.Slice(results, func(i, j int) bool {
		return results[i].specificity > results[j].specificity
	})

	return results[0].result
}

// matchesBranchPattern checks if a branch name satisfies wildcard, contains, or exact match patterns.
func matchesBranchPattern(branch, pattern string) bool {
	if branch == "" || pattern == "" {
		return false
	}

	// Exclusions
	if strings.HasPrefix(pattern, "!") {
		exclusion := pattern[1:]
		return matchesBranchPattern(branch, exclusion)
	}

	// Contains
	if strings.HasPrefix(pattern, "contains:") {
		substr := pattern[9:]
		return strings.Contains(branch, substr)
	}

	// Wildcard
	if strings.Contains(pattern, "*") {
		escaped := regexp.QuoteMeta(pattern)
		regexStr := "^" + strings.ReplaceAll(escaped, `\*`, ".*") + "$"
		re, err := regexp.Compile(regexStr)
		if err != nil {
			return false
		}
		return re.MatchString(branch)
	}

	// Exact match
	return branch == pattern
}

// calculateBranchSpecificity computes weighted specificity score for rule resolution.
func calculateBranchSpecificity(sourcePattern, targetPattern string) int {
	score := 0

	// Source Pattern Score
	switch {
	case sourcePattern == "*":
		score += 1
	case strings.HasPrefix(sourcePattern, "!"):
		score += 15
	case strings.HasPrefix(sourcePattern, "contains:"):
		score += 5
	case strings.Contains(sourcePattern, "*"):
		score += 8
	default:
		score += 10
	}

	// Target Pattern Score - Exclusions receive maximum priority
	switch {
	case strings.HasPrefix(targetPattern, "!"):
		score += 100
	case targetPattern == "*":
		score += 1
	case strings.Contains(targetPattern, "*"):
		score += 3
	default:
		score += 8
	}

	return score
}

// MergeBaseBranches merges user-configured branch rules with repository default branch.
func MergeBaseBranches(configuredBranches []string, apiBaseBranch string) []string {
	exclusions := make(map[string]struct{})
	inclusions := make(map[string]struct{})

	for _, branch := range configuredBranches {
		b := strings.TrimSpace(branch)
		if b == "" {
			continue
		}
		if strings.HasPrefix(b, "!") {
			exclusions[b] = struct{}{}
		} else {
			inclusions[b] = struct{}{}
		}
	}

	if apiBaseBranch != "" {
		apiExclusion := "!" + apiBaseBranch
		if _, hasExcl := exclusions[apiExclusion]; !hasExcl {
			if _, hasIncl := inclusions[apiBaseBranch]; !hasIncl {
				inclusions[apiBaseBranch] = struct{}{}
			}
		}
	}

	merged := make(map[string]struct{})
	for inc := range inclusions {
		if _, excluded := exclusions["!"+inc]; !excluded {
			merged[inc] = struct{}{}
		}
	}

	for excl := range exclusions {
		inc := excl[1:]
		if _, included := inclusions[inc]; !included {
			merged[excl] = struct{}{}
		}
	}

	result := make([]string, 0, len(merged))
	for b := range merged {
		result = append(result, b)
	}
	sort.Strings(result)
	return result
}

// NormalizeBranchesForPlatform formats branch patterns for provider conventions (e.g. Azure DevOps refs/heads/).
func NormalizeBranchesForPlatform(branches []string, provider models.SCMProvider) []string {
	if provider != models.ProviderAzure {
		return branches
	}

	normalized := make([]string, 0, len(branches))
	for _, branch := range branches {
		b := strings.TrimSpace(branch)
		if b == "" {
			continue
		}
		if strings.HasPrefix(b, "refs/heads/") {
			normalized = append(normalized, b)
			continue
		}
		if strings.HasPrefix(b, "!") {
			pattern := b[1:]
			if strings.HasPrefix(pattern, "refs/heads/") {
				normalized = append(normalized, b)
			} else {
				normalized = append(normalized, "!refs/heads/"+pattern)
			}
			continue
		}
		if strings.HasPrefix(b, "=") {
			pattern := b[1:]
			if strings.HasPrefix(pattern, "refs/heads/") {
				normalized = append(normalized, b)
			} else {
				normalized = append(normalized, "=refs/heads/"+pattern)
			}
			continue
		}
		if strings.HasPrefix(b, "contains:") {
			normalized = append(normalized, b)
			continue
		}
		normalized = append(normalized, "refs/heads/"+b)
	}
	return normalized
}

// CadenceExecutionStore captures prior executions to detect rapid push bursts.
type CadenceExecutionStore interface {
	FindRecentSuccessfulRuns(ctx context.Context, repoID string, prNumber int, since time.Time) ([]time.Time, error)
	GetLastReviewCadenceState(ctx context.Context, repoID string, prNumber int) (domain.ReviewCadenceState, error)
}

// CadenceDecision encapsulates the outcome of review cadence evaluation.
type CadenceDecision struct {
	ShouldProcess         bool
	Reason                string
	ShouldSaveSkipped     bool
	PreviousCadenceStatus domain.ReviewCadenceState
	CurrentCadenceStatus  domain.ReviewCadenceState
	PauseCommentBody      string
}

// ReviewCadenceEngine evaluates automatic, manual, and auto-pause modes.
type ReviewCadenceEngine struct {
	store CadenceExecutionStore
}

// NewReviewCadenceEngine constructs a ReviewCadenceEngine.
func NewReviewCadenceEngine(store CadenceExecutionStore) *ReviewCadenceEngine {
	return &ReviewCadenceEngine{store: store}
}

// EvaluateCadence determines whether the review should proceed based on cadence rules and push bursts.
func (e *ReviewCadenceEngine) EvaluateCadence(ctx context.Context, pCtx *pipeline.PipelineContext) (CadenceDecision, error) {
	// Manual slash command trigger (@scandrix review or @drixy review or --force) always proceeds
	if strings.HasPrefix(pCtx.Origin, "command") {
		prevStatus := domain.CadenceStateAutomatic
		if e.store != nil {
			if s, err := e.store.GetLastReviewCadenceState(ctx, pCtx.RepositoryID.String(), pCtx.PullNumber); err == nil && s != "" {
				prevStatus = s
			}
		}
		return CadenceDecision{
			ShouldProcess:         true,
			Reason:                "Review triggered by start-review command",
			ShouldSaveSkipped:     false,
			PreviousCadenceStatus: prevStatus,
			CurrentCadenceStatus:  domain.CadenceStateCommand,
		}, nil
	}

	cadenceType := pCtx.ResolvedConfig.ReviewCadence.Type
	if cadenceType == "" {
		cadenceType = domain.CadenceAutomatic
	}

	switch cadenceType {
	case domain.CadenceAutomatic:
		return CadenceDecision{
			ShouldProcess:         true,
			Reason:                "Automatic review execution enabled",
			ShouldSaveSkipped:     false,
			PreviousCadenceStatus: domain.CadenceStateAutomatic,
			CurrentCadenceStatus:  domain.CadenceStateAutomatic,
		}, nil

	case domain.CadenceManual:
		// In manual mode, only the first PR review is executed automatically. Subsequent pushes require a command.
		hasPrior := false
		if pCtx.LastExecution != nil && pCtx.LastExecution.LastAnalyzedCommit != "" {
			hasPrior = true
		}
		if !hasPrior {
			return CadenceDecision{
				ShouldProcess:         true,
				Reason:                "Initial manual mode review executed automatically",
				ShouldSaveSkipped:     false,
				PreviousCadenceStatus: domain.CadenceStateAutomatic,
				CurrentCadenceStatus:  domain.CadenceStateAutomatic,
			}, nil
		}

		prevStatus := domain.CadenceStateAutomatic
		if e.store != nil {
			if s, err := e.store.GetLastReviewCadenceState(ctx, pCtx.RepositoryID.String(), pCtx.PullNumber); err == nil && s != "" {
				prevStatus = s
			}
		}

		return CadenceDecision{
			ShouldProcess:         false,
			Reason:                "Manual review required to initiate next analysis pass",
			ShouldSaveSkipped:     true,
			PreviousCadenceStatus: prevStatus,
			CurrentCadenceStatus:  domain.CadenceStatePaused,
		}, nil

	case domain.CadenceAutoPause:
		hasPrior := false
		if pCtx.LastExecution != nil && pCtx.LastExecution.LastAnalyzedCommit != "" {
			hasPrior = true
		}
		if !hasPrior {
			return CadenceDecision{
				ShouldProcess:         true,
				Reason:                "Initial auto-pause mode review executed",
				ShouldSaveSkipped:     false,
				PreviousCadenceStatus: domain.CadenceStateAutomatic,
				CurrentCadenceStatus:  domain.CadenceStateAutomatic,
			}, nil
		}

		prevStatus := domain.CadenceStateAutomatic
		if e.store != nil {
			if s, err := e.store.GetLastReviewCadenceState(ctx, pCtx.RepositoryID.String(), pCtx.PullNumber); err == nil && s != "" {
				prevStatus = s
			}
		}

		if prevStatus == domain.CadenceStatePaused {
			return CadenceDecision{
				ShouldProcess:         false,
				Reason:                "Pull request is currently paused; comment @scandrix start-review to resume",
				ShouldSaveSkipped:     true,
				PreviousCadenceStatus: domain.CadenceStatePaused,
				CurrentCadenceStatus:  domain.CadenceStatePaused,
			}, nil
		}

		// Check burst threshold
		pushesToTrigger := pCtx.ResolvedConfig.ReviewCadence.PushesToTrigger
		if pushesToTrigger <= 0 {
			pushesToTrigger = 3
		}
		timeWindow := pCtx.ResolvedConfig.ReviewCadence.TimeWindowMinutes
		if timeWindow <= 0 {
			timeWindow = 15
		}

		if e.store != nil {
			since := time.Now().Add(-time.Duration(timeWindow) * time.Minute)
			recentRuns, err := e.store.FindRecentSuccessfulRuns(ctx, pCtx.RepositoryID.String(), pCtx.PullNumber, since)
			if err == nil && len(recentRuns) >= pushesToTrigger {
				return CadenceDecision{
					ShouldProcess:         false,
					Reason:                fmt.Sprintf("Multiple pushes (%d) detected in %d minute window; auto-paused", len(recentRuns), timeWindow),
					ShouldSaveSkipped:     true,
					PreviousCadenceStatus: domain.CadenceStateAutomatic,
					CurrentCadenceStatus:  domain.CadenceStatePaused,
					PauseCommentBody:      "Auto-paused – comment `@scandrix start-review` when you're ready.",
				}, nil
			}
		}

		return CadenceDecision{
			ShouldProcess:         true,
			Reason:                "Auto-pause cadence threshold not exceeded",
			ShouldSaveSkipped:     false,
			PreviousCadenceStatus: domain.CadenceStateAutomatic,
			CurrentCadenceStatus:  domain.CadenceStateAutomatic,
		}, nil
	}

	return CadenceDecision{ShouldProcess: true, Reason: "Default cadence processing"}, nil
}

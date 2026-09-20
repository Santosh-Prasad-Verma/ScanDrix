package stages

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/review/pipeline"
)

// ICommitFetcher abstracts retrieving git commit history from the SCM platform.
type ICommitFetcher interface {
	GetCommitsForPR(ctx context.Context, repoNamespace string, prNumber int) ([]pipeline.CommitInfo, error)
	HasStageWithStatus(ctx context.Context, executionID string, stageNames []string, statuses []pipeline.AutomationStatus) (bool, error)
	FindLatestExecution(ctx context.Context, workspaceID, repoID string, prNumber int) (*pipeline.PreviousExecutionInfo, error)
}

// DeepValidateNewCommitsStage implements Stage 2 canonical commit gating with merge-commit graph traversal and rebase drift detection.
type DeepValidateNewCommitsStage struct {
	commitFetcher           ICommitFetcher
	rerunEligibleStageNames []string
	rerunEligibleStatuses   []pipeline.AutomationStatus
}

// NewDeepValidateNewCommitsStage constructs Stage 2.
func NewDeepValidateNewCommitsStage(fetcher ICommitFetcher) *DeepValidateNewCommitsStage {
	return &DeepValidateNewCommitsStage{
		commitFetcher: fetcher,
		rerunEligibleStageNames: []string{
			"FileAnalysisStage",
			"PRLevelReviewStage",
			"AgentReviewStage",
		},
		rerunEligibleStatuses: []pipeline.AutomationStatus{
			pipeline.StatusPartialError,
			pipeline.StatusError,
		},
	}
}

func (s *DeepValidateNewCommitsStage) Name() string {
	return "DeepValidateNewCommitsStage"
}

func (s *DeepValidateNewCommitsStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if pCtx.SkipReview {
		return nil
	}

	workspaceID := pCtx.WorkspaceID.String()
	repoID := pCtx.RepositoryID.String()

	// 1. Check previous successful execution to resolve lastAnalyzedCommit
	var lastAnalyzedCommit string
	var lastExecutionResult *pipeline.PreviousExecutionInfo
	forceFullRerun := false
	var orphanedBaseCommit *pipeline.OrphanedBaseCommitInfo

	if s.commitFetcher != nil && pCtx.LastExecution == nil {
		latest, err := s.commitFetcher.FindLatestExecution(ctx, workspaceID, repoID, pCtx.PullNumber)
		if err == nil && latest != nil && latest.LastAnalyzedCommit != "" {
			pCtx.LastExecution = latest
		}
	}

	if pCtx.LastExecution != nil && pCtx.LastExecution.LastAnalyzedCommit != "" {
		lastAnalyzedCommit = pCtx.LastExecution.LastAnalyzedCommit
		lastExecutionResult = pCtx.LastExecution

		forceFullRerun = s.shouldForceFullRerun(ctx, pCtx, pCtx.LastExecution.ExecutionID)
	}

	// 2. Fetch all commits for the pull request
	var allCommits []pipeline.CommitInfo
	if len(pCtx.PrAllCommits) > 0 {
		allCommits = pCtx.PrAllCommits
	} else if s.commitFetcher != nil {
		commits, err := s.commitFetcher.GetCommitsForPR(ctx, pCtx.RepoNamespace, pCtx.PullNumber)
		if err != nil {
			if strings.Contains(err.Error(), "429") || strings.Contains(strings.ToLower(err.Error()), "rate limit") {
				pCtx.SkipReview = true
				pCtx.SkipReason = "Provider rate-limited the commit fetch (HTTP 429)"
				pCtx.StatusInfo = pipeline.PipelineStatusInfo{
					Status:     pipeline.StatusSkipped,
					Message:    "Provider rate-limited the commit fetch (HTTP 429)",
					ReasonCode: "PROVIDER_RATE_LIMITED",
				}
				return nil
			}
			return fmt.Errorf("failed to fetch commits for PR#%d: %w", pCtx.PullNumber, err)
		}
		allCommits = commits
	} else if pCtx.HeadSHA != "" {
		// Fallback synthetic single-commit context if no external fetcher is injected
		allCommits = []pipeline.CommitInfo{
			{SHA: pCtx.HeadSHA, Message: pCtx.Title, Author: pCtx.Author},
		}
	}

	// 3. Filter commits that appeared after lastAnalyzedCommit
	newCommits := allCommits
	lastCommitSHA := lastAnalyzedCommit

	if lastCommitSHA != "" && len(allCommits) > 0 {
		lastCommitIndex := -1
		for idx, commit := range allCommits {
			if commit.SHA == lastCommitSHA {
				lastCommitIndex = idx
				break
			}
		}

		if lastCommitIndex != -1 {
			newCommits = allCommits[lastCommitIndex+1:]
		} else {
			// Base commit is no longer reachable from the PR branch (rebase or force-push history rewrite).
			// Falling back to a full review is the only safe option — using compare(orphan, head)
			// would return diff lines that came from the target branch via rebase, and Drixy would
			// comment on code the author never wrote.
			forceFullRerun = true
			if lastExecutionResult != nil {
				lastExecutionResult.LastAnalyzedCommit = ""
			}
			orphanedBaseCommit = &pipeline.OrphanedBaseCommitInfo{
				PreviousSHA:    lastCommitSHA,
				CurrentHeadSHA: pCtx.HeadSHA,
				TotalCommits:   len(allCommits),
			}
			newCommits = allCommits
		}
	}

	// 4. Validate commit changes
	validationResult := s.validateCommits(pCtx, newCommits, allCommits, lastCommitSHA)
	if !validationResult.CanProceed {
		pCtx.SkipReview = true
		pCtx.SkipReason = validationResult.Message
		pCtx.StatusInfo = pipeline.PipelineStatusInfo{
			Status:     pipeline.StatusSkipped,
			Message:    validationResult.Message,
			ReasonCode: validationResult.ReasonCode,
		}
		pCtx.PrAllCommits = allCommits
		if lastExecutionResult != nil {
			pCtx.LastExecution = lastExecutionResult
		}
		if pCtx.PipelineMetadata == nil {
			pCtx.PipelineMetadata = make(map[string]interface{})
		}
		pCtx.PipelineMetadata["forceFullRerun"] = false
		return nil
	}

	// 5. Populate context with resolved commits
	pCtx.PrCommits = newCommits
	pCtx.PrAllCommits = allCommits
	if lastExecutionResult != nil {
		pCtx.LastExecution = lastExecutionResult
	}
	if orphanedBaseCommit != nil {
		pCtx.OrphanedBaseCommit = orphanedBaseCommit
	}
	if pCtx.PipelineMetadata == nil {
		pCtx.PipelineMetadata = make(map[string]interface{})
	}
	pCtx.PipelineMetadata["forceFullRerun"] = forceFullRerun

	return nil
}

type commitValidationResult struct {
	CanProceed bool
	Message    string
	ReasonCode string
}

func (s *DeepValidateNewCommitsStage) validateCommits(
	pCtx *pipeline.PipelineContext,
	newCommits []pipeline.CommitInfo,
	allCommits []pipeline.CommitInfo,
	lastCommitSHA string,
) commitValidationResult {
	// 1. Force Re-review: `@scandrix review --force` or `@drixy review --force` (or origin == 'command')
	if strings.HasPrefix(pCtx.Origin, "command") {
		return commitValidationResult{
			CanProceed: true,
			Message:    "Proceeding due to manual re-review request",
			ReasonCode: "PROCESSING_MANUAL",
		}
	}

	// 2. No commits found at all
	if len(allCommits) == 0 {
		return commitValidationResult{
			CanProceed: false,
			Message:    "PR has 0 commits to inspect",
			ReasonCode: "NO_COMMITS_IN_PR",
		}
	}

	// 3. No new commits since last analyzed commit
	if len(newCommits) == 0 {
		return commitValidationResult{
			CanProceed: false,
			Message:    fmt.Sprintf("No changes detected since last review (analyzed: %s)", lastCommitSHA),
			ReasonCode: "NO_NEW_COMMITS_SINCE_LAST",
		}
	}

	// 4. Only merge commits found
	if s.checkIfOnlyMergeCommits(newCommits) {
		return commitValidationResult{
			CanProceed: false,
			Message:    "All new commits identified as merge commits from target branch",
			ReasonCode: "ONLY_MERGE_COMMITS_SINCE_LAST",
		}
	}

	return commitValidationResult{CanProceed: true}
}

// checkIfOnlyMergeCommits executes a DAG traversal over commit parents to verify if all new commits are merge commits or their introduced ancestors.
func (s *DeepValidateNewCommitsStage) checkIfOnlyMergeCommits(commits []pipeline.CommitInfo) bool {
	var mergeCommits []pipeline.CommitInfo
	for _, c := range commits {
		if len(c.Parents) > 1 {
			mergeCommits = append(mergeCommits, c)
		}
	}

	if len(mergeCommits) == 0 {
		return false
	}

	allNewCommitSHAs := make(map[string]struct{})
	commitMap := make(map[string]pipeline.CommitInfo)
	for _, c := range commits {
		allNewCommitSHAs[c.SHA] = struct{}{}
		commitMap[c.SHA] = c
	}

	mergedCommitTracker := make(map[string]struct{})
	var stack []string

	for _, commit := range mergeCommits {
		mergedCommitTracker[commit.SHA] = struct{}{}
		// Push second and subsequent parents (branches merged in)
		for i := 1; i < len(commit.Parents); i++ {
			parentSHA := commit.Parents[i]
			if parentSHA != "" {
				stack = append(stack, parentSHA)
			}
		}
	}

	for len(stack) > 0 {
		// Pop
		sha := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if sha == "" {
			continue
		}
		if _, isNew := allNewCommitSHAs[sha]; !isNew {
			continue
		}
		if _, alreadyTracked := mergedCommitTracker[sha]; alreadyTracked {
			continue
		}

		mergedCommitTracker[sha] = struct{}{}
		commit, exists := commitMap[sha]
		if !exists || len(commit.Parents) == 0 {
			continue
		}

		for _, p := range commit.Parents {
			if p != "" {
				stack = append(stack, p)
			}
		}
	}

	return len(mergedCommitTracker) == len(allNewCommitSHAs)
}

func (s *DeepValidateNewCommitsStage) shouldForceFullRerun(ctx context.Context, pCtx *pipeline.PipelineContext, lastExecutionID string) bool {
	// Origin == "command-force" always forces full rerun
	if pCtx.Origin == "command-force" {
		return true
	}

	// Plain command only forces full rerun if previous execution ended in partial or error in rerun-eligible stages
	if pCtx.Origin == "command" && s.commitFetcher != nil && lastExecutionID != "" {
		hasEligibleFailure, err := s.commitFetcher.HasStageWithStatus(
			ctx,
			lastExecutionID,
			s.rerunEligibleStageNames,
			s.rerunEligibleStatuses,
		)
		if err == nil && hasEligibleFailure {
			return true
		}
	}

	return false
}

package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/issues/domain"
)

const (
	issueCreateConcurrency = 5
)

// DrixyIssuesManagementService implements domain.DrixyIssuesManagementService.
type DrixyIssuesManagementService struct {
	logger             *slog.Logger
	issuesService      domain.IssuesService
	pullRequestService domain.PullRequestsIssuesService
	parametersService  domain.ParametersService
	analysisEngine     domain.DrixyIssuesAnalysisEngine
	cacheService       domain.IssueCacheService
	externalTracker    domain.ExternalIssueTracker
}

// NewDrixyIssuesManagementService initializes the Drixy issues management service.
func NewDrixyIssuesManagementService(
	logger *slog.Logger,
	issuesService domain.IssuesService,
	pullRequestService domain.PullRequestsIssuesService,
	parametersService domain.ParametersService,
	analysisEngine domain.DrixyIssuesAnalysisEngine,
	cacheService domain.IssueCacheService,
	externalTracker domain.ExternalIssueTracker,
) *DrixyIssuesManagementService {
	if logger == nil {
		logger = slog.Default()
	}
	return &DrixyIssuesManagementService{
		logger:             logger,
		issuesService:      issuesService,
		pullRequestService: pullRequestService,
		parametersService:  parametersService,
		analysisEngine:     analysisEngine,
		cacheService:       cacheService,
		externalTracker:    externalTracker,
	}
}

// ProcessClosedPR executes automated issue creation, merging, and resolution on PR closure.
func (s *DrixyIssuesManagementService) ProcessClosedPR(ctx context.Context, params domain.ContextToGenerateIssues) error {
	s.logger.Info("starting issue processing for closed PR",
		"prNumber", params.PullRequest.Number,
		"orgId", params.OrganizationAndTeamData.OrganizationID,
		"repo", params.Repository.Name)

	var issuesConfig *domain.IssueCreationConfig
	if s.parametersService != nil {
		cfg, err := s.parametersService.GetIssueCreationConfig(ctx, params.OrganizationAndTeamData)
		if err == nil {
			issuesConfig = cfg
		}
	}

	shouldAutoCreate := true
	if issuesConfig != nil && !issuesConfig.AutomaticCreationEnabled {
		shouldAutoCreate = false
	}

	if shouldAutoCreate {
		// 1. Gather valid suggestions from PR files
		validSuggestions := s.filterValidSuggestions(params.PRFiles)
		filteredSuggestions := s.applyIssuesFilters(issuesConfig, validSuggestions)

		if len(filteredSuggestions) == 0 {
			s.logger.Info("no eligible suggestions found to create issues",
				"prNumber", params.PullRequest.Number)
		} else {
			// 2. Group by file
			grouped := s.groupSuggestionsByFile(filteredSuggestions)

			// 3. Merge or create for each file
			for filePath, fileSuggestions := range grouped {
				if err := s.MergeSuggestionsIntoIssues(ctx, params, filePath, fileSuggestions); err != nil {
					s.logger.Error("error merging suggestions into issues",
						"filePath", filePath,
						"error", err)
				}
			}
		}
	} else {
		s.logger.Info("automatic issue creation is disabled by configuration",
			"orgId", params.OrganizationAndTeamData.OrganizationID)
	}

	// 4. Resolve issues that were fixed in this PR
	if err := s.ResolveExistingIssues(ctx, params, params.PRFiles); err != nil {
		s.logger.Error("error resolving existing issues", "error", err)
	}

	// 5. Update PR synced flag
	if s.pullRequestService != nil {
		_ = s.pullRequestService.UpdateSyncedWithIssuesFlag(
			ctx,
			params.PullRequest.Number,
			params.Repository.ID,
			params.OrganizationAndTeamData.OrganizationID,
			true,
		)
	}

	// 6. Clear cache
	_ = s.ClearIssuesCache(ctx, params.OrganizationAndTeamData.OrganizationID)

	return nil
}

// MergeSuggestionsIntoIssues groups new suggestions against existing open issues or creates new ones.
func (s *DrixyIssuesManagementService) MergeSuggestionsIntoIssues(
	ctx context.Context,
	context domain.ContextToGenerateIssues,
	filePath string,
	newSuggestions []domain.SuggestionItem,
) error {
	if len(newSuggestions) == 0 {
		return nil
	}

	existingIssues, err := s.issuesService.FindByFileAndStatus(
		ctx,
		context.OrganizationAndTeamData.OrganizationID,
		context.Repository.ID,
		filePath,
		domain.StatusOpen,
	)
	if err != nil {
		return fmt.Errorf("failed fetching existing issues: %w", err)
	}

	// If no existing open issues for this file, all suggestions are new issues
	if len(existingIssues) == 0 || s.analysisEngine == nil {
		return s.CreateNewIssues(ctx, context, newSuggestions)
	}

	// Call analysis engine for deduplication
	mergeResult, err := s.analysisEngine.MergeSuggestionsIntoIssues(
		ctx,
		context.OrganizationAndTeamData,
		context.PullRequest,
		map[string]any{
			"filePath":       filePath,
			"existingIssues": existingIssues,
			"newSuggestions": newSuggestions,
		},
	)
	if err != nil {
		s.logger.Warn("AI merge engine failed, falling back to direct creation", "error", err)
		return s.CreateNewIssues(ctx, context, newSuggestions)
	}

	matchedMap := make(map[string]string)
	if mergeResult != nil {
		for _, m := range mergeResult.Matches {
			if m.ExistingIssueID != "" {
				matchedMap[m.SuggestionID] = m.ExistingIssueID
			}
		}
	}

	var unmatched []domain.SuggestionItem
	for _, sugg := range newSuggestions {
		if issueID, ok := matchedMap[sugg.ID]; ok {
			_, _ = s.issuesService.AddSuggestionIDs(ctx, issueID, []string{sugg.ID})
		} else {
			unmatched = append(unmatched, sugg)
		}
	}

	if len(unmatched) > 0 {
		return s.CreateNewIssues(ctx, context, unmatched)
	}
	return nil
}

// CreateNewIssues converts unmatched suggestions into persisted issues with bounded concurrency.
func (s *DrixyIssuesManagementService) CreateNewIssues(
	ctx context.Context,
	context domain.ContextToGenerateIssues,
	unmatchedSuggestions []domain.SuggestionItem,
) error {
	if len(unmatchedSuggestions) == 0 {
		return nil
	}

	sem := make(chan struct{}, issueCreateConcurrency)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error

	now := time.Now().UTC()
	for _, item := range unmatchedSuggestions {
		sugg := item
		wg.Add(1)
		sem <- struct{}{}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			newIssue := &domain.Issue{
				Title:       sugg.OneSentenceSummary,
				Description: sugg.SuggestionContent,
				FilePath:    sugg.RelevantFile,
				Language:    sugg.Language,
				Label:       sugg.Label,
				Severity:    sugg.Severity,
				Status:      domain.StatusOpen,
				ContributingSuggestions: []domain.ContributingSuggestion{
					{
						ID:                 sugg.ID,
						PRNumber:           context.PullRequest.Number,
						PRAuthor:           context.PullRequest.User,
						SuggestionContent:  sugg.SuggestionContent,
						OneSentenceSummary: sugg.OneSentenceSummary,
						RelevantFile:       sugg.RelevantFile,
						Language:           sugg.Language,
						ExistingCode:       sugg.ExistingCode,
						ImprovedCode:       sugg.ImprovedCode,
						StartLine:          sugg.StartLine,
						EndLine:            sugg.EndLine,
						BrokenDrixyRulesIDs: sugg.BrokenDrixyRulesIDs,
					},
				},
				Repository:     context.Repository,
				OrganizationID: context.OrganizationAndTeamData.OrganizationID,
				Owner:          &context.PullRequest.User,
				Reporter: &domain.UserRef{
					GitID:    "scandrix",
					Username: "ScanDrix",
					Name:     "ScanDrix",
				},
				CreatedAt: now,
				UpdatedAt: now,
			}

			created, err := s.issuesService.Create(ctx, newIssue)
			if err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
				return
			}

			// Synchronize with external tracker if available
			if s.externalTracker != nil && created != nil {
				_, _ = s.externalTracker.CreateIssue(ctx, created)
			}
		}()
	}

	wg.Wait()
	return firstErr
}

// ResolveExistingIssues detects removed files or verified code fixes and marks issues resolved or dismissed.
func (s *DrixyIssuesManagementService) ResolveExistingIssues(
	ctx context.Context,
	context domain.ContextToGenerateIssues,
	files []domain.PRFileInfo,
) error {
	if len(files) == 0 {
		return nil
	}

	for _, file := range files {
		openIssues, err := s.issuesService.FindByFileAndStatus(
			ctx,
			context.OrganizationAndTeamData.OrganizationID,
			context.Repository.ID,
			file.Path,
			domain.StatusOpen,
		)
		if err != nil || len(openIssues) == 0 {
			continue
		}

		if file.Status == "removed" {
			var ids []string
			for _, iss := range openIssues {
				ids = append(ids, iss.UUID)
			}
			_, _ = s.issuesService.UpdateStatusByIds(ctx, ids, domain.StatusDismissed)
			continue
		}

		if s.analysisEngine != nil {
			res, err := s.analysisEngine.ResolveExistingIssues(
				ctx,
				context.OrganizationAndTeamData,
				context.PullRequest,
				map[string]any{
					"filePath":    file.Path,
					"currentCode": file.FileContent,
					"issues":      openIssues,
				},
			)
			if err == nil && res != nil {
				for _, verification := range res.IssueVerificationResults {
					if !verification.IsIssuePresentInCode {
						_, _ = s.issuesService.UpdateStatus(ctx, verification.IssueID, domain.StatusResolved)
						if s.externalTracker != nil {
							_ = s.externalTracker.UpdateIssueStatus(ctx, verification.IssueID, domain.StatusResolved)
						}
					}
				}
			}
		}
	}
	return nil
}

// AgeCalculation formats the issue age into human-readable text.
func (s *DrixyIssuesManagementService) AgeCalculation(createdAt time.Time) string {
	now := time.Now().UTC()
	diffHours := now.Sub(createdAt).Hours()
	diffDays := int(math.Ceil(diffHours / 24.0))
	if diffDays <= 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", diffDays)
}

// BuildFilter converts query DTOs into structured MongoDB/in-memory query maps.
func (s *DrixyIssuesManagementService) BuildFilter(ctx context.Context, filters domain.GetIssuesFilter) (map[string]any, error) {
	filter := make(map[string]any)

	if filters.Title != "" {
		filter["title"] = regexp.MustCompile("(?i)" + regexp.QuoteMeta(filters.Title))
	}
	if filters.Severity != "" {
		filter["severity"] = filters.Severity
	}
	if filters.Category != "" {
		filter["category"] = filters.Category
	}
	if filters.OrganizationID != "" {
		filter["organizationId"] = filters.OrganizationID
	}
	if filters.FilePath != "" {
		filter["filePath"] = filters.FilePath
	}
	if filters.Status != "" {
		filter["status"] = filters.Status
	}
	if filters.RepositoryName != "" {
		filter["repository.name"] = regexp.MustCompile("(?i)" + regexp.QuoteMeta(filters.RepositoryName))
	}
	if len(filters.RepositoryIDs) > 0 {
		filter["repository.id"] = filters.RepositoryIDs
	}
	if filters.BeforeAt != nil || filters.AfterAt != nil {
		dateFilter := make(map[string]any)
		if filters.BeforeAt != nil {
			dateFilter["$lt"] = *filters.BeforeAt
		}
		if filters.AfterAt != nil {
			dateFilter["$gt"] = *filters.AfterAt
		}
		filter["createdAt"] = dateFilter
	}

	return filter, nil
}

// EnrichContributingSuggestions fills in full suggestion details from pull request records.
func (s *DrixyIssuesManagementService) EnrichContributingSuggestions(
	ctx context.Context,
	suggestions []domain.ContributingSuggestion,
	orgID string,
) ([]domain.ContributingSuggestion, error) {
	if len(suggestions) == 0 || s.pullRequestService == nil {
		return suggestions, nil
	}

	cache := make(map[int]map[string]domain.SuggestionItem)
	enriched := make([]domain.ContributingSuggestion, len(suggestions))

	for i, cs := range suggestions {
		enriched[i] = cs
		if cs.PRNumber <= 0 || cs.ID == "" {
			continue
		}

		if _, exists := cache[cs.PRNumber]; !exists {
			items, err := s.pullRequestService.FindSuggestionsByPR(ctx, orgID, cs.PRNumber, "sent")
			if err == nil {
				sMap := make(map[string]domain.SuggestionItem, len(items))
				for _, it := range items {
					sMap[it.ID] = it
				}
				cache[cs.PRNumber] = sMap
			}
		}

		if sMap, ok := cache[cs.PRNumber]; ok {
			if full, found := sMap[cs.ID]; found {
				enriched[i].ExistingCode = full.ExistingCode
				enriched[i].ImprovedCode = full.ImprovedCode
				enriched[i].StartLine = full.StartLine
				enriched[i].EndLine = full.EndLine
				enriched[i].OneSentenceSummary = full.OneSentenceSummary
				enriched[i].SuggestionContent = full.SuggestionContent
				enriched[i].Language = full.Language
				enriched[i].RelevantFile = full.RelevantFile
				enriched[i].BrokenDrixyRulesIDs = full.BrokenDrixyRulesIDs
			}
		}
	}

	return enriched, nil
}

// ClearIssuesCache evicts cached issue lists for the given organization.
func (s *DrixyIssuesManagementService) ClearIssuesCache(ctx context.Context, orgID string) error {
	if s.cacheService == nil || orgID == "" {
		return nil
	}
	cacheKey := fmt.Sprintf("issues_%s", orgID)
	return s.cacheService.Delete(ctx, cacheKey)
}

func (s *DrixyIssuesManagementService) filterValidSuggestions(files []domain.PRFileInfo) []domain.SuggestionItem {
	var valid []domain.SuggestionItem
	for _, f := range files {
		for _, sugg := range f.Suggestions {
			if sugg.ImplementationStatus == domain.ImplementationImplemented {
				continue
			}
			if sugg.PriorityStatus == domain.PriorityDiscardedBySafeguard ||
				sugg.PriorityStatus == domain.PriorityDiscardedByDrixyFineTuning ||
				sugg.PriorityStatus == domain.PriorityDiscardedByCodeDiff {
				continue
			}
			if sugg.RelevantFile == "" {
				sugg.RelevantFile = f.Path
			}
			valid = append(valid, sugg)
		}
	}
	return valid
}

func (s *DrixyIssuesManagementService) applyIssuesFilters(
	cfg *domain.IssueCreationConfig,
	suggestions []domain.SuggestionItem,
) []domain.SuggestionItem {
	if cfg == nil {
		return suggestions
	}

	var filtered []domain.SuggestionItem
	for _, sugg := range suggestions {
		if cfg.SeverityFilters.MinimumSeverity != "" {
			minIdx := severityIndex(cfg.SeverityFilters.MinimumSeverity)
			suggIdx := severityIndex(sugg.Severity)
			if suggIdx > minIdx {
				continue
			}
		}

		if len(cfg.SeverityFilters.AllowedSeverities) > 0 {
			allowed := false
			for _, sev := range cfg.SeverityFilters.AllowedSeverities {
				if sugg.Severity == sev {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}

		if !cfg.SourceFilters.IncludeCodeReviewEngine && sugg.Label != string(domain.LabelDrixyRules) {
			continue
		}
		if !cfg.SourceFilters.IncludeDrixyRules && sugg.Label == string(domain.LabelDrixyRules) {
			continue
		}

		filtered = append(filtered, sugg)
	}
	return filtered
}

func (s *DrixyIssuesManagementService) groupSuggestionsByFile(suggestions []domain.SuggestionItem) map[string][]domain.SuggestionItem {
	grouped := make(map[string][]domain.SuggestionItem)
	for _, s := range suggestions {
		grouped[s.RelevantFile] = append(grouped[s.RelevantFile], s)
	}
	return grouped
}

func severityIndex(sev domain.SeverityLevel) int {
	for i, s := range domain.SeverityOrder {
		if s == sev {
			return i
		}
	}
	return len(domain.SeverityOrder)
}

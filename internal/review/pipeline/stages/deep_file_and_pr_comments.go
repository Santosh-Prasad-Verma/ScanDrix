package stages

import (
	"context"
	"log/slog"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// AddedLineAnchor models GitLab anchor resolution results onto added (+) diff lines.
type AddedLineAnchor struct {
	Line      int   `json:"line"`
	StartLine *int  `json:"start_line,omitempty"`
	Snapped   bool  `json:"snapped"`
}

// ResolveAddedLineAnchor snaps comments to an added (+) line within the specified span.
func ResolveAddedLineAnchor(validDiffLines [][2]int, targetStart *int, targetEnd int) *AddedLineAnchor {
	anchor := targetEnd
	if targetStart != nil && *targetStart > 0 {
		anchor = *targetStart
	}

	spanStart := int(math.Min(float64(anchor), float64(targetEnd)))
	spanEnd := int(math.Max(float64(anchor), float64(targetEnd)))

	isAdded := func(n int) bool {
		for _, r := range validDiffLines {
			if n >= r[0] && n <= r[1] {
				return true
			}
		}
		return false
	}

	// If the anchor line is already on an added line, keep original bounds
	if isAdded(anchor) {
		return &AddedLineAnchor{
			Line:      targetEnd,
			StartLine: targetStart,
			Snapped:   false,
		}
	}

	// Find added lines falling within [spanStart, spanEnd]
	inSpanAdded := []int{}
	for _, r := range validDiffLines {
		from := int(math.Max(float64(r[0]), float64(spanStart)))
		to := int(math.Min(float64(r[1]), float64(spanEnd)))
		for n := from; n <= to; n++ {
			inSpanAdded = append(inSpanAdded, n)
		}
	}

	if len(inSpanAdded) == 0 {
		return nil
	}

	// Sort by distance to anchor; ties break to lower line number
	sort.Slice(inSpanAdded, func(i, j int) bool {
		distI := int(math.Abs(float64(inSpanAdded[i] - anchor)))
		distJ := int(math.Abs(float64(inSpanAdded[j] - anchor)))
		if distI != distJ {
			return distI < distJ
		}
		return inSpanAdded[i] < inSpanAdded[j]
	})

	return &AddedLineAnchor{
		Line:      inSpanAdded[0],
		StartLine: nil,
		Snapped:   true,
	}
}

// CalculateCommentStartLine computes the multi-line comment start boundary.
func CalculateCommentStartLine(start, end int) *int {
	if start <= 0 || start == end {
		return nil
	}
	if start+15 > end {
		val := start
		return &val
	}
	return nil
}

// CalculateCommentEndLine computes the effective end line for SCM inline placement.
func CalculateCommentEndLine(start, end int) int {
	if start <= 0 || start == end {
		return end
	}
	if start+15 > end {
		return end
	}
	return start
}

// PRLevelCommentManager defines contract for posting PR overview comments.
type PRLevelCommentManager interface {
	CreatePrLevelReviewComments(
		ctx context.Context,
		workspaceID string,
		prNumber int,
		repoName string,
		suggestions []domain.CodeSuggestion,
		languageResultPrompt string,
		suggestionCopyPrompt bool,
	) ([]domain.LineCommentResult, error)
}

// PRLevelStorageService handles database persistence of PR level suggestions.
type PRLevelStorageService interface {
	AddPrLevelSuggestions(
		ctx context.Context,
		prNumber int,
		repoName string,
		suggestions []domain.CodeSuggestion,
		workspaceID string,
	) error
}

// DeepCreatePrLevelCommentsStage posts top-level pull request suggestions.
type DeepCreatePrLevelCommentsStage struct {
	logger         *slog.Logger
	commentManager PRLevelCommentManager
	storageService PRLevelStorageService
}

// NewDeepCreatePrLevelCommentsStage instantiates Stage 11.
func NewDeepCreatePrLevelCommentsStage(
	logger *slog.Logger,
	cm PRLevelCommentManager,
	ss PRLevelStorageService,
) *DeepCreatePrLevelCommentsStage {
	if logger == nil {
		logger = slog.Default()
	}
	return &DeepCreatePrLevelCommentsStage{
		logger:         logger.With("stage", "DeepCreatePrLevelCommentsStage"),
		commentManager: cm,
		storageService: ss,
	}
}

// Name returns the stage identifier.
func (s *DeepCreatePrLevelCommentsStage) Name() string {
	return "DeepCreatePrLevelCommentsStage"
}

// Execute processes and posts PR-level suggestions and business logic findings.
func (s *DeepCreatePrLevelCommentsStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if pCtx.WorkspaceID == uuid.Nil || pCtx.PullNumber == 0 || pCtx.RepositoryID == uuid.Nil {
		s.logger.Error("Missing required context fields for PR level comments",
			"workspace_id", pCtx.WorkspaceID.String(),
			"pr_number", pCtx.PullNumber,
		)
		return nil
	}

	prLevelSuggestions := make([]domain.CodeSuggestion, 0, len(pCtx.PRLevelSuggestions)+len(pCtx.BusinessLogicResults))
	prLevelSuggestions = append(prLevelSuggestions, pCtx.PRLevelSuggestions...)
	prLevelSuggestions = append(prLevelSuggestions, pCtx.BusinessLogicResults...)

	if len(prLevelSuggestions) == 0 {
		s.logger.Info("No PR-level suggestions to process", "pr_number", pCtx.PullNumber)
		return nil
	}

	s.logger.Info("Starting PR-level comments creation",
		"pr_number", pCtx.PullNumber,
		"suggestions_count", len(prLevelSuggestions),
	)

	results, err := s.commentManager.CreatePrLevelReviewComments(
		ctx,
		pCtx.WorkspaceID.String(),
		pCtx.PullNumber,
		pCtx.RepoNamespace,
		prLevelSuggestions,
		pCtx.ResolvedConfig.LanguageResultPrompt,
		false,
	)
	if err != nil {
		s.logger.Error("Error creating PR level comments", "error", err, "pr_number", pCtx.PullNumber)
		pCtx.AddError(s.Name(), "CreatePrLevelReviewComments", err, "partial", map[string]interface{}{
			"pr_number": pCtx.PullNumber,
		})
		results = []domain.LineCommentResult{}
	}

	deliveredCount := 0
	for _, res := range results {
		if res.DeliveryStatus == domain.DeliveryStatusSent {
			deliveredCount++
		}
	}
	failedCount := len(results) - deliveredCount

	s.logger.Info("Finished posting PR-level comments",
		"pr_number", pCtx.PullNumber,
		"delivered", deliveredCount,
		"failed", failedCount,
	)

	// Save PR-level suggestions to storage
	if s.storageService != nil && len(prLevelSuggestions) > 0 {
		if saveErr := s.storageService.AddPrLevelSuggestions(
			ctx,
			pCtx.PullNumber,
			pCtx.RepoNamespace,
			prLevelSuggestions,
			pCtx.WorkspaceID.String(),
		); saveErr != nil {
			s.logger.Error("Failed saving PR level suggestions to database", "error", saveErr)
		}
	}

	pCtx.PRLevelCommentResults = append(pCtx.PRLevelCommentResults, results...)
	return nil
}

// SuggestionSyntaxValidator checks syntax validity in sandbox or AST.
type SuggestionSyntaxValidator interface {
	ValidateSyntax(ctx context.Context, filePath, code string) (bool, error)
}

// SuggestionLLMValidator performs semantic patch and committability verification.
type SuggestionLLMValidator interface {
	ValidateCommittable(ctx context.Context, filePath, diff, suggestion string) (bool, error)
}

// DeepValidateSuggestionsStage validates committable suggestions against syntax and AST limits.
type DeepValidateSuggestionsStage struct {
	logger          *slog.Logger
	syntaxValidator SuggestionSyntaxValidator
	llmValidator    SuggestionLLMValidator
}

// NewDeepValidateSuggestionsStage instantiates Stage 12.
func NewDeepValidateSuggestionsStage(
	logger *slog.Logger,
	syntaxVal SuggestionSyntaxValidator,
	llmVal SuggestionLLMValidator,
) *DeepValidateSuggestionsStage {
	if logger == nil {
		logger = slog.Default()
	}
	return &DeepValidateSuggestionsStage{
		logger:          logger.With("stage", "DeepValidateSuggestionsStage"),
		syntaxValidator: syntaxVal,
		llmValidator:    llmVal,
	}
}

// Name returns stage identifier.
func (s *DeepValidateSuggestionsStage) Name() string {
	return "DeepValidateSuggestionsStage"
}

var supportedCodeExtensions = map[string]bool{
	".go":    true,
	".ts":    true,
	".tsx":   true,
	".js":    true,
	".jsx":   true,
	".py":    true,
	".java":  true,
	".rs":    true,
	".c":     true,
	".cpp":   true,
	".cs":    true,
	".rb":    true,
	".php":   true,
	".swift": true,
	".kt":    true,
}

// Execute filters and validates committable replacement suggestions.
func (s *DeepValidateSuggestionsStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if !pCtx.ResolvedConfig.EnableCommittableSuggestions {
		s.logger.Info("Committable suggestions disabled in configuration", "pr_number", pCtx.PullNumber)
		return nil
	}

	// GitHub is the primary SCM supporting committable suggestions
	if pCtx.Provider != models.SCMProviderGitHub {
		s.logger.Info("Skipping committable suggestion validation for non-GitHub platform",
			"provider", pCtx.Provider,
			"pr_number", pCtx.PullNumber,
		)
		return nil
	}

	if len(pCtx.ValidSuggestions) == 0 || len(pCtx.ChangedFiles) == 0 {
		return nil
	}

	const (
		concurrencyLimit = 10
		maxLinesLimit    = 15
		maxCharsLimit    = 1000
	)

	sem := make(chan struct{}, concurrencyLimit)
	var wg sync.WaitGroup
	var mu sync.Mutex

	updatedSuggestions := make([]domain.CodeSuggestion, len(pCtx.ValidSuggestions))
	copy(updatedSuggestions, pCtx.ValidSuggestions)

	filesMap := make(map[string]pipeline.FileChangeInfo)
	for _, f := range pCtx.ChangedFiles {
		filesMap[f.Filename] = f
	}

	for i := range updatedSuggestions {
		idx := i
		sug := &updatedSuggestions[idx]

		filePath := sug.GetFilePath()
		ext := strings.ToLower(filepath.Ext(filePath))
		if !supportedCodeExtensions[ext] {
			sug.IsCommittable = false
			sug.ValidatedData = nil
			continue
		}

		code := sug.ImprovedCode
		linesCount := strings.Count(code, "\n") + 1
		charsCount := len(code)

		if linesCount >= maxLinesLimit || charsCount >= maxCharsLimit {
			sug.IsCommittable = false
			sug.ValidatedData = nil
			continue
		}

		fileInfo, ok := filesMap[filePath]
		if !ok || fileInfo.Status == "removed" {
			sug.IsCommittable = false
			sug.ValidatedData = nil
			continue
		}

		wg.Add(1)
		go func(targetSug *domain.CodeSuggestion) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			startLine := targetSug.GetStartLine()
			endLine := targetSug.GetEndLine()

			// Check syntax
			if s.syntaxValidator != nil {
				syntaxValid, err := s.syntaxValidator.ValidateSyntax(ctx, filePath, targetSug.ImprovedCode)
				if err != nil || !syntaxValid {
					targetSug.IsCommittable = false
					targetSug.ValidatedData = nil
					return
				}
			}

			// Check LLM / AST committability
			if s.llmValidator != nil {
				valid, err := s.llmValidator.ValidateCommittable(ctx, filePath, fileInfo.Patch, targetSug.ImprovedCode)
				if err != nil || !valid {
					targetSug.IsCommittable = false
					targetSug.ValidatedData = nil
					return
				}
			}

			mu.Lock()
			targetSug.IsCommittable = true
			targetSug.ValidatedData = &domain.ValidatedDiffData{
				Code:      targetSug.ImprovedCode,
				Diff:      fileInfo.Patch,
				LineStart: startLine,
				LineEnd:   endLine,
			}
			mu.Unlock()
		}(sug)
	}

	wg.Wait()
	pCtx.ValidSuggestions = updatedSuggestions
	return nil
}

// FileCommentManager abstracts inline SCM comment publishing.
type FileCommentManager interface {
	CreateLineComments(
		ctx context.Context,
		orgID string,
		repo models.TrackedRepository,
		prNumber int,
		comments []domain.LineCommentRequest,
	) ([]domain.LineCommentResult, error)
}

// ImplementedSuggestionResolver reconciles addressed suggestions on git platform.
type ImplementedSuggestionResolver interface {
	ResolveImplementedSuggestionsOnPlatform(
		ctx context.Context,
		workspaceID string,
		repo models.TrackedRepository,
		prNumber int,
		provider models.SCMProvider,
	) error
}

// PullRequestReviewPersistence records the completed review results in persistent storage.
type PullRequestReviewPersistence interface {
	SavePullRequestReview(
		ctx context.Context,
		workspaceID string,
		prNumber int,
		repo models.TrackedRepository,
		changedFiles []pipeline.FileChangeInfo,
		prioritizedSuggestions []domain.CodeSuggestion,
		discardedSuggestions []domain.CodeSuggestion,
		provider models.SCMProvider,
		commits []pipeline.CommitInfo,
		heavy bool,
	) error
}

// DeepCreateFileCommentsStage posts sorted, deduplicated, and anchor-resolved inline comments.
type DeepCreateFileCommentsStage struct {
	logger              *slog.Logger
	commentManager      FileCommentManager
	implementedResolver ImplementedSuggestionResolver
	persistence         PullRequestReviewPersistence
}

// NewDeepCreateFileCommentsStage instantiates Stage 13.
func NewDeepCreateFileCommentsStage(
	logger *slog.Logger,
	cm FileCommentManager,
	ir ImplementedSuggestionResolver,
	pers PullRequestReviewPersistence,
) *DeepCreateFileCommentsStage {
	if logger == nil {
		logger = slog.Default()
	}
	return &DeepCreateFileCommentsStage{
		logger:              logger.With("stage", "DeepCreateFileCommentsStage"),
		commentManager:      cm,
		implementedResolver: ir,
		persistence:         pers,
	}
}

// Name returns stage identifier.
func (s *DeepCreateFileCommentsStage) Name() string {
	return "DeepCreateFileCommentsStage"
}

// Execute performs sorting, clustering discard, GitLab anchor snapping, and inline posting.
func (s *DeepCreateFileCommentsStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if pCtx.WorkspaceID == uuid.Nil || pCtx.PullNumber == 0 || pCtx.RepositoryID == uuid.Nil {
		s.logger.Error("Missing required context fields in DeepCreateFileCommentsStage",
			"workspace_id", pCtx.WorkspaceID.String(),
			"pr_number", pCtx.PullNumber,
		)
		return nil
	}

	trackedRepo := models.TrackedRepository{
		ID:            pCtx.RepositoryID,
		NamespacePath: pCtx.RepoNamespace,
		Provider:      pCtx.Provider,
	}

	// Step 1: Resolve suggestions that were partially or fully implemented
	if s.implementedResolver != nil {
		if err := s.implementedResolver.ResolveImplementedSuggestionsOnPlatform(
			ctx,
			pCtx.WorkspaceID.String(),
			trackedRepo,
			pCtx.PullNumber,
			pCtx.Provider,
		); err != nil {
			s.logger.Warn("Failed to resolve implemented suggestions on platform", "error", err)
		}
	}

	// Step 2: If no valid suggestions exist, save state and exit early
	if len(pCtx.ValidSuggestions) == 0 {
		s.logger.Info("No file-level suggestions to process",
			"pr_number", pCtx.PullNumber,
			"discarded_count", len(pCtx.DiscardedSuggestions),
		)

		if s.persistence != nil {
			if err := s.persistence.SavePullRequestReview(
				ctx,
				pCtx.WorkspaceID.String(),
				pCtx.PullNumber,
				trackedRepo,
				pCtx.ChangedFiles,
				[]domain.CodeSuggestion{},
				pCtx.DiscardedSuggestions,
				pCtx.Provider,
				pCtx.PrCommits,
				pCtx.ResolvedConfig.ReviewHeavyMode,
			); err != nil {
				s.logger.Error("Failed saving pull request review without suggestions", "error", err)
			}
		}

		pCtx.LineCommentResults = []domain.LineCommentResult{}
		return nil
	}

	// Step 3: Sort suggestions by FilePath ASC, then Severity Rank DESC
	severityRank := func(sev domain.ReviewSeverity) int {
		switch sev {
		case domain.SeverityCritical:
			return 4
		case domain.SeverityHigh, domain.SeverityMajor:
			return 3
		case domain.SeverityMedium:
			return 2
		case domain.SeverityLow, domain.SeverityMinor:
			return 1
		default:
			return 0
		}
	}

	sortedSuggestions := make([]domain.CodeSuggestion, len(pCtx.ValidSuggestions))
	copy(sortedSuggestions, pCtx.ValidSuggestions)

	sort.Slice(sortedSuggestions, func(i, j int) bool {
		fileA := sortedSuggestions[i].GetFilePath()
		fileB := sortedSuggestions[j].GetFilePath()
		if fileA != fileB {
			return fileA < fileB
		}
		rankA := severityRank(sortedSuggestions[i].Severity)
		rankB := severityRank(sortedSuggestions[j].Severity)
		return rankA > rankB
	})

	// Step 4: Clustering & Deleted File Filters
	removedFiles := make(map[string]bool)
	patchByFile := make(map[string]pipeline.FileChangeInfo)
	for _, f := range pCtx.ChangedFiles {
		if f.Status == "removed" || f.Status == "deleted" {
			removedFiles[f.Filename] = true
		}
		patchByFile[f.Filename] = f
	}

	candidateSuggestions := make([]domain.CodeSuggestion, 0, len(sortedSuggestions))
	for _, sug := range sortedSuggestions {
		// Child cluster items are merged into parent, mark discarded
		if sug.Clustering != nil && sug.Clustering.Type == domain.ClusteringTypeChild {
			sug.PriorityStatus = domain.PriorityStatusDiscardedByClustering
			pCtx.DiscardedSuggestions = append(pCtx.DiscardedSuggestions, sug)
			continue
		}

		// Discard suggestions on removed files
		if removedFiles[sug.GetFilePath()] {
			sug.PriorityStatus = domain.PriorityStatusDiscardedBySafeguard
			sug.DiscardReason = "File removed in PR"
			pCtx.DiscardedSuggestions = append(pCtx.DiscardedSuggestions, sug)
			continue
		}

		candidateSuggestions = append(candidateSuggestions, sug)
	}

	// Step 5: GitLab Anchor Resolution & Line Boundary Clamping
	isGitLab := pCtx.Provider == models.SCMProviderGitLab
	lineComments := make([]domain.LineCommentRequest, 0, len(candidateSuggestions))
	finalPrioritizedSuggestions := make([]domain.CodeSuggestion, 0, len(candidateSuggestions))

	for _, sug := range candidateSuggestions {
		filePath := sug.GetFilePath()
		startLine := sug.GetStartLine()
		endLine := sug.GetEndLine()

		if isGitLab {
			fileInfo, hasFile := patchByFile[filePath]
			if hasFile && len(fileInfo.ValidDiffLines) > 0 {
				var startPtr *int
				if startLine > 0 {
					startPtr = &startLine
				}
				anchor := ResolveAddedLineAnchor(fileInfo.ValidDiffLines, startPtr, endLine)
				if anchor == nil {
					// Discard comment since it doesn't touch added code on GitLab
					sug.PriorityStatus = domain.PriorityStatusDiscardedByCodeDiff
					sug.DiscardReason = "GitLab comment anchor has no added line in diff span"
					pCtx.DiscardedSuggestions = append(pCtx.DiscardedSuggestions, sug)
					continue
				}

				if anchor.StartLine != nil {
					startLine = *anchor.StartLine
				} else {
					startLine = 0
				}
				endLine = anchor.Line
			}
		}

		calcStart := CalculateCommentStartLine(startLine, endLine)
		calcEnd := CalculateCommentEndLine(startLine, endLine)

		startVal := 0
		if calcStart != nil {
			startVal = *calcStart
		}

		commentReq := domain.LineCommentRequest{
			FilePath:        filePath,
			LineNumber:      calcEnd,
			StartLineNumber: startVal,
			Side:            "RIGHT",
			Body:            sug.SuggestionContent,
			Suggestion:      &sug,
		}

		lineComments = append(lineComments, commentReq)
		finalPrioritizedSuggestions = append(finalPrioritizedSuggestions, sug)
	}

	// Step 6: Post inline comments to remote SCM
	s.logger.Info("Posting inline file comments",
		"pr_number", pCtx.PullNumber,
		"comments_count", len(lineComments),
	)

	var commentResults []domain.LineCommentResult
	if len(lineComments) > 0 {
		var err error
		commentResults, err = s.commentManager.CreateLineComments(
			ctx,
			pCtx.WorkspaceID.String(),
			trackedRepo,
			pCtx.PullNumber,
			lineComments,
		)
		if err != nil {
			s.logger.Error("Failed posting line comments", "error", err, "pr_number", pCtx.PullNumber)
			pCtx.AddError(s.Name(), "CreateLineComments", err, "partial", map[string]interface{}{
				"pr_number": pCtx.PullNumber,
			})
			commentResults = []domain.LineCommentResult{}
		}
	}

	// Step 7: Persist review results
	if s.persistence != nil {
		if err := s.persistence.SavePullRequestReview(
			ctx,
			pCtx.WorkspaceID.String(),
			pCtx.PullNumber,
			trackedRepo,
			pCtx.ChangedFiles,
			finalPrioritizedSuggestions,
			pCtx.DiscardedSuggestions,
			pCtx.Provider,
			pCtx.PrCommits,
			pCtx.ResolvedConfig.ReviewHeavyMode,
		); err != nil {
			s.logger.Error("Error persisting pull request suggestions", "error", err)
		}
	}

	pCtx.LineCommentResults = commentResults
	return nil
}

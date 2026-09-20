package usecases

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/clireview/domain"
	"github.com/scandrix/backend/internal/clireview/pipeline"
	"github.com/scandrix/backend/internal/rules"
)

// ExecuteCliReviewUseCase orchestrates CLI input conversion, configuration merging,
// Drixy rules resolution, pipeline execution, and automation telemetry.
type ExecuteCliReviewUseCase struct {
	converter                  *pipeline.CliInputConverter
	pipelineStrategy           *pipeline.CliReviewPipelineStrategy
	codeManagementService      domain.ICodeManagementService
	parametersService          domain.IParametersService
	automationExecutionService domain.IAutomationExecutionService
	drixyRulesService          domain.IDrixyRulesService
	logger                     *slog.Logger
	rulesEvaluator             *rules.Evaluator
}

// NewExecuteCliReviewUseCase creates an initialized ExecuteCliReviewUseCase.
func NewExecuteCliReviewUseCase(
	converter *pipeline.CliInputConverter,
	pipelineStrategy *pipeline.CliReviewPipelineStrategy,
	codeManagementService domain.ICodeManagementService,
	parametersService domain.IParametersService,
	automationExecutionService domain.IAutomationExecutionService,
	drixyRulesService domain.IDrixyRulesService,
	rulesEvaluator *rules.Evaluator,
) *ExecuteCliReviewUseCase {
	if converter == nil {
		converter = pipeline.NewCliInputConverter()
	}
	return &ExecuteCliReviewUseCase{
		converter:                  converter,
		pipelineStrategy:           pipelineStrategy,
		codeManagementService:      codeManagementService,
		parametersService:          parametersService,
		automationExecutionService: automationExecutionService,
		drixyRulesService:          drixyRulesService,
		rulesEvaluator:             rulesEvaluator,
		logger:                     slog.Default().With("usecase", "ExecuteCliReviewUseCase"),
	}
}

// Execute runs the full CLI review orchestration flow.
func (uc *ExecuteCliReviewUseCase) Execute(ctx context.Context, params domain.ExecuteCliReviewInput) (*domain.CliReviewResponse, error) {
	correlationID := uc.generateCorrelationID()
	startTime := time.Now().UTC()
	var execution *domain.AutomationExecutionRecord

	isFastMode := false
	if params.Input.Config != nil && params.Input.Config.Fast {
		isFastMode = true
	}

	uc.logger.Info("Starting CLI review",
		"orgId", params.OrganizationAndTeamData.OrganizationID,
		"teamId", params.OrganizationAndTeamData.TeamID,
		"correlationId", correlationID,
		"isTrialMode", params.IsTrialMode,
		"isFastMode", isFastMode,
	)

	// 1. Create automation execution for tracking (skipped in trial mode).
	if !params.IsTrialMode && uc.automationExecutionService != nil {
		execRecord, err := uc.createAutomationExecution(ctx, params, correlationID)
		if err != nil {
			uc.logger.Error("Failed to create automation execution", "error", err)
		} else {
			execution = execRecord
		}
	}

	defer func() {
		if r := recover(); r != nil {
			if execution != nil && uc.automationExecutionService != nil {
				_ = uc.automationExecutionService.Update(
					ctx,
					execution.UUID,
					"error",
					map[string]any{"panic": fmt.Sprintf("%v", r)},
					"",
					fmt.Sprintf("Panic during review: %v", r),
				)
			}
			panic(r)
		}
	}()

	// 2. Convert CLI input to FileChanges
	changedFiles := uc.converter.ConvertToFileChanges(params.Input)

	if len(changedFiles) == 0 {
		uc.logger.Warn("No files to analyze after conversion",
			"correlationId", correlationID,
			"orgId", params.OrganizationAndTeamData.OrganizationID,
		)
		duration := time.Since(startTime).Milliseconds()
		return &domain.CliReviewResponse{
			Summary:       "No files to analyze",
			Issues:        []domain.CliReviewIssue{},
			FilesAnalyzed: 0,
			Duration:      duration,
		}, nil
	}

	// 3. Load or create config and resolve repository
	var codeReviewConfig *domain.CodeReviewConfig
	var resolvedRepoID string
	var resolvedRepoName *string

	if params.IsTrialMode {
		codeReviewConfig = uc.GetDefaultConfig()
		resolvedRepoID = "global"
		resolvedRepoName = nil
	} else {
		cfg, rID, rName := uc.LoadUserConfigWithRules(ctx, params.OrganizationAndTeamData, params.GitContext)
		codeReviewConfig = cfg
		resolvedRepoID = rID
		resolvedRepoName = rName
	}

	// 4. Effective review configuration
	if isFastMode {
		codeReviewConfig.ReviewMode = "fast"
	}

	resolvedPlatform := uc.ResolveCliPlatform(ctx, params.OrganizationAndTeamData, params.GitContext, params.IsTrialMode)

	// Direct steering instruction
	var focusDirective string
	if params.Input.Config != nil && params.Input.Config.Focus != "" {
		focusDirective = uc.NormalizeReviewDirective(params.Input.Config.Focus)
	}

	isHeavy := false
	if params.Input.Config != nil && params.Input.Config.Heavy {
		isHeavy = true
	}

	// 5. Execute Analysis / Pipeline
	var issues []domain.CliReviewIssue
	var summaryText string

	if uc.rulesEvaluator != nil {
		for _, f := range changedFiles {
			evalIssues := uc.evaluateFileRules(f, codeReviewConfig)
			issues = append(issues, evalIssues...)
		}
	} else {
		// Default rule inspection over changed files
		for _, f := range changedFiles {
			issues = append(issues, uc.analyzeDefaultFileIssues(f)...)
		}
	}

	duration := time.Since(startTime).Milliseconds()
	if len(issues) > 0 {
		summaryText = fmt.Sprintf("Found %d issue(s) across %d file(s)", len(issues), len(changedFiles))
	} else {
		summaryText = fmt.Sprintf("ScanDrix CLI review passed: 0 issues found in %d file(s)", len(changedFiles))
	}

	response := &domain.CliReviewResponse{
		Summary:       summaryText,
		Issues:        issues,
		FilesAnalyzed: len(changedFiles),
		Duration:      duration,
	}

	if uc.pipelineStrategy != nil {
		stages := uc.pipelineStrategy.ConfigureStages()
		pipelineName := uc.pipelineStrategy.GetPipelineName()
		repoNameVal := "cli-review"
		if resolvedRepoName != nil {
			repoNameVal = *resolvedRepoName
		}

		pipelineCtx := &pipeline.CliReviewPipelineContext{
			IsFastMode:              isFastMode,
			IsTrialMode:             params.IsTrialMode,
			StartTime:               startTime,
			CorrelationID:           correlationID,
			OrganizationAndTeamData: params.OrganizationAndTeamData,
			CodeReviewConfig:        codeReviewConfig,
			ChangedFiles:            changedFiles,
			ReviewDirective:         focusDirective,
			Heavy:                   isHeavy,
			ValidSuggestions:        issues,
			Repository: domain.RepositoryRef{
				ID:   resolvedRepoID,
				Name: repoNameVal,
			},
			Branch:          "cli",
			PlatformType:    resolvedPlatform,
			GitContext:      params.GitContext,
			CliRawDiff:      params.Input.Diff,
			PipelineVersion: "1.0",
		}

		executor := pipeline.NewPipelineExecutor()
		resCtx, pipeErr := executor.Execute(ctx, pipelineCtx, stages, pipelineName)
		if pipeErr == nil && resCtx != nil && resCtx.CliResponse != nil {
			response = resCtx.CliResponse
			issues = response.Issues
		}
	}

	// 6. Update execution as completed
	if execution != nil && uc.automationExecutionService != nil {
		resultData := map[string]any{
			"filesAnalyzed": len(changedFiles),
			"issuesFound":   len(issues),
			"duration":      duration,
			"fastMode":      isFastMode,
			"heavyMode":     isHeavy,
			"focus":         focusDirective,
			"platform":      resolvedPlatform,
			"repositoryResolution": map[string]any{
				"resolvedRepositoryId":   resolvedRepoID,
				"resolvedRepositoryName": resolvedRepoName,
				"matchedByRemote":        resolvedRepoID != "global",
				"gitRemote":              uc.safeGitRemote(params.GitContext),
			},
		}
		repoUpdate := ""
		if resolvedRepoID != "global" {
			repoUpdate = resolvedRepoID
		}
		_ = uc.automationExecutionService.Update(ctx, execution.UUID, "success", resultData, repoUpdate, "")
	}

	uc.logger.Info("CLI review completed successfully",
		"correlationId", correlationID,
		"orgId", params.OrganizationAndTeamData.OrganizationID,
		"issuesFound", len(issues),
		"duration", duration,
	)

	return response, nil
}

// ResolveCliPlatform determines the backing code host platform.
func (uc *ExecuteCliReviewUseCase) ResolveCliPlatform(
	ctx context.Context,
	orgAndTeam domain.OrganizationAndTeamData,
	gitContext *domain.GitContext,
	isTrialMode bool,
) string {
	if gitContext != nil && gitContext.InferredPlatform != "" {
		return gitContext.InferredPlatform
	}

	if isTrialMode {
		return ""
	}

	if uc.codeManagementService != nil {
		platform, err := uc.codeManagementService.GetTypeIntegration(ctx, orgAndTeam)
		if err == nil && platform != "" {
			return platform
		}
	}

	return ""
}

// LoadUserConfigWithRules loads config and applies repository-specific Drixy rules.
func (uc *ExecuteCliReviewUseCase) LoadUserConfigWithRules(
	ctx context.Context,
	orgAndTeam domain.OrganizationAndTeamData,
	gitContext *domain.GitContext,
) (*domain.CodeReviewConfig, string, *string) {
	defaultCfg := uc.GetDefaultConfig()

	if uc.parametersService == nil {
		return defaultCfg, "global", nil
	}

	cfg, repos, err := uc.parametersService.GetCodeReviewConfig(ctx, orgAndTeam)
	if err != nil || cfg == nil {
		uc.logger.Warn("Could not load config from database, using defaults", "error", err)
		return defaultCfg, "global", nil
	}

	normalizedConfig := uc.NormalizeCliConfig(cfg, defaultCfg)

	var remoteURL string
	if gitContext != nil {
		remoteURL = gitContext.Remote
	}

	repoID, repoName := uc.ResolveRepositoryFromRemote(remoteURL, repos)

	var effectiveRules []domain.DrixyRule
	if uc.drixyRulesService != nil {
		loadedRules, err := uc.drixyRulesService.FindByOrganizationID(ctx, orgAndTeam.OrganizationID)
		if err == nil {
			syncedRules, syncErr := uc.drixyRulesService.SyncRulesWithPlanLimit(ctx, orgAndTeam, loadedRules)
			if syncErr == nil && syncedRules != nil {
				effectiveRules = syncedRules
			} else {
				effectiveRules = loadedRules
			}
		}
	}

	standardRules, memoryRules := uc.filterDrixyRules(effectiveRules, repoID)

	normalizedConfig.Rules = standardRules
	normalizedConfig.MemoryRules = memoryRules

	return normalizedConfig, repoID, repoName
}

// ResolveRepositoryFromRemote matches git remote against configured repositories.
func (uc *ExecuteCliReviewUseCase) ResolveRepositoryFromRemote(
	remote string,
	repositories []domain.RepositoryRef,
) (string, *string) {
	if strings.TrimSpace(remote) == "" || len(repositories) == 0 {
		return "global", nil
	}

	normalizedRemote := uc.NormalizeGitURL(remote)

	// Match by full normalized URL
	for _, repo := range repositories {
		targetURL := repo.HTTPURL
		if targetURL == "" {
			targetURL = repo.CloneURL
		}
		if targetURL != "" && uc.NormalizeGitURL(targetURL) == normalizedRemote {
			name := repo.Name
			return repo.ID, &name
		}
	}

	// Fallback: match by repo name extracted from remote URL
	repoName := uc.ExtractRepoNameFromRemote(remote)
	if repoName != nil {
		lowerName := strings.ToLower(*repoName)
		for _, repo := range repositories {
			if strings.ToLower(repo.Name) == lowerName {
				name := repo.Name
				return repo.ID, &name
			}
		}
	}

	return "global", nil
}

// NormalizeGitURL strips protocols, credentials, and suffixes for deterministic comparison.
func (uc *ExecuteCliReviewUseCase) NormalizeGitURL(urlStr string) string {
	s := strings.TrimSpace(urlStr)
	s = strings.ToLower(s)

	// Remove protocols
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "ssh://")
	s = strings.TrimPrefix(s, "git@")

	// Normalize git SSH colon separator (e.g. github.com:org/repo -> github.com/org/repo)
	if idx := strings.Index(s, ":"); idx != -1 {
		if idx+1 < len(s) && s[idx+1] != '/' {
			s = s[:idx] + "/" + s[idx+1:]
		}
	}

	// Strip trailing slashes first (handles cases like project.git/)
	s = strings.TrimRight(s, "/")
	// Strip .git suffix
	s = strings.TrimSuffix(s, ".git")
	// Strip trailing slashes again
	s = strings.TrimRight(s, "/")

	return s
}

// ExtractRepoNameFromRemote gets the repo name segment from a remote string.
func (uc *ExecuteCliReviewUseCase) ExtractRepoNameFromRemote(remote string) *string {
	clean := strings.TrimSuffix(strings.TrimRight(remote, "/"), ".git")
	parts := regexp.MustCompile(`[/:]`).Split(clean, -1)

	var valid []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			valid = append(valid, p)
		}
	}

	if len(valid) >= 2 {
		last := valid[len(valid)-1]
		return &last
	}

	return nil
}

// NormalizeCliConfig validates and sets defaults for CLI execution.
func (uc *ExecuteCliReviewUseCase) NormalizeCliConfig(config, defaults *domain.CodeReviewConfig) *domain.CodeReviewConfig {
	if defaults == nil {
		defaults = uc.GetDefaultConfig()
	}
	if config == nil {
		return defaults
	}

	res := *config
	res.CodeReviewVersion = "v2"
	if res.LanguageResultPrompt == "" {
		res.LanguageResultPrompt = "en-US"
	}

	return &res
}

// GetDefaultConfig returns the standard baseline configuration.
func (uc *ExecuteCliReviewUseCase) GetDefaultConfig() *domain.CodeReviewConfig {
	return &domain.CodeReviewConfig{
		CodeReviewVersion:         "v2",
		AutomatedReviewActive:     true,
		PullRequestApprovalActive: false,
		LanguageResultPrompt:      "en-US",
		ReviewOptions: domain.ReviewOptions{
			Bug:         true,
			Performance: true,
			Security:    true,
			CrossFile:   false,
		},
	}
}

// NormalizeReviewDirective sanitizes and caps user review focus prompt.
func (uc *ExecuteCliReviewUseCase) NormalizeReviewDirective(directive string) string {
	clean := strings.TrimSpace(directive)
	if clean == "" {
		return ""
	}
	// Limit to 300 characters
	if len(clean) > 300 {
		clean = clean[:300]
	}
	return clean
}

func (uc *ExecuteCliReviewUseCase) filterDrixyRules(allRules []domain.DrixyRule, repoID string) ([]domain.DrixyRule, []domain.DrixyRule) {
	var standard []domain.DrixyRule
	var memory []domain.DrixyRule

	for _, r := range allRules {
		if !r.Active || r.Locked {
			continue
		}

		// Check repository scope: applies if repoID is global or rule repoID matches
		if r.RepositoryID != "" && r.RepositoryID != "global" && repoID != "global" && r.RepositoryID != repoID {
			continue
		}

		if r.Type == "memory" {
			memory = append(memory, r)
		} else {
			standard = append(standard, r)
		}
	}

	return standard, memory
}

func (uc *ExecuteCliReviewUseCase) createAutomationExecution(
	ctx context.Context,
	params domain.ExecuteCliReviewInput,
	correlationID string,
) (*domain.AutomationExecutionRecord, error) {
	record := &domain.AutomationExecutionRecord{
		UUID:           uuid.New().String(),
		Status:         "in_progress",
		Origin:         "cli",
		OrganizationID: params.OrganizationAndTeamData.OrganizationID,
		TeamID:         params.OrganizationAndTeamData.TeamID,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
		DataExecution: map[string]any{
			"type":                    "CLI_REVIEW",
			"correlationId":           correlationID,
			"userEmail":               params.UserEmail,
			"organizationAndTeamData": params.OrganizationAndTeamData,
			"cliAuth":                 params.CliAuth,
			"cliVersion":              uc.safeCliVersion(params.GitContext),
			"git":                     params.GitContext,
		},
	}

	return uc.automationExecutionService.Create(ctx, record)
}

func (uc *ExecuteCliReviewUseCase) evaluateFileRules(file domain.FileChange, cfg *domain.CodeReviewConfig) []domain.CliReviewIssue {
	return uc.analyzeDefaultFileIssues(file)
}

func (uc *ExecuteCliReviewUseCase) analyzeDefaultFileIssues(file domain.FileChange) []domain.CliReviewIssue {
	var issues []domain.CliReviewIssue
	lines := strings.Split(file.Patch, "\n")
	lineNum := 1

	for _, l := range lines {
		if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			content := strings.TrimPrefix(l, "+")
			// Secret detection
			if strings.Contains(content, "sk_live_") || strings.Contains(content, "ghp_") || strings.Contains(content, "AIzaSy") {
				issues = append(issues, domain.CliReviewIssue{
					File:           file.Filename,
					Line:           lineNum,
					Severity:       "critical",
					Category:       "security",
					Message:        "Potential hardcoded credential or API secret token detected.",
					Recommendation: "Move credentials to secure environment variables or secret store.",
					RuleID:         "sec-no-hardcoded-secrets",
				})
			}
			// Insecure eval / SQL injection
			if strings.Contains(content, "eval(") {
				issues = append(issues, domain.CliReviewIssue{
					File:           file.Filename,
					Line:           lineNum,
					Severity:       "high",
					Category:       "security",
					Message:        "Dangerous use of eval() detected.",
					Recommendation: "Avoid dynamic code execution with eval.",
					RuleID:         "sec-avoid-eval",
				})
			}
			if strings.Contains(content, "TODO") || strings.Contains(content, "FIXME") {
				issues = append(issues, domain.CliReviewIssue{
					File:           file.Filename,
					Line:           lineNum,
					Severity:       "low",
					Category:       "style",
					Message:        "Unresolved TODO/FIXME comment introduced.",
					Recommendation: "Resolve or track task before committing.",
					RuleID:         "style-no-todo",
				})
			}
		}
		lineNum++
	}

	return issues
}

func (uc *ExecuteCliReviewUseCase) generateCorrelationID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("corr_%s", hex.EncodeToString(buf))
}

func (uc *ExecuteCliReviewUseCase) safeGitRemote(gc *domain.GitContext) string {
	if gc == nil {
		return ""
	}
	return gc.Remote
}

func (uc *ExecuteCliReviewUseCase) safeCliVersion(gc *domain.GitContext) string {
	if gc == nil {
		return ""
	}
	return gc.CLIVersion
}

// InMemoryAutomationExecutionService provides an in-memory mock service for testing & standalone usage.
type InMemoryAutomationExecutionService struct {
	mu      sync.RWMutex
	records map[string]*domain.AutomationExecutionRecord
}

// NewInMemoryAutomationExecutionService creates an initialized in-memory execution service.
func NewInMemoryAutomationExecutionService() *InMemoryAutomationExecutionService {
	return &InMemoryAutomationExecutionService{
		records: make(map[string]*domain.AutomationExecutionRecord),
	}
}

func (s *InMemoryAutomationExecutionService) Create(ctx context.Context, record *domain.AutomationExecutionRecord) (*domain.AutomationExecutionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[record.UUID] = record
	return record, nil
}

func (s *InMemoryAutomationExecutionService) Update(ctx context.Context, uuid string, status string, data map[string]any, repositoryID string, errorMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[uuid]
	if !ok {
		return fmt.Errorf("record %s not found", uuid)
	}
	rec.Status = status
	rec.UpdatedAt = time.Now().UTC()
	if repositoryID != "" {
		rec.RepositoryID = repositoryID
	}
	if errorMsg != "" {
		rec.ErrorMessage = errorMsg
	}
	if rec.DataExecution == nil {
		rec.DataExecution = make(map[string]any)
	}
	for k, v := range data {
		rec.DataExecution[k] = v
	}
	return nil
}

func (s *InMemoryAutomationExecutionService) FindByID(ctx context.Context, uuid string) (*domain.AutomationExecutionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[uuid]
	if !ok {
		return nil, fmt.Errorf("record %s not found", uuid)
	}
	return rec, nil
}

func (s *InMemoryAutomationExecutionService) Records() []*domain.AutomationExecutionRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []*domain.AutomationExecutionRecord
	for _, r := range s.records {
		list = append(list, r)
	}
	return list
}

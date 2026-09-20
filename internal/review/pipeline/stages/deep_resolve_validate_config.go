package stages

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/review/configengine"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/review/rulesengine"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

// IByokSlotResolver resolves provider credentials and model slots for review tasks.
type IByokSlotResolver interface {
	ResolveTaskSlot(ctx context.Context, orgID, taskName, modelOverride string) (*domain.ResolvedModelSlotInfo, error)
}

// DeepResolveConfigStage implements Stage 3 configuration resolution with hierarchical merging,
// in-repo override inspection, and Drixy rules cascading.
type DeepResolveConfigStage struct {
	resolver          ConfigResolver
	slotResolver      IByokSlotResolver
	inRepoParser      *configengine.InRepoConfigParser
	rulesScanner      *rulesengine.InRepoRulesScanner
	rulesCatalog      *rulesengine.RulesCatalog
	rulesResolver     *rulesengine.InheritanceResolver
	detectorEvaluator *rulesengine.DetectorEvaluator
}

// NewDeepResolveConfigStage constructs Stage 3.
func NewDeepResolveConfigStage(resolver ConfigResolver, slotResolver IByokSlotResolver) *DeepResolveConfigStage {
	catalog := rulesengine.NewRulesCatalog()
	return &DeepResolveConfigStage{
		resolver:          resolver,
		slotResolver:      slotResolver,
		inRepoParser:      configengine.NewInRepoConfigParser(),
		rulesScanner:      rulesengine.NewInRepoRulesScanner(),
		rulesCatalog:      catalog,
		rulesResolver:     rulesengine.NewInheritanceResolver(catalog),
		detectorEvaluator: rulesengine.NewDetectorEvaluator(),
	}
}

// WithRulesCatalog overrides the rules catalog.
func (s *DeepResolveConfigStage) WithRulesCatalog(catalog *rulesengine.RulesCatalog) *DeepResolveConfigStage {
	s.rulesCatalog = catalog
	s.rulesResolver = rulesengine.NewInheritanceResolver(catalog)
	return s
}

// WithInRepoParser overrides the in-repo configuration parser.
func (s *DeepResolveConfigStage) WithInRepoParser(parser *configengine.InRepoConfigParser) *DeepResolveConfigStage {
	s.inRepoParser = parser
	return s
}

// WithDetectorEvaluator overrides the detector evaluator.
func (s *DeepResolveConfigStage) WithDetectorEvaluator(evaluator *rulesengine.DetectorEvaluator) *DeepResolveConfigStage {
	s.detectorEvaluator = evaluator
	return s
}

func (s *DeepResolveConfigStage) Name() string {
	return "DeepResolveConfigStage"
}

func (s *DeepResolveConfigStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if pCtx.SkipReview {
		return nil
	}

	if pCtx.PipelineMetadata == nil {
		pCtx.PipelineMetadata = make(map[string]interface{})
	}

	orgID := pCtx.WorkspaceID.String()
	repoID := pCtx.RepositoryID.String()

	// 1. Resolve configuration from backend store
	cfg := domain.DefaultCodeReviewConfig()
	if s.resolver != nil {
		resolved, err := s.resolver.GetCodeReviewParameter(ctx, orgID, "", repoID)
		if err == nil {
			cfg = resolved
		}
	}

	// 2. Check for committed in-repo configuration in patches (.scandrix.yml, .scandrix/config.json, .drixy/config.json)
	if s.inRepoParser != nil && len(pCtx.ParsedPatches) > 0 {
		inRepo, path, warnings, err := s.inRepoParser.ExtractFromPatches(pCtx.ParsedPatches)
		if err == nil && inRepo != nil {
			pCtx.PipelineMetadata["in_repo_config"] = inRepo
			pCtx.PipelineMetadata["in_repo_config_path"] = path

			if inRepo.Enabled != nil {
				cfg.Enabled = *inRepo.Enabled
			}
			if inRepo.ReviewMode != nil {
				cfg.ReviewMode = string(*inRepo.ReviewMode)
			}
			if inRepo.Sensitivity != nil {
				cfg.Sensitivity = string(*inRepo.Sensitivity)
			}
			if inRepo.Strictness != nil {
				cfg.Strictness = *inRepo.Strictness
			}
			if inRepo.MaxCommentsPerReview != nil {
				cfg.MaxSuggestions = *inRepo.MaxCommentsPerReview
			}
			if inRepo.MaxSuggestions != nil {
				cfg.MaxSuggestions = *inRepo.MaxSuggestions
			}
			if inRepo.ByokModelID != "" {
				cfg.ByokModelID = inRepo.ByokModelID
			}
			if inRepo.ByokModel != "" {
				cfg.ByokModel = inRepo.ByokModel
			}
			if inRepo.ReviewOptions != nil {
				cfg.ReviewOptions = *inRepo.ReviewOptions
			}
			if inRepo.RequireTicketContext != nil {
				cfg.RequireTicketContext = *inRepo.RequireTicketContext
			}
			if inRepo.CommittableSuggestions != nil {
				cfg.EnableCommittableSuggestions = *inRepo.CommittableSuggestions
			}
			if inRepo.AutoApproveCleanPRs != nil {
				cfg.AutoApprove = *inRepo.AutoApproveCleanPRs
			}
			for _, p := range inRepo.IgnoredFilePatterns {
				clean := strings.TrimSpace(p)
				if clean != "" {
					cfg.IgnorePaths = append(cfg.IgnorePaths, clean)
				}
			}
			for _, b := range inRepo.IncludedBranchPatterns {
				clean := strings.TrimSpace(b)
				if clean != "" {
					cfg.BaseBranches = append(cfg.BaseBranches, clean)
				}
			}
		}
		if len(warnings) > 0 {
			pCtx.PipelineMetadata["in_repo_config_warnings"] = warnings
		}
	}

	// 3. Resolve BYOK task model slot with ID-over-name override precedence
	modelOverride := strings.TrimSpace(cfg.ByokModelID)
	if modelOverride == "" {
		modelOverride = strings.TrimSpace(cfg.ByokModel)
	}

	if s.slotResolver != nil {
		slot, err := s.slotResolver.ResolveTaskSlot(ctx, orgID, "codeReview", modelOverride)
		if err == nil && slot != nil {
			cfg.ResolvedModelSlot = slot
		}
	}

	pCtx.ResolvedConfig = cfg

	// 4. Resolve custom message templates (PR start, end, error messages)
	if s.resolver != nil {
		msgs, err := s.resolver.FindByRepoOrDirectory(ctx, orgID, repoID, "")
		if err == nil && msgs != nil {
			pCtx.PullRequestMessages = msgs
		}
	}

	// 5. In-Repo Rules Discovery & Drixy Rules Inheritance Resolution
	var inRepoRules []*rulesengine.DrixyRule
	if s.rulesScanner != nil && len(pCtx.ParsedPatches) > 0 {
		scanned, err := s.rulesScanner.ScanPatches(pCtx.ParsedPatches)
		if err == nil && len(scanned) > 0 {
			inRepoRules = scanned
			pCtx.PipelineMetadata["in_repo_rules"] = scanned
		}
	}

	var changedFilePaths []string
	for _, f := range pCtx.ChangedFiles {
		if f.Filename != "" {
			changedFilePaths = append(changedFilePaths, f.Filename)
		}
	}
	if len(changedFilePaths) == 0 {
		for _, p := range pCtx.ParsedPatches {
			name := p.NewPath
			if name == "" {
				name = p.OldPath
			}
			if name != "" {
				changedFilePaths = append(changedFilePaths, name)
			}
		}
	}

	if s.rulesResolver != nil {
		resolvedRules := s.rulesResolver.ResolveActiveRules(ctx, rulesengine.InheritanceInput{
			WorkspaceID:  pCtx.WorkspaceID,
			RepositoryID: repoID,
			ChangedFiles: changedFilePaths,
			InRepoRules:  inRepoRules,
		})

		if resolvedRules != nil {
			pCtx.PipelineMetadata["resolved_rules"] = resolvedRules

			// Map semantic rules into pCtx.ActiveRules for downstream agent deliberation
			for _, r := range resolvedRules.SemanticRules {
				pCtx.ActiveRules = append(pCtx.ActiveRules, mapDrixyRuleToRuleSpec(r))
			}

			// Evaluate mechanical detector rules directly against diff hunks (T0 Zero-LLM)
			if s.detectorEvaluator != nil && len(resolvedRules.MechanicalRules) > 0 && len(pCtx.ParsedPatches) > 0 {
				mechanicalFindings := s.detectorEvaluator.EvaluateRules(
					ctx,
					pCtx.ReviewID,
					pCtx.WorkspaceID,
					resolvedRules.MechanicalRules,
					pCtx.ParsedPatches,
				)
				if len(mechanicalFindings) > 0 {
					pCtx.StaticFindings = append(pCtx.StaticFindings, mechanicalFindings...)
					pCtx.AddMetric("mechanical_rule_findings", 0, true, nil, len(mechanicalFindings))
				}
			}

			pCtx.AddMetric("active_rules_total", 0, true, nil, resolvedRules.TotalActive)
		}
	}

	return nil
}

func mapDrixyRuleToRuleSpec(r *rulesengine.DrixyRule) rules.RuleSpec {
	pattern := ""
	if len(r.PathGlobs) > 0 {
		pattern = r.PathGlobs[0]
	}
	regexRule := ""
	if r.Detector != nil {
		regexRule = r.Detector.Pattern
	}
	return rules.RuleSpec{
		ID:          r.ID,
		Name:        r.Title,
		PathPattern: pattern,
		RegexRule:   regexRule,
		Severity:    r.Severity,
		Category:    "drixy_rules",
		Description: r.Description,
		Remediation: r.RemediationHint,
	}
}

// DeepValidateConfigStage implements Stage 4 configuration verification, branch pattern matching, cadence enforcement, and title filtering.
type DeepValidateConfigStage struct {
	cadenceEngine *ReviewCadenceEngine
	scmFeedback   ISCMFeedbackReaction
}

// NewDeepValidateConfigStage constructs Stage 4.
func NewDeepValidateConfigStage(cadenceEngine *ReviewCadenceEngine, feedback ISCMFeedbackReaction) *DeepValidateConfigStage {
	return &DeepValidateConfigStage{
		cadenceEngine: cadenceEngine,
		scmFeedback:   feedback,
	}
}

func (s *DeepValidateConfigStage) Name() string {
	return "DeepValidateConfigStage"
}

func (s *DeepValidateConfigStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if pCtx.SkipReview {
		return nil
	}

	config := pCtx.ResolvedConfig
	isCommand := strings.HasPrefix(pCtx.Origin, "command")

	// 1. Basic feature toggle
	if !config.Enabled || (!config.AutomatedReviewActive && !isCommand) {
		pCtx.SkipReview = true
		pCtx.SkipReason = "Code review automation is disabled for this repository"
		pCtx.StatusInfo = pipeline.PipelineStatusInfo{
			Status:     pipeline.StatusSkipped,
			Message:    "Code reviews are disabled in configuration",
			ReasonCode: "CONFIG_DISABLED",
		}
		return nil
	}

	// 2. Draft PR evaluation
	if pCtx.IsDraft && !config.RunOnDraft && !isCommand {
		pCtx.SkipReview = true
		pCtx.SkipReason = "Review skipped for draft pull request (runOnDraft=false)"
		pCtx.StatusInfo = pipeline.PipelineStatusInfo{
			Status:     pipeline.StatusSkipped,
			Message:    "Draft PRs skipped by repository configuration",
			ReasonCode: "DRAFT_PR_SKIPPED",
		}
		return nil
	}

	// 3. Ignored title keywords
	lowerTitle := strings.ToLower(pCtx.Title)
	for _, kw := range config.IgnoredTitleKeywords {
		if kw != "" && strings.Contains(lowerTitle, strings.ToLower(kw)) && !isCommand {
			pCtx.SkipReview = true
			pCtx.SkipReason = fmt.Sprintf("Title matches ignored keyword: '%s'", kw)
			pCtx.StatusInfo = pipeline.PipelineStatusInfo{
				Status:     pipeline.StatusSkipped,
				Message:    fmt.Sprintf("Title matches ignored keyword: '%s'", kw),
				ReasonCode: "IGNORED_TITLE_KEYWORD",
			}
			return nil
		}
	}

	// 4. Branch rule evaluation
	if len(config.BaseBranches) > 0 && !isCommand {
		mergedBranches := MergeBaseBranches(config.BaseBranches, config.BaseBranchDefault)
		normalizedBranches := NormalizeBranchesForPlatform(mergedBranches, pCtx.Provider)
		expression := strings.Join(normalizedBranches, ", ")

		reviewRules := ProcessBranchExpression(expression)
		shouldReview := ShouldReviewBranches(pCtx.Branch, pCtx.BaseBranch, reviewRules)

		if !shouldReview {
			pCtx.SkipReview = true
			reason := fmt.Sprintf("Target branch '%s' does not match configured branch rules: [%s]", pCtx.BaseBranch, expression)
			pCtx.SkipReason = reason
			pCtx.StatusInfo = pipeline.PipelineStatusInfo{
				Status:     pipeline.StatusSkipped,
				Message:    reason,
				ReasonCode: "BRANCH_MISMATCH",
			}
			return nil
		}
	}

	// 5. Review Cadence evaluation (automatic, manual, auto-pause)
	if s.cadenceEngine != nil {
		cadenceResult, err := s.cadenceEngine.EvaluateCadence(ctx, pCtx)
		if err == nil && !cadenceResult.ShouldProcess {
			pCtx.SkipReview = true
			pCtx.SkipReason = cadenceResult.Reason
			pCtx.StatusInfo = pipeline.PipelineStatusInfo{
				Status:     pipeline.StatusSkipped,
				Message:    cadenceResult.Reason,
				ReasonCode: "CADENCE_SUPPRESSED",
			}

			// Post auto-pause comment if burst pushes detected
			if cadenceResult.PauseCommentBody != "" && s.scmFeedback != nil {
				repo := models.TrackedRepository{
					ID:            pCtx.RepositoryID,
					WorkspaceID:   pCtx.WorkspaceID,
					Provider:      pCtx.Provider,
					NamespacePath: pCtx.RepoNamespace,
				}
				_ = s.scmFeedback.CreateNoticeComment(ctx, repo, pCtx.PullNumber, cadenceResult.PauseCommentBody)
			}
			return nil
		}
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rules_module.go
// ═══════════════════════════════════════════════════════════════

package modules

import (
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/rules/drixy/application/usecases"
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/listeners"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/repositories"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

// DrixyRulesModuleConfig holds parameters to wire the Drixy rules module.
type DrixyRulesModuleConfig struct {
	DatabaseClient  *database.Client
	LLMGateway      *llm.Gateway
	CLIKeyValidator controllers.TeamCLIKeyValidator
}

// DrixyRulesModule aggregates all repositories, services, use cases, listeners, and controllers.
type DrixyRulesModule struct {
	// Repositories
	RulesRepo    contracts.IDrixyRulesRepository
	RuleLikeRepo contracts.IRuleLikeRepository

	// Services
	RulesService      *services.DrixyRulesService
	RuleLikeService   *services.RuleLikeService
	SyncService       *services.DrixyRulesSyncService
	SummaryService    *services.DrixyRuleSummaryService
	DetectorCompiler  *services.DrixyRuleDetectorCompiler
	SweepService      *services.DrixyRuleDetectorSweepService
	ValidationService *services.DrixyRulesValidationService
	ReferenceLoader   *services.ExternalReferenceLoaderService

	// Listeners
	SyncListener *listeners.DrixyRulesSyncListener

	// Controllers
	RulesController    *controllers.DrixyRulesController
	RuleLikeController *controllers.RuleLikeController
	CliRulesController *controllers.CliDrixyRulesController
}

// NewDrixyRulesModule constructs and wires all dependencies for Drixy Rules.
func NewDrixyRulesModule(cfg DrixyRulesModuleConfig) *DrixyRulesModule {
	ruleLikeRepo := repositories.NewPostgresRuleLikeRepository(cfg.DatabaseClient)
	ruleLikeService := services.NewRuleLikeService(ruleLikeRepo)

	rulesRepo := repositories.NewPostgresDrixyRulesRepository(cfg.DatabaseClient)
	rulesService := services.NewDrixyRulesService(rulesRepo, ruleLikeService)
	syncService := services.NewDrixyRulesSyncService(rulesService)
	if cfg.LLMGateway != nil {
		syncService.SetLLMGateway(cfg.LLMGateway)
	}
	commentAnalysis := services.NewDrixyCommentAnalysisService(cfg.LLMGateway, rulesService)
	summaryService := services.NewDrixyRuleSummaryService(cfg.LLMGateway)
	detectorCompiler := services.NewDrixyRuleDetectorCompiler(rulesService, cfg.LLMGateway)
	sweepService := services.NewDrixyRuleDetectorSweepService()
	validationService := services.NewDrixyRulesValidationService()
	referenceLoader := services.NewExternalReferenceLoaderService()

	syncListener := listeners.NewDrixyRulesSyncListener(syncService, rulesService)

	// Use cases
	createOrUpdateUC := usecases.NewCreateOrUpdateDrixyRuleUseCase(rulesService, detectorCompiler, summaryService, referenceLoader)
	findByOrgUC := usecases.NewFindByOrganizationIDDrixyRulesUseCase(rulesService, referenceLoader)
	findRulesByFilterUC := usecases.NewFindRulesInOrganizationByFilterDrixyRulesUseCase(rulesService, referenceLoader)
	deleteRuleInOrgUC := usecases.NewDeleteRuleInOrganizationByIDDrixyRulesUseCase(rulesService)
	findLibraryRulesUC := usecases.NewFindLibraryDrixyRulesUseCase(rulesService)
	findLibraryFeedbackUC := usecases.NewFindLibraryDrixyRulesWithFeedbackUseCase(rulesService)
	findLibraryBucketsUC := usecases.NewFindLibraryDrixyRulesBucketsUseCase(rulesService)
	findRecommendedRulesUC := usecases.NewFindRecommendedDrixyRulesUseCase(rulesService, nil)
	addLibraryRulesUC := usecases.NewAddLibraryDrixyRulesUseCase(createOrUpdateUC)
	sendNotifUC := usecases.NewSendRulesNotificationUseCase(nil)
	generateRulesUC := usecases.NewGenerateDrixyRulesUseCase(rulesService, createOrUpdateUC, commentAnalysis, sendNotifUC)
	applyPendingRulesUC := usecases.NewApplyPendingDrixyRulesUseCase(rulesService)
	getPendingRulesUC := usecases.NewGetPendingDrixyRulesUseCase(rulesService)
	changeStatusRulesUC := usecases.NewChangeStatusDrixyRulesUseCase(rulesService)
	checkSyncStatusUC := usecases.NewCheckSyncStatusUseCase(rulesService)
	listPastReviewersUC := usecases.NewListPastReviewersUseCase(nil)
	syncSelectedReposUC := usecases.NewSyncSelectedRepositoriesDrixyRulesUseCase(syncService)
	getGlobalSourceReposUC := usecases.NewGetGlobalRulesSourceRepositoriesUseCase(nil)
	updateGlobalSourceReposUC := usecases.NewUpdateGlobalRulesSourceRepositoriesUseCase(nil, syncService)
	resyncGlobalRulesUC := usecases.NewResyncGlobalRulesUseCase(nil, syncService)
	getGlobalImportStatusUC := usecases.NewGetGlobalRulesImportStatusUseCase(nil, syncService)
	getInheritedRulesUC := usecases.NewGetInheritedDrixyRulesUseCase(rulesService)
	getRulesLimitStatusUC := usecases.NewGetRulesLimitStatusUseCase(rulesService)
	findSuggestionsByRuleUC := usecases.NewFindSuggestionsByRuleUseCase(rulesService, nil)
	resyncRulesFromIdeUC := usecases.NewResyncRulesFromIdeUseCase(syncService)
	fastSyncIdeRulesUC := usecases.NewFastSyncIdeRulesUseCase(syncService)
	importFastRulesUC := usecases.NewImportFastDrixyRulesUseCase(createOrUpdateUC)
	convertPendingUpdatesUC := usecases.NewConvertPendingUpdatesToNewUseCase(rulesService)
	manageImportedRulesUC := usecases.NewManageImportedDrixyRulesUseCase(syncService)
	countRulesByRepoUC := usecases.NewCountRulesByRepositoryUseCase(rulesService)
	backfillDetectorsUC := usecases.NewBackfillRuleDetectorsUseCase(rulesService, detectorCompiler)

	setRuleLikeUC := usecases.NewSetRuleLikeUseCase(ruleLikeService)
	removeRuleLikeUC := usecases.NewRemoveRuleLikeUseCase(ruleLikeService)

	rulesCtrl := controllers.NewDrixyRulesController(controllers.DrixyRulesUseCases{
		CreateOrUpdateUseCase:               createOrUpdateUC,
		FindByOrganizationIDUseCase:         findByOrgUC,
		FindRulesByFilterUseCase:            findRulesByFilterUC,
		DeleteRuleInOrgByIDUseCase:          deleteRuleInOrgUC,
		FindLibraryDrixyRulesUseCase:        findLibraryRulesUC,
		FindLibraryDrixyRulesWithFeedbackUC: findLibraryFeedbackUC,
		FindLibraryDrixyRulesBucketsUC:      findLibraryBucketsUC,
		FindRecommendedDrixyRulesUC:         findRecommendedRulesUC,
		AddLibraryDrixyRulesUseCase:         addLibraryRulesUC,
		GenerateDrixyRulesUseCase:           generateRulesUC,
		ApplyPendingDrixyRulesUseCase:       applyPendingRulesUC,
		GetPendingDrixyRulesUseCase:         getPendingRulesUC,
		ChangeStatusDrixyRulesUseCase:       changeStatusRulesUC,
		CheckSyncStatusUseCase:              checkSyncStatusUC,
		ListPastReviewersUseCase:            listPastReviewersUC,
		SyncSelectedReposUseCase:            syncSelectedReposUC,
		GetGlobalRulesSourceRepositoriesUC:  getGlobalSourceReposUC,
		UpdateGlobalRulesSourceReposUC:      updateGlobalSourceReposUC,
		ResyncGlobalRulesUseCase:            resyncGlobalRulesUC,
		GetGlobalRulesImportStatusUC:        getGlobalImportStatusUC,
		GetInheritedRulesUseCase:            getInheritedRulesUC,
		GetRulesLimitStatusUseCase:          getRulesLimitStatusUC,
		FindSuggestionsByRuleUseCase:        findSuggestionsByRuleUC,
		ResyncRulesFromIdeUseCase:           resyncRulesFromIdeUC,
		FastSyncIdeRulesUseCase:             fastSyncIdeRulesUC,
		ImportFastDrixyRulesUseCase:         importFastRulesUC,
		ConvertPendingUpdatesToNewUseCase:   convertPendingUpdatesUC,
		ManageImportedDrixyRulesUseCase:     manageImportedRulesUC,
		CountRulesByRepositoryUseCase:       countRulesByRepoUC,
		BackfillRuleDetectorsUseCase:        backfillDetectorsUC,
	})

	ruleLikeCtrl := controllers.NewRuleLikeController(setRuleLikeUC, removeRuleLikeUC)
	cliRulesCtrl := controllers.NewCliDrixyRulesController(cfg.CLIKeyValidator, createOrUpdateUC, findRulesByFilterUC)

	return &DrixyRulesModule{
		RulesRepo:          rulesRepo,
		RuleLikeRepo:       ruleLikeRepo,
		RulesService:       rulesService,
		RuleLikeService:    ruleLikeService,
		SyncService:        syncService,
		SummaryService:     summaryService,
		DetectorCompiler:   detectorCompiler,
		SweepService:       sweepService,
		ValidationService:  validationService,
		ReferenceLoader:    referenceLoader,
		SyncListener:       syncListener,
		RulesController:    rulesCtrl,
		RuleLikeController: ruleLikeCtrl,
		CliRulesController: cliRulesCtrl,
	}
}

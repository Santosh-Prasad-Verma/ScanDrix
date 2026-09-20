package organization

import (
	"github.com/jackc/pgx/v5/pgxpool"
	onboardingusecases "github.com/scandrix/backend/internal/organization/application/usecases/onboarding"
	orgusecases "github.com/scandrix/backend/internal/organization/application/usecases/organization"
	orgparamusecases "github.com/scandrix/backend/internal/organization/application/usecases/organizationparameters"
	paramusecases "github.com/scandrix/backend/internal/organization/application/usecases/parameters"
	teamusecases "github.com/scandrix/backend/internal/organization/application/usecases/team"
	memberusecases "github.com/scandrix/backend/internal/organization/application/usecases/teammembers"
	clidevicedomain "github.com/scandrix/backend/internal/organization/domain/clidevice"
	globalparamdomain "github.com/scandrix/backend/internal/organization/domain/globalparameters"
	orgdomain "github.com/scandrix/backend/internal/organization/domain/organization"
	orgparamdomain "github.com/scandrix/backend/internal/organization/domain/organizationparameters"
	paramdomain "github.com/scandrix/backend/internal/organization/domain/parameters"
	teamdomain "github.com/scandrix/backend/internal/organization/domain/team"
	teamclikeydomain "github.com/scandrix/backend/internal/organization/domain/teamclikey"
	memberdomain "github.com/scandrix/backend/internal/organization/domain/teammembers"
	"github.com/scandrix/backend/internal/organization/infrastructure/repositories"
	"github.com/scandrix/backend/internal/organization/infrastructure/services"
)

// OrganizationModule bundles all repositories, domain services, and application use cases
// providing exact architectural parity with the NestJS organization module.
type OrganizationModule struct {
	// Repositories
	OrganizationRepo           orgdomain.IOrganizationRepository
	OrganizationParametersRepo orgparamdomain.IOrganizationParametersRepository
	ParametersRepo             paramdomain.IParametersRepository
	TeamRepo                   teamdomain.ITeamRepository
	TeamMembersRepo            memberdomain.ITeamMembersRepository
	TeamCliKeyRepo             teamclikeydomain.ITeamCliKeyRepository
	CliDeviceRepo              clidevicedomain.ICliDeviceRepository
	GlobalParametersRepo       globalparamdomain.IGlobalParametersRepository

	// Domain Services
	OrganizationService           orgdomain.IOrganizationService
	OrganizationParametersService orgparamdomain.IOrganizationParametersService
	ParametersService             paramdomain.IParametersService
	TeamService                   teamdomain.ITeamService
	TeamMembersService            memberdomain.ITeamMembersService
	TeamCliKeyService             teamclikeydomain.ITeamCliKeyService
	CliDeviceService              clidevicedomain.ICliDeviceService
	GlobalParametersService       globalparamdomain.IGlobalParametersService

	// Use Cases - Organization
	GetOrganizationNameUC       *orgusecases.GetOrganizationNameUseCase
	GetOrganizationLanguageUC   *orgusecases.GetOrganizationLanguageUseCase
	GetOrganizationsByDomainUC  *orgusecases.GetOrganizationsByDomainUseCase
	GetReleaseTrackUC           *orgusecases.GetReleaseTrackUseCase
	UpdateInfosUC               *orgusecases.UpdateInfosUseCase

	// Use Cases - Organization Parameters
	OrgParamFindByKeyUC         *orgparamusecases.FindByKeyUseCase
	OrgParamCreateOrUpdateUC    *orgparamusecases.CreateOrUpdateUseCase
	GetCockpitMetricsVisUC      *orgparamusecases.GetCockpitMetricsVisibilityUseCase
	GetLLMConfigStatusUC        *orgparamusecases.GetLLMConfigStatusUseCase
	ListModelOverridesUC        *orgparamusecases.ListModelOverridesUseCase
	ClearModelOverridesUC       *orgparamusecases.ClearModelOverridesUseCase
	DeleteBYOKConfigUC          *orgparamusecases.DeleteBYOKConfigUseCase
	GetBYOKProvidersUC          *orgparamusecases.GetBYOKProvidersUseCase
	GetModelsByProviderUC       *orgparamusecases.GetModelsByProviderUseCase
	GetModelCapabilitiesUC      *orgparamusecases.GetModelCapabilitiesUseCase
	IgnoreBotsUC                *orgparamusecases.IgnoreBotsUseCase
	TestBYOKConnectionUC        *orgparamusecases.TestBYOKConnectionUseCase
	TestBYOKModelUC             *orgparamusecases.TestBYOKModelUseCase

	// Use Cases - Parameters
	ParamGetDefaultConfigUC     *paramusecases.GetDefaultConfigUseCase
	ParamFindByKeyUC            *paramusecases.FindByKeyParametersUseCase
	ParamCreateOrUpdateUC       *paramusecases.CreateOrUpdateParametersUseCase

	// Use Cases - Team
	CreateTeamUC                *teamusecases.CreateTeamUseCase
	ListTeamsUC                 *teamusecases.ListTeamsUseCase
	ListTeamsWithIntegrationsUC *teamusecases.ListTeamsWithIntegrationsUseCase

	// Use Cases - Team Members
	CreateTeamMemberUC          *memberusecases.CreateOrUpdateTeamMembersUseCase
	DeleteTeamMemberUC          *memberusecases.DeleteTeamMemberUseCase
	GetTeamMembersUC            *memberusecases.GetTeamMembersUseCase

	// Use Cases - Onboarding
	JoinOrganizationUC          *onboardingusecases.JoinOrganizationUseCase
}

// ModuleConfig configures options for the organization module.
type ModuleConfig struct {
	EncryptionKey string // AES-256 key for BYOK credentials encryption
}

// NewOrganizationModule initializes all organization repositories, services, and use cases.
func NewOrganizationModule(pool *pgxpool.Pool, userRepo onboardingusecases.IUserAccountRepository, cfg ModuleConfig) *OrganizationModule {
	if userRepo == nil {
		userRepo = repositories.NewPostgresUserAccountRepository(pool)
	}

	// 1. Repositories
	orgRepo := repositories.NewPostgresOrganizationRepository(pool)
	orgParamRepo := repositories.NewPostgresOrganizationParametersRepository(pool)
	paramRepo := repositories.NewPostgresParametersRepository(pool)
	teamRepo := repositories.NewPostgresTeamRepository(pool)
	teamMembersRepo := repositories.NewPostgresTeamMemberRepository(pool)
	teamCliKeyRepo := repositories.NewPostgresTeamCliKeyRepository(pool)
	cliDeviceRepo := repositories.NewPostgresCliDeviceRepository(pool)
	globalParamRepo := repositories.NewPostgresGlobalParametersRepository(pool)

	// 2. Services
	orgService := services.NewOrganizationService(orgRepo)
	orgParamService := services.NewOrganizationParametersService(orgParamRepo)
	paramService := services.NewParametersService(paramRepo)
	teamService := services.NewTeamService(teamRepo)
	teamMembersService := services.NewTeamMembersService(teamMembersRepo)
	if checker, ok := userRepo.(services.UserOrganizationChecker); ok {
		teamMembersService.WithUserChecker(checker)
	}
	teamCliKeyService := services.NewTeamCliKeyService(teamCliKeyRepo)
	cliDeviceService := services.NewCliDeviceService(cliDeviceRepo)
	globalParamService := services.NewGlobalParametersService(globalParamRepo)

	// 3. Organization Use Cases
	getNameUC := orgusecases.NewGetOrganizationNameUseCase(orgRepo)
	getLangUC := orgusecases.NewGetOrganizationLanguageUseCase(repositories.NewPostgresTrackedRepositoryReader(pool))
	getDomainUC := orgusecases.NewGetOrganizationsByDomainUseCase(orgRepo, orgParamRepo)
	getReleaseTrackUC := orgusecases.NewGetReleaseTrackUseCase(orgRepo)
	updateInfosUC := orgusecases.NewUpdateInfosUseCase(orgRepo)

	// 4. Organization Parameters Use Cases
	orgParamFindByKeyUC := orgparamusecases.NewFindByKeyUseCase(orgParamRepo)
	orgParamCreateOrUpdateUC := orgparamusecases.NewCreateOrUpdateUseCase(orgParamRepo)
	getCockpitMetricsVisUC := orgparamusecases.NewGetCockpitMetricsVisibilityUseCase(orgParamRepo)
	getLLMConfigStatusUC := orgparamusecases.NewGetLLMConfigStatusUseCase(orgParamRepo)
	listModelOverridesUC := orgparamusecases.NewListModelOverridesUseCase(orgParamRepo)
	clearModelOverridesUC := orgparamusecases.NewClearModelOverridesUseCase(orgParamRepo)
	deleteBYOKConfigUC := orgparamusecases.NewDeleteBYOKConfigUseCase(orgParamRepo)
	getBYOKProvidersUC := orgparamusecases.NewGetBYOKProvidersUseCase(orgParamRepo)
	getModelsByProviderUC := orgparamusecases.NewGetModelsByProviderUseCase()
	getModelCapabilitiesUC := orgparamusecases.NewGetModelCapabilitiesUseCase()
	ignoreBotsUC := orgparamusecases.NewIgnoreBotsUseCase(orgParamRepo)
	testBYOKConnUC := orgparamusecases.NewTestBYOKConnectionUseCase()
	testBYOKModelUC := orgparamusecases.NewTestBYOKModelUseCase()

	// 5. Parameters Use Cases
	paramGetDefaultConfigUC := paramusecases.NewGetDefaultConfigUseCase()
	paramFindByKeyUC := paramusecases.NewFindByKeyParametersUseCase(paramRepo)
	paramCreateOrUpdateUC := paramusecases.NewCreateOrUpdateParametersUseCase(paramRepo)

	// 6. Team Use Cases
	createTeamUC := teamusecases.NewCreateTeamUseCase(teamRepo, paramRepo)
	listTeamsUC := teamusecases.NewListTeamsUseCase(teamRepo)
	listTeamsWithIntegrationsUC := teamusecases.NewListTeamsWithIntegrationsUseCase(teamRepo)

	// 7. Team Members Use Cases
	createTeamMemberUC := memberusecases.NewCreateOrUpdateTeamMembersUseCase(teamMembersRepo).WithService(teamMembersService)
	deleteTeamMemberUC := memberusecases.NewDeleteTeamMemberUseCase(teamMembersRepo).WithUserRepo(userRepo).WithTeamRepo(teamRepo)
	getTeamMembersUC := memberusecases.NewGetTeamMembersUseCase(teamMembersRepo).WithService(teamMembersService)

	// 8. Onboarding Use Cases
	joinOrgUC := onboardingusecases.NewJoinOrganizationUseCase(userRepo, orgRepo, teamRepo, teamMembersRepo, paramRepo)

	return &OrganizationModule{
		OrganizationRepo:           orgRepo,
		OrganizationParametersRepo: orgParamRepo,
		ParametersRepo:             paramRepo,
		TeamRepo:                   teamRepo,
		TeamMembersRepo:            teamMembersRepo,
		TeamCliKeyRepo:             teamCliKeyRepo,
		CliDeviceRepo:              cliDeviceRepo,
		GlobalParametersRepo:       globalParamRepo,

		OrganizationService:           orgService,
		OrganizationParametersService: orgParamService,
		ParametersService:             paramService,
		TeamService:                   teamService,
		TeamMembersService:            teamMembersService,
		TeamCliKeyService:             teamCliKeyService,
		CliDeviceService:              cliDeviceService,
		GlobalParametersService:       globalParamService,

		GetOrganizationNameUC:       getNameUC,
		GetOrganizationLanguageUC:   getLangUC,
		GetOrganizationsByDomainUC:  getDomainUC,
		GetReleaseTrackUC:           getReleaseTrackUC,
		UpdateInfosUC:               updateInfosUC,

		OrgParamFindByKeyUC:         orgParamFindByKeyUC,
		OrgParamCreateOrUpdateUC:    orgParamCreateOrUpdateUC,
		GetCockpitMetricsVisUC:      getCockpitMetricsVisUC,
		GetLLMConfigStatusUC:        getLLMConfigStatusUC,
		ListModelOverridesUC:        listModelOverridesUC,
		ClearModelOverridesUC:       clearModelOverridesUC,
		DeleteBYOKConfigUC:          deleteBYOKConfigUC,
		GetBYOKProvidersUC:          getBYOKProvidersUC,
		GetModelsByProviderUC:       getModelsByProviderUC,
		GetModelCapabilitiesUC:      getModelCapabilitiesUC,
		IgnoreBotsUC:                ignoreBotsUC,
		TestBYOKConnectionUC:        testBYOKConnUC,
		TestBYOKModelUC:             testBYOKModelUC,

		ParamGetDefaultConfigUC:     paramGetDefaultConfigUC,
		ParamFindByKeyUC:            paramFindByKeyUC,
		ParamCreateOrUpdateUC:       paramCreateOrUpdateUC,

		CreateTeamUC:                createTeamUC,
		ListTeamsUC:                 listTeamsUC,
		ListTeamsWithIntegrationsUC: listTeamsWithIntegrationsUC,

		CreateTeamMemberUC:          createTeamMemberUC,
		DeleteTeamMemberUC:          deleteTeamMemberUC,
		GetTeamMembersUC:            getTeamMembersUC,

		JoinOrganizationUC:          joinOrgUC,
	}
}

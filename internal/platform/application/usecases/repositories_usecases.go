package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/platform/application/services"
	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/internal/platform/dtos"
)

// PaginationMeta carries page numbering information.
type PaginationMeta struct {
	Page    int `json:"page"`
	PerPage int `json:"perPage"`
	Total   int `json:"total"`
}

// PaginatedRepositoriesResponse wraps repositories with pagination metadata.
type PaginatedRepositoriesResponse struct {
	Data       []*types.Repositories `json:"data"`
	Pagination PaginationMeta        `json:"pagination"`
}

// IAuthorizationScopeService resolves repository scopes granted to a user.
type IAuthorizationScopeService interface {
	GetRepositoryScope(ctx context.Context, userID string, orgID, teamID string) ([]string, error)
}

// IRepositoryConfigStore reads and writes repository configurations for teams.
type IRepositoryConfigStore interface {
	SaveRepositories(ctx context.Context, organizationID, teamID string, repos []*types.Repositories) error
	GetRepositories(ctx context.Context, organizationID, teamID string) ([]*types.Repositories, error)
	UpdateTeamStatus(ctx context.Context, teamID string, status string) error
}

// IAstGraphQueueService enqueues background AST code graph indexing.
type IAstGraphQueueService interface {
	EnqueueAstGraphBuild(ctx context.Context, orgID, teamID, repoID, repoName, cloneURL, defaultBranch string) error
}

// IHistoricalPRBackfillService triggers backfilling historical pull requests for analytics.
type IHistoricalPRBackfillService interface {
	BackfillHistoricalPRs(ctx context.Context, orgID, teamID string, repos []*types.Repositories) error
}

// ═══════════════════════════════════════════════════════════════
// 1. GetRepositoriesUseCase (translates get-repositories.ts)
// ═══════════════════════════════════════════════════════════════

type GetRepositoriesParams struct {
	OrganizationID string
	TeamID         string
	UserID         string
	Archived       *bool
	Visibility     string
	Language       string
	IsSelected     *bool
	Page           *int
	PerPage        *int
}

type GetRepositoriesUseCase struct {
	codeManagement contracts.ICodeManagementService
	authScope      IAuthorizationScopeService
}

func NewGetRepositoriesUseCase(
	codeManagement contracts.ICodeManagementService,
	authScope IAuthorizationScopeService,
) *GetRepositoriesUseCase {
	return &GetRepositoriesUseCase{
		codeManagement: codeManagement,
		authScope:      authScope,
	}
}

func (uc *GetRepositoriesUseCase) Execute(ctx context.Context, params GetRepositoriesParams) (*PaginatedRepositoriesResponse, error) {
	orgData := types.OrganizationAndTeamData{
		OrganizationID: params.OrganizationID,
		TeamID:         params.TeamID,
	}

	repos, err := uc.codeManagement.GetRepositories(ctx, orgData, params.Archived, params.Visibility, params.Language)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get repositories from code management",
			"organizationId", params.OrganizationID,
			"teamId", params.TeamID,
			"error", err,
		)
		return nil, err
	}

	// Filter by user repository scope permissions if restricted
	if uc.authScope != nil && params.UserID != "" {
		scope, err := uc.authScope.GetRepositoryScope(ctx, params.UserID, params.OrganizationID, params.TeamID)
		if err == nil && scope != nil {
			allowed := make(map[string]bool)
			for _, id := range scope {
				allowed[id] = true
			}

			var scoped []*types.Repositories
			for _, r := range repos {
				if r != nil && allowed[fmt.Sprintf("%v", r.ID)] {
					scoped = append(scoped, r)
				}
			}
			repos = scoped
		}
	}

	// Filter by selection status
	if params.IsSelected != nil {
		var selected []*types.Repositories
		for _, r := range repos {
			if r != nil && r.Selected == *params.IsSelected {
				selected = append(selected, r)
			}
		}
		repos = selected
	}

	total := len(repos)

	page := 1
	if params.Page != nil && *params.Page > 0 {
		page = *params.Page
	}

	perPage := 20
	if params.PerPage != nil && *params.PerPage > 0 {
		perPage = *params.PerPage
	}

	startIndex := (page - 1) * perPage
	if startIndex >= total {
		return &PaginatedRepositoriesResponse{
			Data: []*types.Repositories{},
			Pagination: PaginationMeta{
				Page:    page,
				PerPage: perPage,
				Total:   total,
			},
		}, nil
	}

	endIndex := startIndex + perPage
	if endIndex > total {
		endIndex = total
	}

	return &PaginatedRepositoriesResponse{
		Data: repos[startIndex:endIndex],
		Pagination: PaginationMeta{
			Page:    page,
			PerPage: perPage,
			Total:   total,
		},
	}, nil
}

// ═══════════════════════════════════════════════════════════════
// 2. GetSelectedRepositoriesUseCase (translates get-selected-repositories.use-case.ts)
// ═══════════════════════════════════════════════════════════════

type GetSelectedRepositoriesUseCase struct {
	configStore IRepositoryConfigStore
	authScope   IAuthorizationScopeService
}

func NewGetSelectedRepositoriesUseCase(
	configStore IRepositoryConfigStore,
	authScope IAuthorizationScopeService,
) *GetSelectedRepositoriesUseCase {
	return &GetSelectedRepositoriesUseCase{
		configStore: configStore,
		authScope:   authScope,
	}
}

func (uc *GetSelectedRepositoriesUseCase) Execute(
	ctx context.Context,
	orgID, teamID, userID string,
	page, perPage *int,
) (*PaginatedRepositoriesResponse, error) {
	if uc.configStore == nil {
		return &PaginatedRepositoriesResponse{Data: []*types.Repositories{}}, nil
	}

	repos, err := uc.configStore.GetRepositories(ctx, orgID, teamID)
	if err != nil {
		return nil, err
	}

	if uc.authScope != nil && userID != "" {
		scope, err := uc.authScope.GetRepositoryScope(ctx, userID, orgID, teamID)
		if err == nil && scope != nil {
			allowed := make(map[string]bool)
			for _, id := range scope {
				allowed[id] = true
			}
			var filtered []*types.Repositories
			for _, r := range repos {
				if r != nil && allowed[fmt.Sprintf("%v", r.ID)] {
					filtered = append(filtered, r)
				}
			}
			repos = filtered
		}
	}

	total := len(repos)
	p := 1
	if page != nil && *page > 0 {
		p = *page
	}
	pp := 20
	if perPage != nil && *perPage > 0 {
		pp = *perPage
	}

	startIndex := (p - 1) * pp
	if startIndex >= total {
		return &PaginatedRepositoriesResponse{
			Data: []*types.Repositories{},
			Pagination: PaginationMeta{
				Page:    p,
				PerPage: pp,
				Total:   total,
			},
		}, nil
	}

	endIndex := startIndex + pp
	if endIndex > total {
		endIndex = total
	}

	return &PaginatedRepositoriesResponse{
		Data: repos[startIndex:endIndex],
		Pagination: PaginationMeta{
			Page:    p,
			PerPage: pp,
			Total:   total,
		},
	}, nil
}

// ═══════════════════════════════════════════════════════════════
// 3. CreateRepositoriesUseCase (translates create-repositories.ts)
// ═══════════════════════════════════════════════════════════════

type CreateRepositoriesUseCase struct {
	configStore     IRepositoryConfigStore
	backfillService IHistoricalPRBackfillService
	astQueueService IAstGraphQueueService
	inFlightMu      sync.Mutex
	inFlightBackfill map[string]bool
}

func NewCreateRepositoriesUseCase(
	configStore IRepositoryConfigStore,
	backfillService IHistoricalPRBackfillService,
	astQueueService IAstGraphQueueService,
) *CreateRepositoriesUseCase {
	return &CreateRepositoriesUseCase{
		configStore:      configStore,
		backfillService:  backfillService,
		astQueueService:  astQueueService,
		inFlightBackfill: make(map[string]bool),
	}
}

type CreateRepositoriesParams struct {
	OrganizationID string
	TeamID         string
	Repositories   []*types.Repositories
}

func (uc *CreateRepositoriesUseCase) Execute(ctx context.Context, params CreateRepositoriesParams) error {
	if strings.TrimSpace(params.OrganizationID) == "" {
		return fmt.Errorf("organizationId is required")
	}
	if strings.TrimSpace(params.TeamID) == "" {
		return fmt.Errorf("teamId is required")
	}

	// 1. Persist repository configuration
	if uc.configStore != nil {
		if err := uc.configStore.SaveRepositories(ctx, params.OrganizationID, params.TeamID, params.Repositories); err != nil {
			return fmt.Errorf("failed to save repositories configuration: %w", err)
		}
		// Activate team
		_ = uc.configStore.UpdateTeamStatus(ctx, params.TeamID, "ACTIVE")
	}

	// 2. Single-flight historical PR backfill
	backfillKey := fmt.Sprintf("%s:%s", params.OrganizationID, params.TeamID)
	uc.inFlightMu.Lock()
	alreadyRunning := uc.inFlightBackfill[backfillKey]
	if !alreadyRunning && len(params.Repositories) > 0 {
		uc.inFlightBackfill[backfillKey] = true
	}
	uc.inFlightMu.Unlock()

	if !alreadyRunning && len(params.Repositories) > 0 && uc.backfillService != nil {
		go func() {
			defer func() {
				uc.inFlightMu.Lock()
				delete(uc.inFlightBackfill, backfillKey)
				uc.inFlightMu.Unlock()
			}()

			bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			if err := uc.backfillService.BackfillHistoricalPRs(bgCtx, params.OrganizationID, params.TeamID, params.Repositories); err != nil {
				slog.Error("Historical PR backfill failed",
					"organizationId", params.OrganizationID,
					"teamId", params.TeamID,
					"error", err,
				)
			}
		}()
	}

	// 3. Enqueue AST Graph Builds asynchronously
	if uc.astQueueService != nil && len(params.Repositories) > 0 {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			for _, repo := range params.Repositories {
				if repo == nil {
					continue
				}
				repoID := fmt.Sprintf("%v", repo.ID)
				_ = uc.astQueueService.EnqueueAstGraphBuild(
					bgCtx,
					params.OrganizationID,
					params.TeamID,
					repoID,
					repo.Name,
					repo.HTTPURL,
					repo.DefaultBranch,
				)
			}
		}()
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════
// 4. GetRepositoryTreeByDirectoryUseCase (translates get-repository-tree-by-directory.use-case.ts)
// ═══════════════════════════════════════════════════════════════

type DirectoryItem struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	SHA         string `json:"sha"`
	HasChildren bool   `json:"hasChildren"`
}

type RepositoryTreeByDirectoryResponse struct {
	Repository  string          `json:"repository"`
	ParentPath  *string         `json:"parentPath"`
	CurrentPath string          `json:"currentPath"`
	Directories []DirectoryItem `json:"directories"`
}

type GetRepositoryTreeByDirectoryUseCase struct {
	codeManagement contracts.ICodeManagementService
	cache          *services.MemoryCache
}

func NewGetRepositoryTreeByDirectoryUseCase(
	codeManagement contracts.ICodeManagementService,
	cache *services.MemoryCache,
) *GetRepositoryTreeByDirectoryUseCase {
	if cache == nil {
		cache = services.NewMemoryCache()
	}
	return &GetRepositoryTreeByDirectoryUseCase{
		codeManagement: codeManagement,
		cache:          cache,
	}
}

func (uc *GetRepositoryTreeByDirectoryUseCase) Execute(
	ctx context.Context,
	orgID string,
	dto *dtos.GetRepositoryTreeByDirectoryDTO,
) (*RepositoryTreeByDirectoryResponse, error) {
	if err := dto.Validate(); err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("tree_%s_%s_%s_%s", orgID, dto.TeamID, dto.RepositoryID, dto.DirectoryPath)

	if dto.UseCache {
		if cached, ok := uc.cache.Get(cacheKey); ok {
			if resp, ok := cached.(*RepositoryTreeByDirectoryResponse); ok {
				return resp, nil
			}
		}
	}

	orgData := types.OrganizationAndTeamData{
		OrganizationID: orgID,
		TeamID:         dto.TeamID,
	}

	treeItems, err := uc.codeManagement.GetRepositoryTreeByDirectory(ctx, orgData, dto.RepositoryID, dto.DirectoryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository tree: %w", err)
	}

	items := make([]DirectoryItem, 0, len(treeItems))
	for _, ti := range treeItems {
		if ti == nil {
			continue
		}
		items = append(items, DirectoryItem{
			Name:        path.Base(ti.Path),
			Path:        ti.Path,
			SHA:         ti.SHA,
			HasChildren: ti.Type == "tree" || ti.Type == "dir",
		})
	}

	var parentPath *string
	cleanDir := strings.Trim(dto.DirectoryPath, "/")
	if cleanDir != "" {
		parent := path.Dir(cleanDir)
		if parent == "." {
			parent = ""
		}
		parentPath = &parent
	}

	resp := &RepositoryTreeByDirectoryResponse{
		Repository:  dto.RepositoryID,
		ParentPath:  parentPath,
		CurrentPath: cleanDir,
		Directories: items,
	}

	uc.cache.Set(cacheKey, resp, 15*time.Minute)

	return resp, nil
}

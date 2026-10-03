package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	orgusecases "github.com/scandrix/backend/internal/organization/application/usecases/organization"
	orgdomain "github.com/scandrix/backend/internal/organization/domain/organization"
	orgparamdomain "github.com/scandrix/backend/internal/organization/domain/organizationparameters"
	"github.com/scandrix/backend/internal/organization/infrastructure/repositories"
	"github.com/scandrix/backend/pkg/models"
)

// OrganizationRepository defines the data access contract for organization metadata (Clean Architecture).
type OrganizationRepository interface {
	GetWorkspaceByID(ctx context.Context, id uuid.UUID) (*models.Workspace, error)
	ListWorkspacesForUser(ctx context.Context, email string) ([]models.Workspace, error)
}

// WorkspaceUpdater optionally updates workspace names if supported by repository.
type WorkspaceUpdater interface {
	UpdateWorkspace(ctx context.Context, id uuid.UUID, name string) error
}

// OrganizationController handles organization metadata, language preferences, and domain discovery via Clean Architecture use cases.
type OrganizationController struct {
	repo              OrganizationRepository
	getNameUC         *orgusecases.GetOrganizationNameUseCase
	getLanguageUC     *orgusecases.GetOrganizationLanguageUseCase
	getDomainUC       *orgusecases.GetOrganizationsByDomainUseCase
	getReleaseTrackUC *orgusecases.GetReleaseTrackUseCase
	updateInfosUC     *orgusecases.UpdateInfosUseCase
}

// orgRepoAdapter bridges OrganizationRepository into orgdomain.IOrganizationRepository.
type orgRepoAdapter struct {
	repo OrganizationRepository
	mem  orgdomain.IOrganizationRepository
}

func newOrgRepoAdapter(repo OrganizationRepository) orgdomain.IOrganizationRepository {
	mem := repositories.NewPostgresOrganizationRepository(nil)
	return &orgRepoAdapter{repo: repo, mem: mem}
}

func (a *orgRepoAdapter) Find(ctx context.Context, filter orgdomain.OrganizationFilter) ([]*orgdomain.OrganizationEntity, error) {
	if a.repo != nil {
		// Scoped to the caller. If there is no authenticated caller in the
		// context, return nothing: this adapter previously listed every
		// workspace in the deployment, which handed any authenticated user an
		// arbitrary tenant.
		email, cerr := auth.CallerEmail(ctx)
		if cerr != nil {
			return nil, nil
		}
		wsList, err := a.repo.ListWorkspacesForUser(ctx, email)
		if err == nil && len(wsList) > 0 {
			var entities []*orgdomain.OrganizationEntity
			for _, w := range wsList {
				entities = append(entities, &orgdomain.OrganizationEntity{
					UUID:         w.ID,
					Name:         w.Name,
					TenantName:   w.Slug,
					Status:       w.Status == models.TenantStatusActive,
					ReleaseTrack: orgdomain.ReleaseTrackStable,
					CreatedAt:    w.CreatedAt,
					UpdatedAt:    w.UpdatedAt,
				})
			}
			return entities, nil
		}
	}
	return a.mem.Find(ctx, filter)
}

func (a *orgRepoAdapter) FindOne(ctx context.Context, filter orgdomain.OrganizationFilter) (*orgdomain.OrganizationEntity, error) {
	list, err := a.Find(ctx, filter)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return list[0], nil
}

func (a *orgRepoAdapter) FindByID(ctx context.Context, id uuid.UUID) (*orgdomain.OrganizationEntity, error) {
	if a.repo != nil {
		ws, err := a.repo.GetWorkspaceByID(ctx, id)
		if err == nil && ws != nil {
			return &orgdomain.OrganizationEntity{
				UUID:         ws.ID,
				Name:         ws.Name,
				TenantName:   ws.Slug,
				Status:       ws.Status == models.TenantStatusActive,
				ReleaseTrack: orgdomain.ReleaseTrackStable,
				CreatedAt:    ws.CreatedAt,
				UpdatedAt:    ws.UpdatedAt,
			}, nil
		}
	}
	return a.mem.FindByID(ctx, id)
}

func (a *orgRepoAdapter) FindByUserID(ctx context.Context, userID uuid.UUID) (*orgdomain.OrganizationEntity, error) {
	return a.mem.FindByUserID(ctx, userID)
}

func (a *orgRepoAdapter) Create(ctx context.Context, entity *orgdomain.OrganizationEntity) (*orgdomain.OrganizationEntity, error) {
	return a.mem.Create(ctx, entity)
}

func (a *orgRepoAdapter) DeleteOne(ctx context.Context, filter orgdomain.OrganizationFilter) error {
	return a.mem.DeleteOne(ctx, filter)
}

func (a *orgRepoAdapter) Delete(ctx context.Context, id uuid.UUID) error {
	return a.mem.Delete(ctx, id)
}

func (a *orgRepoAdapter) Update(ctx context.Context, filter orgdomain.OrganizationFilter, data *orgdomain.OrganizationEntity) (*orgdomain.OrganizationEntity, error) {
	if a.repo != nil && filter.UUID != nil {
		if updater, ok := a.repo.(WorkspaceUpdater); ok {
			_ = updater.UpdateWorkspace(ctx, *filter.UUID, data.Name)
		}
	}
	return a.mem.Update(ctx, filter, data)
}

// NewOrganizationController initializes the organization controller with wired Clean Architecture use cases.
func NewOrganizationController(repo OrganizationRepository) *OrganizationController {
	if isNilInterface(repo) {
		repo = nil
	}

	adapter := newOrgRepoAdapter(repo)
	var reader orgusecases.TrackedRepositoryReader
	if r, ok := repo.(orgusecases.TrackedRepositoryReader); ok {
		reader = r
	}

	var paramRepo orgparamdomain.IOrganizationParametersRepository
	if pr, ok := repo.(orgparamdomain.IOrganizationParametersRepository); ok {
		paramRepo = pr
	} else {
		paramRepo = repositories.NewPostgresOrganizationParametersRepository(nil)
	}

	return &OrganizationController{
		repo:              repo,
		getNameUC:         orgusecases.NewGetOrganizationNameUseCase(adapter),
		getLanguageUC:     orgusecases.NewGetOrganizationLanguageUseCase(reader),
		getDomainUC:       orgusecases.NewGetOrganizationsByDomainUseCase(adapter, paramRepo),
		getReleaseTrackUC: orgusecases.NewGetReleaseTrackUseCase(adapter),
		updateInfosUC:     orgusecases.NewUpdateInfosUseCase(adapter),
	}
}

// WithUseCases overrides constructor-wired use cases with module-injected ones.
func (c *OrganizationController) WithUseCases(
	getNameUC *orgusecases.GetOrganizationNameUseCase,
	getLanguageUC *orgusecases.GetOrganizationLanguageUseCase,
	updateInfosUC *orgusecases.UpdateInfosUseCase,
	getDomainUC *orgusecases.GetOrganizationsByDomainUseCase,
	getReleaseTrackUC *orgusecases.GetReleaseTrackUseCase,
) *OrganizationController {
	if getNameUC != nil {
		c.getNameUC = getNameUC
	}
	if getLanguageUC != nil {
		c.getLanguageUC = getLanguageUC
	}
	if updateInfosUC != nil {
		c.updateInfosUC = updateInfosUC
	}
	if getDomainUC != nil {
		c.getDomainUC = getDomainUC
	}
	if getReleaseTrackUC != nil {
		c.getReleaseTrackUC = getReleaseTrackUC
	}
	return c
}

// Routes mounts organization endpoints.
func (c *OrganizationController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/name", c.handleGetName)
	r.Get("/language", c.handleGetLanguage)
	r.Get("/domain", c.handleGetDomain)
	r.Get("/release-track", c.handleGetReleaseTrack)
	r.Post("/update-infos", c.handleUpdateInfos)
	r.Patch("/update-infos", c.handleUpdateInfos)

	return r
}

func (c *OrganizationController) handleGetName(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())

	name := "Workspace"
	if c.getNameUC != nil && wsID != uuid.Nil {
		if fetchedName, err := c.getNameUC.Execute(r.Context(), wsID); err == nil && fetchedName != "" {
			name = fetchedName
		}
	} else if c.repo != nil {
		// Scoped to the caller: an unscoped list would leak another tenant name.
		if email, e := auth.CallerEmail(r.Context()); e == nil {
			if wsList, err := c.repo.ListWorkspacesForUser(r.Context(), email); err == nil && len(wsList) > 0 {
				name = wsList[0].Name
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": name,
	})
}

func (c *OrganizationController) handleGetLanguage(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())
	teamIDStr := r.URL.Query().Get("teamId")
	repoID := r.URL.Query().Get("repositoryId")
	sampleSizeStr := r.URL.Query().Get("sampleSize")

	var teamID *uuid.UUID
	if tid, err := uuid.Parse(teamIDStr); err == nil {
		teamID = &tid
	}
	sampleSize := 5
	if sampleSizeStr != "" {
		if s, err := strconv.Atoi(sampleSizeStr); err == nil && s > 0 {
			sampleSize = s
		}
	}

	lang := "en"
	if c.getLanguageUC != nil && wsID != uuid.Nil {
		if detected, err := c.getLanguageUC.Execute(r.Context(), wsID, teamID, repoID, sampleSize); err == nil && detected != "" {
			lang = detected
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]any{
			"language": lang,
		},
	})
}

func (c *OrganizationController) handleGetDomain(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")

	var orgs []*orgdomain.OrganizationEntity
	if c.getDomainUC != nil && domain != "" {
		if matched, err := c.getDomainUC.Execute(r.Context(), domain); err == nil {
			orgs = matched
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if len(orgs) == 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []any{},
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": orgs,
	})
}

func (c *OrganizationController) handleUpdateInfos(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())

	var payload struct {
		Name             string `json:"name"`
		OrganizationName string `json:"organizationName"`
		Phone            string `json:"phone"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)

	newName := payload.OrganizationName
	if newName == "" {
		newName = payload.Name
	}

	profile, ok := auth.AccountProfileFromContext(r.Context())
	var userID *uuid.UUID
	if ok && profile != nil && profile.ID != uuid.Nil {
		userID = &profile.ID
	}
	var phonePtr *string
	if payload.Phone != "" {
		phonePtr = &payload.Phone
	}

	if c.updateInfosUC != nil && wsID != uuid.Nil && (newName != "" || phonePtr != nil) {
		_ = c.updateInfosUC.ExecuteWithPhone(r.Context(), wsID, userID, newName, phonePtr)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"success": true,
		},
	})
}

func (c *OrganizationController) handleGetReleaseTrack(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())
	track := orgdomain.DefaultReleaseTrack
	if c.getReleaseTrackUC != nil && wsID != uuid.Nil {
		if t, err := c.getReleaseTrackUC.Execute(r.Context(), wsID); err == nil {
			track = t
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":         string(track),
		"releaseTrack": string(track),
	})
}

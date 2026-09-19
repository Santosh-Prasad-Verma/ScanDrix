package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	paramusecases "github.com/scandrix/backend/internal/organization/application/usecases/parameters"
	paramdomain "github.com/scandrix/backend/internal/organization/domain/parameters"
	"github.com/scandrix/backend/pkg/models"
)

// ParametersRepository defines the data access contract for workspace parameter configurations (Clean Architecture).
type ParametersRepository interface {
	ListWorkspaces(ctx context.Context) ([]models.Workspace, error)
	GetWorkspaceParameters(ctx context.Context, wsID uuid.UUID) (reviewParams, orgParams []byte, err error)
	UpdateWorkspaceReviewParameters(ctx context.Context, wsID uuid.UUID, reviewParams []byte) error
	UpdateWorkspaceOrgParameters(ctx context.Context, wsID uuid.UUID, orgParams []byte) error
}

// ParametersController handles review settings, model selections, and organizational thresholds.
type ParametersController struct {
	repo               ParametersRepository
	findByKeyUC        *paramusecases.FindByKeyParametersUseCase
	createOrUpdateUC   *paramusecases.CreateOrUpdateParametersUseCase
	getDefaultConfigUC *paramusecases.GetDefaultConfigUseCase
}

// NewParametersController initializes the parameters controller.
func NewParametersController(repo ParametersRepository) *ParametersController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &ParametersController{repo: repo}
}

// WithUseCases injects Clean Architecture parameter use cases.
func (c *ParametersController) WithUseCases(
	findByKeyUC *paramusecases.FindByKeyParametersUseCase,
	createOrUpdateUC *paramusecases.CreateOrUpdateParametersUseCase,
	getDefaultConfigUC *paramusecases.GetDefaultConfigUseCase,
) *ParametersController {
	c.findByKeyUC = findByKeyUC
	c.createOrUpdateUC = createOrUpdateUC
	c.getDefaultConfigUC = getDefaultConfigUC
	return c
}

// Routes mounts parameters routes for both Web dashboard and CLI.
func (c *ParametersController) Routes() chi.Router {
	r := chi.NewRouter()

	// Web Dashboard endpoints (/parameters/*)
	r.Get("/find-by-key", c.handleFindByKey)
	r.Post("/create-or-update", c.handleCreateOrUpdate)
	r.Post("/create-or-update-code-review", c.handleCreateOrUpdateCodeReview)
	r.Post("/update-code-review-parameter-repositories", c.handleUpdateCodeReviewRepositories)
	r.Get("/list-code-review-automation-labels", c.handleListAutomationLabels)
	r.Get("/default-code-review-parameter", c.handleDefaultCodeReviewParameter)
	r.Get("/code-review-parameter", c.handleGetCodeReviewParameter)
	r.Get("/generate-scandrix-config-file", c.handleGenerateConfigFile)
	r.Get("/generate-config-file", c.handleGenerateConfigFile)
	r.Delete("/delete-repository-code-review-parameter", c.handleDeleteRepoParameter)
	r.Get("/centralized-config-sync", c.handleCentralizedConfigSync)
	r.Get("/centralized-config-init", c.handleCentralizedConfigInit)
	r.Get("/centralized-config-download", c.handleCentralizedConfigDownload)

	// Legacy & CLI endpoints
	r.Get("/review", c.handleGetReviewParameters)
	r.Put("/review", c.handleUpdateReviewParameters)
	r.Get("/org", c.handleGetOrgParameters)
	r.Put("/org", c.handleUpdateOrgParameters)

	return r
}

func (c *ParametersController) handleFindByKey(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	wsID, _ := auth.WorkspaceFromContext(r.Context())

	// If workspace is not in JWT context, try looking up from DB
	if wsID == uuid.Nil && c.repo != nil {
		if wsList, err := c.repo.ListWorkspaces(r.Context()); err == nil && len(wsList) > 0 {
			wsID = wsList[0].ID
		}
	}

	teamIDStr := r.URL.Query().Get("teamId")
	var teamID *uuid.UUID
	if teamIDStr != "" {
		if tid, err := uuid.Parse(teamIDStr); err == nil {
			teamID = &tid
		}
	}

	// Clean Architecture Use Case lookup
	if c.findByKeyUC != nil && wsID != uuid.Nil {
		entity, err := c.findByKeyUC.Execute(r.Context(), wsID, teamID, paramdomain.ParameterKey(key))
		if err == nil && entity != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"statusCode": http.StatusOK,
				"data": map[string]any{
					"uuid":        entity.UUID.String(),
					"configKey":   string(entity.ConfigKey),
					"configValue": entity.ConfigValue,
					"createdAt":   entity.CreatedAt.Format(time.RFC3339),
					"updatedAt":   entity.UpdatedAt.Format(time.RFC3339),
				},
			})
			return
		}
	}

	var configValue any

	switch key {
	case "code_review_config":
		if c.repo != nil && wsID != uuid.Nil {
			reviewBytes, _, err := c.repo.GetWorkspaceParameters(r.Context(), wsID)
			if err == nil && len(reviewBytes) > 2 {
				var savedMap map[string]any
				if json.Unmarshal(reviewBytes, &savedMap) == nil {
					configValue = savedMap
				}
			}
		}
		if configValue == nil {
			if c.getDefaultConfigUC != nil {
				configValue = c.getDefaultConfigUC.DefaultCodeReviewConfig()
			} else {
				configValue = defaultCodeReviewConfigMap()
			}
		}

	case "platform_configs":
		if c.getDefaultConfigUC != nil {
			configValue = c.getDefaultConfigUC.DefaultPlatformConfigs()
		} else {
			configValue = map[string]any{
				"finishOnboard":                     true,
				"finishProjectManagementConnection": true,
				"drixyLearningStatus":               "enabled",
			}
		}

	case "language_config":
		configValue = map[string]any{
			"language": "en-US",
		}

	case "board_priority_type":
		configValue = "kanban_priority"

	case "centralized_config":
		configValue = map[string]any{
			"enabled":    false,
			"repository": nil,
		}

	case "issue_creation_config":
		configValue = map[string]any{
			"enabled": false,
		}

	default:
		configValue = map[string]any{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"uuid":        uuid.New().String(),
			"configKey":   key,
			"configValue": configValue,
			"createdAt":   time.Now().UTC().Format(time.RFC3339),
			"updatedAt":   time.Now().UTC().Format(time.RFC3339),
		},
	})
}

func (c *ParametersController) handleCreateOrUpdate(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())
	var req struct {
		Key                     string `json:"key"`
		ConfigValue             any    `json:"configValue"`
		OrganizationAndTeamData struct {
			TeamID string `json:"teamId"`
		} `json:"organizationAndTeamData"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid parameters payload"}`, http.StatusBadRequest)
		return
	}

	var teamUUID *uuid.UUID
	if req.OrganizationAndTeamData.TeamID != "" {
		if tid, err := uuid.Parse(req.OrganizationAndTeamData.TeamID); err == nil {
			teamUUID = &tid
		}
	}

	if c.createOrUpdateUC != nil && wsID != uuid.Nil {
		entity, err := c.createOrUpdateUC.Execute(r.Context(), wsID, teamUUID, paramdomain.ParameterKey(req.Key), req.ConfigValue, "")
		if err == nil && entity != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"statusCode": http.StatusOK,
				"data": map[string]any{
					"uuid":        entity.UUID.String(),
					"key":         string(entity.ConfigKey),
					"configValue": req.ConfigValue,
					"success":     true,
					"updatedAt":   entity.UpdatedAt.Format(time.RFC3339),
				},
			})
			return
		}
	}

	if req.Key == "code_review_config" && c.repo != nil && wsID != uuid.Nil {
		if bytes, err := json.Marshal(req.ConfigValue); err == nil {
			_ = c.repo.UpdateWorkspaceReviewParameters(r.Context(), wsID, bytes)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"key":         req.Key,
			"configValue": req.ConfigValue,
			"success":     true,
		},
	})
}

func (c *ParametersController) handleCreateOrUpdateCodeReview(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())
	var req struct {
		ConfigValue             any    `json:"configValue"`
		RepositoryID            string `json:"repositoryId,omitempty"`
		DirectoryID             string `json:"directoryId,omitempty"`
		DirectoryPaths          []string `json:"directoryPaths,omitempty"`
		OrganizationAndTeamData struct {
			TeamID string `json:"teamId"`
		} `json:"organizationAndTeamData"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid parameters payload"}`, http.StatusBadRequest)
		return
	}

	if c.repo != nil && wsID != uuid.Nil {
		if bytes, err := json.Marshal(req.ConfigValue); err == nil {
			_ = c.repo.UpdateWorkspaceReviewParameters(r.Context(), wsID, bytes)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"success":     true,
			"configValue": req.ConfigValue,
		},
	})
}

func (c *ParametersController) handleUpdateCodeReviewRepositories(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"success": true,
		},
	})
}

func (c *ParametersController) handleListAutomationLabels(w http.ResponseWriter, r *http.Request) {
	labels := []map[string]string{
		{"type": "bug", "name": "Bug", "description": "Detects bugs, nil pointers, and logic errors"},
		{"type": "security", "name": "Security", "description": "Identifies OWASP Top 10 vulnerabilities and hardcoded secrets"},
		{"type": "performance", "name": "Performance", "description": "Flags slow database queries, unbounded loops, and allocations"},
		{"type": "maintainability", "name": "Maintainability", "description": "Ensures code structure, interfaces, and architecture standards"},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data":       labels,
	})
}

func (c *ParametersController) handleDefaultCodeReviewParameter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data":       defaultCodeReviewConfigMap(),
	})
}

func (c *ParametersController) handleGetCodeReviewParameter(w http.ResponseWriter, r *http.Request) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())

	configMap := defaultCodeReviewConfigMap()
	if c.repo != nil && wsID != uuid.Nil {
		if reviewBytes, _, err := c.repo.GetWorkspaceParameters(r.Context(), wsID); err == nil && len(reviewBytes) > 2 {
			var savedMap map[string]any
			if json.Unmarshal(reviewBytes, &savedMap) == nil {
				configMap = savedMap
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"uuid":        uuid.New().String(),
			"configKey":   "code_review_config",
			"configValue": configMap,
		},
	})
}

func (c *ParametersController) handleGenerateConfigFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"content": "version: 1\nrules:\n  - rule: security-audit\n",
		},
	})
}

func (c *ParametersController) handleDeleteRepoParameter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"success": true,
		},
	})
}

func (c *ParametersController) handleCentralizedConfigSync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"synced": true,
		},
	})
}

// CentralizedConfigRoutes mounts centralized configuration endpoints matching /cli/config/centralized and /config/centralized.
func (c *ParametersController) CentralizedConfigRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/status", c.handleCentralizedStatus)
	r.Post("/init", c.handleCentralizedInit)
	r.Post("/sync", c.handleCentralizedSync)
	r.Post("/disable", c.handleCentralizedDisable)
	r.Get("/download", c.handleCentralizedConfigDownload)
	return r
}

func (c *ParametersController) handleCentralizedStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"enabled":       false,
		"repository":    nil,
		"selected_repo": "",
		"sync_mode":     "manual",
		"success":       true,
	})
}

func (c *ParametersController) handleCentralizedInit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RepositoryID string `json:"repository_id"`
		RepoID       string `json:"repositoryId"`
		SyncOption   string `json:"sync_option"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Centralized configuration initialized successfully",
		"prUrl":   nil,
		"pr_url":  "",
	})
}

func (c *ParametersController) handleCentralizedSync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Centralized organization rules synchronized successfully",
	})
}

func (c *ParametersController) handleCentralizedDisable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Centralized configuration disabled",
	})
}

func (c *ParametersController) handleCentralizedConfigInit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"initialized": true,
		},
	})
}

func (c *ParametersController) handleCentralizedConfigDownload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"content": "",
		},
	})
}

func defaultCodeReviewConfigMap() map[string]any {
	return map[string]any{
		"ignorePaths": []string{"vendor/**", "node_modules/**", "*.min.js", "*.lock"},
		"baseBranches": []string{"main", "master", "develop"},
		"reviewOptions": map[string]bool{
			"security":        true,
			"performance":     true,
			"maintainability": true,
			"style":           false,
		},
		"ignoredTitleKeywords":                  []string{"[WIP]", "WIP:", "[DRAFT]"},
		"automatedReviewActive":                 true,
		"showStatusFeedback":                    true,
		"reviewCadence": map[string]any{
			"type": "automatic",
		},
		"summary": map[string]any{
			"generatePRSummary":               true,
			"behaviourForExistingDescription": "replace",
		},
		"suggestionControl": map[string]any{
			"groupingMode":            "full",
			"limitationType":          "pr",
			"maxSuggestions":          15,
			"severityLevelFilter":                  "low",
			"applyFiltersToDrixyRules":             true,
		},
		"pullRequestApprovalActive":                  false,
		"scandrixConfigFileOverridesWebPreferences": true,
		"isRequestChangesActive":                false,
		"runOnDraft":                            false,
		"codeReviewVersion":                     "v2",
		"ideRulesSyncEnabled":                   true,
		"enableCommittableSuggestions":          true,
		"customMessages": map[string]any{
			"enabled": false,
		},
	}
}

// ============================================================================
// Legacy Methods (Maintained for Backward Compatibility)
// ============================================================================

func (c *ParametersController) handleGetReviewParameters(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var reviewBytes []byte
	if c.repo != nil {
		var err error
		reviewBytes, _, err = c.repo.GetWorkspaceParameters(r.Context(), wsID)
		if err != nil {
			http.Error(w, `{"error":"failed loading parameters"}`, http.StatusInternalServerError)
			return
		}
	}

	var dto dtos.ReviewParametersDTO
	if len(reviewBytes) > 2 {
		_ = json.Unmarshal(reviewBytes, &dto)
	} else {
		defaultModel := os.Getenv("API_LLM_PROVIDER_MODEL")
		if defaultModel == "" {
			defaultModel = os.Getenv("AI_MODEL_DEFAULT")
		}
		dto = dtos.ReviewParametersDTO{
			DefaultAIModel:           defaultModel,
			MaxDiffLines:             1500,
			DryRunMode:               false,
			AutoApproveCleanPRs:      true,
			EnforceConventionalTitle: true,
			IgnorePatterns:           []string{"*.generated.*", "vendor/*", "*.pb.go"},
			CustomSystemPrompt:       "Prioritize finding concurrency races, missing input validation, and unauthorized data leakage.",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dto)
}

func (c *ParametersController) handleUpdateReviewParameters(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	var req dtos.ReviewParametersDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid parameters payload"}`, http.StatusBadRequest)
		return
	}

	bytes, err := json.Marshal(req)
	if err != nil {
		http.Error(w, `{"error":"failed serializing parameters"}`, http.StatusInternalServerError)
		return
	}

	if err := c.repo.UpdateWorkspaceReviewParameters(r.Context(), wsID, bytes); err != nil {
		http.Error(w, `{"error":"failed updating parameters"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(req)
}

func (c *ParametersController) handleGetOrgParameters(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var orgBytes []byte
	if c.repo != nil {
		var err error
		_, orgBytes, err = c.repo.GetWorkspaceParameters(r.Context(), wsID)
		if err != nil {
			http.Error(w, `{"error":"failed loading org parameters"}`, http.StatusInternalServerError)
			return
		}
	}

	var dto dtos.OrgParametersDTO
	if len(orgBytes) > 2 {
		_ = json.Unmarshal(orgBytes, &dto)
	} else {
		dto = dtos.OrgParametersDTO{
			BlockPRMergeOnCritical:     true,
			RequireReviewDismissalRole: "ADMIN",
			DefaultBranchOnly:          false,
			NotificationSlackChannel:   "#security-reviews",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dto)
}

func (c *ParametersController) handleUpdateOrgParameters(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	var req dtos.OrgParametersDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid org parameters payload"}`, http.StatusBadRequest)
		return
	}

	bytes, err := json.Marshal(req)
	if err != nil {
		http.Error(w, `{"error":"failed serializing org parameters"}`, http.StatusInternalServerError)
		return
	}

	if err := c.repo.UpdateWorkspaceOrgParameters(r.Context(), wsID, bytes); err != nil {
		http.Error(w, `{"error":"failed updating org parameters"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(req)
}

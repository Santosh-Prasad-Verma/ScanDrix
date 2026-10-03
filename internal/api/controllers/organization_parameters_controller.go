// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers"
	_ "github.com/scandrix/backend/internal/llm/providers/all"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
	"github.com/scandrix/backend/internal/llm/validation"
	orgparamusecases "github.com/scandrix/backend/internal/organization/application/usecases/organizationparameters"
	orgparamdomain "github.com/scandrix/backend/internal/organization/domain/organizationparameters"
	"github.com/scandrix/backend/pkg/models"
)

// OrgParametersRepository defines the data access contract for organization parameter configurations (Clean Architecture).
type OrgParametersRepository interface {
	ListWorkspacesForUser(ctx context.Context, email string) ([]models.Workspace, error)
	GetOrganizationParameter(ctx context.Context, wsID uuid.UUID, key string) (*models.OrganizationParameter, error)
	SetOrganizationParameter(ctx context.Context, wsID uuid.UUID, key string, val []byte, desc string) error
	DeleteOrganizationParameter(ctx context.Context, wsID uuid.UUID, key string) error
	ListOrganizationParameters(ctx context.Context, wsID uuid.UUID) ([]models.OrganizationParameter, error)
}

// OrganizationParametersController manages organization-level settings, BYOK, models, and metric visibility.
type OrganizationParametersController struct {
	repo                   OrgParametersRepository
	findByKeyUC            *orgparamusecases.FindByKeyUseCase
	createOrUpdateUC       *orgparamusecases.CreateOrUpdateUseCase
	getCockpitMetricsVisUC *orgparamusecases.GetCockpitMetricsVisibilityUseCase
	getLLMConfigStatusUC   *orgparamusecases.GetLLMConfigStatusUseCase
	listModelOverridesUC   *orgparamusecases.ListModelOverridesUseCase
	clearModelOverridesUC  *orgparamusecases.ClearModelOverridesUseCase
	deleteBYOKConfigUC     *orgparamusecases.DeleteBYOKConfigUseCase
	getByokProvidersUC     *orgparamusecases.GetBYOKProvidersUseCase
	getModelsByProviderUC  *orgparamusecases.GetModelsByProviderUseCase
	getModelCapabilitiesUC *orgparamusecases.GetModelCapabilitiesUseCase
	ignoreBotsUC           *orgparamusecases.IgnoreBotsUseCase
	testByokConnUC         *orgparamusecases.TestBYOKConnectionUseCase
	testByokModelUC        *orgparamusecases.TestBYOKModelUseCase
}

// NewOrganizationParametersController initializes the organization parameters controller.
func NewOrganizationParametersController(repo OrgParametersRepository) *OrganizationParametersController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &OrganizationParametersController{repo: repo}
}

// WithUseCases injects Clean Architecture organization parameter use cases.
func (c *OrganizationParametersController) WithUseCases(
	findByKeyUC *orgparamusecases.FindByKeyUseCase,
	createOrUpdateUC *orgparamusecases.CreateOrUpdateUseCase,
	getCockpitMetricsVisUC *orgparamusecases.GetCockpitMetricsVisibilityUseCase,
	getLLMConfigStatusUC *orgparamusecases.GetLLMConfigStatusUseCase,
	listModelOverridesUC *orgparamusecases.ListModelOverridesUseCase,
	clearModelOverridesUC *orgparamusecases.ClearModelOverridesUseCase,
	deleteBYOKConfigUC *orgparamusecases.DeleteBYOKConfigUseCase,
	getByokProvidersUC *orgparamusecases.GetBYOKProvidersUseCase,
	getModelsByProviderUC *orgparamusecases.GetModelsByProviderUseCase,
	getModelCapabilitiesUC *orgparamusecases.GetModelCapabilitiesUseCase,
	ignoreBotsUC *orgparamusecases.IgnoreBotsUseCase,
	testByokConnUC *orgparamusecases.TestBYOKConnectionUseCase,
	testByokModelUC *orgparamusecases.TestBYOKModelUseCase,
) *OrganizationParametersController {
	c.findByKeyUC = findByKeyUC
	c.createOrUpdateUC = createOrUpdateUC
	c.getCockpitMetricsVisUC = getCockpitMetricsVisUC
	c.getLLMConfigStatusUC = getLLMConfigStatusUC
	c.listModelOverridesUC = listModelOverridesUC
	c.clearModelOverridesUC = clearModelOverridesUC
	c.deleteBYOKConfigUC = deleteBYOKConfigUC
	c.getByokProvidersUC = getByokProvidersUC
	c.getModelsByProviderUC = getModelsByProviderUC
	c.getModelCapabilitiesUC = getModelCapabilitiesUC
	c.ignoreBotsUC = ignoreBotsUC
	c.testByokConnUC = testByokConnUC
	c.testByokModelUC = testByokModelUC
	return c
}

// Routes mounts organization parameters routes.
func (c *OrganizationParametersController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/find-by-key", c.handleFindByKey)
	r.Post("/create-or-update", c.handleCreateOrUpdate)
	r.Get("/list-providers", c.handleListProviders)
	r.Get("/list-models", c.handleListModels)
	r.Post("/list-models", c.handleListModels)
	r.Get("/model-capabilities", c.handleModelCapabilities)
	r.Delete("/delete-byok-config", c.handleDeleteBYOK)
	r.Post("/test-byok", c.handleTestBYOK)
	r.Post("/test-byok-model", c.handleTestBYOK)
	r.Get("/model-overrides", c.handleModelOverrides)
	r.Post("/model-overrides/clear", c.handleClearModelOverrides)
	r.Get("/llm-config/status", c.handleLLMConfigStatus)
	r.Get("/byok/providers", c.handleBYOKProviders)
	r.Get("/cockpit-metrics-visibility", c.handleGetCockpitMetricsVisibility)
	r.Post("/cockpit-metrics-visibility", c.handleUpdateCockpitMetricsVisibility)
	r.Post("/auto-license/allowed-users", c.handleUpdateAutoLicenseAllowedUsers)
	r.Post("/update-auto-license-allowed-users", c.handleUpdateAutoLicenseAllowedUsers)

	return r
}

func (c *OrganizationParametersController) resolveWorkspaceID(r *http.Request) (uuid.UUID, error) {
	wsID, _ := auth.WorkspaceFromContext(r.Context())
	if wsID != uuid.Nil {
		return wsID, nil
	}
	if c.repo != nil {
		// Scoped to the caller. An empty email yields an empty list, never
		// every workspace in the deployment.
		email, _ := auth.CallerEmail(r.Context())
		wsList, err := c.repo.ListWorkspacesForUser(r.Context(), email)
		if err == nil && len(wsList) > 0 {
			return wsList[0].ID, nil
		}
	}
	return uuid.Nil, errors.New("workspace ID missing from request")
}

func maskSecret(s string) string {
	if len(s) <= 6 {
		return "••••"
	}
	return s[:2] + "..." + s[len(s)-3:]
}

func isMasked(s string) bool {
	return s == "••••" || strings.Contains(s, "...")
}

func (c *OrganizationParametersController) handleFindByKey(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		http.Error(w, `{"error":"key parameter is required"}`, http.StatusBadRequest)
		return
	}

	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, `{"error":"organization not found"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if c.findByKeyUC != nil && wsID != uuid.Nil {
		entity, err := c.findByKeyUC.Execute(r.Context(), wsID, orgparamdomain.ParameterKey(key), true)
		if err == nil && entity != nil {
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

	if c.repo == nil {
		// This used to invent a record: a fresh UUID and current timestamps for
		// a configuration value that was never stored. A client caching that
		// response would treat invented state as real, which is worse than an
		// error because an error is honest (AUDIT_REMEDIATION.md F-10).
		http.Error(w, `{"error":"organization parameters unavailable: no data source"}`, http.StatusServiceUnavailable)
		return
	}

	param, err := c.repo.GetOrganizationParameter(r.Context(), wsID, key)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed fetching parameter: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if param == nil {
		// Nothing is stored for this key.
		//
		// This previously answered 200 with a freshly generated uuid and the
		// current timestamps, presenting a record that was never written as
		// real persisted state (AUDIT_REMEDIATION.md F-10). A client caching
		// that would treat invented identifiers as authoritative.
		//
		// For the keys that have a genuine, documented application default the
		// default is still returned, because it is a real value the application
		// uses — but it is labelled as a default, and uuid/createdAt/updatedAt
		// are null rather than invented. For any other key there is no
		// meaningful default, so the key simply does not exist.
		var defaultVal any
		switch key {
		case models.OrgParamKeyBYOKConfig:
			defaultVal = map[string]any{
				"version":     2,
				"credentials": []any{},
				"models":      []any{},
				"routing":     map[string]any{},
			}
		case models.OrgParamKeyCockpitMetricsVisibility:
			defaultVal = map[string]any{"showMetrics": true}
		case models.OrgParamKeyAutoLicenseAssignment:
			defaultVal = map[string]any{"enabled": true}
		case models.OrgParamKeyTimezoneConfig:
			defaultVal = map[string]any{"timezone": "UTC"}
		default:
			http.Error(w,
				fmt.Sprintf(`{"error":"no organization parameter with key %q exists for this workspace","persisted":false}`, key),
				http.StatusNotFound,
			)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": http.StatusOK,
			"data": map[string]any{
				"uuid":        nil, // nothing persisted, so there is no identifier
				"configKey":   key,
				"configValue": defaultVal,
				"createdAt":   nil,
				"updatedAt":   nil,
				"persisted":   false,
				"isDefault":   true,
				"note":        "no stored value for this key; the configValue is the documented application default and is not persisted",
			},
		})
		return
	}

	var configValue any
	if len(param.ConfigValue) > 0 {
		if err := json.Unmarshal(param.ConfigValue, &configValue); err != nil {
			configValue = map[string]any{}
		}
	} else {
		configValue = map[string]any{}
	}

	// Mask BYOK credentials so plaintext secrets never leak to the client
	if key == models.OrgParamKeyBYOKConfig {
		if migrated, err := byok.MigrateLegacyToV2(param.ConfigValue); err == nil && migrated != nil {
			for i := range migrated.Credentials {
				if migrated.Credentials[i].APIKey != "" {
					migrated.Credentials[i].APIKey = maskSecret(migrated.Credentials[i].APIKey)
				}
				if migrated.Credentials[i].Settings != nil {
					for _, k := range []string{"awsSecretAccessKey", "awsSessionToken", "awsBearerToken"} {
						if val, ok := migrated.Credentials[i].Settings[k]; ok {
							if strVal, ok := val.(string); ok && strVal != "" {
								migrated.Credentials[i].Settings[k] = maskSecret(strVal)
							}
						}
					}
				}
			}
			configValue = *migrated
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"uuid":        param.ID.String(),
			"configKey":   param.ConfigKey,
			"configValue": configValue,
			"description": param.Description,
			"createdAt":   param.CreatedAt.UTC().Format(time.RFC3339),
			"updatedAt":   param.UpdatedAt.UTC().Format(time.RFC3339),
		},
	})
}

func (c *OrganizationParametersController) handleCreateOrUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key         string `json:"key"`
		ConfigValue any    `json:"configValue"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid parameters payload"}`, http.StatusBadRequest)
		return
	}

	key := strings.TrimSpace(req.Key)
	if key == "" {
		http.Error(w, `{"error":"key is required"}`, http.StatusBadRequest)
		return
	}

	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, `{"error":"organization not found"}`, http.StatusBadRequest)
		return
	}

	valBytes, err := json.Marshal(req.ConfigValue)
	if err != nil {
		http.Error(w, `{"error":"failed encoding configValue"}`, http.StatusBadRequest)
		return
	}

	// Referential integrity validation and secret encryption for BYOK
	if key == models.OrgParamKeyBYOKConfig {
		var cfg byok.BYOKConfig
		if err := json.Unmarshal(valBytes, &cfg); err != nil {
			http.Error(w, `{"error":"invalid BYOK config JSON"}`, http.StatusBadRequest)
			return
		}

		refRes := validation.ValidateByokConfigRefs(&cfg)
		if !refRes.Valid {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"statusCode": http.StatusBadRequest,
				"error":      "Invalid BYOK configuration: unresolved model/routing references",
				"details":    refRes.Errors,
			})
			return
		}

		// Load existing credentials to retain unchanged masked secrets
		var existingCreds = make(map[string]byok.BYOKCredential)
		if c.repo != nil {
			if existingParam, _ := c.repo.GetOrganizationParameter(r.Context(), wsID, key); existingParam != nil {
				var existingCfg byok.BYOKConfig
				if json.Unmarshal(existingParam.ConfigValue, &existingCfg) == nil {
					for _, cr := range existingCfg.Credentials {
						existingCreds[cr.ID] = cr
					}
				}
			}
		}

		for i := range cfg.Credentials {
			cred := &cfg.Credentials[i]
			old, hadOld := existingCreds[cred.ID]

			if isMasked(cred.APIKey) && hadOld {
				cred.APIKey = old.APIKey
			} else if cred.APIKey != "" && !isMasked(cred.APIKey) {
				enc, encErr := byok.EncryptKey(cred.APIKey)
				if encErr == nil {
					cred.APIKey = enc
				}
			}

			if cred.Settings != nil {
				for _, k := range []string{"awsSecretAccessKey", "awsSessionToken", "awsBearerToken"} {
					if v, ok := cred.Settings[k]; ok {
						if strV, ok := v.(string); ok {
							if isMasked(strV) && hadOld && old.Settings != nil {
								if oldV, ok := old.Settings[k]; ok {
									cred.Settings[k] = oldV
								}
							} else if strV != "" && !isMasked(strV) {
								if enc, err := byok.EncryptKey(strV); err == nil {
									cred.Settings[k] = enc
								}
							}
						}
					}
				}
			}
		}

		valBytes, _ = json.Marshal(cfg)
	}

	if c.repo != nil {
		if err := c.repo.SetOrganizationParameter(r.Context(), wsID, key, valBytes, req.Description); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed saving parameter: %s"}`, err.Error()), http.StatusInternalServerError)
			return
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

func (c *OrganizationParametersController) handleDeleteBYOK(w http.ResponseWriter, r *http.Request) {
	modelID := strings.TrimSpace(r.URL.Query().Get("modelId"))
	if modelID == "" {
		var req struct {
			ModelID string `json:"modelId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		modelID = strings.TrimSpace(req.ModelID)
	}

	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, `{"error":"organization not found"}`, http.StatusBadRequest)
		return
	}

	if c.repo == nil {
		// This reported {"success":true} without deleting anything, so an
		// operator could believe a BYOK API credential was revoked while it
		// remained live in the database (AUDIT_REMEDIATION.md F-10).
		http.Error(w, `{"error":"cannot delete: no data source"}`, http.StatusServiceUnavailable)
		return
	}

	param, err := c.repo.GetOrganizationParameter(r.Context(), wsID, models.OrgParamKeyBYOKConfig)
	if err != nil || param == nil {
		http.Error(w, `{"error":"BYOK configuration not found"}`, http.StatusBadRequest)
		return
	}

	var cfg byok.BYOKConfig
	migrated, err := byok.MigrateLegacyToV2(param.ConfigValue)
	if err != nil || migrated == nil {
		http.Error(w, `{"error":"corrupted BYOK configuration"}`, http.StatusInternalServerError)
		return
	}
	cfg = *migrated

	if modelID != "" {
		found := false
		for _, m := range cfg.Models {
			if m.ID == modelID {
				found = true
				break
			}
		}
		if !found {
			http.Error(w, fmt.Sprintf(`{"error":"model %s not found in BYOK config"}`, modelID), http.StatusBadRequest)
			return
		}

		remainingCount := 0
		for _, m := range cfg.Models {
			if m.ID != modelID {
				remainingCount++
			}
		}

		// Multi-model referential integrity guard
		if remainingCount > 0 {
			refs := validation.FindModelReferences(&cfg, modelID)
			if len(refs) > 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"statusCode": http.StatusBadRequest,
					"error":      fmt.Sprintf("Cannot delete model %s because it is referenced in routing: %v", modelID, refs),
				})
				return
			}

			var updatedModels []byok.BYOKModelConfig
			for _, m := range cfg.Models {
				if m.ID != modelID {
					updatedModels = append(updatedModels, m)
				}
			}
			cfg.Models = updatedModels
			valBytes, _ := json.Marshal(cfg)
			_ = c.repo.SetOrganizationParameter(r.Context(), wsID, models.OrgParamKeyBYOKConfig, valBytes, param.Description)

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"statusCode": http.StatusOK,
				"data":       map[string]any{"success": true},
			})
			return
		}
	}

	// Last model or full teardown: delete entire BYOK config
	_ = c.repo.DeleteOrganizationParameter(r.Context(), wsID, models.OrgParamKeyBYOKConfig)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"success": true,
		},
	})
}

func (c *OrganizationParametersController) handleTestBYOK(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider        string   `json:"provider"`
		APIKey          string   `json:"apiKey"`
		BaseURL         string   `json:"baseURL"`
		Model           string   `json:"model"`
		Temperature     *float64 `json:"temperature"`
		ReasoningEffort string   `json:"reasoningEffort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid test-byok payload"}`, http.StatusBadRequest)
		return
	}

	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		http.Error(w, `{"error":"provider is required"}`, http.StatusBadRequest)
		return
	}

	if !kernel.DefaultRegistry.Has(provider) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": http.StatusBadRequest,
			"error":      fmt.Sprintf("unknown provider %s", provider),
		})
		return
	}

	issues := validation.ValidateModelTuning(validation.ModelTuningInput{
		Provider:        provider,
		Model:           req.Model,
		Temperature:     req.Temperature,
		ReasoningEffort: req.ReasoningEffort,
	})
	if len(issues) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": http.StatusBadRequest,
			"error":      "Invalid model tuning parameters",
			"issues":     issues,
		})
		return
	}

	// Dispatch a live probe. The previous response reported success with a fixed
	// 85 ms latency without ever using the API key, so a user testing a broken
	// credential was told it worked. The use case performs a real authenticated
	// request, measures the real latency, classifies provider errors, and
	// rejects unsafe base URLs.
	if c.testByokModelUC == nil {
		writeByokTestError(w, http.StatusServiceUnavailable,
			"BYOK verification is not available on this deployment")
		return
	}

	result := c.testByokModelUC.Execute(r.Context(), orgparamusecases.ByokTestInput{
		Provider:        provider,
		APIKey:          req.APIKey,
		BaseURL:         req.BaseURL,
		ModelID:         req.Model,
		Temperature:     req.Temperature,
		ReasoningEffort: req.ReasoningEffort,
	})

	status := http.StatusOK
	if !result.Success {
		// A refused credential is a client-visible outcome, not a server fault.
		switch result.Code {
		case "auth", "not_found", "bad_request", "rate_limit":
			status = http.StatusBadRequest
		default:
			status = http.StatusBadGateway
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": status,
		"data": map[string]any{
			"ok":              result.Success,
			"code":            result.Code,
			"latencyMs":       result.LatencyMs,
			"message":         result.Message,
			"httpStatus":      result.HTTPStatus,
			"providerMessage": result.ProviderMessage,
		},
	})
}

func writeByokTestError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": code,
		"error":      message,
	})
}

func (c *OrganizationParametersController) handleListProviders(w http.ResponseWriter, r *http.Request) {
	descriptors := providers.DescribeAllProviderIDs(kernel.DefaultRegistry.List())

	type ProviderDTO struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Label           string `json:"label"`
		Description     string `json:"description"`
		RequiresApiKey  bool   `json:"requiresApiKey"`
		RequiresBaseUrl bool   `json:"requiresBaseUrl"`
		AutoListModels  bool   `json:"autoListModels"`
		ListsModelsLive bool   `json:"listsModelsLive"`
		Doc             string `json:"doc"`
	}

	res := make([]ProviderDTO, 0, len(descriptors))
	for _, d := range descriptors {
		res = append(res, ProviderDTO{
			ID:              d.ID,
			Name:            d.Label,
			Label:           d.Label,
			Description:     d.Doc,
			RequiresApiKey:  d.RequiresAPIKey,
			RequiresBaseUrl: d.RequiresBaseURL,
			AutoListModels:  d.AutoListModels,
			ListsModelsLive: d.ListsModelsLive,
			Doc:             d.Doc,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data":       res,
	})
}

func (c *OrganizationParametersController) handleListModels(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	if provider == "" && r.Method == http.MethodPost {
		var body struct {
			Provider string `json:"provider"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		provider = strings.TrimSpace(body.Provider)
	}

	type ModelDTO struct {
		ID                string         `json:"id"`
		Name              string         `json:"name"`
		Provider          string         `json:"provider"`
		Description       string         `json:"description,omitempty"`
		Recommended       bool           `json:"recommended,omitempty"`
		SupportsReasoning bool           `json:"supportsReasoning,omitempty"`
		ReasoningConfig   map[string]any `json:"reasoningConfig,omitempty"`
	}

	var modelsList []ModelDTO

	if provider != "" {
		m, ok := kernel.Get(provider)
		if !ok {
			http.Error(w, fmt.Sprintf(`{"error":"unknown provider %s"}`, provider), http.StatusNotFound)
			return
		}

		listing := m.ModelListing(provider)
		if listing != nil {
			for _, cm := range listing.StaticModels {
				modelsList = append(modelsList, ModelDTO{
					ID:                cm.ID,
					Name:              cm.Name,
					Provider:          provider,
					SupportsReasoning: cm.SupportsReasoning,
					ReasoningConfig:   cm.ReasoningConfig,
				})
			}
			if len(modelsList) == 0 {
				for _, cm := range listing.FallbackModels {
					modelsList = append(modelsList, ModelDTO{
						ID:                cm.ID,
						Name:              cm.Name,
						Provider:          provider,
						SupportsReasoning: cm.SupportsReasoning,
						ReasoningConfig:   cm.ReasoningConfig,
					})
				}
			}
		}
	} else {
		for _, m := range kernel.DefaultRegistry.List() {
			id := m.ID()
			listing := m.ModelListing(id)
			if listing != nil {
				for _, cm := range listing.StaticModels {
					modelsList = append(modelsList, ModelDTO{
						ID:                cm.ID,
						Name:              cm.Name,
						Provider:          id,
						SupportsReasoning: cm.SupportsReasoning,
						ReasoningConfig:   cm.ReasoningConfig,
					})
				}
				for _, cm := range listing.FallbackModels {
					modelsList = append(modelsList, ModelDTO{
						ID:                cm.ID,
						Name:              cm.Name,
						Provider:          id,
						SupportsReasoning: cm.SupportsReasoning,
						ReasoningConfig:   cm.ReasoningConfig,
					})
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data":       modelsList,
	})
}

func (c *OrganizationParametersController) handleModelCapabilities(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	model := strings.TrimSpace(r.URL.Query().Get("model"))

	if provider == "" {
		http.Error(w, `{"error":"provider parameter is required"}`, http.StatusBadRequest)
		return
	}

	m, ok := kernel.Get(provider)
	if !ok {
		http.Error(w, fmt.Sprintf(`{"error":"provider %s not found"}`, provider), http.StatusNotFound)
		return
	}

	caps := m.Capabilities(model)
	traits := m.ReasoningTraits(byok.NormalizedModel{
		Provider: byok.BYOKProvider(provider),
		Model:    model,
	})
	policy := m.TemperaturePolicy(byok.NormalizedModel{
		Provider: byok.BYOKProvider(provider),
		Model:    model,
	})

	supportsTemp := true
	if policy != nil && policy.Mode == kernel.TemperatureUnsupported {
		supportsTemp = false
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"supportsThinking":       caps.SupportsReasoning || traits.ThinksByDefault,
			"supportsStreaming":      caps.SupportsStreaming,
			"supportsTools":          caps.ToolCalling == "native",
			"supportsJsonSchema":     caps.StructuredOutput == "json_schema",
			"supportsVision":         true,
			"supportsTemperature":    supportsTemp,
			"maxContextWindowTokens": caps.MaxInputTokens,
		},
	})
}

func (c *OrganizationParametersController) handleModelOverrides(w http.ResponseWriter, r *http.Request) {
	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, `{"error":"organization not found"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if c.repo == nil {
		http.Error(w, `{"error":"model overrides unavailable: no data source"}`, http.StatusServiceUnavailable)
		return
	}

	param, err := c.repo.GetOrganizationParameter(r.Context(), wsID, models.OrgParamKeyModelOverrides)
	if err != nil {
		// A query failure used to be reported as "no overrides configured",
		// which is indistinguishable from a real empty result and hides the
		// outage (AUDIT_REMEDIATION.md F-43).
		slog.Error("orgparams.model_overrides_failed", "workspace_id", wsID, "error", err)
		http.Error(w, `{"error":"model overrides unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	if param == nil {
		// Genuinely absent: an empty list is the truthful answer here.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": http.StatusOK,
			"data":       map[string]any{"overrides": []any{}},
		})
		return
	}

	var overrides any
	if err := json.Unmarshal(param.ConfigValue, &overrides); err != nil {
		// The stored value is not the expected shape. Returning an empty list
		// here would silently discard real configuration.
		slog.Error("orgparams.model_overrides_malformed", "workspace_id", wsID, "error", err)
		http.Error(w, `{"error":"stored model overrides are malformed"}`, http.StatusInternalServerError)
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"overrides": overrides,
		},
	})
}

func (c *OrganizationParametersController) handleClearModelOverrides(w http.ResponseWriter, r *http.Request) {
	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, `{"error":"organization not found"}`, http.StatusBadRequest)
		return
	}

	if c.repo != nil {
		_ = c.repo.DeleteOrganizationParameter(r.Context(), wsID, models.OrgParamKeyModelOverrides)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data":       map[string]any{"success": true},
	})
}

func (c *OrganizationParametersController) handleLLMConfigStatus(w http.ResponseWriter, r *http.Request) {
	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, `{"error":"organization not found"}`, http.StatusBadRequest)
		return
	}

	var cfg byok.BYOKConfig
	if c.repo != nil {
		param, _ := c.repo.GetOrganizationParameter(r.Context(), wsID, models.OrgParamKeyBYOKConfig)
		if param != nil && len(param.ConfigValue) > 0 {
			if migrated, err := byok.MigrateLegacyToV2(param.ConfigValue); err == nil && migrated != nil {
				cfg = *migrated
			}
		}
	}

	status := llm.DescribeLLMConfigStatus(&cfg)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data":       status,
	})
}

func (c *OrganizationParametersController) handleBYOKProviders(w http.ResponseWriter, r *http.Request) {
	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, `{"error":"organization not found"}`, http.StatusBadRequest)
		return
	}

	var configuredProviders []string
	if c.repo != nil {
		param, _ := c.repo.GetOrganizationParameter(r.Context(), wsID, models.OrgParamKeyBYOKConfig)
		if param != nil && len(param.ConfigValue) > 0 {
			if migrated, err := byok.MigrateLegacyToV2(param.ConfigValue); err == nil && migrated != nil {
				seen := make(map[string]bool)
				for _, cr := range migrated.Credentials {
					if !seen[cr.Provider] {
						seen[cr.Provider] = true
						configuredProviders = append(configuredProviders, cr.Provider)
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data":       configuredProviders,
	})
}

func (c *OrganizationParametersController) handleGetCockpitMetricsVisibility(w http.ResponseWriter, r *http.Request) {
	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, `{"error":"organization not found"}`, http.StatusBadRequest)
		return
	}

	showMetrics := true
	if c.repo != nil {
		param, _ := c.repo.GetOrganizationParameter(r.Context(), wsID, models.OrgParamKeyCockpitMetricsVisibility)
		if param != nil && len(param.ConfigValue) > 0 {
			var val map[string]any
			if json.Unmarshal(param.ConfigValue, &val) == nil {
				if sm, ok := val["showMetrics"].(bool); ok {
					showMetrics = sm
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data": map[string]any{
			"showMetrics": showMetrics,
		},
	})
}

func (c *OrganizationParametersController) handleUpdateCockpitMetricsVisibility(w http.ResponseWriter, r *http.Request) {
	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, `{"error":"organization not found"}`, http.StatusBadRequest)
		return
	}

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	if c.repo != nil {
		valBytes, _ := json.Marshal(req)
		_ = c.repo.SetOrganizationParameter(r.Context(), wsID, models.OrgParamKeyCockpitMetricsVisibility, valBytes, "Cockpit metrics visibility")
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data":       map[string]any{"success": true},
	})
}

func (c *OrganizationParametersController) handleUpdateAutoLicenseAllowedUsers(w http.ResponseWriter, r *http.Request) {
	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, `{"error":"organization not found"}`, http.StatusBadRequest)
		return
	}

	var req any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	if c.repo != nil {
		valBytes, _ := json.Marshal(req)
		_ = c.repo.SetOrganizationParameter(r.Context(), wsID, models.OrgParamKeyAutoLicenseAllowedUsers, valBytes, "Auto-license allowed users")
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": http.StatusOK,
		"data":       map[string]any{"success": true},
	})
}

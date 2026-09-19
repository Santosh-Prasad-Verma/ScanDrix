// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: cli_drixy_rules_controller.go
// ═══════════════════════════════════════════════════════════════

package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/rules/drixy/application/usecases"
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
	"github.com/scandrix/backend/pkg/models"
)

// CapabilityDrixyRulesManage defines the team CLI key permission for rules management.
const CapabilityDrixyRulesManage = "drixy_rules_manage"

// TeamCLIKeyValidator abstracts lookup and validation of team CLI keys.
type TeamCLIKeyValidator interface {
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*models.TeamCLIKey, error)
}

// CliDrixyRulesAuthContext captures the resolved organization, team, and identity context.
type CliDrixyRulesAuthContext struct {
	OrganizationID string
	TeamID         string
	Capabilities   []string
	KeyID          string
	KeyName        string
	UserID         string
	UserEmail      string
}

// CliDrixyRulesController implements CLI rules management endpoints.
type CliDrixyRulesController struct {
	keyValidator             TeamCLIKeyValidator
	createOrUpdateUseCase    *usecases.CreateOrUpdateDrixyRuleUseCase
	findRulesByFilterUseCase *usecases.FindRulesInOrganizationByFilterDrixyRulesUseCase
}

// NewCliDrixyRulesController constructs the CLI rules controller.
func NewCliDrixyRulesController(
	validator TeamCLIKeyValidator,
	createOrUpdateUC *usecases.CreateOrUpdateDrixyRuleUseCase,
	findRulesUC *usecases.FindRulesInOrganizationByFilterDrixyRulesUseCase,
) *CliDrixyRulesController {
	return &CliDrixyRulesController{
		keyValidator:             validator,
		createOrUpdateUseCase:    createOrUpdateUC,
		findRulesByFilterUseCase: findRulesUC,
	}
}

// Routes mounts CLI rules endpoints.
func (c *CliDrixyRulesController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", c.handleListRules)
	r.Post("/", c.handleCreateRule)
	r.Patch("/{ruleId}", c.handleUpdateRule)

	return r
}

func (c *CliDrixyRulesController) resolveCliContext(r *http.Request) (*CliDrixyRulesAuthContext, error) {
	teamKey := strings.TrimSpace(r.Header.Get("x-team-key"))
	if teamKey == "" {
		teamKey = strings.TrimSpace(r.Header.Get("X-Team-Key"))
	}

	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		authHeader = strings.TrimSpace(r.Header.Get("authorization"))
	}

	bearerToken := ""
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		bearerToken = strings.TrimSpace(authHeader[7:])
	}

	resolvedKey := teamKey
	if resolvedKey == "" {
		resolvedKey = bearerToken
	}

	// 1. Authenticate via Team CLI Key (X-Team-Key or Bearer token)
	if resolvedKey != "" && c.keyValidator != nil {
		hasher := sha256.New()
		hasher.Write([]byte(resolvedKey))
		hashStr := hex.EncodeToString(hasher.Sum(nil))

		key, err := c.keyValidator.GetAPIKeyByHash(r.Context(), hashStr)
		if err == nil && key != nil {
			if !key.Active {
				return nil, fmt.Errorf("revoked team API key")
			}
			if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
				return nil, fmt.Errorf("expired team API key")
			}

			orgID := ""
			if key.WorkspaceID != nil {
				orgID = key.WorkspaceID.String()
			}
			teamID := ""
			if key.TeamID != nil {
				teamID = key.TeamID.String()
			}

			var capabilities []string
			if len(key.Config) > 0 {
				var cfg struct {
					Capabilities []string `json:"capabilities"`
				}
				if err := json.Unmarshal(key.Config, &cfg); err == nil {
					capabilities = cfg.Capabilities
				}
			}

			keyID := key.ID.String()
			keyName := strings.TrimSpace(key.Name)
			if keyName == "" {
				keyName = key.KeyPrefix
			}

			return &CliDrixyRulesAuthContext{
				OrganizationID: orgID,
				TeamID:         teamID,
				Capabilities:   capabilities,
				KeyID:          keyID,
				KeyName:        keyName,
			}, nil
		}
	}

	// 2. Fallback: Authenticate via context (logged-in web session or JWT middleware)
	if profile, ok := auth.AccountProfileFromContext(r.Context()); ok && profile != nil {
		return &CliDrixyRulesAuthContext{
			OrganizationID: profile.WorkspaceID.String(),
			TeamID:         "",
			Capabilities:   []string{CapabilityDrixyRulesManage},
			UserID:         profile.ID.String(),
			UserEmail:      profile.Email,
		}, nil
	}
	if wsID, err := auth.WorkspaceFromContext(r.Context()); err == nil && wsID != uuid.Nil {
		return &CliDrixyRulesAuthContext{
			OrganizationID: wsID.String(),
			TeamID:         "",
			Capabilities:   []string{CapabilityDrixyRulesManage},
			UserID:         wsID.String(),
		}, nil
	}

	return nil, fmt.Errorf("team API key required")
}

func (c *CliDrixyRulesController) ensureDrixyRulesCapability(authCtx *CliDrixyRulesAuthContext) error {
	if authCtx == nil {
		return fmt.Errorf("unauthorized")
	}
	// If no explicit capabilities are stored on the key, default to authorized for backwards compatibility
	if len(authCtx.Capabilities) == 0 {
		return nil
	}
	for _, cap := range authCtx.Capabilities {
		if strings.EqualFold(cap, CapabilityDrixyRulesManage) ||
			strings.EqualFold(cap, "rules_manage") ||
			strings.EqualFold(cap, "all") ||
			strings.EqualFold(cap, "admin") {
			return nil
		}
	}
	return fmt.Errorf("team API key does not have permission to manage rules")
}

func (c *CliDrixyRulesController) handleListRules(w http.ResponseWriter, r *http.Request) {
	authCtx, err := c.resolveCliContext(r)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusUnauthorized)
		return
	}
	if err := c.ensureDrixyRulesCapability(authCtx); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusForbidden)
		return
	}

	ruleID := r.URL.Query().Get("ruleId")
	repoID := r.URL.Query().Get("repositoryId")

	filter := map[string]any{}
	if ruleID != "" {
		filter["uuid"] = ruleID
	} else if repoID != "" {
		filter["repositoryId"] = repoID
	}

	rules, err := c.findRulesByFilterUseCase.Execute(r.Context(), authCtx.OrganizationID, filter, repoID, "")
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rules)
}

func (c *CliDrixyRulesController) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	authCtx, err := c.resolveCliContext(r)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusUnauthorized)
		return
	}
	if err := c.ensureDrixyRulesCapability(authCtx); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusForbidden)
		return
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	if _, exists := body["uuid"]; exists && body["uuid"] != nil && body["uuid"] != "" {
		http.Error(w, `{"error":"UUID should not be provided when creating a new rule"}`, http.StatusForbidden)
		return
	}

	title, _ := body["title"].(string)
	ruleContent, _ := body["rule"].(string)
	repoID, _ := body["repositoryId"].(string)
	if repoID == "" {
		repoID, _ = body["repo_id"].(string)
	}

	if title == "" || ruleContent == "" || repoID == "" {
		http.Error(w, `{"error":"Missing required fields: title, rule, repositoryId"}`, http.StatusForbidden)
		return
	}

	status := interfaces.DrixyRulesStatusActive
	if s, ok := body["status"].(string); ok && s != "" {
		status = interfaces.DrixyRulesStatus(s)
	}

	ruleType := interfaces.DrixyRulesTypeStandard
	if t, ok := body["type"].(string); ok && t != "" {
		ruleType = interfaces.DrixyRulesType(t)
	}

	path := "*/**"
	if p, ok := body["path"].(string); ok && p != "" {
		path = p
	}

	scope := interfaces.DrixyRulesScopeFile
	if sc, ok := body["scope"].(string); ok && sc != "" {
		scope = interfaces.DrixyRulesScope(sc)
	}

	severity := "MEDIUM"
	if sev, ok := body["severity"].(string); ok && sev != "" {
		severity = sev
	}

	dto := dtos.CreateDrixyRuleDto{
		Title:        title,
		Rule:         ruleContent,
		Status:       status,
		Type:         ruleType,
		Path:         path,
		Origin:       interfaces.DrixyRulesOriginCLI,
		Scope:        scope,
		Severity:     severity,
		RepositoryID: repoID,
		TeamID:       authCtx.TeamID,
	}

	var userInfo *contracts.UserAuditInfo
	if authCtx.UserID != "" {
		userInfo = &contracts.UserAuditInfo{
			UserID:    authCtx.UserID,
			UserEmail: authCtx.UserEmail,
		}
	} else if authCtx.KeyID != "" {
		userInfo = &contracts.UserAuditInfo{
			UserID:    authCtx.KeyID,
			UserEmail: authCtx.KeyName,
		}
	}

	created, err := c.createOrUpdateUseCase.Execute(r.Context(), dto, authCtx.OrganizationID, userInfo)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

func (c *CliDrixyRulesController) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
	authCtx, err := c.resolveCliContext(r)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusUnauthorized)
		return
	}
	if err := c.ensureDrixyRulesCapability(authCtx); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusForbidden)
		return
	}

	ruleID := chi.URLParam(r, "ruleId")
	if ruleID == "" {
		http.Error(w, `{"error":"Rule ID is required for update"}`, http.StatusForbidden)
		return
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	if bodyUUID, exists := body["uuid"].(string); exists && bodyUUID != "" && bodyUUID != ruleID {
		http.Error(w, `{"error":"Body UUID must match the ruleId path parameter"}`, http.StatusForbidden)
		return
	}

	// Validate required non-null fields if present in update
	checkFields := []string{"title", "rule", "repositoryId", "severity", "scope", "path"}
	for _, f := range checkFields {
		if val, exists := body[f]; exists && val == nil {
			http.Error(w, fmt.Sprintf(`{"error":"Field '%s' cannot be set to null or undefined."}`, f), http.StatusForbidden)
			return
		}
	}

	title, _ := body["title"].(string)
	ruleContent, _ := body["rule"].(string)
	repoID, _ := body["repositoryId"].(string)
	severity, _ := body["severity"].(string)
	path, _ := body["path"].(string)
	scopeStr, _ := body["scope"].(string)

	dto := dtos.CreateDrixyRuleDto{
		UUID:         ruleID,
		Title:        title,
		Rule:         ruleContent,
		RepositoryID: repoID,
		Severity:     severity,
		Path:         path,
		Scope:        interfaces.DrixyRulesScope(scopeStr),
		TeamID:       authCtx.TeamID,
	}

	var userInfo *contracts.UserAuditInfo
	if authCtx.UserID != "" {
		userInfo = &contracts.UserAuditInfo{
			UserID:    authCtx.UserID,
			UserEmail: authCtx.UserEmail,
		}
	} else if authCtx.KeyID != "" {
		userInfo = &contracts.UserAuditInfo{
			UserID:    authCtx.KeyID,
			UserEmail: authCtx.KeyName,
		}
	}

	updated, err := c.createOrUpdateUseCase.Execute(r.Context(), dto, authCtx.OrganizationID, userInfo)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

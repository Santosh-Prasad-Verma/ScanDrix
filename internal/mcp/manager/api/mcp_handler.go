// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/mcp/manager/models"
	"github.com/scandrix/backend/internal/mcp/manager/service"
)

// MCPHandler handles HTTP REST endpoints for the Model Context Protocol manager.
type MCPHandler struct {
	mcpService          *service.MCPService
	integrationsService *service.IntegrationsService
}

// NewMCPHandler creates an MCP REST controller.
func NewMCPHandler(mcpService *service.MCPService, integrationsService *service.IntegrationsService) *MCPHandler {
	return &MCPHandler{
		mcpService:          mcpService,
		integrationsService: integrationsService,
	}
}

// ═══════════════════════════════════════════════════════════════
// 1. CONNECTION ENDPOINTS
// ═══════════════════════════════════════════════════════════════

func (h *MCPHandler) GetConnections(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))

	query := models.QueryDTO{
		Page:          page,
		PageSize:      pageSize,
		Provider:      r.URL.Query().Get("provider"),
		AppName:       r.URL.Query().Get("appName"),
		IntegrationID: r.URL.Query().Get("integrationId"),
		Status:        r.URL.Query().Get("status"),
	}

	res, err := h.mcpService.GetConnections(r.Context(), query, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *MCPHandler) GetConnection(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	connectionID := chi.URLParam(r, "connectionId")

	conn, err := h.mcpService.GetConnection(r.Context(), connectionID, orgID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, conn)
}

func (h *MCPHandler) UpdateConnection(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	var dto models.UpdateConnectionDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	conn, err := h.mcpService.UpdateConnection(r.Context(), dto, orgID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, conn)
}

func (h *MCPHandler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	ref := chi.URLParam(r, "connectionId")

	if err := h.mcpService.DeleteConnection(r.Context(), ref, orgID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models.MessageResponseDTO{Message: "Connection deleted successfully"})
}

func (h *MCPHandler) UpdateAllowedTools(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	integrationID := chi.URLParam(r, "integrationId")

	var dto models.UpdateAllowedToolsDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	conn, err := h.mcpService.UpdateAllowedTools(r.Context(), integrationID, dto.AllowedTools, orgID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, models.MessageResponseDTO{
		Message: "Allowed tools updated successfully",
		Connection: map[string]any{
			"id":           conn.ID,
			"integrationId": conn.IntegrationID,
			"allowedTools": conn.AllowedTools,
		},
	})
}

// ═══════════════════════════════════════════════════════════════
// 2. INTEGRATIONS CATALOG ENDPOINTS
// ═══════════════════════════════════════════════════════════════

func (h *MCPHandler) GetIntegrations(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))

	query := models.QueryDTO{
		Page:     page,
		PageSize: pageSize,
		AppName:  r.URL.Query().Get("appName"),
	}

	list, err := h.mcpService.GetIntegrations(r.Context(), query, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *MCPHandler) GetIntegration(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	provider := chi.URLParam(r, "provider")
	integrationID := chi.URLParam(r, "integrationId")

	item, err := h.mcpService.GetIntegration(r.Context(), integrationID, provider, orgID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *MCPHandler) GetIntegrationRequiredParams(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	integrationID := chi.URLParam(r, "integrationId")

	params, err := h.mcpService.GetIntegrationRequiredParams(r.Context(), integrationID, provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, params)
}

func (h *MCPHandler) GetIntegrationTools(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	provider := chi.URLParam(r, "provider")
	integrationID := chi.URLParam(r, "integrationId")

	tools, err := h.mcpService.GetIntegrationTools(r.Context(), integrationID, provider, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tools)
}

func (h *MCPHandler) InitiateConnection(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	provider := chi.URLParam(r, "provider")

	var dto models.InitiateConnectionDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	conn, err := h.mcpService.InitiateConnection(r.Context(), orgID, provider, dto)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, conn)
}

// ═══════════════════════════════════════════════════════════════
// 3. BRING-YOUR-OWN-TOKEN (BYOT) & MANAGED AUTH
// ═══════════════════════════════════════════════════════════════

func (h *MCPHandler) ConnectManagedToken(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	integrationID := chi.URLParam(r, "integrationId")

	var dto models.ConnectTokenDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	conn, err := h.mcpService.ConnectManagedToken(r.Context(), orgID, integrationID, dto)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, conn)
}

func (h *MCPHandler) GetManagedConnectionConfig(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	integrationID := chi.URLParam(r, "integrationId")

	headers, err := h.mcpService.GetScandrixMCPConnectionConfig(r.Context(), orgID, integrationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"headers": headers})
}

// ═══════════════════════════════════════════════════════════════
// 4. CUSTOM INTEGRATIONS CRUD
// ═══════════════════════════════════════════════════════════════

func (h *MCPHandler) GetCustomIntegrations(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	active := true
	if r.URL.Query().Get("active") == "false" {
		active = false
	}

	integrations, err := h.mcpService.GetIntegrations(r.Context(), models.QueryDTO{Page: 1, PageSize: 100}, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	customOnly := make([]models.MCPIntegration, 0)
	for _, it := range integrations {
		if it.Provider == models.ProviderCustom && (!active || it.Active) {
			customOnly = append(customOnly, it)
		}
	}
	writeJSON(w, http.StatusOK, customOnly)
}

func (h *MCPHandler) GetCustomIntegration(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	integrationID := chi.URLParam(r, "integrationId")

	item, err := h.mcpService.GetIntegration(r.Context(), integrationID, string(models.ProviderCustom), orgID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *MCPHandler) GetCustomIntegrationConnectionConfig(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	integrationID := chi.URLParam(r, "integrationId")

	cfg, err := h.mcpService.GetCustomIntegrationConnectionConfig(r.Context(), orgID, integrationID)
	if err != nil || cfg == nil {
		writeError(w, http.StatusNotFound, "Connection configuration not found")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *MCPHandler) CreateIntegration(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	provider := chi.URLParam(r, "provider")

	var dto models.CreateIntegrationDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if provider == string(models.ProviderCustom) {
		res, err := h.integrationsService.CreateCustomIntegration(r.Context(), orgID, dto)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
		return
	}

	// Managed provider fallback
	conn, err := h.mcpService.InitiateConnection(r.Context(), orgID, provider, models.InitiateConnectionDTO{
		IntegrationID: dto.IntegrationID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models.MessageResponseDTO{
		Message: "Integration created successfully",
		Connection: map[string]any{
			"id":           conn.ID,
			"integrationId": conn.IntegrationID,
			"provider":     conn.Provider,
			"status":       conn.Status,
		},
	})
}

func (h *MCPHandler) EditIntegration(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	provider := chi.URLParam(r, "provider")
	integrationID := chi.URLParam(r, "integrationId")

	if provider != string(models.ProviderCustom) {
		writeError(w, http.StatusBadRequest, "Editing integrations is only supported for custom provider")
		return
	}

	var dto models.CreateIntegrationDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	res, err := h.integrationsService.EditCustomIntegration(r.Context(), orgID, integrationID, dto)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *MCPHandler) DeleteIntegration(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	provider := chi.URLParam(r, "provider")
	integrationID := chi.URLParam(r, "integrationId")

	if provider != string(models.ProviderCustom) {
		writeError(w, http.StatusBadRequest, "Deleting integrations is only supported for custom provider")
		return
	}

	if err := h.integrationsService.DeleteCustomIntegration(r.Context(), orgID, integrationID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models.MessageResponseDTO{Message: "Integration deleted successfully"})
}

// ═══════════════════════════════════════════════════════════════
// 5. OAUTH FLOWS
// ═══════════════════════════════════════════════════════════════

func (h *MCPHandler) InitializeOAuthIntegration(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	provider := chi.URLParam(r, "provider")

	var dto models.InitiateOAuthDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	authURL, err := h.mcpService.InitiateOAuthIntegration(r.Context(), orgID, provider, dto)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models.OAuthInitResponseDTO{AuthURL: authURL})
}

func (h *MCPHandler) FinalizeOAuthIntegration(w http.ResponseWriter, r *http.Request) {
	orgID := GetOrgIDFromContext(r.Context())
	provider := chi.URLParam(r, "provider")

	var dto models.FinishOAuthDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.mcpService.FinalizeOAuthIntegration(r.Context(), orgID, provider, dto); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models.MessageResponseDTO{Message: "OAuth integration finalized"})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

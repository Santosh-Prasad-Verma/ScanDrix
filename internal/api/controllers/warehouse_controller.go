package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/analytics/warehouse"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
)

// WarehouseController exposes HTTP REST endpoints for enterprise BI warehouse synchronization.
type WarehouseController struct {
	syncer *warehouse.WarehouseSyncer
}

// NewWarehouseController initializes the warehouse HTTP controller.
func NewWarehouseController(repo *database.Repository) *WarehouseController {
	return &WarehouseController{
		syncer: warehouse.NewWarehouseSyncer(repo),
	}
}

// Routes mounts the controller endpoints.
func (c *WarehouseController) Routes() chi.Router {
	r := chi.NewRouter()
	c.RegisterRoutes(r)
	return r
}

// RegisterRoutes registers the individual warehouse endpoints onto a router.
func (c *WarehouseController) RegisterRoutes(r chi.Router) {
	r.Get("/config", c.HandleGetConfig)
	r.Get("/{workspaceId}/config", c.HandleGetConfig)

	r.Put("/config", c.HandleSaveConfig)
	r.Put("/{workspaceId}/config", c.HandleSaveConfig)

	r.Post("/test", c.HandleTestConnection)
	r.Post("/{workspaceId}/test", c.HandleTestConnection)

	r.Post("/sync", c.HandleTriggerSync)
	r.Post("/{workspaceId}/sync", c.HandleTriggerSync)

	r.Get("/history", c.HandleGetHistory)
	r.Get("/{workspaceId}/history", c.HandleGetHistory)

	r.Get("/schema-ddl", c.HandleGetSchemaDDL)
	r.Get("/{workspaceId}/schema-ddl", c.HandleGetSchemaDDL)

	r.Get("/preview-payload", c.HandlePreviewPayload)
	r.Get("/{workspaceId}/preview-payload", c.HandlePreviewPayload)
}

func (c *WarehouseController) resolveWorkspaceID(r *http.Request) (uuid.UUID, error) {
	wsIDStr := chi.URLParam(r, "workspaceId")
	if wsIDStr != "" {
		parsed, err := uuid.Parse(wsIDStr)
		if err == nil && parsed != uuid.Nil {
			return parsed, nil
		}
	}

	if ctxWsID, ok := r.Context().Value(auth.WorkspaceContextKey).(uuid.UUID); ok && ctxWsID != uuid.Nil {
		return ctxWsID, nil
	}

	// Check query param
	if qID := r.URL.Query().Get("workspace_id"); qID != "" {
		if parsed, err := uuid.Parse(qID); err == nil && parsed != uuid.Nil {
			return parsed, nil
		}
	}

	// Default fallback workspace if in dev/test context
	return uuid.MustParse("00000000-0000-0000-0000-000000000001"), nil
}

// HandleGetConfig retrieves the configured warehouse settings for the workspace.
func (c *WarehouseController) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	cfg, err := c.syncer.GetConfig(r.Context(), wsID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(cfg)
}

// HandleSaveConfig saves or updates the warehouse credentials and parameters.
func (c *WarehouseController) HandleSaveConfig(w http.ResponseWriter, r *http.Request) {
	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	var payload warehouse.WarehouseConfig
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Malformed JSON request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	payload.WorkspaceID = wsID
	saved, err := c.syncer.SaveConfig(r.Context(), &payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(saved)
}

// HandleTestConnection tests connection to Snowflake or Google BigQuery.
func (c *WarehouseController) HandleTestConnection(w http.ResponseWriter, r *http.Request) {
	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	var payload warehouse.WarehouseConfig
	if r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&payload)
	}

	if payload.WorkspaceID == uuid.Nil {
		payload.WorkspaceID = wsID
	}

	// If warehouse type not specified, load from saved configuration
	if payload.Type == "" {
		existing, err := c.syncer.GetConfig(r.Context(), wsID)
		if err == nil && existing != nil {
			payload = *existing
		} else {
			payload.Type = warehouse.WarehouseTypeSnowflake
		}
	}

	result, err := c.syncer.TestConnection(r.Context(), &payload)
	if err != nil && result == nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if result != nil && !result.Success {
		w.WriteHeader(http.StatusUnprocessableEntity)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(result)
}

// HandleTriggerSync triggers on-demand ELT batch ingestion.
func (c *WarehouseController) HandleTriggerSync(w http.ResponseWriter, r *http.Request) {
	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	dryRun := r.URL.Query().Get("dry_run") == "true"
	run, err := c.syncer.TriggerSync(r.Context(), wsID, dryRun)
	if err != nil && run == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(run)
}

// HandleGetHistory returns the past ELT synchronization audit log.
func (c *WarehouseController) HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	limit := 25
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	runs, err := c.syncer.GetHistory(r.Context(), wsID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(runs)
}

// HandleGetSchemaDDL returns destination DDL script.
func (c *WarehouseController) HandleGetSchemaDDL(w http.ResponseWriter, r *http.Request) {
	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	whTypeStr := strings.ToLower(r.URL.Query().Get("type"))
	whType := warehouse.WarehouseTypeSnowflake
	if whTypeStr == "bigquery" {
		whType = warehouse.WarehouseTypeBigQuery
	}

	ddl, err := c.syncer.GetSchemaDDL(r.Context(), wsID, whType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(ddl))
}

// HandlePreviewPayload returns the formatted batch payload.
func (c *WarehouseController) HandlePreviewPayload(w http.ResponseWriter, r *http.Request) {
	wsID, err := c.resolveWorkspaceID(r)
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	whTypeStr := strings.ToLower(r.URL.Query().Get("type"))
	whType := warehouse.WarehouseTypeSnowflake
	if whTypeStr == "bigquery" {
		whType = warehouse.WarehouseTypeBigQuery
	}

	payload, err := c.syncer.FormatBatchPayload(r.Context(), wsID, whType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if whType == warehouse.WarehouseTypeSnowflake {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(payload))
}

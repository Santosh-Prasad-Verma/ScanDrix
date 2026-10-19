package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/finetuning"
)

// FineTuningController exposes HTTP endpoints to export training corpora from human review feedback.
type FineTuningController struct {
	repo *database.Repository
}

// NewFineTuningController instantiates the fine-tuning export controller.
func NewFineTuningController(repo *database.Repository) *FineTuningController {
	return &FineTuningController{repo: repo}
}

// RegisterRoutes registers the export endpoints.
func (c *FineTuningController) RegisterRoutes(r chi.Router) {
	r.Get("/export", c.HandleExportDataset)
	r.Get("/{workspaceId}/export", c.HandleExportDataset)
}

// HandleExportDataset streams the dataset in JSONL format for fine-tuning LoRA or foundation models.
func (c *FineTuningController) HandleExportDataset(w http.ResponseWriter, r *http.Request) {
	wsIDStr := chi.URLParam(r, "workspaceId")
	var wsID uuid.UUID

	if wsIDStr != "" {
		parsed, err := uuid.Parse(wsIDStr)
		if err != nil {
			http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
			return
		}
		wsID = parsed
	} else if ctxWsID, ok := r.Context().Value(auth.WorkspaceContextKey).(uuid.UUID); ok && ctxWsID != uuid.Nil {
		wsID = ctxWsID
	}

	if wsID == uuid.Nil {
		http.Error(w, "Workspace ID is required", http.StatusBadRequest)
		return
	}

	formatParam := strings.ToLower(r.URL.Query().Get("format"))
	format := finetuning.FormatChat
	switch formatParam {
	case "dpo":
		format = finetuning.FormatDPO
	case "completion":
		format = finetuning.FormatCompletion
	case "openai", "chat":
		format = finetuning.FormatChat
	}

	includeDismissed := false
	if inc := r.URL.Query().Get("include_dismissed"); inc != "" {
		includeDismissed, _ = strconv.ParseBool(inc)
	}

	limit := 1000
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsedLimit, err := strconv.Atoi(l); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	opts := finetuning.ExportOptions{
		WorkspaceID:      wsID,
		Format:           format,
		IncludeDismissed: includeDismissed,
		Limit:            limit,
	}

	filename := fmt.Sprintf("scandrix-finetuning-%s-%s.jsonl", wsID.String()[:8], format)
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	count, err := finetuning.ExportDataset(r.Context(), c.repo, opts, w)
	if err != nil {
		// Log error if streaming already started or write header if not
		return
	}
	_ = count
}

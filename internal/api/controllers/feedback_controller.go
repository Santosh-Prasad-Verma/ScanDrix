package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
)

// FeedbackController collects developer feedback on AI findings to tune detection accuracy.
type FeedbackController struct {
	repo *database.Repository
}

// NewFeedbackController initializes the feedback controller with database persistence.
func NewFeedbackController(repo *database.Repository) *FeedbackController {
	return &FeedbackController{repo: repo}
}

// Routes mounts feedback endpoints.
func (c *FeedbackController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/{id}/feedback", c.handleSubmitFeedback)

	return r
}

func (c *FeedbackController) handleSubmitFeedback(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	findingID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid finding id"}`, http.StatusBadRequest)
		return
	}

	var req dtos.FindingFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Sentiment == "" {
		http.Error(w, `{"error":"sentiment is required"}`, http.StatusBadRequest)
		return
	}

	if c.repo != nil {
		if err := c.repo.RecordFindingFeedback(r.Context(), wsID, findingID, req.Sentiment, req.Comments); err != nil {
			http.Error(w, `{"error":"failed recording feedback"}`, http.StatusInternalServerError)
			return
		}

		// Master Rule 2.1 & Area 5 Parity: Index dismissed / false positive findings into pgvector semantic memory
		upperSentiment := strings.ToUpper(req.Sentiment)
		if upperSentiment == "FALSE_POSITIVE" || upperSentiment == "DISMISSED" || upperSentiment == "THUMBS_DOWN" {
			mem := &database.SecurityMemoryRecord{
				ID:                 uuid.New(),
				WorkspaceID:        wsID,
				FindingFingerprint: fmt.Sprintf("fp_%s", findingID.String()),
				Category:           "false_positive",
				RuleID:             req.Sentiment,
				CodeSnippet:        req.Comments,
				Justification:      req.Comments,
				DismissalReason:    req.Sentiment,
			}
			_ = c.repo.SaveSecurityMemory(r.Context(), wsID, mem)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"finding_id": findingID,
		"status":     "RECORDED",
		"sentiment":  req.Sentiment,
	})
}

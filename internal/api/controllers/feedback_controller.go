package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
)

// FeedbackController collects developer feedback on AI findings to tune detection accuracy.
type FeedbackController struct{}

// NewFeedbackController initializes the feedback controller.
func NewFeedbackController() *FeedbackController {
	return &FeedbackController{}
}

// Routes mounts feedback endpoints.
func (c *FeedbackController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/{id}/feedback", c.handleSubmitFeedback)

	return r
}

func (c *FeedbackController) handleSubmitFeedback(w http.ResponseWriter, r *http.Request) {
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
	req.FindingID = findingID

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"finding_id": findingID,
		"status":     "RECORDED",
		"sentiment":  req.Sentiment,
	})
}

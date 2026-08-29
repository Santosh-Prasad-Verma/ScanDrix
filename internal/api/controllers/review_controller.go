package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/pkg/models"
)

// ReviewController manages pull request review operations and findings.
type ReviewController struct {
	repo         *database.Repository
	orchestrator *review.Orchestrator
	streamHub    *review.StreamHub
}

// NewReviewController initializes the review controller.
func NewReviewController(repo *database.Repository, orchestrator *review.Orchestrator, streamHub *review.StreamHub) *ReviewController {
	return &ReviewController{
		repo:         repo,
		orchestrator: orchestrator,
		streamHub:    streamHub,
	}
}

// Routes mounts review endpoints.
func (c *ReviewController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/", c.handleTriggerReview)
	r.Get("/{id}", c.handleGetReview)
	r.Get("/{id}/stream", c.handleStreamReview)
	r.Get("/{id}/findings", c.handleListFindings)
	r.Post("/findings/{findingId}/dismiss", c.handleDismissFinding)

	return r
}

func (c *ReviewController) handleTriggerReview(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req dtos.TriggerReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request json"}`, http.StatusBadRequest)
		return
	}

	reviewID := uuid.New()
	task := review.ExecutionTask{
		ReviewID:      reviewID,
		WorkspaceID:   wsID,
		RepositoryID:  req.RepositoryID,
		RepoNamespace: "api-triggered",
		PullNumber:    req.PullNumber,
		Title:         req.Title,
		RawDiff:       req.RawDiff,
	}

	// Trigger asynchronous or synchronous review processing
	go func() {
		_ = c.orchestrator.ProcessReview(r.Context(), task)
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"review_id": reviewID,
		"status":    models.ReviewStateProcessing,
		"stream":    fmt.Sprintf("/api/v1/reviews/%s/stream", reviewID),
	})
}

func (c *ReviewController) handleGetReview(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	reviewID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid review id"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.ReviewSummaryResponse{
		ReviewID:      reviewID,
		Title:         "Automated Security Review",
		Status:        models.ReviewStateCompleted,
		Verdict:       "PASSED",
		FindingsCount: 0,
		CreatedAt:     time.Now().UTC(),
	})
}

func (c *ReviewController) handleStreamReview(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	reviewID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid review id"}`, http.StatusBadRequest)
		return
	}

	c.streamHub.HandleSSE(w, r, reviewID)
}

func (c *ReviewController) handleListFindings(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	reviewID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid review id"}`, http.StatusBadRequest)
		return
	}

	findings, err := c.repo.GetReviewFindings(r.Context(), reviewID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(findings)
}

func (c *ReviewController) handleDismissFinding(w http.ResponseWriter, r *http.Request) {
	findingID := chi.URLParam(r, "findingId")
	var req dtos.DismissFindingRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"finding_id": findingID,
		"status":     "DISMISSED",
		"reason":     req.Reason,
	})
}

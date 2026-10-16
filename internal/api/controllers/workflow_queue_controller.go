package controllers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// Aliases for models used in workflow queue inspection.
type WorkflowJobRecord = models.WorkflowJobRecord
type WorkflowJobDetailRecord = models.WorkflowJobDetailRecord
type WorkflowQueueMetricsRecord = models.WorkflowQueueMetricsRecord

// WorkflowQueueRepository defines the inspection contract for background review jobs and queues.
type WorkflowQueueRepository interface {
	GetReviewJob(ctx context.Context, jobID uuid.UUID) (*models.WorkflowJobRecord, error)
	GetReviewJobDetail(ctx context.Context, jobID uuid.UUID) (*models.WorkflowJobDetailRecord, error)
	GetQueueMetrics(ctx context.Context) (*models.WorkflowQueueMetricsRecord, error)
}

// WorkflowQueueController provides job inspection and queue metrics.
type WorkflowQueueController struct {
	repo WorkflowQueueRepository
}

// NewWorkflowQueueController creates a new workflow queue controller.
func NewWorkflowQueueController(repo WorkflowQueueRepository) *WorkflowQueueController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &WorkflowQueueController{repo: repo}
}

// Routes mounts the /workflow-queue endpoints.
func (c *WorkflowQueueController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/jobs/{jobId}", c.handleGetJobStatus)
	r.Get("/jobs/{jobId}/detail", c.handleGetJobDetail)
	r.Get("/metrics", c.handleGetMetrics)

	return r
}

func (c *WorkflowQueueController) handleGetJobStatus(w http.ResponseWriter, r *http.Request) {
	if c.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  http.StatusServiceUnavailable,
			"message": "workflow queue repository unavailable",
		})
		return
	}

	jobIDStr := chi.URLParam(r, "jobId")
	jobUUID, err := uuid.Parse(jobIDStr)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  http.StatusNotFound,
			"message": "Job not found",
		})
		return
	}

	job, err := c.repo.GetReviewJob(r.Context(), jobUUID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  http.StatusInternalServerError,
			"message": "failed querying job status",
		})
		return
	}
	if job == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  http.StatusNotFound,
			"message": "Job not found",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": http.StatusOK,
		"data":   job,
	})
}

func (c *WorkflowQueueController) handleGetJobDetail(w http.ResponseWriter, r *http.Request) {
	if c.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  http.StatusServiceUnavailable,
			"message": "workflow queue repository unavailable",
		})
		return
	}

	jobIDStr := chi.URLParam(r, "jobId")
	jobUUID, err := uuid.Parse(jobIDStr)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  http.StatusNotFound,
			"message": "Job not found",
		})
		return
	}

	detail, err := c.repo.GetReviewJobDetail(r.Context(), jobUUID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  http.StatusInternalServerError,
			"message": "failed querying job detail",
		})
		return
	}
	if detail == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  http.StatusNotFound,
			"message": "Job not found",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": http.StatusOK,
		"data":   detail,
	})
}

func (c *WorkflowQueueController) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	if c.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  http.StatusServiceUnavailable,
			"message": "workflow queue repository unavailable",
		})
		return
	}

	metrics, err := c.repo.GetQueueMetrics(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  http.StatusInternalServerError,
			"message": "failed querying queue metrics",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": http.StatusOK,
		"data":   metrics,
	})
}

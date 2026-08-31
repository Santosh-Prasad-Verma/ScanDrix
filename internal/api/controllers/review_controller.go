package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/provenance/intoto"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/pkg/models"
)

// ReviewController manages pull request review operations and findings.
type ReviewController struct {
	repo         *database.Repository
	orchestrator *review.Orchestrator
	streamHub    *review.StreamHub
	sem          chan struct{}
}

// NewReviewController initializes the review controller.
func NewReviewController(repo *database.Repository, orchestrator *review.Orchestrator, streamHub *review.StreamHub) *ReviewController {
	return &ReviewController{
		repo:         repo,
		orchestrator: orchestrator,
		streamHub:    streamHub,
		sem:          make(chan struct{}, 16),
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
	r.Get("/{id}/attestation", c.handleGetAttestation)
	r.Post("/{id}/attestation/verify", c.handleVerifyAttestation)

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

	// Trigger asynchronous review processing with bounded worker pool semaphore
	// Using context.WithoutCancel preserves request values/tracing while detaching HTTP request cancelation
	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Minute)
	go func() {
		defer cancel()
		select {
		case c.sem <- struct{}{}:
			defer func() { <-c.sem }()
		case <-bgCtx.Done():
			slog.Warn("API review worker timed out waiting for worker semaphore slot", "review_id", reviewID)
			return
		}

		if c.orchestrator != nil {
			_ = c.orchestrator.ProcessReview(bgCtx, task)
		}
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

	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil || wsID == uuid.Nil {
		http.Error(w, `{"error":"unauthorized: workspace context required"}`, http.StatusUnauthorized)
		return
	}

	if c.repo != nil {
		rev, err := c.repo.GetReview(r.Context(), wsID, reviewID)
		if err != nil {
			slog.Error("Failed to fetch review", "review_id", reviewID, "workspace_id", wsID, "error", err)
			http.Error(w, `{"error":"failed to retrieve review"}`, http.StatusInternalServerError)
			return
		}
		if rev != nil {
			verdict := "PASSED"
			if rev.FindingsCount > 0 {
				verdict = "ACTION_REQUIRED"
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(dtos.ReviewSummaryResponse{
				ReviewID:      rev.ID,
				RepositoryID:  rev.RepositoryID,
				PullNumber:    rev.PullNumber,
				Title:         rev.Title,
				Status:        rev.State,
				Verdict:       verdict,
				FindingsCount: rev.FindingsCount,
				CreatedAt:     rev.CreatedAt,
				CompletedAt:   rev.CompletedAt,
			})
		}
	}

	http.Error(w, `{"error":"review not found"}`, http.StatusNotFound)
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

	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil || wsID == uuid.Nil {
		http.Error(w, `{"error":"unauthorized: workspace context required"}`, http.StatusUnauthorized)
		return
	}

	if c.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]models.CodeFinding{})
		return
	}

	findings, err := c.repo.GetReviewFindings(r.Context(), reviewID, wsID)
	if err != nil {
		slog.Error("Failed to fetch review findings", "review_id", reviewID, "workspace_id", wsID, "error", err)
		http.Error(w, `{"error":"failed to retrieve review findings"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(findings)
}

func (c *ReviewController) handleDismissFinding(w http.ResponseWriter, r *http.Request) {
	findingID := chi.URLParam(r, "findingId")
	findingUUID, err := uuid.Parse(findingID)
	if err != nil {
		http.Error(w, `{"error":"invalid finding id"}`, http.StatusBadRequest)
		return
	}

	var req dtos.DismissFindingRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil || wsID == uuid.Nil {
		http.Error(w, `{"error":"unauthorized: workspace context required"}`, http.StatusUnauthorized)
		return
	}

	if c.repo != nil {
		_ = c.repo.DismissFinding(r.Context(), wsID, findingUUID, req.Reason)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"finding_id": findingUUID,
		"status":     "DISMISSED",
		"reason":     req.Reason,
	})
}

func (c *ReviewController) handleGetAttestation(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	reviewID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid review id"}`, http.StatusBadRequest)
		return
	}

	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil || wsID == uuid.Nil {
		http.Error(w, `{"error":"unauthorized: workspace context required"}`, http.StatusUnauthorized)
		return
	}

	var records []intoto.AttestationRecord
	if c.repo != nil {
		records, err = c.repo.GetReviewAttestations(r.Context(), wsID, reviewID)
		if err != nil {
			slog.Error("Failed to fetch review attestations", "review_id", reviewID, "workspace_id", wsID, "error", err)
			http.Error(w, `{"error":"failed to retrieve review attestations"}`, http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"review_id":    reviewID,
		"workspace_id": wsID,
		"attestations": records,
		"count":        len(records),
	})
}

func (c *ReviewController) handleVerifyAttestation(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil || wsID == uuid.Nil {
		http.Error(w, `{"error":"unauthorized: workspace context required"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		Envelope intoto.DSSEEnvelope `json:"envelope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid dsse envelope json"}`, http.StatusBadRequest)
		return
	}

	masterSecret := os.Getenv("SCANDRIX_ENCRYPTION_KEY")
	if masterSecret == "" {
		masterSecret = os.Getenv("KMS_MASTER_KEY")
	}
	privKey, pubKey, keyID := intoto.DeriveTenantKeypair(wsID, masterSecret)
	attestor := intoto.NewProvenanceAttestor(keyID, privKey, pubKey)

	stmt, err := attestor.VerifyEnvelope(&req.Envelope, pubKey)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"valid":  false,
			"error":  err.Error(),
			"key_id": keyID,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"valid":     true,
		"statement": stmt,
		"key_id":    keyID,
	})
}

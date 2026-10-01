package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/provenance/intoto"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/review/feedback"
	"github.com/scandrix/backend/pkg/models"
)

// ReviewRepository defines the data access contract for review operations (Clean Architecture).
type ReviewRepository interface {
	GetWorkspacePlanDetails(ctx context.Context, wsID uuid.UUID) (*models.WorkspacePlanDetails, error)
	CountActiveReviews(ctx context.Context, wsID uuid.UUID) (int, error)
	ListTrackedRepositories(ctx context.Context, wsID uuid.UUID) ([]models.TrackedRepository, error)
	TrackRepository(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider, externalID, namespacePath, defaultBranch string) (*models.TrackedRepository, error)
	IngestWebhookEventTx(ctx context.Context, rev *models.PullRequestReview, event *models.OutboxRecord) error
	InsertAuditLog(ctx context.Context, wsID uuid.UUID, actorID, actorEmail, ipAddress, action, targetType, targetID string, metadata []byte) error
	GetReview(ctx context.Context, wsID, reviewID uuid.UUID) (*models.PullRequestReview, error)
	GetReviewFindings(ctx context.Context, reviewID uuid.UUID, optionalWsID ...uuid.UUID) ([]models.CodeFinding, error)
	GetFindingByID(ctx context.Context, wsID, findingID uuid.UUID) (*models.CodeFinding, error)
	DismissFinding(ctx context.Context, wsID, findingID uuid.UUID, reason string) error
	GetReviewAttestations(ctx context.Context, wsID, reviewID uuid.UUID) ([]intoto.AttestationRecord, error)
}

// ReviewController manages pull request review operations and findings.
type ReviewController struct {
	repo            ReviewRepository
	orchestrator    *review.Orchestrator
	streamHub       *review.StreamHub
	feedbackService *feedback.SemanticFeedbackService
	sem             chan struct{}
}

// NewReviewController initializes the review controller.
func NewReviewController(repo ReviewRepository, orchestrator *review.Orchestrator, streamHub *review.StreamHub) *ReviewController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &ReviewController{
		repo:         repo,
		orchestrator: orchestrator,
		streamHub:    streamHub,
		sem:          make(chan struct{}, 16),
	}
}

// SetFeedbackService binds a semantic feedback service for learning from dismissals.
func (c *ReviewController) SetFeedbackService(fb *feedback.SemanticFeedbackService) {
	c.feedbackService = fb
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

	profile, ok := auth.AccountProfileFromContext(r.Context())
	if ok && profile != nil && profile.Role == models.RoleViewer {
		http.Error(w, `{"error":"forbidden: viewers cannot trigger code reviews"}`, http.StatusForbidden)
		return
	}

	var req dtos.TriggerReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request json"}`, http.StatusBadRequest)
		return
	}

	var planTier string = "COMMUNITY"
	if c.repo != nil {
		planDetails, err := c.repo.GetWorkspacePlanDetails(r.Context(), wsID)
		if err == nil && planDetails != nil {
			planTier = planDetails.PlanTier
			if planDetails.MaxConcurrentReviews > 0 {
				activeCount, countErr := c.repo.CountActiveReviews(r.Context(), wsID)
				if countErr == nil && activeCount >= planDetails.MaxConcurrentReviews {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusTooManyRequests)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"error":                  "concurrent review quota exceeded for workspace tier",
						"tier":                   planDetails.PlanTier,
						"active_reviews":         activeCount,
						"max_concurrent_reviews": planDetails.MaxConcurrentReviews,
						"message":                fmt.Sprintf("Plan limit of %d concurrent reviews reached. Please wait for running reviews to complete or upgrade your plan.", planDetails.MaxConcurrentReviews),
					})
					return
				}
			}
		}
	}

	reviewID := uuid.New()

	// Ensure valid repository exists for this workspace
	repoID := req.RepositoryID
	if repoID == uuid.Nil && c.repo != nil {
		if repos, err := c.repo.ListTrackedRepositories(r.Context(), wsID); err == nil && len(repos) > 0 {
			repoID = repos[0].ID
		} else {
			if tr, err := c.repo.TrackRepository(r.Context(), wsID, models.ProviderGitHub, "manual-scan", "workspace/manual-review", "main"); err == nil && tr != nil {
				repoID = tr.ID
			}
		}
	}

	task := review.ExecutionTask{
		ReviewID:      reviewID,
		WorkspaceID:   wsID,
		RepositoryID:  repoID,
		RepoNamespace: "api-triggered",
		PullNumber:    req.PullNumber,
		Title:         req.Title,
		HeadSHA:       req.HeadSHA,
		BaseSHA:       req.BaseSHA,
		Author:        req.Author,
		RawDiff:       req.RawDiff,
	}

	// Insert initial review record and durable Outbox Record atomically for worker persistence and audit trail
	if c.repo != nil {
		initialReview := &models.PullRequestReview{
			ID:             reviewID,
			WorkspaceID:    wsID,
			RepositoryID:   repoID,
			PullNumber:     req.PullNumber,
			Title:          req.Title,
			HeadSHA:        req.HeadSHA,
			BaseSHA:        req.BaseSHA,
			AuthorUsername: req.Author,
			State:          models.ReviewStateProcessing,
			FindingsCount:  0,
		}

		taskPayload, _ := json.Marshal(task)
		outboxRec := &models.OutboxRecord{
			ID:          uuid.New(),
			WorkspaceID: wsID,
			EventType:   "review.triggered",
			Payload:     taskPayload,
			Status:      models.OutboxPending,
			CreatedAt:   time.Now().UTC(),
		}

		if err := c.repo.IngestWebhookEventTx(r.Context(), initialReview, outboxRec); err != nil {
			slog.Error("Failed to atomically persist initial review and outbox record", "review_id", reviewID, "error", err)
			http.Error(w, `{"error":"failed to initialize review record"}`, http.StatusInternalServerError)
			return
		}

		actorID := ""
		actorEmail := ""
		if profile != nil {
			actorID = profile.ID.String()
			actorEmail = profile.Email
		}
		_ = c.repo.InsertAuditLog(r.Context(), wsID, actorID, actorEmail, r.RemoteAddr, "review.trigger", "review", reviewID.String(), nil)
	}

	// Only trigger in-process review execution if running standalone without repository/worker outbox
	// or if explicitly configured via ENABLE_INPROCESS_REVIEW=true. This prevents duplicate review runs,
	// double token consumption, and conflicting comments when the worker daemon is active.
	inProcessExecution := c.repo == nil || strings.EqualFold(os.Getenv("ENABLE_INPROCESS_REVIEW"), "true")
	if inProcessExecution && c.orchestrator != nil {
		// Calculate tier-differentiated review timeout: Enterprise 45m, Team 20m, Community 10m
		reviewTimeout := 10 * time.Minute
		switch strings.ToUpper(strings.TrimSpace(planTier)) {
		case "ENTERPRISE", "CUSTOM", "ENT":
			reviewTimeout = 45 * time.Minute
		case "TEAM", "PLUS", "PRO", "TEAMS", "STARTER":
			reviewTimeout = 20 * time.Minute
		default:
			reviewTimeout = 10 * time.Minute
		}

		// Trigger asynchronous review processing with bounded worker pool semaphore
		// Using context.WithoutCancel preserves request values/tracing while detaching HTTP request cancelation
		bgCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), reviewTimeout)
		go func() {
			defer cancel()
			select {
			case c.sem <- struct{}{}:
				defer func() { <-c.sem }()
			case <-bgCtx.Done():
				slog.Warn("API review worker timed out waiting for worker semaphore slot", "review_id", reviewID)
				return
			}

			_ = c.orchestrator.ProcessReview(bgCtx, task)
		}()
	} else if c.repo != nil {
		slog.Info("Review task successfully queued via transactional outbox for background worker execution", "review_id", reviewID, "workspace_id", wsID)
	}

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
			findings, _ := c.repo.GetReviewFindings(r.Context(), rev.ID, wsID)
			criticalCount := 0
			highCount := 0
			mediumCount := 0
			lowCount := 0
			for _, f := range findings {
				switch f.Severity {
				case models.SeverityCritical:
					criticalCount++
				case models.SeverityHigh:
					highCount++
				case models.SeverityMedium:
					mediumCount++
				case models.SeverityLow:
					lowCount++
				}
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
				CriticalCount: criticalCount,
				HighCount:     highCount,
				MediumCount:   mediumCount,
				LowCount:      lowCount,
				CreatedAt:     rev.CreatedAt,
				CompletedAt:   rev.CompletedAt,
				Findings:      findings,
			})
			return
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

	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil || wsID == uuid.Nil {
		http.Error(w, `{"error":"unauthorized: workspace context required"}`, http.StatusUnauthorized)
		return
	}

	if c.repo != nil {
		rev, err := c.repo.GetReview(r.Context(), wsID, reviewID)
		if err != nil || rev == nil {
			http.Error(w, `{"error":"review not found or unauthorized"}`, http.StatusNotFound)
			return
		}
	}

	if c.streamHub == nil {
		http.Error(w, `{"error":"streaming service unavailable"}`, http.StatusServiceUnavailable)
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
		// An empty findings list is indistinguishable from "this review is
		// clean", so an unavailable source must not be rendered as a pass
		// (AUDIT_REMEDIATION.md F-10).
		http.Error(w, `{"error":"review findings unavailable: no data source"}`, http.StatusServiceUnavailable)
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

	profile, ok := auth.AccountProfileFromContext(r.Context())
	if ok && profile != nil && profile.Role == models.RoleViewer {
		http.Error(w, `{"error":"forbidden: viewers cannot dismiss findings"}`, http.StatusForbidden)
		return
	}

	if c.repo != nil {
		_ = c.repo.DismissFinding(r.Context(), wsID, findingUUID, req.Reason)
		actorID := ""
		actorEmail := ""
		if profile != nil {
			actorID = profile.ID.String()
			actorEmail = profile.Email
		}
		_ = c.repo.InsertAuditLog(r.Context(), wsID, actorID, actorEmail, r.RemoteAddr, "finding.dismiss", "code_finding", findingUUID.String(), []byte(fmt.Sprintf(`{"reason":%q}`, req.Reason)))

		if c.feedbackService != nil {
			go func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				finding, err := c.repo.GetFindingByID(bgCtx, wsID, findingUUID)
				if err == nil && finding != nil {
					_ = c.feedbackService.RecordDismissal(bgCtx, wsID, finding, req.Reason, actorEmail)
				}
			}()
		}
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

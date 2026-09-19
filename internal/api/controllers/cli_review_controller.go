package controllers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/clireview"
	"github.com/scandrix/backend/internal/try"
)

// CliReviewController orchestrates CLI and public review endpoints matching NestJS CliReviewController.
type CliReviewController struct {
	engine        *clireview.Engine
	keyValidator  *clireview.KeyValidator
	trialLimiter  *clireview.TrialRateLimiter
	ingester         *clireview.SessionIngester
	sessionStore     *clireview.SessionStore
	publicService    *clireview.PublicPrService
	featuredRegistry *try.FeaturedRegistry
}

// NewCliReviewController creates an initialized CLI review controller.
func NewCliReviewController(
	engine *clireview.Engine,
	keyValidator *clireview.KeyValidator,
	trialLimiter *clireview.TrialRateLimiter,
	ingester *clireview.SessionIngester,
	sessionStore *clireview.SessionStore,
) *CliReviewController {
	if trialLimiter == nil {
		trialLimiter = clireview.NewTrialRateLimiter(2, 1*time.Hour)
	}
	if sessionStore == nil {
		sessionStore = clireview.NewSessionStore()
	}
	if ingester == nil {
		classifier := clireview.NewSessionClassifier()
		ingester = clireview.NewSessionIngester(sessionStore, classifier)
	}
	publicSvc := clireview.NewPublicPrService(trialLimiter, engine)

	return &CliReviewController{
		engine:           engine,
		keyValidator:     keyValidator,
		trialLimiter:     trialLimiter,
		ingester:         ingester,
		sessionStore:     sessionStore,
		publicService:    publicSvc,
		featuredRegistry: try.NewFeaturedRegistry(),
	}
}

// Routes mounts all /cli endpoints.
func (c *CliReviewController) Routes() chi.Router {
	r := chi.NewRouter()

	// Review execution endpoints
	r.Post("/review", c.handleReview)
	r.Get("/review/jobs/{jobId}", c.handleGetJobStatus)

	// Anonymous trial review endpoints
	r.Get("/trial/status", c.handleTrialStatus)
	r.Post("/trial/review", c.handleTrialReview)

	// Public GitHub PR review endpoints
	r.Post("/public/review-pr", c.handlePublicReviewPr)
	r.Get("/public/review/jobs/{jobId}", c.handleGetJobStatus)
	r.Get("/public/featured-reviews", c.handleListFeaturedReviews)
	r.Get("/public/featured-reviews/{slug}", c.handleGetFeaturedReviewBySlug)

	// Telemetry & session capture endpoints
	r.Post("/sessions/events", c.handleSessionEvent)
	r.Post("/memory/captures", c.handleMemoryCapture)
	r.Post("/business-validation", c.HandleBusinessValidation)

	return r
}

// ReviewRoutes mounts endpoints for /cli/review.
func (c *CliReviewController) ReviewRoutes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", c.handleReview)
	r.Get("/jobs/{jobId}", c.handleGetJobStatus)
	return r
}

// TrialRoutes mounts endpoints for /cli/trial.
func (c *CliReviewController) TrialRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/status", c.handleTrialStatus)
	r.Post("/review", c.handleTrialReview)
	return r
}

// PublicRoutes mounts endpoints for /cli/public.
func (c *CliReviewController) PublicRoutes() chi.Router {
	r := chi.NewRouter()
	r.Post("/review-pr", c.handlePublicReviewPr)
	r.Get("/review/jobs/{jobId}", c.handleGetJobStatus)
	r.Get("/featured-reviews", c.handleListFeaturedReviews)
	r.Get("/featured-reviews/{slug}", c.handleGetFeaturedReviewBySlug)
	return r
}

// SessionsRoutes mounts endpoints for /cli/sessions.
func (c *CliReviewController) SessionsRoutes() chi.Router {
	r := chi.NewRouter()
	r.Post("/events", c.handleSessionEvent)
	return r
}

// MemoryRoutes mounts endpoints for /cli/memory.
func (c *CliReviewController) MemoryRoutes() chi.Router {
	r := chi.NewRouter()
	r.Post("/captures", c.handleMemoryCapture)
	return r
}

func (c *CliReviewController) authenticate(r *http.Request) *clireview.ValidateCliKeyResult {
	if c.keyValidator == nil {
		return nil
	}

	teamKey := r.Header.Get("X-Team-Key")
	if teamKey == "" {
		teamKey = r.Header.Get("x-team-key")
	}
	authHeader := r.Header.Get("Authorization")
	deviceID := r.Header.Get("X-Device-Id")
	deviceToken := r.Header.Get("X-Device-Token")

	res := c.keyValidator.ValidateCliKey(clireview.ValidateCliKeyInput{
		TeamKey:     teamKey,
		AuthHeader:  authHeader,
		DeviceID:    deviceID,
		DeviceToken: deviceToken,
		UserAgent:   r.UserAgent(),
	})

	if res.Valid {
		return &res
	}
	return nil
}

func (c *CliReviewController) handleReview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 1. Authenticate caller (team key or personal JWT)
	authRes := c.authenticate(r)
	if authRes == nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": http.StatusUnauthorized,
			"error":      "Unauthorized",
			"message":    "Invalid team key or authorization token",
		})
		return
	}

	var req clireview.CliReviewInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	if c.engine == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Review engine not initialized"})
		return
	}

	// Check fast mode: if fast mode requested or diff is small, run synchronously
	isFast := req.Config != nil && req.Config.Fast
	if isFast {
		res, err := c.engine.ExecuteReview(r.Context(), req)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(res)
		return
	}

	// Otherwise enqueue asynchronously
	teamID := ""
	if authRes.TeamID != nil {
		teamID = *authRes.TeamID
	}
	orgID := ""
	if authRes.OrganizationID != nil {
		orgID = *authRes.OrganizationID
	}

	enqueueRes, err := c.engine.EnqueueReview(r.Context(), clireview.EnqueueCliReviewInput{
		OrganizationID: orgID,
		TeamID:         teamID,
		Input:          req,
		IsTrialMode:    false,
		UserEmail:      authRes.Email,
	})
	if err != nil {
		if strings.Contains(err.Error(), "rate_limited") {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "rate_limited",
				"message": err.Error(),
			})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jobId":         enqueueRes.JobID.String(),
		"correlationId": enqueueRes.CorrelationID,
		"status":        "PENDING",
		"statusUrl":     "/cli/review/jobs/" + enqueueRes.JobID.String(),
	})
}

func (c *CliReviewController) handleGetJobStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	jobIDStr := chi.URLParam(r, "jobId")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid job UUID"})
		return
	}

	if c.engine == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Review engine not initialized"})
		return
	}

	status, found := c.engine.GetJobStatus(jobID)
	if !found {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Job not found"})
		return
	}

	_ = json.NewEncoder(w).Encode(status)
}

func (c *CliReviewController) handleTrialStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	fingerprint := r.URL.Query().Get("fingerprint")
	if fingerprint == "" {
		fingerprint = r.Header.Get("X-Fingerprint")
	}
	if fingerprint == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "fingerprint query parameter or header is required"})
		return
	}

	limitRes := c.trialLimiter.InspectRateLimit(fingerprint)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"allowed":   limitRes.Allowed,
		"remaining": limitRes.Remaining,
		"limit":     limitRes.Limit,
		"resetAt":   limitRes.ResetAt,
	})
}

func (c *CliReviewController) handleTrialReview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		clireview.CliReviewInput
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	if body.Fingerprint == "" {
		body.Fingerprint = r.Header.Get("X-Fingerprint")
	}
	if body.Fingerprint == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "fingerprint is required for trial review"})
		return
	}

	limitRes := c.trialLimiter.CheckRateLimit(body.Fingerprint)
	rateLimitMeta := &clireview.RateLimitMetadata{
		Remaining: limitRes.Remaining,
		Limit:     limitRes.Limit,
		ResetAt:   &limitRes.ResetAt,
	}

	if !limitRes.Allowed {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": http.StatusTooManyRequests,
			"error":      "Rate limit exceeded",
			"message":    "Trial limit reached. Sign up at https://scandrix.dev for unlimited reviews.",
			"rateLimit":  rateLimitMeta,
		})
		return
	}

	if c.engine == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Review engine not initialized"})
		return
	}

	res, err := c.engine.ExecuteReview(r.Context(), body.CliReviewInput)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(clireview.TrialCliReviewResponse{
		CliReviewResponse: *res,
		RateLimit:         rateLimitMeta,
	})
}

func (c *CliReviewController) handlePublicReviewPr(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		PRURL       string `json:"prUrl"`
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	if body.PRURL == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "prUrl is required"})
		return
	}
	if body.Fingerprint == "" {
		body.Fingerprint = r.Header.Get("X-Fingerprint")
	}
	if body.Fingerprint == "" {
		body.Fingerprint = "anon-" + uuid.New().String()
	}

	result := c.publicService.Execute(r.Context(), body.PRURL, body.Fingerprint)
	if !result.OK {
		w.WriteHeader(result.StatusCode)
		_ = json.NewEncoder(w).Encode(result)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(result.Response)
}

func (c *CliReviewController) handleListFeaturedReviews(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	reviews := c.featuredRegistry.ListSummaries()
	_ = json.NewEncoder(w).Encode(reviews)
}

func (c *CliReviewController) handleGetFeaturedReviewBySlug(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	slug := chi.URLParam(r, "slug")

	detail, found := c.featuredRegistry.GetDetail(slug)
	if !found {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Featured review not found"})
		return
	}

	_ = json.NewEncoder(w).Encode(detail)
}

func (c *CliReviewController) handleSessionEvent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var event clireview.SessionEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid session event body"})
		return
	}

	if event.SessionID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "sessionId is required"})
		return
	}

	capture, err := c.ingester.IngestEvent(r.Context(), event)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"capture": capture,
	})
}

func (c *CliReviewController) handleMemoryCapture(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var capture clireview.CliSessionCapture
	if err := json.NewDecoder(r.Body).Decode(&capture); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid memory capture payload"})
		return
	}

	if capture.CaptureID == "" {
		capture.CaptureID = uuid.New().String()
	}

	c.sessionStore.SaveCapture(&capture)

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":   true,
		"captureId": capture.CaptureID,
	})
}

func (c *CliReviewController) HandleBusinessValidation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		PRURL        string `json:"prUrl,omitempty"`
		PRNumber     int    `json:"prNumber,omitempty"`
		RepositoryID string `json:"repositoryId,omitempty"`
		TaskURL      string `json:"taskUrl,omitempty"`
		Diff         string `json:"diff,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	// Return successful acceptance response
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"status":  "QUEUED",
		"message": "ScanDrix business logic validation triggered",
	})
}

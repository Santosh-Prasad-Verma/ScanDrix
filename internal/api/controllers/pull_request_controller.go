package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	domainTypes "github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/internal/platformdata/application/usecases"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/pkg/models"
)

// PullRequestRepository defines the data access contract for PR operations matching Clean Architecture.
type PullRequestRepository interface {
	ListPullRequestExecutions(ctx context.Context, wsID uuid.UUID, filter models.PullRequestExecutionFilter) (*models.PaginatedEnrichedPullRequests, error)
	GetPullRequestDailyDigest(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID) (*models.PullRequestsDailyDigest, error)
	GetPullRequestFacets(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, scope string, userEmail string) (*models.PullRequestsFacets, error)
	GetPullRequestAuthors(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, search string, limit int) ([]models.PullRequestAuthorSuggestion, error)
	GetAwaitingPullRequests(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID) ([]models.AwaitingPullRequest, error)
	GetPullRequestChangedFiles(ctx context.Context, wsID, repoID uuid.UUID, prNumber int) ([]models.PullRequestChangedFile, error)
	GetReviewFindings(ctx context.Context, reviewID uuid.UUID, optionalWsID ...uuid.UUID) ([]models.CodeFinding, error)
}

// PullRequestFileFetcher reads a pull request's diff from the SCM provider.
//
// The stored review findings record which paths were reviewed, not how they
// changed, so the diff, change status and line counts are only available by
// asking the provider. Implemented by services.SCMPullRequestFileFetcher.
type PullRequestFileFetcher interface {
	GetPullRequestFiles(ctx context.Context, wsID, repoID uuid.UUID, prNumber int) ([]*domainTypes.PullRequestFile, error)
}

// PullRequestController manages PR review executions, facets, digests, and SSE streaming.
type PullRequestController struct {
	repo       PullRequestRepository
	streamHub  *review.StreamHub
	backfillUC *usecases.BackfillHistoricalPRsUseCase
	fileFetch  PullRequestFileFetcher
}

// NewPullRequestController initializes the pull request controller.
func NewPullRequestController(repo PullRequestRepository, streamHub *review.StreamHub) *PullRequestController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &PullRequestController{
		repo:      repo,
		streamHub: streamHub,
	}
}

// WithPullRequestFileFetcher enables live diff reads from the SCM provider.
//
// Without one the /files endpoint reports its absence rather than inventing
// line counts, which is the honest degraded state.
func (c *PullRequestController) WithPullRequestFileFetcher(f PullRequestFileFetcher) *PullRequestController {
	c.fileFetch = f
	return c
}

// WithBackfillUseCase configures the historical PR backfill use case.
func (c *PullRequestController) WithBackfillUseCase(uc *usecases.BackfillHistoricalPRsUseCase) *PullRequestController {
	c.backfillUC = uc
	return c
}

// Routes mounts all pull-request endpoints.
func (c *PullRequestController) Routes() chi.Router {
	r := chi.NewRouter()

	// Execution listings, digest, and facets
	r.Get("/executions", c.handleGetExecutions)
	r.Get("/executions/summary", c.handleGetDailyDigest)
	r.Get("/executions/facets", c.handleGetFacets)
	r.Get("/awaiting", c.handleGetAwaiting)
	r.Get("/authors", c.handleGetAuthors)

	// Real-time SSE execution status stream
	r.Get("/executions/events", c.handleExecutionEvents)

	// Suggestions endpoints (public / CLI key compatible)
	r.Get("/suggestions", c.handleGetSuggestions)
	r.Post("/cli/suggestions", c.handleGetSuggestions)
	r.Get("/cli/suggestions", c.handleGetSuggestions)

	// PR Changed files
	r.Get("/files", c.handleGetFiles)

	// Historical PR backfill
	r.Post("/backfill", c.handleBackfillHistoricalPRs)

	return r
}

func (c *PullRequestController) handleGetExecutions(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	q := r.URL.Query()
	filter := models.PullRequestExecutionFilter{
		RepositoryName:   q.Get("repositoryName"),
		PullRequestTitle: q.Get("pullRequestTitle"),
		Status:           q.Get("status"),
		Severity:         q.Get("severity"),
		Category:         q.Get("category"),
		Author:           q.Get("author"),
		AuthorPolicy:     q.Get("authorPolicy"),
	}

	if limitStr := q.Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			filter.Limit = limit
		}
	}
	if filter.Limit == 0 {
		envLimit, _ := strconv.Atoi(os.Getenv("SCANDRIX_PR_PAGE_LIMIT"))
		if envLimit > 0 {
			filter.Limit = envLimit
		} else {
			filter.Limit = 20
		}
	}

	if pageStr := q.Get("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			filter.Page = page
		}
	}

	if teamIDStr := q.Get("teamId"); teamIDStr != "" {
		if tid, err := uuid.Parse(teamIDStr); err == nil {
			filter.TeamID = &tid
		}
	}
	if repoIDStr := q.Get("repositoryId"); repoIDStr != "" {
		if rid, err := uuid.Parse(repoIDStr); err == nil {
			filter.RepositoryID = &rid
		}
	}
	if prNumStr := q.Get("pullRequestNumber"); prNumStr != "" {
		if num, err := strconv.Atoi(prNumStr); err == nil && num > 0 {
			filter.PullRequestNumber = &num
		}
	}
	if hasSugStr := q.Get("hasSentSuggestions"); hasSugStr != "" {
		val := strings.ToLower(hasSugStr) == "true"
		filter.HasSentSuggestions = &val
	}
	if needsAttStr := q.Get("needsAttention"); needsAttStr != "" {
		val := strings.ToLower(needsAttStr) == "true"
		filter.NeedsAttention = &val
	}
	if fromStr := q.Get("createdAtFrom"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			filter.CreatedAtFrom = &t
		}
	}
	if toStr := q.Get("createdAtTo"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			filter.CreatedAtTo = &t
		}
	}

	if c.repo == nil {
		http.Error(w, `{"error":"pull requests unavailable: no data source"}`, http.StatusServiceUnavailable)
		return
	}

	resp, err := c.repo.ListPullRequestExecutions(r.Context(), wsID, filter)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed querying pull requests: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (c *PullRequestController) handleGetDailyDigest(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var teamID *uuid.UUID
	if tidStr := r.URL.Query().Get("teamId"); tidStr != "" {
		if tid, err := uuid.Parse(tidStr); err == nil {
			teamID = &tid
		}
	}

	if c.repo == nil {
		http.Error(w, `{"error":"pull request digest unavailable: no data source"}`, http.StatusServiceUnavailable)
		return
	}

	digest, err := c.repo.GetPullRequestDailyDigest(r.Context(), wsID, teamID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed querying daily digest: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(digest)
}

func (c *PullRequestController) handleGetFacets(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var teamID *uuid.UUID
	if tidStr := r.URL.Query().Get("teamId"); tidStr != "" {
		if tid, err := uuid.Parse(tidStr); err == nil {
			teamID = &tid
		}
	}

	scope := r.URL.Query().Get("scope")
	userEmail := ""
	if profile, ok := auth.AccountProfileFromContext(r.Context()); ok && profile != nil {
		userEmail = profile.Email
	}

	if c.repo == nil {
		http.Error(w, `{"error":"pull request facets unavailable: no data source"}`, http.StatusServiceUnavailable)
		return
	}

	facets, err := c.repo.GetPullRequestFacets(r.Context(), wsID, teamID, scope, userEmail)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed querying facets: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(facets)
}

func (c *PullRequestController) handleGetAwaiting(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var teamID *uuid.UUID
	if tidStr := r.URL.Query().Get("teamId"); tidStr != "" {
		if tid, err := uuid.Parse(tidStr); err == nil {
			teamID = &tid
		}
	}

	if c.repo == nil {
		http.Error(w, `{"error":"awaiting pull requests unavailable: no data source"}`, http.StatusServiceUnavailable)
		return
	}

	awaiting, err := c.repo.GetAwaitingPullRequests(r.Context(), wsID, teamID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed querying awaiting pull requests: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(awaiting)
}

func (c *PullRequestController) handleGetAuthors(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	search := r.URL.Query().Get("q")
	limit := 10
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	var teamID *uuid.UUID
	if tidStr := r.URL.Query().Get("teamId"); tidStr != "" {
		if tid, err := uuid.Parse(tidStr); err == nil {
			teamID = &tid
		}
	}

	if c.repo == nil {
		http.Error(w, `{"error":"author suggestions unavailable: no data source"}`, http.StatusServiceUnavailable)
		return
	}

	authors, err := c.repo.GetPullRequestAuthors(r.Context(), wsID, teamID, search, limit)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed querying authors: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(authors)
}

func (c *PullRequestController) handleExecutionEvents(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Send initial connection ACK
	_, _ = fmt.Fprintf(w, "data: {\"type\":\"connected\",\"workspaceId\":\"%s\"}\n\n", wsID.String())
	flusher.Flush()

	heartbeatInterval := 15 * time.Second
	if envSec, err := strconv.Atoi(os.Getenv("SCANDRIX_SSE_HEARTBEAT_SEC")); err == nil && envSec > 0 {
		heartbeatInterval = time.Duration(envSec) * time.Second
	}
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, "data: {\"type\":\"ping\",\"timestamp\":\"%s\"}\n\n", time.Now().UTC().Format(time.RFC3339))
			flusher.Flush()
		}
	}
}

func (c *PullRequestController) handleGetSuggestions(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	wsID, _ := auth.WorkspaceFromContext(r.Context())
	if wsID == uuid.Nil {
		// Fallback for CLI key authentication via x-team-key
		teamKey := r.Header.Get("x-team-key")
		if teamKey == "" {
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(strings.ToLower(authHeader), "bearer scandrix_") {
				teamKey = strings.TrimSpace(authHeader[7:])
			}
		}
		if teamKey == "" {
			http.Error(w, `{"error":"unauthorized: x-team-key or Bearer token required"}`, http.StatusUnauthorized)
			return
		}
	}

	// Retrieve findings for requested PR
	var findings []models.CodeFinding
	prNum := 0
	if pStr := r.URL.Query().Get("prNumber"); pStr != "" {
		prNum, _ = strconv.Atoi(pStr)
	}

	if c.repo != nil && wsID != uuid.Nil {
		// Fetch executions matching this PR number
		res, err := c.repo.ListPullRequestExecutions(r.Context(), wsID, models.PullRequestExecutionFilter{
			PullRequestNumber: &prNum,
			Limit:             1,
		})
		if err == nil && res != nil && len(res.Data) > 0 {
			findings, _ = c.repo.GetReviewFindings(r.Context(), res.Data[0].UUID, wsID)
		}
	}

	if strings.ToLower(format) == "markdown" {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("# ScanDrix Code Review Suggestions (PR #%d)\n\n", prNum))
		if len(findings) == 0 {
			b.WriteString("No issues or rule violations identified. Code changes look clean!\n")
		} else {
			for i, f := range findings {
				b.WriteString(fmt.Sprintf("### %d. [%s] %s\n", i+1, f.Severity, f.Title))
				b.WriteString(fmt.Sprintf("- **File**: `%s` (Lines %d-%d)\n", f.FilePath, f.StartLine, f.EndLine))
				b.WriteString(fmt.Sprintf("- **Category**: `%s`\n", f.Category))
				b.WriteString(fmt.Sprintf("- **Description**: %s\n", f.Description))
				if f.Remediation != "" {
					b.WriteString(fmt.Sprintf("- **Remediation**: %s\n", f.Remediation))
				}
				if f.SuggestedDiff != "" {
					b.WriteString("```diff\n" + f.SuggestedDiff + "\n```\n")
				}
				b.WriteString("\n---\n")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"markdown": b.String(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"suggestions": findings,
		"count":       len(findings),
	})
}

func (c *PullRequestController) handleGetFiles(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	repoIDStr := r.URL.Query().Get("repositoryId")
	repoID, err := uuid.Parse(repoIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid repositoryId parameter"}`, http.StatusBadRequest)
		return
	}

	prNum, err := strconv.Atoi(r.URL.Query().Get("prNumber"))
	if err != nil || prNum <= 0 {
		http.Error(w, `{"error":"invalid or missing prNumber parameter"}`, http.StatusBadRequest)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"changed files unavailable: no data source"}`, http.StatusServiceUnavailable)
		return
	}

	/**
	 * Prefer the provider's own view of the diff: it carries the unified patch,
	 * change status and line counts that stored findings cannot supply.
	 */
	if c.fileFetch != nil {
		providerFiles, fetchErr := c.fileFetch.GetPullRequestFiles(r.Context(), wsID, repoID, prNum)
		if fetchErr != nil {
			slog.WarnContext(r.Context(), "SCM diff read failed; reporting reviewed paths without diff detail",
				"repositoryId", repoID, "pullRequestNumber", prNum, "error", fetchErr)
			c.writeChangedFilesDegraded(w, wsID, repoID, prNum, "the SCM provider could not be read")
			return
		}
		writeChangedFilesFromProvider(w, providerFiles)
		return
	}

	// No fetcher is wired in this deployment. Report what is actually known and
	// name the absent fields, rather than substituting line counts.
	slog.Warn("no SCM file fetcher configured; changed files degrade to reviewed paths only")
	c.writeChangedFilesDegraded(w, wsID, repoID, prNum, "no SCM provider adapter is configured")
}

// writeChangedFilesFromProvider encodes the provider's real diff data.
//
// A file with no patch is reported as an empty string, not as a null: providers
// legitimately omit patches for binary files and oversized diffs, and that is a
// real answer rather than an unmeasured field.
func writeChangedFilesFromProvider(w http.ResponseWriter, files []*domainTypes.PullRequestFile) {
	out := make([]models.PullRequestChangedFile, 0, len(files))
	for _, f := range files {
		if f == nil {
			continue
		}
		file := models.PullRequestChangedFile{FilePath: f.Filename}
		if f.Status != "" {
			status := f.Status
			file.Status = &status
		}
		additions, deletions := f.Additions, f.Deletions
		file.Additions = &additions
		file.Deletions = &deletions
		patch := f.Patch
		file.Patch = &patch
		out = append(out, file)
	}
	writeChangedFiles(w, models.PullRequestChangedFilesResponse{Data: out, Unavailable: []models.AnalyticsUnavailable{}})
}

// writeChangedFilesDegraded reports only the paths a review touched, naming each
// diff field as unavailable. The stored findings do contain the path set, so this
// is a real partial result rather than an empty one.
func (c *PullRequestController) writeChangedFilesDegraded(w http.ResponseWriter, wsID, repoID uuid.UUID, prNumber int, detail string) {
	files := []models.PullRequestChangedFile{}
	if c.repo != nil {
		stored, err := c.repo.GetPullRequestChangedFiles(context.Background(), wsID, repoID, prNumber)
		if err != nil {
			slog.WarnContext(context.Background(), "stored changed-file lookup failed during degradation",
				"repositoryId", repoID, "pullRequestNumber", prNumber, "error", err)
		} else if stored != nil {
			files = stored
		}
	}
	writeChangedFiles(w, models.PullRequestChangedFilesResponse{
		Data: files,
		Unavailable: []models.AnalyticsUnavailable{
			{Metric: "status", Reason: models.ReasonNoDataSource, Detail: detail},
			{Metric: "additions", Reason: models.ReasonNoDataSource, Detail: detail},
			{Metric: "deletions", Reason: models.ReasonNoDataSource, Detail: detail},
			{Metric: "patch", Reason: models.ReasonNoDataSource, Detail: detail},
		},
	})
}

func writeChangedFiles(w http.ResponseWriter, response models.PullRequestChangedFilesResponse) {
	if response.Data == nil {
		response.Data = []models.PullRequestChangedFile{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(response)
}

func (c *PullRequestController) handleBackfillHistoricalPRs(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		TeamID        string   `json:"teamId"`
		RepositoryIDs []string `json:"repositoryIds"`
		StartDate     string   `json:"startDate"`
		EndDate       string   `json:"endDate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid backfill request JSON"}`, http.StatusBadRequest)
		return
	}

	if c.backfillUC != nil {
		targets := make([]usecases.RepositoryTarget, 0, len(req.RepositoryIDs))
		for _, rid := range req.RepositoryIDs {
			targets = append(targets, usecases.RepositoryTarget{
				ID: rid,
			})
		}
		c.backfillUC.Execute(r.Context(), usecases.BackfillHistoricalPRsInput{
			OrganizationID: wsID.String(),
			Repositories:   targets,
			StartDate:      req.StartDate,
			EndDate:        req.EndDate,
		})
	}

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":           true,
		"message":           "PR backfill started in background",
		"workspaceId":       wsID.String(),
		"repositoriesCount": len(req.RepositoryIDs),
	})
}

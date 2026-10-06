package controllers

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type reviewAnalyticsRepository interface {
	GetReviewAnalytics(context.Context, uuid.UUID, models.ReviewAnalyticsFilter) (*models.ReviewAnalytics, error)
}

func (c *CockpitController) handleReviewAnalytics(w http.ResponseWriter, r *http.Request) {
	wsID, analytics, ok := c.requireAnalytics(w, r)
	if !ok {
		return
	}
	repo, ok := analytics.(reviewAnalyticsRepository)
	if !ok {
		writeCockpitError(w, 503, "review analytics unavailable")
		return
	}
	filter, ok := parseReviewWindow(w, r)
	if !ok {
		return
	}
	report, err := repo.GetReviewAnalytics(r.Context(), wsID, filter)
	if err != nil {
		writeCockpitError(w, 500, "review analytics could not be loaded")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(report)
}

func parseReviewWindow(w http.ResponseWriter, r *http.Request) (models.ReviewAnalyticsFilter, bool) {
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -30)
	query := r.URL.Query()
	for key, target := range map[string]*time.Time{"start": &start, "end": &end} {
		if value := query.Get(key); value != "" {
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				writeCockpitError(w, 400, "start and end must be RFC3339 timestamps")
				return models.ReviewAnalyticsFilter{}, false
			}
			*target = parsed.UTC()
		}
	}
	if !start.Before(end) || end.Sub(start) > 366*24*time.Hour {
		writeCockpitError(w, 400, "reporting window must be positive and at most 366 days")
		return models.ReviewAnalyticsFilter{}, false
	}
	filter := models.ReviewAnalyticsFilter{Start: start, End: end}
	if value := query.Get("repositoryId"); value != "" {
		id, err := uuid.Parse(value)
		if err != nil || id == uuid.Nil {
			writeCockpitError(w, 400, "invalid repositoryId")
			return models.ReviewAnalyticsFilter{}, false
		}
		filter.RepositoryID = &id
	}
	return filter, true
}

func (c *CockpitController) handleSearchSuggestions(w http.ResponseWriter, r *http.Request) {
	wsID, analytics, ok := c.requireAnalytics(w, r)
	if !ok {
		return
	}
	repo, ok := analytics.(interface {
		SearchReviewSuggestions(context.Context, uuid.UUID, models.SuggestionSearchFilter) (*models.SuggestionSearchResult, error)
	})
	if !ok {
		writeCockpitError(w, 503, "suggestions unavailable")
		return
	}
	window, ok := parseReviewWindow(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	filter := models.SuggestionSearchFilter{ReviewAnalyticsFilter: window, Search: q.Get("q"), Severity: strings.ToUpper(q.Get("severity")), Category: q.Get("category"), Page: 1, Limit: 20}
	if len(filter.Search) > 200 || len(filter.Category) > 100 {
		writeCockpitError(w, 400, "search is too long")
		return
	}
	switch filter.Severity {
	case "", "CRITICAL", "HIGH", "MEDIUM", "LOW", "INFO":
	default:
		writeCockpitError(w, 400, "invalid severity")
		return
	}
	for key, target := range map[string]*int{"page": &filter.Page, "limit": &filter.Limit} {
		if raw := q.Get(key); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 100000 || (key == "limit" && n > 100) {
				writeCockpitError(w, 400, "invalid pagination")
				return
			}
			*target = n
		}
	}
	report, err := repo.SearchReviewSuggestions(r.Context(), wsID, filter)
	if err != nil {
		writeCockpitError(w, 500, "suggestions could not be loaded")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(report)
}

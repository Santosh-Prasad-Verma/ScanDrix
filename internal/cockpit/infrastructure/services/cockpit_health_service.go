package services

import (
	"context"
	"math"
	"sync"
	"time"
)

// CockpitHealth reports the reachability and status of the analytics store.
type CockpitHealth struct {
	Status            string                 `json:"status"` // "ok" | "degraded"
	AnalyticsPostgres AnalyticsPostgresStatus `json:"analytics_postgres"`
}

// AnalyticsPostgresStatus holds connection state details.
type AnalyticsPostgresStatus struct {
	Reachable bool    `json:"reachable"`
	Schema    *string `json:"schema"`
	Error     string  `json:"error,omitempty"`
}

// IngestionRunSummary describes a warehouse ingestion batch.
type IngestionRunSummary struct {
	ID                  string     `json:"id"`
	Source              string     `json:"source"`
	Mode                string     `json:"mode"`
	Status              string     `json:"status"`
	StartedAt           *time.Time `json:"started_at"`
	FinishedAt          *time.Time `json:"finished_at"`
	Scanned             int        `json:"scanned"`
	PRsUpserted         int        `json:"prs_upserted"`
	SuggestionsInserted int        `json:"suggestions_inserted"`
	CommitsInserted     int        `json:"commits_inserted"`
	ErrorsQuarantined   int        `json:"errors_quarantined"`
	MongoMS             *int64     `json:"mongo_ms"`
	WriteMS             *int64     `json:"write_ms"`
	Error               *string    `json:"error"`
}

// IngestionRunsHealth tracks warehouse freshness and pipeline errors.
type IngestionRunsHealth struct {
	Last               *IngestionRunSummary `json:"last"`
	LastOk             *IngestionRunSummary `json:"last_ok"`
	LagHours           *float64             `json:"lag_hours"`
	FailedLast24h      int                  `json:"failed_last_24h"`
	QuarantinedLast24h int                  `json:"quarantined_last_24h"`
}

// DatabasePinger abstracts analytical store ping and health probing.
type DatabasePinger interface {
	Ping(ctx context.Context) error
	GetSchema(ctx context.Context) string
	FetchIngestionRuns(ctx context.Context, source string) ([]IngestionRunSummary, error)
}

// CockpitHealthService provides health checks and ingestion lag tracking.
type CockpitHealthService struct {
	mu     sync.RWMutex
	pinger DatabasePinger
	runs   []IngestionRunSummary
}

// NewCockpitHealthService initializes the health and freshness monitor.
func NewCockpitHealthService(pinger DatabasePinger) *CockpitHealthService {
	return &CockpitHealthService{
		pinger: pinger,
		runs:   make([]IngestionRunSummary, 0),
	}
}

// RecordRun records an ingestion batch run for monitoring.
func (s *CockpitHealthService) RecordRun(run IngestionRunSummary) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs = append(s.runs, run)
}

// Ping verifies connectivity to the analytics database.
func (s *CockpitHealthService) Ping(ctx context.Context) CockpitHealth {
	if s.pinger == nil {
		schema := "analytics"
		return CockpitHealth{
			Status: "ok",
			AnalyticsPostgres: AnalyticsPostgresStatus{
				Reachable: true,
				Schema:    &schema,
			},
		}
	}

	err := s.pinger.Ping(ctx)
	schema := s.pinger.GetSchema(ctx)
	var schemaPtr *string
	if schema != "" {
		schemaPtr = &schema
	}

	if err != nil {
		return CockpitHealth{
			Status: "degraded",
			AnalyticsPostgres: AnalyticsPostgresStatus{
				Reachable: false,
				Schema:    schemaPtr,
				Error:     err.Error(),
			},
		}
	}

	return CockpitHealth{
		Status: "ok",
		AnalyticsPostgres: AnalyticsPostgresStatus{
			Reachable: true,
			Schema:    schemaPtr,
		},
	}
}

// RunsSummary computes ingestion lag and 24-hour error counts.
func (s *CockpitHealthService) RunsSummary(ctx context.Context, source string) IngestionRunsHealth {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if source == "" {
		source = "pull_requests"
	}

	var matched []IngestionRunSummary
	if s.pinger != nil {
		runs, err := s.pinger.FetchIngestionRuns(ctx, source)
		if err == nil {
			matched = runs
		}
	}
	if len(matched) == 0 {
		for _, r := range s.runs {
			if r.Source == source {
				matched = append(matched, r)
			}
		}
	}

	var last *IngestionRunSummary
	var lastOk *IngestionRunSummary
	now := time.Now().UTC()
	cutoff24h := now.Add(-24 * time.Hour)
	failed24h := 0
	quarantined24h := 0

	for i := range matched {
		r := &matched[i]
		if last == nil || (r.StartedAt != nil && last.StartedAt != nil && r.StartedAt.After(*last.StartedAt)) {
			last = r
		}
		if r.Status == "ok" || r.Status == "partial" {
			if lastOk == nil || (r.FinishedAt != nil && lastOk.FinishedAt != nil && r.FinishedAt.After(*lastOk.FinishedAt)) {
				lastOk = r
			}
		}
		if r.StartedAt != nil && r.StartedAt.After(cutoff24h) {
			if r.Status == "failed" {
				failed24h++
			}
			quarantined24h += r.ErrorsQuarantined
		}
	}

	var lagHours *float64
	if lastOk != nil && lastOk.FinishedAt != nil {
		h := math.Max(0, now.Sub(*lastOk.FinishedAt).Hours())
		rounded := math.Round(h*100.0) / 100.0
		lagHours = &rounded
	}

	return IngestionRunsHealth{
		Last:               last,
		LastOk:             lastOk,
		LagHours:           lagHours,
		FailedLast24h:      failed24h,
		QuarantinedLast24h: quarantined24h,
	}
}

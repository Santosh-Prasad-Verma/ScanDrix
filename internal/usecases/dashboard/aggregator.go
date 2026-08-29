package dashboard

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const maxAwaitingLimit = 100

// DashboardAggregator compiles high-performance review metrics and facets.
type DashboardAggregator struct {
	mu      sync.RWMutex
	records []PullRequestRecord
}

func NewDashboardAggregator() *DashboardAggregator {
	return &DashboardAggregator{
		records: make([]PullRequestRecord, 0),
	}
}

// IngestRecord records a completed or enqueued review record.
func (a *DashboardAggregator) IngestRecord(rec PullRequestRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.records = append(a.records, rec)
}

// GetDailyDigest computes today's UTC review throughput and risk posture.
func (a *DashboardAggregator) GetDailyDigest(workspaceID uuid.UUID) DailyDigest {
	a.mu.RLock()
	defer a.mu.RUnlock()

	nowUTC := time.Now().UTC()
	startOfDay := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)
	dateStr := startOfDay.Format("2006-01-02")

	digest := DailyDigest{
		Date: dateStr,
	}

	for _, r := range a.records {
		if r.WorkspaceID != workspaceID {
			continue
		}

		// Awaiting reviews opened today
		if r.Status == "awaiting" && (r.OpenedAt.After(startOfDay) || r.OpenedAt.Equal(startOfDay)) {
			digest.AwaitingReview++
		}

		if r.ReviewedAt != nil && (r.ReviewedAt.After(startOfDay) || r.ReviewedAt.Equal(startOfDay)) {
			if r.Status == "reviewed" {
				digest.ReviewedToday++
				if r.SeverityMax == "critical" || r.SeverityMax == "high" {
					digest.NeedsAttention++
				}
			} else if r.Status == "errored" {
				digest.ErroredToday++
			}
		}
	}

	return digest
}

// GetAwaitingPullRequests lists open PRs needing reviewer assurance.
func (a *DashboardAggregator) GetAwaitingPullRequests(workspaceID uuid.UUID) []AwaitingPullRequest {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var awaiting []AwaitingPullRequest

	for _, r := range a.records {
		if r.WorkspaceID != workspaceID {
			continue
		}
		if r.Status == "awaiting" {
			awaiting = append(awaiting, AwaitingPullRequest{
				PRID:           r.PRID,
				PRNumber:       r.PRNumber,
				Title:          r.Title,
				URL:            r.URL,
				RepositoryName: r.RepoName,
				RepositoryID:   r.RepositoryID,
				Author:         r.Author,
				OpenedAt:       r.OpenedAt,
				HasBlocker:     r.FindingCount > 0,
			})
		}
	}

	// Sort newest first
	sort.Slice(awaiting, func(i, j int) bool {
		return awaiting[i].OpenedAt.After(awaiting[j].OpenedAt)
	})

	if len(awaiting) > maxAwaitingLimit {
		awaiting = awaiting[:maxAwaitingLimit]
	}

	return awaiting
}

// GetFacets compiles multi-dimensional aggregation metrics for dashboard filters.
func (a *DashboardAggregator) GetFacets(workspaceID uuid.UUID) PullRequestFacets {
	a.mu.RLock()
	defer a.mu.RUnlock()

	facets := PullRequestFacets{
		SeverityCounts:  make(map[string]int),
		TopAuthors:      make(map[string]int),
		RepositoryStats: make(map[string]int),
	}

	for _, r := range a.records {
		if r.WorkspaceID != workspaceID {
			continue
		}

		if r.Status == "reviewed" {
			facets.TotalReviewed++
			if r.SeverityMax == "critical" || r.SeverityMax == "high" {
				facets.NeedsAttention++
			} else {
				facets.CleanApproved++
			}
		} else if r.Status == "errored" {
			facets.Errored++
		}

		// Count severity
		if r.SeverityMax != "" && r.SeverityMax != "clean" {
			sevKey := strings.ToLower(r.SeverityMax)
			facets.SeverityCounts[sevKey]++
		}

		// Count authors
		if r.Author != "" {
			facets.TopAuthors[r.Author]++
		}

		// Count repositories
		if r.RepoName != "" {
			facets.RepositoryStats[r.RepoName]++
		}
	}

	return facets
}

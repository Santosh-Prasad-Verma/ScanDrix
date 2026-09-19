package services

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/cockpit/application"
	"github.com/scandrix/backend/internal/cockpit/domain"
)

// PullRequestRecord models a pull request for code health calculations.
type PullRequestRecord struct {
	ID             string
	OrganizationID string
	RepositoryID   string
	RepositoryName string
	Number         int
	Title          string
	IsBugFix       bool
	Status         string // "closed" | "open"
	ClosedAt       time.Time
	CreatedAt      time.Time
}

// InMemoryCockpitCodeHealthService computes code health, debt, and bug ratio metrics.
type InMemoryCockpitCodeHealthService struct {
	mu           sync.RWMutex
	prs          []PullRequestRecord
	reviewAnalytics domain.CockpitReviewAnalyticsService
}

// NewInMemoryCockpitCodeHealthService initializes the code health service.
func NewInMemoryCockpitCodeHealthService(
	reviewAnalytics domain.CockpitReviewAnalyticsService,
) *InMemoryCockpitCodeHealthService {
	return &InMemoryCockpitCodeHealthService{
		prs:             make([]PullRequestRecord, 0),
		reviewAnalytics: reviewAnalytics,
	}
}

// AddPullRequest records a pull request for testing and metric calculations.
func (s *InMemoryCockpitCodeHealthService) AddPullRequest(pr PullRequestRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prs = append(s.prs, pr)
}

func (s *InMemoryCockpitCodeHealthService) GetSuggestionsByCategory(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.SuggestionCategoryCount, error) {
	byCat, err := s.reviewAnalytics.GetImplementationRateByCategory(ctx, q)
	if err != nil {
		return nil, err
	}

	var results []domain.SuggestionCategoryCount
	for _, c := range byCat {
		results = append(results, domain.SuggestionCategoryCount{
			Category: c.Category,
			Count:    c.Sent,
		})
	}
	return results, nil
}

func (s *InMemoryCockpitCodeHealthService) GetSuggestionsByRepository(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.RepositorySuggestions, error) {
	healthRows, err := s.reviewAnalytics.GetRepositoriesHealth(ctx, q)
	if err != nil {
		return nil, err
	}

	var results []domain.RepositorySuggestions
	for _, r := range healthRows {
		repoQ := q
		repoQ.Repository = r.Repository
		byCat, _ := s.reviewAnalytics.GetImplementationRateByCategory(ctx, repoQ)

		var catCounts []domain.SuggestionCategoryCount
		for _, c := range byCat {
			catCounts = append(catCounts, domain.SuggestionCategoryCount{
				Category: c.Category,
				Count:    c.Sent,
			})
		}

		results = append(results, domain.RepositorySuggestions{
			Repository: r.Repository,
			TotalCount: r.SuggestionsSent,
			Categories: catCounts,
		})
	}

	return results, nil
}

func (s *InMemoryCockpitCodeHealthService) GetBugRatioChart(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.BugRatioRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	start, err := time.Parse("2006-01-02", q.StartDate)
	if err != nil {
		return nil, err
	}
	end, err := time.Parse("2006-01-02", q.EndDate)
	if err != nil {
		return nil, err
	}
	end = end.Add(24*time.Hour - time.Nanosecond)

	type weekAccumulator struct {
		total  int
		bugFix int
	}

	weeks := make(map[string]*weekAccumulator)

	for _, pr := range s.prs {
		if pr.OrganizationID != q.OrganizationID {
			continue
		}
		if q.Repository != "" && pr.RepositoryName != q.Repository && pr.RepositoryID != q.Repository {
			continue
		}
		if pr.ClosedAt.IsZero() || pr.ClosedAt.Before(start) || pr.ClosedAt.After(end) {
			continue
		}

		weekday := pr.ClosedAt.Weekday()
		daysToMonday := (int(weekday) + 6) % 7
		weekStart := pr.ClosedAt.AddDate(0, 0, -daysToMonday).Format("2006-01-02")

		w, exists := weeks[weekStart]
		if !exists {
			w = &weekAccumulator{}
			weeks[weekStart] = w
		}

		w.total++
		isBug := pr.IsBugFix ||
			strings.Contains(strings.ToLower(pr.Title), "fix") ||
			strings.Contains(strings.ToLower(pr.Title), "bug")
		if isBug {
			w.bugFix++
		}
	}

	var results []domain.BugRatioRow
	for weekStart, w := range weeks {
		ratio := 0.0
		if w.total > 0 {
			ratio = round2(float64(w.bugFix) / float64(w.total))
		}
		results = append(results, domain.BugRatioRow{
			WeekStart: weekStart,
			TotalPRs:  w.total,
			BugFixPRs: w.bugFix,
			Ratio:     ratio,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].WeekStart < results[j].WeekStart
	})

	return results, nil
}

func (s *InMemoryCockpitCodeHealthService) GetBugRatioHighlight(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) (*domain.BugRatioHighlight, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prevPeriod, err := application.ComputePreviousPeriod(q.StartDate, q.EndDate)
	if err != nil {
		return nil, err
	}

	computePeriodData := func(startStr, endStr string) (domain.BugRatioData, error) {
		start, err := time.Parse("2006-01-02", startStr)
		if err != nil {
			return domain.BugRatioData{}, err
		}
		end, err := time.Parse("2006-01-02", endStr)
		if err != nil {
			return domain.BugRatioData{}, err
		}
		end = end.Add(24*time.Hour - time.Nanosecond)

		total, bugFix := 0, 0
		for _, pr := range s.prs {
			if pr.OrganizationID != q.OrganizationID {
				continue
			}
			if q.Repository != "" && pr.RepositoryName != q.Repository && pr.RepositoryID != q.Repository {
				continue
			}
			if pr.ClosedAt.IsZero() || pr.ClosedAt.Before(start) || pr.ClosedAt.After(end) {
				continue
			}

			total++
			if pr.IsBugFix ||
				strings.Contains(strings.ToLower(pr.Title), "fix") ||
				strings.Contains(strings.ToLower(pr.Title), "bug") {
				bugFix++
			}
		}

		ratio := 0.0
		if total > 0 {
			ratio = round2(float64(bugFix) / float64(total))
		}
		return domain.BugRatioData{
			TotalPRs:  total,
			BugFixPRs: bugFix,
			Ratio:     ratio,
		}, nil
	}

	cur, err := computePeriodData(q.StartDate, q.EndDate)
	if err != nil {
		return nil, err
	}
	prev, err := computePeriodData(prevPeriod.StartDate, prevPeriod.EndDate)
	if err != nil {
		return nil, err
	}

	comp := application.ComputeTrend(cur.Ratio, prev.Ratio, "down")

	return &domain.BugRatioHighlight{
		CurrentPeriod:  cur,
		PreviousPeriod: prev,
		Comparison:     comp,
	}, nil
}

func (s *InMemoryCockpitCodeHealthService) GetImplementationRate(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) (*domain.SuggestionsImplementationRate, error) {
	weekly, err := s.reviewAnalytics.GetImplementationRateWeekly(ctx, q)
	if err != nil {
		return nil, err
	}

	totalSent, totalImpl := 0, 0
	for _, w := range weekly {
		totalSent += w.Sent
		totalImpl += w.Implemented
	}

	rate := 0.0
	if totalSent > 0 {
		rate = round2(float64(totalImpl) / float64(totalSent))
	}

	return &domain.SuggestionsImplementationRate{
		SuggestionsSent:        totalSent,
		SuggestionsImplemented: totalImpl,
		ImplementationRate:     rate,
	}, nil
}

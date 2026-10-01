package services

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/cockpit/application"
	"github.com/scandrix/backend/internal/cockpit/domain"
)

// ProductivityPRRecord models pull request details for developer productivity.
type ProductivityPRRecord struct {
	ID             string
	OrganizationID string
	RepositoryID   string
	RepositoryName string
	Number         int
	Author         string
	LinesAdded     int
	LinesDeleted   int
	CreatedAt      time.Time
	FirstCommitAt  time.Time
	FirstReviewAt  time.Time
	MergedAt       time.Time
	ClosedAt       time.Time
}

// InMemoryCockpitDeveloperProductivityService computes cycle velocity, sizing, and productivity metrics.
type InMemoryCockpitDeveloperProductivityService struct {
	mu  sync.RWMutex
	prs []ProductivityPRRecord
}

// NewInMemoryCockpitDeveloperProductivityService initializes the developer productivity service.
func NewInMemoryCockpitDeveloperProductivityService() *InMemoryCockpitDeveloperProductivityService {
	return &InMemoryCockpitDeveloperProductivityService{
		prs: make([]ProductivityPRRecord, 0),
	}
}

// AddPR records a PR record for productivity testing/calculations.
func (s *InMemoryCockpitDeveloperProductivityService) AddPR(pr ProductivityPRRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prs = append(s.prs, pr)
}

func (s *InMemoryCockpitDeveloperProductivityService) GetDeployFrequencyChart(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.DeployFrequencyRow, error) {
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

	weeks := make(map[string]int)

	for _, pr := range s.prs {
		if pr.OrganizationID != q.OrganizationID {
			continue
		}
		if q.Repository != "" && pr.RepositoryName != q.Repository && pr.RepositoryID != q.Repository {
			continue
		}
		if pr.MergedAt.IsZero() || pr.MergedAt.Before(start) || pr.MergedAt.After(end) {
			continue
		}

		weekday := pr.MergedAt.Weekday()
		daysToMonday := (int(weekday) + 6) % 7
		weekStart := pr.MergedAt.AddDate(0, 0, -daysToMonday).Format("2006-01-02")
		weeks[weekStart]++
	}

	var results []domain.DeployFrequencyRow
	for weekStart, count := range weeks {
		results = append(results, domain.DeployFrequencyRow{
			WeekStart: weekStart,
			PRCount:   count,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].WeekStart < results[j].WeekStart
	})

	return results, nil
}

func (s *InMemoryCockpitDeveloperProductivityService) GetDeployFrequencyHighlight(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) (*domain.DeployFrequencyHighlight, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prevPeriod, err := application.ComputePreviousPeriod(q.StartDate, q.EndDate)
	if err != nil {
		return nil, err
	}

	computePeriod := func(startStr, endStr string) (domain.DeployFrequencyData, error) {
		start, err := time.Parse("2006-01-02", startStr)
		if err != nil {
			return domain.DeployFrequencyData{}, err
		}
		end, err := time.Parse("2006-01-02", endStr)
		if err != nil {
			return domain.DeployFrequencyData{}, err
		}
		end = end.Add(24*time.Hour - time.Nanosecond)

		count := 0
		for _, pr := range s.prs {
			if pr.OrganizationID != q.OrganizationID {
				continue
			}
			if q.Repository != "" && pr.RepositoryName != q.Repository && pr.RepositoryID != q.Repository {
				continue
			}
			if !pr.MergedAt.IsZero() && !pr.MergedAt.Before(start) && !pr.MergedAt.After(end) {
				count++
			}
		}

		days := end.Sub(start).Hours() / 24.0
		weeks := math.Max(days/7.0, 1.0)
		avg := round2(float64(count) / weeks)

		return domain.DeployFrequencyData{
			TotalDeployments: count,
			AveragePerWeek:   avg,
		}, nil
	}

	cur, err := computePeriod(q.StartDate, q.EndDate)
	if err != nil {
		return nil, err
	}
	prev, err := computePeriod(prevPeriod.StartDate, prevPeriod.EndDate)
	if err != nil {
		return nil, err
	}

	comp := application.ComputeTrend(float64(cur.TotalDeployments), float64(prev.TotalDeployments), "up")

	return &domain.DeployFrequencyHighlight{
		CurrentPeriod:  cur,
		PreviousPeriod: prev,
		Comparison:     comp,
	}, nil
}

func (s *InMemoryCockpitDeveloperProductivityService) GetLeadTimeChart(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.LeadTimeRow, error) {
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

	weeks := make(map[string][]float64)

	for _, pr := range s.prs {
		if pr.OrganizationID != q.OrganizationID {
			continue
		}
		if q.Repository != "" && pr.RepositoryName != q.Repository && pr.RepositoryID != q.Repository {
			continue
		}
		if pr.MergedAt.IsZero() || pr.MergedAt.Before(start) || pr.MergedAt.After(end) {
			continue
		}

		leadTimeMinutes := pr.MergedAt.Sub(pr.CreatedAt).Minutes()
		if leadTimeMinutes < 0 {
			continue
		}

		weekday := pr.MergedAt.Weekday()
		daysToMonday := (int(weekday) + 6) % 7
		weekStart := pr.MergedAt.AddDate(0, 0, -daysToMonday).Format("2006-01-02")
		weeks[weekStart] = append(weeks[weekStart], leadTimeMinutes)
	}

	var results []domain.LeadTimeRow
	for weekStart, times := range weeks {
		sort.Float64s(times)
		p75Idx := int(float64(len(times)) * 0.75)
		if p75Idx >= len(times) {
			p75Idx = len(times) - 1
		}
		p75Min := times[p75Idx]
		p75Hours := round2(p75Min / 60.0)

		results = append(results, domain.LeadTimeRow{
			WeekStart:          weekStart,
			LeadTimeP75Minutes: round2(p75Min),
			LeadTimeP75Hours:   p75Hours,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].WeekStart < results[j].WeekStart
	})

	return results, nil
}

func (s *InMemoryCockpitDeveloperProductivityService) GetLeadTimeHighlight(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) (*domain.LeadTimeHighlight, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prevPeriod, err := application.ComputePreviousPeriod(q.StartDate, q.EndDate)
	if err != nil {
		return nil, err
	}

	computePeriod := func(startStr, endStr string) (domain.LeadTimeData, error) {
		start, err := time.Parse("2006-01-02", startStr)
		if err != nil {
			return domain.LeadTimeData{}, err
		}
		end, err := time.Parse("2006-01-02", endStr)
		if err != nil {
			return domain.LeadTimeData{}, err
		}
		end = end.Add(24*time.Hour - time.Nanosecond)

		var times []float64
		for _, pr := range s.prs {
			if pr.OrganizationID != q.OrganizationID {
				continue
			}
			if q.Repository != "" && pr.RepositoryName != q.Repository && pr.RepositoryID != q.Repository {
				continue
			}
			if !pr.MergedAt.IsZero() && !pr.MergedAt.Before(start) && !pr.MergedAt.After(end) {
				m := pr.MergedAt.Sub(pr.CreatedAt).Minutes()
				if m >= 0 {
					times = append(times, m)
				}
			}
		}

		if len(times) == 0 {
			return domain.LeadTimeData{}, nil
		}

		sort.Float64s(times)
		p75Idx := int(float64(len(times)) * 0.75)
		if p75Idx >= len(times) {
			p75Idx = len(times) - 1
		}
		p75Min := times[p75Idx]

		return domain.LeadTimeData{
			LeadTimeP75Minutes: round2(p75Min),
			LeadTimeP75Hours:   round2(p75Min / 60.0),
		}, nil
	}

	cur, err := computePeriod(q.StartDate, q.EndDate)
	if err != nil {
		return nil, err
	}
	prev, err := computePeriod(prevPeriod.StartDate, prevPeriod.EndDate)
	if err != nil {
		return nil, err
	}

	comp := application.ComputeTrend(cur.LeadTimeP75Hours, prev.LeadTimeP75Hours, "down")

	return &domain.LeadTimeHighlight{
		CurrentPeriod:  cur,
		PreviousPeriod: prev,
		Comparison:     comp,
	}, nil
}

func (s *InMemoryCockpitDeveloperProductivityService) GetPullRequestsByDev(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.PullRequestsByDevRow, error) {
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

	type devKey struct {
		weekStart string
		author    string
	}
	devMap := make(map[devKey]int)

	for _, pr := range s.prs {
		if pr.OrganizationID != q.OrganizationID {
			continue
		}
		if q.Repository != "" && pr.RepositoryName != q.Repository && pr.RepositoryID != q.Repository {
			continue
		}
		if pr.CreatedAt.Before(start) || pr.CreatedAt.After(end) {
			continue
		}

		weekday := pr.CreatedAt.Weekday()
		daysToMonday := (int(weekday) + 6) % 7
		weekStart := pr.CreatedAt.AddDate(0, 0, -daysToMonday).Format("2006-01-02")

		devMap[devKey{weekStart: weekStart, author: pr.Author}]++
	}

	var results []domain.PullRequestsByDevRow
	for k, count := range devMap {
		results = append(results, domain.PullRequestsByDevRow{
			WeekStart: k.weekStart,
			Author:    k.author,
			PRCount:   count,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].WeekStart == results[j].WeekStart {
			return results[i].PRCount > results[j].PRCount
		}
		return results[i].WeekStart < results[j].WeekStart
	})

	return results, nil
}

func (s *InMemoryCockpitDeveloperProductivityService) GetPullRequestSizeHighlight(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) (*domain.PRSizeHighlight, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prevPeriod, err := application.ComputePreviousPeriod(q.StartDate, q.EndDate)
	if err != nil {
		return nil, err
	}

	computePeriod := func(startStr, endStr string) (domain.PRSizeData, error) {
		start, err := time.Parse("2006-01-02", startStr)
		if err != nil {
			return domain.PRSizeData{}, err
		}
		end, err := time.Parse("2006-01-02", endStr)
		if err != nil {
			return domain.PRSizeData{}, err
		}
		end = end.Add(24*time.Hour - time.Nanosecond)

		totalLines := 0
		totalPRs := 0

		for _, pr := range s.prs {
			if pr.OrganizationID != q.OrganizationID {
				continue
			}
			if q.Repository != "" && pr.RepositoryName != q.Repository && pr.RepositoryID != q.Repository {
				continue
			}
			if !pr.CreatedAt.Before(start) && !pr.CreatedAt.After(end) {
				totalLines += (pr.LinesAdded + pr.LinesDeleted)
				totalPRs++
			}
		}

		avg := 0.0
		if totalPRs > 0 {
			avg = round2(float64(totalLines) / float64(totalPRs))
		}

		return domain.PRSizeData{
			AveragePRSize: avg,
			TotalPRs:      totalPRs,
		}, nil
	}

	cur, err := computePeriod(q.StartDate, q.EndDate)
	if err != nil {
		return nil, err
	}
	prev, err := computePeriod(prevPeriod.StartDate, prevPeriod.EndDate)
	if err != nil {
		return nil, err
	}

	comp := application.ComputeTrend(cur.AveragePRSize, prev.AveragePRSize, "down")

	return &domain.PRSizeHighlight{
		CurrentPeriod:  cur,
		PreviousPeriod: prev,
		Comparison:     comp,
	}, nil
}

func (s *InMemoryCockpitDeveloperProductivityService) GetPullRequestSizeChart(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.PullRequestSizeRow, error) {
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

	type sizeAcc struct {
		totalLines int
		totalPRs   int
	}
	weeks := make(map[string]*sizeAcc)

	for _, pr := range s.prs {
		if pr.OrganizationID != q.OrganizationID {
			continue
		}
		if q.Repository != "" && pr.RepositoryName != q.Repository && pr.RepositoryID != q.Repository {
			continue
		}
		if pr.CreatedAt.Before(start) || pr.CreatedAt.After(end) {
			continue
		}

		weekday := pr.CreatedAt.Weekday()
		daysToMonday := (int(weekday) + 6) % 7
		weekStart := pr.CreatedAt.AddDate(0, 0, -daysToMonday).Format("2006-01-02")

		acc, exists := weeks[weekStart]
		if !exists {
			acc = &sizeAcc{}
			weeks[weekStart] = acc
		}
		acc.totalLines += (pr.LinesAdded + pr.LinesDeleted)
		acc.totalPRs++
	}

	var results []domain.PullRequestSizeRow
	for weekStart, acc := range weeks {
		avg := 0.0
		if acc.totalPRs > 0 {
			avg = round2(float64(acc.totalLines) / float64(acc.totalPRs))
		}
		results = append(results, domain.PullRequestSizeRow{
			WeekStart:     weekStart,
			AveragePRSize: avg,
			TotalPRs:      acc.totalPRs,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].WeekStart < results[j].WeekStart
	})

	return results, nil
}

func (s *InMemoryCockpitDeveloperProductivityService) GetLeadTimeBreakdown(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.LeadTimeBreakdownRow, error) {
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

	type breakdownAcc struct {
		prCount    int
		codingMins float64
		pickupMins float64
		reviewMins float64
		totalMins  float64
	}
	weeks := make(map[string]*breakdownAcc)

	for _, pr := range s.prs {
		if pr.OrganizationID != q.OrganizationID {
			continue
		}
		if q.Repository != "" && pr.RepositoryName != q.Repository && pr.RepositoryID != q.Repository {
			continue
		}
		if pr.MergedAt.IsZero() || pr.MergedAt.Before(start) || pr.MergedAt.After(end) {
			continue
		}

		coding := 0.0
		if !pr.FirstCommitAt.IsZero() && pr.CreatedAt.After(pr.FirstCommitAt) {
			coding = pr.CreatedAt.Sub(pr.FirstCommitAt).Minutes()
		}
		pickup := 0.0
		if !pr.FirstReviewAt.IsZero() && pr.FirstReviewAt.After(pr.CreatedAt) {
			pickup = pr.FirstReviewAt.Sub(pr.CreatedAt).Minutes()
		}
		review := 0.0
		if !pr.FirstReviewAt.IsZero() && pr.MergedAt.After(pr.FirstReviewAt) {
			review = pr.MergedAt.Sub(pr.FirstReviewAt).Minutes()
		} else {
			review = pr.MergedAt.Sub(pr.CreatedAt).Minutes()
		}
		total := pr.MergedAt.Sub(pr.CreatedAt).Minutes()

		weekday := pr.MergedAt.Weekday()
		daysToMonday := (int(weekday) + 6) % 7
		weekStart := pr.MergedAt.AddDate(0, 0, -daysToMonday).Format("2006-01-02")

		acc, exists := weeks[weekStart]
		if !exists {
			acc = &breakdownAcc{}
			weeks[weekStart] = acc
		}
		acc.prCount++
		acc.codingMins += coding
		acc.pickupMins += pickup
		acc.reviewMins += review
		acc.totalMins += total
	}

	var results []domain.LeadTimeBreakdownRow
	for weekStart, acc := range weeks {
		cnt := float64(acc.prCount)
		if cnt == 0 {
			cnt = 1
		}
		results = append(results, domain.LeadTimeBreakdownRow{
			WeekStart:         weekStart,
			PRCount:           acc.prCount,
			CodingTimeMinutes: round2(acc.codingMins / cnt),
			CodingTimeHours:   round2((acc.codingMins / cnt) / 60.0),
			PickupTimeMinutes: round2(acc.pickupMins / cnt),
			PickupTimeHours:   round2((acc.pickupMins / cnt) / 60.0),
			ReviewTimeMinutes: round2(acc.reviewMins / cnt),
			ReviewTimeHours:   round2((acc.reviewMins / cnt) / 60.0),
			TotalTimeMinutes:  round2(acc.totalMins / cnt),
			TotalTimeHours:    round2((acc.totalMins / cnt) / 60.0),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].WeekStart < results[j].WeekStart
	})

	return results, nil
}

func (s *InMemoryCockpitDeveloperProductivityService) GetPullRequestsOpenedVsClosed(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.PullRequestsOpenedVsClosedRow, error) {
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

	type flowAcc struct {
		opened int
		closed int
	}
	weeks := make(map[string]*flowAcc)

	for _, pr := range s.prs {
		if pr.OrganizationID != q.OrganizationID {
			continue
		}
		if q.Repository != "" && pr.RepositoryName != q.Repository && pr.RepositoryID != q.Repository {
			continue
		}

		if !pr.CreatedAt.Before(start) && !pr.CreatedAt.After(end) {
			weekday := pr.CreatedAt.Weekday()
			daysToMonday := (int(weekday) + 6) % 7
			weekStart := pr.CreatedAt.AddDate(0, 0, -daysToMonday).Format("2006-01-02")
			acc, exists := weeks[weekStart]
			if !exists {
				acc = &flowAcc{}
				weeks[weekStart] = acc
			}
			acc.opened++
		}

		if !pr.ClosedAt.IsZero() && !pr.ClosedAt.Before(start) && !pr.ClosedAt.After(end) {
			weekday := pr.ClosedAt.Weekday()
			daysToMonday := (int(weekday) + 6) % 7
			weekStart := pr.ClosedAt.AddDate(0, 0, -daysToMonday).Format("2006-01-02")
			acc, exists := weeks[weekStart]
			if !exists {
				acc = &flowAcc{}
				weeks[weekStart] = acc
			}
			acc.closed++
		}
	}

	var results []domain.PullRequestsOpenedVsClosedRow
	for weekStart, acc := range weeks {
		ratio := 0.0
		if acc.closed > 0 {
			ratio = round2(float64(acc.opened) / float64(acc.closed))
		}
		results = append(results, domain.PullRequestsOpenedVsClosedRow{
			WeekStart:   weekStart,
			OpenedCount: acc.opened,
			ClosedCount: acc.closed,
			Ratio:       ratio,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].WeekStart < results[j].WeekStart
	})

	return results, nil
}

func (s *InMemoryCockpitDeveloperProductivityService) GetDeveloperActivity(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.DeveloperActivityRow, error) {
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

	type actKey struct {
		developer string
		date      string
	}
	counts := make(map[actKey]int)

	for _, pr := range s.prs {
		if pr.OrganizationID != q.OrganizationID {
			continue
		}
		if q.Repository != "" && pr.RepositoryName != q.Repository && pr.RepositoryID != q.Repository {
			continue
		}
		if !pr.CreatedAt.Before(start) && !pr.CreatedAt.After(end) {
			dateStr := pr.CreatedAt.Format("2006-01-02")
			counts[actKey{developer: pr.Author, date: dateStr}]++
		}
	}

	var results []domain.DeveloperActivityRow
	for k, cnt := range counts {
		results = append(results, domain.DeveloperActivityRow{
			Developer: k.developer,
			Date:      k.date,
			PRCount:   cnt,
		})
	}

	return results, nil
}

func (s *InMemoryCockpitDeveloperProductivityService) GetCompanyDashboard(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) (*domain.CompanyDashboard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	devMap := make(map[string]int)
	totalPRs := 0

	start, _ := time.Parse("2006-01-02", q.StartDate)
	end, _ := time.Parse("2006-01-02", q.EndDate)
	if !end.IsZero() {
		end = end.Add(24*time.Hour - time.Nanosecond)
	}

	for _, pr := range s.prs {
		if pr.OrganizationID != q.OrganizationID {
			continue
		}
		if !start.IsZero() && pr.CreatedAt.Before(start) {
			continue
		}
		if !end.IsZero() && pr.CreatedAt.After(end) {
			continue
		}
		totalPRs++
		devMap[pr.Author]++
	}

	topDevName := ""
	topDevPRs := 0
	for dev, cnt := range devMap {
		if cnt > topDevPRs {
			topDevPRs = cnt
			topDevName = dev
		}
	}

	dash := &domain.CompanyDashboard{
		OrganizationID: q.OrganizationID,
	}
	dash.Period.StartDate = q.StartDate
	dash.Period.EndDate = q.EndDate
	dash.Metrics.TotalPRs = totalPRs
	dash.Metrics.TopDeveloper = domain.TopDeveloper{
		Name:     topDevName,
		TotalPRs: topDevPRs,
	}
	// Cross-company ranking has no collected cohort data, so it stays absent.
	// It used to be Rank 1 / TotalCompanies 1 / 100.0%, where the percentage
	// divided this company's PRs by themselves. AUDIT_REMEDIATION.md F-41.
	dash.Metrics.Unavailable = append(dash.Metrics.Unavailable,
		"company_ranking.rank:no_data_source",
		"company_ranking.total_companies:no_data_source",
		"company_ranking.percentage_of_total_prs:no_data_source",
		"company_ranking.total_prs_all_companies:no_data_source",
	)

	return dash, nil
}

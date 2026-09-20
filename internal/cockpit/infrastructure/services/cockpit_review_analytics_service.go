package services

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/cockpit/application"
	"github.com/scandrix/backend/internal/cockpit/domain"
	automationdomain "github.com/scandrix/backend/internal/automation/domain"
)

const (
	weakestCategoryMinSent   = 5
	explorerDefaultPageSize  = 20
	explorerMaxPageSize      = 100
	ignoredCriticalsMaxItems = 50
)

// SuggestionRecord represents an underlying analytical suggestion row.
type SuggestionRecord struct {
	ID                   string
	OrganizationID       string
	PullRequestID        string
	RepositoryID         string
	RepositoryName       string
	FilePath             string
	Category             string
	Severity             string
	ImplementationStatus string // "implemented" | "partially_implemented" | "not_implemented"
	DeliveryStatus       string // "sent" | "ignored"
	BrokenRuleID         string
	ThumbsUp             int
	ThumbsDown           int
	Summary              string
	ExistingCode         string
	ImprovedCode         string
	Language             string
	PRNumber             int
	CommentID            int64
	PRStatus             string    // "closed" | "open"
	PRClosedAt           time.Time
	CreatedAt            time.Time
}

// ReviewExecutionRecord models an analytical execution record for operational metrics.
type ReviewExecutionRecord struct {
	ID             string
	OrganizationID string
	RepositoryID   string
	PRNumber       int
	Status         automationdomain.AutomationStatus // "success", "error", "skipped"
	CreatedAt      time.Time
}

// InMemoryCockpitReviewAnalyticsService provides analytics aggregations.
type InMemoryCockpitReviewAnalyticsService struct {
	mu          sync.RWMutex
	suggestions []SuggestionRecord
	executions  []ReviewExecutionRecord
}

// NewInMemoryCockpitReviewAnalyticsService initializes the review analytics service.
func NewInMemoryCockpitReviewAnalyticsService() *InMemoryCockpitReviewAnalyticsService {
	return &InMemoryCockpitReviewAnalyticsService{
		suggestions: make([]SuggestionRecord, 0),
		executions:  make([]ReviewExecutionRecord, 0),
	}
}

// AddSuggestion records a suggestion for analytics testing/ingestion.
func (s *InMemoryCockpitReviewAnalyticsService) AddSuggestion(sugg SuggestionRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.suggestions = append(s.suggestions, sugg)
}

// AddExecution records an execution for operational metrics.
func (s *InMemoryCockpitReviewAnalyticsService) AddExecution(exec ReviewExecutionRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executions = append(s.executions, exec)
}

func round2(v float64) float64 {
	return math.Round(v*100.0) / 100.0
}

func calcRate(sent, implemented int) float64 {
	if sent <= 0 {
		return 0
	}
	return round2(float64(implemented) / float64(sent))
}

func isImplemented(status string) bool {
	return status == "implemented" || status == "partially_implemented"
}

func (s *InMemoryCockpitReviewAnalyticsService) filterClosedPRSuggestions(
	q domain.CockpitRangeQuery,
) ([]SuggestionRecord, error) {
	start, err := time.Parse("2006-01-02", q.StartDate)
	if err != nil {
		return nil, err
	}
	end, err := time.Parse("2006-01-02", q.EndDate)
	if err != nil {
		return nil, err
	}
	end = end.Add(24*time.Hour - time.Nanosecond)

	var matched []SuggestionRecord
	for _, item := range s.suggestions {
		if item.OrganizationID != q.OrganizationID {
			continue
		}
		if q.Repository != "" && item.RepositoryName != q.Repository && item.RepositoryID != q.Repository {
			continue
		}
		if item.PRStatus != "closed" || item.PRClosedAt.IsZero() {
			continue
		}
		if item.PRClosedAt.Before(start) || item.PRClosedAt.After(end) {
			continue
		}
		if item.DeliveryStatus != "sent" {
			continue
		}
		matched = append(matched, item)
	}

	return matched, nil
}

func (s *InMemoryCockpitReviewAnalyticsService) GetImplementationRateWeekly(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.ImplementationRateWeeklyRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched, err := s.filterClosedPRSuggestions(q)
	if err != nil {
		return nil, err
	}

	weeks := make(map[string]*domain.ImplementationRateWeeklyRow)

	for _, item := range matched {
		// Calculate week start (Monday)
		weekday := item.PRClosedAt.Weekday()
		daysToMonday := (int(weekday) + 6) % 7
		weekStart := item.PRClosedAt.AddDate(0, 0, -daysToMonday).Format("2006-01-02")

		w, exists := weeks[weekStart]
		if !exists {
			w = &domain.ImplementationRateWeeklyRow{
				WeekStart:  weekStart,
				BySeverity: make(map[string]domain.ImplementationRateBreakdown),
			}
			weeks[weekStart] = w
		}

		w.Sent++
		impl := isImplemented(item.ImplementationStatus)
		if impl {
			w.Implemented++
		}

		sev := strings.ToLower(item.Severity)
		if sev == "" {
			sev = "unknown"
		}

		sevData := w.BySeverity[sev]
		sevData.Sent++
		if impl {
			sevData.Implemented++
		}
		sevData.Rate = calcRate(sevData.Sent, sevData.Implemented)
		w.BySeverity[sev] = sevData
	}

	var results []domain.ImplementationRateWeeklyRow
	for _, w := range weeks {
		w.Rate = calcRate(w.Sent, w.Implemented)
		results = append(results, *w)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].WeekStart < results[j].WeekStart
	})

	return results, nil
}

func (s *InMemoryCockpitReviewAnalyticsService) GetImplementationRateByCategory(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.ImplementationRateByCategoryRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched, err := s.filterClosedPRSuggestions(q)
	if err != nil {
		return nil, err
	}

	cats := make(map[string]*domain.ImplementationRateByCategoryRow)
	for _, item := range matched {
		cat := strings.ToLower(item.Category)
		if cat == "" {
			cat = "other"
		}

		c, exists := cats[cat]
		if !exists {
			c = &domain.ImplementationRateByCategoryRow{Category: cat}
			cats[cat] = c
		}
		c.Sent++
		if isImplemented(item.ImplementationStatus) {
			c.Implemented++
		}
	}

	var results []domain.ImplementationRateByCategoryRow
	for _, c := range cats {
		c.Rate = calcRate(c.Sent, c.Implemented)
		results = append(results, *c)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Sent > results[j].Sent
	})

	return results, nil
}

func (s *InMemoryCockpitReviewAnalyticsService) GetImplementationRateBySeverity(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.ImplementationRateBySeverityRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched, err := s.filterClosedPRSuggestions(q)
	if err != nil {
		return nil, err
	}

	sevs := make(map[string]*domain.ImplementationRateBySeverityRow)
	for _, item := range matched {
		sev := strings.ToLower(item.Severity)
		if sev == "" {
			sev = "unknown"
		}

		row, exists := sevs[sev]
		if !exists {
			row = &domain.ImplementationRateBySeverityRow{Severity: sev}
			sevs[sev] = row
		}

		row.Sent++
		impl := isImplemented(item.ImplementationStatus)
		if impl {
			row.Implemented++
		}

		// Native calculation (no custom rule attached)
		if item.BrokenRuleID == "" {
			row.NativeSent++
			if impl {
				row.NativeImplemented++
			}
		}
	}

	var results []domain.ImplementationRateBySeverityRow
	for _, r := range sevs {
		r.Rate = calcRate(r.Sent, r.Implemented)
		r.NativeRate = calcRate(r.NativeSent, r.NativeImplemented)
		results = append(results, *r)
	}

	return results, nil
}

func (s *InMemoryCockpitReviewAnalyticsService) GetIgnoredCriticals(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) (*domain.IgnoredCriticalsHighlight, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched, err := s.filterClosedPRSuggestions(q)
	if err != nil {
		return nil, err
	}

	var items []domain.IgnoredCriticalItem
	for _, item := range matched {
		if strings.ToLower(item.Severity) == "critical" && !isImplemented(item.ImplementationStatus) {
			closedAtStr := item.PRClosedAt.Format(time.RFC3339)
			repoName := item.RepositoryName
			filePath := item.FilePath
			cat := item.Category
			summary := item.Summary
			prNum := item.PRNumber

			items = append(items, domain.IgnoredCriticalItem{
				SuggestionID:  item.ID,
				Repository:    &repoName,
				FilePath:      &filePath,
				Category:      &cat,
				Summary:       &summary,
				PullRequestID: item.PullRequestID,
				PRNumber:      &prNum,
				PRClosedAt:    &closedAtStr,
			})
			if len(items) >= ignoredCriticalsMaxItems {
				break
			}
		}
	}

	return &domain.IgnoredCriticalsHighlight{
		Count: len(items),
		Items: items,
	}, nil
}

func (s *InMemoryCockpitReviewAnalyticsService) GetRepositoriesHealth(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.RepositoryHealthRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched, err := s.filterClosedPRSuggestions(q)
	if err != nil {
		return nil, err
	}

	type repoAccumulator struct {
		repo        string
		prs         map[int]bool
		sent        int
		implemented int
		up          int
		down        int
		byCategory  map[string]*domain.ImplementationRateBreakdown
	}

	repos := make(map[string]*repoAccumulator)
	for _, item := range matched {
		repoKey := item.RepositoryName
		if repoKey == "" {
			repoKey = item.RepositoryID
		}

		acc, exists := repos[repoKey]
		if !exists {
			acc = &repoAccumulator{
				repo:       repoKey,
				prs:        make(map[int]bool),
				byCategory: make(map[string]*domain.ImplementationRateBreakdown),
			}
			repos[repoKey] = acc
		}

		acc.prs[item.PRNumber] = true
		acc.sent++
		impl := isImplemented(item.ImplementationStatus)
		if impl {
			acc.implemented++
		}
		acc.up += item.ThumbsUp
		acc.down += item.ThumbsDown

		cat := strings.ToLower(item.Category)
		if cat != "" {
			b := acc.byCategory[cat]
			if b == nil {
				b = &domain.ImplementationRateBreakdown{}
				acc.byCategory[cat] = b
			}
			b.Sent++
			if impl {
				b.Implemented++
			}
		}
	}

	var results []domain.RepositoryHealthRow
	for _, acc := range repos {
		row := domain.RepositoryHealthRow{
			Repository:             acc.repo,
			PRsReviewed:            len(acc.prs),
			SuggestionsSent:        acc.sent,
			SuggestionsImplemented: acc.implemented,
			ImplementationRate:     calcRate(acc.sent, acc.implemented),
			ThumbsUp:               acc.up,
			ThumbsDown:             acc.down,
		}

		// Find weakest category
		var weakestCat string
		weakestRate := 1.0
		weakestSent := 0
		for cat, b := range acc.byCategory {
			if b.Sent >= weakestCategoryMinSent {
				r := calcRate(b.Sent, b.Implemented)
				if r <= weakestRate {
					weakestRate = r
					weakestCat = cat
					weakestSent = b.Sent
				}
			}
		}

		if weakestCat != "" {
			row.WeakestCategory = &domain.WeakestCategory{
				Category: weakestCat,
				Rate:     weakestRate,
				Sent:     weakestSent,
			}
		}

		results = append(results, row)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].SuggestionsSent > results[j].SuggestionsSent
	})

	return results, nil
}

func (s *InMemoryCockpitReviewAnalyticsService) GetNegativeFeedbackByCategory(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.NegativeFeedbackByCategoryRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched, err := s.filterClosedPRSuggestions(q)
	if err != nil {
		return nil, err
	}

	cats := make(map[string]*domain.NegativeFeedbackByCategoryRow)
	for _, item := range matched {
		cat := strings.ToLower(item.Category)
		if cat == "" {
			cat = "other"
		}

		c, exists := cats[cat]
		if !exists {
			c = &domain.NegativeFeedbackByCategoryRow{Category: cat}
			cats[cat] = c
		}
		c.ThumbsUp += item.ThumbsUp
		c.ThumbsDown += item.ThumbsDown
	}

	var results []domain.NegativeFeedbackByCategoryRow
	for _, c := range cats {
		results = append(results, *c)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ThumbsDown > results[j].ThumbsDown
	})

	return results, nil
}

func (s *InMemoryCockpitReviewAnalyticsService) GetNegativeFeedbackWeekly(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.NegativeFeedbackWeeklyRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched, err := s.filterClosedPRSuggestions(q)
	if err != nil {
		return nil, err
	}

	weeks := make(map[string]*domain.NegativeFeedbackWeeklyRow)
	for _, item := range matched {
		weekday := item.PRClosedAt.Weekday()
		daysToMonday := (int(weekday) + 6) % 7
		weekStart := item.PRClosedAt.AddDate(0, 0, -daysToMonday).Format("2006-01-02")

		w, exists := weeks[weekStart]
		if !exists {
			w = &domain.NegativeFeedbackWeeklyRow{WeekStart: weekStart}
			weeks[weekStart] = w
		}
		w.ThumbsUp += item.ThumbsUp
		w.ThumbsDown += item.ThumbsDown
	}

	var results []domain.NegativeFeedbackWeeklyRow
	for _, w := range weeks {
		results = append(results, *w)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].WeekStart < results[j].WeekStart
	})

	return results, nil
}

func (s *InMemoryCockpitReviewAnalyticsService) GetNegativeVoteRateHighlight(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) (*domain.NegativeVoteRateHighlight, error) {
	curMatched, err := s.filterClosedPRSuggestions(q)
	if err != nil {
		return nil, err
	}

	prevPeriod, err := application.ComputePreviousPeriod(q.StartDate, q.EndDate)
	if err != nil {
		return nil, err
	}
	prevQ := q
	prevQ.StartDate = prevPeriod.StartDate
	prevQ.EndDate = prevPeriod.EndDate

	prevMatched, err := s.filterClosedPRSuggestions(prevQ)
	if err != nil {
		return nil, err
	}

	calcVoteData := func(items []SuggestionRecord) domain.NegativeVoteData {
		up, down := 0, 0
		for _, it := range items {
			up += it.ThumbsUp
			down += it.ThumbsDown
		}
		rate := 0.0
		if up+down > 0 {
			rate = round2(float64(down) / float64(up+down))
		}
		return domain.NegativeVoteData{
			ThumbsUp:     up,
			ThumbsDown:   down,
			NegativeRate: rate,
		}
	}

	curData := calcVoteData(curMatched)
	prevData := calcVoteData(prevMatched)
	comp := application.ComputeTrend(curData.NegativeRate, prevData.NegativeRate, "down")

	return &domain.NegativeVoteRateHighlight{
		CurrentPeriod:  curData,
		PreviousPeriod: prevData,
		Comparison:     comp,
	}, nil
}

func (s *InMemoryCockpitReviewAnalyticsService) GetReviewOperationalMetrics(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) (*domain.ReviewOperationalMetrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prevPeriod, err := application.ComputePreviousPeriod(q.StartDate, q.EndDate)
	if err != nil {
		return nil, err
	}

	computePeriod := func(startStr, endStr string) (domain.ReviewOperationalMetricsPeriod, error) {
		start, err := time.Parse("2006-01-02", startStr)
		if err != nil {
			return domain.ReviewOperationalMetricsPeriod{}, err
		}
		end, err := time.Parse("2006-01-02", endStr)
		if err != nil {
			return domain.ReviewOperationalMetricsPeriod{}, err
		}
		end = end.Add(24*time.Hour - time.Nanosecond)

		prs := make(map[int]bool)
		total, succ, errs, skipped := 0, 0, 0, 0

		for _, ex := range s.executions {
			if ex.OrganizationID != q.OrganizationID {
				continue
			}
			if ex.CreatedAt.Before(start) || ex.CreatedAt.After(end) {
				continue
			}
			prs[ex.PRNumber] = true
			total++
			switch ex.Status {
			case automationdomain.StatusSuccess:
				succ++
			case automationdomain.StatusError:
				errs++
			case automationdomain.StatusSkipped:
				skipped++
			}
		}

		p := domain.ReviewOperationalMetricsPeriod{
			ProcessedPRs:      len(prs),
			ProcessedReviews:  total,
			SuccessfulReviews: succ,
			ErrorReviews:      errs,
			SkippedReviews:    skipped,
		}
		if total > 0 {
			p.SuccessRate = round2(float64(succ) / float64(total))
			p.ErrorRate = round2(float64(errs) / float64(total))
			p.SkippedRate = round2(float64(skipped) / float64(total))
		}
		return p, nil
	}

	cur, err := computePeriod(q.StartDate, q.EndDate)
	if err != nil {
		return nil, err
	}
	prev, err := computePeriod(prevPeriod.StartDate, prevPeriod.EndDate)
	if err != nil {
		return nil, err
	}

	compPRs := application.ComputeTrend(float64(cur.ProcessedPRs), float64(prev.ProcessedPRs), "up")
	compReviews := application.ComputeTrend(float64(cur.ProcessedReviews), float64(prev.ProcessedReviews), "up")
	compSucc := application.ComputeTrend(cur.SuccessRate, prev.SuccessRate, "up")
	compErr := application.ComputeTrend(cur.ErrorRate, prev.ErrorRate, "down")
	compSkip := application.ComputeTrend(cur.SkippedRate, prev.SkippedRate, "down")

	res := &domain.ReviewOperationalMetrics{
		CurrentPeriod:  cur,
		PreviousPeriod: prev,
	}
	res.Comparison.ProcessedPRs = domain.ReviewOperationalMetricComparison{
		PercentageChange: compPRs.PercentageChange,
		Trend:            compPRs.Trend,
	}
	res.Comparison.ProcessedReviews = domain.ReviewOperationalMetricComparison{
		PercentageChange: compReviews.PercentageChange,
		Trend:            compReviews.Trend,
	}
	res.Comparison.SuccessRate = domain.ReviewOperationalRateComparison{
		PercentageChange:      compSucc.PercentageChange,
		Trend:                 compSucc.Trend,
		PercentagePointChange: round2((cur.SuccessRate - prev.SuccessRate) * 100.0),
	}
	res.Comparison.ErrorRate = domain.ReviewOperationalRateComparison{
		PercentageChange:      compErr.PercentageChange,
		Trend:                 compErr.Trend,
		PercentagePointChange: round2((cur.ErrorRate - prev.ErrorRate) * 100.0),
	}
	res.Comparison.SkippedRate = domain.ReviewOperationalRateComparison{
		PercentageChange:      compSkip.PercentageChange,
		Trend:                 compSkip.Trend,
		PercentagePointChange: round2((cur.SkippedRate - prev.SkippedRate) * 100.0),
	}

	return res, nil
}

func (s *InMemoryCockpitReviewAnalyticsService) GetReviewOperationalMetricsWeekly(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.ReviewOperationalMetricsWeeklyRow, error) {
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

	type weekAcc struct {
		prs     map[int]bool
		total   int
		succ    int
		errs    int
		skipped int
	}

	weeks := make(map[string]*weekAcc)
	for _, ex := range s.executions {
		if ex.OrganizationID != q.OrganizationID {
			continue
		}
		if ex.CreatedAt.Before(start) || ex.CreatedAt.After(end) {
			continue
		}
		weekday := ex.CreatedAt.Weekday()
		daysToMonday := (int(weekday) + 6) % 7
		weekStart := ex.CreatedAt.AddDate(0, 0, -daysToMonday).Format("2006-01-02")

		w, exists := weeks[weekStart]
		if !exists {
			w = &weekAcc{prs: make(map[int]bool)}
			weeks[weekStart] = w
		}
		w.prs[ex.PRNumber] = true
		w.total++
		switch ex.Status {
		case automationdomain.StatusSuccess:
			w.succ++
		case automationdomain.StatusError:
			w.errs++
		case automationdomain.StatusSkipped:
			w.skipped++
		}
	}

	var results []domain.ReviewOperationalMetricsWeeklyRow
	for weekStart, w := range weeks {
		row := domain.ReviewOperationalMetricsWeeklyRow{
			WeekStart: weekStart,
			ReviewOperationalMetricsPeriod: domain.ReviewOperationalMetricsPeriod{
				ProcessedPRs:      len(w.prs),
				ProcessedReviews:  w.total,
				SuccessfulReviews: w.succ,
				ErrorReviews:      w.errs,
				SkippedReviews:    w.skipped,
			},
		}
		if w.total > 0 {
			row.SuccessRate = round2(float64(w.succ) / float64(w.total))
			row.ErrorRate = round2(float64(w.errs) / float64(w.total))
			row.SkippedRate = round2(float64(w.skipped) / float64(w.total))
		}
		results = append(results, row)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].WeekStart < results[j].WeekStart
	})

	return results, nil
}

func (s *InMemoryCockpitReviewAnalyticsService) GetDrixyRulesUsage(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.DrixyRuleUsageRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched, err := s.filterClosedPRSuggestions(q)
	if err != nil {
		return nil, err
	}

	type ruleAcc struct {
		ruleID      string
		triggers    int
		implemented int
		up          int
		down        int
		lastTrigger time.Time
	}

	rules := make(map[string]*ruleAcc)
	for _, item := range matched {
		if item.BrokenRuleID == "" {
			continue
		}
		acc, exists := rules[item.BrokenRuleID]
		if !exists {
			acc = &ruleAcc{ruleID: item.BrokenRuleID}
			rules[item.BrokenRuleID] = acc
		}
		acc.triggers++
		if isImplemented(item.ImplementationStatus) {
			acc.implemented++
		}
		acc.up += item.ThumbsUp
		acc.down += item.ThumbsDown
		if item.CreatedAt.After(acc.lastTrigger) {
			acc.lastTrigger = item.CreatedAt
		}
	}

	var results []domain.DrixyRuleUsageRow
	for _, acc := range rules {
		var lastStr *string
		if !acc.lastTrigger.IsZero() {
			formatted := acc.lastTrigger.Format(time.RFC3339)
			lastStr = &formatted
		}
		results = append(results, domain.DrixyRuleUsageRow{
			RuleID:          acc.ruleID,
			Triggers:        acc.triggers,
			Implemented:     acc.implemented,
			Rate:            calcRate(acc.triggers, acc.implemented),
			ThumbsUp:        acc.up,
			ThumbsDown:      acc.down,
			LastTriggeredAt: lastStr,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Triggers > results[j].Triggers
	})

	return results, nil
}

func (s *InMemoryCockpitReviewAnalyticsService) GetReviewQualityByRuleGroup(
	ctx context.Context,
	q domain.CockpitRangeQuery,
) ([]domain.ReviewQualityByRuleGroupRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched, err := s.filterClosedPRSuggestions(q)
	if err != nil {
		return nil, err
	}

	var drixySent, drixyImpl, drixyUp, drixyDown int
	var genSent, genImpl, genUp, genDown int

	for _, item := range matched {
		impl := isImplemented(item.ImplementationStatus)
		if item.BrokenRuleID != "" {
			drixySent++
			if impl {
				drixyImpl++
			}
			drixyUp += item.ThumbsUp
			drixyDown += item.ThumbsDown
		} else {
			genSent++
			if impl {
				genImpl++
			}
			genUp += item.ThumbsUp
			genDown += item.ThumbsDown
		}
	}

	return []domain.ReviewQualityByRuleGroupRow{
		{
			Group:       "drixy_rules",
			Sent:        drixySent,
			Implemented: drixyImpl,
			Rate:        calcRate(drixySent, drixyImpl),
			ThumbsUp:    drixyUp,
			ThumbsDown:  drixyDown,
		},
		{
			Group:       "general",
			Sent:        genSent,
			Implemented: genImpl,
			Rate:        calcRate(genSent, genImpl),
			ThumbsUp:    genUp,
			ThumbsDown:  genDown,
		},
	}, nil
}

func (s *InMemoryCockpitReviewAnalyticsService) GetRepositoryNames(
	ctx context.Context,
	organizationID string,
) (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make(map[string]string)
	for _, item := range s.suggestions {
		if item.OrganizationID == organizationID && item.RepositoryID != "" && item.RepositoryName != "" {
			names[item.RepositoryID] = item.RepositoryName
		}
	}
	return names, nil
}

func (s *InMemoryCockpitReviewAnalyticsService) SearchSuggestions(
	ctx context.Context,
	q domain.SuggestionsExplorerQuery,
) (*domain.SuggestionsExplorerResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	start, _ := time.Parse("2006-01-02", q.StartDate)
	end, _ := time.Parse("2006-01-02", q.EndDate)
	if !end.IsZero() {
		end = end.Add(24*time.Hour - time.Nanosecond)
	}

	var matched []domain.SuggestionsExplorerItem

	for _, item := range s.suggestions {
		if item.OrganizationID != q.OrganizationID {
			continue
		}
		if !start.IsZero() && item.CreatedAt.Before(start) {
			continue
		}
		if !end.IsZero() && item.CreatedAt.After(end) {
			continue
		}
		if q.Repository != nil && *q.Repository != "" {
			if item.RepositoryName != *q.Repository && item.RepositoryID != *q.Repository {
				continue
			}
		}
		if q.Category != nil && *q.Category != "" && !strings.EqualFold(item.Category, *q.Category) {
			continue
		}
		if q.Severity != nil && *q.Severity != "" && !strings.EqualFold(item.Severity, *q.Severity) {
			continue
		}
		if q.RuleID != nil && *q.RuleID != "" && item.BrokenRuleID != *q.RuleID {
			continue
		}
		if q.ImplementationStatus != nil && *q.ImplementationStatus != "" && item.ImplementationStatus != *q.ImplementationStatus {
			continue
		}
		if q.Search != nil && *q.Search != "" {
			term := strings.ToLower(*q.Search)
			if !strings.Contains(strings.ToLower(item.Summary), term) &&
				!strings.Contains(strings.ToLower(item.FilePath), term) {
				continue
			}
		}

		repo := item.RepositoryName
		repoID := item.RepositoryID
		filePath := item.FilePath
		cat := item.Category
		sev := item.Severity
		implSt := item.ImplementationStatus
		summary := item.Summary
		existCode := item.ExistingCode
		impCode := item.ImprovedCode
		lang := item.Language
		prNum := item.PRNumber
		commID := item.CommentID
		createdAtStr := item.CreatedAt.Format(time.RFC3339)

		matched = append(matched, domain.SuggestionsExplorerItem{
			SuggestionID:         item.ID,
			Repository:           &repo,
			RepositoryID:         &repoID,
			FilePath:             &filePath,
			Category:             &cat,
			Severity:             &sev,
			ImplementationStatus: &implSt,
			Summary:              &summary,
			ExistingCode:         &existCode,
			ImprovedCode:         &impCode,
			Language:             &lang,
			PullRequestID:        item.PullRequestID,
			PRNumber:             &prNum,
			CommentID:            &commID,
			CreatedAt:            &createdAtStr,
		})
	}

	pageSize := q.PageSize
	if pageSize <= 0 {
		pageSize = explorerDefaultPageSize
	}
	if pageSize > explorerMaxPageSize {
		pageSize = explorerMaxPageSize
	}
	page := q.Page
	if page <= 0 {
		page = 1
	}

	total := len(matched)
	startIdx := (page - 1) * pageSize
	if startIdx >= total {
		return &domain.SuggestionsExplorerResult{
			Total:    total,
			Page:     page,
			PageSize: pageSize,
			Items:    []domain.SuggestionsExplorerItem{},
		}, nil
	}

	endIdx := startIdx + pageSize
	if endIdx > total {
		endIdx = total
	}

	return &domain.SuggestionsExplorerResult{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Items:    matched[startIdx:endIdx],
	}, nil
}

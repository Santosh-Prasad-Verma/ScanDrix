package services

import (
	"context"
	"sort"
	"sync"

	"github.com/scandrix/backend/internal/cockpit/application"
	"github.com/scandrix/backend/internal/cockpit/domain"
	"github.com/scandrix/backend/internal/cockpit/domain/helpers"
)

const (
	maxCategoryRows       = 6
	maxRepoRuleRows       = 6
	maxOrgRulesAttention  = 8
	repoBuildConcurrency = 4
	minFeedbackVotes      = 10
	rankingMinReviews     = 10
	highlightMinReviews   = 10
)

// DrixyRulesService provides rule metadata for reports.
type DrixyRulesService interface {
	GetRuleTitles(ctx context.Context, organizationID string) (map[string]domain.RuleMeta, error)
}

// CockpitReportsService handles aggregation and assembly of executive and repository reports.
type CockpitReportsService struct {
	reviewAnalytics domain.CockpitReviewAnalyticsService
	codeHealth      domain.CockpitCodeHealthService
	productivity    domain.CockpitDeveloperProductivityService
	rulesService    DrixyRulesService
}

// NewCockpitReportsService initializes the reports service.
func NewCockpitReportsService(
	reviewAnalytics domain.CockpitReviewAnalyticsService,
	codeHealth domain.CockpitCodeHealthService,
	productivity domain.CockpitDeveloperProductivityService,
	rulesService DrixyRulesService,
) *CockpitReportsService {
	return &CockpitReportsService{
		reviewAnalytics: reviewAnalytics,
		codeHealth:      codeHealth,
		productivity:    productivity,
		rulesService:    rulesService,
	}
}

func (s *CockpitReportsService) BuildRepoSection(
	ctx context.Context,
	orgID, repository, startDate, endDate string,
	ruleTitles map[string]domain.RuleMeta,
) (*domain.RepoReportSection, error) {
	q := domain.CockpitRangeQuery{
		OrganizationID: orgID,
		StartDate:      startDate,
		EndDate:        endDate,
		Repository:     repository,
	}

	ops, err := s.reviewAnalytics.GetReviewOperationalMetrics(ctx, q)
	if err != nil {
		return nil, err
	}
	reviews := ops.CurrentPeriod.ProcessedReviews
	if reviews <= 0 {
		return nil, nil
	}

	prevPeriod, err := application.ComputePreviousPeriod(startDate, endDate)
	if err != nil {
		return nil, err
	}
	prevQ := q
	prevQ.StartDate = prevPeriod.StartDate
	prevQ.EndDate = prevPeriod.EndDate

	titles := ruleTitles
	if titles == nil && s.rulesService != nil {
		titles, _ = s.rulesService.GetRuleTitles(ctx, orgID)
	}
	if titles == nil {
		titles = make(map[string]domain.RuleMeta)
	}

	implCur, _ := s.codeHealth.GetImplementationRate(ctx, q)
	implPrev, _ := s.codeHealth.GetImplementationRate(ctx, prevQ)
	severity, _ := s.reviewAnalytics.GetImplementationRateBySeverity(ctx, q)
	groups, _ := s.reviewAnalytics.GetReviewQualityByRuleGroup(ctx, q)

	wStart, wEnd, _ := application.LastNCompleteWeeks(endDate, 4)
	weeklyQ := q
	weeklyQ.StartDate = wStart
	weeklyQ.EndDate = wEnd
	weekly, _ := s.reviewAnalytics.GetImplementationRateWeekly(ctx, weeklyQ)

	implByCat, _ := s.reviewAnalytics.GetImplementationRateByCategory(ctx, q)
	negByCat, _ := s.reviewAnalytics.GetNegativeFeedbackByCategory(ctx, q)
	rulesUsage, _ := s.reviewAnalytics.GetDrixyRulesUsage(ctx, q)

	critSent := 0
	critImpl := 0
	for _, sev := range severity {
		if sev.Severity == "critical" {
			critSent = sev.Sent
			critImpl = sev.Implemented
			break
		}
	}

	// Feedback assembly
	feedback := s.buildRuleGroupFeedback(groups)

	// Weekly trend points
	var weeklyPoints []domain.WeeklyImplementedPoint
	for _, w := range weekly {
		weeklyPoints = append(weeklyPoints, domain.WeeklyImplementedPoint{
			WeekStart:   w.WeekStart,
			Sent:        w.Sent,
			Implemented: w.Implemented,
		})
	}

	// Category quality
	categories := s.buildCategoryRows(implByCat, negByCat)

	// Rules health
	ruleRows, rulesMore := s.buildRuleHealthRows(rulesUsage, titles)

	// Metric comparisons
	revTrend := domain.ReportTrend(ops.Comparison.ProcessedReviews.Trend)
	revChange := ops.Comparison.ProcessedReviews.PercentageChange

	suggSentCur := 0
	if implCur != nil {
		suggSentCur = implCur.SuggestionsSent
	}
	suggSentPrev := 0
	if implPrev != nil {
		suggSentPrev = implPrev.SuggestionsSent
	}
	suggComp := application.ComputeTrend(float64(suggSentCur), float64(suggSentPrev), "up")

	implRateCur := 0.0
	if implCur != nil {
		implRateCur = implCur.ImplementationRate
	}
	implRatePrev := 0.0
	if implPrev != nil {
		implRatePrev = implPrev.ImplementationRate
	}
	implTrend := application.ComputeTrend(implRateCur, implRatePrev, "up")
	implPpChange := round2((implRateCur - implRatePrev) * 100.0)

	return &domain.RepoReportSection{
		Repository:                 repository,
		Reviews:                    reviews,
		ReviewsTrend:               revTrend,
		ReviewsChangePct:           revChange,
		SuggestionsSent:            suggSentCur,
		SuggestionsSentTrend:       domain.ReportTrend(suggComp.Trend),
		SuggestionsSentChangePct:   suggComp.PercentageChange,
		ImplementationRate:         implRateCur,
		ImplementationRateTrend:    domain.ReportTrend(implTrend.Trend),
		ImplementationRatePpChange: implPpChange,
		CriticalImplemented:        critImpl,
		CriticalSent:               critSent,
		WeeklyImplemented:          weeklyPoints,
		Feedback:                   feedback,
		Categories:                 categories,
		Rules:                      ruleRows,
		RulesMore:                  rulesMore,
	}, nil
}

func (s *CockpitReportsService) BuildRepoSections(
	ctx context.Context,
	orgID string,
	repositories []string,
	startDate, endDate string,
) ([]domain.RepoReportSection, error) {
	var titles map[string]domain.RuleMeta
	if s.rulesService != nil {
		titles, _ = s.rulesService.GetRuleTitles(ctx, orgID)
	}

	var mu sync.Mutex
	var sections []domain.RepoReportSection

	sem := make(chan struct{}, repoBuildConcurrency)
	var wg sync.WaitGroup

	for _, repo := range repositories {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			sec, err := s.BuildRepoSection(ctx, orgID, r, startDate, endDate, titles)
			if err == nil && sec != nil {
				mu.Lock()
				sections = append(sections, *sec)
				mu.Unlock()
			}
		}(repo)
	}

	wg.Wait()

	sort.Slice(sections, func(i, j int) bool {
		return sections[i].Reviews > sections[j].Reviews
	})

	return sections, nil
}

func (s *CockpitReportsService) BuildOrgReport(
	ctx context.Context,
	orgID, company, startDate, endDate string,
) (*domain.OrgReportData, error) {
	q := domain.CockpitRangeQuery{
		OrganizationID: orgID,
		StartDate:      startDate,
		EndDate:        endDate,
	}
	prevPeriod, err := application.ComputePreviousPeriod(startDate, endDate)
	if err != nil {
		return nil, err
	}
	prevQ := q
	prevQ.StartDate = prevPeriod.StartDate
	prevQ.EndDate = prevPeriod.EndDate

	ops, err := s.reviewAnalytics.GetReviewOperationalMetrics(ctx, q)
	if err != nil {
		return nil, err
	}
	reviews := ops.CurrentPeriod.ProcessedReviews

	implCur, _ := s.codeHealth.GetImplementationRate(ctx, q)
	implPrev, _ := s.codeHealth.GetImplementationRate(ctx, prevQ)
	severity, _ := s.reviewAnalytics.GetImplementationRateBySeverity(ctx, q)
	leadTimeCur, _ := s.productivity.GetLeadTimeHighlight(ctx, q)
	reposHealth, _ := s.reviewAnalytics.GetRepositoriesHealth(ctx, q)
	rulesUsage, _ := s.reviewAnalytics.GetDrixyRulesUsage(ctx, q)

	var titles map[string]domain.RuleMeta
	if s.rulesService != nil {
		titles, _ = s.rulesService.GetRuleTitles(ctx, orgID)
	}

	critImpl := 0
	for _, sev := range severity {
		if sev.Severity == "critical" {
			critImpl = sev.Implemented
			break
		}
	}

	implRateCur := 0.0
	suggImpl := 0
	if implCur != nil {
		implRateCur = implCur.ImplementationRate
		suggImpl = implCur.SuggestionsImplemented
	}
	implRatePrev := 0.0
	if implPrev != nil {
		implRatePrev = implPrev.ImplementationRate
	}

	implTrend := application.ComputeTrend(implRateCur, implRatePrev, "up")
	implPpChange := round2((implRateCur - implRatePrev) * 100.0)

	cycleHours := 0.0
	cycleTrend := domain.TrendUnchanged
	cycleChange := 0.0
	if leadTimeCur != nil {
		cycleHours = leadTimeCur.CurrentPeriod.LeadTimeP75Hours
		cycleTrend = domain.ReportTrend(leadTimeCur.Comparison.Trend)
		cycleChange = leadTimeCur.Comparison.PercentageChange
	}

	// 3-month evolution
	evolution := s.buildMonthlyEvolution(ctx, orgID, endDate)

	// Ranking
	ranking := s.buildRanking(reposHealth)

	// Highlights
	highlights := s.buildHighlights(reposHealth)

	// Attention rules
	attentionRules, rulesMore := s.buildOrgAttentionRules(rulesUsage, titles)

	return &domain.OrgReportData{
		Company:                     company,
		StartDate:                   startDate,
		EndDate:                     endDate,
		Reviews:                     reviews,
		ReviewsTrend:                domain.ReportTrend(ops.Comparison.ProcessedReviews.Trend),
		ReviewsChangePct:            ops.Comparison.ProcessedReviews.PercentageChange,
		ImplementationRate:          implRateCur,
		ImplementationRateTrend:     domain.ReportTrend(implTrend.Trend),
		ImplementationRatePpChange:  implPpChange,
		SuggestionsImplemented:      suggImpl,
		CriticalImplemented:         critImpl,
		PRCycleTimeHours:            cycleHours,
		PRCycleTimeTrend:            cycleTrend,
		PRCycleTimeChangePct:        cycleChange,
		ImplementationRateEvolution: evolution,
		RepoRanking:                 ranking,
		Highlights:                  highlights,
		RulesNeedingAttention:       attentionRules,
		RulesNeedingAttentionMore:   rulesMore,
	}, nil
}

func (s *CockpitReportsService) buildRuleGroupFeedback(groups []domain.ReviewQualityByRuleGroupRow) domain.RuleGroupFeedback {
	var drixyGroup, genGroup domain.FeedbackGroup
	totalVotes := 0

	for _, g := range groups {
		var negRate *float64
		votes := g.ThumbsUp + g.ThumbsDown
		totalVotes += votes
		if votes > 0 {
			r := round2(float64(g.ThumbsDown) / float64(votes))
			negRate = &r
		}

		fg := domain.FeedbackGroup{
			SuggestionsSent:    g.Sent,
			ImplementationRate: g.Rate,
			ThumbsUp:           g.ThumbsUp,
			ThumbsDown:         g.ThumbsDown,
			NegativeRate:       negRate,
		}

		if g.Group == "drixy_rules" {
			drixyGroup = fg
		} else {
			genGroup = fg
		}
	}

	return domain.RuleGroupFeedback{
		TotalVotes:     totalVotes,
		HasEnoughVotes: totalVotes >= minFeedbackVotes,
		DrixyRules:     drixyGroup,
		General:        genGroup,
	}
}

func (s *CockpitReportsService) buildCategoryRows(
	implByCat []domain.ImplementationRateByCategoryRow,
	negByCat []domain.NegativeFeedbackByCategoryRow,
) []domain.CategoryQualityRow {
	negMap := make(map[string]domain.NegativeFeedbackByCategoryRow)
	for _, n := range negByCat {
		negMap[n.Category] = n
	}

	var rows []domain.CategoryQualityRow
	for _, c := range implByCat {
		n := negMap[c.Category]
		rows = append(rows, domain.CategoryQualityRow{
			Category:           c.Category,
			Sent:               c.Sent,
			ImplementationRate: c.Rate,
			ThumbsUp:           n.ThumbsUp,
			ThumbsDown:         n.ThumbsDown,
		})
		if len(rows) >= maxCategoryRows {
			break
		}
	}
	return rows
}

func (s *CockpitReportsService) buildRuleHealthRows(
	rulesUsage []domain.DrixyRuleUsageRow,
	titles map[string]domain.RuleMeta,
) ([]domain.RuleHealthRow, int) {
	var rows []domain.RuleHealthRow

	for _, u := range rulesUsage {
		state, evaluatedUsage := helpers.ComputeRuleState(&u)
		title := u.RuleID
		if meta, ok := titles[u.RuleID]; ok && meta.Title != "" {
			title = meta.Title
		}

		rows = append(rows, domain.RuleHealthRow{
			RuleID:             u.RuleID,
			Title:              title,
			Triggers:           evaluatedUsage.Triggers,
			ImplementationRate: evaluatedUsage.Rate,
			ThumbsUp:           evaluatedUsage.ThumbsUp,
			ThumbsDown:         evaluatedUsage.ThumbsDown,
			State:              state,
		})
	}

	// Sort worst health first: noisy > ignored > low_data > healthy > stale
	stateWeight := map[domain.RuleHealthState]int{
		domain.RuleStateNoisy:   5,
		domain.RuleStateIgnored: 4,
		domain.RuleStateLowData: 3,
		domain.RuleStateHealthy: 2,
		domain.RuleStateStale:   1,
	}

	sort.Slice(rows, func(i, j int) bool {
		wI := stateWeight[rows[i].State]
		wJ := stateWeight[rows[j].State]
		if wI != wJ {
			return wI > wJ
		}
		return rows[i].Triggers > rows[j].Triggers
	})

	more := 0
	if len(rows) > maxRepoRuleRows {
		more = len(rows) - maxRepoRuleRows
		rows = rows[:maxRepoRuleRows]
	}

	return rows, more
}

func (s *CockpitReportsService) buildMonthlyEvolution(
	ctx context.Context,
	orgID, endDate string,
) []domain.MonthlyRatePoint {
	months, err := application.LastNMonths(endDate, 3)
	if err != nil {
		return nil
	}

	var points []domain.MonthlyRatePoint
	for _, m := range months {
		rateObj, _ := s.codeHealth.GetImplementationRate(ctx, domain.CockpitRangeQuery{
			OrganizationID: orgID,
			StartDate:      m.MonthStart,
			EndDate:        m.MonthEnd,
		})
		r := 0.0
		if rateObj != nil {
			r = rateObj.ImplementationRate
		}
		points = append(points, domain.MonthlyRatePoint{
			MonthStart: m.MonthStart,
			Label:      m.Label,
			Rate:       r,
		})
	}
	return points
}

func (s *CockpitReportsService) buildRanking(reposHealth []domain.RepositoryHealthRow) []domain.RepoRankingRow {
	var eligible []domain.RepositoryHealthRow
	for _, r := range reposHealth {
		if r.PRsReviewed >= rankingMinReviews {
			eligible = append(eligible, r)
		}
	}

	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].ImplementationRate != eligible[j].ImplementationRate {
			return eligible[i].ImplementationRate > eligible[j].ImplementationRate
		}
		return eligible[i].PRsReviewed > eligible[j].PRsReviewed
	})

	var ranking []domain.RepoRankingRow
	for i, r := range eligible {
		ranking = append(ranking, domain.RepoRankingRow{
			Rank:               i + 1,
			Repository:         r.Repository,
			Reviews:            r.PRsReviewed,
			ImplementationRate: r.ImplementationRate,
		})
	}
	return ranking
}

func (s *CockpitReportsService) buildHighlights(reposHealth []domain.RepositoryHealthRow) []domain.ReportHighlight {
	if len(reposHealth) == 0 {
		return nil
	}

	var bestRepo string
	bestRate := 0.0
	for _, r := range reposHealth {
		if r.PRsReviewed >= highlightMinReviews && r.ImplementationRate > bestRate {
			bestRate = r.ImplementationRate
			bestRepo = r.Repository
		}
	}

	if bestRepo == "" {
		return nil
	}

	return []domain.ReportHighlight{
		{
			Kind:       "impl_rate_growth",
			Repository: bestRepo,
			Detail:     "Highest recommendation implementation rate among active repositories",
		},
	}
}

func (s *CockpitReportsService) buildOrgAttentionRules(
	rulesUsage []domain.DrixyRuleUsageRow,
	titles map[string]domain.RuleMeta,
) ([]domain.RuleHealthRow, int) {
	var attention []domain.RuleHealthRow

	for _, u := range rulesUsage {
		state, evaluatedUsage := helpers.ComputeRuleState(&u)
		if state == domain.RuleStateNoisy || state == domain.RuleStateIgnored {
			title := u.RuleID
			if meta, ok := titles[u.RuleID]; ok && meta.Title != "" {
				title = meta.Title
			}
			attention = append(attention, domain.RuleHealthRow{
				RuleID:             u.RuleID,
				Title:              title,
				Triggers:           evaluatedUsage.Triggers,
				ImplementationRate: evaluatedUsage.Rate,
				ThumbsUp:           evaluatedUsage.ThumbsUp,
				ThumbsDown:         evaluatedUsage.ThumbsDown,
				State:              state,
			})
		}
	}

	sort.Slice(attention, func(i, j int) bool {
		return attention[i].Triggers > attention[j].Triggers
	})

	more := 0
	if len(attention) > maxOrgRulesAttention {
		more = len(attention) - maxOrgRulesAttention
		attention = attention[:maxOrgRulesAttention]
	}

	return attention, more
}

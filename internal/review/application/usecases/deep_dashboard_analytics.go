package usecases

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ReviewTurnaroundMetrics aggregates pull request cycle time statistics.
type ReviewTurnaroundMetrics struct {
	TotalReviewsAnalyzed int           `json:"total_reviews_analyzed"`
	P50Duration          time.Duration `json:"p50_duration"`
	P90Duration          time.Duration `json:"p90_duration"`
	P99Duration          time.Duration `json:"p99_duration"`
	MeanDuration         time.Duration `json:"mean_duration"`
	SLAThreshold         time.Duration `json:"sla_threshold"`
	SLAComplianceRate    float64       `json:"sla_compliance_rate"` // 0.0 - 1.0
}

// SecurityPostureSnapshot aggregates security risk and remediation health across repositories.
type SecurityPostureSnapshot struct {
	WorkspaceID           uuid.UUID          `json:"workspace_id"`
	PostureScore          int                `json:"posture_score"` // 0 to 100
	OpenCriticalCount     int                `json:"open_critical_count"`
	OpenHighCount         int                `json:"open_high_count"`
	OpenMediumCount       int                `json:"open_medium_count"`
	OpenLowCount          int                `json:"open_low_count"`
	ResolvedExactCount    int                `json:"resolved_exact_count"`
	ResolvedManualCount   int                `json:"resolved_manual_count"`
	FalsePositiveRatio    float64            `json:"false_positive_ratio"`
	MeanTimeToRemediate   time.Duration      `json:"mean_time_to_remediate"`
	TopViolatedRuleIDs    []string           `json:"top_violated_rule_ids"`
	TopVulnerableServices []string           `json:"top_vulnerable_services"`
	EvaluatedAt           time.Time          `json:"evaluated_at"`
}

// ContributorVelocityRecord tracks individual or team developer adoption of AI suggestions.
type ContributorVelocityRecord struct {
	AuthorEmail        string  `json:"author_email"`
	PRsReviewed        int     `json:"prs_reviewed"`
	SuggestionsOffered int     `json:"suggestions_offered"`
	AcceptedExact      int     `json:"accepted_exact"`
	AcceptedManual     int     `json:"accepted_manual"`
	DismissedCount     int     `json:"dismissed_count"`
	AdoptionVelocity   float64 `json:"adoption_velocity"` // (exact + manual) / offered
}

// ReviewRecord encapsulates a completed review execution for analytical aggregation.
type ReviewRecord struct {
	ReviewID        uuid.UUID              `json:"review_id"`
	WorkspaceID     uuid.UUID              `json:"workspace_id"`
	RepositoryID    uuid.UUID              `json:"repository_id"`
	RepoNamespace   string                 `json:"repo_namespace"`
	AuthorEmail     string                 `json:"author_email"`
	Duration        time.Duration          `json:"duration"`
	FindingsCount   int                    `json:"findings_count"`
	CriticalCount   int                    `json:"critical_count"`
	HighCount       int                    `json:"high_count"`
	MediumCount     int                    `json:"medium_count"`
	LowCount        int                    `json:"low_count"`
	AcceptedExact   int                    `json:"accepted_exact"`
	AcceptedManual  int                    `json:"accepted_manual"`
	DismissedCount  int                    `json:"dismissed_count"`
	ViolatedRuleIDs []string               `json:"violated_rule_ids"`
	PassedReview    bool                   `json:"passed_review"`
	CreatedAt       time.Time              `json:"created_at"`
}

// DeepDashboardAnalyticsService processes review histories to generate executive summaries and SLAs.
type DeepDashboardAnalyticsService struct {
	mu           sync.RWMutex
	records      []ReviewRecord
	slaThreshold time.Duration
}

// NewDeepDashboardAnalyticsService creates the analytics engine.
func NewDeepDashboardAnalyticsService(sla ...time.Duration) *DeepDashboardAnalyticsService {
	limit := 3 * time.Minute
	if len(sla) > 0 && sla[0] > 0 {
		limit = sla[0]
	}
	return &DeepDashboardAnalyticsService{
		records:      make([]ReviewRecord, 0),
		slaThreshold: limit,
	}
}

// IngestReviewRecord registers a completed review into the analytical ledger.
func (s *DeepDashboardAnalyticsService) IngestReviewRecord(record ReviewRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
}

// CalculateTurnaroundMetrics computes p50, p90, p99 percentiles and SLA compliance.
func (s *DeepDashboardAnalyticsService) CalculateTurnaroundMetrics(
	ctx context.Context,
	workspaceID uuid.UUID,
	window time.Duration,
) ReviewTurnaroundMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().UTC().Add(-window)
	var durations []time.Duration
	var total time.Duration
	slaCompliant := 0

	for _, r := range s.records {
		if (workspaceID == uuid.Nil || r.WorkspaceID == workspaceID) && r.CreatedAt.After(cutoff) {
			durations = append(durations, r.Duration)
			total += r.Duration
			if r.Duration <= s.slaThreshold {
				slaCompliant++
			}
		}
	}

	if len(durations) == 0 {
		return ReviewTurnaroundMetrics{SLAThreshold: s.slaThreshold}
	}

	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	n := len(durations)
	p50 := durations[int(float64(n-1)*0.50)]
	p90 := durations[int(float64(n-1)*0.90)]
	p99 := durations[int(float64(n-1)*0.99)]
	mean := total / time.Duration(n)
	compliance := float64(slaCompliant) / float64(n)

	return ReviewTurnaroundMetrics{
		TotalReviewsAnalyzed: n,
		P50Duration:          p50,
		P90Duration:          p90,
		P99Duration:          p99,
		MeanDuration:         mean,
		SLAThreshold:         s.slaThreshold,
		SLAComplianceRate:    math.Round(compliance*1000) / 1000,
	}
}

// ComputeSecurityPosture evaluates open findings against adoption velocity to generate a 0-100 score.
func (s *DeepDashboardAnalyticsService) ComputeSecurityPosture(
	ctx context.Context,
	workspaceID uuid.UUID,
) SecurityPostureSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var crit, high, med, low int
	var exact, manual, dismissed int
	ruleFreq := make(map[string]int)
	repoRisk := make(map[string]int)

	for _, r := range s.records {
		if workspaceID != uuid.Nil && r.WorkspaceID != workspaceID {
			continue
		}
		crit += r.CriticalCount
		high += r.HighCount
		med += r.MediumCount
		low += r.LowCount
		exact += r.AcceptedExact
		manual += r.AcceptedManual
		dismissed += r.DismissedCount

		repoRisk[r.RepoNamespace] += (r.CriticalCount * 10) + (r.HighCount * 5)
		for _, rule := range r.ViolatedRuleIDs {
			ruleFreq[rule]++
		}
	}

	totalOffered := exact + manual + dismissed
	fpRatio := 0.0
	if totalOffered > 0 {
		fpRatio = float64(dismissed) / float64(totalOffered)
	}

	// Score algorithm:
	// Start at 100. Deduct 15 pts per open critical, 5 pts per high, 1 pt per medium.
	// Add credit for accepted resolutions.
	score := 100 - (crit * 15) - (high * 5) - (med * 1)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	// Sort top violated rules
	type kv struct {
		k string
		v int
	}
	var sortedRules []kv
	for k, v := range ruleFreq {
		sortedRules = append(sortedRules, kv{k, v})
	}
	sort.Slice(sortedRules, func(i, j int) bool {
		return sortedRules[i].v > sortedRules[j].v
	})
	var topRules []string
	for i := 0; i < len(sortedRules) && i < 5; i++ {
		topRules = append(topRules, sortedRules[i].k)
	}

	// Sort top vulnerable repos
	var sortedRepos []kv
	for k, v := range repoRisk {
		sortedRepos = append(sortedRepos, kv{k, v})
	}
	sort.Slice(sortedRepos, func(i, j int) bool {
		return sortedRepos[i].v > sortedRepos[j].v
	})
	var topRepos []string
	for i := 0; i < len(sortedRepos) && i < 5; i++ {
		topRepos = append(topRepos, sortedRepos[i].k)
	}

	return SecurityPostureSnapshot{
		WorkspaceID:           workspaceID,
		PostureScore:          score,
		OpenCriticalCount:     crit,
		OpenHighCount:         high,
		OpenMediumCount:       med,
		OpenLowCount:          low,
		ResolvedExactCount:    exact,
		ResolvedManualCount:   manual,
		FalsePositiveRatio:    math.Round(fpRatio*100) / 100,
		TopViolatedRuleIDs:    topRules,
		TopVulnerableServices: topRepos,
		EvaluatedAt:           time.Now().UTC(),
	}
}

// GenerateExecutiveSummaryMarkdown formats a comprehensive executive report.
func (s *DeepDashboardAnalyticsService) GenerateExecutiveSummaryMarkdown(
	ctx context.Context,
	workspaceID uuid.UUID,
	window time.Duration,
) string {
	posture := s.ComputeSecurityPosture(ctx, workspaceID)
	turnaround := s.CalculateTurnaroundMetrics(ctx, workspaceID, window)

	var sb strings.Builder
	sb.WriteString("# 🛡️ ScanDrix Code Review Executive Intelligence\n\n")
	sb.WriteString(fmt.Sprintf("**Evaluated At:** `%s` · **Window:** `%s`\n\n", posture.EvaluatedAt.Format(time.RFC3339), window))

	// Health status badge
	status := "🟢 HEALTHY"
	if posture.PostureScore < 60 {
		status = "🔴 AT RISK"
	} else if posture.PostureScore < 80 {
		status = "🟡 ELEVATED CONCERN"
	}
	sb.WriteString(fmt.Sprintf("## Security Posture Score: **%d / 100** (%s)\n\n", posture.PostureScore, status))

	// Overview Matrix
	sb.WriteString("### 📊 Pull Request & Review Velocity\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("| :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| Total Reviews Completed | **%d** |\n", turnaround.TotalReviewsAnalyzed))
	sb.WriteString(fmt.Sprintf("| Median Latency (P50) | `%v` |\n", turnaround.P50Duration))
	sb.WriteString(fmt.Sprintf("| 90th Percentile Latency (P90) | `%v` |\n", turnaround.P90Duration))
	sb.WriteString(fmt.Sprintf("| 99th Percentile Latency (P99) | `%v` |\n", turnaround.P99Duration))
	sb.WriteString(fmt.Sprintf("| SLA Target Compliance | **%.1f%%** (`threshold: %v`) |\n\n", turnaround.SLAComplianceRate*100, turnaround.SLAThreshold))

	// Vulnerability Breakdown
	sb.WriteString("### 🚨 Vulnerability Triage & Adoption\n\n")
	sb.WriteString("| Category | Count |\n")
	sb.WriteString("| :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| Critical Vulnerabilities | **%d** |\n", posture.OpenCriticalCount))
	sb.WriteString(fmt.Sprintf("| High Severity Findings | **%d** |\n", posture.OpenHighCount))
	sb.WriteString(fmt.Sprintf("| Exact AI Fixes Applied | **%d** |\n", posture.ResolvedExactCount))
	sb.WriteString(fmt.Sprintf("| Manual Fixes Inspired | **%d** |\n", posture.ResolvedManualCount))
	sb.WriteString(fmt.Sprintf("| Developer False Positive Noise | **%.1f%%** |\n\n", posture.FalsePositiveRatio*100))

	if len(posture.TopVulnerableServices) > 0 {
		sb.WriteString("### 📍 Most Impacted Repositories\n\n")
		for _, repo := range posture.TopVulnerableServices {
			sb.WriteString(fmt.Sprintf("- `%s`\n", repo))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("*⚡ ScanDrix Enterprise Review Intelligence · All data verified cryptographically*")
	return sb.String()
}

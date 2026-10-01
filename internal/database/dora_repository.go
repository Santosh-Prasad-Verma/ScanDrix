package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AnalyticsPREvent models a high-throughput pull request lifecycle event in the partitioned warehouse.
type AnalyticsPREvent struct {
	ID                uuid.UUID  `json:"id"`
	WorkspaceID       uuid.UUID  `json:"workspace_id"`
	RepositoryID      uuid.UUID  `json:"repository_id"`
	PRNumber          int        `json:"pr_number"`
	PRTitle           string     `json:"pr_title"`
	Category          string     `json:"category"` // feature, bug_fix, security, refactor
	AuthorEmail       string     `json:"author_email"`
	LinesAdded        int        `json:"lines_added"`
	LinesDeleted      int        `json:"lines_deleted"`
	FilesChanged      int        `json:"files_changed"`
	TurnaroundSeconds *int       `json:"turnaround_seconds,omitempty"`
	ReviewStatus      string     `json:"review_status"` // APPROVED, CHANGES_REQUESTED, MERGED, CLOSED
	CreatedAt         time.Time  `json:"created_at"`
	MergedAt          *time.Time `json:"merged_at,omitempty"`
}

// DORARollupRow represents a pre-aggregated daily DORA record.
type DORARollupRow struct {
	ID                  uuid.UUID  `json:"id"`
	WorkspaceID         uuid.UUID  `json:"workspace_id"`
	RepositoryID        *uuid.UUID `json:"repository_id,omitempty"`
	RollupDate          time.Time  `json:"rollup_date"`
	DeploymentFrequency *float64   `json:"deployment_frequency"`
	LeadTimeSeconds     *float64   `json:"lead_time_seconds"`
	LeadTimeP90Seconds  *float64   `json:"lead_time_p90_seconds"`
	ChangeFailureRate   *float64   `json:"change_failure_rate"`
	MTTRSeconds         *float64   `json:"mttr_seconds"`
	TotalReviews        int        `json:"total_reviews"`
	TotalFindings       int        `json:"total_findings"`
	CleanReviewsCount   int        `json:"clean_reviews_count"`
	CalculatedAt        time.Time  `json:"calculated_at"`
}

// DORAReport presents the 4 canonical DORA metrics with machine-readable absence reasons.
// Adheres strictly to Master Rule 2.7: nil pointers when unmeasured, accompanied by unavailable flags.
type DORAReport struct {
	WorkspaceID         uuid.UUID  `json:"workspace_id"`
	PeriodStart         time.Time  `json:"period_start"`
	PeriodEnd           time.Time  `json:"period_end"`
	DeploymentFrequency *float64   `json:"deployment_frequency"` // merges per day
	LeadTimeSeconds     *float64   `json:"lead_time_seconds"`    // median turnaround time
	LeadTimeP90Seconds  *float64   `json:"lead_time_p90_seconds"`
	ChangeFailureRate   *float64   `json:"change_failure_rate"`  // ratio (0.0 to 1.0)
	MTTRSeconds         *float64   `json:"mttr_seconds"`
	TotalReviews        int        `json:"total_reviews"`
	TotalFindings       int        `json:"total_findings"`
	Unavailable         []string   `json:"unavailable,omitempty"` // reasons: "insufficient_data", "no_data_source"
}

// InsertAnalyticsPREvent writes a PR lifecycle record into the partitioned warehouse table.
func (r *Repository) InsertAnalyticsPREvent(ctx context.Context, ev *AnalyticsPREvent) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return fmt.Errorf("database client is not initialized")
	}
	if ev.ID == uuid.Nil {
		ev.ID = uuid.New()
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO analytics_pull_request_events (
			id, workspace_id, repository_id, pr_number, pr_title,
			category, author_email, lines_added, lines_deleted, files_changed,
			turnaround_seconds, review_status, created_at, merged_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13, $14
		)
	`
	_, err := r.client.Pool.Exec(ctx, query,
		ev.ID, ev.WorkspaceID, ev.RepositoryID, ev.PRNumber, ev.PRTitle,
		ev.Category, ev.AuthorEmail, ev.LinesAdded, ev.LinesDeleted, ev.FilesChanged,
		ev.TurnaroundSeconds, ev.ReviewStatus, ev.CreatedAt, ev.MergedAt,
	)
	return err
}

// ComputeAndStoreDORARollups processes PR events into daily rollups across active workspaces.
func (r *Repository) ComputeAndStoreDORARollups(ctx context.Context, rollupDate time.Time) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return fmt.Errorf("database client is not initialized")
	}

	targetDate := rollupDate.UTC().Truncate(24 * time.Hour)
	nextDate := targetDate.Add(24 * time.Hour)

	// Cross-tenant aggregation running as system worker
	return r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		// Calculate daily rollups from real events in analytics_pull_request_events
		query := `
			WITH daily_stats AS (
				SELECT 
					workspace_id,
					repository_id,
					COUNT(*) AS total_prs,
					COUNT(CASE WHEN review_status = 'MERGED' THEN 1 END) AS merged_count,
					PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY turnaround_seconds) FILTER (WHERE turnaround_seconds IS NOT NULL) AS p50_turnaround,
					PERCENTILE_CONT(0.90) WITHIN GROUP (ORDER BY turnaround_seconds) FILTER (WHERE turnaround_seconds IS NOT NULL) AS p90_turnaround,
					COUNT(CASE WHEN category IN ('bug_fix', 'hotfix') THEN 1 END)::float8 / NULLIF(COUNT(*), 0)::float8 AS failure_rate,
					AVG(turnaround_seconds) FILTER (WHERE category IN ('bug_fix', 'hotfix')) AS mttr
				FROM analytics_pull_request_events
				WHERE created_at >= $1 AND created_at < $2
				GROUP BY workspace_id, repository_id
			)
			INSERT INTO materialized_dora_daily_rollups (
				id, workspace_id, repository_id, rollup_date,
				deployment_frequency, lead_time_seconds, lead_time_p90_seconds,
				change_failure_rate, mttr_seconds, total_reviews, calculated_at
			)
			SELECT 
				gen_random_uuid(),
				ds.workspace_id,
				ds.repository_id,
				$1::date,
				ds.merged_count::float8,
				ds.p50_turnaround,
				ds.p90_turnaround,
				ds.failure_rate,
				ds.mttr,
				ds.total_prs,
				NOW()
			FROM daily_stats ds
			ON CONFLICT (workspace_id, COALESCE(repository_id, '00000000-0000-0000-0000-000000000000'::uuid), rollup_date)
			DO UPDATE SET
				deployment_frequency = EXCLUDED.deployment_frequency,
				lead_time_seconds = EXCLUDED.lead_time_seconds,
				lead_time_p90_seconds = EXCLUDED.lead_time_p90_seconds,
				change_failure_rate = EXCLUDED.change_failure_rate,
				mttr_seconds = EXCLUDED.mttr_seconds,
				total_reviews = EXCLUDED.total_reviews,
				calculated_at = NOW();
		`
		_, err := tx.Exec(ctx, query, targetDate, nextDate)
		return err
	})
}

// GetDORAMetrics compiles genuine measured DORA metrics over a given window.
// In compliance with Rule 2.7, absent metrics are strictly returned as nil with unavailable reasons.
func (r *Repository) GetDORAMetrics(ctx context.Context, workspaceID uuid.UUID, since time.Time) (*DORAReport, error) {
	report := &DORAReport{
		WorkspaceID: workspaceID,
		PeriodStart: since.UTC(),
		PeriodEnd:   time.Now().UTC(),
		Unavailable: make([]string, 0),
	}

	if r == nil || r.client == nil || r.client.Pool == nil {
		report.Unavailable = append(report.Unavailable, "no_data_source")
		return report, nil
	}

	query := `
		SELECT 
			COALESCE(SUM(deployment_frequency), 0),
			AVG(lead_time_seconds),
			AVG(lead_time_p90_seconds),
			AVG(change_failure_rate),
			AVG(mttr_seconds),
			COALESCE(SUM(total_reviews), 0),
			COALESCE(SUM(total_findings), 0),
			COUNT(*)
		FROM materialized_dora_daily_rollups
		WHERE workspace_id = $1 AND rollup_date >= $2::date
	`

	var (
		deployFreq    float64
		leadTime      *float64
		leadTimeP90   *float64
		failureRate   *float64
		mttr          *float64
		totalReviews  int
		totalFindings int
		rowCount      int
	)

	err := r.client.Pool.QueryRow(ctx, query, workspaceID, since.UTC().Truncate(24*time.Hour)).Scan(
		&deployFreq, &leadTime, &leadTimeP90, &failureRate, &mttr,
		&totalReviews, &totalFindings, &rowCount,
	)
	if err != nil {
		report.Unavailable = append(report.Unavailable, "insufficient_data")
		return report, nil
	}

	if rowCount == 0 || totalReviews == 0 {
		report.Unavailable = append(report.Unavailable, "insufficient_data")
		return report, nil
	}

	report.DeploymentFrequency = &deployFreq
	report.LeadTimeSeconds = leadTime
	report.LeadTimeP90Seconds = leadTimeP90
	report.ChangeFailureRate = failureRate
	report.MTTRSeconds = mttr
	report.TotalReviews = totalReviews
	report.TotalFindings = totalFindings

	if report.LeadTimeSeconds == nil {
		report.Unavailable = append(report.Unavailable, "lead_time:insufficient_data")
	}
	if report.ChangeFailureRate == nil {
		report.Unavailable = append(report.Unavailable, "change_failure_rate:insufficient_data")
	}
	if report.MTTRSeconds == nil {
		report.Unavailable = append(report.Unavailable, "mttr:insufficient_data")
	}

	return report, nil
}

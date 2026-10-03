package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/pkg/models"
)

// activeReviewStates are the non-terminal review states.
const activeReviewStatesSQL = "('QUEUED', 'PROCESSING', 'RECEIVED')"

// PostgresAnalyticsRepository computes workspace metrics from real data.
//
// Every method here is a database query. A metric that cannot be derived from
// the schema is reported through the Unavailable list on the response rather
// than being filled with a constant.
type PostgresAnalyticsRepository struct {
	client *Client
}

// NewPostgresAnalyticsRepository builds the analytics repository.
func NewPostgresAnalyticsRepository(client *Client) *PostgresAnalyticsRepository {
	return &PostgresAnalyticsRepository{client: client}
}

// GetCockpitOverview computes the workspace activity summary.
//
// PassRatePercentage is left unavailable: "pass rate" has no agreed definition
// in this product (reviews without critical findings? reviews completed without
// error? PRs approved on first pass?), and guessing one would produce a number
// that looks authoritative and means nothing.
func (r *PostgresAnalyticsRepository) GetCockpitOverview(ctx context.Context, wsID uuid.UUID) (*models.CockpitOverview, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("analytics repository unavailable")
	}

	out := &models.CockpitOverview{
		WorkspaceID: wsID.String(),
		Unavailable: make([]models.AnalyticsUnavailable, 0, 4),
	}

	type counts struct {
		Active    int
		Completed int
		Failed    int
		Repos     int
		MeanMins  *float64
	}

	var c counts
	var meanMins *float64

	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT
				(SELECT COUNT(*) FROM pull_request_reviews
				  WHERE workspace_id = $1 AND state IN `+activeReviewStatesSQL+`)::int,
				(SELECT COUNT(*) FROM pull_request_reviews
				  WHERE workspace_id = $1 AND state = 'COMPLETED')::int,
				(SELECT COUNT(*) FROM pull_request_reviews
				  WHERE workspace_id = $1 AND state = 'FAILED')::int,
				(SELECT COUNT(*) FROM tracked_repositories
				  WHERE workspace_id = $1 AND is_active)::int,
				(SELECT AVG(EXTRACT(EPOCH FROM (completed_at - created_at)) / 60.0)
				   FROM pull_request_reviews
				  WHERE workspace_id = $1
				    AND state = 'COMPLETED'
				    AND completed_at IS NOT NULL
				    AND completed_at >= created_at)
		`, wsID).Scan(&c.Active, &c.Completed, &c.Failed, &c.Repos, &meanMins)
	})
	if err != nil {
		return nil, fmt.Errorf("failed querying cockpit overview: %w", err)
	}

	out.ActiveReviews = c.Active
	out.ActiveRepositories = c.Repos
	out.CompletedReviews = c.Completed
	out.FailedReviews = c.Failed
	out.MeanTimeToReviewMin = meanMins

	if meanMins == nil {
		out.Unavailable = append(out.Unavailable, models.AnalyticsUnavailable{
			Metric: "meanTimeToReviewMin",
			Reason: models.ReasonInsufficientData,
			Detail: "no completed review has both created_at and completed_at set",
		})
	}

	out.Unavailable = append(out.Unavailable,
		models.AnalyticsUnavailable{
			Metric: "passRatePercentage",
			Reason: models.ReasonNoFormula,
			Detail: "no agreed definition of pass rate for this product",
		},
		models.AnalyticsUnavailable{
			Metric: "securityScore",
			Reason: models.ReasonNoFormula,
			Detail: "no scoring formula defined; the previous value was a constant",
		},
		models.AnalyticsUnavailable{
			Metric: "developerHoursSaved",
			Reason: models.ReasonNoFormula,
			Detail: "requires a per-finding time-to-address model",
		},
	)

	return out, nil
}

// GetDoraMetrics reports the DORA indicators.
//
// The schema records no deployment events, so none of the four indicators can be
// derived. They are reported as unavailable with ReasonNoDataSource. Returning a
// rating here would mean asserting a deployment cadence the platform never
// observed.
func (r *PostgresAnalyticsRepository) GetDoraMetrics(ctx context.Context, wsID uuid.UUID) (*models.DoraMetrics, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("analytics repository unavailable")
	}

	deploymentSources := 0
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT COUNT(*)::int FROM information_schema.tables
			WHERE table_schema = 'public'
			  AND (table_name LIKE '%deploy%' OR table_name LIKE '%incident%')
		`).Scan(&deploymentSources)
	})
	if err != nil {
		return nil, fmt.Errorf("failed probing deployment data sources: %w", err)
	}

	detail := "no deployment or incident table exists in the schema"
	if deploymentSources > 0 {
		detail = "a deployment table exists but is not populated by any writer yet"
	}

	unavailable := make([]models.AnalyticsUnavailable, 0, 5)
	for _, metric := range []string{
		"deploymentFrequency", "leadTimeForChanges",
		"changeFailureRate", "timeToRestore", "rating",
	} {
		unavailable = append(unavailable, models.AnalyticsUnavailable{
			Metric: metric,
			Reason: models.ReasonNoDataSource,
			Detail: detail,
		})
	}

	return &models.DoraMetrics{Unavailable: unavailable}, nil
}

// GetProductivityMetrics computes review throughput and turnaround.
//
// CycleTimeHours uses the pull request opened_at to closed_at span, which is the
// only lifecycle the schema records. Throughput counts reviews completed in the
// last seven days.
func (r *PostgresAnalyticsRepository) GetProductivityMetrics(ctx context.Context, wsID uuid.UUID) (*models.ProductivityMetrics, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("analytics repository unavailable")
	}

	out := &models.ProductivityMetrics{
		Unavailable: make([]models.AnalyticsUnavailable, 0, 2),
	}

	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT
				(SELECT AVG(EXTRACT(EPOCH FROM (p.closed_at - p.opened_at)) / 3600.0)
				   FROM platform_pull_requests p
				  WHERE p.workspace_id = $1
				    AND p.closed_at IS NOT NULL AND p.opened_at IS NOT NULL
				    AND p.closed_at >= p.opened_at),
				(SELECT AVG(EXTRACT(EPOCH FROM (r.completed_at - r.created_at)) / 60.0)
				   FROM pull_request_reviews r
				  WHERE r.workspace_id = $1
				    AND r.state = 'COMPLETED'
				    AND r.completed_at IS NOT NULL
				    AND r.completed_at >= r.created_at),
				(SELECT COUNT(*)::int FROM pull_request_reviews
				  WHERE workspace_id = $1 AND state = 'COMPLETED'
				    AND completed_at >= now() - interval '7 days'),
				(SELECT COUNT(*)::int FROM pull_request_reviews
				  WHERE workspace_id = $1 AND state = 'COMPLETED')
		`, wsID).Scan(
			&out.CycleTimeHours,
			&out.ReviewTurnaroundMin,
			&out.ThroughputPerWeek,
			&out.PrsReviewedByScanDrix,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("failed querying productivity metrics: %w", err)
	}

	if out.CycleTimeHours == nil {
		out.Unavailable = append(out.Unavailable, models.AnalyticsUnavailable{
			Metric: "cycleTimeHours",
			Reason: models.ReasonInsufficientData,
			Detail: "no tracked pull request has both opened_at and closed_at set",
		})
	}
	if out.ReviewTurnaroundMin == nil {
		out.Unavailable = append(out.Unavailable, models.AnalyticsUnavailable{
			Metric: "reviewTurnaroundMin",
			Reason: models.ReasonInsufficientData,
			Detail: "no completed review has both created_at and completed_at set",
		})
	}

	return out, nil
}

// GetCodeHealthMetrics summarises structural debt from real finding data.
//
// HealthScore and ComplexityHotspots are unavailable: cyclomatic complexity is
// not recorded anywhere in the schema, and a score derived from finding counts
// alone would not be the metric the field is named after.
func (r *PostgresAnalyticsRepository) GetCodeHealthMetrics(ctx context.Context, wsID uuid.UUID) (*models.CodeHealthMetrics, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("analytics repository unavailable")
	}

	out := &models.CodeHealthMetrics{
		WorkspaceID: wsID.String(),
		EvaluatedAt: time.Now().UTC().Format(time.RFC3339),
		Unavailable: make([]models.AnalyticsUnavailable, 0, 3),
	}

	var criticalFiles *int
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT
				(SELECT COUNT(*)::int FROM code_findings WHERE workspace_id = $1),
				(SELECT COUNT(*)::int FROM code_findings
				  WHERE workspace_id = $1 AND status = 'OPEN'),
				(SELECT COUNT(DISTINCT file_path)::int FROM code_findings
				  WHERE workspace_id = $1
				    AND severity IN ('CRITICAL', 'HIGH')
				    AND status = 'OPEN')
		`, wsID).Scan(&out.TotalFindings, &out.OpenFindings, &criticalFiles)
	})
	if err != nil {
		return nil, fmt.Errorf("failed querying code health metrics: %w", err)
	}

	out.CriticalDebtFiles = criticalFiles
	if criticalFiles == nil {
		out.Unavailable = append(out.Unavailable, models.AnalyticsUnavailable{
			Metric: "criticalDebtFiles",
			Reason: models.ReasonInsufficientData,
		})
	}

	out.Unavailable = append(out.Unavailable,
		models.AnalyticsUnavailable{
			Metric: "healthScore",
			Reason: models.ReasonNoDataSource,
			Detail: "cyclomatic complexity is not recorded; code_ast_nodes carries no complexity measure",
		},
		models.AnalyticsUnavailable{
			Metric: "complexityHotspots",
			Reason: models.ReasonNoDataSource,
			Detail: "requires per-file complexity, which no table stores",
		},
		models.AnalyticsUnavailable{
			Metric: "trend",
			Reason: models.ReasonNoFormula,
			Detail: "a trend needs at least two evaluation points; none are persisted",
		},
	)

	return out, nil
}

// ListCodeHotspotFiles returns the files with the most unresolved findings.
func (r *PostgresAnalyticsRepository) ListCodeHotspotFiles(ctx context.Context, wsID uuid.UUID, limit int) ([]models.CodeHotspotFile, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("analytics repository unavailable")
	}
	if limit <= 0 || limit > 200 {
		limit = 20
	}

	files := make([]models.CodeHotspotFile, 0, limit)
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT file_path,
			       COUNT(*)::int AS findings,
			       COUNT(*) FILTER (WHERE severity = 'CRITICAL')::int AS critical
			FROM code_findings
			WHERE workspace_id = $1 AND status = 'OPEN'
			GROUP BY file_path
			ORDER BY critical DESC, findings DESC, file_path ASC
			LIMIT $2
		`, wsID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var f models.CodeHotspotFile
			if err := rows.Scan(&f.FilePath, &f.FindingsCount, &f.CriticalCount); err != nil {
				return err
			}
			files = append(files, f)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed querying code hotspot files: %w", err)
	}

	return files, nil
}

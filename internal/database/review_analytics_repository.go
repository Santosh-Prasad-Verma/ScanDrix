package database

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/pkg/models"
)

const scopedReviewsSQL = `WITH reviews AS (
	SELECT * FROM pull_request_reviews
	WHERE workspace_id = $1 AND ($2::uuid IS NULL OR repository_id = $2)
	AND created_at >= $3 AND created_at < $4
) `

func (r *PostgresAnalyticsRepository) SearchReviewSuggestions(ctx context.Context, wsID uuid.UUID, filter models.SuggestionSearchFilter) (*models.SuggestionSearchResult, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("analytics repository unavailable")
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 20
	}
	out := &models.SuggestionSearchResult{Items: []models.SuggestionSearchItem{}, Page: filter.Page, Limit: filter.Limit}
	const from = ` FROM code_findings f JOIN reviews r ON r.id=f.review_id
		JOIN tracked_repositories t ON t.id=r.repository_id AND t.workspace_id=$1
		WHERE f.workspace_id=$1 AND ($5='' OR f.title ILIKE '%' || $5 || '%' OR f.file_path ILIKE '%' || $5 || '%')
		AND ($6='' OR f.severity=$6) AND ($7='' OR f.category=$7)`
	args := []any{wsID, filter.RepositoryID, filter.Start, filter.End, filter.Search, filter.Severity, filter.Category}
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, scopedReviewsSQL+`SELECT COUNT(*)::int`+from, args...).Scan(&out.Total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, scopedReviewsSQL+`SELECT f.id, f.review_id, f.workspace_id, f.file_path, f.start_line, f.end_line,
			f.severity, f.category, f.title, f.description, f.remediation, COALESCE(f.suggested_diff,''), f.fingerprint, f.created_at,
			t.namespace_path, r.pull_number, r.title`+from+` ORDER BY f.created_at DESC, f.id LIMIT $8 OFFSET $9`, append(args, filter.Limit, (filter.Page-1)*filter.Limit)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item models.SuggestionSearchItem
			if err := rows.Scan(&item.ID, &item.ReviewID, &item.WorkspaceID, &item.FilePath, &item.StartLine, &item.EndLine, &item.Severity, &item.Category, &item.Title, &item.Description, &item.Remediation, &item.SuggestedDiff, &item.Fingerprint, &item.CreatedAt, &item.Repository, &item.PullNumber, &item.PullTitle); err != nil {
				return err
			}
			out.Items = append(out.Items, item)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("suggestion search failed: %w", err)
	}
	return out, nil
}

func (r *PostgresAnalyticsRepository) GetReviewAnalytics(ctx context.Context, wsID uuid.UUID, filter models.ReviewAnalyticsFilter) (*models.ReviewAnalytics, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("analytics repository unavailable")
	}
	out := &models.ReviewAnalytics{
		Start: filter.Start, End: filter.End, RepositoryID: filter.RepositoryID,
		Weekly: []models.ReviewAnalyticsBucket{}, Severity: []models.ReviewAnalyticsBucket{},
		Categories: []models.ReviewAnalyticsBucket{}, Repositories: []models.ReviewAnalyticsBucket{},
		Unavailable: []models.AnalyticsUnavailable{
			{Metric: "implementationRate", Reason: models.ReasonNoFormula, Detail: "finding implementation is not defined for this reporting contract"},
			{Metric: "negativeFeedbackRate", Reason: models.ReasonNoFormula, Detail: "feedback eligibility and denominator are not defined for this reporting contract"},
		},
	}
	args := []any{wsID, filter.RepositoryID, filter.Start, filter.End}
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, scopedReviewsSQL+`SELECT COUNT(*)::int,
			COUNT(*) FILTER (WHERE state = 'COMPLETED')::int,
			COUNT(*) FILTER (WHERE state = 'FAILED')::int,
			COUNT(*) FILTER (WHERE state IN ('QUEUED','PROCESSING','RECEIVED'))::int,
			AVG(EXTRACT(EPOCH FROM (completed_at-created_at))/60.0) FILTER (WHERE state='COMPLETED' AND completed_at >= created_at),
			(SELECT COUNT(*)::int FROM code_findings f JOIN reviews r ON f.review_id=r.id WHERE f.workspace_id=$1)
			FROM reviews`, args...).Scan(&out.Total, &out.Completed, &out.Failed, &out.Active, &out.MeanMinutes, &out.Findings); err != nil {
			return err
		}
		queries := []struct {
			sql    string
			target *[]models.ReviewAnalyticsBucket
		}{
			{`SELECT to_char(date_trunc('week', created_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD'), COUNT(*)::int FROM reviews GROUP BY 1 ORDER BY 1`, &out.Weekly},
			{`SELECT f.severity, COUNT(*)::int FROM code_findings f JOIN reviews r ON f.review_id=r.id WHERE f.workspace_id=$1 GROUP BY 1 ORDER BY 2 DESC, 1`, &out.Severity},
			{`SELECT f.category, COUNT(*)::int FROM code_findings f JOIN reviews r ON f.review_id=r.id WHERE f.workspace_id=$1 GROUP BY 1 ORDER BY 2 DESC, 1`, &out.Categories},
			{`SELECT t.namespace_path, COUNT(*)::int FROM reviews r JOIN tracked_repositories t ON t.id=r.repository_id AND t.workspace_id=$1 GROUP BY 1 ORDER BY 2 DESC, 1`, &out.Repositories},
		}
		for _, q := range queries {
			rows, err := tx.Query(ctx, scopedReviewsSQL+q.sql, args...)
			if err != nil {
				return err
			}
			for rows.Next() {
				var bucket models.ReviewAnalyticsBucket
				if err := rows.Scan(&bucket.Label, &bucket.Count); err != nil {
					rows.Close()
					return err
				}
				*q.target = append(*q.target, bucket)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("review analytics query failed: %w", err)
	}
	if out.MeanMinutes == nil {
		out.Unavailable = append(out.Unavailable, models.AnalyticsUnavailable{Metric: "meanMinutes", Reason: models.ReasonInsufficientData})
	}
	return out, nil
}

package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
)

// TokenUsageRepository defines operations for LLM token usage accounting and billing quotas.
type TokenUsageRepository interface {
	Record(ctx context.Context, usage *domain.TokenUsageRecord) error
	GetMonthlySpend(ctx context.Context, wsID uuid.UUID, year int, month time.Month) (float64, error)
	GetAggregatedUsage(ctx context.Context, wsID uuid.UUID, since time.Time) (map[string]int64, error)
}

// PostgresTokenUsageRepository implements TokenUsageRepository.
type PostgresTokenUsageRepository struct {
	db *sql.DB
}

// NewPostgresTokenUsageRepository instantiates a token usage repository.
func NewPostgresTokenUsageRepository(db *sql.DB) *PostgresTokenUsageRepository {
	return &PostgresTokenUsageRepository{db: db}
}

// Record inserts a token usage ledger entry.
func (r *PostgresTokenUsageRepository) Record(ctx context.Context, usage *domain.TokenUsageRecord) error {
	if usage.ID == uuid.Nil {
		usage.ID = uuid.New()
	}
	if usage.RecordedAt.IsZero() {
		usage.RecordedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO token_usage_records (id, workspace_id, review_id, repository_id, model_name, prompt_tokens, completion_tokens, total_tokens, estimated_cost_usd, operation_type, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := r.db.ExecContext(ctx, query,
		usage.ID, usage.WorkspaceID, usage.ReviewID, usage.RepositoryID,
		usage.ModelName, usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens,
		usage.EstimatedCostUSD, usage.OperationType, usage.RecordedAt,
	)
	if err != nil {
		return fmt.Errorf("failed recording token usage: %w", err)
	}
	return nil
}

// GetMonthlySpend aggregates the total USD cost for an organization in a calendar month.
func (r *PostgresTokenUsageRepository) GetMonthlySpend(ctx context.Context, wsID uuid.UUID, year int, month time.Month) (float64, error) {
	start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)

	query := `
		SELECT COALESCE(SUM(estimated_cost_usd), 0)
		FROM token_usage_records
		WHERE workspace_id = $1 AND recorded_at >= $2 AND recorded_at < $3
	`
	var spend float64
	err := r.db.QueryRowContext(ctx, query, wsID, start, end).Scan(&spend)
	if err != nil {
		return 0, fmt.Errorf("failed aggregating monthly spend: %w", err)
	}
	return spend, nil
}

// GetAggregatedUsage returns prompt vs completion tokens since a given timestamp.
func (r *PostgresTokenUsageRepository) GetAggregatedUsage(ctx context.Context, wsID uuid.UUID, since time.Time) (map[string]int64, error) {
	query := `
		SELECT 
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(completion_tokens), 0),
			COALESCE(SUM(total_tokens), 0)
		FROM token_usage_records
		WHERE workspace_id = $1 AND recorded_at >= $2
	`
	var prompt, completion, total int64
	err := r.db.QueryRowContext(ctx, query, wsID, since).Scan(&prompt, &completion, &total)
	if err != nil {
		return nil, fmt.Errorf("failed fetching aggregated usage: %w", err)
	}

	return map[string]int64{
		"prompt_tokens":     prompt,
		"completion_tokens": completion,
		"total_tokens":      total,
	}, nil
}

// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: postgres_rule_like_repository.go
// ═══════════════════════════════════════════════════════════════

package repositories

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
)

// PostgresRuleLikeRepository implements IRuleLikeRepository with PostgreSQL backing and in-memory fallback.
type PostgresRuleLikeRepository struct {
	client   *database.Client
	mu       sync.RWMutex
	likes    map[string]*entities.RuleLikeEntity // key: ruleId + ":" + userId
	initOnce sync.Once
}

// NewPostgresRuleLikeRepository initializes the feedback repository.
func NewPostgresRuleLikeRepository(client *database.Client) *PostgresRuleLikeRepository {
	repo := &PostgresRuleLikeRepository{
		client: client,
		likes:  make(map[string]*entities.RuleLikeEntity),
	}
	repo.ensureSchema(context.Background())
	return repo
}

func (r *PostgresRuleLikeRepository) ensureSchema(ctx context.Context) {
	if r.client == nil || r.client.Pool == nil {
		return
	}
	r.initOnce.Do(func() {
		query := `
			CREATE TABLE IF NOT EXISTS drixy_rule_likes (
				id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				rule_id VARCHAR(255) NOT NULL,
				user_id VARCHAR(255) NOT NULL,
				feedback VARCHAR(32) NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				CONSTRAINT uq_drixy_rule_user_like UNIQUE (rule_id, user_id)
			);
			CREATE INDEX IF NOT EXISTS idx_drixy_rule_likes_rule ON drixy_rule_likes(rule_id);
		`
		_, _ = r.client.Pool.Exec(ctx, query)
	})
}

func makeLikeKey(ruleID, userID string) string {
	return ruleID + ":" + userID
}

// SetFeedback inserts or updates feedback for a user and rule.
func (r *PostgresRuleLikeRepository) SetFeedback(ctx context.Context, ruleID string, feedback entities.RuleFeedbackType, userID string) (*entities.RuleLikeEntity, error) {
	if ruleID == "" {
		return nil, errors.New("ruleId is required")
	}
	if userID == "" {
		userID = "anonymous"
	}

	now := time.Now().UTC()
	entity := entities.NewRuleLikeEntity(entities.IRuleLike{
		ID:        uuid.New().String(),
		RuleID:    ruleID,
		UserID:    userID,
		Feedback:  feedback,
		CreatedAt: &now,
		UpdatedAt: &now,
	})

	if r.client != nil && r.client.Pool != nil {
		query := `
			INSERT INTO drixy_rule_likes (rule_id, user_id, feedback, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (rule_id, user_id) DO UPDATE
			SET feedback = EXCLUDED.feedback, updated_at = NOW()
			RETURNING id, rule_id, user_id, feedback, created_at, updated_at;
		`
		var id uuid.UUID
		var rID, uID, fb string
		var cAt, uAt time.Time
		err := r.client.Pool.QueryRow(ctx, query, ruleID, userID, string(feedback)).Scan(
			&id, &rID, &uID, &fb, &cAt, &uAt,
		)
		if err == nil {
			saved := entities.NewRuleLikeEntity(entities.IRuleLike{
				ID:        id.String(),
				RuleID:    rID,
				UserID:    uID,
				Feedback:  entities.RuleFeedbackType(fb),
				CreatedAt: &cAt,
				UpdatedAt: &uAt,
			})
			r.mu.Lock()
			r.likes[makeLikeKey(ruleID, userID)] = saved
			r.mu.Unlock()
			return saved, nil
		}
	}

	r.mu.Lock()
	r.likes[makeLikeKey(ruleID, userID)] = entity
	r.mu.Unlock()
	return entity, nil
}

// FindOne retrieves existing feedback for a specific user and rule.
func (r *PostgresRuleLikeRepository) FindOne(ctx context.Context, ruleID, userID string) (*entities.RuleLikeEntity, error) {
	if ruleID == "" || userID == "" {
		return nil, nil
	}

	if r.client != nil && r.client.Pool != nil {
		query := `
			SELECT id, rule_id, user_id, feedback, created_at, updated_at
			FROM drixy_rule_likes
			WHERE rule_id = $1 AND user_id = $2
			LIMIT 1;
		`
		var id uuid.UUID
		var rID, uID, fb string
		var cAt, uAt time.Time
		err := r.client.Pool.QueryRow(ctx, query, ruleID, userID).Scan(
			&id, &rID, &uID, &fb, &cAt, &uAt,
		)
		if err == nil {
			return entities.NewRuleLikeEntity(entities.IRuleLike{
				ID:        id.String(),
				RuleID:    rID,
				UserID:    uID,
				Feedback:  entities.RuleFeedbackType(fb),
				CreatedAt: &cAt,
				UpdatedAt: &uAt,
			}), nil
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	if found, ok := r.likes[makeLikeKey(ruleID, userID)]; ok {
		return found, nil
	}
	return nil, nil
}

// CountByRule computes net positive feedback for a rule.
func (r *PostgresRuleLikeRepository) CountByRule(ctx context.Context, ruleID string) (int, error) {
	if ruleID == "" {
		return 0, nil
	}

	if r.client != nil && r.client.Pool != nil {
		query := `
			SELECT COUNT(*)
			FROM drixy_rule_likes
			WHERE rule_id = $1 AND feedback = 'positive';
		`
		var cnt int
		err := r.client.Pool.QueryRow(ctx, query, ruleID).Scan(&cnt)
		if err == nil {
			return cnt, nil
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, l := range r.likes {
		if l.RuleID() == ruleID && l.Feedback() == entities.RuleFeedbackTypePositive {
			count++
		}
	}
	return count, nil
}

// TopByLanguage ranks rules by like counts.
func (r *PostgresRuleLikeRepository) TopByLanguage(ctx context.Context, language string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 10
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	counts := make(map[string]int)
	for _, l := range r.likes {
		if l.Feedback() == entities.RuleFeedbackTypePositive {
			counts[l.RuleID()]++
		}
	}

	var results []map[string]any
	for ruleID, cnt := range counts {
		results = append(results, map[string]any{
			"ruleId": ruleID,
			"count":  cnt,
		})
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

// GetAllRulesWithFeedback generates aggregated vote summaries across all rules.
func (r *PostgresRuleLikeRepository) GetAllRulesWithFeedback(ctx context.Context, userID string) ([]contracts.RuleFeedbackSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	byRule := make(map[string]*contracts.RuleFeedbackSummary)
	for _, l := range r.likes {
		s, ok := byRule[l.RuleID()]
		if !ok {
			s = &contracts.RuleFeedbackSummary{RuleID: l.RuleID()}
			byRule[l.RuleID()] = s
		}
		if l.Feedback() == entities.RuleFeedbackTypePositive {
			s.PositiveCount++
		} else if l.Feedback() == entities.RuleFeedbackTypeNegative {
			s.NegativeCount++
		}
		if userID != "" && l.UserID() == userID {
			fb := l.Feedback()
			s.UserFeedback = &fb
		}
	}

	var list []contracts.RuleFeedbackSummary
	for _, s := range byRule {
		list = append(list, *s)
	}
	return list, nil
}

// Unlike deletes user feedback for a rule.
func (r *PostgresRuleLikeRepository) Unlike(ctx context.Context, ruleID, userID string) (bool, error) {
	if ruleID == "" || userID == "" {
		return false, nil
	}

	if r.client != nil && r.client.Pool != nil {
		query := `
			DELETE FROM drixy_rule_likes
			WHERE rule_id = $1 AND user_id = $2;
		`
		_, _ = r.client.Pool.Exec(ctx, query, ruleID, userID)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	key := makeLikeKey(ruleID, userID)
	if _, ok := r.likes[key]; ok {
		delete(r.likes, key)
		return true, nil
	}
	return false, nil
}

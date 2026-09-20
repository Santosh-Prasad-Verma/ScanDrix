// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: postgres_drixy_rules_repository.go
// ═══════════════════════════════════════════════════════════════

package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// PostgresDrixyRulesRepository implements IDrixyRulesRepository with PostgreSQL JSONB storage and thread-safe in-memory fallback.
type PostgresDrixyRulesRepository struct {
	client    *database.Client
	mu        sync.RWMutex
	memStore  map[string]*entities.DrixyRulesEntity // keyed by organizationId
	byRuleID  map[string]*interfaces.DrixyRule
	initOnce  sync.Once
}

// NewPostgresDrixyRulesRepository constructs a repository instance.
func NewPostgresDrixyRulesRepository(client *database.Client) *PostgresDrixyRulesRepository {
	repo := &PostgresDrixyRulesRepository{
		client:   client,
		memStore: make(map[string]*entities.DrixyRulesEntity),
		byRuleID: make(map[string]*interfaces.DrixyRule),
	}
	repo.ensureSchema(context.Background())
	return repo
}

func (r *PostgresDrixyRulesRepository) ensureSchema(ctx context.Context) {
	if r.client == nil || r.client.Pool == nil {
		return
	}
	r.initOnce.Do(func() {
		query := `
			CREATE TABLE IF NOT EXISTS drixy_rules (
				id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				organization_id VARCHAR(255) NOT NULL,
				rules JSONB NOT NULL DEFAULT '[]'::jsonb,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				CONSTRAINT uq_drixy_rules_org UNIQUE (organization_id)
			);
			CREATE INDEX IF NOT EXISTS idx_drixy_rules_org ON drixy_rules(organization_id);
			CREATE INDEX IF NOT EXISTS idx_drixy_rules_gin ON drixy_rules USING gin (rules);
		`
		_, _ = r.client.Pool.Exec(ctx, query)
	})
}

// Create registers or initializes a rule aggregate for an organization.
func (r *PostgresDrixyRulesRepository) Create(ctx context.Context, drixyRules *interfaces.DrixyRules) (*entities.DrixyRulesEntity, error) {
	if drixyRules == nil || drixyRules.OrganizationID == "" {
		return nil, errors.New("invalid drixy rules data: missing organization ID")
	}

	entity := entities.NewDrixyRulesEntity(*drixyRules)

	if r.client != nil && r.client.Pool != nil {
		rulesJSON, err := json.Marshal(entity.Rules())
		if err != nil {
			return nil, fmt.Errorf("failed marshaling rules: %w", err)
		}

		query := `
			INSERT INTO drixy_rules (organization_id, rules, created_at, updated_at)
			VALUES ($1, $2, NOW(), NOW())
			ON CONFLICT (organization_id) DO UPDATE
			SET rules = EXCLUDED.rules, updated_at = NOW()
			RETURNING id, organization_id, rules, created_at, updated_at;
		`
		var id uuid.UUID
		var orgID string
		var rawRules []byte
		var createdAt, updatedAt time.Time

		err = r.client.Pool.QueryRow(ctx, query, entity.OrganizationID(), rulesJSON).Scan(
			&id, &orgID, &rawRules, &createdAt, &updatedAt,
		)
		if err == nil {
			var parsed []interfaces.DrixyRule
			_ = json.Unmarshal(rawRules, &parsed)
			savedEntity := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
				UUID:           id.String(),
				OrganizationID: orgID,
				Rules:          parsed,
				CreatedAt:      &createdAt,
				UpdatedAt:      &updatedAt,
			})
			r.updateMemoryCache(savedEntity)
			return savedEntity, nil
		}
	}

	// In-memory fallback
	r.updateMemoryCache(entity)
	return entity, nil
}

// FindByID locates a single rule by its embedded UUID.
func (r *PostgresDrixyRulesRepository) FindByID(ctx context.Context, ruleUUID string) (*interfaces.DrixyRule, error) {
	if ruleUUID == "" {
		return nil, errors.New("rule uuid is required")
	}

	if r.client != nil && r.client.Pool != nil {
		query := `
			SELECT elem
			FROM drixy_rules, jsonb_array_elements(rules) elem
			WHERE elem->>'uuid' = $1
			LIMIT 1;
		`
		var raw []byte
		err := r.client.Pool.QueryRow(ctx, query, ruleUUID).Scan(&raw)
		if err == nil {
			var rule interfaces.DrixyRule
			if err := json.Unmarshal(raw, &rule); err == nil {
				return &rule, nil
			}
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	if rule, ok := r.byRuleID[ruleUUID]; ok {
		copied := *rule
		return &copied, nil
	}

	return nil, nil
}

// FindByOrganizationID retrieves all rules belonging to an organization.
func (r *PostgresDrixyRulesRepository) FindByOrganizationID(ctx context.Context, organizationID string) (*entities.DrixyRulesEntity, error) {
	if organizationID == "" {
		return nil, errors.New("organization ID is required")
	}

	if r.client != nil && r.client.Pool != nil {
		query := `
			SELECT id, organization_id, rules, created_at, updated_at
			FROM drixy_rules
			WHERE organization_id = $1
			LIMIT 1;
		`
		var id uuid.UUID
		var orgID string
		var rawRules []byte
		var createdAt, updatedAt time.Time

		err := r.client.Pool.QueryRow(ctx, query, organizationID).Scan(
			&id, &orgID, &rawRules, &createdAt, &updatedAt,
		)
		if err == nil {
			var parsed []interfaces.DrixyRule
			_ = json.Unmarshal(rawRules, &parsed)
			entity := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
				UUID:           id.String(),
				OrganizationID: orgID,
				Rules:          parsed,
				CreatedAt:      &createdAt,
				UpdatedAt:      &updatedAt,
			})
			r.updateMemoryCache(entity)
			return entity, nil
		}
		// Query failure or not found: fall through to in-memory cache
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	if entity, ok := r.memStore[organizationID]; ok {
		return entity, nil
	}

	return nil, nil
}

// Find returns matching entity collections by organization and optional filter.
func (r *PostgresDrixyRulesRepository) Find(ctx context.Context, organizationID string, filter map[string]any) ([]*entities.DrixyRulesEntity, error) {
	entity, err := r.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return []*entities.DrixyRulesEntity{}, nil
	}
	return []*entities.DrixyRulesEntity{entity}, nil
}

// FindOrganizationIDsWithRules identifies all organizations having at least one active rule.
func (r *PostgresDrixyRulesRepository) FindOrganizationIDsWithRules(ctx context.Context) ([]string, error) {
	if r.client != nil && r.client.Pool != nil {
		query := `
			SELECT organization_id
			FROM drixy_rules
			WHERE jsonb_array_length(rules) > 0;
		`
		rows, err := r.client.Pool.Query(ctx, query)
		if err == nil {
			defer rows.Close()
			var orgs []string
			for rows.Next() {
				var org string
				if err := rows.Scan(&org); err == nil {
					orgs = append(orgs, org)
				}
			}
			return orgs, nil
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	var orgs []string
	for orgID, ent := range r.memStore {
		if len(ent.Rules()) > 0 {
			orgs = append(orgs, orgID)
		}
	}
	return orgs, nil
}

// CountRules returns total rules matching an optional status.
func (r *PostgresDrixyRulesRepository) CountRules(ctx context.Context, organizationID string, status *interfaces.DrixyRulesStatus) (int, error) {
	entity, err := r.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return 0, nil
	}

	count := 0
	for _, rule := range entity.Rules() {
		if status == nil || rule.Status == *status {
			count++
		}
	}
	return count, nil
}

// CountRulesByRepository aggregates rules per repository and optional directory.
func (r *PostgresDrixyRulesRepository) CountRulesByRepository(ctx context.Context, organizationID string, statuses []interfaces.DrixyRulesStatus) ([]contracts.RepositoryRuleCount, error) {
	entity, err := r.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return []contracts.RepositoryRuleCount{}, nil
	}

	allowedStatuses := make(map[interfaces.DrixyRulesStatus]bool)
	if len(statuses) == 0 {
		allowedStatuses[interfaces.DrixyRulesStatusActive] = true
		allowedStatuses[interfaces.DrixyRulesStatusPaused] = true
	} else {
		for _, s := range statuses {
			allowedStatuses[s] = true
		}
	}

	counts := make(map[string]map[string]int) // repoID -> dirID -> count
	for _, rule := range entity.Rules() {
		if !allowedStatuses[rule.Status] {
			continue
		}
		repoID := rule.RepositoryID
		if repoID == "" {
			repoID = "global"
		}
		dirKey := ""
		if rule.DirectoryID != "" {
			dirKey = rule.DirectoryID
		}
		if counts[repoID] == nil {
			counts[repoID] = make(map[string]int)
		}
		counts[repoID][dirKey]++
	}

	var results []contracts.RepositoryRuleCount
	for repoID, dirMap := range counts {
		for dirID, cnt := range dirMap {
			var dID *string
			if dirID != "" {
				tmp := dirID
				dID = &tmp
			}
			results = append(results, contracts.RepositoryRuleCount{
				RepositoryID: repoID,
				DirectoryID:  dID,
				Count:        cnt,
			})
		}
	}

	return results, nil
}

// Save commits the state of an existing rules aggregate.
func (r *PostgresDrixyRulesRepository) Save(ctx context.Context, entity *entities.DrixyRulesEntity) error {
	if entity == nil || entity.OrganizationID() == "" {
		return errors.New("invalid entity for save")
	}

	if r.client != nil && r.client.Pool != nil {
		rulesJSON, err := json.Marshal(entity.Rules())
		if err != nil {
			return fmt.Errorf("failed marshaling rules: %w", err)
		}

		query := `
			INSERT INTO drixy_rules (organization_id, rules, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (organization_id) DO UPDATE
			SET rules = EXCLUDED.rules, updated_at = NOW();
		`
		_, err = r.client.Pool.Exec(ctx, query, entity.OrganizationID(), rulesJSON)
		if err != nil {
			return fmt.Errorf("failed executing save query: %w", err)
		}
	}

	r.updateMemoryCache(entity)
	return nil
}

// DeleteRule removes a rule from an organization's rule list.
func (r *PostgresDrixyRulesRepository) DeleteRule(ctx context.Context, organizationID string, ruleUUID string) error {
	entity, err := r.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return nil
	}

	var retained []interfaces.DrixyRule
	for _, rule := range entity.Rules() {
		if rule.UUID != ruleUUID {
			retained = append(retained, rule)
		}
	}

	updatedEntity := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		UUID:           entity.UUID(),
		OrganizationID: entity.OrganizationID(),
		Rules:          retained,
		CreatedAt:      entity.CreatedAt(),
	})

	return r.Save(ctx, updatedEntity)
}

func (r *PostgresDrixyRulesRepository) updateMemoryCache(entity *entities.DrixyRulesEntity) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.memStore[entity.OrganizationID()] = entity
	for _, rule := range entity.Rules() {
		if rule.UUID != "" {
			cp := rule
			r.byRuleID[rule.UUID] = &cp
		}
	}
}

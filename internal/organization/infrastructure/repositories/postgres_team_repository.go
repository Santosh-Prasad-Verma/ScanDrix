package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/database"
	teamdomain "github.com/scandrix/backend/internal/organization/domain/team"
)

// PostgresTeamRepository implements ITeamRepository with PostgreSQL and in-memory fallback.
type PostgresTeamRepository struct {
	client *database.Client
	pool   *pgxpool.Pool
	mu     sync.RWMutex
	memory map[uuid.UUID]*teamdomain.TeamEntity
}

// NewPostgresTeamRepository creates a new repository.
func NewPostgresTeamRepository(pool *pgxpool.Pool) *PostgresTeamRepository {
	return &PostgresTeamRepository{
		client: &database.Client{Pool: pool},
		pool:   pool,
		memory: make(map[uuid.UUID]*teamdomain.TeamEntity),
	}
}

// Find retrieves teams matching the filter.
func (r *PostgresTeamRepository) Find(ctx context.Context, filter teamdomain.TeamFilter) ([]*teamdomain.TeamEntity, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		var res []*teamdomain.TeamEntity
		for _, team := range r.memory {
			if filter.UUID != nil && team.UUID != *filter.UUID {
				continue
			}
			if filter.WorkspaceID != nil && team.WorkspaceID != *filter.WorkspaceID {
				continue
			}
			if filter.Name != nil && team.Name != *filter.Name {
				continue
			}
			if filter.Status != nil && team.Status != *filter.Status {
				continue
			}
			res = append(res, team)
		}
		return res, nil
	}

	if filter.WorkspaceID == nil {
		return nil, ErrTenantRequired
	}

	query := `
		SELECT id, workspace_id, name, description, created_at, updated_at
		FROM teams
		WHERE ($1::uuid IS NULL OR id = $1)
		  AND ($2::uuid IS NULL OR workspace_id = $2)
		  AND ($3::varchar IS NULL OR name = $3)
		ORDER BY created_at ASC
	`
	var filterID, filterWsID *uuid.UUID
	if filter.UUID != nil {
		filterID = filter.UUID
	}
	if filter.WorkspaceID != nil {
		filterWsID = filter.WorkspaceID
	}

	var list []*teamdomain.TeamEntity
	err := r.client.ExecWithTenant(ctx, *filter.WorkspaceID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, filterID, filterWsID, filter.Name)
		if err != nil {
			return fmt.Errorf("failed to query teams: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var id, wsID uuid.UUID
			var name, desc string
			var createdAt, updatedAt time.Time

			if err := rows.Scan(&id, &wsID, &name, &desc, &createdAt, &updatedAt); err != nil {
				return fmt.Errorf("failed to scan team: %w", err)
			}

			list = append(list, &teamdomain.TeamEntity{
				UUID:           id,
				WorkspaceID:    wsID,
				Name:           name,
				Description:    desc,
				RepositoryIDs:  []uuid.UUID{},
				AutoAssignMode: "round_robin",
				Status:         true,
				CreatedAt:      createdAt,
				UpdatedAt:      updatedAt,
			})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

// FindOne returns a single team matching the filter.
func (r *PostgresTeamRepository) FindOne(ctx context.Context, filter teamdomain.TeamFilter) (*teamdomain.TeamEntity, error) {
	list, err := r.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}

// FindByID retrieves a team by its UUID within a workspace.
func (r *PostgresTeamRepository) FindByID(ctx context.Context, wsID, id uuid.UUID) (*teamdomain.TeamEntity, error) {
	return r.FindOne(ctx, teamdomain.TeamFilter{UUID: &id, WorkspaceID: &wsID})
}

// FindByWorkspaceID retrieves all teams belonging to a workspace.
func (r *PostgresTeamRepository) FindByWorkspaceID(ctx context.Context, workspaceID uuid.UUID) ([]*teamdomain.TeamEntity, error) {
	return r.Find(ctx, teamdomain.TeamFilter{WorkspaceID: &workspaceID})
}

// GetTeamsByUserID retrieves teams that include the specified user.
func (r *PostgresTeamRepository) GetTeamsByUserID(ctx context.Context, userID, workspaceID uuid.UUID) ([]*teamdomain.TeamEntity, error) {
	if r.pool == nil {
		return r.FindByWorkspaceID(ctx, workspaceID)
	}
	if workspaceID == uuid.Nil {
		return nil, ErrTenantRequired
	}

	query := `
		SELECT t.id, t.workspace_id, t.name, t.description, t.created_at, t.updated_at
		FROM teams t
		JOIN team_members tm ON tm.team_id = t.id
		WHERE tm.user_id = $1 AND t.workspace_id = $2
		ORDER BY t.created_at ASC
	`
	var list []*teamdomain.TeamEntity
	err := r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, userID, workspaceID)
		if err != nil {
			return fmt.Errorf("failed to get teams by user: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var id, wsID uuid.UUID
			var name, desc string
			var createdAt, updatedAt time.Time

			if err := rows.Scan(&id, &wsID, &name, &desc, &createdAt, &updatedAt); err != nil {
				return err
			}
			list = append(list, &teamdomain.TeamEntity{
				UUID:           id,
				WorkspaceID:    wsID,
				Name:           name,
				Description:    desc,
				RepositoryIDs:  []uuid.UUID{},
				AutoAssignMode: "round_robin",
				Status:         true,
				CreatedAt:      createdAt,
				UpdatedAt:      updatedAt,
			})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

// FindFirstCreatedTeam gets the default/initial team for a workspace.
func (r *PostgresTeamRepository) FindFirstCreatedTeam(ctx context.Context, workspaceID uuid.UUID) (*teamdomain.TeamEntity, error) {
	teams, err := r.FindByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if len(teams) == 0 {
		return nil, nil
	}
	return teams[0], nil
}

// Create stores a new team.
func (r *PostgresTeamRepository) Create(ctx context.Context, entity *teamdomain.TeamEntity) (*teamdomain.TeamEntity, error) {
	if entity == nil {
		return nil, errors.New("team entity cannot be nil")
	}
	if entity.WorkspaceID == uuid.Nil {
		return nil, ErrTenantRequired
	}
	if entity.UUID == uuid.Nil {
		entity.UUID = uuid.New()
	}
	now := time.Now().UTC()
	if entity.CreatedAt.IsZero() {
		entity.CreatedAt = now
	}
	entity.UpdatedAt = now

	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.memory[entity.UUID] = entity
		return entity, nil
	}

	query := `
		INSERT INTO teams (id, workspace_id, name, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			updated_at = EXCLUDED.updated_at
		RETURNING id, workspace_id, name, description, created_at, updated_at
	`
	var id, wsID uuid.UUID
	var name, desc string
	var createdAt, updatedAt time.Time

	err := r.client.ExecWithTenant(ctx, entity.WorkspaceID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, entity.UUID, entity.WorkspaceID, entity.Name, entity.Description, entity.CreatedAt, entity.UpdatedAt).
			Scan(&id, &wsID, &name, &desc, &createdAt, &updatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to insert team: %w", err)
	}

	entity.UUID = id
	entity.WorkspaceID = wsID
	entity.Name = name
	entity.Description = desc
	entity.CreatedAt = createdAt
	entity.UpdatedAt = updatedAt
	return entity, nil
}

// Update updates team attributes.
func (r *PostgresTeamRepository) Update(ctx context.Context, filter teamdomain.TeamFilter, data *teamdomain.TeamEntity) (*teamdomain.TeamEntity, error) {
	existing, err := r.FindOne(ctx, filter)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("team not found to update")
	}

	existing.Name = data.Name
	existing.Description = data.Description
	existing.AutoAssignMode = data.AutoAssignMode
	existing.Status = data.Status
	existing.UpdatedAt = time.Now().UTC()

	return r.Create(ctx, existing)
}

// Delete removes a team by ID within a workspace.
func (r *PostgresTeamRepository) Delete(ctx context.Context, wsID, id uuid.UUID) error {
	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.memory, id)
		return nil
	}
	if wsID == uuid.Nil {
		return ErrTenantRequired
	}

	query := `DELETE FROM teams WHERE id = $1 AND workspace_id = $2`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, query, id, wsID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errors.New("team not found")
		}
		return nil
	})
}

// ListWithIntegrations retrieves teams with aggregated integration and member stats.
func (r *PostgresTeamRepository) ListWithIntegrations(ctx context.Context, workspaceID uuid.UUID) ([]*teamdomain.TeamWithIntegrations, error) {
	teams, err := r.FindByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if len(teams) == 0 {
		return []*teamdomain.TeamWithIntegrations{}, nil
	}

	if r.pool == nil {
		var results []*teamdomain.TeamWithIntegrations
		for _, t := range teams {
			results = append(results, &teamdomain.TeamWithIntegrations{
				Team:             t,
				MemberCount:      0,
				RepositoryCount:  len(t.RepositoryIDs),
				GitIntegrations:  []string{},
				PMIntegrations:   []string{},
				ChatIntegrations: []string{},
			})
		}
		return results, nil
	}

	memberCounts := make(map[uuid.UUID]int)
	var gitIntegrations []string
	var pmIntegrations []string
	var chatIntegrations []string

	err = r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		// 1. Query real member counts per team
		teamRows, err := tx.Query(ctx, `
			SELECT team_id, count(*) 
			FROM team_members 
			WHERE team_id IN (SELECT id FROM teams WHERE workspace_id = $1)
			GROUP BY team_id
		`, workspaceID)
		if err == nil {
			defer teamRows.Close()
			for teamRows.Next() {
				var tID uuid.UUID
				var count int
				if err := teamRows.Scan(&tID, &count); err == nil {
					memberCounts[tID] = count
				}
			}
		}

		// 2. Query real integration connections for this workspace
		intRows, err := tx.Query(ctx, `
			SELECT provider 
			FROM integration_connections 
			WHERE workspace_id = $1 AND is_connected = true
		`, workspaceID)
		if err == nil {
			defer intRows.Close()
			for intRows.Next() {
				var provider string
				if err := intRows.Scan(&provider); err == nil {
					p := strings.ToLower(provider)
					switch p {
					case "github", "gitlab", "bitbucket", "azure_devops":
						gitIntegrations = append(gitIntegrations, p)
					case "jira", "linear", "azure_boards":
						pmIntegrations = append(pmIntegrations, p)
					}
				}
			}
		}

		// 3. Query real notification channels (chat)
		notifRows, err := tx.Query(ctx, `
			SELECT type 
			FROM notification_channels 
			WHERE workspace_id = $1 AND enabled = true
		`, workspaceID)
		if err == nil {
			defer notifRows.Close()
			for notifRows.Next() {
				var cType string
				if err := notifRows.Scan(&cType); err == nil {
					chatIntegrations = append(chatIntegrations, strings.ToLower(cType))
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	var results []*teamdomain.TeamWithIntegrations
	for _, t := range teams {
		results = append(results, &teamdomain.TeamWithIntegrations{
			Team:             t,
			MemberCount:      memberCounts[t.UUID],
			RepositoryCount:  len(t.RepositoryIDs),
			GitIntegrations:  gitIntegrations,
			PMIntegrations:   pmIntegrations,
			ChatIntegrations: chatIntegrations,
		})
	}
	return results, nil
}

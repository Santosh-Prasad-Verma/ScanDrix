package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/database"
	memberdomain "github.com/scandrix/backend/internal/organization/domain/teammembers"
)

// PostgresTeamMemberRepository implements ITeamMembersRepository with PostgreSQL and in-memory fallback.
type PostgresTeamMemberRepository struct {
	client *database.Client
	pool   *pgxpool.Pool
	mu     sync.RWMutex
	memory map[string]*memberdomain.TeamMemberEntity // key: teamID:userID
}

// NewPostgresTeamMemberRepository creates a new repository.
func NewPostgresTeamMemberRepository(pool *pgxpool.Pool) *PostgresTeamMemberRepository {
	return &PostgresTeamMemberRepository{
		client: &database.Client{Pool: pool},
		pool:   pool,
		memory: make(map[string]*memberdomain.TeamMemberEntity),
	}
}

func memberKey(teamID, userID uuid.UUID) string {
	return fmt.Sprintf("%s:%s", teamID.String(), userID.String())
}

// Find retrieves team members matching the filter.
func (r *PostgresTeamMemberRepository) Find(ctx context.Context, filter memberdomain.TeamMemberFilter) ([]*memberdomain.TeamMemberEntity, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		var res []*memberdomain.TeamMemberEntity
		for _, m := range r.memory {
			if filter.UUID != nil && m.UUID != *filter.UUID {
				continue
			}
			if filter.WorkspaceID != nil && m.WorkspaceID != *filter.WorkspaceID {
				continue
			}
			if filter.TeamID != nil && m.TeamID != *filter.TeamID {
				continue
			}
			if filter.UserID != nil && m.UserID != *filter.UserID {
				continue
			}
			if filter.Email != nil && m.Email != *filter.Email {
				continue
			}
			if filter.Role != nil && m.Role != *filter.Role {
				continue
			}
			if filter.Status != nil && m.Status != *filter.Status {
				continue
			}
			res = append(res, m)
		}
		return res, nil
	}

	if filter.WorkspaceID == nil {
		return nil, ErrTenantRequired
	}

	query := `
		SELECT tm.id, tm.team_id, tm.user_id, COALESCE(tm.email, ''), tm.role, tm.created_at, t.workspace_id,
		       COALESCE(tm.name, tm.email, ''), COALESCE(tm.avatar, ''), COALESCE(tm.status, true),
		       COALESCE(tm.code_management, '{}'::jsonb), COALESCE(tm.communication, '{}'::jsonb),
		       COALESCE(tm.project_management, '{}'::jsonb), COALESCE(tm.communication_id, ''),
		       COALESCE(tm.review_count, 0)
		FROM team_members tm
		JOIN teams t ON t.id = tm.team_id
		WHERE ($1::uuid IS NULL OR tm.id = $1)
		  AND ($2::uuid IS NULL OR tm.team_id = $2)
		  AND ($3::uuid IS NULL OR tm.user_id = $3)
		  AND ($4::varchar IS NULL OR tm.email = $4)
		  AND ($5::uuid IS NULL OR t.workspace_id = $5)
		  AND ($6::boolean IS NULL OR tm.status = $6)
		ORDER BY tm.created_at ASC
	`
	var filterID, filterTeamID, filterUserID, filterWsID *uuid.UUID
	if filter.UUID != nil {
		filterID = filter.UUID
	}
	if filter.TeamID != nil {
		filterTeamID = filter.TeamID
	}
	if filter.UserID != nil {
		filterUserID = filter.UserID
	}
	if filter.WorkspaceID != nil {
		filterWsID = filter.WorkspaceID
	}

	var list []*memberdomain.TeamMemberEntity
	err := r.client.ExecWithTenant(ctx, *filter.WorkspaceID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, filterID, filterTeamID, filterUserID, filter.Email, filterWsID, filter.Status)
		if err != nil {
			return fmt.Errorf("failed to query team_members: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var id, teamID, userID, wsID uuid.UUID
			var email, roleStr, name, avatar, commID string
			var status bool
			var reviewCount int
			var codeMgmtRaw, commRaw, pmRaw []byte
			var createdAt time.Time

			if err := rows.Scan(
				&id, &teamID, &userID, &email, &roleStr, &createdAt, &wsID,
				&name, &avatar, &status, &codeMgmtRaw, &commRaw, &pmRaw, &commID, &reviewCount,
			); err != nil {
				return fmt.Errorf("failed to scan team_member: %w", err)
			}

			entity := &memberdomain.TeamMemberEntity{
				UUID:            id,
				WorkspaceID:     wsID,
				TeamID:          teamID,
				UserID:          userID,
				Email:           email,
				Name:            name,
				Role:            memberdomain.TeamMemberRole(roleStr),
				Avatar:          avatar,
				Status:          status,
				CommunicationID: commID,
				ReviewCount:     reviewCount,
				JoinedAt:        createdAt,
			}

			if len(codeMgmtRaw) > 0 && string(codeMgmtRaw) != "{}" {
				var cm memberdomain.CodeManagementMemberConfig
				if err := json.Unmarshal(codeMgmtRaw, &cm); err == nil {
					entity.CodeManagement = &cm
				}
			}
			if len(commRaw) > 0 && string(commRaw) != "{}" {
				var c memberdomain.CommunicationMemberConfig
				if err := json.Unmarshal(commRaw, &c); err == nil {
					entity.Communication = &c
				}
			}
			if len(pmRaw) > 0 && string(pmRaw) != "{}" {
				var pm memberdomain.ProjectManagementMemberConfig
				if err := json.Unmarshal(pmRaw, &pm); err == nil {
					entity.ProjectManagement = &pm
				}
			}

			list = append(list, entity)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

// FindOne returns a single team member matching the filter.
func (r *PostgresTeamMemberRepository) FindOne(ctx context.Context, filter memberdomain.TeamMemberFilter) (*memberdomain.TeamMemberEntity, error) {
	list, err := r.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}

// FindByID retrieves a team member by UUID within a workspace.
func (r *PostgresTeamMemberRepository) FindByID(ctx context.Context, wsID, id uuid.UUID) (*memberdomain.TeamMemberEntity, error) {
	return r.FindOne(ctx, memberdomain.TeamMemberFilter{UUID: &id, WorkspaceID: &wsID})
}

// FindManyByWorkspaceID retrieves all members within a workspace.
func (r *PostgresTeamMemberRepository) FindManyByWorkspaceID(ctx context.Context, wsID uuid.UUID) ([]*memberdomain.TeamMemberEntity, error) {
	return r.Find(ctx, memberdomain.TeamMemberFilter{WorkspaceID: &wsID})
}

// FindManyByUserID retrieves all team memberships for a given user in a workspace.
func (r *PostgresTeamMemberRepository) FindManyByUserID(ctx context.Context, wsID, userID uuid.UUID) ([]*memberdomain.TeamMemberEntity, error) {
	return r.Find(ctx, memberdomain.TeamMemberFilter{WorkspaceID: &wsID, UserID: &userID})
}

// FindMembersByCommunicationID retrieves team members associated with a specific chat or communication handle.
func (r *PostgresTeamMemberRepository) FindMembersByCommunicationID(ctx context.Context, wsID uuid.UUID, communicationID string) ([]*memberdomain.TeamMemberEntity, error) {
	if communicationID == "" {
		return nil, nil
	}
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		var res []*memberdomain.TeamMemberEntity
		for _, m := range r.memory {
			if (wsID == uuid.Nil || m.WorkspaceID == wsID) && m.CommunicationID == communicationID && m.Status {
				res = append(res, m)
			}
		}
		return res, nil
	}
	if wsID == uuid.Nil {
		return nil, ErrTenantRequired
	}

	query := `
		SELECT tm.id, tm.team_id, tm.user_id, COALESCE(tm.email, ''), tm.role, tm.created_at, t.workspace_id,
		       COALESCE(tm.name, tm.email, ''), COALESCE(tm.avatar, ''), COALESCE(tm.status, true),
		       COALESCE(tm.code_management, '{}'::jsonb), COALESCE(tm.communication, '{}'::jsonb),
		       COALESCE(tm.project_management, '{}'::jsonb), COALESCE(tm.communication_id, ''),
		       COALESCE(tm.review_count, 0)
		FROM team_members tm
		JOIN teams t ON t.id = tm.team_id
		WHERE tm.communication_id = $1 AND t.workspace_id = $2 AND tm.status = true
		ORDER BY tm.created_at ASC
	`
	var list []*memberdomain.TeamMemberEntity
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, communicationID, wsID)
		if err != nil {
			return fmt.Errorf("failed to query members by communication ID: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var id, teamID, userID, wID uuid.UUID
			var email, roleStr, name, avatar, commID string
			var status bool
			var reviewCount int
			var codeMgmtRaw, commRaw, pmRaw []byte
			var createdAt time.Time

			if err := rows.Scan(
				&id, &teamID, &userID, &email, &roleStr, &createdAt, &wID,
				&name, &avatar, &status, &codeMgmtRaw, &commRaw, &pmRaw, &commID, &reviewCount,
			); err != nil {
				return fmt.Errorf("failed to scan member: %w", err)
			}

			entity := &memberdomain.TeamMemberEntity{
				UUID:            id,
				WorkspaceID:     wID,
				TeamID:          teamID,
				UserID:          userID,
				Email:           email,
				Name:            name,
				Role:            memberdomain.TeamMemberRole(roleStr),
				Avatar:          avatar,
				Status:          status,
				CommunicationID: commID,
				ReviewCount:     reviewCount,
				JoinedAt:        createdAt,
			}
			list = append(list, entity)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

// CountByUser counts how many teams a user belongs to within a workspace.
func (r *PostgresTeamMemberRepository) CountByUser(ctx context.Context, wsID, userID uuid.UUID, teamMemberStatus *bool) (int, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		count := 0
		for _, m := range r.memory {
			if m.UserID == userID && (wsID == uuid.Nil || m.WorkspaceID == wsID) {
				if teamMemberStatus == nil || m.Status == *teamMemberStatus {
					count++
				}
			}
		}
		return count, nil
	}
	if wsID == uuid.Nil {
		return 0, ErrTenantRequired
	}

	query := `
		SELECT COUNT(*) FROM team_members tm
		JOIN teams t ON t.id = tm.team_id
		WHERE tm.user_id = $1 AND t.workspace_id = $2 AND ($3::boolean IS NULL OR tm.status = $3)
	`
	var count int
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, userID, wsID, teamMemberStatus).Scan(&count)
	})
	return count, err
}

// CountTeamMembers counts members within a specific team.
func (r *PostgresTeamMemberRepository) CountTeamMembers(ctx context.Context, wsID, teamID uuid.UUID) (int, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		count := 0
		for _, m := range r.memory {
			if m.TeamID == teamID && m.Status {
				count++
			}
		}
		return count, nil
	}
	if wsID == uuid.Nil {
		return 0, ErrTenantRequired
	}

	query := `
		SELECT COUNT(*) FROM team_members tm
		JOIN teams t ON t.id = tm.team_id
		WHERE tm.team_id = $1 AND t.workspace_id = $2 AND tm.status = true
	`
	var count int
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, teamID, wsID).Scan(&count)
	})
	return count, err
}

// FindManyByOrganizationID returns all team members across an organization.
func (r *PostgresTeamMemberRepository) FindManyByOrganizationID(ctx context.Context, wsID uuid.UUID, teamStatus []string) ([]*memberdomain.TeamMemberEntity, error) {
	filter := memberdomain.TeamMemberFilter{
		WorkspaceID: &wsID,
	}
	return r.Find(ctx, filter)
}

// Create inserts or updates a team member with full integration identities.
func (r *PostgresTeamMemberRepository) Create(ctx context.Context, entity *memberdomain.TeamMemberEntity) (*memberdomain.TeamMemberEntity, error) {
	if entity == nil {
		return nil, errors.New("team member entity cannot be nil")
	}
	if entity.WorkspaceID == uuid.Nil {
		return nil, ErrTenantRequired
	}
	if entity.UUID == uuid.Nil {
		entity.UUID = uuid.New()
	}
	now := time.Now().UTC()
	if entity.JoinedAt.IsZero() {
		entity.JoinedAt = now
	}

	codeMgmtBytes, _ := json.Marshal(entity.CodeManagement)
	if len(codeMgmtBytes) == 0 || string(codeMgmtBytes) == "null" {
		codeMgmtBytes = []byte("{}")
	}
	commBytes, _ := json.Marshal(entity.Communication)
	if len(commBytes) == 0 || string(commBytes) == "null" {
		commBytes = []byte("{}")
	}
	pmBytes, _ := json.Marshal(entity.ProjectManagement)
	if len(pmBytes) == 0 || string(pmBytes) == "null" {
		pmBytes = []byte("{}")
	}

	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.memory[memberKey(entity.TeamID, entity.UserID)] = entity
		return entity, nil
	}

	query := `
		INSERT INTO team_members (
			id, team_id, user_id, email, role, created_at,
			name, avatar, status, code_management, communication, project_management,
			communication_id, review_count
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (team_id, user_id) DO UPDATE SET
			email = EXCLUDED.email,
			role = EXCLUDED.role,
			name = EXCLUDED.name,
			avatar = EXCLUDED.avatar,
			status = EXCLUDED.status,
			code_management = EXCLUDED.code_management,
			communication = EXCLUDED.communication,
			project_management = EXCLUDED.project_management,
			communication_id = EXCLUDED.communication_id,
			review_count = EXCLUDED.review_count
		RETURNING id, team_id, user_id, email, role, created_at, name, avatar, status, communication_id, review_count
	`
	var id, teamID, userID uuid.UUID
	var email, roleStr, name, avatar, commID string
	var status bool
	var reviewCount int
	var createdAt time.Time

	err := r.client.ExecWithTenant(ctx, entity.WorkspaceID, func(tx pgx.Tx) error {
		return tx.QueryRow(
			ctx, query,
			entity.UUID, entity.TeamID, entity.UserID, entity.Email, string(entity.Role), entity.JoinedAt,
			entity.Name, entity.Avatar, entity.Status, codeMgmtBytes, commBytes, pmBytes,
			entity.CommunicationID, entity.ReviewCount,
		).Scan(&id, &teamID, &userID, &email, &roleStr, &createdAt, &name, &avatar, &status, &commID, &reviewCount)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upsert team_member: %w", err)
	}

	entity.UUID = id
	entity.TeamID = teamID
	entity.UserID = userID
	entity.Email = email
	entity.Role = memberdomain.TeamMemberRole(roleStr)
	entity.JoinedAt = createdAt
	entity.Name = name
	entity.Avatar = avatar
	entity.Status = status
	entity.CommunicationID = commID
	entity.ReviewCount = reviewCount
	return entity, nil
}

// Update updates team member attributes.
func (r *PostgresTeamMemberRepository) Update(ctx context.Context, filter memberdomain.TeamMemberFilter, data *memberdomain.TeamMemberEntity) (*memberdomain.TeamMemberEntity, error) {
	existing, err := r.FindOne(ctx, filter)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("team member not found to update")
	}

	existing.Role = data.Role
	existing.Email = data.Email
	existing.Status = data.Status
	existing.Name = data.Name
	existing.Avatar = data.Avatar
	existing.CommunicationID = data.CommunicationID
	if data.CodeManagement != nil {
		existing.CodeManagement = data.CodeManagement
	}
	if data.Communication != nil {
		existing.Communication = data.Communication
	}
	if data.ProjectManagement != nil {
		existing.ProjectManagement = data.ProjectManagement
	}

	return r.Create(ctx, existing)
}

// Delete removes a team member by team and user ID within a workspace.
func (r *PostgresTeamMemberRepository) Delete(ctx context.Context, wsID, teamID, userID uuid.UUID) error {
	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.memory, memberKey(teamID, userID))
		return nil
	}
	if wsID == uuid.Nil {
		return ErrTenantRequired
	}

	query := `
		DELETE FROM team_members tm
		USING teams t
		WHERE tm.team_id = t.id AND tm.team_id = $1 AND tm.user_id = $2 AND t.workspace_id = $3
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, teamID, userID, wsID)
		return err
	})
}

// DeleteMembers deletes a slice of team members by their primary UUIDs within a workspace.
func (r *PostgresTeamMemberRepository) DeleteMembers(ctx context.Context, wsID uuid.UUID, memberUUIDs []uuid.UUID) error {
	if len(memberUUIDs) == 0 {
		return nil
	}

	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, id := range memberUUIDs {
			for k, m := range r.memory {
				if m.UUID == id {
					delete(r.memory, k)
				}
			}
		}
		return nil
	}
	if wsID == uuid.Nil {
		return ErrTenantRequired
	}

	query := `
		DELETE FROM team_members tm
		USING teams t
		WHERE tm.team_id = t.id AND tm.id = ANY($1) AND t.workspace_id = $2
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, memberUUIDs, wsID)
		return err
	})
}

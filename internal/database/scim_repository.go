// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - SCIM 2.0 Directory Repository
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Schema note: the `users` table names its primary key `uuid` and its timestamps
// in camelCase ("createdAt"/"updatedAt"), while most other tables use snake_case
// with an `id` key. Queries below are written against the real column names
// rather than an assumed convention, because the mismatch only surfaces at
// runtime against a real database.

// SCIMUserRecord is the neutral row shape backing a SCIM user resource. It is
// declared here rather than reusing the SCIM wire types so this package never
// has to import the SCIM layer, which already depends on this one.
type SCIMUserRecord struct {
	ID         string
	UserName   string
	Email      string
	GivenName  string
	FamilyName string
	Active     bool
	Role       string
	CreatedAt  time.Time
	ModifiedAt time.Time
}

// ListSCIMUsers returns SCIM user resources for a workspace, optionally
// filtered by userName, with startIndex/count paging per RFC 7644 §3.4.2.4.
//
// Reads go through the tenant-scoped transaction so RLS applies: a caller can
// only ever see its own workspace's directory.
func (r *Repository) ListSCIMUsers(ctx context.Context, wsID uuid.UUID, userName string, startIndex, count int) ([]SCIMUserRecord, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	if wsID == uuid.Nil {
		return nil, fmt.Errorf("workspace scope is required")
	}
	if startIndex < 1 {
		startIndex = 1
	}
	if count <= 0 || count > 100 {
		count = 100
	}

	query := `
		SELECT uuid, email, role, status, "createdAt", "updatedAt"
		FROM users
		WHERE organization_id = $1
	`
	args := []any{wsID}
	if userName != "" {
		query += ` AND lower(email) = lower($2)`
		args = append(args, userName)
	}
	query += ` ORDER BY "createdAt" ASC LIMIT $` + fmt.Sprint(len(args)+1) + ` OFFSET $` + fmt.Sprint(len(args)+2)
	args = append(args, count, startIndex-1)

	var records []SCIMUserRecord
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var (
				id        uuid.UUID
				email     string
				role      string
				status    string
				createdAt time.Time
				updatedAt time.Time
			)
			if err := rows.Scan(&id, &email, &role, &status, &createdAt, &updatedAt); err != nil {
				return err
			}
			records = append(records, scimRecordFromRow(id, email, role, status, createdAt, updatedAt))
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed listing scim users: %w", err)
	}
	return records, nil
}

// CountSCIMUsers returns the total matching a filter, for the ListResponse's
// totalResults. Counted separately from the page so paging stays correct.
func (r *Repository) CountSCIMUsers(ctx context.Context, wsID uuid.UUID, userName string) (int, error) {
	if r == nil || r.client == nil {
		return 0, fmt.Errorf("database unavailable")
	}
	if wsID == uuid.Nil {
		return 0, fmt.Errorf("workspace scope is required")
	}

	query := `SELECT COUNT(*) FROM users WHERE organization_id = $1`
	args := []any{wsID}
	if userName != "" {
		query += ` AND lower(email) = lower($2)`
		args = append(args, userName)
	}

	var total int
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(&total)
	})
	if err != nil {
		return 0, fmt.Errorf("failed counting scim users: %w", err)
	}
	return total, nil
}

// GetSCIMUserByEmail looks a user up within a workspace, for the SCIM
// userName-eq lookup and for deprovisioning.
func (r *Repository) GetSCIMUserByEmail(ctx context.Context, wsID uuid.UUID, email string) (*SCIMUserRecord, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	if wsID == uuid.Nil {
		return nil, fmt.Errorf("workspace scope is required")
	}

	const query = `
		SELECT uuid, email, role, status, "createdAt", "updatedAt"
		FROM users
		WHERE organization_id = $1 AND lower(email) = lower($2);
	`

	var (
		rec       SCIMUserRecord
		id        uuid.UUID
		role      string
		status    string
		createdAt time.Time
		updatedAt time.Time
	)
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID, email).Scan(
			&id, &rec.Email, &role, &status, &createdAt, &updatedAt)
	})
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed reading scim user: %w", err)
	}

	rec.ID = id.String()
	rec.UserName = rec.Email
	rec.Role = role
	rec.Active = SCIMStatusIsActive(status)
	rec.CreatedAt = createdAt
	rec.ModifiedAt = updatedAt
	return &rec, nil
}

// scimRecordFromRow projects a users row onto the neutral record the SCIM layer
// renders. The email doubles as userName, which is what every IdP sends and what
// the unique index on lower(email) makes unambiguous.
func scimRecordFromRow(id uuid.UUID, email, role, status string, createdAt, updatedAt time.Time) SCIMUserRecord {
	return SCIMUserRecord{
		ID:         id.String(),
		UserName:   email,
		Email:      email,
		Role:       role,
		Active:     SCIMStatusIsActive(status),
		CreatedAt:  createdAt,
		ModifiedAt: updatedAt,
	}
}

// SCIMStatusIsActive maps an internal user status onto SCIM's active flag. Only
// an explicitly inactive or suspended account reports active=false; every other
// state counts as active so a status rename cannot silently deactivate an entire
// directory.
func SCIMStatusIsActive(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "inactive", "suspended", "disabled", "deleted":
		return false
	default:
		return true
	}
}

// ResolveSCIMWorkspace returns the workspace a single-tenant SCIM connection
// provisions into.
//
// SCIM is bound to one organization: an IdP holds one bearer token and pushes
// its own directory, which maps to one workspace. A self-hosted deployment
// normally has exactly one, so the oldest is the tenant. The lookup runs as the
// system worker because no session exists yet at boot, and it returns uuid.Nil
// when the instance has no workspace at all rather than guessing.
//
// A multi-tenant SaaS deployment must not rely on this: it should issue a
// per-tenant SCIM bearer and bind the service per tenant instead.
// ResolveSCIMWorkspace is retained only for the single-tenant self-hosted case,
// and returns an error whenever more than one workspace exists. The previous
// implementation returned the oldest workspace unconditionally, which on a
// multi-tenant deployment bound every SCIM connection to the same customer
// regardless of which one had configured it.
//
// Callers should prefer ResolveSCIMWorkspaceByToken, which resolves the tenant
// from the credential that was actually presented.
func (r *Repository) ResolveSCIMWorkspace(ctx context.Context) (uuid.UUID, error) {
	if r == nil || r.client == nil {
		return uuid.Nil, fmt.Errorf("database unavailable")
	}

	// workspaces uses snake_case columns, while the users table uses camelCase.
	// The schema is not uniform, so each query is written against the columns
	// that table actually has rather than an assumed convention.
	const query = `SELECT id FROM workspaces ORDER BY created_at ASC;`

	var ids []uuid.UUID
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed resolving scim workspace: %w", err)
	}

	switch len(ids) {
	case 0:
		return uuid.Nil, nil
	case 1:
		return ids[0], nil
	default:
		// Ambiguous. Guessing here is what caused the cross-tenant bug, so this
		// fails closed and the caller must resolve by token instead.
		return uuid.Nil, fmt.Errorf(
			"ambiguous scim tenant: %d workspaces exist; resolve by provisioning token", len(ids))
	}
}

// ResolveSCIMWorkspaceByToken maps a presented SCIM bearer token to the
// workspace that owns it.
//
// tokenHash is the SHA-256 hex of the presented token; the plaintext is never
// stored or logged. Returns uuid.Nil when no workspace holds that token, which
// the SCIM layer treats as unauthorized.
func (r *Repository) ResolveSCIMWorkspaceByToken(ctx context.Context, tokenHash string) (uuid.UUID, error) {
	if r == nil || r.client == nil {
		return uuid.Nil, fmt.Errorf("database unavailable")
	}
	hash := strings.ToLower(strings.TrimSpace(tokenHash))
	if hash == "" {
		return uuid.Nil, nil
	}

	const query = `SELECT id FROM workspaces WHERE scim_token_hash = $1 LIMIT 1;`

	var wsID uuid.UUID
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, hash).Scan(&wsID)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, nil
		}
		return uuid.Nil, fmt.Errorf("failed resolving scim workspace by token: %w", err)
	}
	return wsID, nil
}

// SetSCIMToken stores the hash of a workspace's SCIM provisioning token.
// Passing an empty hash clears it, which disables SCIM for that workspace.
func (r *Repository) SetSCIMToken(ctx context.Context, wsID uuid.UUID, tokenHash, prefix string) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}
	if wsID == uuid.Nil {
		return fmt.Errorf("workspace is required")
	}
	hash := strings.ToLower(strings.TrimSpace(tokenHash))

	const query = `
		UPDATE workspaces
		SET scim_token_hash = NULLIF($2, ''),
		    scim_token_prefix = NULLIF($3, ''),
		    scim_token_issued_at = CASE WHEN $2 = '' THEN NULL ELSE NOW() END,
		    updated_at = NOW()
		WHERE id = $1;`

	return r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, query, wsID, hash, strings.TrimSpace(prefix))
		if err != nil {
			return fmt.Errorf("failed storing scim token: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("workspace %s not found", wsID)
		}
		return nil
	})
}

// HasSCIMToken reports whether SCIM provisioning is enabled for a workspace.
func (r *Repository) HasSCIMToken(ctx context.Context, wsID uuid.UUID) (bool, error) {
	if r == nil || r.client == nil {
		return false, fmt.Errorf("database unavailable")
	}
	if wsID == uuid.Nil {
		return false, nil
	}
	const query = `SELECT scim_token_hash IS NOT NULL FROM workspaces WHERE id = $1;`
	var enabled bool
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID).Scan(&enabled)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("failed reading scim token state: %w", err)
	}
	return enabled, nil
}

// GetSCIMUserByID reads one directory record by its SCIM id (the users.uuid
// primary key), scoped to the workspace.
func (r *Repository) GetSCIMUserByID(ctx context.Context, wsID uuid.UUID, id string) (*SCIMUserRecord, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	if wsID == uuid.Nil {
		return nil, fmt.Errorf("workspace scope is required")
	}

	userUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, nil // not a uuid: report as absent rather than an error
	}

	const query = `
		SELECT uuid, email, role, status, "createdAt", "updatedAt"
		FROM users
		WHERE organization_id = $1 AND uuid = $2;
	`

	var (
		idRaw     uuid.UUID
		email     string
		role      string
		status    string
		createdAt time.Time
		updatedAt time.Time
	)
	err = r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID, userUUID).Scan(
			&idRaw, &email, &role, &status, &createdAt, &updatedAt)
	})
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed reading scim user: %w", err)
	}

	rec := scimRecordFromRow(idRaw, email, role, status, createdAt, updatedAt)
	return &rec, nil
}

// SetSCIMUserActive flips a user's active state. SCIM deprovisioning arrives as
// active=false and must take effect in the database, not only in whatever cache
// the request happened to touch: a suspended account that still authenticates is
// exactly the offboarding failure SCIM exists to prevent.
func (r *Repository) SetSCIMUserActive(ctx context.Context, wsID uuid.UUID, id string, active bool) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}
	userUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid user id")
	}

	// users.status is the enum users_status_enum, whose values are
	// active/inactive/pending/awaiting_approval/removed/pending_email. There is no
	// "suspended" value: "inactive" is the deprovisioned state, and it is the one
	// CountSeats excludes, so deprovisioning frees the seat.
	status := "active"
	if !active {
		status = "inactive"
	}

	const query = `
		UPDATE users
		SET status = $3::users_status_enum, "updatedAt" = now()
		WHERE organization_id = $1 AND uuid = $2;
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, query, wsID, userUUID, status)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("no such user in this workspace")
		}
		return nil
	})
}

// ═════════════════════════════════════════════════════════════════════════════
// SCIM Groups
//
// Groups are stored relationally (migration 035) rather than in process memory.
// Members live in their own table so a membership change is a single DELETE and
// membership can be joined against users, which an embedded JSON blob cannot do.
// ═════════════════════════════════════════════════════════════════════════════

// SCIMGroupRecord is the storage shape of a SCIM group.
type SCIMGroupRecord struct {
	ID          string
	DisplayName string
	Members     []SCIMGroupMemberRecord
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// SCIMGroupMemberRecord is one membership row.
type SCIMGroupMemberRecord struct {
	Ref     string
	Display string
}

// ListSCIMGroups returns the workspace's groups with their members attached.
func (r *Repository) ListSCIMGroups(ctx context.Context, wsID uuid.UUID) ([]SCIMGroupRecord, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	if wsID == uuid.Nil {
		return nil, fmt.Errorf("workspace scope is required")
	}

	var groups []SCIMGroupRecord
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, scim_id, display_name, created_at, updated_at
			FROM scim_groups
			WHERE workspace_id = $1
			ORDER BY display_name ASC, scim_id ASC`, wsID)
		if err != nil {
			return fmt.Errorf("failed listing scim groups: %w", err)
		}
		defer rows.Close()

		type memberKey struct{ groupUUID, ref string }
		bySCIMID := map[string]int{}

		for rows.Next() {
			var g SCIMGroupRecord
			var groupUUID string
			if err := rows.Scan(&groupUUID, &g.ID, &g.DisplayName, &g.CreatedAt, &g.UpdatedAt); err != nil {
				return fmt.Errorf("failed scanning scim group: %w", err)
			}
			bySCIMID[g.ID] = len(groups)
			groups = append(groups, g)
			_ = groupUUID
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(groups) == 0 {
			return nil
		}

		// One membership query for the whole page rather than N+1 per group.
		mrows, err := tx.Query(ctx, `
			SELECT g.scim_id, m.member_ref, m.display_ref
			FROM scim_group_members m
			JOIN scim_groups g ON g.id = m.group_id
			WHERE m.workspace_id = $1
			ORDER BY m.member_ref ASC`, wsID)
		if err != nil {
			return fmt.Errorf("failed listing scim group members: %w", err)
		}
		defer mrows.Close()
		for mrows.Next() {
			var groupRef, ref, display string
			if err := mrows.Scan(&groupRef, &ref, &display); err != nil {
				return fmt.Errorf("failed scanning scim group member: %w", err)
			}
			if idx, ok := bySCIMID[groupRef]; ok {
				groups[idx].Members = append(groups[idx].Members, SCIMGroupMemberRecord{Ref: ref, Display: display})
			}
		}
		return mrows.Err()
	})
	if err != nil {
		return nil, err
	}
	return groups, nil
}

// GetSCIMGroup returns one group by its SCIM id.
func (r *Repository) GetSCIMGroup(ctx context.Context, wsID uuid.UUID, scimID string) (*SCIMGroupRecord, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	if wsID == uuid.Nil {
		return nil, fmt.Errorf("workspace scope is required")
	}

	groups, err := r.ListSCIMGroups(ctx, wsID)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].ID == scimID {
			return &groups[i], nil
		}
	}
	return nil, nil
}

// UpsertSCIMGroup creates or replaces a group and its full membership set.
// The whole membership is rewritten in one transaction so a partially applied
// IdP push cannot leave half a group behind.
func (r *Repository) UpsertSCIMGroup(ctx context.Context, wsID uuid.UUID, g SCIMGroupRecord) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}
	if wsID == uuid.Nil {
		return fmt.Errorf("workspace scope is required")
	}
	if g.ID == "" {
		return fmt.Errorf("group id is required")
	}

	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		var groupUUID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO scim_groups (workspace_id, scim_id, display_name)
			VALUES ($1, $2, $3)
			ON CONFLICT (workspace_id, scim_id) DO UPDATE
			  SET display_name = EXCLUDED.display_name, updated_at = now()
			RETURNING id`, wsID, g.ID, g.DisplayName).Scan(&groupUUID); err != nil {
			return fmt.Errorf("failed upserting scim group: %w", err)
		}

		if _, err := tx.Exec(ctx,
			`DELETE FROM scim_group_members WHERE group_id = $1`, groupUUID); err != nil {
			return fmt.Errorf("failed clearing scim group members: %w", err)
		}
		for _, m := range g.Members {
			if m.Ref == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO scim_group_members (workspace_id, group_id, member_ref, display_ref)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (group_id, member_ref) DO UPDATE
				  SET display_ref = EXCLUDED.display_ref`,
				wsID, groupUUID, m.Ref, m.Display); err != nil {
				return fmt.Errorf("failed inserting scim group member: %w", err)
			}
		}
		return nil
	})
}

// DeleteSCIMGroup removes a group. It reports whether a row was deleted so the
// caller can answer 404 rather than pretending a delete succeeded.
func (r *Repository) DeleteSCIMGroup(ctx context.Context, wsID uuid.UUID, scimID string) (bool, error) {
	if r == nil || r.client == nil {
		return false, fmt.Errorf("database unavailable")
	}
	if wsID == uuid.Nil {
		return false, fmt.Errorf("workspace scope is required")
	}

	deleted := false
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`DELETE FROM scim_groups WHERE workspace_id = $1 AND scim_id = $2`, wsID, scimID)
		if err != nil {
			return fmt.Errorf("failed deleting scim group: %w", err)
		}
		deleted = tag.RowsAffected() > 0
		return nil
	})
	if err != nil {
		return false, err
	}
	return deleted, nil
}

// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/mcp/manager/models"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// MCPRepository manages tenant-isolated persistence for MCP connections and integrations.
type MCPRepository struct {
	pool *pgxpool.Pool
}

// NewMCPRepository initializes a new data access repository.
func NewMCPRepository(pool *pgxpool.Pool) *MCPRepository {
	return &MCPRepository{pool: pool}
}

// ═══════════════════════════════════════════════════════════════
// CONNECTIONS REPOSITORY METHODS
// ═══════════════════════════════════════════════════════════════

// GetConnections queries paginated connections for a specific organization with deterministic ordering.
func (r *MCPRepository) GetConnections(
	ctx context.Context,
	orgID string,
	page, pageSize int,
	appName, provider, integrationID, status string,
) ([]models.MCPConnectionEntity, int64, error) {
	if r.pool == nil {
		return []models.MCPConnectionEntity{}, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	baseFilter := `WHERE "organizationId" = $1 AND "deletedAt" IS NULL`
	args := []any{orgID}
	argIdx := 2

	if appName != "" {
		baseFilter += fmt.Sprintf(` AND "appName" = $%d`, argIdx)
		args = append(args, appName)
		argIdx++
	}
	if provider != "" {
		baseFilter += fmt.Sprintf(` AND "provider" = $%d`, argIdx)
		args = append(args, provider)
		argIdx++
	}
	if integrationID != "" {
		baseFilter += fmt.Sprintf(` AND "integrationId" = $%d`, argIdx)
		args = append(args, integrationID)
		argIdx++
	}
	if status != "" {
		baseFilter += fmt.Sprintf(` AND "status" = $%d`, argIdx)
		args = append(args, status)
		argIdx++
	}

	// 1. Count total
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM "mcp-manager"."mcp_connections" %s`, baseFilter)
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed counting mcp connections: %w", err)
	}

	// 2. Query items (deterministic order by createdAt ASC)
	selectQuery := fmt.Sprintf(`
		SELECT "id", "organizationId", "integrationId", "provider", "status", "appName",
		       "mcpUrl", "allowedTools", "metadata", "createdAt", "updatedAt", "deletedAt"
		FROM "mcp-manager"."mcp_connections"
		%s
		ORDER BY "createdAt" ASC
		LIMIT $%d OFFSET $%d`, baseFilter, argIdx, argIdx+1)

	queryArgs := append(args, pageSize, offset)
	rows, err := r.pool.Query(ctx, selectQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed querying mcp connections: %w", err)
	}
	defer rows.Close()

	items := make([]models.MCPConnectionEntity, 0)
	for rows.Next() {
		var item models.MCPConnectionEntity
		var allowedToolsRaw []byte
		var metadataRaw []byte

		if err := rows.Scan(
			&item.ID, &item.OrganizationID, &item.IntegrationID, &item.Provider,
			&item.Status, &item.AppName, &item.MCPURL, &allowedToolsRaw, &metadataRaw,
			&item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed scanning connection row: %w", err)
		}

		if len(allowedToolsRaw) > 0 {
			_ = json.Unmarshal(allowedToolsRaw, &item.AllowedTools)
		}
		if item.AllowedTools == nil {
			item.AllowedTools = []string{}
		}
		if len(metadataRaw) > 0 {
			_ = json.Unmarshal(metadataRaw, &item.Metadata)
		}

		items = append(items, item)
	}

	return items, total, nil
}

// GetConnectionByID returns a connection by primary key UUID scoped strictly to the organization.
func (r *MCPRepository) GetConnectionByID(ctx context.Context, orgID, connectionID string) (*models.MCPConnectionEntity, error) {
	if r.pool == nil {
		return nil, nil
	}
	query := `
		SELECT "id", "organizationId", "integrationId", "provider", "status", "appName",
		       "mcpUrl", "allowedTools", "metadata", "createdAt", "updatedAt", "deletedAt"
		FROM "mcp-manager"."mcp_connections"
		WHERE "id" = $1 AND "organizationId" = $2 AND "deletedAt" IS NULL`

	row := r.pool.QueryRow(ctx, query, connectionID, orgID)
	return r.scanConnection(row)
}

// GetConnectionByIntegrationID returns a connection by integration ID scoped to the organization.
func (r *MCPRepository) GetConnectionByIntegrationID(ctx context.Context, orgID, integrationID string) (*models.MCPConnectionEntity, error) {
	if r.pool == nil {
		return nil, nil
	}
	query := `
		SELECT "id", "organizationId", "integrationId", "provider", "status", "appName",
		       "mcpUrl", "allowedTools", "metadata", "createdAt", "updatedAt", "deletedAt"
		FROM "mcp-manager"."mcp_connections"
		WHERE "integrationId" = $1 AND "organizationId" = $2 AND "deletedAt" IS NULL
		ORDER BY "createdAt" ASC
		LIMIT 1`

	row := r.pool.QueryRow(ctx, query, integrationID, orgID)
	return r.scanConnection(row)
}

// GetConnectionByIDOrIntegrationID finds a connection by either UUID or integrationId safely.
func (r *MCPRepository) GetConnectionByIDOrIntegrationID(ctx context.Context, orgID, ref string) (*models.MCPConnectionEntity, error) {
	if uuidRegex.MatchString(ref) {
		conn, err := r.GetConnectionByID(ctx, orgID, ref)
		if err == nil && conn != nil {
			return conn, nil
		}
	}
	return r.GetConnectionByIntegrationID(ctx, orgID, ref)
}

// ListAllConnectionsForOrg returns all active connections for an organization.
func (r *MCPRepository) ListAllConnectionsForOrg(ctx context.Context, orgID string) ([]models.MCPConnectionEntity, error) {
	if r.pool == nil {
		return []models.MCPConnectionEntity{}, nil
	}
	query := `
		SELECT "id", "organizationId", "integrationId", "provider", "status", "appName",
		       "mcpUrl", "allowedTools", "metadata", "createdAt", "updatedAt", "deletedAt"
		FROM "mcp-manager"."mcp_connections"
		WHERE "organizationId" = $1 AND "deletedAt" IS NULL
		ORDER BY "createdAt" ASC`

	rows, err := r.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed listing organization connections: %w", err)
	}
	defer rows.Close()

	items := make([]models.MCPConnectionEntity, 0)
	for rows.Next() {
		var item models.MCPConnectionEntity
		var allowedToolsRaw, metadataRaw []byte

		if err := rows.Scan(
			&item.ID, &item.OrganizationID, &item.IntegrationID, &item.Provider,
			&item.Status, &item.AppName, &item.MCPURL, &allowedToolsRaw, &metadataRaw,
			&item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("failed scanning connection row: %w", err)
		}

		if len(allowedToolsRaw) > 0 {
			_ = json.Unmarshal(allowedToolsRaw, &item.AllowedTools)
		}
		if item.AllowedTools == nil {
			item.AllowedTools = []string{}
		}
		if len(metadataRaw) > 0 {
			_ = json.Unmarshal(metadataRaw, &item.Metadata)
		}

		items = append(items, item)
	}

	return items, nil
}

// SaveConnection inserts or updates a connection entity.
func (r *MCPRepository) SaveConnection(ctx context.Context, conn *models.MCPConnectionEntity) (*models.MCPConnectionEntity, error) {
	if r.pool == nil {
		return conn, nil
	}
	allowedToolsJSON, _ := json.Marshal(conn.AllowedTools)
	if conn.AllowedTools == nil {
		allowedToolsJSON = []byte("[]")
	}

	var metadataJSON []byte
	if conn.Metadata != nil {
		metadataJSON, _ = json.Marshal(conn.Metadata)
	}

	now := time.Now().UTC()

	if conn.ID == "" {
		insertQuery := `
			INSERT INTO "mcp-manager"."mcp_connections" (
				"organizationId", "integrationId", "provider", "status", "appName",
				"mcpUrl", "allowedTools", "metadata", "createdAt", "updatedAt"
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
			RETURNING "id", "createdAt", "updatedAt"`

		err := r.pool.QueryRow(ctx, insertQuery,
			conn.OrganizationID, conn.IntegrationID, conn.Provider, string(conn.Status),
			conn.AppName, conn.MCPURL, allowedToolsJSON, metadataJSON, now,
		).Scan(&conn.ID, &conn.CreatedAt, &conn.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed inserting connection: %w", err)
		}
		return conn, nil
	}

	updateQuery := `
		UPDATE "mcp-manager"."mcp_connections"
		SET "status" = $1, "appName" = $2, "mcpUrl" = $3, "allowedTools" = $4,
		    "metadata" = $5, "updatedAt" = $6
		WHERE "id" = $7 AND "organizationId" = $8 AND "deletedAt" IS NULL
		RETURNING "updatedAt"`

	err := r.pool.QueryRow(ctx, updateQuery,
		string(conn.Status), conn.AppName, conn.MCPURL, allowedToolsJSON,
		metadataJSON, now, conn.ID, conn.OrganizationID,
	).Scan(&conn.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed updating connection: %w", err)
	}

	return conn, nil
}

// DeleteConnection hard deletes a connection row scoped to the organization.
func (r *MCPRepository) DeleteConnection(ctx context.Context, orgID, connectionID string) error {
	if r.pool == nil {
		return nil
	}
	query := `DELETE FROM "mcp-manager"."mcp_connections" WHERE "id" = $1 AND "organizationId" = $2`
	_, err := r.pool.Exec(ctx, query, connectionID, orgID)
	return err
}

// UpdateAllowedTools updates the permitted tool slugs for an integration in an organization.
func (r *MCPRepository) UpdateAllowedTools(ctx context.Context, orgID, integrationID string, allowedTools []string) (*models.MCPConnectionEntity, error) {
	conn, err := r.GetConnectionByIntegrationID(ctx, orgID, integrationID)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, errors.New("connection not found")
	}

	conn.AllowedTools = allowedTools
	return r.SaveConnection(ctx, conn)
}

func (r *MCPRepository) scanConnection(row pgx.Row) (*models.MCPConnectionEntity, error) {
	var item models.MCPConnectionEntity
	var allowedToolsRaw []byte
	var metadataRaw []byte

	err := row.Scan(
		&item.ID, &item.OrganizationID, &item.IntegrationID, &item.Provider,
		&item.Status, &item.AppName, &item.MCPURL, &allowedToolsRaw, &metadataRaw,
		&item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed scanning connection: %w", err)
	}

	if len(allowedToolsRaw) > 0 {
		_ = json.Unmarshal(allowedToolsRaw, &item.AllowedTools)
	}
	if item.AllowedTools == nil {
		item.AllowedTools = []string{}
	}
	if len(metadataRaw) > 0 {
		_ = json.Unmarshal(metadataRaw, &item.Metadata)
	}

	return &item, nil
}

// ═══════════════════════════════════════════════════════════════
// CUSTOM INTEGRATIONS REPOSITORY METHODS
// ═══════════════════════════════════════════════════════════════

// GetCustomIntegrations retrieves custom integrations for an organization.
func (r *MCPRepository) GetCustomIntegrations(ctx context.Context, orgID string, activeOnly bool) ([]models.MCPIntegrationEntity, error) {
	if r.pool == nil {
		return []models.MCPIntegrationEntity{}, nil
	}
	query := `
		SELECT "id", "active", "organizationId", "protocol", "baseUrl", "name",
		       "description", "logoUrl", "authType", "auth", "headers", "createdAt", "updatedAt", "deletedAt"
		FROM "mcp-manager"."mcp_integrations"
		WHERE "organizationId" = $1 AND ($2 = false OR "active" = true) AND "deletedAt" IS NULL
		ORDER BY "createdAt" ASC`

	rows, err := r.pool.Query(ctx, query, orgID, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("failed querying custom integrations: %w", err)
	}
	defer rows.Close()

	items := make([]models.MCPIntegrationEntity, 0)
	for rows.Next() {
		var item models.MCPIntegrationEntity
		if err := rows.Scan(
			&item.ID, &item.Active, &item.OrganizationID, &item.Protocol, &item.BaseURL,
			&item.Name, &item.Description, &item.LogoURL, &item.AuthType, &item.Auth,
			&item.Headers, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("failed scanning custom integration: %w", err)
		}
		items = append(items, item)
	}

	return items, nil
}

// GetCustomIntegrationByID retrieves a custom integration by ID and org.
func (r *MCPRepository) GetCustomIntegrationByID(ctx context.Context, orgID, integrationID string, activeOnly bool) (*models.MCPIntegrationEntity, error) {
	if r.pool == nil {
		return nil, nil
	}
	query := `
		SELECT "id", "active", "organizationId", "protocol", "baseUrl", "name",
		       "description", "logoUrl", "authType", "auth", "headers", "createdAt", "updatedAt", "deletedAt"
		FROM "mcp-manager"."mcp_integrations"
		WHERE "id" = $1 AND "organizationId" = $2 AND ($3 = false OR "active" = true) AND "deletedAt" IS NULL`

	var item models.MCPIntegrationEntity
	err := r.pool.QueryRow(ctx, query, integrationID, orgID, activeOnly).Scan(
		&item.ID, &item.Active, &item.OrganizationID, &item.Protocol, &item.BaseURL,
		&item.Name, &item.Description, &item.LogoURL, &item.AuthType, &item.Auth,
		&item.Headers, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed querying custom integration: %w", err)
	}

	return &item, nil
}

// SaveCustomIntegration inserts or updates a custom integration entity.
func (r *MCPRepository) SaveCustomIntegration(ctx context.Context, entity *models.MCPIntegrationEntity) (*models.MCPIntegrationEntity, error) {
	if r.pool == nil {
		return entity, nil
	}
	now := time.Now().UTC()

	if entity.ID == "" {
		query := `
			INSERT INTO "mcp-manager"."mcp_integrations" (
				"active", "organizationId", "protocol", "baseUrl", "name",
				"description", "logoUrl", "authType", "auth", "headers", "createdAt", "updatedAt"
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)
			RETURNING "id", "createdAt", "updatedAt"`

		err := r.pool.QueryRow(ctx, query,
			entity.Active, entity.OrganizationID, string(entity.Protocol), entity.BaseURL,
			entity.Name, entity.Description, entity.LogoURL, string(entity.AuthType),
			entity.Auth, entity.Headers, now,
		).Scan(&entity.ID, &entity.CreatedAt, &entity.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed inserting custom integration: %w", err)
		}
		return entity, nil
	}

	query := `
		UPDATE "mcp-manager"."mcp_integrations"
		SET "active" = $1, "protocol" = $2, "baseUrl" = $3, "name" = $4,
		    "description" = $5, "logoUrl" = $6, "authType" = $7, "auth" = $8,
		    "headers" = $9, "updatedAt" = $10
		WHERE "id" = $11 AND "organizationId" = $12 AND "deletedAt" IS NULL
		RETURNING "updatedAt"`

	err := r.pool.QueryRow(ctx, query,
		entity.Active, string(entity.Protocol), entity.BaseURL, entity.Name,
		entity.Description, entity.LogoURL, string(entity.AuthType), entity.Auth,
		entity.Headers, now, entity.ID, entity.OrganizationID,
	).Scan(&entity.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed updating custom integration: %w", err)
	}

	return entity, nil
}

// DeleteCustomIntegration soft-deletes or hard-deletes a custom integration.
func (r *MCPRepository) DeleteCustomIntegration(ctx context.Context, orgID, integrationID string) error {
	if r.pool == nil {
		return nil
	}
	query := `DELETE FROM "mcp-manager"."mcp_integrations" WHERE "id" = $1 AND "organizationId" = $2`
	_, err := r.pool.Exec(ctx, query, integrationID, orgID)
	return err
}

// ═══════════════════════════════════════════════════════════════
// OAUTH & MANAGED CREDENTIALS REPOSITORY METHODS
// ═══════════════════════════════════════════════════════════════

// GetOAuthEntity retrieves OAuth/managed token state for (orgId, integrationId).
func (r *MCPRepository) GetOAuthEntity(ctx context.Context, orgID, integrationID string) (*models.MCPIntegrationOAuthEntity, error) {
	if r.pool == nil {
		return nil, nil
	}
	query := `
		SELECT "id", "status", "organizationId", "integrationId", "auth", "createdAt", "updatedAt"
		FROM "mcp-manager"."mcp_integration_oauth"
		WHERE "organizationId" = $1 AND "integrationId" = $2`

	var item models.MCPIntegrationOAuthEntity
	err := r.pool.QueryRow(ctx, query, orgID, integrationID).Scan(
		&item.ID, &item.Status, &item.OrganizationID, &item.IntegrationID,
		&item.Auth, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed querying oauth entity: %w", err)
	}

	return &item, nil
}

// SaveOAuthEntity inserts or updates OAuth/token state with upsert semantics.
func (r *MCPRepository) SaveOAuthEntity(ctx context.Context, entity *models.MCPIntegrationOAuthEntity) error {
	if r.pool == nil {
		return nil
	}
	now := time.Now().UTC()
	query := `
		INSERT INTO "mcp-manager"."mcp_integration_oauth" (
			"status", "organizationId", "integrationId", "auth", "createdAt", "updatedAt"
		) VALUES ($1, $2, $3, $4, $5, $5)
		ON CONFLICT ("organizationId", "integrationId")
		DO UPDATE SET "status" = EXCLUDED."status", "auth" = EXCLUDED."auth", "updatedAt" = EXCLUDED."updatedAt"
		RETURNING "id", "createdAt", "updatedAt"`

	return r.pool.QueryRow(ctx, query,
		string(entity.Status), entity.OrganizationID, entity.IntegrationID,
		entity.Auth, now,
	).Scan(&entity.ID, &entity.CreatedAt, &entity.UpdatedAt)
}

// DeleteOAuthEntity purges OAuth/token credentials for an integration.
func (r *MCPRepository) DeleteOAuthEntity(ctx context.Context, orgID, integrationID string) error {
	if r.pool == nil {
		return nil
	}
	query := `DELETE FROM "mcp-manager"."mcp_integration_oauth" WHERE "organizationId" = $1 AND "integrationId" = $2`
	_, err := r.pool.Exec(ctx, query, orgID, integrationID)
	return err
}

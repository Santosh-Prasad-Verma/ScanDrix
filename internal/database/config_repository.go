// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Database Access Layer
// File: config_repository.go
// ═══════════════════════════════════════════════════════════════

package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/pkg/crypto"
	"github.com/scandrix/backend/pkg/models"
)

// GetTrackedRepositoryByID returns one tracked repository scoped to its workspace.
//
// The workspace id is part of the predicate, not merely the row id, so a caller
// cannot read a repository belonging to another workspace by guessing its id.
func (r *Repository) GetTrackedRepositoryByID(ctx context.Context, wsID, repositoryID uuid.UUID) (*models.TrackedRepository, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	query := `
		SELECT id, workspace_id, provider, external_id, namespace_path, default_branch, is_active, created_at, updated_at
		FROM tracked_repositories
		WHERE workspace_id = $1 AND id = $2
		LIMIT 1;
	`
	var tr models.TrackedRepository
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID, repositoryID).Scan(
			&tr.ID, &tr.WorkspaceID, &tr.Provider, &tr.ExternalID, &tr.NamespacePath,
			&tr.DefaultBranch, &tr.IsActive, &tr.CreatedAt, &tr.UpdatedAt,
		)
	})
	if err != nil {
		return nil, err
	}
	return &tr, nil
}

// ListTrackedRepositories lists all monitored repositories for a workspace.
// GetTrackedRepositoryByNamespace resolves a provider + namespace to its
// tracked repository row.
//
// The review pipeline needs the repository id, not just the workspace: the
// pull_request_reviews.repository_id foreign key points at tracked_repositories,
// so a review inserted with a nil repository id violates the constraint and no
// review row is ever persisted.
func (r *Repository) GetTrackedRepositoryByNamespace(ctx context.Context, provider models.SCMProvider, namespacePath string) (*models.TrackedRepository, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	query := `
		SELECT id, workspace_id, provider, external_id, namespace_path, default_branch, is_active, created_at, updated_at
		FROM tracked_repositories
		WHERE provider = $1 AND namespace_path = $2 AND is_active = TRUE
		LIMIT 1;
	`
	var tr models.TrackedRepository
	// Reverse lookup with no tenant in hand: this is what supplies the tenant.
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, string(provider), namespacePath).Scan(
			&tr.ID, &tr.WorkspaceID, &tr.Provider, &tr.ExternalID, &tr.NamespacePath,
			&tr.DefaultBranch, &tr.IsActive, &tr.CreatedAt, &tr.UpdatedAt,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("no tracked repository for %s/%s: %w", provider, namespacePath, err)
	}
	return &tr, nil
}

func (r *Repository) ListTrackedRepositories(ctx context.Context, wsID uuid.UUID) ([]models.TrackedRepository, error) {
	if r == nil || r.client == nil {
		return []models.TrackedRepository{}, nil
	}

	// is_active is filtered so this agrees with
	// PostgresTrackedRepositoryReader.ListTrackedRepositories. Untracking a
	// repository (UntrackRepository) clears is_active, and without this filter a
	// deleted repository kept appearing in repo listings, cockpit dashboards and
	// language detection.
	query := `
		SELECT id, workspace_id, provider, external_id, namespace_path, default_branch, is_active, created_at, updated_at
		FROM tracked_repositories
		WHERE workspace_id = $1 AND is_active = true
		ORDER BY namespace_path ASC;
	`

	var repos []models.TrackedRepository
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var repo models.TrackedRepository
			if err := rows.Scan(
				&repo.ID, &repo.WorkspaceID, &repo.Provider, &repo.ExternalID,
				&repo.NamespacePath, &repo.DefaultBranch, &repo.IsActive,
				&repo.CreatedAt, &repo.UpdatedAt,
			); err != nil {
				return err
			}
			repos = append(repos, repo)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed listing tracked repositories: %w", err)
	}
	return repos, nil
}

// TrackRepository registers or updates a monitored repository.
func (r *Repository) TrackRepository(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider, externalID, namespacePath, defaultBranch string) (*models.TrackedRepository, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	if defaultBranch == "" {
		defaultBranch = "main"
	}
	repo := &models.TrackedRepository{
		ID:            uuid.New(),
		WorkspaceID:   wsID,
		Provider:      provider,
		ExternalID:    externalID,
		NamespacePath: namespacePath,
		DefaultBranch: defaultBranch,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	query := `
		INSERT INTO tracked_repositories (id, workspace_id, provider, external_id, namespace_path, default_branch, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (workspace_id, provider, external_id) DO UPDATE
		SET namespace_path = EXCLUDED.namespace_path, default_branch = EXCLUDED.default_branch, is_active = EXCLUDED.is_active, updated_at = EXCLUDED.updated_at
		RETURNING id, workspace_id, provider, external_id, namespace_path, default_branch, is_active, created_at, updated_at;
	`

	var res models.TrackedRepository
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			repo.ID, repo.WorkspaceID, string(repo.Provider), repo.ExternalID,
			repo.NamespacePath, repo.DefaultBranch, repo.IsActive, repo.CreatedAt, repo.UpdatedAt,
		).Scan(&res.ID, &res.WorkspaceID, &res.Provider, &res.ExternalID, &res.NamespacePath, &res.DefaultBranch, &res.IsActive, &res.CreatedAt, &res.UpdatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("failed tracking repository: %w", err)
	}
	return &res, nil
}

// GetWorkspaceParameters loads the review and organization parameters JSON payloads.
func (r *Repository) GetWorkspaceParameters(ctx context.Context, wsID uuid.UUID) (reviewParams, orgParams []byte, err error) {
	if r == nil || r.client == nil {
		return []byte("{}"), []byte("{}"), nil
	}

	query := `SELECT review_params, org_params FROM workspace_parameters WHERE workspace_id = $1;`
	err = r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, query, wsID)
		return row.Scan(&reviewParams, &orgParams)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []byte("{}"), []byte("{}"), nil
		}
		return nil, nil, err
	}
	return reviewParams, orgParams, nil
}

// UpdateWorkspaceReviewParameters stores updated review settings.
func (r *Repository) UpdateWorkspaceReviewParameters(ctx context.Context, wsID uuid.UUID, reviewParams []byte) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	query := `
		INSERT INTO workspace_parameters (workspace_id, review_params, org_params, updated_at)
		VALUES ($1, $2, '{}'::jsonb, now())
		ON CONFLICT (workspace_id) DO UPDATE
		SET review_params = EXCLUDED.review_params, updated_at = now();
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, wsID, reviewParams)
		return err
	})
}

// UpdateWorkspaceOrgParameters stores updated organizational governance thresholds.
func (r *Repository) UpdateWorkspaceOrgParameters(ctx context.Context, wsID uuid.UUID, orgParams []byte) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	query := `
		INSERT INTO workspace_parameters (workspace_id, review_params, org_params, updated_at)
		VALUES ($1, '{}'::jsonb, $2, now())
		ON CONFLICT (workspace_id) DO UPDATE
		SET org_params = EXCLUDED.org_params, updated_at = now();
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, wsID, orgParams)
		return err
	})
}

// ListNotificationChannels retrieves configured alert destinations.
func (r *Repository) ListNotificationChannels(ctx context.Context, wsID uuid.UUID) ([]models.NotificationChannel, error) {
	if r == nil || r.client == nil {
		return []models.NotificationChannel{}, nil
	}

	query := `
		SELECT id, workspace_id, type, target, severity, enabled, created_at
		FROM notification_channels
		WHERE workspace_id = $1
		ORDER BY created_at ASC;
	`
	var channels []models.NotificationChannel
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var ch models.NotificationChannel
			if err := rows.Scan(&ch.ID, &ch.WorkspaceID, &ch.Type, &ch.Target, &ch.Severity, &ch.Enabled, &ch.CreatedAt); err != nil {
				return err
			}
			channels = append(channels, ch)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed listing channels: %w", err)
	}
	return channels, nil
}

// CreateNotificationChannel creates a new alert destination.
func (r *Repository) CreateNotificationChannel(ctx context.Context, wsID uuid.UUID, chType, target string, severity models.FindingSeverity) (*models.NotificationChannel, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	ch := &models.NotificationChannel{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Type:        chType,
		Target:      target,
		Severity:    severity,
		Enabled:     true,
		CreatedAt:   time.Now().UTC(),
	}

	query := `
		INSERT INTO notification_channels (id, workspace_id, type, target, severity, enabled, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, workspace_id, type, target, severity, enabled, created_at;
	`

	var res models.NotificationChannel
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, ch.ID, ch.WorkspaceID, ch.Type, ch.Target, string(ch.Severity), ch.Enabled, ch.CreatedAt).
			Scan(&res.ID, &res.WorkspaceID, &res.Type, &res.Target, &res.Severity, &res.Enabled, &res.CreatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("failed creating channel: %w", err)
	}
	return &res, nil
}

// ListIntegrationConnections retrieves external SCM connections for the workspace.
func (r *Repository) ListIntegrationConnections(ctx context.Context, wsID uuid.UUID) ([]models.IntegrationConnection, error) {
	if r == nil || r.client == nil {
		return []models.IntegrationConnection{}, nil
	}

	query := `
		SELECT id, workspace_id, provider, account_name, is_connected, access_token_enc, repo_count, last_synced_at, created_at, updated_at
		FROM integration_connections
		WHERE workspace_id = $1
		ORDER BY provider ASC;
	`

	var conns []models.IntegrationConnection
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c models.IntegrationConnection
			if err := rows.Scan(
				&c.ID, &c.WorkspaceID, &c.Provider, &c.AccountName, &c.IsConnected,
				&c.AccessTokenEnc, &c.RepoCount, &c.LastSyncedAt, &c.CreatedAt, &c.UpdatedAt,
			); err != nil {
				return err
			}
			if c.AccessTokenEnc != "" {
				c.AccessTokenEnc = decryptStoredSecret(ctx, wsID, c.AccessTokenEnc)
			}
			conns = append(conns, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed listing integrations: %w", err)
	}
	return conns, nil
}

// GetIntegrationConnection returns an active SCM connection by provider with decrypted token.
func (r *Repository) GetIntegrationConnection(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider) (*models.IntegrationConnection, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	query := `
		SELECT id, workspace_id, provider, account_name, is_connected, access_token_enc, repo_count, last_synced_at, created_at, updated_at
		FROM integration_connections
		WHERE workspace_id = $1 AND provider = $2;
	`

	var c models.IntegrationConnection
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, query, wsID, string(provider))
		return row.Scan(
			&c.ID, &c.WorkspaceID, &c.Provider, &c.AccountName, &c.IsConnected,
			&c.AccessTokenEnc, &c.RepoCount, &c.LastSyncedAt, &c.CreatedAt, &c.UpdatedAt,
		)
	})
	if err != nil {
		return nil, err
	}
	if c.AccessTokenEnc != "" {
		c.AccessTokenEnc = decryptStoredSecret(ctx, wsID, c.AccessTokenEnc)
	}
	return &c, nil
}

// GetIntegrationConnectionByID returns one SCM connection by its identifier with
// a decrypted token.
//
// Lookup by id (rather than by provider) is what destructive operations need: the
// caller names the exact connection it wants removed, and the row's own provider
// column decides what gets deleted. Resolving the provider from the stored row
// rather than from a client-supplied string means a request cannot name a
// provider it does not own.
func (r *Repository) GetIntegrationConnectionByID(ctx context.Context, wsID, connectionID uuid.UUID) (*models.IntegrationConnection, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	query := `
		SELECT id, workspace_id, provider, account_name, is_connected, access_token_enc, repo_count, last_synced_at, created_at, updated_at
		FROM integration_connections
		WHERE workspace_id = $1 AND id = $2;
	`

	var c models.IntegrationConnection
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, query, wsID, connectionID)
		return row.Scan(
			&c.ID, &c.WorkspaceID, &c.Provider, &c.AccountName, &c.IsConnected,
			&c.AccessTokenEnc, &c.RepoCount, &c.LastSyncedAt, &c.CreatedAt, &c.UpdatedAt,
		)
	})
	if err != nil {
		return nil, err
	}
	if c.AccessTokenEnc != "" {
		c.AccessTokenEnc = decryptStoredSecret(ctx, wsID, c.AccessTokenEnc)
	}
	return &c, nil
}

// DeleteIntegrationConnection removes an SCM connection for a workspace.
func (r *Repository) DeleteIntegrationConnection(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	query := `DELETE FROM integration_connections WHERE workspace_id = $1 AND provider = $2;`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, wsID, string(provider))
		return err
	})
}

// UntrackAllRepositories removes tracked repositories for a provider in a workspace.
func (r *Repository) UntrackAllRepositories(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	query := `DELETE FROM tracked_repositories WHERE workspace_id = $1 AND provider = $2;`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, wsID, string(provider))
		return err
	})
}

// UpdateIntegrationRepoCount updates the tracked repository count on the active integration.
func (r *Repository) UpdateIntegrationRepoCount(ctx context.Context, wsID uuid.UUID, count int) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	query := `UPDATE integration_connections SET repo_count = $1, last_synced_at = NOW(), updated_at = NOW() WHERE workspace_id = $2;`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, count, wsID)
		return err
	})
}

// UpsertIntegrationConnection stores or updates an SCM connection with encrypted credentials.
func (r *Repository) UpsertIntegrationConnection(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider, accountName, tokenPlain string, isConnected bool, repoCount int) error {
	return r.UpsertIntegrationConnectionWithSecret(ctx, wsID, provider, accountName, tokenPlain, "", isConnected, repoCount)
}

// UpsertIntegrationConnectionWithSecret stores or updates an SCM connection with encrypted credentials and optional webhook secret.
func (r *Repository) UpsertIntegrationConnectionWithSecret(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider, accountName, tokenPlain, secretPlain string, isConnected bool, repoCount int) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	tokenEnc := tokenPlain
	secretEnc := secretPlain
	tenantKey := deriveTenantIntegrationKey(wsID)

	// 2-tier KMS Envelope Encryption with ephemeral DEKs (NIST SP 800-57)
	if tokenPlain != "" {
		if kmsEnvelopeEngine != nil {
			if enc, err := kmsEnvelopeEngine.EncryptString(ctx, "workspace:"+wsID.String(), tokenPlain); err == nil {
				tokenEnc = enc
			}
		}
		if tokenEnc == tokenPlain {
			if enc, err := crypto.EncryptStringAESGCM(tenantKey, tokenPlain); err == nil {
				tokenEnc = enc
			}
		}
		if tokenEnc == tokenPlain {
			return fmt.Errorf("cryptographic failure: cannot persist integration connection with unencrypted access token")
		}
	}
	if secretPlain != "" {
		if kmsEnvelopeEngine != nil {
			if enc, err := kmsEnvelopeEngine.EncryptString(ctx, "workspace:"+wsID.String(), secretPlain); err == nil {
				secretEnc = enc
			}
		}
		if secretEnc == secretPlain {
			if enc, err := crypto.EncryptStringAESGCM(tenantKey, secretPlain); err == nil {
				secretEnc = enc
			}
		}
		if secretEnc == secretPlain {
			return fmt.Errorf("cryptographic failure: cannot persist integration connection with unencrypted webhook secret")
		}
	}

	query := `
		INSERT INTO integration_connections (id, workspace_id, provider, account_name, is_connected, access_token_enc, webhook_secret_enc, repo_count, last_synced_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), now(), now())
		ON CONFLICT (workspace_id, provider) DO UPDATE
		SET account_name = EXCLUDED.account_name,
		    is_connected = EXCLUDED.is_connected,
		    access_token_enc = EXCLUDED.access_token_enc,
		    webhook_secret_enc = CASE WHEN EXCLUDED.webhook_secret_enc <> '' THEN EXCLUDED.webhook_secret_enc ELSE integration_connections.webhook_secret_enc END,
		    repo_count = EXCLUDED.repo_count,
		    last_synced_at = now(),
		    updated_at = now();
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, uuid.New(), wsID, string(provider), accountName, isConnected, tokenEnc, secretEnc, repoCount)
		return err
	})
}

// GetDecryptedIntegrationToken retrieves and decrypts the stored integration token for a provider.
func (r *Repository) GetDecryptedIntegrationToken(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider) (string, error) {
	if r == nil || r.client == nil {
		return "", nil
	}
	conns, err := r.ListIntegrationConnections(ctx, wsID)
	if err != nil {
		return "", err
	}
	for _, c := range conns {
		if c.Provider == provider && c.IsConnected && c.AccessTokenEnc != "" {
			return decryptStoredSecret(ctx, wsID, c.AccessTokenEnc), nil
		}
	}
	return "", nil
}

// GetOrganizationParameter loads a single organization parameter by key for a workspace.
func (r *Repository) GetOrganizationParameter(ctx context.Context, wsID uuid.UUID, key string) (*models.OrganizationParameter, error) {
	if r == nil || r.client == nil {
		return nil, nil
	}

	query := `
		SELECT id, workspace_id, config_key, config_value, description, is_active, created_at, updated_at
		FROM organization_parameters
		WHERE workspace_id = $1 AND config_key = $2 AND is_active = true;
	`
	var param models.OrganizationParameter
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, query, wsID, key)
		return row.Scan(
			&param.ID, &param.WorkspaceID, &param.ConfigKey,
			&param.ConfigValue, &param.Description, &param.IsActive,
			&param.CreatedAt, &param.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed fetching organization parameter %s: %w", key, err)
	}
	return &param, nil
}

// SetOrganizationParameter creates or updates an organization parameter for a workspace.
func (r *Repository) SetOrganizationParameter(ctx context.Context, wsID uuid.UUID, key string, val []byte, desc string) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	if len(val) == 0 {
		val = []byte("{}")
	}

	query := `
		INSERT INTO organization_parameters (id, workspace_id, config_key, config_value, description, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, true, now(), now())
		ON CONFLICT (workspace_id, config_key) DO UPDATE
		SET config_value = EXCLUDED.config_value,
		    description = CASE WHEN EXCLUDED.description <> '' THEN EXCLUDED.description ELSE organization_parameters.description END,
		    is_active = true,
		    updated_at = now();
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, uuid.New(), wsID, key, val, desc)
		return err
	})
}

// DeleteOrganizationParameter soft or hard deletes an organization parameter for a workspace.
func (r *Repository) DeleteOrganizationParameter(ctx context.Context, wsID uuid.UUID, key string) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	query := `DELETE FROM organization_parameters WHERE workspace_id = $1 AND config_key = $2;`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, wsID, key)
		return err
	})
}

// ListOrganizationParameters retrieves all active organization parameters for a workspace.
func (r *Repository) ListOrganizationParameters(ctx context.Context, wsID uuid.UUID) ([]models.OrganizationParameter, error) {
	if r == nil || r.client == nil {
		return []models.OrganizationParameter{}, nil
	}

	query := `
		SELECT id, workspace_id, config_key, config_value, description, is_active, created_at, updated_at
		FROM organization_parameters
		WHERE workspace_id = $1 AND is_active = true
		ORDER BY config_key ASC;
	`
	var params []models.OrganizationParameter
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var p models.OrganizationParameter
			if err := rows.Scan(
				&p.ID, &p.WorkspaceID, &p.ConfigKey,
				&p.ConfigValue, &p.Description, &p.IsActive,
				&p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				return err
			}
			params = append(params, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed listing organization parameters: %w", err)
	}
	return params, nil
}

// GetGlobalParameter loads a system-wide parameter JSON payload by key.
func (r *Repository) GetGlobalParameter(ctx context.Context, key string) ([]byte, error) {
	if r == nil || r.client == nil {
		return nil, nil
	}

	query := `SELECT config_value FROM global_parameters WHERE config_key = $1;`
	var raw []byte
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, query, key)
		return row.Scan(&raw)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed fetching global parameter %s: %w", key, err)
	}
	return raw, nil
}

// SetGlobalParameter creates or updates a system-wide parameter.
func (r *Repository) SetGlobalParameter(ctx context.Context, key string, val []byte, desc string) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	if len(val) == 0 {
		val = []byte("{}")
	}

	query := `
		INSERT INTO global_parameters (id, config_key, config_value, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, now(), now())
		ON CONFLICT (config_key) DO UPDATE
		SET config_value = EXCLUDED.config_value,
		    description = CASE WHEN EXCLUDED.description <> '' THEN EXCLUDED.description ELSE global_parameters.description END,
		    updated_at = now();
	`
	return r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, uuid.New(), key, val, desc)
		return err
	})
}

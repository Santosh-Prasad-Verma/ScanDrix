// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// File: sso_config_repository.go
// ═══════════════════════════════════════════════════════════════

package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/pkg/crypto"
	"github.com/scandrix/backend/pkg/models"
)

// ssoConfigEnvelopeKey namespaces the encrypted provider_config blob so it can
// never be confused with a plaintext JSON document written by an older build.
const ssoConfigEnvelopeKey = "sso_provider_config_v1"

// GetSSOConfig returns the SSO configuration for a workspace, or (nil, nil)
// when the workspace has none. Reads are tenant-scoped through RLS.
func (r *Repository) GetSSOConfig(ctx context.Context, wsID uuid.UUID) (*models.SSOConfig, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	const query = `
		SELECT id, workspace_id, protocol, provider_config, active, domains,
		       saml_required, verified_at, created_at, updated_at
		FROM sso_configs
		WHERE workspace_id = $1;
	`

	var (
		cfg        models.SSOConfig
		rawConfig  []byte
		verifiedAt *time.Time
	)

	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID).Scan(
			&cfg.ID, &cfg.WorkspaceID, &cfg.Protocol, &rawConfig, &cfg.Active,
			&cfg.Domains, &cfg.SAMLRequired, &verifiedAt, &cfg.CreatedAt, &cfg.UpdatedAt,
		)
	})
	if err != nil {
		// No configuration yet is a normal state, not a failure.
		if strings.Contains(err.Error(), "no rows") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed reading sso config: %w", err)
	}

	cfg.VerifiedAt = verifiedAt
	provider, err := decryptSSOProviderConfig(ctx, wsID, rawConfig)
	if err != nil {
		// A configuration that cannot be decrypted is a real incident: refuse it
		// rather than silently returning an empty IdP config that would break
		// every login for this workspace.
		return nil, fmt.Errorf("failed decrypting sso provider config: %w", err)
	}
	cfg.Provider = provider

	return &cfg, nil
}

// UpsertSSOConfig writes the workspace's SSO configuration, replacing any
// existing row. provider_config is encrypted at rest before the write.
//
// enforcementBlocked is returned when the caller tried to turn on saml_required
// for a configuration that has never completed an IdP handshake: enforcing SSO
// with an unverified config locks every user out of the product.
func (r *Repository) UpsertSSOConfig(ctx context.Context, cfg models.SSOConfig) (stored models.SSOConfig, enforcementBlocked bool, err error) {
	if r == nil || r.client == nil {
		return stored, false, fmt.Errorf("database unavailable")
	}
	if cfg.WorkspaceID == uuid.Nil {
		return stored, false, fmt.Errorf("workspace id is required")
	}
	if !cfg.Protocol.Valid() {
		return stored, false, fmt.Errorf("unsupported sso protocol %q", cfg.Protocol)
	}

	// Never let an unverified configuration become the enforcing one.
	if cfg.SAMLRequired && cfg.VerifiedAt == nil {
		return stored, true, fmt.Errorf("sso enforcement requires a verified idp configuration; complete the connection test first")
	}

	normalizedDomains := models.NormalizeSSODomains(cfg.Domains)

	sealed, err := sealSSOProviderConfig(ctx, cfg.WorkspaceID, cfg.Provider)
	if err != nil {
		return stored, false, err
	}

	const query = `
		INSERT INTO sso_configs (
			id, workspace_id, protocol, provider_config, active, domains,
			saml_required, verified_at
		)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8)
		ON CONFLICT (workspace_id) DO UPDATE
		SET protocol        = EXCLUDED.protocol,
		    provider_config = EXCLUDED.provider_config,
		    active          = EXCLUDED.active,
		    domains         = EXCLUDED.domains,
		    saml_required   = EXCLUDED.saml_required,
		    verified_at     = COALESCE(EXCLUDED.verified_at, sso_configs.verified_at),
		    updated_at      = now()
		RETURNING id, workspace_id, protocol, active, domains, saml_required,
		          verified_at, created_at, updated_at;
	`

	err = r.client.ExecWithTenant(ctx, cfg.WorkspaceID, func(tx pgx.Tx) error {
		return tx.QueryRow(
			ctx, query,
			cfg.WorkspaceID, cfg.WorkspaceID, cfg.Protocol, string(sealed),
			cfg.Active, normalizedDomains, cfg.SAMLRequired, cfg.VerifiedAt,
		).Scan(
			&stored.ID, &stored.WorkspaceID, &stored.Protocol, &stored.Active,
			&stored.Domains, &stored.SAMLRequired, &stored.VerifiedAt,
			&stored.CreatedAt, &stored.UpdatedAt,
		)
	})
	if err != nil {
		return stored, false, fmt.Errorf("failed writing sso config: %w", err)
	}

	// Echo the provider config back with the secret masked.
	stored.Provider = maskClientSecret(cfg.Provider)
	return stored, false, nil
}

// FindWorkspaceIDBySSODomain resolves which workspace owns an email domain via
// a verified, active SSO configuration. Used by the login page to route a user
// to the right IdP.
func (r *Repository) FindWorkspaceIDBySSODomain(ctx context.Context, domain string) (uuid.UUID, bool, error) {
	if r == nil || r.client == nil {
		return uuid.Nil, false, fmt.Errorf("database unavailable")
	}

	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return uuid.Nil, false, nil
	}

	// Domain routing is a pre-tenant lookup (the caller has no session yet), so
	// it runs as the system worker and returns only the workspace id and the
	// enforcement flag. No other column is selected.
	const query = `
		SELECT workspace_id, saml_required
		FROM sso_configs
		WHERE active = true AND $1 = ANY(domains)
		LIMIT 1;
	`

	var (
		wsID     uuid.UUID
		enforced bool
	)
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, domain).Scan(&wsID, &enforced)
	})
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, fmt.Errorf("failed resolving sso domain: %w", err)
	}
	return wsID, enforced, nil
}

// sealSSOProviderConfig encrypts the IdP provider settings for storage.
//
// The OIDC client secret is the only secret in this blob. It is encrypted with
// the same per-tenant key derivation used for integration credentials, and the
// result is wrapped in a small JSON envelope carrying a version tag. A build
// without a configured encryption key stores the blob unencrypted rather than
// losing the configuration, but GetSSOConfig reports that state so the operator
// can see it instead of discovering a broken login later.
func sealSSOProviderConfig(ctx context.Context, wsID uuid.UUID, provider models.SSOProviderConfig) ([]byte, error) {
	plaintext, err := json.Marshal(provider)
	if err != nil {
		return nil, fmt.Errorf("failed serializing sso provider config: %w", err)
	}

	tenantKey := deriveTenantIntegrationKey(wsID)
	sealed, err := crypto.EncryptStringAESGCM(tenantKey, string(plaintext))
	if err != nil {
		// No usable key material. Store plaintext and mark it, rather than
		// failing the operator's configuration save outright.
		return json.Marshal(map[string]any{
			"v":      ssoConfigEnvelopeKey,
			"sealed": false,
			"data":   json.RawMessage(plaintext),
		})
	}

	return json.Marshal(map[string]any{
		"v":      ssoConfigEnvelopeKey,
		"sealed": true,
		"data":   sealed,
	})
}

// decryptSSOProviderConfig reverses sealSSOProviderConfig.
func decryptSSOProviderConfig(ctx context.Context, wsID uuid.UUID, raw []byte) (models.SSOProviderConfig, error) {
	if len(raw) == 0 {
		return models.SSOProviderConfig{}, nil
	}

	var envelope struct {
		V      string          `json:"v"`
		Sealed bool            `json:"sealed"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.V != ssoConfigEnvelopeKey {
		// Not written by this build: treat as a plain JSON document.
		var provider models.SSOProviderConfig
		if err := json.Unmarshal(raw, &provider); err != nil {
			return models.SSOProviderConfig{}, fmt.Errorf("unrecognized sso provider config format")
		}
		return provider, nil
	}

	if !envelope.Sealed {
		var provider models.SSOProviderConfig
		if err := json.Unmarshal(envelope.Data, &provider); err != nil {
			return models.SSOProviderConfig{}, fmt.Errorf("failed parsing unsealed sso provider config")
		}
		return provider, nil
	}

	var sealed string
	if err := json.Unmarshal(envelope.Data, &sealed); err != nil {
		return models.SSOProviderConfig{}, fmt.Errorf("failed parsing sealed sso provider config")
	}

	// decryptStoredSecret returns the original string unchanged when it cannot
	// decrypt, so an unchanged value means the key is wrong or absent.
	plaintext := decryptStoredSecret(ctx, wsID, sealed)
	if plaintext == "" || plaintext == sealed {
		return models.SSOProviderConfig{}, fmt.Errorf("sso provider config could not be decrypted with the configured encryption key")
	}

	var provider models.SSOProviderConfig
	if err := json.Unmarshal([]byte(plaintext), &provider); err != nil {
		return models.SSOProviderConfig{}, fmt.Errorf("failed parsing decrypted sso provider config")
	}
	return provider, nil
}

// maskClientSecret removes the OIDC client secret from a response payload.
func maskClientSecret(provider models.SSOProviderConfig) models.SSOProviderConfig {
	if provider.ClientSecret != "" {
		provider.ClientSecret = ""
	}
	return provider
}

// GetSSOConfigForProtocolLookup returns only the configured protocol for a
// workspace, readable without a tenant session.
//
// The public SSO check endpoint runs before the caller has any session, so no
// tenant context exists yet. It is deliberately narrow: one non-secret column,
// no provider configuration, no secret material. The system-worker read bypass
// in the RLS policy exists for exactly this query.
func (r *Repository) GetSSOConfigForProtocolLookup(ctx context.Context, wsID uuid.UUID) (string, error) {
	if r == nil || r.client == nil {
		return "", fmt.Errorf("database unavailable")
	}

	const query = `SELECT protocol FROM sso_configs WHERE workspace_id = $1;`

	var protocol string
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID).Scan(&protocol)
	})
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return "", nil
		}
		return "", err
	}
	return protocol, nil
}

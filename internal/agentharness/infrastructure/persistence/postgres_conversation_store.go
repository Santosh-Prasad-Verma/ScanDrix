// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/scandrix/backend/internal/agentharness/contracts"
)

const (
	defaultTableName = "scandrix_agent_sessions"
	defaultTenant    = "scandrix-agent-conversation"
)

// PgxExecutor defines the minimal query interface satisfied by *pgxpool.Pool and pgx.Tx.
type PgxExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row
}

// PostgresConversationStore persists conversation turns into PostgreSQL.
// Provides sliding window history capping and tenant-indexed session persistence.
type PostgresConversationStore struct {
	pool        PgxExecutor
	tableName   string
	maxMessages int
	tenantID    string
}

// PersistedTurn represents the JSONB schema for an individual conversation message.
type PersistedTurn struct {
	Role      contracts.AgentRole `json:"role"`
	Content   string              `json:"content"`
	Timestamp int64               `json:"ts"`
}

// NewPostgresConversationStore initializes the PostgreSQL conversation store.
// Automatically reads SCANDRIX_AGENT_SESSIONS_TABLE and SCANDRIX_AGENT_MAX_HISTORY.
func NewPostgresConversationStore(pool PgxExecutor) *PostgresConversationStore {
	tableName := defaultTableName
	if envTable := os.Getenv("SCANDRIX_AGENT_SESSIONS_TABLE"); envTable != "" {
		tableName = envTable
	}

	maxMsgs := defaultMaxMessages
	if envMax := os.Getenv("SCANDRIX_AGENT_MAX_HISTORY"); envMax != "" {
		if parsed, err := strconv.Atoi(envMax); err == nil && parsed > 0 {
			maxMsgs = parsed
		}
	}

	tenant := defaultTenant
	if envTenant := os.Getenv("SCANDRIX_AGENT_DEFAULT_TENANT"); envTenant != "" {
		tenant = envTenant
	}

	store := &PostgresConversationStore{
		pool:        pool,
		tableName:   tableName,
		maxMessages: maxMsgs,
		tenantID:    tenant,
	}

	return store
}

// AutoMigrate ensures the agent sessions table and indexes exist.
func (p *PostgresConversationStore) AutoMigrate(ctx context.Context) error {
	if p.pool == nil {
		return fmt.Errorf("postgres pool executor is nil")
	}

	query := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
    thread_id VARCHAR(255) PRIMARY KEY,
    session_id VARCHAR(255) NOT NULL,
    tenant_id VARCHAR(255) NOT NULL,
    organization_id VARCHAR(255),
    team_id VARCHAR(255),
    repository_id VARCHAR(255),
    channel VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    messages JSONB NOT NULL DEFAULT '[]'::jsonb,
    correlation_id_history JSONB NOT NULL DEFAULT '[]'::jsonb,
    last_correlation_id VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_%s_org_team ON %s(organization_id, team_id);
`, p.tableName, p.tableName, p.tableName)

	_, err := p.pool.Exec(ctx, query)
	return err
}

// Load retrieves prior turns for a given thread, oldest first.
// Best-effort: failures log a warning and return empty history without aborting turns.
func (p *PostgresConversationStore) Load(ctx context.Context, threadID string) ([]contracts.ConversationMessage, error) {
	if threadID == "" || p.pool == nil {
		return []contracts.ConversationMessage{}, nil
	}

	query := fmt.Sprintf(`SELECT messages FROM %s WHERE thread_id = $1`, p.tableName)
	var rawJSON []byte
	err := p.pool.QueryRow(ctx, query, threadID).Scan(&rawJSON)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []contracts.ConversationMessage{}, nil
		}
		slog.Warn("PostgresConversationStore.Load failed; returning empty history",
			"thread_id", threadID,
			"error", err,
		)
		return []contracts.ConversationMessage{}, nil
	}

	if len(rawJSON) == 0 {
		return []contracts.ConversationMessage{}, nil
	}

	var stored []PersistedTurn
	if err := json.Unmarshal(rawJSON, &stored); err != nil {
		slog.Warn("PostgresConversationStore.Load JSON decode failed", "thread_id", threadID, "error", err)
		return []contracts.ConversationMessage{}, nil
	}

	messages := make([]contracts.ConversationMessage, len(stored))
	for i, turn := range stored {
		messages[i] = contracts.ConversationMessage{
			Role:    turn.Role,
			Content: turn.Content,
		}
	}
	return messages, nil
}

// Append persists new conversation turns alongside metadata with upsert semantics.
// Bounded to maxMessages sliding window. Best-effort error handling.
func (p *PostgresConversationStore) Append(
	ctx context.Context,
	threadID string,
	turns []contracts.ConversationMessage,
	meta *contracts.ConversationAppendMeta,
) error {
	if threadID == "" || len(turns) == 0 || p.pool == nil {
		return nil
	}

	now := time.Now()
	nowUnix := now.UnixMilli()

	// Load existing to concatenate and slide window
	existing, _ := p.Load(ctx, threadID)
	allTurns := append(existing, turns...)

	if len(allTurns) > p.maxMessages {
		allTurns = allTurns[len(allTurns)-p.maxMessages:]
	}

	persisted := make([]PersistedTurn, len(allTurns))
	for i, t := range allTurns {
		persisted[i] = PersistedTurn{
			Role:      t.Role,
			Content:   t.Content,
			Timestamp: nowUnix,
		}
	}

	messagesJSON, err := json.Marshal(persisted)
	if err != nil {
		slog.Warn("PostgresConversationStore.Append failed to marshal turns", "error", err)
		return nil
	}

	tenantID := p.tenantID
	var orgID, teamID, repoID, channel, correlationID string
	if meta != nil {
		if meta.TenantID != "" {
			tenantID = meta.TenantID
		}
		orgID = meta.OrganizationID
		teamID = meta.TeamID
		repoID = meta.RepositoryID
		channel = meta.Channel
		correlationID = meta.CorrelationID
	}

	sessionID := uuid.New().String()

	query := fmt.Sprintf(`
INSERT INTO %s (
    thread_id,
    session_id,
    tenant_id,
    organization_id,
    team_id,
    repository_id,
    channel,
    status,
    messages,
    correlation_id_history,
    last_correlation_id,
    created_at,
    last_activity_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, 'active', $8, jsonb_build_array($9::text), $9, $10, $10
)
ON CONFLICT (thread_id) DO UPDATE SET
    messages = EXCLUDED.messages,
    organization_id = COALESCE(NULLIF(EXCLUDED.organization_id, ''), %s.organization_id),
    team_id = COALESCE(NULLIF(EXCLUDED.team_id, ''), %s.team_id),
    repository_id = COALESCE(NULLIF(EXCLUDED.repository_id, ''), %s.repository_id),
    channel = COALESCE(NULLIF(EXCLUDED.channel, ''), %s.channel),
    last_correlation_id = COALESCE(NULLIF(EXCLUDED.last_correlation_id, ''), %s.last_correlation_id),
    correlation_id_history = CASE 
        WHEN EXCLUDED.last_correlation_id IS NOT NULL AND EXCLUDED.last_correlation_id <> '' 
        THEN %s.correlation_id_history || jsonb_build_array(EXCLUDED.last_correlation_id)
        ELSE %s.correlation_id_history
    END,
    last_activity_at = EXCLUDED.last_activity_at;
`, p.tableName, p.tableName, p.tableName, p.tableName, p.tableName, p.tableName, p.tableName, p.tableName)

	_, execErr := p.pool.Exec(
		ctx,
		query,
		threadID,
		sessionID,
		tenantID,
		orgID,
		teamID,
		repoID,
		channel,
		messagesJSON,
		correlationID,
		now,
	)

	if execErr != nil {
		slog.Warn("PostgresConversationStore.Append upsert failed",
			"thread_id", threadID,
			"error", execErr,
		)
	}

	return nil
}

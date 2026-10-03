// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Telemetry Subsystem
// Package: beacon
// File: collector.go
// ═══════════════════════════════════════════════════════════════

package beacon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/internal/database"
)

// IHeartbeatCollectorService defines the contract for compiling non-PII operational metrics.
type IHeartbeatCollectorService interface {
	Collect(ctx context.Context, input CollectInput) (*HeartbeatMetrics, error)
}

// DBQuerier abstracts SQL query execution for table aggregations.
type DBQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) interface{ Scan(dest ...any) error }
	Query(ctx context.Context, sql string, args ...any) (interface {
		Close()
		Next() bool
		Scan(dest ...any) error
		Err() error
	}, error)
}

// HeartbeatCollectorService gathers system runtime, database, and aggregated usage metrics.
type HeartbeatCollectorService struct {
	// client, not a bare pool. Several collector queries are deliberately
	// cross-tenant aggregates (e.g. "does any workspace have rules enabled?")
	// and must run with app.is_system_worker set, otherwise row-level security
	// silently filters every row and the metric reports a confident false
	// forever (AUDIT_REMEDIATION.md F-37, AGENTS.md 2.7).
	client *database.Client
	logger *slog.Logger
}

// NewHeartbeatCollectorService initializes a telemetry metrics collector.
func NewHeartbeatCollectorService(client *database.Client, logger *slog.Logger) *HeartbeatCollectorService {
	if logger == nil {
		logger = slog.Default()
	}
	return &HeartbeatCollectorService{
		client: client,
		logger: logger.With("component", "heartbeat_collector"),
	}
}

// Collect compiles the anonymous heartbeat payload.
// Each metric query runs isolated inside safe execution with zero crash propagation.
func (c *HeartbeatCollectorService) Collect(ctx context.Context, input CollectInput) (*HeartbeatMetrics, error) {
	uptimeHours := c.computeUptimeHours(input.FirstSeenAt)

	dbVersion := safeExecute(c.logger, "db_version", func() (string, error) {
		return c.queryDBVersion(ctx)
	}, "unknown")

	workspacesCount := safeExecute(c.logger, "workspaces", func() (int64, error) {
		return c.countTable(ctx, "workspaces")
	}, 0)

	teamsCount := safeExecute(c.logger, "teams", func() (int64, error) {
		return c.countTable(ctx, "teams")
	}, 0)

	reposCount := safeExecute(c.logger, "repos_connected", func() (int64, error) {
		return c.countTable(ctx, "tracked_repositories")
	}, 0)

	activeUsers := safeExecute(c.logger, "active_users", func() (int64, error) {
		return c.queryActiveUsers7d(ctx)
	}, 0)

	integrations := safeExecute(c.logger, "integrations", func() ([]string, error) {
		return c.queryIntegrations(ctx)
	}, []string{})

	prsReviewed := safeExecute(c.logger, "prs_reviewed", func() (int64, error) {
		return c.queryPrsReviewed7d(ctx)
	}, 0)

	drixyRulesEnabled := safeExecute(c.logger, "drixy_rules_enabled", func() (bool, error) {
		return c.queryDrixyRulesEnabled(ctx)
	}, false)

	metrics := &HeartbeatMetrics{
		ScanDrix: ScanDrixInfo{
			Version:     detectScanDrixVersion(),
			Deployment:  detectDeployment(),
			UptimeHours: uptimeHours,
		},
		Runtime: RuntimeInfo{
			GoVersion: runtime.Version(),
			OS:        detectOS(),
			Arch:      runtime.GOARCH,
			CPUCount:  runtime.NumCPU(),
			DBType:    "postgres",
			DBVersion: dbVersion,
		},
		Usage7d: Usage7d{
			ActiveUsers:          activeUsers,
			Organizations:        workspacesCount,
			Teams:                teamsCount,
			ReposConnected:       reposCount,
			PRsReviewed:          prsReviewed,
			SuggestionsGenerated: 0,
			SuggestionsApplied:   0,
		},
		Config: ConfigSummary{
			DrixyRulesEnabled:   drixyRulesEnabled,
			AgentReviewReposPct: 0,
			Integrations:        integrations,
		},
	}

	return metrics, nil
}

func (c *HeartbeatCollectorService) computeUptimeHours(firstSeenAtStr string) int64 {
	if firstSeenAtStr == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, firstSeenAtStr)
	if err != nil {
		t, err = time.Parse(time.RFC3339, firstSeenAtStr)
	}
	if err != nil {
		return 0
	}
	hours := int64(time.Since(t).Hours())
	if hours < 0 {
		return 0
	}
	return hours
}

func (c *HeartbeatCollectorService) queryDBVersion(ctx context.Context) (string, error) {
	if c.client == nil || c.client.Pool == nil {
		return "unknown", nil
	}
	var raw string
	err := c.client.Pool.QueryRow(ctx, "SELECT version() AS version").Scan(&raw)
	if err != nil {
		return "unknown", err
	}
	parts := strings.Fields(raw)
	if len(parts) >= 2 {
		return strings.Join(parts[:2], " "), nil
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return "unknown", nil
}

func (c *HeartbeatCollectorService) countTable(ctx context.Context, table string) (int64, error) {
	if c.client == nil || c.client.Pool == nil {
		return 0, nil
	}
	query := fmt.Sprintf(`SELECT COUNT(*)::bigint FROM "%s"`, table)
	var count int64
	err := c.client.Pool.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (c *HeartbeatCollectorService) queryActiveUsers7d(ctx context.Context) (int64, error) {
	if c.client == nil || c.client.Pool == nil {
		return 0, nil
	}
	var count int64
	// 1. Try account_profiles updated in last 7 days
	if err := c.client.Pool.QueryRow(ctx, `SELECT COUNT(DISTINCT email)::bigint FROM account_profiles WHERE updated_at > now() - interval '7 days'`).Scan(&count); err == nil && count > 0 {
		return count, nil
	}
	// 2. Try user_sessions created/active in last 7 days
	if err := c.client.Pool.QueryRow(ctx, `SELECT COUNT(DISTINCT user_id)::bigint FROM user_sessions WHERE created_at > now() - interval '7 days'`).Scan(&count); err == nil && count > 0 {
		return count, nil
	}
	// 3. Fallback: total account_profiles count
	if err := c.client.Pool.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM account_profiles`).Scan(&count); err == nil {
		return count, nil
	}
	return 0, nil
}

func (c *HeartbeatCollectorService) queryIntegrations(ctx context.Context) ([]string, error) {
	if c.client == nil || c.client.Pool == nil {
		return []string{}, nil
	}
	query := `SELECT DISTINCT lower(provider::text) FROM integration_connections WHERE is_connected = true`
	rows, err := c.client.Pool.Query(ctx, query)
	if err != nil {
		return []string{}, err
	}
	defer rows.Close()

	seen := make(map[string]struct{})
	for rows.Next() {
		var provider string
		if err := rows.Scan(&provider); err != nil {
			continue
		}
		provider = strings.TrimSpace(strings.ToLower(provider))
		if provider == "" {
			continue
		}
		if _, known := KnownIntegrations[provider]; known {
			seen[provider] = struct{}{}
		} else {
			seen["other"] = struct{}{}
		}
	}

	result := make([]string, 0, len(seen))
	for p := range seen {
		result = append(result, p)
	}
	sort.Strings(result)
	return result, nil
}

func (c *HeartbeatCollectorService) queryPrsReviewed7d(ctx context.Context) (int64, error) {
	if c.client == nil || c.client.Pool == nil {
		return 0, nil
	}
	var count int64
	// 1. Try pull_request_reviews
	if err := c.client.Pool.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM pull_request_reviews WHERE created_at > now() - interval '7 days'`).Scan(&count); err == nil {
		return count, nil
	}
	// 2. Fallback to code_reviews
	if err := c.client.Pool.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM code_reviews WHERE created_at > now() - interval '7 days'`).Scan(&count); err == nil {
		return count, nil
	}
	return 0, nil
}

func (c *HeartbeatCollectorService) queryDrixyRulesEnabled(ctx context.Context) (bool, error) {
	if c.client == nil || c.client.Pool == nil {
		return false, nil
	}
	// Cross-tenant, so it runs as a system worker: drixy_rules is RLS-FORCEd
	// and this aggregate is exactly the query that would otherwise see nothing.
	asSystem := func(query string) (int64, error) {
		var n int64
		err := c.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, query).Scan(&n)
		})
		return n, err
	}

	// 1. Try drixy_rules JSON array
	if n, err := asSystem(`SELECT COUNT(*)::bigint FROM drixy_rules WHERE jsonb_array_length(rules) > 0`); err == nil && n > 0 {
		return true, nil
	}
	// 2. Try review_rules table
	if n, err := asSystem(`SELECT COUNT(*)::bigint FROM review_rules WHERE is_enabled = true`); err == nil && n > 0 {
		return true, nil
	}
	return false, nil
}

func safeExecute[T any](logger *slog.Logger, label string, fn func() (T, error), fallback T) T {
	defer func() {
		if r := recover(); r != nil {
			logger.Warn("Telemetry metric collector recovered from panic (using fallback)",
				"label", label,
				"panic", fmt.Sprintf("%v", r),
			)
		}
	}()

	val, err := fn()
	if err != nil {
		logger.Warn("Telemetry metric collection failed (using fallback)",
			"label", label,
			"error", err,
		)
		return fallback
	}
	return val
}

func detectScanDrixVersion() string {
	if v := strings.TrimSpace(os.Getenv("SCANDRIX_VERSION")); v != "" {
		return v
	}
	return "1.0.0"
}

func detectDeployment() DeploymentType {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return DeploymentKubernetes
	}
	if os.Getenv("COMPOSE_PROJECT_NAME") != "" {
		return DeploymentDockerCompose
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return DeploymentDocker
	}
	if os.Getenv("DOCKER_CONTAINER") != "" {
		return DeploymentDocker
	}
	return DeploymentBare
}

func detectOS() string {
	switch runtime.GOOS {
	case "linux":
		return "linux"
	case "darwin":
		return "darwin"
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

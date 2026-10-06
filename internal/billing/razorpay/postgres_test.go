package razorpay_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/database"
	"github.com/stretchr/testify/require"
)

func billingTestRepository(t *testing.T) (*database.Repository, uuid.UUID) {
	t.Helper()
	dsn := os.Getenv("SCANDRIX_ANALYTICS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SCANDRIX_ANALYTICS_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	schema, role := "billing_test_"+suffix, "billing_role_"+suffix
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema+"; CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE; DROP ROLE "+role)
		require.NoError(t, err)
		admin.Close()
	})
	config, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	config.ConnConfig.RuntimeParams["search_path"] = schema
	setup, err := pgxpool.NewWithConfig(ctx, config.Copy())
	require.NoError(t, err)
	defer setup.Close()
	for _, migration := range []string{"001_initial_schema", "004_extended_warehouse_and_billing", "005_teams_parameters_audit_and_integrations", "006_billing_transactions", "007_plan_configurations", "012_system_worker_outbox_rls", "018_workspaces_rls", "032_plan_pricing_integrity", "043_billing_intervals", "045_billing_webhook_integrity", "046_billing_seats_single_row_per_workspace"} {
		body, err := os.ReadFile("../../../migrations/" + migration + ".sql")
		require.NoError(t, err)
		_, err = setup.Exec(ctx, string(body))
		require.NoError(t, err)
	}
	_, err = setup.Exec(ctx, "GRANT USAGE ON SCHEMA "+schema+" TO "+role+"; GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA "+schema+" TO "+role)
	require.NoError(t, err)
	workspace := uuid.New()
	_, err = setup.Exec(ctx, `INSERT INTO workspaces(id,slug,name) VALUES ($1,'billing-fixture','Billing fixture');`, workspace)
	require.NoError(t, err)
	_, err = setup.Exec(ctx, `INSERT INTO account_profiles(workspace_id,email,display_name,role) VALUES ($1,'owner@example.test','Billing fixture owner','OWNER')`, workspace)
	require.NoError(t, err)
	config.AfterConnect = func(ctx context.Context, connection *pgx.Conn) error {
		_, err := connection.Exec(ctx, "SET ROLE "+role)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return database.NewRepository(&database.Client{Pool: pool}), workspace
}

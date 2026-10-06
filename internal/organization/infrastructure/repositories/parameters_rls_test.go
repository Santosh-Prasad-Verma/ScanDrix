package repositories_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	paramdomain "github.com/scandrix/backend/internal/organization/domain/parameters"
	"github.com/scandrix/backend/internal/organization/infrastructure/repositories"
	"github.com/stretchr/testify/require"
)

// migrationNumber extracts the leading numeric prefix of a migration filename
// so the fixture applies them in deployment order.
func migrationNumber(path string) int {
	base := filepath.Base(path)
	prefix, _, _ := strings.Cut(base, "_")
	n, err := strconv.Atoi(prefix)
	if err != nil {
		return 1 << 30
	}
	return n
}

// parametersRLSFixture returns a repository bound to an isolated schema whose
// connections run as a NOSUPERUSER NOBYPASSRLS role, which is how
// scandrix_runtime behaves in production. A bare pool call in this
// configuration matches zero rows instead of erroring, so a green suite here
// means the repository really does carry a tenant context.
func parametersRLSFixture(t *testing.T) (*repositories.PostgresParametersRepository, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	dsn := os.Getenv("SCANDRIX_ANALYTICS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SCANDRIX_ANALYTICS_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer admin.Close()

	// A dedicated database, not a schema: pgvector installs into public, and a
	// schema-scoped search_path would hide the vector type that migration 002
	// needs. An empty database also guarantees CREATE TABLE IF NOT EXISTS
	// cannot silently bind to a table that already exists elsewhere.
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	dbName, role := "params_rls_"+suffix, "params_rls_role_"+suffix
	// CREATE DATABASE cannot run inside a transaction block, so it must not be
	// batched with the role creation in a single Exec.
	_, err = admin.Exec(ctx, "CREATE DATABASE "+dbName+" ENCODING 'UTF8'")
	require.NoError(t, err)
	_, err = admin.Exec(ctx, "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS")
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanup, err := pgxpool.New(ctx, dsn)
		if err == nil {
			_, _ = cleanup.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)")
			_, _ = cleanup.Exec(ctx, "DROP ROLE IF EXISTS "+role)
			cleanup.Close()
		}
	})

	base, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	base.ConnConfig.Database = dbName
	setup, err := pgxpool.NewWithConfig(ctx, base.Copy())
	require.NoError(t, err)

	// Apply every migration in the order the real runner does, so the fixture
	// reflects the shipped schema rather than a hand-picked subset.
	paths, err := filepath.Glob("../../../../migrations/*.sql")
	require.NoError(t, err)
	sort.Slice(paths, func(i, j int) bool {
		return migrationNumber(paths[i]) < migrationNumber(paths[j])
	})
	require.NotEmpty(t, paths)
	for _, path := range paths {
		body, err := os.ReadFile(path)
		require.NoError(t, err, "reading %s", path)
		_, err = setup.Exec(ctx, string(body))
		require.NoError(t, err, "applying %s", filepath.Base(path))
	}
	_, err = setup.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+role+
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO "+role)
	require.NoError(t, err)
	setup.Close()

	wsA, wsB := uuid.New(), uuid.New()
	for i, ws := range []uuid.UUID{wsA, wsB} {
		seed, err := pgxpool.NewWithConfig(ctx, base.Copy())
		require.NoError(t, err)
		_, err = seed.Exec(ctx, `INSERT INTO workspaces(id,slug,name) VALUES ($1,$2,$3)`, ws,
			"params-rls-"+string(rune('a'+i)), "Params RLS fixture")
		require.NoError(t, err)
		seed.Close()
	}

	// Every connection in this pool runs as a NOSUPERUSER NOBYPASSRLS role,
	// which is exactly how scandrix_runtime behaves in production.
	config := base.Copy()
	config.AfterConnect = func(ctx context.Context, connection *pgx.Conn) error {
		_, err := connection.Exec(ctx, "SET ROLE "+role)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	// One team row, owned by wsA, reused by wsB's fixture row below. The
	// team_id foreign key is satisfied for both, so DeleteByTeamID has to rely
	// on the workspace predicate alone to keep the two apart.
	teamID := uuid.New()
	seed, err := pgxpool.NewWithConfig(ctx, base.Copy())
	require.NoError(t, err)
	defer seed.Close()
	_, err = seed.Exec(ctx, `INSERT INTO teams(id,workspace_id,name) VALUES ($1,$2,'Shared team')`, teamID, wsA)
	require.NoError(t, err)

	return repositories.NewPostgresParametersRepository(pool), wsA, wsB, teamID
}

func paramEntity(wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey, val string) *paramdomain.ParametersEntity {
	return &paramdomain.ParametersEntity{
		UUID:        uuid.New(),
		WorkspaceID: wsID,
		TeamID:      teamID,
		ConfigKey:   key,
		ConfigValue: []byte(`{"v":"` + val + `"}`),
		Description: "rls fixture",
		Active:      true,
	}
}

// TestParametersCreateAndReadUnderRLS proves a tenant-scoped write is
// visible to its own tenant. Under a bare pool call the INSERT would be
// rejected or the read would return nothing.
func TestParametersCreateAndReadUnderRLS(t *testing.T) {
	ctx := context.Background()
	repo, wsA, _, teamID := parametersRLSFixture(t)

	created, err := repo.Create(ctx, paramEntity(wsA, &teamID, paramdomain.KeyCodeReviewConfig, "a"))
	require.NoError(t, err)
	require.NotNil(t, created)

	found, err := repo.FindByKey(ctx, wsA, &teamID, paramdomain.KeyCodeReviewConfig)
	require.NoError(t, err)
	require.NotNil(t, found, "tenant-scoped read must see its own row")
	require.Equal(t, wsA, found.WorkspaceID)

	byID, err := repo.FindByID(ctx, wsA, created.UUID)
	require.NoError(t, err)
	require.NotNil(t, byID, "FindByID reads inside the named tenant")
	require.Equal(t, wsA, byID.WorkspaceID)
}

// TestParametersAreInvisibleAcrossTenants is the assertion the whole rollout
// depends on: a row written by workspace A is not readable by workspace B.
func TestParametersAreInvisibleAcrossTenants(t *testing.T) {
	ctx := context.Background()
	repo, wsA, wsB, teamID := parametersRLSFixture(t)

	_, err := repo.Create(ctx, paramEntity(wsA, &teamID, paramdomain.KeyCodeReviewConfig, "secret"))
	require.NoError(t, err)

	// B asks for its own workspace with the same team id and key: no rows.
	leak, err := repo.FindByKey(ctx, wsB, &teamID, paramdomain.KeyCodeReviewConfig)
	require.NoError(t, err)
	require.Nil(t, leak, "workspace B must not observe workspace A's parameter")

	// A filter with no workspace must be refused rather than widened to all tenants.
	all, err := repo.Find(ctx, paramdomain.ParametersFilter{TeamID: &teamID})
	require.ErrorIs(t, err, repositories.ErrTenantRequired)
	require.Nil(t, all)
}

// TestDeleteByTeamIDIsScopedToWorkspace guards the purge path. The query used
// to be `WHERE team_id = $1` with no tenant context, which under RLS matches
// nothing and reports a clean purge while leaving every row behind.
func TestDeleteByTeamIDIsScopedToWorkspace(t *testing.T) {
	ctx := context.Background()
	repo, wsA, wsB, teamID := parametersRLSFixture(t)

	_, err := repo.Create(ctx, paramEntity(wsA, &teamID, paramdomain.KeyCodeReviewConfig, "a"))
	require.NoError(t, err)
	_, err = repo.Create(ctx, paramEntity(wsB, &teamID, paramdomain.KeyPlatformConfigs, "b"))
	require.NoError(t, err)

	require.NoError(t, repo.DeleteByTeamID(ctx, wsA, teamID))

	gone, err := repo.FindByKey(ctx, wsA, &teamID, paramdomain.KeyCodeReviewConfig)
	require.NoError(t, err)
	require.Nil(t, gone, "workspace A's parameter must be deactivated")

	kept, err := repo.FindByKey(ctx, wsB, &teamID, paramdomain.KeyPlatformConfigs)
	require.NoError(t, err)
	require.NotNil(t, kept, "workspace B shares the team_id but keeps its own row")
}

// TestCreateNewActiveVersionRunsInsideTenant covers the two-statement
// deactivate-then-insert. It used to open its own transaction off the pool,
// which carries no app.current_tenant_id, so RLS rejected both statements.
func TestCreateNewActiveVersionRunsInsideTenant(t *testing.T) {
	ctx := context.Background()
	repo, wsA, _, _ := parametersRLSFixture(t)

	first, err := repo.CreateNewActiveVersion(ctx, wsA, nil, paramdomain.KeyCodeReviewConfig, map[string]string{"v": "1"}, 1)
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := repo.CreateNewActiveVersion(ctx, wsA, nil, paramdomain.KeyCodeReviewConfig, map[string]string{"v": "2"}, 2)
	require.NoError(t, err)
	require.NotNil(t, second)

	active, err := repo.FindByKey(ctx, wsA, nil, paramdomain.KeyCodeReviewConfig)
	require.NoError(t, err)
	require.NotNil(t, active, "the new active version must be readable by its tenant")
	require.Equal(t, 2, active.Version)
}

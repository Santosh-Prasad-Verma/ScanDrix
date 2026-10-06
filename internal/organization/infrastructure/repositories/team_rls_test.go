package repositories_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	teamdomain "github.com/scandrix/backend/internal/organization/domain/team"
	memberdomain "github.com/scandrix/backend/internal/organization/domain/teammembers"
	globalparamdomain "github.com/scandrix/backend/internal/organization/domain/globalparameters"
	"github.com/scandrix/backend/internal/organization/infrastructure/repositories"
	"github.com/stretchr/testify/require"
)

// teamRLSFixture provisions an isolated disposable database applying full migrations
// and connects with a NOSUPERUSER NOBYPASSRLS role to replicate scandrix_runtime.
func teamRLSFixture(t *testing.T) (*repositories.PostgresTeamRepository, *repositories.PostgresTeamMemberRepository, *pgxpool.Pool, uuid.UUID, uuid.UUID) {
	t.Helper()
	dsn := os.Getenv("SCANDRIX_ANALYTICS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SCANDRIX_ANALYTICS_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer admin.Close()

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	dbName, role := "team_rls_"+suffix, "team_rls_role_"+suffix

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
			"team-rls-"+string(rune('a'+i)), "Team RLS fixture")
		require.NoError(t, err)
		seed.Close()
	}

	config := base.Copy()
	config.AfterConnect = func(ctx context.Context, connection *pgx.Conn) error {
		_, err := connection.Exec(ctx, "SET ROLE "+role)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return repositories.NewPostgresTeamRepository(pool), repositories.NewPostgresTeamMemberRepository(pool), pool, wsA, wsB
}

func TestTeamCreateAndReadUnderRLS(t *testing.T) {
	ctx := context.Background()
	teamRepo, _, _, wsA, wsB := teamRLSFixture(t)

	teamID := uuid.New()
	teamA := &teamdomain.TeamEntity{
		UUID:        teamID,
		WorkspaceID: wsA,
		Name:        "Backend Core",
		Description: "Platform team",
		Status:      true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	created, err := teamRepo.Create(ctx, teamA)
	require.NoError(t, err)
	require.NotNil(t, created)
	require.Equal(t, teamID, created.UUID)

	// Reading within the same workspace succeeds
	found, err := teamRepo.FindByID(ctx, wsA, teamID)
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, "Backend Core", found.Name)

	// Reading from workspace B must return nil (invisible across tenants)
	foundOther, err := teamRepo.FindByID(ctx, wsB, teamID)
	require.NoError(t, err)
	require.Nil(t, foundOther, "team must not be visible to workspace B")

	// FindByWorkspaceID list check
	teamsA, err := teamRepo.FindByWorkspaceID(ctx, wsA)
	require.NoError(t, err)
	require.Len(t, teamsA, 1)

	teamsB, err := teamRepo.FindByWorkspaceID(ctx, wsB)
	require.NoError(t, err)
	require.Empty(t, teamsB)

	// Update within workspace A succeeds
	created.Name = "Backend Core Updated"
	updated, err := teamRepo.Update(ctx, teamdomain.TeamFilter{UUID: &teamID, WorkspaceID: &wsA}, created)
	require.NoError(t, err)
	require.Equal(t, "Backend Core Updated", updated.Name)

	// Delete from workspace A
	err = teamRepo.Delete(ctx, wsA, teamID)
	require.NoError(t, err)

	gone, err := teamRepo.FindByID(ctx, wsA, teamID)
	require.NoError(t, err)
	require.Nil(t, gone)
}

func TestTeamMemberIsolationUnderRLS(t *testing.T) {
	ctx := context.Background()
	teamRepo, memberRepo, _, wsA, wsB := teamRLSFixture(t)

	// Create team in wsA
	teamA := &teamdomain.TeamEntity{
		UUID:        uuid.New(),
		WorkspaceID: wsA,
		Name:        "Team Alpha",
		Description: "Alpha team",
		Status:      true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	_, err := teamRepo.Create(ctx, teamA)
	require.NoError(t, err)

	// Add member to team in wsA
	userID := uuid.New()
	memberA := &memberdomain.TeamMemberEntity{
		UUID:        uuid.New(),
		TeamID:      teamA.UUID,
		WorkspaceID: wsA,
		UserID:      userID,
		Email:       "alice@example.com",
		Role:        memberdomain.RoleMember,
		Status:      true,
		JoinedAt:    time.Now(),
	}
	createdMember, err := memberRepo.Create(ctx, memberA)
	require.NoError(t, err)
	require.NotNil(t, createdMember)

	// Workspace A can find the member
	membersA, err := memberRepo.Find(ctx, memberdomain.TeamMemberFilter{
		WorkspaceID: &wsA,
		TeamID:      &teamA.UUID,
	})
	require.NoError(t, err)
	require.Len(t, membersA, 1)
	require.Equal(t, "alice@example.com", membersA[0].Email)

	// CountByUser in workspace A works
	countA, err := memberRepo.CountByUser(ctx, wsA, userID, nil)
	require.NoError(t, err)
	require.Equal(t, 1, countA)

	// Calling Find with missing WorkspaceID must return ErrTenantRequired
	_, err = memberRepo.Find(ctx, memberdomain.TeamMemberFilter{TeamID: &teamA.UUID})
	require.ErrorIs(t, err, repositories.ErrTenantRequired)

	// Calling with workspace B must return no rows (isolation)
	membersB, err := memberRepo.Find(ctx, memberdomain.TeamMemberFilter{
		WorkspaceID: &wsB,
		TeamID:      &teamA.UUID,
	})
	require.NoError(t, err)
	require.Empty(t, membersB)

	// Delete member in workspace A
	err = memberRepo.Delete(ctx, wsA, teamA.UUID, userID)
	require.NoError(t, err)

	countAfter, err := memberRepo.CountByUser(ctx, wsA, userID, nil)
	require.NoError(t, err)
	require.Equal(t, 0, countAfter)
}

func TestGlobalParametersUnderSystemWorkerRLS(t *testing.T) {
	ctx := context.Background()
	_, _, pool, _, _ := teamRLSFixture(t)
	globalRepo := repositories.NewPostgresGlobalParametersRepository(pool)

	entity, err := globalparamdomain.NewGlobalParametersEntity("telemetry_heartbeat", map[string]any{"rate_seconds": 60}, "Heartbeat rate")
	require.NoError(t, err)

	err = globalRepo.Create(ctx, entity)
	require.NoError(t, err)

	found, err := globalRepo.FindByKey(ctx, "telemetry_heartbeat")
	require.NoError(t, err)
	require.NotNil(t, found, "global parameter must be readable via ExecAsSystem")
	require.Equal(t, "telemetry_heartbeat", found.ConfigKey)

	list, err := globalRepo.List(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, list)

	byID, err := globalRepo.FindByID(ctx, entity.UUID)
	require.NoError(t, err)
	require.NotNil(t, byID)

	updatedAt, err := globalRepo.FindUpdatedAtByKey(ctx, "telemetry_heartbeat")
	require.NoError(t, err)
	require.NotNil(t, updatedAt)

	err = globalRepo.Delete(ctx, entity.UUID)
	require.NoError(t, err)

	deleted, err := globalRepo.FindByKey(ctx, "telemetry_heartbeat")
	require.NoError(t, err)
	require.Nil(t, deleted)
}


package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// TestAuthSessionsPostgres exercises the durable refresh-session lifecycle
// against the actual migrations using a NOSUPERUSER NOBYPASSRLS login role.
// The opt-in DSN must allow CREATE SCHEMA and CREATE ROLE on a disposable DB.
func TestAuthSessionsPostgres(t *testing.T) {
	dsn := os.Getenv("SCANDRIX_ANALYTICS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SCANDRIX_ANALYTICS_TEST_DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	adminConfig, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	admin, err := pgxpool.NewWithConfig(ctx, adminConfig)
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	require.NoError(t, admin.Ping(ctx))

	schema := "auth_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	runtimeRole := "auth_role_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	roleCreated := false
	_, err = admin.Exec(ctx, `CREATE SCHEMA "`+schema+`"`)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		_, cleanupErr := admin.Exec(cleanupCtx, `DROP SCHEMA "`+schema+`" CASCADE`)
		if cleanupErr != nil {
			t.Errorf("drop test schema: %v", cleanupErr)
		}
		if roleCreated {
			_, cleanupErr = admin.Exec(cleanupCtx, "DROP ROLE "+pgx.Identifier{runtimeRole}.Sanitize())
			if cleanupErr != nil {
				t.Errorf("drop test role: %v", cleanupErr)
			}
		}
	})

	setupConfig, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	setupConfig.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	setup, err := pgxpool.NewWithConfig(ctx, setupConfig)
	require.NoError(t, err)
	defer setup.Close()

	for _, migration := range []string{
		"001_initial_schema.sql",
		"016_users_and_auth_tables.sql",
		"018_workspaces_rls.sql",
		"033_users_rls.sql",
		"039_auth_token_hash.sql",
		"041_revoked_access_tokens.sql",
	} {
		migrationSQL, readErr := os.ReadFile(filepath.Join("..", "..", "migrations", migration))
		require.NoError(t, readErr)
		_, execErr := setup.Exec(ctx, string(migrationSQL))
		require.NoError(t, execErr, migration)
	}

	runtimePassword := integrationToken(t)
	var createRoleSQL string
	require.NoError(t, admin.QueryRow(ctx, `SELECT format('CREATE ROLE %I LOGIN NOSUPERUSER NOBYPASSRLS PASSWORD %L', $1::text, $2::text)`, runtimeRole, runtimePassword).Scan(&createRoleSQL))
	_, err = admin.Exec(ctx, createRoleSQL)
	require.NoError(t, err)
	roleCreated = true
	_, err = setup.Exec(ctx, "GRANT USAGE ON SCHEMA "+pgx.Identifier{schema}.Sanitize()+" TO "+pgx.Identifier{runtimeRole}.Sanitize()+
		"; GRANT SELECT, INSERT, UPDATE, DELETE ON users, auth, workspaces, revoked_access_tokens TO "+pgx.Identifier{runtimeRole}.Sanitize())
	require.NoError(t, err)
	runtimeConfig := setupConfig.Copy()
	runtimeConfig.ConnConfig.User = runtimeRole
	runtimeConfig.ConnConfig.Password = runtimePassword
	runtimePool, err := pgxpool.NewWithConfig(ctx, runtimeConfig)
	require.NoError(t, err)
	t.Cleanup(runtimePool.Close)
	var superuser, bypassRLS bool
	require.NoError(t, runtimePool.QueryRow(ctx, `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&superuser, &bypassRLS))
	require.False(t, superuser)
	require.False(t, bypassRLS)

	workspaceID := uuid.New()
	userID := uuid.New()
	passwordHash := "password-hash-fixture"
	_, err = setup.Exec(ctx, `
		INSERT INTO workspaces (id, slug, name, status)
		VALUES ($1, $2, $3, 'ACTIVE')`, workspaceID, "auth-test-"+workspaceID.String(), "Auth test")
	require.NoError(t, err)
	_, err = setup.Exec(ctx, `
		INSERT INTO users (uuid, email, password, role, status, organization_id)
		VALUES ($1, $2, $3, 'member', 'active', $4)`,
		userID, "auth-test-"+userID.String()+"@example.test", passwordHash, workspaceID)
	require.NoError(t, err)

	client := &Client{Pool: runtimePool}
	repo := NewRepository(client)
	for _, table := range []string{"users", "workspaces"} {
		var count int
		require.NoError(t, runtimePool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count))
		require.Zero(t, count, "unscoped runtime reads must be denied by RLS")
	}
	require.NoError(t, client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
			return err
		}
		require.Equal(t, 1, count, "the authenticated tenant must see its seeded user")
		return nil
	}))
	require.NoError(t, client.ExecWithTenant(ctx, uuid.New(), func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
			return err
		}
		require.Zero(t, count, "another tenant must not see this user")
		return nil
	}))
	user := &UserRecord{
		UUID:           userID,
		Password:       passwordHash,
		Role:           "member",
		Status:         "active",
		OrganizationID: &workspaceID,
	}
	firstToken := integrationToken(t)
	secondToken := integrationToken(t)
	expiry := time.Now().Add(30 * time.Minute)

	require.NoError(t, repo.PersistAuthSession(ctx, user, firstToken, expiry))
	validated, err := repo.ValidateAuthSession(ctx, userID, workspaceID, hashRefreshToken(firstToken))
	require.NoError(t, err)
	require.Equal(t, userID, validated.UUID)
	require.Equal(t, workspaceID, *validated.OrganizationID)

	var storedHash string
	require.NoError(t, setup.QueryRow(ctx, `SELECT "tokenHash" FROM auth WHERE "userUuid"=$1`, userID).Scan(&storedHash))
	require.Equal(t, hashRefreshToken(firstToken), storedHash)
	require.NotEqual(t, firstToken, storedHash)

	require.NoError(t, repo.RotateRefreshToken(ctx, user, firstToken, secondToken, expiry))
	oldRecord, err := repo.GetRefreshToken(ctx, firstToken)
	require.NoError(t, err)
	require.True(t, oldRecord.Used)
	_, err = repo.ValidateAuthSession(ctx, userID, workspaceID, hashRefreshToken(firstToken))
	require.Error(t, err)
	_, err = repo.ValidateAuthSession(ctx, userID, workspaceID, hashRefreshToken(secondToken))
	require.NoError(t, err)

	thirdToken := integrationToken(t)
	require.ErrorIs(t, repo.RotateRefreshToken(ctx, user, firstToken, thirdToken, expiry), ErrRefreshTokenReuse)
	_, err = repo.ValidateAuthSession(ctx, userID, workspaceID, hashRefreshToken(secondToken))
	require.Error(t, err)

	cutoff := time.Now().UTC().Unix()
	require.NoError(t, repo.RevokeAccessTokensUpTo(ctx, userID, cutoff))
	for _, issuedAt := range []int64{cutoff - 1, cutoff} {
		revoked, revokeErr := repo.IsAccessTokenRevoked(ctx, userID, issuedAt)
		require.NoError(t, revokeErr)
		require.True(t, revoked, "issued_at=%d should be covered by cutoff=%d", issuedAt, cutoff)
	}
	revoked, err := repo.IsAccessTokenRevoked(ctx, userID, cutoff+1)
	require.NoError(t, err)
	require.False(t, revoked)

	concurrentUserID := uuid.New()
	_, err = setup.Exec(ctx, `
		INSERT INTO users (uuid, email, password, role, status, organization_id)
		VALUES ($1, $2, $3, 'member', 'active', $4)`,
		concurrentUserID, "auth-concurrent-"+concurrentUserID.String()+"@example.test", passwordHash, workspaceID)
	require.NoError(t, err)
	concurrentUser := &UserRecord{
		UUID:           concurrentUserID,
		Password:       passwordHash,
		Role:           "member",
		Status:         "active",
		OrganizationID: &workspaceID,
	}
	concurrentOld := integrationToken(t)
	concurrentNewA := integrationToken(t)
	concurrentNewB := integrationToken(t)
	require.NoError(t, repo.PersistAuthSession(ctx, concurrentUser, concurrentOld, expiry))

	rotationResults := make(chan error, 2)
	go func() {
		rotationResults <- repo.RotateRefreshToken(ctx, concurrentUser, concurrentOld, concurrentNewA, expiry)
	}()
	go func() {
		rotationResults <- repo.RotateRefreshToken(ctx, concurrentUser, concurrentOld, concurrentNewB, expiry)
	}()
	firstResult, secondResult := <-rotationResults, <-rotationResults
	results := []error{firstResult, secondResult}
	successes, replays := 0, 0
	for _, result := range results {
		if result == nil {
			successes++
		} else if errors.Is(result, ErrRefreshTokenReuse) {
			replays++
		} else {
			t.Fatalf("concurrent rotation returned unexpected error: %v", result)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, replays)
	for _, token := range []string{concurrentNewA, concurrentNewB} {
		record, recordErr := repo.GetRefreshToken(ctx, token)
		if recordErr == nil {
			require.True(t, record.Used, "refresh-token reuse must revoke the whole token family")
		} else {
			require.ErrorIs(t, recordErr, pgx.ErrNoRows)
		}
		_, validationErr := repo.ValidateAuthSession(ctx, concurrentUserID, workspaceID, hashRefreshToken(token))
		require.Error(t, validationErr, "a token from a replayed family must not validate")
	}

	rollbackUserID := uuid.New()
	_, err = setup.Exec(ctx, `
		INSERT INTO users (uuid, email, password, role, status, organization_id)
		VALUES ($1, $2, $3, 'member', 'active', $4)`,
		rollbackUserID, "auth-rollback-"+rollbackUserID.String()+"@example.test", passwordHash, workspaceID)
	require.NoError(t, err)
	rollbackUser := &UserRecord{
		UUID:           rollbackUserID,
		Password:       passwordHash,
		Role:           "member",
		Status:         "active",
		OrganizationID: &workspaceID,
	}
	rollbackOld := integrationToken(t)
	require.NoError(t, repo.PersistAuthSession(ctx, rollbackUser, rollbackOld, expiry))
	require.ErrorIs(t, repo.RotateRefreshToken(ctx, rollbackUser, rollbackOld, integrationToken(t), time.Now().Add(-time.Minute)), ErrInvalidRefreshToken)
	rollbackRecord, err := repo.GetRefreshToken(ctx, rollbackOld)
	require.NoError(t, err)
	require.False(t, rollbackRecord.Used, "failed rotation must leave the old refresh token usable")

	fixture := func(t *testing.T) (*UserRecord, string) {
		t.Helper()
		workspace, id := uuid.New(), uuid.New()
		_, err := setup.Exec(ctx, `INSERT INTO workspaces(id,slug,name) VALUES($1,$2,'Auth fixture')`, workspace, workspace.String())
		require.NoError(t, err)
		_, err = setup.Exec(ctx, `INSERT INTO users(uuid,email,password,role,status,organization_id) VALUES($1,$2,$3,'member','active',$4)`, id, id.String()+"@example.test", passwordHash, workspace)
		require.NoError(t, err)
		user := &UserRecord{UUID: id, Email: id.String() + "@example.test", Password: passwordHash, Role: "member", Status: "active", OrganizationID: &workspace}
		token := integrationToken(t)
		require.NoError(t, repo.PersistAuthSession(ctx, user, token, expiry))
		return user, token
	}

	t.Run("identity and token binding", func(t *testing.T) {
		user, token := fixture(t)
		_, err := repo.ValidateAuthSession(ctx, user.UUID, uuid.New(), hashRefreshToken(token))
		require.Error(t, err)
		_, err = repo.ValidateAuthSession(ctx, uuid.New(), *user.OrganizationID, hashRefreshToken(token))
		require.Error(t, err)
		_, err = repo.ValidateAuthSession(ctx, user.UUID, *user.OrganizationID, hashRefreshToken(integrationToken(t)))
		require.Error(t, err)
		_, err = repo.ValidateAuthSession(ctx, user.UUID, *user.OrganizationID, "invalid-session-hash")
		require.ErrorIs(t, err, ErrInvalidRefreshToken)
	})

	for _, status := range []string{"inactive", "removed", "pending_email"} {
		t.Run("user "+status, func(t *testing.T) {
			user, token := fixture(t)
			require.NoError(t, repo.UpdateUserStatus(ctx, user.UUID, status))
			_, err := repo.ValidateAuthSession(ctx, user.UUID, *user.OrganizationID, hashRefreshToken(token))
			require.Error(t, err)
			require.ErrorIs(t, repo.PersistAuthSession(ctx, user, integrationToken(t), expiry), ErrInvalidRefreshToken)
			require.ErrorIs(t, repo.RotateRefreshToken(ctx, user, token, integrationToken(t), expiry), ErrInvalidRefreshToken)
		})
	}

	for _, status := range []string{"SUSPENDED", "DELETED"} {
		t.Run("workspace "+status, func(t *testing.T) {
			user, token := fixture(t)
			_, err := setup.Exec(ctx, `UPDATE workspaces SET status=$1 WHERE id=$2`, status, *user.OrganizationID)
			require.NoError(t, err)
			_, err = repo.ValidateAuthSession(ctx, user.UUID, *user.OrganizationID, hashRefreshToken(token))
			require.Error(t, err)
			require.ErrorIs(t, repo.PersistAuthSession(ctx, user, integrationToken(t), expiry), ErrInvalidRefreshToken)
			require.ErrorIs(t, repo.RotateRefreshToken(ctx, user, token, integrationToken(t), expiry), ErrInvalidRefreshToken)
		})
	}

	t.Run("deleted user", func(t *testing.T) {
		user, token := fixture(t)
		_, err := setup.Exec(ctx, `DELETE FROM users WHERE uuid=$1`, user.UUID)
		require.NoError(t, err)
		_, err = repo.ValidateAuthSession(ctx, user.UUID, *user.OrganizationID, hashRefreshToken(token))
		require.Error(t, err)
		require.Error(t, repo.PersistAuthSession(ctx, user, integrationToken(t), expiry))
	})

	t.Run("expired session", func(t *testing.T) {
		user, token := fixture(t)
		_, err := setup.Exec(ctx, `UPDATE auth SET "expiryDate"=clock_timestamp()-interval '1 minute' WHERE "userUuid"=$1`, user.UUID)
		require.NoError(t, err)
		_, err = repo.ValidateAuthSession(ctx, user.UUID, *user.OrganizationID, hashRefreshToken(token))
		require.Error(t, err)
		require.ErrorIs(t, repo.RotateRefreshToken(ctx, user, token, integrationToken(t), expiry), ErrInvalidRefreshToken)
	})

	for _, reset := range []bool{false, true} {
		t.Run(fmt.Sprintf("rotation racing reset=%t", reset), func(t *testing.T) {
			user, token := fixture(t)
			replacement := integrationToken(t)
			start := make(chan struct{})
			rotation, revocation := make(chan error, 1), make(chan error, 1)
			go func() {
				<-start
				rotation <- repo.RotateRefreshToken(ctx, user, token, replacement, expiry)
			}()
			go func() {
				<-start
				if reset {
					revocation <- repo.ResetUserPassword(ctx, user.UUID, passwordHash, "changed-password-hash-fixture")
				} else {
					revocation <- repo.RevokeUserSessions(ctx, user.UUID, time.Now().Unix())
				}
			}()
			close(start)
			require.NoError(t, <-revocation)
			rotationErr := <-rotation
			require.True(t, rotationErr == nil || errors.Is(rotationErr, ErrRefreshTokenReuse) || errors.Is(rotationErr, ErrInvalidRefreshToken))
			for _, candidate := range []string{token, replacement} {
				_, err := repo.ValidateAuthSession(ctx, user.UUID, *user.OrganizationID, hashRefreshToken(candidate))
				require.Error(t, err, "a session racing logout/reset must not survive")
			}
			if reset {
				require.ErrorIs(t, repo.ResetUserPassword(ctx, user.UUID, passwordHash, "another-password-hash-fixture"), ErrInvalidRefreshToken)
				require.ErrorIs(t, repo.PersistAuthSession(ctx, user, integrationToken(t), expiry), ErrInvalidRefreshToken)
			}
		})
	}

	t.Run("transaction rollback after replacement insert", func(t *testing.T) {
		user, token := fixture(t)
		record, err := repo.GetRefreshToken(ctx, token)
		require.NoError(t, err)
		_, err = setup.Exec(ctx, `CREATE FUNCTION reject_auth_consume() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected token consume failure'; END $$`)
		require.NoError(t, err)
		_, err = setup.Exec(ctx, fmt.Sprintf(`CREATE TRIGGER reject_auth_consume BEFORE UPDATE ON auth FOR EACH ROW WHEN (OLD.uuid='%s'::uuid AND NEW.used) EXECUTE FUNCTION reject_auth_consume()`, record.UUID))
		require.NoError(t, err)
		replacement := integrationToken(t)
		err = repo.RotateRefreshToken(ctx, user, token, replacement, expiry)
		var postgresErr *pgconn.PgError
		require.ErrorAs(t, err, &postgresErr)
		require.Equal(t, "P0001", postgresErr.Code)
		_, err = repo.GetRefreshToken(ctx, replacement)
		require.ErrorIs(t, err, pgx.ErrNoRows, "failed consume must roll back the replacement insert")
		_, err = repo.ValidateAuthSession(ctx, user.UUID, *user.OrganizationID, hashRefreshToken(token))
		require.NoError(t, err)
		require.Error(t, repo.ResetUserPassword(ctx, user.UUID, passwordHash, "changed-password-hash-fixture"))
		current, err := repo.GetUserByID(ctx, user.UUID)
		require.NoError(t, err)
		require.Equal(t, passwordHash, current.Password, "failed revocation must roll back the password change")
		_, err = setup.Exec(ctx, `DROP TRIGGER reject_auth_consume ON auth; DROP FUNCTION reject_auth_consume()`)
		require.NoError(t, err)
	})

	t.Run("closed store denies session operations", func(t *testing.T) {
		user, token := fixture(t)
		runtimePool.Close()
		_, err := repo.ValidateAuthSession(ctx, user.UUID, *user.OrganizationID, hashRefreshToken(token))
		require.Error(t, err)
		require.Error(t, repo.PersistAuthSession(ctx, user, integrationToken(t), expiry))
		require.Error(t, repo.RotateRefreshToken(ctx, user, token, integrationToken(t), expiry))
		require.Error(t, repo.RevokeUserSessions(ctx, user.UUID, time.Now().Unix()))
	})
}

func integrationToken(t *testing.T) string {
	t.Helper()
	var raw [32]byte
	_, err := rand.Read(raw[:])
	require.NoError(t, err)
	return hex.EncodeToString(raw[:])
}

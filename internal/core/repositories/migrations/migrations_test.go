package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSQLExecutor struct {
	execErr  error
	executed []string
}

func (m *mockSQLExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	m.executed = append(m.executed, query)
	if m.execErr != nil {
		return nil, m.execErr
	}
	return nil, nil
}

func (m *mockSQLExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return nil, m.execErr
}

func (m *mockSQLExecutor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return nil
}

func TestAllMigrationsCompleteness(t *testing.T) {
	all := AllMigrations()
	require.Len(t, all, 29, "Expected exactly 29 registered migrations")

	seenVersions := make(map[string]bool)
	for i, m := range all {
		require.NotNil(t, m, "Migration at index %d is nil", i)
		expectedVer := fmt.Sprintf("%03d", i+1)
		assert.Equal(t, expectedVer, m.Version(), "Migration version sequence mismatch")
		assert.NotEmpty(t, m.Name(), "Migration name should not be empty")
		assert.False(t, seenVersions[m.Version()], "Duplicate migration version: %s", m.Version())
		seenVersions[m.Version()] = true
	}
}

func TestMigrationExecutionWithMockExecutor(t *testing.T) {
	all := AllMigrations()
	ctx := context.Background()

	for _, m := range all {
		t.Run(m.Name(), func(t *testing.T) {
			// Test successful Up
			execSuccess := &mockSQLExecutor{}
			err := m.Up(ctx, execSuccess)
			assert.NoError(t, err)
			assert.Len(t, execSuccess.executed, 1)
			assert.NotEmpty(t, execSuccess.executed[0])

			// Test successful Down
			execDownSuccess := &mockSQLExecutor{}
			err = m.Down(ctx, execDownSuccess)
			assert.NoError(t, err)
			assert.Len(t, execDownSuccess.executed, 1)
			assert.NotEmpty(t, execDownSuccess.executed[0])

			// Test failure propagation on Up
			expectedErr := errors.New("simulated execution failure")
			execFail := &mockSQLExecutor{execErr: expectedErr}
			err = m.Up(ctx, execFail)
			assert.ErrorIs(t, err, expectedErr)

			// Test failure propagation on Down
			execDownFail := &mockSQLExecutor{execErr: expectedErr}
			err = m.Down(ctx, execDownFail)
			assert.ErrorIs(t, err, expectedErr)
		})
	}
}

func TestMigrationRunnerSortingAndRegistration(t *testing.T) {
	runner := NewMigrationRunner(nil)
	require.NotNil(t, runner)

	// Register in reverse order
	all := AllMigrations()
	for i := len(all) - 1; i >= 0; i-- {
		runner.Register(all[i])
	}

	runner.mu.RLock()
	defer runner.mu.RUnlock()
	require.Len(t, runner.migrations, 29)
	for i := 0; i < 29; i++ {
		expectedVer := fmt.Sprintf("%03d", i+1)
		assert.Equal(t, expectedVer, runner.migrations[i].Version())
	}
}

func TestMigrationRunnerRegisterAll(t *testing.T) {
	runner := NewMigrationRunner(nil)
	RegisterAll(runner)

	runner.mu.RLock()
	defer runner.mu.RUnlock()
	require.Len(t, runner.migrations, 29)
	for i := 0; i < 29; i++ {
		expectedVer := fmt.Sprintf("%03d", i+1)
		assert.Equal(t, expectedVer, runner.migrations[i].Version())
	}
}

func TestMigrationStatusModel(t *testing.T) {
	now := time.Now().UTC()
	status := MigrationStatus{
		Version:   "001",
		Name:      "001_initial_schema",
		Applied:   true,
		AppliedAt: &now,
	}
	assert.Equal(t, "001", status.Version)
	assert.Equal(t, "001_initial_schema", status.Name)
	assert.True(t, status.Applied)
	assert.NotNil(t, status.AppliedAt)

	record := MigrationRecord{
		Version:   "001",
		AppliedAt: now,
	}
	assert.Equal(t, "001", record.Version)
	assert.Equal(t, now, record.AppliedAt)
}

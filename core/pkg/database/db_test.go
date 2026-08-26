package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/codehound/codehound/core/pkg/config"
	"github.com/codehound/codehound/core/pkg/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestPostgreSQLConnectivityAndMigrations(t *testing.T) {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, database.DefaultConfig(cfg.DatabaseURL))
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	defer pool.Close()

	// 1. Verify connection ping
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("failed to ping postgres: %v", err)
	}

	// 2. Verify schema version is at least 1
	ver, err := database.GetCurrentVersion(ctx, pool)
	if err != nil {
		t.Fatalf("failed to query schema version: %v", err)
	}
	if ver < 1 {
		t.Fatalf("expected schema version >= 1, got %d", ver)
	}
}

func TestRowLevelSecurityTenantIsolation(t *testing.T) {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, database.DefaultConfig(cfg.DatabaseURL))
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	defer pool.Close()

	// Create 2 test tenants
	tenantA_ID := uuid.New()
	tenantB_ID := uuid.New()

	_, err = pool.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, plan, status)
		VALUES 
			($1, $2, 'Tenant Alpha', 'ENTERPRISE', 'ACTIVE'),
			($3, $4, 'Tenant Beta', 'TEAM', 'ACTIVE')
		ON CONFLICT (slug) DO NOTHING;
	`, tenantA_ID, "tenant-alpha-"+tenantA_ID.String()[:8], tenantB_ID, "tenant-beta-"+tenantB_ID.String()[:8])
	if err != nil {
		t.Fatalf("failed to seed test tenants: %v", err)
	}

	// Insert Project A for Tenant A, and Project B for Tenant B
	projectA_ID := uuid.New()
	projectB_ID := uuid.New()

	_, err = pool.Exec(ctx, `
		INSERT INTO projects (id, tenant_id, slug, name, default_branch)
		VALUES 
			($1, $2, 'repo-alpha', 'Alpha Core', 'main'),
			($3, $4, 'repo-beta', 'Beta Service', 'main')
		ON CONFLICT (tenant_id, slug) DO NOTHING;
	`, projectA_ID, tenantA_ID, projectB_ID, tenantB_ID)
	if err != nil {
		t.Fatalf("failed to seed test projects: %v", err)
	}

	// 1. Query within Tenant A's isolated context
	err = database.ExecInTenantTx(ctx, pool, tenantA_ID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, name FROM projects;`)
		if err != nil {
			return err
		}
		defer rows.Close()

		var projectNames []string
		for rows.Next() {
			var id uuid.UUID
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				return err
			}
			projectNames = append(projectNames, name)
		}

		// Tenant A must ONLY see 'Alpha Core', never 'Beta Service'
		for _, name := range projectNames {
			if name == "Beta Service" {
				t.Fatalf("CRITICAL SECURITY VIOLATION: Tenant A was able to read Tenant B's project!")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("error executing in tenant A context: %v", err)
	}

	// 2. Query within Tenant B's isolated context
	err = database.ExecInTenantTx(ctx, pool, tenantB_ID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, name FROM projects;`)
		if err != nil {
			return err
		}
		defer rows.Close()

		var projectNames []string
		for rows.Next() {
			var id uuid.UUID
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				return err
			}
			projectNames = append(projectNames, name)
		}

		// Tenant B must ONLY see 'Beta Service', never 'Alpha Core'
		for _, name := range projectNames {
			if name == "Alpha Core" {
				t.Fatalf("CRITICAL SECURITY VIOLATION: Tenant B was able to read Tenant A's project!")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("error executing in tenant B context: %v", err)
	}
}

func TestMonthlyPartitionRouting(t *testing.T) {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, database.DefaultConfig(cfg.DatabaseURL))
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	defer pool.Close()

	// Seed tenant, project, repo, commit, scan, finding for foreign key constraints
	tenantID := uuid.New()
	projectID := uuid.New()
	repoID := uuid.New()
	commitID := uuid.New()
	scanID := uuid.New()
	findingID := uuid.New()

	_, err = pool.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, plan, status)
		VALUES ($1, $2, 'Partition Test Org', 'ENTERPRISE', 'ACTIVE')
		ON CONFLICT (slug) DO NOTHING;
	`, tenantID, "part-test-"+tenantID.String()[:8])
	if err != nil {
		t.Fatalf("failed to insert tenant: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO projects (id, tenant_id, slug, name)
		VALUES ($1, $2, 'part-proj', 'Partition Project')
		ON CONFLICT (tenant_id, slug) DO NOTHING;
	`, projectID, tenantID)
	if err != nil {
		t.Fatalf("failed to insert project: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO repositories (id, tenant_id, project_id, provider, clone_url)
		VALUES ($1, $2, $3, 'GITHUB', 'https://github.com/codehound/test.git');
	`, repoID, tenantID, projectID)
	if err != nil {
		t.Fatalf("failed to insert repository: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO commits (id, repository_id, sha, committed_at)
		VALUES ($1, $2, 'a1b2c3d4e5f6', '2026-08-15 12:00:00+00');
	`, commitID, repoID)
	if err != nil {
		t.Fatalf("failed to insert commit: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO scans (id, tenant_id, project_id, repository_id, commit_id, workflow_id)
		VALUES ($1, $2, $3, $4, $5, 'wf-part-001');
	`, scanID, tenantID, projectID, repoID, commitID)
	if err != nil {
		t.Fatalf("failed to insert scan: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO findings (id, tenant_id, project_id, canonical_key, category, severity, title, description, primary_file, primary_line, first_seen_scan_id, last_seen_scan_id)
		VALUES ($1, $2, $3, 'PART-FIND-001', 'SECURITY_VULN', 'HIGH', 'Test Finding', 'Partition Test', 'main.go', 1, $4, $4);
	`, findingID, tenantID, projectID, scanID)
	if err != nil {
		t.Fatalf("failed to insert finding: %v", err)
	}

	// 1. Insert finding_occurrences for August 2026 and September 2026
	occAugID := uuid.New()
	occSepID := uuid.New()

	_, err = pool.Exec(ctx, `
		INSERT INTO finding_occurrences (id, finding_id, scan_id, commit_sha, file_path, start_line, end_line, code_snippet_hash, created_at)
		VALUES 
			($1, $2, $3, 'sha-aug', 'app/main.go', 10, 20, 'hash-aug', '2026-08-20 14:00:00+00'),
			($4, $2, $3, 'sha-sep', 'app/main.go', 10, 20, 'hash-sep', '2026-09-10 14:00:00+00');
	`, occAugID, findingID, scanID, occSepID)
	if err != nil {
		t.Fatalf("failed to insert finding occurrences: %v", err)
	}

	// Verify row routed to specific child table finding_occurrences_y2026m08
	var countAug int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM finding_occurrences_y2026m08 WHERE id = $1;`, occAugID).Scan(&countAug)
	if err != nil {
		t.Fatalf("failed to query finding_occurrences_y2026m08 partition: %v", err)
	}
	if countAug != 1 {
		t.Errorf("expected 1 record in finding_occurrences_y2026m08 partition, got %d", countAug)
	}

	// Verify row routed to specific child table finding_occurrences_y2026m09
	var countSep int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM finding_occurrences_y2026m09 WHERE id = $1;`, occSepID).Scan(&countSep)
	if err != nil {
		t.Fatalf("failed to query finding_occurrences_y2026m09 partition: %v", err)
	}
	if countSep != 1 {
		t.Errorf("expected 1 record in finding_occurrences_y2026m09 partition, got %d", countSep)
	}
}

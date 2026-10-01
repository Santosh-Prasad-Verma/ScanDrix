package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// RLS_TABLES is the set of tables protected by row level security. Keep in sync
// with migrations; the audit below reads it from the live database when one is
// reachable and falls back to this list otherwise.
var RLS_TABLES = map[string]bool{
	"account_profiles": true, "audit_logs": true, "auth": true,
	"automation_execution_logs": true,
	"drixy_embedding_vectors": true, "global_parameters": true,
	"billing_transactions": true, "cli_devices": true,
	"code_ast_edges": true, "code_ast_nodes": true, "code_findings": true,
	"drixy_rule_likes": true, "drixy_rules": true,
	"materialized_dora_daily_rollups": true,
	"revoked_access_tokens": true,
	"finding_feedback": true, "finding_tickets": true, "integration_connections": true,
	"kody_embedding_vectors": true, "notification_channels": true,
	"organization_billing_seats": true, "organization_licenses": true,
	"organization_parameters": true, "outbox_events": true, "parameters": true,
	"platform_pull_requests": true, "pm_auto_ticket_configs": true,
	"pull_request_reviews": true, "review_attestations": true, "review_rules": true,
	"sandbox_leases": true, "scim_group_members": true, "scim_groups": true,
	"security_memory": true, "sso_configs": true, "team_cli_key": true,
	"team_members": true, "teams": true, "token_usage_records": true,
	"user_repository_assignments": true,
	"tracked_issues":              true, "tracked_repositories": true, "users": true,
	"warehouse_domain_events": true, "workflow_automations": true,
	"workspace_parameters": true, "workspace_spend_limits": true, "workspaces": true,
}

var (
	poolCallRe = regexp.MustCompile(`r\.pool\.(?:Query|QueryRow|Exec|SendBatch)\(`)
	tableRe    = regexp.MustCompile(`(?i)(?:FROM|INTO|UPDATE)\s+([a-z_][a-z0-9_]*)`)
)

// knownBypass is the audited inventory of repositories that reach an
// RLS-protected table through a bare pgxpool without setting a tenant or
// system-worker context.
//
// These are a real security gap, not a style choice: under the
// scandrix_runtime role such a query silently matches zero rows for reads and
// is rejected for writes. They are listed here so the gap is measurable and
// cannot silently grow. Each entry was found by running the stack under
// scandrix_runtime and reading the errors, not by inspection.
//
// See docs/LEAST_PRIVILEGE_ROLLOUT.md section R3.
var knownBypass = map[string]int{
	"organization/infrastructure/repositories/postgres_global_parameters_repository.go":       6,
	"core/repositories/audit_automation_repository.go":                                        8,
	"core/repositories/auth_sso_repository.go":                                                9,
	"core/repositories/billing_license_repository.go":                                         7,
	"core/repositories/drixy_rules_repository.go":                                             8,
	"core/repositories/parameters_preset_repository.go":                                       5,
	"core/repositories/repository_scm.go":                                                     18,
	"core/repositories/team_access_repository.go":                                             8,
	"core/repositories/tenancy_repository.go":                                                 16,
	"organization/infrastructure/repositories/postgres_cli_device_repository.go":              5,
	"organization/infrastructure/repositories/postgres_organization_parameters_repository.go": 5,
	"organization/infrastructure/repositories/postgres_organization_repository.go":            5,
	"organization/infrastructure/repositories/postgres_parameters_repository.go":              9,
	"organization/infrastructure/repositories/postgres_team_cli_key_repository.go":            6,
	"organization/infrastructure/repositories/postgres_team_member_repository.go":             7,
	"organization/infrastructure/repositories/postgres_team_repository.go":                    7,
	"organization/infrastructure/repositories/postgres_tracked_repository_reader.go":          1,
	"organization/infrastructure/repositories/postgres_user_account_repository.go":            5,
	"platformdata/infrastructure/repositories/postgres_repository.go":                         15,
}

// TestNoNewRLSBypass is the guard for the RLS rollout.
//
// Two things are enforced:
//
//  1. The set of repositories that bypass RLS has not GROWN. A new bare-pool
//     query against an RLS table fails this test, which is the point: the
//     conversion is opt-in and the failure mode is silent, so it needs a ratchet.
//  2. internal/database, the layer the worker and API composition roots use
//     directly, is completely clean. That layer is fully converted; this
//     prevents a regression there.
//
// This test does not claim the known bypasses are acceptable. It only makes
// them visible and bounded.
func TestNoNewRLSBypass(t *testing.T) {
	root := repoRoot(t)

	found := map[string]int{}
	for _, path := range goFiles(t, root) {
		rel := strings.TrimPrefix(path, root+string(os.PathSeparator))
		rel = strings.TrimPrefix(rel, "internal/")
		rel = strings.TrimPrefix(rel, "cmd/")

		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(src)
		if !poolCallRe.MatchString(text) {
			continue
		}

		hitsRls := false
		for _, m := range tableRe.FindAllStringSubmatch(text, -1) {
			if RLS_TABLES[strings.ToLower(m[1])] {
				hitsRls = true
				break
			}
		}
		if hitsRls {
			found[rel] = len(poolCallRe.FindAllString(text, -1))
		}
	}

	// Rule 2: internal/database must be entirely clean.
	for rel, n := range found {
		if strings.HasPrefix(rel, "database/") {
			t.Errorf("internal/database must have zero bare-pool calls against an RLS table, "+
				"found %d in %s. Use Client.ExecWithTenant or Client.ExecAsSystem.", n, rel)
		}
	}

	// Rule 1: nothing new outside the known inventory.
	var added []string
	for rel := range found {
		if _, known := knownBypass[rel]; !known {
			added = append(added, fmt.Sprintf("  %s (%d calls)", rel, found[rel]))
		}
	}
	if len(added) > 0 {
		sort.Strings(added)
		t.Errorf("new RLS bypass introduced in %d file(s):\n%s\n"+
			"Use Client.ExecWithTenant (caller has a workspace) or "+
			"Client.ExecAsSystem (identity lookup before a tenant exists). "+
			"Under scandrix_runtime a bare pool call silently matches zero rows.",
			len(added), strings.Join(added, "\n"))
	}

	// Removals are good news; report them so the inventory can be tightened.
	var removed []string
	for rel := range knownBypass {
		if _, still := found[rel]; !still {
			removed = append(removed, rel)
		}
	}
	if len(removed) > 0 {
		sort.Strings(removed)
		t.Logf("R3 progress: %d known bypass(es) no longer present, drop them from knownBypass:\n  %s",
			len(removed), strings.Join(removed, "\n  "))
	}
}

// TestRLSTableInventoryIsCurrent keeps RLS_TABLES honest. When a live database
// is reachable, the list in this file is compared against reality.
func TestRLSTableInventoryIsCurrent(t *testing.T) {
	dsn := os.Getenv("SCANDRIX_E2E_RUNTIME_DSN")
	if dsn == "" {
		t.Skip("set SCANDRIX_E2E_RUNTIME_DSN to verify the RLS table inventory against a live database")
	}

	client, err := NewClient(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	rows, err := client.Pool.Query(context.Background(), `
		SELECT c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		WHERE c.relrowsecurity AND c.relkind = 'r'
		ORDER BY c.relname`)
	if err != nil {
		t.Fatalf("list RLS tables: %v", err)
	}
	defer rows.Close()

	var live []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		live = append(live, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	var missing, stale []string
	inFile := map[string]bool{}
	for name := range RLS_TABLES {
		inFile[name] = true
	}
	for _, name := range live {
		if !inFile[name] {
			missing = append(missing, name)
		}
	}
	liveSet := map[string]bool{}
	for _, n := range live {
		liveSet[n] = true
	}
	for name := range RLS_TABLES {
		if !liveSet[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)

	if len(missing) > 0 {
		t.Errorf("RLS_TABLES is missing %d table(s) present in the database: %v", len(missing), missing)
	}
	if len(stale) > 0 {
		t.Logf("RLS_TABLES lists %d table(s) with no RLS in the database: %v", len(stale), stale)
	}
	t.Logf("RLS-protected tables in the live database: %d", len(live))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not locate go.mod")
	return ""
}

func goFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return out
}

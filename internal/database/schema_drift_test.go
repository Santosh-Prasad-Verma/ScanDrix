package database

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// This test exists because a column-name typo shipped in a live query
// (finding_feedback."findingID" against a finding_id column) and the audit
// spent its effort on a dead file instead. A query that names a column the
// table does not have fails at runtime and nowhere else, so the drift is
// cheapest to catch by comparing the SQL in the Go source against the schema
// that actually exists.
//
// AUDIT_REMEDIATION.md F-34, plus the live finding_id bug that motivated it.

var (
	// Matches a bare table reference in a query string, capturing the
	// optionally-quoted identifier immediately after FROM / INTO / UPDATE.
	sqlTableRefRe = regexp.MustCompile(`(?i)\b(?:FROM|INTO|UPDATE)\s+"?([a-z_][a-z0-9_]*)"?`)

	// A CTE alias ("WITH daily_stats AS (", ", day_reviews AS (") is
	// indistinguishable from a table reference by pattern alone, so the
	// aliases defined in a literal are collected and subtracted.
	cteAliasRe = regexp.MustCompile(`(?i)(?:\bWITH\b|,)\s*"?([a-z_][a-z0-9_]*)"?\s+AS\s*\(`)

	// Words that legitimately follow FROM/UPDATE without naming a table.
	nonTableWords = map[string]bool{
		"set": true, "skip": true, "only": true, "lateral": true,
		"the": true, "a": true, "an": true, "nothing": true,
		"everything": true, "reading": true, "detected": true,
		"open_with_unresolved": true, "all": true, "each": true,
	}

	// Matches a column list in an INSERT: INSERT INTO t (a, b, c) VALUES (...).
	sqlInsertColsRe = regexp.MustCompile(`(?is)INSERT\s+INTO\s+"?([a-z_][a-z0-9_]*)"?\s*\(([^)]*)\)`)
)

// catalogTables are queried deliberately and are not application tables.
var catalogTables = map[string]bool{
	"information_schema": true,
	"pg_class":           true,
	"pg_roles":           true,
	"pg_namespace":       true,
	"pg_attribute":       true,
	"pg_policies":        true,
	"pg_extension":       true,
	"pg_indexes":         true,
	"pg_tables":          true,
	"pg_type":            true,
	"pg_authid":          true,
	"pg_constraint":      true,
}

// connectLiveDB opens a connection for schema-drift checks, skipping when no
// runtime DSN is available. Same opt-in contract as the RLS inventory test.
func connectLiveDB(t *testing.T) *Client {
	t.Helper()
	dsn := os.Getenv("SCANDRIX_E2E_RUNTIME_DSN")
	if dsn == "" {
		t.Skip("set SCANDRIX_E2E_RUNTIME_DSN to verify SQL against a live database")
	}
	client, err := NewClient(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(client.Close)
	return client
}

// loadSchema returns the set of real tables and, per table, its column names.
func loadSchema(t *testing.T, client *Client) (map[string]bool, map[string]map[string]bool) {
	t.Helper()
	ctx := context.Background()

	tables := map[string]bool{}
	rows, err := client.Pool.Query(ctx, `
		SELECT c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		WHERE c.relkind IN ('r', 'p')`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatalf("scan table: %v", err)
		}
		tables[name] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("tables rows: %v", err)
	}

	columns := map[string]map[string]bool{}
	rows, err = client.Pool.Query(ctx, `
		SELECT table_name, column_name
		FROM information_schema.columns
		WHERE table_schema = 'public'`)
	if err != nil {
		t.Fatalf("list columns: %v", err)
	}
	for rows.Next() {
		var tbl, col string
		if err := rows.Scan(&tbl, &col); err != nil {
			rows.Close()
			t.Fatalf("scan column: %v", err)
		}
		if columns[tbl] == nil {
			columns[tbl] = map[string]bool{}
		}
		columns[tbl][col] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("columns rows: %v", err)
	}
	return tables, columns
}

// goSQLFiles lists the Go source files holding live SQL, excluding tests and the
// dead internal/core/repositories tree (F-35/F-36: those files name tables and
// columns that have never existed and are never instantiated).
func goSQLFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.Contains(path, "internal/core/repositories") {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return out
}

func readSource(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// rawSQLLiterals extracts the contents of Go raw string literals (backticks).
// The SQL in this codebase lives in raw strings; scanning whole files also
// matches English prose in comments ("FROM a", "INTO everything"), which
// produced dozens of false positives before this filter existed.
func rawSQLLiterals(src string) []string {
	var out []string
	var cur strings.Builder
	inRaw := false
	for _, r := range src {
		switch {
		case r == '`':
			if inRaw {
				out = append(out, cur.String())
				cur.Reset()
			}
			inRaw = !inRaw
		case inRaw:
			cur.WriteRune(r)
		}
	}
	return out
}

// TestSQLReferencesExistingTables asserts that every table named by an
// INSERT/FROM/UPDATE in live SQL actually exists. CTE aliases are filtered out
// by requiring the name to be absent from the schema AND absent from a
// WITH-clause list; in practice the CTE names are lowercase and short, so this
// test reports them as informational rather than failing.
func TestSQLReferencesExistingTables(t *testing.T) {
	client := connectLiveDB(t)
	tables, _ := loadSchema(t, client)

	missing := map[string][]string{}
	for _, f := range goSQLFiles(t) {
		for _, lit := range rawSQLLiterals(readSource(t, f)) {
			ctes := map[string]bool{}
			for _, m := range cteAliasRe.FindAllStringSubmatch(lit, -1) {
				ctes[strings.ToLower(m[1])] = true
			}
			for _, m := range sqlTableRefRe.FindAllStringSubmatch(lit, -1) {
				name := strings.ToLower(m[1])
				if catalogTables[name] || tables[name] || ctes[name] || nonTableWords[name] {
					continue
				}
				missing[name] = append(missing[name], f)
			}
		}
	}
	if len(missing) == 0 {
		return
	}
	names := make([]string, 0, len(missing))
	for n := range missing {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		files := missing[n]
		sort.Strings(files)
		t.Errorf("SQL references table %q which does not exist in the live schema (first seen in %s)",
			n, files[0])
	}
}

// TestSQLInsertColumnsExist is the regression test for the live
// finding_feedback."findingID" bug: an INSERT naming a column the table does
// not have. This is the check that would have caught it before release.
func TestSQLInsertColumnsExist(t *testing.T) {
	client := connectLiveDB(t)
	tables, columns := loadSchema(t, client)

	var problems []string
	for _, f := range goSQLFiles(t) {
		for _, lit := range rawSQLLiterals(readSource(t, f)) {
			for _, m := range sqlInsertColsRe.FindAllStringSubmatch(lit, -1) {
				tbl := strings.ToLower(m[1])
				if !tables[tbl] {
					continue // reported by TestSQLReferencesExistingTables
				}
				known := columns[tbl]
				for _, raw := range strings.Split(m[2], ",") {
					col := strings.TrimSpace(raw)
					col = strings.Trim(col, `"`)
					if col == "" {
						continue
					}
					// Case is folded because Postgres folds unquoted identifiers.
					if !known[strings.ToLower(col)] && !known[col] {
						problems = append(problems,
							"INSERT INTO "+tbl+" names column \""+col+"\" which does not exist ("+f+")")
					}
				}
			}
		}
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Error(p)
	}
}

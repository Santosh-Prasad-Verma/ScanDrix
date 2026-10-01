package config

import "testing"

// TestRejectSuperuserDSN pins AUDIT_REMEDIATION.md F-46.
//
// A missing DATABASE_URL used to fall back to
// postgres://postgres:postgres@127.0.0.1:5432/scandrix, a superuser with a
// trivial password. Superusers bypass row-level security, so that fallback
// silently granted unrestricted access to every tenant while the surrounding
// code correctly refused to guess its other secrets.
func TestRejectSuperuserDSN(t *testing.T) {
	rejected := []string{
		"postgres://postgres:postgres@127.0.0.1:5432/scandrix?sslmode=disable",
		"postgres://postgres@db:5432/scandrix",
		"postgresql://POSTGRES:pw@db:5432/scandrix",
		"postgres://root:pw@db:5432/scandrix",
		"postgres://admin:pw@db:5432/scandrix",
		"postgres://superuser:pw@db:5432/scandrix",
		// scandrix_app is a superuser with BYPASSRLS in this project's schema.
		"postgres://scandrix_app:pw@db:5432/scandrix",
	}
	for _, dsn := range rejected {
		if err := rejectSuperuserDSN(dsn); err == nil {
			t.Errorf("expected superuser DSN to be rejected: %s", dsn)
		}
	}

	accepted := []string{
		"postgres://scandrix_runtime:pw@postgres:5432/scandrix?sslmode=disable",
		"postgres://app_user:pw@db:5432/scandrix",
		"postgres://scandrix_app_runtime:pw@db:5432/scandrix",
		// No userinfo at all: trust the environment (IAM, .pgpass, etc).
		"postgres://db:5432/scandrix",
		// Unparseable: the driver reports the real error, so this must not
		// invent one here.
		"://::bad::",
	}
	for _, dsn := range accepted {
		if err := rejectSuperuserDSN(dsn); err != nil {
			t.Errorf("expected DSN to be accepted, got %v: %s", err, dsn)
		}
	}
}

// TestLoadRequiresDatabaseURL proves the fallback is gone rather than merely
// guarded: an empty DATABASE_URL must be an error, not a default.
func TestLoadRequiresDatabaseURL(t *testing.T) {
	for _, k := range []string{
		"DATABASE_URL", "API_MCP_MANAGER_ENCRYPTION_SECRET", "MCP_MANAGER_SECRET",
		"KMS_MASTER_KEY", "API_MCP_MANAGER_JWT_SECRET", "JWT_SECRET",
	} {
		t.Setenv(k, "")
	}
	if _, err := Load(); err == nil {
		t.Fatal("Load must fail when DATABASE_URL is unset; it must not invent one")
	}
}

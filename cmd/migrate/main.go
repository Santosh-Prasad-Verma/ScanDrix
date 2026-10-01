package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	// Migrations need privileges the runtime role deliberately lacks: CREATE
	// EXTENSION for uuid-ossp / pgcrypto / vector is superuser-only. So the
	// migration job reads its own DSN, and the services keep only the
	// least-privilege DATABASE_URL. This is what stops the privileged credential
	// from being present in every API and worker container.
	dbURL := firstNonEmptyEnv(
		"MIGRATION_DATABASE_URL",
		"DIRECT_URL",
		"SUPABASE_DATABASE_URL",
		"SUPABASE_POOLER_URL",
		"DATABASE_URL",
	)
	if dbURL == "" {
		log.Fatal("MIGRATION_DATABASE_URL (or DATABASE_URL) environment variable is required")
	}
	if os.Getenv("MIGRATION_DATABASE_URL") == "" {
		log.Println("MIGRATION_DATABASE_URL is not set; falling back to a service DSN. " +
			"Migrations may fail if that role cannot create extensions.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Println("Connecting to PostgreSQL database...")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to create connection pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Failed connecting to database: %v", err)
	}
	fmt.Println("Connected successfully!")

	// 1. Enable required extensions
	extensionsSQL := `
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "vector";
`
	fmt.Println("Enabling required PostgreSQL extensions (uuid-ossp, pgcrypto, vector)...")
	if _, err := pool.Exec(ctx, extensionsSQL); err != nil {
		log.Printf("Warning during extension initialization: %v", err)
	} else {
		fmt.Println("Extensions verified successfully.")
	}

	// 2. Discover and run migrations in order
	migrationsDir := "migrations"
	if len(os.Args) > 1 {
		migrationsDir = os.Args[1]
	}

	// Ensure schema_migrations tracker exists
	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		log.Fatalf("Failed ensuring schema_migrations table: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		log.Fatalf("Failed listing migrations: %v", err)
	}
	sort.Strings(files)

	for _, file := range files {
		baseName := filepath.Base(file)

		// Check if already applied
		var alreadyApplied bool
		err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", baseName).Scan(&alreadyApplied)
		if err == nil && alreadyApplied {
			fmt.Printf("Skipping already applied migration: %s\n", baseName)
			continue
		}

		fmt.Printf("Applying migration atomically in transaction: %s\n", baseName)
		content, err := os.ReadFile(file)
		if err != nil {
			log.Fatalf("Failed reading migration %s: %v", file, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			log.Fatalf("Failed starting transaction for %s: %v", baseName, err)
		}

		if _, err := tx.Exec(ctx, string(content)); err != nil {
			_ = tx.Rollback(ctx)
			log.Fatalf("Migration failed on %s (transaction rolled back): %v", baseName, err)
		}

		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING", baseName); err != nil {
			_ = tx.Rollback(ctx)
			log.Fatalf("Failed recording migration %s (transaction rolled back): %v", baseName, err)
		}

		if err := tx.Commit(ctx); err != nil {
			log.Fatalf("Failed committing migration transaction for %s: %v", baseName, err)
		}
		fmt.Printf("Successfully applied %s\n", baseName)
	}

	fmt.Println("\nAll database migrations applied successfully!")

	// 3. Print verified table list
	rows, err := pool.Query(ctx, "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name")
	if err == nil {
		defer rows.Close()
		fmt.Println("\nVerified Public Tables in Database:")
		for rows.Next() {
			var tableName string
			_ = rows.Scan(&tableName)
			fmt.Printf("  ✔ %s\n", tableName)
		}
	}
}

// firstNonEmptyEnv returns the value of the first variable that is set and
// non-blank. Order matters: the most privileged DSN must be considered first.
func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	dbURL := os.Getenv("DIRECT_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = os.Getenv("SUPABASE_DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = os.Getenv("SUPABASE_POOLER_URL")
	}
	if dbURL == "" {
		log.Fatal("DATABASE_URL or SUPABASE_DATABASE_URL environment variable is required")
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

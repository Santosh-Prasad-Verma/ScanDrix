package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/codehound/codehound/core/pkg/config"
	"github.com/codehound/codehound/core/pkg/database"
)

func main() {
	appCfg := config.Load()
	dbURL := appCfg.DatabaseURL

	flag.Parse()
	command := "up"
	if flag.NArg() > 0 {
		command = flag.Arg(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := database.DefaultConfig(dbURL)
	pool, err := database.NewPool(ctx, cfg)
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}
	defer pool.Close()

	switch command {
	case "up":
		fmt.Println("🚀 Running database migrations (UP)...")
		if err := database.MigrateUp(ctx, pool); err != nil {
			log.Fatalf("❌ Migration up failed: %v", err)
		}
		ver, _ := database.GetCurrentVersion(ctx, pool)
		fmt.Printf("✅ Migration completed successfully! Current schema version: %d\n", ver)

	case "down":
		fmt.Println("⚠️  Running database migrations (DOWN/ROLLBACK)...")
		if err := database.MigrateDown(ctx, pool); err != nil {
			log.Fatalf("❌ Migration down failed: %v", err)
		}
		ver, _ := database.GetCurrentVersion(ctx, pool)
		fmt.Printf("✅ Rollback completed successfully! Current schema version: %d\n", ver)

	case "status":
		ver, err := database.GetCurrentVersion(ctx, pool)
		if err != nil {
			log.Fatalf("❌ Failed to check migration status: %v", err)
		}
		fmt.Printf("ℹ️  Current database schema version: %d (Latest available: %d)\n", ver, database.CurrentSchemaVersion)

	case "reset":
		fmt.Println("🔄 Resetting database schema (DOWN -> UP)...")
		_ = database.MigrateDown(ctx, pool)
		if err := database.MigrateUp(ctx, pool); err != nil {
			log.Fatalf("❌ Reset failed during up migration: %v", err)
		}
		ver, _ := database.GetCurrentVersion(ctx, pool)
		fmt.Printf("✅ Database reset successfully! Current schema version: %d\n", ver)

	default:
		fmt.Printf("Unknown command '%s'. Valid commands: up, down, status, reset\n", command)
		os.Exit(1)
	}
}

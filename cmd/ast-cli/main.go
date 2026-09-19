// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package main

import (
	"context"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/config"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

func main() {
	var (
		workspaceID string
		repoID      string
		filePath    string
		diffPath    string
		limit       int
		dryRun      bool
		force       bool
	)

	flag.StringVar(&workspaceID, "org", "", "Restrict backfill to organization/workspace UUID")
	flag.StringVar(&repoID, "repo", "", "Target repository UUID")
	flag.StringVar(&filePath, "file", "", "Inspect AST tokens and syntax of local file")
	flag.StringVar(&diffPath, "diff", "", "Inspect diff AST hunks")
	flag.IntVar(&limit, "limit", 10, "Cap jobs enqueued per batch")
	flag.BoolVar(&dryRun, "dry-run", false, "Report what would happen without writing to database")
	flag.BoolVar(&force, "force", false, "Force rebuild of existing AST graphs")
	flag.Parse()

	// 1. If local file is supplied, inspect AST directly
	if filePath != "" {
		src, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}

		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
		if err != nil {
			fmt.Fprintf(os.Stderr, "AST parse error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("🌳 AST Structure Inspection: %s (Package: %s)\n", filePath, node.Name.Name)
		fmt.Printf("   Imports: %d | Declarations: %d | Comments: %d\n", len(node.Imports), len(node.Decls), len(node.Comments))
		for _, imp := range node.Imports {
			fmt.Printf("   • import %s\n", imp.Path.Value)
		}
		return
	}

	// 2. If local diff path is supplied, analyze diff hunks directly
	if diffPath != "" {
		bytes, err := os.ReadFile(diffPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading diff: %v\n", err)
			os.Exit(1)
		}

		patches, err := diff.ParseUnifiedDiff(strings.NewReader(string(bytes)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing diff: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("🌳 AST Diff Inspection: Parsed %d file patches\n", len(patches))
		for _, p := range patches {
			fmt.Printf("  File: %s (+%d -%d) | Hunks: %d\n", p.NewPath, p.Additions, p.Deletions, len(p.Hunks))
			for i, h := range p.Hunks {
				fmt.Printf("    Hunk #%d: Line %d-%d (Context: %q)\n", i+1, h.NewStart, h.NewStart+h.NewLines, h.Header)
			}
		}
		return
	}

	// 3. Database backfill daemon mode
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	dbClient, err := database.NewClient(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Database connection failed: %v\n", err)
		os.Exit(1)
	}
	defer dbClient.Close()

	repo := database.NewRepository(dbClient)

	fmt.Println("🌳 ScanDrix AST Graph Backfill Daemon")
	fmt.Printf("   Dry-Run: %v | Force: %v | Limit: %d\n", dryRun, force, limit)

	var targetWS uuid.UUID
	if workspaceID == "" {
		workspaceID = os.Getenv("SCANDRIX_WORKSPACE_ID")
	}
	if workspaceID != "" {
		parsedWS, err := uuid.Parse(workspaceID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid workspace UUID: %v\n", err)
			os.Exit(1)
		}
		targetWS = parsedWS
	} else {
		targetWS = uuid.New()
	}

	if dryRun {
		fmt.Printf("[DRY-RUN] Would enqueue AST graph build jobs for workspace %s (up to %d repos)\n", targetWS, limit)
		return
	}

	// Enqueue AST graph generation task into transactional outbox
	outboxEvent := &models.OutboxRecord{
		ID:          uuid.New(),
		WorkspaceID: targetWS,
		EventType:   "ast.graph.build",
		Payload:     fmt.Appendf(nil, `{"workspace_id":"%s","force":%v,"limit":%d}`, targetWS, force, limit),
		Status:      models.OutboxPending,
		CreatedAt:   time.Now().UTC(),
	}

	if err := repo.InsertOutboxEvent(ctx, outboxEvent); err != nil {
		fmt.Fprintf(os.Stderr, "Failed inserting outbox AST event: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Enqueued AST graph backfill job (Event ID: %s)\n", outboxEvent.ID)
}

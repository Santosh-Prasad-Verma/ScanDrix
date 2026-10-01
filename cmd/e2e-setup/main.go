// Command e2e-setup stages a workspace for a least-privilege verification run.
//
// Local development tool. It is not part of any service image and no Dockerfile
// references it.
//
// It exists because re-verifying the RLS rollout should not require rebuilding the
// staging by hand: the token must be sealed with the same KMS envelope the
// product uses, and the tracked-repository row has to exist before the worker
// can resolve a workspace from a repo namespace. Both are done here through the
// real repository methods, so setup exercises the same code path the API does.
//
// Usage
//
//	export DATABASE_URL='postgres://scandrix_app:...@localhost:5432/scandrix?sslmode=disable'
//	export SCM_TEST_TOKEN='github_pat_...'
//	export SCM_TEST_REPO='owner/repo'        # optional, defaults below
//	export SCM_TEST_BRANCH='main'            # optional
//	go run ./cmd/e2e-setup
//
// The token is read from the environment and never printed. It is written to the
// database only through Repository.UpsertIntegrationConnectionWithSecret, which
// encrypts it (internal/security/kms, NIST SP 800-57 envelope encryption).
//
// The workspace it creates is disposable. Remove it when the run is done:
//
//	DELETE FROM workspaces WHERE slug = 'scandrix-e2e';
//
// which cascades to the tracked repository, integration connection, reviews and
// findings.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

const (
	defaultRepo   = "Droid-33/scandrix-e2e-public"
	defaultBranch = "main"
	workspaceSlug = "scandrix-e2e"
	workspaceName = "ScanDrix E2E Verification"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "e2e-setup failed:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	dbURL := env("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL is required (use the migration/owner DSN, not the runtime role)")
	}
	token := env("SCM_TEST_TOKEN")
	if token == "" {
		return fmt.Errorf("SCM_TEST_TOKEN is required; read it from the environment, never paste it into a command")
	}
	namespace := envOr("SCM_TEST_REPO", defaultRepo)
	branch := envOr("SCM_TEST_BRANCH", defaultBranch)

	client, err := database.NewClient(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Close()
	repo := database.NewRepository(client)

	// Workspace creation runs as a system bootstrap: the row does not exist yet,
	// so tenant RLS cannot admit it. Repository.CreateWorkspace documents that
	// authorization is the caller's responsibility.
	wsID := uuid.New()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID:     wsID,
		Slug:   workspaceSlug,
		Name:   workspaceName,
		Status: "ACTIVE",
	}); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	fmt.Printf("workspace      %s\n", wsID)

	// external_id must be stable: the unique constraint is
	// (workspace_id, provider, external_id).
	if _, err := repo.TrackRepository(ctx, wsID, models.SCMProvider("github"), namespace, namespace, branch); err != nil {
		return fmt.Errorf("track repository: %w", err)
	}
	fmt.Printf("tracked repo   %s @ %s\n", namespace, branch)

	owner, _, _ := strings.Cut(namespace, "/")

	// No webhook secret: this verification enqueues the review job directly rather
	// than arriving through a real webhook, so signature verification has nothing
	// to check here.
	if err := repo.UpsertIntegrationConnectionWithSecret(
		ctx, wsID, models.SCMProvider("github"), owner, token, "", true, 1,
	); err != nil {
		return fmt.Errorf("connect integration: %w", err)
	}
	fmt.Printf("integration    github as %s (token encrypted at rest)\n", owner)

	fmt.Println()
	fmt.Println("ready. enqueue a ReviewTaskPayload with:")
	fmt.Printf("  workspace_id   = %s\n", wsID)
	fmt.Printf("  repo_namespace = %s\n", namespace)
	fmt.Printf("  provider       = github\n")
	fmt.Println()
	fmt.Println("Then verify with:")
	fmt.Println("  export SCANDRIX_E2E_RUNTIME_DSN=<runtime-role DSN>")
	fmt.Println("  go test ./internal/database/ -run 'TestBatchInsertFindings|TestListWorkspaces|TestNoNewRLSBypass' -v")
	return nil
}

func env(key string) string { return strings.TrimSpace(os.Getenv(key)) }

func envOr(key, fallback string) string {
	if v := env(key); v != "" {
		return v
	}
	return fallback
}

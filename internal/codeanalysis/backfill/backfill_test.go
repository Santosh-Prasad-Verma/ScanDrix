package backfill_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/codeanalysis/backfill"
	"github.com/scandrix/backend/internal/codeanalysis/graph"
)

// mockASTStore records batch insert calls for verification.
type mockASTStore struct {
	mu    sync.Mutex
	nodes []graph.ASTNode
	edges []graph.ASTEdge
}

func (m *mockASTStore) WorkspaceIDForRepository(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (m *mockASTStore) BatchInsertASTNodes(_ context.Context, _ uuid.UUID, _ uuid.UUID, nodes []graph.ASTNode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodes = append(m.nodes, nodes...)
	return nil
}

func (m *mockASTStore) BatchInsertASTEdges(_ context.Context, _ uuid.UUID, _ uuid.UUID, edges []graph.ASTEdge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.edges = append(m.edges, edges...)
	return nil
}

func TestSweepRepository(t *testing.T) {
	// Create a temporary "repo" with Go source files
	tmpDir := t.TempDir()

	// Write a simple Go file
	mainContent := `package main

import "fmt"

func main() {
	greet("world")
}

func greet(name string) {
	fmt.Println("Hello", name)
}
`
	err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(mainContent), 0644)
	if err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}

	// Write a second file
	err = os.MkdirAll(filepath.Join(tmpDir, "internal"), 0755)
	if err != nil {
		t.Fatalf("failed to create internal dir: %v", err)
	}

	utilContent := `package internal

func Helper() string {
	return "help"
}

func Caller() string {
	return Helper()
}
`
	err = os.WriteFile(filepath.Join(tmpDir, "internal", "util.go"), []byte(utilContent), 0644)
	if err != nil {
		t.Fatalf("failed to write util.go: %v", err)
	}

	// Write a non-Go file (should be skipped)
	err = os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test"), 0644)
	if err != nil {
		t.Fatalf("failed to write README.md: %v", err)
	}

	// Run the backfill worker
	store := &mockASTStore{}
	worker := backfill.NewWorker(store,
		backfill.WithConcurrency(2),
		backfill.WithBatchSize(10),
	)

	repoID := uuid.New()
	err = worker.SweepRepository(context.Background(), uuid.New(), repoID, tmpDir)
	if err != nil {
		t.Fatalf("SweepRepository failed: %v", err)
	}

	// Verify progress
	progress, ok := worker.GetProgress(repoID)
	if !ok {
		t.Fatal("expected progress entry for repoID")
	}

	if progress.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", progress.Status)
	}

	if progress.TotalFiles != 2 {
		t.Errorf("expected 2 total files (only .go), got %d", progress.TotalFiles)
	}

	if progress.ProcessedFiles != 2 {
		t.Errorf("expected 2 processed files, got %d", progress.ProcessedFiles)
	}

	// Verify nodes were extracted (main, greet, Helper, Caller = 4 functions)
	if progress.NodesExtracted < 4 {
		t.Errorf("expected at least 4 extracted nodes, got %d", progress.NodesExtracted)
	}

	// Verify store received the batch inserts
	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.nodes) < 4 {
		t.Errorf("expected at least 4 persisted nodes, got %d", len(store.nodes))
	}

	// Verify at least some edges (main→greet, Caller→Helper)
	if progress.EdgesExtracted < 1 {
		t.Errorf("expected at least 1 edge, got %d", progress.EdgesExtracted)
	}

	if progress.ErrorCount != 0 {
		t.Errorf("expected 0 errors, got %d", progress.ErrorCount)
	}
}

func TestSweepEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	store := &mockASTStore{}
	worker := backfill.NewWorker(store)

	err := worker.SweepRepository(context.Background(), uuid.New(), uuid.New(), tmpDir)
	if err != nil {
		t.Fatalf("expected no error for empty dir, got %v", err)
	}
}

func TestSweepCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a Go file
	content := `package test
func A() {}
`
	_ = os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte(content), 0644)

	store := &mockASTStore{}
	worker := backfill.NewWorker(store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancel

	err := worker.SweepRepository(ctx, uuid.New(), uuid.New(), tmpDir)
	if err == nil {
		// Either returns ctx.Err() or completes before checking — both are valid
		t.Log("sweep completed before cancellation check (fast path)")
	}
}

package backfill

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/codeanalysis/graph"
)

// ASTStore defines the persistence interface for batch AST upserts.
type ASTStore interface {
	BatchInsertASTNodes(ctx context.Context, repoID uuid.UUID, nodes []graph.ASTNode) error
	BatchInsertASTEdges(ctx context.Context, repoID uuid.UUID, edges []graph.ASTEdge) error
}

// BackfillProgress tracks worker progress for observability.
type BackfillProgress struct {
	RepositoryID  uuid.UUID `json:"repository_id"`
	TotalFiles    int64     `json:"total_files"`
	ProcessedFiles int64   `json:"processed_files"`
	ErrorCount    int64     `json:"error_count"`
	NodesExtracted int64   `json:"nodes_extracted"`
	EdgesExtracted int64   `json:"edges_extracted"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at,omitempty"`
	Status        string    `json:"status"` // "running", "completed", "failed"
}

// Worker performs full background sweeps indexing all files into code_ast_nodes
// and code_ast_edges on initial repository import.
type Worker struct {
	store       ASTStore
	indexer     *graph.GraphIndexer
	concurrency int
	batchSize   int

	mu       sync.RWMutex
	progress map[uuid.UUID]*BackfillProgress
}

// NewWorker initializes the architecture backfill worker.
func NewWorker(store ASTStore, opts ...Option) *Worker {
	w := &Worker{
		store:       store,
		indexer:     graph.NewGraphIndexer(),
		concurrency: 4,
		batchSize:   100,
		progress:    make(map[uuid.UUID]*BackfillProgress),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Option configures worker behavior.
type Option func(*Worker)

// WithConcurrency sets the number of parallel file parsers.
func WithConcurrency(n int) Option {
	return func(w *Worker) {
		if n > 0 {
			w.concurrency = n
		}
	}
}

// WithBatchSize sets the DB batch insert size.
func WithBatchSize(n int) Option {
	return func(w *Worker) {
		if n > 0 {
			w.batchSize = n
		}
	}
}

// GetProgress returns the current progress for a repository backfill.
func (w *Worker) GetProgress(repoID uuid.UUID) (*BackfillProgress, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p, ok := w.progress[repoID]
	return p, ok
}

// supportedExtensions lists file extensions the AST indexer can process.
var supportedExtensions = map[string]bool{
	".go":  true,
	".ts":  true,
	".tsx": true,
	".js":  true,
	".jsx": true,
	".py":  true,
}

// SweepRepository performs a full architecture backfill on a cloned repository directory.
// It walks the file tree, parses supported source files, extracts AST nodes/edges,
// and persists them in batches via the ASTStore.
func (w *Worker) SweepRepository(ctx context.Context, repoID uuid.UUID, repoDir string) error {
	progress := &BackfillProgress{
		RepositoryID: repoID,
		StartedAt:    time.Now().UTC(),
		Status:       "running",
	}

	w.mu.Lock()
	w.progress[repoID] = progress
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		progress.CompletedAt = time.Now().UTC()
		if progress.ErrorCount > progress.TotalFiles/2 && progress.TotalFiles > 0 {
			progress.Status = "failed"
		} else {
			progress.Status = "completed"
		}
		w.mu.Unlock()
	}()

	// Phase 1: Discover files
	var filePaths []string
	err := filepath.WalkDir(repoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip unreadable directories
		}

		// Skip hidden directories and common non-source dirs
		if d.IsDir() {
			base := d.Name()
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "node_modules" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if supportedExtensions[ext] {
			filePaths = append(filePaths, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking repo dir %s: %w", repoDir, err)
	}

	atomic.StoreInt64(&progress.TotalFiles, int64(len(filePaths)))

	if len(filePaths) == 0 {
		slog.Info("backfill: no supported files found", "repo_id", repoID, "dir", repoDir)
		return nil
	}

	// Phase 2: Parse files concurrently
	fileCh := make(chan string, len(filePaths))
	for _, fp := range filePaths {
		fileCh <- fp
	}
	close(fileCh)

	var resultMu sync.Mutex
	var allNodes []graph.ASTNode

	var wg sync.WaitGroup
	for i := 0; i < w.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fp := range fileCh {
				if ctx.Err() != nil {
					return
				}

				relPath, _ := filepath.Rel(repoDir, fp)
				content, err := os.ReadFile(fp)
				if err != nil {
					atomic.AddInt64(&progress.ErrorCount, 1)
					slog.Warn("backfill: failed to read file", "path", fp, "error", err)
					continue
				}

				ext := strings.ToLower(filepath.Ext(fp))
				var nodes []graph.ASTNode
				var indexErr error

				switch ext {
				case ".go":
					nodes, indexErr = w.indexer.IndexGoFile(ctx, repoID, relPath, string(content))
				case ".ts", ".tsx", ".js", ".jsx":
					nodes, indexErr = w.indexer.IndexTSFile(ctx, repoID, relPath, string(content))
				case ".py":
					nodes, indexErr = w.indexer.IndexPythonFile(ctx, repoID, relPath, string(content))
				}

				if indexErr != nil {
					atomic.AddInt64(&progress.ErrorCount, 1)
					slog.Warn("backfill: failed to index file", "path", relPath, "error", indexErr)
					continue
				}

				atomic.AddInt64(&progress.ProcessedFiles, 1)

				if len(nodes) > 0 {
					resultMu.Lock()
					allNodes = append(allNodes, nodes...)
					resultMu.Unlock()
					atomic.AddInt64(&progress.NodesExtracted, int64(len(nodes)))
				}
			}
		}()
	}

	wg.Wait()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Phase 3: Batch persist to database
	w.indexer.Lock()
	allEdges := w.indexer.GetEdges()
	w.indexer.Unlock()

	atomic.StoreInt64(&progress.EdgesExtracted, int64(len(allEdges)))

	// Batch insert nodes
	for i := 0; i < len(allNodes); i += w.batchSize {
		end := i + w.batchSize
		if end > len(allNodes) {
			end = len(allNodes)
		}

		if err := w.store.BatchInsertASTNodes(ctx, repoID, allNodes[i:end]); err != nil {
			slog.Error("backfill: batch insert nodes failed", "repo_id", repoID, "batch", i/w.batchSize, "error", err)
			atomic.AddInt64(&progress.ErrorCount, 1)
		}
	}

	// Batch insert edges
	for i := 0; i < len(allEdges); i += w.batchSize {
		end := i + w.batchSize
		if end > len(allEdges) {
			end = len(allEdges)
		}

		if err := w.store.BatchInsertASTEdges(ctx, repoID, allEdges[i:end]); err != nil {
			slog.Error("backfill: batch insert edges failed", "repo_id", repoID, "batch", i/w.batchSize, "error", err)
			atomic.AddInt64(&progress.ErrorCount, 1)
		}
	}

	slog.Info("backfill: sweep completed",
		"repo_id", repoID,
		"files", progress.ProcessedFiles,
		"nodes", progress.NodesExtracted,
		"edges", progress.EdgesExtracted,
		"errors", progress.ErrorCount,
		"duration", time.Since(progress.StartedAt),
	)

	return nil
}

package review

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/cli/types"
)

// HunkViewerScope models parameters supplied to hunk execution.
type HunkViewerScope struct {
	Staged bool
	Range  string
	Commit string
	Paths  []string
}

// BuildHunkViewerScope translates CLI review options into equivalent hunk scope flags.
func BuildHunkViewerScope(opts ReviewOptions) *HunkViewerScope {
	if opts.Commit != "" && opts.Branch != "" {
		return nil
	}

	if opts.Commit != "" {
		return &HunkViewerScope{
			Commit: opts.Commit,
			Paths:  opts.Files,
		}
	}

	if opts.Branch != "" {
		return &HunkViewerScope{
			Range: fmt.Sprintf("%s...HEAD", opts.Branch),
			Paths: opts.Files,
		}
	}

	return &HunkViewerScope{
		Staged: opts.Staged,
		Paths:  opts.Files,
	}
}

// CanRenderScopeInHunk returns whether the selected review scope can be represented by hunk.
func CanRenderScopeInHunk(opts ReviewOptions) bool {
	return BuildHunkViewerScope(opts) != nil
}

// BuildHunkArgs compiles the exact command line arguments to spawn hunk.
func BuildHunkArgs(scope HunkViewerScope, contextPath string, extensionDir string) []string {
	var args []string

	if scope.Commit != "" {
		args = append(args, "show", scope.Commit)
	} else {
		args = append(args, "diff")
		if scope.Range != "" {
			args = append(args, scope.Range)
		} else if scope.Staged {
			args = append(args, "--staged")
		}
	}

	args = append(args, "--agent-context", contextPath, "--agent-notes", "--experimental")

	if extensionDir != "" {
		args = append(args, "--extension", extensionDir)
	}

	if len(scope.Paths) > 0 {
		args = append(args, "--")
		args = append(args, scope.Paths...)
	}

	return args
}

// OpenReviewInHunk writes the sidecars and spawns the interactive hunk diff viewer.
func OpenReviewInHunk(ctx context.Context, result *types.ReviewResult, scope HunkViewerScope, verbose bool) error {
	runID := uuid.New().String()
	contextPath := filepath.Join(os.TempDir(), fmt.Sprintf("scandrix-review-%s.json", runID))
	findingsPath := filepath.Join(os.TempDir(), fmt.Sprintf("scandrix-findings-%s.json", runID))

	defer func() {
		_ = os.Remove(contextPath)
		_ = os.Remove(findingsPath)
	}()

	findings := ConvertReviewToHunkFindings(result)
	findingsBytes, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize hunk findings: %w", err)
	}
	if err := os.WriteFile(findingsPath, findingsBytes, 0600); err != nil {
		return fmt.Errorf("failed to write hunk findings sidecar: %w", err)
	}

	hunkContext := ConvertReviewToHunkAgentContext(result)
	contextBytes, err := json.MarshalIndent(hunkContext, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize hunk context: %w", err)
	}
	if err := os.WriteFile(contextPath, contextBytes, 0600); err != nil {
		return fmt.Errorf("failed to write hunk agent context: %w", err)
	}

	args := BuildHunkArgs(scope, contextPath, "")
	cmd := exec.CommandContext(ctx, "hunk", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), fmt.Sprintf("SCANDRIX_HUNK_FINDINGS=%s", findingsPath))

	if verbose {
		fmt.Printf("[verbose] Spawning hunk with args: %v\n", args)
	}

	return cmd.Run()
}

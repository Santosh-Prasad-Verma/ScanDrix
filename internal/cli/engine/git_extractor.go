package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// GitExtractor executes local git operations safely without shell expansion.
type GitExtractor struct {
	workDir string
}

// NewGitExtractor creates a git extractor bound to a working directory.
func NewGitExtractor(workDir string) *GitExtractor {
	return &GitExtractor{workDir: workDir}
}

// ExtractDiff retrieves the unified diff according to CLI options.
func (g *GitExtractor) ExtractDiff(ctx context.Context, opts CLIOptions) (string, error) {
	// 1. Verify working directory is a git repository
	if err := g.checkGitRepo(ctx); err != nil {
		return "", err
	}

	// 2. Formulate git diff arguments
	args := []string{"diff", "--no-color", "--unified=3"}

	if opts.Staged {
		args = append(args, "--cached")
	} else if opts.CommitRange != "" {
		args = append(args, opts.CommitRange)
	} else if opts.Branch != "" {
		args = append(args, opts.Branch+"...HEAD")
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	if g.workDir != "" {
		cmd.Dir = g.workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git diff failed: %s (stderr: %s)", err, stderr.String())
	}

	diffStr := stdout.String()
	if strings.TrimSpace(diffStr) == "" && !opts.Staged && opts.Branch == "" && opts.CommitRange == "" {
		// Try staged diff if unstaged is empty
		cmdStaged := exec.CommandContext(ctx, "git", "diff", "--cached", "--no-color", "--unified=3")
		if g.workDir != "" {
			cmdStaged.Dir = g.workDir
		}
		var stdoutStaged bytes.Buffer
		cmdStaged.Stdout = &stdoutStaged
		if err := cmdStaged.Run(); err == nil && strings.TrimSpace(stdoutStaged.String()) != "" {
			return stdoutStaged.String(), nil
		}
	}

	return diffStr, nil
}

func (g *GitExtractor) checkGitRepo(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	if g.workDir != "" {
		cmd.Dir = g.workDir
	}
	if err := cmd.Run(); err != nil {
		return errors.New("current directory is not a valid git repository")
	}
	return nil
}

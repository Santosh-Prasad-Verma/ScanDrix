package review

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/cli/git"
)

// ResolveReviewDiffParams provides inputs for resolving the review diff.
type ResolveReviewDiffParams struct {
	Files   []string
	Options ReviewOptions
	Verbose bool
	Git     *git.GitService
}

// ResolveReviewDiffResult contains the computed diff text and verbose logging steps.
type ResolveReviewDiffResult struct {
	Diff            string
	VerboseMessages []string
}

// ResolveReviewDiff determines the target git diff according to scope options.
func ResolveReviewDiff(ctx context.Context, params ResolveReviewDiffParams) (*ResolveReviewDiffResult, error) {
	if params.Git == nil {
		return nil, fmt.Errorf("git service is required")
	}

	params.Git.SetVerbose(params.Verbose)
	var verboseMessages []string
	var diff string
	var err error

	if len(params.Files) > 0 {
		if params.Verbose {
			verboseMessages = append(verboseMessages, fmt.Sprintf("[verbose] Getting diff for specific files: %s", strings.Join(params.Files, ", ")))
		}
		diff, err = params.Git.GetDiffForFiles(ctx, params.Files)
	} else if params.Options.Branch != "" {
		if params.Verbose {
			verboseMessages = append(verboseMessages, fmt.Sprintf("[verbose] Getting diff for branch: %s", params.Options.Branch))
		}
		diff, err = params.Git.GetDiffForBranch(ctx, params.Options.Branch)
	} else if params.Options.Commit != "" {
		if params.Verbose {
			verboseMessages = append(verboseMessages, fmt.Sprintf("[verbose] Getting diff for commit: %s", params.Options.Commit))
		}
		diff, err = params.Git.GetDiffForCommit(ctx, params.Options.Commit)
	} else if params.Options.Staged {
		if params.Verbose {
			verboseMessages = append(verboseMessages, "[verbose] Getting staged diff only")
		}
		diff, err = params.Git.GetStagedDiff(ctx)
	} else {
		if params.Verbose {
			verboseMessages = append(verboseMessages, "[verbose] Getting working tree diff (staged + unstaged)")
		}
		diff, err = params.Git.GetWorkingTreeDiff(ctx)
	}

	if err != nil {
		return nil, err
	}

	if params.Verbose {
		charCount := len(diff)
		if charCount > 0 {
			verboseMessages = append(verboseMessages, fmt.Sprintf("[verbose] Diff result: %d characters", charCount))
			previewLen := 500
			if charCount < previewLen {
				previewLen = charCount
			}
			preview := diff[:previewLen]
			trunc := ""
			if charCount > 500 {
				trunc = "\n... (truncated)"
			}
			verboseMessages = append(verboseMessages, fmt.Sprintf("[verbose] Diff preview:\n%s%s", preview, trunc))
		} else {
			verboseMessages = append(verboseMessages, "[verbose] Diff result: empty")
			verboseMessages = append(verboseMessages, "[verbose] No changes detected in the requested scope")
		}
	}

	return &ResolveReviewDiffResult{
		Diff:            diff,
		VerboseMessages: verboseMessages,
	}, nil
}

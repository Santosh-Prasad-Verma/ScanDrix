// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package pr

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/cli/git"
	"github.com/scandrix/backend/internal/cli/pr"
	"github.com/scandrix/backend/internal/cli/utils"
)

// ExecuteBusinessValidationAction runs task specification compliance against local diffs.
func ExecuteBusinessValidationAction(ctx context.Context, client *pr.PRClient, gitSvc *git.GitService, opts BusinessValidationOptions) (int, error) {
	if client == nil {
		client = pr.NewPRClient("", "")
	}
	if gitSvc == nil {
		gitSvc = git.NewGitService(".")
	}

	if opts.TaskURL == "" && opts.TaskID == "" {
		return 1, fmt.Errorf("either `--task-url` or `--task-id` must be provided for business rules validation")
	}

	// 1. Resolve local git diff
	var rawDiff string
	var err error

	if len(opts.Files) > 0 {
		rawDiff, err = gitSvc.GetDiffForFiles(ctx, opts.Files)
	} else if opts.Branch != "" {
		rawDiff, err = gitSvc.GetDiffForBranch(ctx, opts.Branch)
	} else if opts.Commit != "" {
		rawDiff, err = gitSvc.GetDiffForCommit(ctx, opts.Commit)
	} else if opts.Staged {
		rawDiff, err = gitSvc.GetStagedDiff(ctx)
	} else {
		rawDiff, err = gitSvc.GetWorkingTreeDiff(ctx)
	}

	if err != nil {
		return 1, fmt.Errorf("failed resolving diff for validation: %w", err)
	}

	rawDiff = strings.TrimSpace(rawDiff)
	if rawDiff == "" {
		if !opts.Quiet {
			utils.Warn("No code changes detected to validate against task specifications.")
		}
		return 0, nil
	}

	req := pr.BusinessValidationRequest{
		TaskURL: opts.TaskURL,
		TaskID:  opts.TaskID,
		RawDiff: rawDiff,
	}

	// 2. Dry run preview
	if opts.DryRun {
		data, _ := json.MarshalIndent(req, "", "  ")
		fmt.Printf("🔍 Dry-run Business Validation Request Payload:\n%s\n", string(data))
		return 0, nil
	}

	// 3. Submit validation request to ScanDrix API gateway
	res, err := client.RunBusinessValidation(ctx, req)
	if err != nil {
		return 1, fmt.Errorf("business rules validation failed: %w", err)
	}

	// 4. Output response
	if opts.JSONOutput {
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(data))
		if !res.Valid {
			return 1, nil
		}
		return 0, nil
	}

	printBusinessValidationSummary(res)

	if !res.Valid {
		return 1, nil
	}
	return 0, nil
}

func printBusinessValidationSummary(res *pr.BusinessValidationResponse) {
	if res == nil {
		return
	}

	statusBadge := "✅ PASSED"
	if !res.Valid {
		statusBadge = "❌ NON-COMPLIANT"
	}

	fmt.Printf("\n📋 Business Specification Validation: %s\n", statusBadge)
	if res.TaskTitle != "" {
		fmt.Printf("   Task: %s\n", res.TaskTitle)
	}
	fmt.Printf("   Compliance Score: %.1f%%\n\n", res.Score*100)

	if len(res.Requirements) > 0 {
		fmt.Println("   Requirements Met:")
		for _, r := range res.Requirements {
			fmt.Printf("     ✔ %s\n", r)
		}
		fmt.Println()
	}

	if len(res.MissingDetails) > 0 {
		fmt.Println("   Missing or Incomplete Specifications:")
		for _, m := range res.MissingDetails {
			fmt.Printf("     ✖ %s\n", m)
		}
		fmt.Println()
	}
}

// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"context"
	"strings"

	"github.com/scandrix/backend/internal/cli/services/git"
)

// RecallOptions configures decision recall query.
type RecallOptions struct {
	Paths  []string
	Limit  int
	Remote string
	Branch string
}

// RecallDecisions queries decisions matching file paths from the active branch or orphan trace ref.
func RecallDecisions(ctx context.Context, gitRoot string, opts RecallOptions) ([]Decision, error) {
	if gitRoot == "" {
		gitRoot = "."
	}
	if opts.Limit <= 0 {
		opts.Limit = 10
	}

	gitSvc := git.NewService(gitRoot)
	branch := opts.Branch
	if branch == "" {
		branch, _ = gitSvc.GetCurrentBranch(ctx)
		if branch == "" {
			branch = "main"
		}
	}

	rec, err := ReadBranchRecord(ctx, gitRoot, branch, opts.Remote)
	if err != nil || rec == nil || len(rec.Decisions) == 0 {
		return []Decision{}, nil
	}

	// Apply user overrides
	overrides, _ := ReadOverrides(gitRoot)
	allDecisions := ApplyOverrides(rec.Decisions, overrides)

	if len(opts.Paths) == 0 {
		if len(allDecisions) > opts.Limit {
			return allDecisions[:opts.Limit], nil
		}
		return allDecisions, nil
	}

	var matches []Decision
	for _, d := range allDecisions {
		matched := false
		for _, reqPath := range opts.Paths {
			reqNorm := strings.ToLower(strings.TrimSpace(reqPath))
			for _, scopePath := range d.Scope {
				scopeNorm := strings.ToLower(strings.TrimSpace(scopePath))
				if strings.Contains(scopeNorm, reqNorm) || strings.Contains(reqNorm, scopeNorm) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			matches = append(matches, d)
			if len(matches) >= opts.Limit {
				break
			}
		}
	}

	return matches, nil
}

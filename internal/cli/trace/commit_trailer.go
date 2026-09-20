// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/cli/services/git"
)

const TraceTrailerKey = "ScanDrix-Trace"

// ResolveCommitTrailer returns "ScanDrix-Trace: <id>" for the active session matching the current branch.
func ResolveCommitTrailer(ctx context.Context, gitRoot string) string {
	if gitRoot == "" {
		gitRoot = "."
	}
	gitSvc := git.NewService(gitRoot)
	branch, err := gitSvc.GetCurrentBranch(ctx)
	if err != nil {
		return ""
	}

	sessions, err := ListSessions(gitRoot)
	if err != nil || len(sessions) == 0 {
		return ""
	}

	var match *SessionSummary
	if branch != "" {
		for i := range sessions {
			if sessions[i].Branch == branch {
				match = &sessions[i]
				break
			}
		}
	}
	if match == nil {
		match = &sessions[0]
	}

	if match.SessionID == "" {
		return ""
	}

	id := match.SessionID
	if len(id) > 12 {
		id = id[:12]
	}

	return fmt.Sprintf("%s: %s", TraceTrailerKey, id)
}

// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/services/git"
)

// DistillOptions holds parameters for branch distillation.
type DistillOptions struct {
	Branch        string
	Head          string
	DefaultBranch string
	Remote        string
	Push          bool
}

// DistillResult reports outcomes of branch distillation.
type DistillResult struct {
	Branch           string   `json:"branch"`
	Head             string   `json:"head"`
	MergeBase        string   `json:"merge_base"`
	CommitsProcessed int      `json:"commits_processed"`
	DecisionsCount   int      `json:"decisions_count"`
	Pushed           bool     `json:"pushed"`
	CommitSha        string   `json:"commit_sha,omitempty"`
	Decisions        []string `json:"decisions,omitempty"`
}

// DistillBranch distills commit activity and assistant sessions into durable decisions.
func DistillBranch(ctx context.Context, gitRoot string, opts DistillOptions) (*DistillResult, error) {
	if gitRoot == "" {
		gitRoot = "."
	}

	gitSvc := git.NewService(gitRoot)

	// 1. Resolve branch, head, and default branch
	branch := opts.Branch
	if branch == "" {
		branch, _ = gitSvc.GetCurrentBranch(ctx)
		if branch == "" {
			branch = "main"
		}
	}

	head := opts.Head
	if head == "" {
		head, _ = gitSvc.GetHeadSha(ctx)
	}

	defaultBranch := opts.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	// 2. Resolve merge base
	mergeBase, _ := gitSvc.GetMergeBaseSha(ctx, defaultBranch)
	if mergeBase == "" {
		mergeBase = head
	}

	// 3. List commits in branch range
	var commitList []string
	if mergeBase != head && mergeBase != "" {
		rawCommits, _ := runGit(ctx, gitRoot, nil, "rev-list", "--reverse", fmt.Sprintf("%s..%s", mergeBase, head))
		for _, c := range strings.Split(rawCommits, "\n") {
			trimmed := strings.TrimSpace(c)
			if trimmed != "" {
				commitList = append(commitList, trimmed)
			}
		}
	}
	if len(commitList) == 0 && head != "" {
		commitList = []string{head}
	}

	// 4. Correlate with local recorded sessions
	allSessions, _ := ListSessions(gitRoot)
	var decisions []Decision
	matchedSessionIDs := make(map[string]bool)

	commitSet := make(map[string]bool)
	for _, c := range commitList {
		commitSet[c] = true
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)

	for _, sSummary := range allSessions {
		sess, err := ReadSession(gitRoot, sSummary.SessionID)
		if err != nil || sess == nil {
			continue
		}

		sessionMatched := false
		for _, turn := range sess.Turns {
			if (turn.CommitBefore != "" && commitSet[turn.CommitBefore]) ||
				(turn.CommitAfter != "" && commitSet[turn.CommitAfter]) ||
				sess.Branch == branch {
				sessionMatched = true

				// Synthesize decision from turn if it has modified files and a prompt
				if len(turn.FilesModified) > 0 && strings.TrimSpace(turn.Prompt) != "" {
					scopeFiles := make([]string, 0, len(turn.FilesModified))
					for _, f := range turn.FilesModified {
						scopeFiles = append(scopeFiles, f.Path)
					}

					decisionText := fmt.Sprintf("Implement: %s", turn.Prompt)
					if len(decisionText) > 200 {
						decisionText = decisionText[:200] + "..."
					}

					id := computeStableDecisionID(branch, decisionText, scopeFiles)
					decisions = append(decisions, Decision{
						ID:         id,
						Type:       DecisionTypeArchitectural,
						Origin:     "agent",
						Decision:   decisionText,
						Rationale:  turn.Response,
						Confidence: 0.9,
						Scope:      scopeFiles,
						Branch:     branch,
						Commits:    []string{turn.CommitAfter},
						SessionIDs: []string{sess.SessionID},
						CreatedAt:  nowStr,
					})
				}
			}
		}

		if sessionMatched {
			matchedSessionIDs[sSummary.SessionID] = true
		}
	}

	// If no sessions were recorded yet (e.g. standard git commits), synthesize from git log
	if len(decisions) == 0 {
		for _, c := range commitList {
			subject, _ := runGit(ctx, gitRoot, nil, "log", "-1", "--format=%s", c)
			filesChanged, _ := runGit(ctx, gitRoot, nil, "diff-tree", "--no-commit-id", "--name-only", "-r", c)
			var scopes []string
			for _, f := range strings.Split(filesChanged, "\n") {
				if trimmed := strings.TrimSpace(f); trimmed != "" {
					scopes = append(scopes, trimmed)
				}
			}

			if subject != "" {
				id := computeStableDecisionID(branch, subject, scopes)
				decisions = append(decisions, Decision{
					ID:         id,
					Type:       DecisionTypeConvention,
					Origin:     "human",
					Decision:   subject,
					Scope:      scopes,
					Branch:     branch,
					Commits:    []string{c},
					CreatedAt:  nowStr,
				})
			}
		}
	}

	// 5. Apply user overrides (pins & forgets)
	overrides, _ := ReadOverrides(gitRoot)
	decisions = ApplyOverrides(decisions, overrides)

	// 6. Write branch record to local store and git orphan branch (scandrix/trace/v1)
	branchRecord := &TraceBranchRecord{
		Version:     1,
		Branch:      branch,
		MergeBase:   mergeBase,
		Head:        head,
		Commits:     commitList,
		UpdatedAt:   nowStr,
		Decisions:   decisions,
		Corrections: overrides,
	}

	commitSha, err := WriteBranchRecord(ctx, gitRoot, branchRecord)
	if err != nil {
		return nil, fmt.Errorf("failed writing trace branch record: %w", err)
	}

	// 7. Push to remote if requested
	remote := opts.Remote
	if remote == "" {
		remote = "origin"
	}

	pushed := false
	if opts.Push {
		if err := PushTraceBranch(ctx, gitRoot, remote); err == nil {
			pushed = true
		}
	}

	var decisionSummaries []string
	for _, d := range decisions {
		decisionSummaries = append(decisionSummaries, fmt.Sprintf("[%s] %s (%s)", d.ID, d.Decision, strings.Join(d.Scope, ", ")))
	}

	return &DistillResult{
		Branch:           branch,
		Head:             head,
		MergeBase:        mergeBase,
		CommitsProcessed: len(commitList),
		DecisionsCount:   len(decisions),
		Pushed:           pushed,
		CommitSha:        commitSha,
		Decisions:        decisionSummaries,
	}, nil
}

func computeStableDecisionID(branch, text string, scope []string) string {
	h := sha256.New()
	h.Write([]byte(branch))
	h.Write([]byte(text))
	for _, s := range scope {
		h.Write([]byte(s))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// GenerateDecisionID calculates a deterministic hash for a decision.
func GenerateDecisionID(branch, text string, scope []string) string {
	return computeStableDecisionID(branch, text, scope)
}

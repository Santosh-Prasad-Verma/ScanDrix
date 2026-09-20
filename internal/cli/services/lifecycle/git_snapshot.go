package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/git"
)

// GitTurnSnapshot captures working tree state at turn boundaries.
type GitTurnSnapshot struct {
	CapturedAt    time.Time `json:"captured_at"`
	Branch        string    `json:"branch"`
	HeadSHA       string    `json:"head_sha"`
	IsClean       bool      `json:"is_clean"`
	ModifiedFiles []string  `json:"modified_files"`
	StagedFiles   []string  `json:"staged_files"`
	UntrackedFiles []string `json:"untracked_files"`
}

// GitTurnDelta reports mutations occurring between before and after snapshots.
type GitTurnDelta struct {
	HeadChanged      bool     `json:"head_changed"`
	OldHead          string   `json:"old_head"`
	NewHead          string   `json:"new_head"`
	FilesAdded       []string `json:"files_added"`
	FilesModified    []string `json:"files_modified"`
	FilesDeleted     []string `json:"files_deleted"`
	TotalMutations   int      `json:"total_mutations"`
}

// CaptureGitSnapshot queries git to record current head and dirty index state.
func CaptureGitSnapshot(ctx context.Context, gitSvc *git.GitService) (*GitTurnSnapshot, error) {
	if gitSvc == nil {
		gitSvc = git.NewGitService(".")
	}

	info, err := gitSvc.GetGitInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed capturing git info: %w", err)
	}

	fileDiffs, _ := gitSvc.GetModifiedFiles(ctx)
	var modified []string
	var staged []string
	for _, fd := range fileDiffs {
		if fd.Status == "staged" || fd.Status == "added" {
			staged = append(staged, fd.Path)
		} else {
			modified = append(modified, fd.Path)
		}
	}

	return &GitTurnSnapshot{
		CapturedAt:     time.Now().UTC(),
		Branch:         info.Branch,
		HeadSHA:        info.HeadSHA,
		IsClean:        info.IsClean,
		ModifiedFiles:  modified,
		StagedFiles:    staged,
		UntrackedFiles: nil,
	}, nil
}

// ComputeDelta calculates files touched between two turn snapshots.
func ComputeDelta(before, after *GitTurnSnapshot) *GitTurnDelta {
	delta := &GitTurnDelta{
		FilesAdded:    make([]string, 0),
		FilesModified: make([]string, 0),
		FilesDeleted:  make([]string, 0),
	}

	if before == nil || after == nil {
		return delta
	}

	if before.HeadSHA != after.HeadSHA {
		delta.HeadChanged = true
		delta.OldHead = before.HeadSHA
		delta.NewHead = after.HeadSHA
	}

	beforeSet := make(map[string]bool)
	for _, f := range before.ModifiedFiles {
		beforeSet[f] = true
	}
	for _, f := range before.StagedFiles {
		beforeSet[f] = true
	}
	for _, f := range before.UntrackedFiles {
		beforeSet[f] = true
	}

	afterSet := make(map[string]bool)
	for _, f := range after.ModifiedFiles {
		afterSet[f] = true
		if !beforeSet[f] {
			delta.FilesModified = append(delta.FilesModified, f)
		}
	}
	for _, f := range after.StagedFiles {
		afterSet[f] = true
		if !beforeSet[f] {
			delta.FilesModified = append(delta.FilesModified, f)
		}
	}
	for _, f := range after.UntrackedFiles {
		afterSet[f] = true
		if !beforeSet[f] {
			delta.FilesAdded = append(delta.FilesAdded, f)
		}
	}

	// Detect deleted files
	for f := range beforeSet {
		if !afterSet[f] && !delta.HeadChanged {
			delta.FilesDeleted = append(delta.FilesDeleted, f)
		}
	}

	delta.TotalMutations = len(delta.FilesAdded) + len(delta.FilesModified) + len(delta.FilesDeleted)
	return delta
}

// FormatDeltaSummary returns a compact string description of turn mutations.
func FormatDeltaSummary(d *GitTurnDelta) string {
	if d == nil || d.TotalMutations == 0 {
		return "No filesystem changes detected in this turn."
	}

	var parts []string
	if len(d.FilesAdded) > 0 {
		parts = append(parts, fmt.Sprintf("+%d created", len(d.FilesAdded)))
	}
	if len(d.FilesModified) > 0 {
		parts = append(parts, fmt.Sprintf("~%d modified", len(d.FilesModified)))
	}
	if len(d.FilesDeleted) > 0 {
		parts = append(parts, fmt.Sprintf("-%d deleted", len(d.FilesDeleted)))
	}
	if d.HeadChanged {
		parts = append(parts, fmt.Sprintf("committed (%s -> %s)", truncateSHA(d.OldHead), truncateSHA(d.NewHead)))
	}

	return strings.Join(parts, ", ")
}

func truncateSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

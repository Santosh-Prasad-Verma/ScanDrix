package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/scandrix/backend/internal/cli/types"
)

// GitService provides complete interface to git operations and repository state.
type GitService struct {
	WorkDir string
	Verbose bool
}

// NewGitService instantiates a GitService for the given working directory.
func NewGitService(workDir string) *GitService {
	return &GitService{
		WorkDir: workDir,
		Verbose: false,
	}
}

// SetVerbose toggles verbose output mode.
func (s *GitService) SetVerbose(verbose bool) {
	s.Verbose = verbose
}

func (s *GitService) runGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if s.WorkDir != "" {
		cmd.Dir = s.WorkDir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// IsGitRepository checks whether the current directory is inside a valid Git worktree or repo.
func (s *GitService) IsGitRepository(ctx context.Context) bool {
	_, err := s.runGit(ctx, "rev-parse", "--git-dir")
	return err == nil
}

// EnsureRepo validates that the working directory is inside a Git repository.
func (s *GitService) EnsureRepo(ctx context.Context) error {
	if !s.IsGitRepository(ctx) {
		return fmt.Errorf("not a git repository (or any of the parent directories). Initialize one with 'git init'")
	}
	return nil
}

// GetGitRoot returns the top-level directory of the current git repository.
func (s *GitService) GetGitRoot(ctx context.Context) (string, error) {
	if err := s.EnsureRepo(ctx); err != nil {
		return "", err
	}
	return s.runGit(ctx, "rev-parse", "--show-toplevel")
}

// GetHooksDir resolves the true directory Git executes hooks from, taking worktrees and core.hooksPath into account.
func (s *GitService) GetHooksDir(ctx context.Context) (string, error) {
	if err := s.EnsureRepo(ctx); err != nil {
		return "", err
	}

	// Try git rev-parse --path-format=absolute --git-path hooks (Git >= 2.31)
	if resolved, err := s.runGit(ctx, "rev-parse", "--path-format=absolute", "--git-path", "hooks"); err == nil && resolved != "" {
		return resolved, nil
	}

	// Fallback for older git clients
	commonDir, err := s.runGit(ctx, "rev-parse", "--git-common-dir")
	if err != nil {
		root, rErr := s.GetGitRoot(ctx)
		if rErr != nil {
			return "", err
		}
		return filepath.Join(root, ".git", "hooks"), nil
	}

	baseDir := s.WorkDir
	if baseDir == "" {
		var cwdErr error
		baseDir, cwdErr = os.Getwd()
		if cwdErr != nil {
			return "", cwdErr
		}
	}

	if filepath.IsAbs(commonDir) {
		return filepath.Join(commonDir, "hooks"), nil
	}
	return filepath.Clean(filepath.Join(baseDir, commonDir, "hooks")), nil
}

// GetHeadSHA returns the current commit hash at HEAD.
func (s *GitService) GetHeadSHA(ctx context.Context) (string, error) {
	if err := s.EnsureRepo(ctx); err != nil {
		return "", err
	}
	return s.runGit(ctx, "rev-parse", "HEAD")
}

// GetCurrentBranch returns current branch name or HEAD if detached.
func (s *GitService) GetCurrentBranch(ctx context.Context) (string, error) {
	if err := s.EnsureRepo(ctx); err != nil {
		return "", err
	}
	return s.runGit(ctx, "rev-parse", "--abbrev-ref", "HEAD")
}

// GetRemoteURL returns the fetch URL for the specified remote name (default: "origin").
func (s *GitService) GetRemoteURL(ctx context.Context, remote string) (string, error) {
	if remote == "" {
		remote = "origin"
	}
	return s.runGit(ctx, "remote", "get-url", remote)
}

// GetUserEmail retrieves the configured user.email in git config.
func (s *GitService) GetUserEmail(ctx context.Context) (string, error) {
	return s.runGit(ctx, "config", "user.email")
}

// GetMergeBaseWithUpstream attempts to locate the common ancestor between HEAD and upstream.
func (s *GitService) GetMergeBaseWithUpstream(ctx context.Context) (string, error) {
	candidates := []string{"@{upstream}", "origin/HEAD", "origin/main", "origin/master"}
	for _, ref := range candidates {
		if _, err := s.runGit(ctx, "rev-parse", "--verify", ref); err == nil {
			if sha, err := s.runGit(ctx, "merge-base", "HEAD", ref); err == nil && sha != "" {
				return sha, nil
			}
		}
	}
	return "", fmt.Errorf("no upstream merge-base candidate found")
}

// GetGitInfo collects full repository context and metadata.
func (s *GitService) GetGitInfo(ctx context.Context) (*types.GitInfo, error) {
	root, err := s.GetGitRoot(ctx)
	if err != nil {
		return nil, err
	}

	info := &types.GitInfo{
		RootPath: root,
		Platform: types.PlatformUnknown,
	}

	if branch, err := s.GetCurrentBranch(ctx); err == nil {
		info.Branch = branch
	}
	if sha, err := s.GetHeadSHA(ctx); err == nil {
		info.HeadSHA = sha
	}
	if remoteURL, err := s.GetRemoteURL(ctx, "origin"); err == nil {
		info.RemoteURL = remoteURL
		remoteInfo := ParseRemoteURL(remoteURL)
		if remoteInfo != nil {
			info.Platform = types.PlatformType(remoteInfo.Provider)
			parts := strings.Split(remoteInfo.NamespacePath, "/")
			if len(parts) >= 2 {
				info.Owner = parts[0]
				info.Repo = parts[1]
			}
		}
	}

	if hooksDir, err := s.GetHooksDir(ctx); err == nil {
		info.HooksDir = hooksDir
	}

	// Check worktree status
	if gitDir, err := s.runGit(ctx, "rev-parse", "--git-dir"); err == nil {
		info.IsWorktree = strings.Contains(gitDir, ".git/worktrees")
	}

	// Check clean status
	statusOut, err := s.runGit(ctx, "status", "--porcelain")
	if err == nil {
		info.IsClean = strings.TrimSpace(statusOut) == ""
	}

	// Check tracking ahead/behind
	if aheadBehind, err := s.runGit(ctx, "rev-list", "--left-right", "--count", "HEAD...@{upstream}"); err == nil {
		fields := strings.Fields(aheadBehind)
		if len(fields) >= 2 {
			info.TrackingAhead, _ = strconv.Atoi(fields[0])
			info.TrackingBehind, _ = strconv.Atoi(fields[1])
		}
	}

	return info, nil
}

// GetWorkingTreeDiff retrieves full diff of staged + unstaged changes.
func (s *GitService) GetWorkingTreeDiff(ctx context.Context) (string, error) {
	if err := s.EnsureRepo(ctx); err != nil {
		return "", err
	}

	staged, _ := s.runGit(ctx, "diff", "--cached")
	unstaged, _ := s.runGit(ctx, "diff")

	combined := strings.TrimSpace(staged + "\n" + unstaged)
	return combined, nil
}

// GetStagedDiff retrieves only staged changes.
func (s *GitService) GetStagedDiff(ctx context.Context) (string, error) {
	if err := s.EnsureRepo(ctx); err != nil {
		return "", err
	}
	return s.runGit(ctx, "diff", "--cached")
}

// GetDiffForCommit retrieves diff for a specific commit SHA.
func (s *GitService) GetDiffForCommit(ctx context.Context, commitSHA string) (string, error) {
	if err := s.EnsureRepo(ctx); err != nil {
		return "", err
	}
	return s.runGit(ctx, "diff", commitSHA+"^", commitSHA)
}

// GetDiffForBranch retrieves diff between branch reference and HEAD.
func (s *GitService) GetDiffForBranch(ctx context.Context, branch string) (string, error) {
	if err := s.EnsureRepo(ctx); err != nil {
		return "", err
	}
	return s.runGit(ctx, "diff", branch+"...HEAD")
}

// GetDiffForFiles retrieves staged and unstaged diffs for an explicit list of files.
func (s *GitService) GetDiffForFiles(ctx context.Context, files []string) (string, error) {
	if err := s.EnsureRepo(ctx); err != nil {
		return "", err
	}

	var buf strings.Builder
	for _, f := range files {
		staged, _ := s.runGit(ctx, "diff", "--cached", "--", f)
		unstaged, _ := s.runGit(ctx, "diff", "--", f)
		if staged != "" {
			buf.WriteString(staged)
			buf.WriteString("\n")
		}
		if unstaged != "" {
			buf.WriteString(unstaged)
			buf.WriteString("\n")
		}
	}

	return strings.TrimSpace(buf.String()), nil
}

// GetModifiedFiles parses porcelain status and counts additions/deletions.
func (s *GitService) GetModifiedFiles(ctx context.Context) ([]types.FileDiff, error) {
	if err := s.EnsureRepo(ctx); err != nil {
		return nil, err
	}

	out, err := s.runGit(ctx, "status", "--porcelain")
	if err != nil {
		return nil, err
	}

	entries := ParsePorcelainStatus(out)
	diffs := make([]types.FileDiff, 0, len(entries))

	for _, entry := range entries {
		diffText, _ := s.runGit(ctx, "diff", "--", entry.File)
		if entry.IsStaged && diffText == "" {
			diffText, _ = s.runGit(ctx, "diff", "--cached", "--", entry.File)
		}
		stats := CountDiffChanges(diffText)

		diffs = append(diffs, types.FileDiff{
			Path:      entry.File,
			OldPath:   entry.OldFile,
			Status:    entry.Status,
			Additions: stats.Additions,
			Deletions: stats.Deletions,
			Patch:     diffText,
		})
	}

	return diffs, nil
}

// GetFullFileContents loads the complete contents and diffs for target files.
func (s *GitService) GetFullFileContents(ctx context.Context, explicitFiles []string, options *GitFileReadOptions) ([]types.FileContent, error) {
	if err := s.EnsureRepo(ctx); err != nil {
		return nil, err
	}

	var selection FileSelection
	if len(explicitFiles) > 0 {
		selection = CreateFileSelectionFromPaths(explicitFiles)
	} else if options != nil && options.Branch != "" {
		nameStatus, err := s.runGit(ctx, "diff", "--name-status", options.Branch+"...HEAD")
		if err != nil {
			return nil, err
		}
		selection = CreateFileSelectionFromNameStatus(nameStatus)
	} else if options != nil && options.Commit != "" {
		nameStatus, err := s.runGit(ctx, "diff", "--name-status", options.Commit+"^", options.Commit)
		if err != nil {
			return nil, err
		}
		selection = CreateFileSelectionFromNameStatus(nameStatus)
	} else {
		modified, err := s.GetModifiedFiles(ctx)
		if err != nil {
			return nil, err
		}
		selection = CreateFileSelectionFromModifiedFiles(modified)
	}

	root, _ := s.GetGitRoot(ctx)
	ignorePatterns := LoadIgnorePatterns(root)
	filteredPaths := FilterReviewTargets(selection.FilesToRead, selection.FileStatusMap, ignorePatterns)

	contents := make([]types.FileContent, 0, len(filteredPaths))
	for _, relPath := range filteredPaths {
		contentPlan := BuildFileContentReadPlan(relPath, options)
		var fileData string

		if contentPlan.Mode == "git-show" {
			showArgs := append([]string{"show"}, contentPlan.Args...)
			out, err := s.runGit(ctx, showArgs...)
			if err != nil {
				continue // skip binary or missing file
			}
			fileData = out
		} else {
			fullPath := filepath.Join(root, relPath)
			bytesData, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			fileData = string(bytesData)
		}

		lineCount := strings.Count(fileData, "\n")
		if len(fileData) > 0 && !strings.HasSuffix(fileData, "\n") {
			lineCount++
		}

		contents = append(contents, types.FileContent{
			Path:    relPath,
			Content: fileData,
			Lines:   lineCount,
		})
	}

	return contents, nil
}

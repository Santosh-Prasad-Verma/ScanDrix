package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/scandrix/backend/internal/cli/utils"
)

// GIT SERVICE & REPOSITORY OPERATIONS

// Service provides git operations and diff extraction.
type Service struct {
	workDir string
}

var defaultGitService = &Service{workDir: "."}

// DefaultService returns the default GitService instance.
func DefaultService() *Service {
	return defaultGitService
}

// NewService creates a GitService bound to a work directory.
func NewService(workDir string) *Service {
	if workDir == "" {
		workDir = "."
	}
	return &Service{workDir: workDir}
}

func (s *Service) runGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if s.workDir != "" && s.workDir != "." {
		cmd.Dir = s.workDir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// REPOSITORY STATUS & METADATA INSPECTION

// IsGitRepository checks whether workDir is inside a Git work tree.
func (s *Service) IsGitRepository(ctx context.Context) bool {
	_, err := s.runGit(ctx, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

// GetGitRoot returns the top-level directory of the Git repository.
func (s *Service) GetGitRoot(ctx context.Context) (string, error) {
	out, err := s.runGit(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", utils.NewCommandError(utils.ErrCodeNotInGitRepo, "Not a git repository. Run inside a git repo or run 'git init'.", 1, nil)
	}
	return strings.TrimSpace(out), nil
}

// GetHeadSha returns the full commit hash of HEAD.
func (s *Service) GetHeadSha(ctx context.Context) (string, error) {
	out, err := s.runGit(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// GetCurrentBranch returns the active git branch name.
func (s *Service) GetCurrentBranch(ctx context.Context) (string, error) {
	out, err := s.runGit(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "HEAD", nil
	}
	branch := strings.TrimSpace(out)
	if branch == "" {
		return "HEAD", nil
	}
	return branch, nil
}

// GetRemoteURL returns the fetch URL for the specified remote.
func (s *Service) GetRemoteURL(ctx context.Context, remote string) (string, error) {
	if remote == "" {
		remote = "origin"
	}
	out, err := s.runGit(ctx, "remote", "get-url", remote)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// DIFF EXTRACTION & WORKING TREE INSPECTION

// GetWorkingTreeDiff extracts unstaged changes in the working directory.
func (s *Service) GetWorkingTreeDiff(ctx context.Context) (string, error) {
	return s.runGit(ctx, "diff", "--no-color", "--unified=3")
}

// GetStagedDiff extracts changes staged in the index (git diff --cached).
func (s *Service) GetStagedDiff(ctx context.Context) (string, error) {
	return s.runGit(ctx, "diff", "--cached", "--no-color", "--unified=3")
}

// GetBranchDiff extracts diff between base branch and HEAD.
func (s *Service) GetBranchDiff(ctx context.Context, baseBranch string) (string, error) {
	if baseBranch == "" {
		baseBranch = "main"
	}
	// Try origin/<base> first, then fallback to local <base>
	if diff, err := s.runGit(ctx, "diff", "--no-color", "--unified=3", fmt.Sprintf("origin/%s...HEAD", baseBranch)); err == nil && len(diff) > 0 {
		return diff, nil
	}
	return s.runGit(ctx, "diff", "--no-color", "--unified=3", fmt.Sprintf("%s...HEAD", baseBranch))
}

// GetCommitDiff extracts diff introduced by a specific commit SHA.
func (s *Service) GetCommitDiff(ctx context.Context, sha string) (string, error) {
	return s.runGit(ctx, "show", "--no-color", "--unified=3", sha)
}

// GetFilesDiff extracts diff for specific files.
func (s *Service) GetFilesDiff(ctx context.Context, files []string, staged bool) (string, error) {
	args := []string{"diff", "--no-color", "--unified=3"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--")
	args = append(args, files...)
	return s.runGit(ctx, args...)
}
// GetHooksDir returns the absolute path to the directory Git executes hooks from.
// It uses `git rev-parse --path-format=absolute --git-path hooks` to properly handle
// linked git worktrees (where .git is a file) and respects core.hooksPath configuration.
func (s *Service) GetHooksDir(ctx context.Context) (string, error) {
	out, err := s.runGit(ctx, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	if err == nil {
		resolved := strings.TrimSpace(out)
		if resolved != "" {
			return resolved, nil
		}
	}

	// Fallback for older git versions (< 2.31) without --path-format=absolute
	commonDir, err := s.runGit(ctx, "rev-parse", "--git-common-dir")
	if err != nil {
		gitDir, gErr := s.runGit(ctx, "rev-parse", "--git-dir")
		if gErr != nil {
			return "", fmt.Errorf("failed resolving git directory: %w", gErr)
		}
		commonDir = gitDir
	}
	trimmedCommon := strings.TrimSpace(commonDir)
	if filepath.IsAbs(trimmedCommon) {
		return filepath.Join(trimmedCommon, "hooks"), nil
	}
	base := s.workDir
	if base == "" || base == "." {
		if cwd, cErr := os.Getwd(); cErr == nil {
			base = cwd
		}
	}
	return filepath.Join(base, trimmedCommon, "hooks"), nil
}

// GetMergeBaseSha finds the common ancestor between HEAD and upstream base branch.
func (s *Service) GetMergeBaseSha(ctx context.Context, baseBranch string) (string, error) {
	if baseBranch == "" {
		baseBranch = "main"
	}
	// Try origin/<baseBranch> first
	if out, err := s.runGit(ctx, "merge-base", fmt.Sprintf("origin/%s", baseBranch), "HEAD"); err == nil && len(strings.TrimSpace(out)) > 0 {
		return strings.TrimSpace(out), nil
	}
	// Try local <baseBranch>
	if out, err := s.runGit(ctx, "merge-base", baseBranch, "HEAD"); err == nil && len(strings.TrimSpace(out)) > 0 {
		return strings.TrimSpace(out), nil
	}
	// Fallback: merge-base against HEAD~1 if present
	out, err := s.runGit(ctx, "rev-parse", "HEAD~1")
	if err == nil {
		return strings.TrimSpace(out), nil
	}
	return "", fmt.Errorf("could not determine merge-base for branch %s", baseBranch)
}

// GetUserEmail returns configured git user email or empty string.
func (s *Service) GetUserEmail(ctx context.Context) string {
	out, err := s.runGit(ctx, "config", "user.email")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// FileContent holds file path and full text content for review inlining.
type FileContent struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// GetFullFileContents reads file contents for review inlining, skipping binary or oversized files.
func (s *Service) GetFullFileContents(ctx context.Context, files []string, maxFileSize int64, maxTotalBytes int64) ([]FileContent, error) {
	if maxFileSize <= 0 {
		maxFileSize = 500 * 1024 // 500 KB default per file
	}
	if maxTotalBytes <= 0 {
		maxTotalBytes = 20 * 1024 * 1024 // 20 MB total payload guard
	}

	var results []FileContent
	var currentTotal int64

	for _, f := range files {
		cleanPath := filepath.Clean(f)
		fullPath := cleanPath
		if !filepath.IsAbs(cleanPath) {
			fullPath = filepath.Join(s.workDir, cleanPath)
		}

		info, err := os.Stat(fullPath)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Size() > maxFileSize || currentTotal+info.Size() > maxTotalBytes {
			continue
		}

		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		// Check for binary data (null bytes in first 512 bytes)
		probeLen := len(data)
		if probeLen > 512 {
			probeLen = 512
		}
		if bytes.IndexByte(data[:probeLen], 0) != -1 {
			continue // Skip binary file
		}

		results = append(results, FileContent{
			Path:    cleanPath,
			Content: string(data),
		})
		currentTotal += info.Size()
	}

	return results, nil
}

// CountDiffChanges parses a unified diff and returns files changed, additions, and deletions.
func CountDiffChanges(diff string) (filesChanged, additions, deletions int) {
	seenFiles := make(map[string]bool)
	lines := strings.Split(diff, "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "+++ b/") {
			file := strings.TrimPrefix(line, "+++ b/")
			if file != "/dev/null" {
				seenFiles[file] = true
			}
		} else if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			deletions++
		}
	}
	return len(seenFiles), additions, deletions
}

// InferPlatform extracts VCS platform identifier from a remote URL.
func InferPlatform(remoteURL string) string {
	lower := strings.ToLower(remoteURL)
	switch {
	case strings.Contains(lower, "github.com"):
		return "GITHUB"
	case strings.Contains(lower, "gitlab.com"):
		return "GITLAB"
	case strings.Contains(lower, "bitbucket.org"):
		return "BITBUCKET"
	case strings.Contains(lower, "dev.azure.com") || strings.Contains(lower, "visualstudio.com"):
		return "AZURE_REPOS"
	default:
		return "GITHUB"
	}
}

// ExtractOrgRepo parses org and repo name from remote URL (SSH or HTTPS).
func ExtractOrgRepo(remoteURL string) (org, repo string, ok bool) {
	trimmed := strings.TrimSpace(remoteURL)
	trimmed = strings.TrimSuffix(trimmed, ".git")

	// SSH: git@github.com:org/repo
	if strings.Contains(trimmed, "@") && strings.Contains(trimmed, ":") {
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) == 2 {
			subParts := strings.Split(parts[1], "/")
			if len(subParts) >= 2 {
				return subParts[len(subParts)-2], subParts[len(subParts)-1], true
			}
		}
	}

	// HTTPS: https://github.com/org/repo
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		parts := strings.Split(trimmed, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2], parts[len(parts)-1], true
		}
	}

	return "", "", false
}

// GitInfo aggregates current Git environment details for telemetry and sandbox checkout.
type GitInfo struct {
	Remote           string `json:"remote,omitempty"`
	Branch           string `json:"branch,omitempty"`
	CommitSHA        string `json:"commit_sha,omitempty"`
	MergeBaseSHA     string `json:"merge_base_sha,omitempty"`
	UserEmail        string `json:"user_email,omitempty"`
	InferredPlatform string `json:"inferred_platform,omitempty"`
	Org              string `json:"org,omitempty"`
	Repo             string `json:"repo,omitempty"`
}

// GetGitInfo collects GitInfo for current HEAD.
func (s *Service) GetGitInfo(ctx context.Context, baseBranch string) (*GitInfo, error) {
	info := &GitInfo{}
	info.Branch, _ = s.GetCurrentBranch(ctx)
	info.CommitSHA, _ = s.GetHeadSha(ctx)
	info.Remote, _ = s.GetRemoteURL(ctx, "origin")
	info.UserEmail = s.GetUserEmail(ctx)
	if info.Remote != "" {
		info.InferredPlatform = InferPlatform(info.Remote)
		if org, repo, ok := ExtractOrgRepo(info.Remote); ok {
			info.Org = org
			info.Repo = repo
		}
	}
	if baseBranch != "" {
		info.MergeBaseSHA, _ = s.GetMergeBaseSha(ctx, baseBranch)
	}
	return info, nil
}

// GetUntrackedFiles returns relative paths of untracked files.
func (s *Service) GetUntrackedFiles(ctx context.Context) ([]string, error) {
	out, err := s.runGit(ctx, "status", "--porcelain")
	if err != nil {
		return nil, err
	}

	var untracked []string
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "?? ") {
			untracked = append(untracked, strings.TrimSpace(l[3:]))
		}
	}
	return untracked, nil
}

// ReadFileContentsAtWorkingTree reads file content directly from disk.
func (s *Service) ReadFileContentsAtWorkingTree(filePath string) (string, error) {
	fullPath := filePath
	if !filepath.IsAbs(filePath) {
		fullPath = filepath.Join(s.workDir, filePath)
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

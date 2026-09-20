// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	TraceBranch = "scandrix/trace/v1"
	TraceRef    = "refs/heads/" + TraceBranch
)

// RecordPathForBranch returns sharded git repository path for a branch record.
func RecordPathForBranch(branchName string) string {
	h := sha256.Sum256([]byte(branchName))
	hashStr := hex.EncodeToString(h[:])
	return fmt.Sprintf("records/%s/%s.json", hashStr[:2], hashStr)
}

func runGit(ctx context.Context, gitRoot string, stdin []byte, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = gitRoot
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func runGitWithEnv(ctx context.Context, gitRoot string, env []string, stdin []byte, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = gitRoot
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func runGitQuiet(ctx context.Context, gitRoot string, args ...string) (string, bool) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = gitRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// ResolveTraceRef checks if the trace branch exists locally or on the remote.
func ResolveTraceRef(ctx context.Context, gitRoot, remote string) (string, error) {
	if _, ok := runGitQuiet(ctx, gitRoot, "rev-parse", "--verify", TraceRef); ok {
		return TraceRef, nil
	}

	if remote == "" {
		remote = "origin"
	}
	remoteRef := fmt.Sprintf("refs/remotes/%s/%s", remote, TraceBranch)
	if _, ok := runGitQuiet(ctx, gitRoot, "rev-parse", "--verify", remoteRef); ok {
		return remoteRef, nil
	}

	return "", fmt.Errorf("trace branch not found")
}

// ReadBranchRecord reads a branch's decisions from local cache or the git object database.
func ReadBranchRecord(ctx context.Context, gitRoot, branchName, remote string) (*TraceBranchRecord, error) {
	// 1. Try local disk cache first
	localPath := LocalBranchDecisionPath(gitRoot, branchName)
	if data, err := os.ReadFile(localPath); err == nil {
		var rec TraceBranchRecord
		if err := json.Unmarshal(data, &rec); err == nil {
			return &rec, nil
		}
	}

	// 2. Read directly from git object database on orphan branch
	ref, err := ResolveTraceRef(ctx, gitRoot, remote)
	if err != nil {
		return nil, err
	}

	relPath := RecordPathForBranch(branchName)
	raw, err := runGit(ctx, gitRoot, nil, "show", fmt.Sprintf("%s:%s", ref, relPath))
	if err != nil {
		return nil, err
	}

	var rec TraceBranchRecord
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		return nil, fmt.Errorf("failed decoding trace record: %w", err)
	}
	return &rec, nil
}

// WriteBranchRecord persists a branch record both to local disk and the git orphan branch.
func WriteBranchRecord(ctx context.Context, gitRoot string, record *TraceBranchRecord) (string, error) {
	if gitRoot == "" {
		gitRoot = "."
	}
	if record == nil {
		return "", fmt.Errorf("cannot write nil branch record")
	}

	// 1. Save to local disk cache outside repository
	localDir := LocalDecisionsDir(gitRoot)
	_ = os.MkdirAll(localDir, 0700)
	localPath := LocalBranchDecisionPath(gitRoot, record.Branch)
	jsonData, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", err
	}
	_ = os.WriteFile(localPath, append(jsonData, '\n'), 0600)

	// 2. Write blob into Git Object Database (ODB)
	blobSha, err := runGit(ctx, gitRoot, jsonData, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", fmt.Errorf("failed hashing record blob: %w", err)
	}

	recordRelPath := RecordPathForBranch(record.Branch)

	// Check if TraceRef exists
	parentSha, hasParent := runGitQuiet(ctx, gitRoot, "rev-parse", "--verify", TraceRef)

	// Create temporary index file to build tree with arbitrary nested paths
	gitDir, _ := runGit(ctx, gitRoot, nil, "rev-parse", "--git-dir")
	if gitDir == "" {
		gitDir = filepath.Join(gitRoot, ".git")
	} else if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(gitRoot, gitDir)
	}
	tmpIndex := filepath.Join(gitDir, fmt.Sprintf("scandrix_trace_idx_%d", time.Now().UnixNano()))
	defer os.Remove(tmpIndex)
	idxEnv := []string{"GIT_INDEX_FILE=" + tmpIndex}

	if hasParent {
		if _, err := runGitWithEnv(ctx, gitRoot, idxEnv, nil, "read-tree", parentSha); err != nil {
			return "", fmt.Errorf("failed reading parent tree into temp index: %w", err)
		}
	}

	if _, err := runGitWithEnv(ctx, gitRoot, idxEnv, nil, "update-index", "--add", "--cacheinfo", "100644", blobSha, recordRelPath); err != nil {
		return "", fmt.Errorf("failed updating temp index with trace record: %w", err)
	}

	treeSha, err := runGitWithEnv(ctx, gitRoot, idxEnv, nil, "write-tree")
	if err != nil {
		return "", fmt.Errorf("failed writing git tree for trace: %w", err)
	}

	// Create commit object
	commitMsg := fmt.Sprintf("scandrix(trace): record decisions for branch '%s'", record.Branch)
	var commitSha string
	if hasParent {
		commitSha, err = runGit(ctx, gitRoot, nil, "commit-tree", treeSha, "-p", parentSha, "-m", commitMsg)
	} else {
		commitSha, err = runGit(ctx, gitRoot, nil, "commit-tree", treeSha, "-m", commitMsg)
	}

	if err != nil {
		return "", fmt.Errorf("failed creating git commit for trace: %w", err)
	}

	// Update git ref
	_, err = runGit(ctx, gitRoot, nil, "update-ref", TraceRef, commitSha)
	if err != nil {
		return "", fmt.Errorf("failed updating %s ref: %w", TraceRef, err)
	}

	return commitSha, nil
}

// PushTraceBranch pushes the local trace orphan ref to the remote repository.
func PushTraceBranch(ctx context.Context, gitRoot, remote string) error {
	if remote == "" {
		remote = "origin"
	}

	refspec := fmt.Sprintf("%s:%s", TraceRef, TraceRef)
	_, err := runGit(ctx, gitRoot, nil, "push", remote, refspec)
	return err
}

// ConfigureTraceRefspec ensures standard git fetch synchronizes the scandrix/trace/v1 branch.
func ConfigureTraceRefspec(ctx context.Context, gitRoot, remote string) error {
	if remote == "" {
		remote = "origin"
	}

	fetchRefspec := fmt.Sprintf("+%s:refs/remotes/%s/%s", TraceRef, remote, TraceBranch)
	existing, _ := runGit(ctx, gitRoot, nil, "config", "--get-all", fmt.Sprintf("remote.%s.fetch", remote))
	if strings.Contains(existing, TraceBranch) {
		return nil
	}

	_, err := runGit(ctx, gitRoot, nil, "config", "--add", fmt.Sprintf("remote.%s.fetch", remote), fetchRefspec)
	return err
}

// ReadAllBranchRecords retrieves all branch decision records from both local disk cache and the git orphan branch.
func ReadAllBranchRecords(ctx context.Context, gitRoot, remote string) ([]TraceBranchRecord, error) {
	byBranch := make(map[string]TraceBranchRecord)

	// 1. Read from Git orphan branch (if exists)
	if ref, err := ResolveTraceRef(ctx, gitRoot, remote); err == nil {
		if listing, err := runGit(ctx, gitRoot, nil, "ls-tree", "-r", "--name-only", ref); err == nil {
			for _, filePath := range strings.Split(listing, "\n") {
				filePath = strings.TrimSpace(filePath)
				if !strings.HasPrefix(filePath, "records/") || !strings.HasSuffix(filePath, ".json") {
					continue
				}
				if raw, err := runGit(ctx, gitRoot, nil, "show", fmt.Sprintf("%s:%s", ref, filePath)); err == nil {
					var rec TraceBranchRecord
					if err := json.Unmarshal([]byte(raw), &rec); err == nil && rec.Branch != "" {
						byBranch[rec.Branch] = rec
					}
				}
			}
		}
	}

	// 2. Read from local disk cache (overriding any older branch records)
	localDir := LocalDecisionsDir(gitRoot)
	if entries, err := os.ReadDir(localDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			p := filepath.Join(localDir, entry.Name())
			if data, err := os.ReadFile(p); err == nil {
				var rec TraceBranchRecord
				if err := json.Unmarshal(data, &rec); err == nil && rec.Branch != "" {
					byBranch[rec.Branch] = rec
				}
			}
		}
	}

	result := make([]TraceBranchRecord, 0, len(byBranch))
	for _, rec := range byBranch {
		result = append(result, rec)
	}
	return result, nil
}


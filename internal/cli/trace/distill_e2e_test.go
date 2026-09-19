// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func setupTestGitRepo(t *testing.T) (string, string) {
	tmpDir := t.TempDir()
	originDir := filepath.Join(tmpDir, "origin.git")
	repoDir := filepath.Join(tmpDir, "repo")

	// Init bare origin
	cmd := exec.Command("git", "init", "--bare", "--initial-branch=main", originDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init bare origin: %v", err)
	}

	// Init local repo
	cmd = exec.Command("git", "init", "--initial-branch=main", repoDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init repo: %v", err)
	}

	gitConfig(t, repoDir)

	// Add remote
	cmd = exec.Command("git", "-C", repoDir, "remote", "add", "origin", originDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add remote: %v", err)
	}

	// Initial commit
	readmePath := filepath.Join(repoDir, "README.md")
	os.WriteFile(readmePath, []byte("# ScanDrix Test Repo\n"), 0644)
	exec.Command("git", "-C", repoDir, "add", "README.md").Run()
	exec.Command("git", "-C", repoDir, "commit", "-m", "Initial commit").Run()
	exec.Command("git", "-C", repoDir, "push", "-u", "origin", "main").Run()

	return repoDir, originDir
}

func gitConfig(t *testing.T, repoDir string) {
	exec.Command("git", "-C", repoDir, "config", "user.name", "ScanDrix Tester").Run()
	exec.Command("git", "-C", repoDir, "config", "user.email", "test@scandrix.dev").Run()
}

func TestDistillBranch_E2E(t *testing.T) {
	repoDir, _ := setupTestGitRepo(t)
	ctx := context.Background()

	// Checkout feature branch
	exec.Command("git", "-C", repoDir, "checkout", "-b", "feature/auth").Run()

	// Record trace session lines
	sessionID := "sess-test-auth-101"
	startLine := TraceRecordLine{
		Kind:      "session-start",
		SessionID: sessionID,
		AgentType: AgentClaudeCode,
		Branch:    "feature/auth",
		Timestamp: time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
	}
	if err := AppendSessionLine(repoDir, startLine); err != nil {
		t.Fatalf("AppendSessionLine start failed: %v", err)
	}

	turnLine := TraceRecordLine{
		Kind:      "turn-end",
		SessionID: sessionID,
		TurnID:    "turn-1",
		Prompt:    "Implement JWT verification with expiration check",
		Response:  "I will implement ValidateJWT with claims and expiry validation.",
		FilesModified: []FileChange{
			{Path: "pkg/auth/jwt.go", Additions: 10, Deletions: 0},
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if err := AppendSessionLine(repoDir, turnLine); err != nil {
		t.Fatalf("AppendSessionLine turn failed: %v", err)
	}

	// Create and commit a file
	authFile := filepath.Join(repoDir, "pkg", "auth", "jwt.go")
	os.MkdirAll(filepath.Dir(authFile), 0755)
	os.WriteFile(authFile, []byte("package auth\n\nfunc ValidateJWT() bool { return true }\n"), 0644)
	exec.Command("git", "-C", repoDir, "add", ".").Run()
	exec.Command("git", "-C", repoDir, "commit", "-m", "Add ValidateJWT implementation").Run()

	// Run DistillBranch
	res, err := DistillBranch(ctx, repoDir, DistillOptions{
		Branch:        "feature/auth",
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("DistillBranch failed: %v", err)
	}

	if res.Branch != "feature/auth" {
		t.Errorf("Expected branch 'feature/auth', got %q", res.Branch)
	}
	if res.CommitsProcessed < 1 {
		t.Errorf("Expected >= 1 commits processed, got %d", res.CommitsProcessed)
	}

	// Verify Recall
	recalled, err := RecallDecisions(ctx, repoDir, RecallOptions{
		Branch: "feature/auth",
		Paths:  []string{"pkg/auth/jwt.go"},
	})
	if err != nil {
		t.Fatalf("RecallDecisions failed: %v", err)
	}
	_ = recalled
}

func TestDecisionID_Consistency(t *testing.T) {
	id1 := GenerateDecisionID("feature/x", "Implement rate limit", []string{"pkg/api/limiter.go"})
	id2 := GenerateDecisionID("feature/x", "Implement rate limit", []string{"pkg/api/limiter.go"})
	id3 := GenerateDecisionID("feature/y", "Implement rate limit", []string{"pkg/api/limiter.go"})

	if id1 != id2 {
		t.Errorf("Decision IDs should be deterministic: %s != %s", id1, id2)
	}
	if id1 == id3 {
		t.Errorf("Decision IDs on different branches should differ: %s == %s", id1, id3)
	}
}

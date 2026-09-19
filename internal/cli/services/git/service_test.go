package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/scandrix/backend/internal/cli/services/git"
)

func TestGitServiceOperations(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a temporary Git repository
	runCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}

	runCmd("init")
	runCmd("config", "user.email", "test@scandrix.dev")
	runCmd("config", "user.name", "ScanDrix Tester")

	svc := git.NewService(tmpDir)

	// Verify repo detection
	if !svc.IsGitRepository(context.Background()) {
		t.Fatalf("expected tmpDir to be recognized as a Git repository")
	}

	// Verify Git root
	root, err := svc.GetGitRoot(context.Background())
	if err != nil {
		t.Fatalf("GetGitRoot error: %v", err)
	}
	// On symlinked OS paths, compare eval symlinks
	realTmp, _ := filepath.EvalSymlinks(tmpDir)
	realRoot, _ := filepath.EvalSymlinks(root)
	if realTmp != realRoot {
		t.Errorf("expected git root %s, got %s", realTmp, realRoot)
	}

	// Create an untracked file
	testFile := filepath.Join(tmpDir, "example.txt")
	if err := os.WriteFile(testFile, []byte("hello initial\n"), 0644); err != nil {
		t.Fatalf("write file error: %v", err)
	}

	untracked, err := svc.GetUntrackedFiles(context.Background())
	if err != nil {
		t.Fatalf("GetUntrackedFiles error: %v", err)
	}
	if len(untracked) != 1 || untracked[0] != "example.txt" {
		t.Fatalf("expected 1 untracked file 'example.txt', got %+v", untracked)
	}

	// Commit file
	runCmd("add", "example.txt")
	runCmd("commit", "-m", "initial commit")

	// Modify file (working tree diff)
	if err := os.WriteFile(testFile, []byte("hello modified\n"), 0644); err != nil {
		t.Fatalf("write file error: %v", err)
	}

	diff, err := svc.GetWorkingTreeDiff(context.Background())
	if err != nil {
		t.Fatalf("GetWorkingTreeDiff error: %v", err)
	}
	if len(diff) == 0 {
		t.Fatalf("expected non-empty working tree diff")
	}

	// Stage file (staged diff)
	runCmd("add", "example.txt")
	stagedDiff, err := svc.GetStagedDiff(context.Background())
	if err != nil {
		t.Fatalf("GetStagedDiff error: %v", err)
	}
	if len(stagedDiff) == 0 {
		t.Fatalf("expected non-empty staged diff")
	}

	// Verify HooksDir resolution
	hooksDir, err := svc.GetHooksDir(context.Background())
	if err != nil {
		t.Fatalf("GetHooksDir error: %v", err)
	}
	if !filepath.IsAbs(hooksDir) || !filepath.IsAbs(hooksDir) || filepath.Base(hooksDir) != "hooks" {
		t.Errorf("unexpected hooksDir: %s", hooksDir)
	}

	// Verify Diff changes count
	filesChanged, adds, dels := git.CountDiffChanges(stagedDiff)
	if filesChanged == 0 || adds == 0 {
		t.Errorf("CountDiffChanges failed: got files=%d, adds=%d, dels=%d", filesChanged, adds, dels)
	}

	// Verify Full file contents reading
	contents, err := svc.GetFullFileContents(context.Background(), []string{"example.txt"}, 0, 0)
	if err != nil {
		t.Fatalf("GetFullFileContents error: %v", err)
	}
	if len(contents) != 1 || contents[0].Path != "example.txt" || contents[0].Content != "hello modified\n" {
		t.Errorf("unexpected GetFullFileContents: %+v", contents)
	}

	// Verify platform inference
	if git.InferPlatform("git@github.com:scandrix/scanner.git") != "GITHUB" {
		t.Errorf("expected GITHUB")
	}
	if git.InferPlatform("https://gitlab.com/group/repo.git") != "GITLAB" {
		t.Errorf("expected GITLAB")
	}
	if git.InferPlatform("git@bitbucket.org:team/repo.git") != "BITBUCKET" {
		t.Errorf("expected BITBUCKET")
	}

	// Verify org/repo extraction
	org, repo, ok := git.ExtractOrgRepo("git@github.com:scandrix/backend.git")
	if !ok || org != "scandrix" || repo != "backend" {
		t.Errorf("ExtractOrgRepo SSH failed: got %s/%s ok=%v", org, repo, ok)
	}
	org, repo, ok = git.ExtractOrgRepo("https://github.com/my-org/my-project")
	if !ok || org != "my-org" || repo != "my-project" {
		t.Errorf("ExtractOrgRepo HTTPS failed: got %s/%s ok=%v", org, repo, ok)
	}
}

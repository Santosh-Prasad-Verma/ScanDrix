package tools

import "testing"

// The read-only sandbox used to run `sh -c <string>` after checking a blocklist
// with strings.HasPrefix. Every shell metacharacter was live, so any of these
// executed a destructive command while starting with an allowed word.
//
// AUDIT_REMEDIATION.md: gosec G702, command injection.
func TestValidateReadOnlyCommandRejectsShellInjection(t *testing.T) {
	attacks := []string{
		"cat /etc/passwd; rm -rf /",
		"cat /etc/passwd && rm -rf /",
		"cat /etc/passwd || rm -rf /",
		"echo hi & rm -rf /",
		"echo $(rm -rf /)",
		"echo `rm -rf /`",
		"ls\nrm -rf /",
		"echo $HOME",
		"grep foo /etc/shadow | tee /tmp/x",
	}
	for _, cmd := range attacks {
		if argv, err := validateReadOnlyCommand(cmd); err == nil {
			t.Errorf("must reject %q, got argv %v", cmd, argv)
		}
	}
}

func TestValidateReadOnlyCommandRejectsWriteCommands(t *testing.T) {
	// The old blocklist covered most of these, but not e.g. `truncate`,
	// `shred`, `npm`, or `git push`.
	for _, cmd := range []string{
		"rm -rf /", "mv a b", "dd if=/dev/zero of=/dev/sda", "mkfs.ext4 /dev/sda",
		"truncate -s 0 file", "shred -u file", "git push", "git commit -m x",
		"git checkout .", "npm install", "curl http://evil", "wget http://evil",
		"python -c 'import os;os.system(\"rm -rf /\")'",
	} {
		if argv, err := validateReadOnlyCommand(cmd); err == nil {
			t.Errorf("must reject %q, got argv %v", cmd, argv)
		}
	}
}

func TestValidateReadOnlyCommandAllowsReadOnly(t *testing.T) {
	for _, cmd := range []string{
		"ls -la",
		"cat README.md",
		"grep -rn TODO .",
		"git status",
		"git log --oneline -10",
		"git diff HEAD~1",
		`grep -n "func main" main.go`,
		"wc -l internal/auth/oauth/oauth_service.go",
	} {
		if argv, err := validateReadOnlyCommand(cmd); err != nil {
			t.Errorf("must allow %q, got %v", cmd, err)
		} else if len(argv) == 0 {
			t.Errorf("%q produced an empty argv", cmd)
		}
	}
}

func TestSplitCommandDoesNotExpand(t *testing.T) {
	// The tokenizer must be literal: no globbing, no variable expansion, no
	// command substitution. These stay as single literal arguments.
	args, err := splitCommand(`echo "$HOME" '*' a\ b`)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	// 4 args: echo | "$HOME" | "*" | "a b" (the backslash escapes the space).
	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d: %q", len(args), args)
	}
	want := []string{"echo", "$HOME", "*", "a b"}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("arg %d = %q, want %q (full: %q)", i, args[i], want[i], args)
		}
	}
}

// A binary that only reads is not enough: its flags decide what it does.
// `find -delete` and `find -exec` write and execute, so the flag check has to
// run before the allowlist admits the binary at all.
func TestValidateReadOnlyCommandRejectsWritingAndExecutingFlags(t *testing.T) {
	rejected := []string{
		"find . -delete",
		"find . -name *.go -exec rm {} ;",
		"find . -execdir sh -c id \\;",
		"find . -ok cat {} \\;",
		"find . -fprintf out.txt %p",
		"diff -u a b --output=patch.diff",
		"diff --output patch.diff a b",
		"git diff --output=leak.patch",
		"git log -p --ext-diff",
	}
	for _, cmd := range rejected {
		t.Run(cmd, func(t *testing.T) {
			if _, err := validateReadOnlyCommand(cmd); err == nil {
				t.Fatalf("expected %q to be rejected, but it was allowed", cmd)
			}
		})
	}
}

// git subcommands that mutate were reachable through an allowed verb. They have
// to be rejected on the subcommand itself, not on their arguments.
func TestValidateReadOnlyCommandRejectsMutatingGitSubcommands(t *testing.T) {
	rejected := []string{
		"git remote remove origin",
		"git remote add upstream https://example.test/x.git",
		"git remote set-url origin https://example.test/x.git",
		"git branch -D main",
		"git branch new-feature",
		"git config user.name attacker",
		"git stash",
		"git clean -fd",
		"git checkout main",
		"git commit -m x",
		"git push",
	}
	for _, cmd := range rejected {
		t.Run(cmd, func(t *testing.T) {
			if _, err := validateReadOnlyCommand(cmd); err == nil {
				t.Fatalf("expected %q to be rejected, but it was allowed", cmd)
			}
		})
	}
}

// The allowlist must keep admitting the read-only invocations it exists for, so
// the flag check cannot be tightened into uselessness.
func TestValidateReadOnlyCommandStillAllowsReadOnlyInvocations(t *testing.T) {
	allowed := []string{
		"ls -la",
		"git status",
		"git log --oneline -20",
		"git diff HEAD~1..HEAD",
		"git show abc123",
		"git rev-parse HEAD",
		"git ls-files",
		"find . -name '*.go' -type f",
		"grep -rn TODO internal/",
		"cat go.mod",
		"wc -l main.go",
	}
	for _, cmd := range allowed {
		t.Run(cmd, func(t *testing.T) {
			args, err := validateReadOnlyCommand(cmd)
			if err != nil {
				t.Fatalf("expected %q to be allowed, got %v", cmd, err)
			}
			if len(args) == 0 {
				t.Fatalf("expected an argument vector for %q", cmd)
			}
		})
	}
}

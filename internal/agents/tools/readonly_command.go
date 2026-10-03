package tools

import (
	"fmt"
	"path"
	"strings"
)

// This replaces a blocklist of destructive commands (`rm`, `mv`, `dd`, ...).
// A blocklist is trivially defeated: `cat x; rm -rf /`, `echo hi && rm -rf /`
// and `$(rm -rf /)` all start with an allowed word, and the command was
// passed to `sh -c`, so every shell metacharacter was live. An allowlist of
// binaries plus no shell is what actually makes it read-only.
//
// AUDIT_REMEDIATION.md: gosec G702, command injection.
var commandAllowlist = map[string]bool{
	// inspection
	"ls": true, "cat": true, "head": true, "tail": true, "wc": true,
	"pwd": true, "echo": true, "file": true, "stat": true, "du": true,
	// search
	"grep": true, "egrep": true, "fgrep": true, "rg": true, "find": true, "which": true,
	// diff
	"diff": true, "cmp": true,
	// git, read-only subcommands only (enforced separately)
	"git": true,
}

// gitReadOnlySubcommands are the only git operations permitted. `git` can write
// (commit, push, checkout, clean, config) so the binary alone is not safe.
//
// `branch` and `remote` are deliberately absent even though a bare
// `git branch` or `git remote -v` only reads: `git branch -D x` deletes a
// branch, `git branch new` creates one, `git remote remove origin` rewrites
// .git/config, and the subcommand is the only thing this check looks at, so
// those mutations would otherwise ride in on an allowed verb.
var gitReadOnlySubcommands = map[string]bool{
	"status": true, "log": true, "diff": true, "show": true, "blame": true,
	"rev-parse": true, "ls-files": true, "ls-tree": true, "shortlog": true,
	"describe": true, "cat-file": true, "grep": true,
}

// dangerousFlags lists flags that turn an otherwise read-only binary into a
// writer or an executor, keyed by binary.
//
//	find  -exec/-execdir/-ok/-okdir run another command; -delete removes
//	       matches; -fprint/-fprintf/-fls write to a file.
//	diff / git  --output=<file> (and -o) write the result to disk.
//	git  --ext-diff runs the user's configured external diff driver, which is
//	       arbitrary command execution.
var dangerousFlags = map[string][]string{
	"find": {"-delete", "-exec", "-execdir", "-ok", "-okdir", "-fprint", "-fprintf", "-fls"},
	"diff": {"--output", "-o"},
	"git":  {"--output", "-o", "--ext-diff"},
}

// splitCommand tokenizes a command line into argv, honouring single and double
// quotes and backslash escapes. It performs no variable expansion, globbing or
// command substitution, so the result is a literal argument vector.
func splitCommand(s string) ([]string, error) {
	var (
		args    []string
		cur     strings.Builder
		inWord  bool
		quote   rune
		escaped bool
	)
	flush := func() {
		if inWord {
			args = append(args, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			inWord = true
			escaped = false
		case r == '\\' && quote != '\'':
			escaped = true
			inWord = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inWord = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote in command")
	}
	if escaped {
		return nil, fmt.Errorf("trailing backslash in command")
	}
	flush()
	return args, nil
}

// validateReadOnlyCommand returns the argument vector when the command is a
// single allowed read-only invocation, and an error otherwise.
//
// Rejecting anything with shell metacharacters up front is belt-and-braces:
// the executor never uses a shell, so they would be inert, but refusing them
// means a caller cannot smuggle one past a future refactor.
func validateReadOnlyCommand(s string) ([]string, error) {
	if strings.ContainsAny(s, ";|&$`\n") {
		return nil, fmt.Errorf("command contains shell metacharacters, which are never needed in a read-only sandbox")
	}
	args, err := splitCommand(s)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("command is empty")
	}
	bin := path.Base(args[0])
	if !commandAllowlist[bin] {
		return nil, fmt.Errorf("command %q is not permitted in a read-only sandbox", bin)
	}
	for _, flag := range dangerousFlags[bin] {
		for _, a := range args[1:] {
			// Covers both "-delete" and "-delete=..." / "--output=file".
			if a == flag || strings.HasPrefix(a, flag+"=") {
				return nil, fmt.Errorf("%s flag %q is not permitted in a read-only sandbox", bin, flag)
			}
		}
	}
	if bin == "git" {
		sub := ""
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "-") {
				continue // global flags such as -C, --no-pager
			}
			sub = a
			break
		}
		if sub == "" {
			return nil, fmt.Errorf("git requires a subcommand")
		}
		allowed, listed := gitReadOnlySubcommands[sub]
		if !listed || !allowed {
			return nil, fmt.Errorf("git %s is not permitted in a read-only sandbox", sub)
		}
	}
	return args, nil
}

package pathguard_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/pathguard"
)

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("abs %q: %v", p, err)
	}
	return abs
}

// A plain relative join must work, and must come back absolute and cleaned.
func TestResolveAllowsOrdinaryRelativePaths(t *testing.T) {
	base := t.TempDir()

	got, err := pathguard.Resolve(base, "hooks", "pre-commit")
	if err != nil {
		t.Fatalf("expected a valid path, got %v", err)
	}
	want := filepath.Join(mustAbs(t, base), "hooks", "pre-commit")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if !filepath.IsAbs(got) {
		t.Fatal("Resolve must return an absolute path")
	}
}

// The base itself is a legitimate result.
func TestResolveAllowsTheBaseItself(t *testing.T) {
	base := t.TempDir()
	got, err := pathguard.Resolve(base)
	if err != nil {
		t.Fatalf("expected the base to be allowed, got %v", err)
	}
	if got != mustAbs(t, base) {
		t.Fatalf("got %q, want %q", got, mustAbs(t, base))
	}
}

// The escape that matters most: filepath.Join cleans `..`, so without a
// containment check `/repo` + `../../etc/cron.d` silently becomes
// `/etc/cron.d`.
func TestResolveRejectsDotDotEscape(t *testing.T) {
	base := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := [][]string{
		{"..", "etc"},
		{"..", "..", "etc"},
		{"hooks", "..", "..", "escape"},
		{"a/b/../../../escape"},
		{"./../escape"},
	}
	for _, elems := range cases {
		t.Run(strings.Join(elems, "/"), func(t *testing.T) {
			got, err := pathguard.Resolve(base, elems...)
			if err == nil {
				t.Fatalf("expected %v to be rejected, got %q", elems, got)
			}
			if !pathguard.IsEscape(err) {
				t.Fatalf("expected an escape error, got %T: %v", err, err)
			}
		})
	}
}

// An absolute component is not an escape after Join (it becomes base-relative)
// but it always signals a caller bug, so it is refused rather than silently
// reinterpreted.
func TestResolveRejectsAbsoluteComponent(t *testing.T) {
	base := t.TempDir()
	for _, elem := range []string{"/etc/passwd", "/", "/tmp"} {
		t.Run(elem, func(t *testing.T) {
			if _, err := pathguard.Resolve(base, elem); err == nil {
				t.Fatalf("expected absolute component %q to be rejected", elem)
			}
		})
	}
}

// Empty components are how filepath.Join is normally called with optional
// pieces; they must not be treated as an error or as an escape.
func TestResolveIgnoresEmptyComponents(t *testing.T) {
	base := t.TempDir()
	got, err := pathguard.Resolve(base, "", "hooks", "")
	if err != nil {
		t.Fatalf("empty components must be ignored, got %v", err)
	}
	if got != filepath.Join(mustAbs(t, base), "hooks") {
		t.Fatalf("got %q", got)
	}
}

// The check that a naive filepath.Rel test gets wrong: the base contains a
// symlink directory pointing outside, and the leaf does not exist yet. This is
// the git-hooks case -- scandrix writes pre-commit into a hooks dir reached
// through a path an untrusted repository controls.
func TestResolveRejectsSymlinkedDirectoryEscapeForNotYetCreatedLeaf(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	link := filepath.Join(root, "hooks")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	// The leaf does NOT exist. EvalSymlinks on the full path would fail here,
	// so a check that only evaluates the leaf would pass this through.
	got, err := pathguard.Resolve(root, "hooks", "pre-commit")
	if err == nil {
		t.Fatalf("expected the symlinked parent to be rejected, got %q", got)
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected the error to name the symlink, got %v", err)
	}
}

func TestResolveRejectsSymlinkedLeafEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "evil")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	if _, err := pathguard.Resolve(root, "evil"); err == nil {
		t.Fatal("expected a symlinked leaf pointing outside to be rejected")
	}
}

// A symlink that stays inside the base is fine. Over-rejecting would break
// legitimate layouts, such as a hooks dir symlinked to a sibling dir in the
// same repository.
func TestResolveAllowsSymlinkThatStaysInsideBase(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "real", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "alias")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	got, err := pathguard.Resolve(root, "alias", "hooks", "pre-commit")
	if err != nil {
		t.Fatalf("expected an internal symlink to be allowed, got %v", err)
	}
	if !strings.HasPrefix(got, mustAbs(t, root)) {
		t.Fatalf("got %q, expected it under %q", got, root)
	}
}

// A base that is itself a symlink must not cause false rejections.
func TestResolveAllowsSymlinkedBase(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "realbase")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linkbase")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	got, err := pathguard.Resolve(link, "hooks")
	if err != nil {
		t.Fatalf("expected a symlinked base to be allowed, got %v", err)
	}
	// The result keeps the path the caller used (through the link) rather than
	// the resolved target: both are inside the base, and preserving the caller's
	// spelling means error messages match what the user typed.
	want := filepath.Join(mustAbs(t, link), "hooks")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// A base that does not exist yet cannot be escaped from.
func TestResolveAllowsMissingBase(t *testing.T) {
	base := filepath.Join(t.TempDir(), "not", "created", "yet")
	got, err := pathguard.Resolve(base, "a", "b")
	if err != nil {
		t.Fatalf("expected a missing base to be allowed, got %v", err)
	}
	if got != filepath.Join(mustAbs(t, base), "a", "b") {
		t.Fatalf("got %q", got)
	}
}

func TestResolveRejectsEmptyBase(t *testing.T) {
	if _, err := pathguard.Resolve(""); err == nil {
		t.Fatal("expected an empty base to be rejected")
	}
}

// ResolveExisting must distinguish "not a valid path" from "valid but absent".
func TestResolveExistingDistinguishesAbsentFromEscape(t *testing.T) {
	base := t.TempDir()

	if _, err := pathguard.ResolveExisting(base, "nope.txt"); err == nil {
		t.Fatal("expected an absent path to error")
	} else if pathguard.IsEscape(err) {
		t.Fatalf("an absent path is not an escape: %v", err)
	}

	if err := os.WriteFile(filepath.Join(base, "here.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := pathguard.ResolveExisting(base, "here.txt")
	if err != nil {
		t.Fatalf("expected an existing path to resolve, got %v", err)
	}
	if got != filepath.Join(mustAbs(t, base), "here.txt") {
		t.Fatalf("got %q", got)
	}

	if _, err := pathguard.ResolveExisting(base, "../escape.txt"); err == nil {
		t.Fatal("expected an escape to be rejected")
	} else if !pathguard.IsEscape(err) {
		t.Fatalf("expected an escape error, got %v", err)
	}
}

// The error must name the offending path, because these surface in CLI output
// where the user has to know which input was refused.
func TestEscapeErrorMentionsCandidateAndBase(t *testing.T) {
	base := t.TempDir()
	_, err := pathguard.Resolve(base, "..", "escape")
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "escape") {
		t.Errorf("error should name the candidate: %q", msg)
	}
	if !strings.Contains(msg, filepath.Base(base)) && !strings.Contains(msg, base) {
		t.Errorf("error should name the base: %q", msg)
	}
}

func TestResolveUnderAcceptsContainedAbsolutePath(t *testing.T) {
	base := t.TempDir()
	candidate := filepath.Join(base, "a", "b")
	got, err := pathguard.ResolveUnder(base, candidate)
	if err != nil {
		t.Fatalf("expected a contained path to be accepted, got %v", err)
	}
	if got != filepath.Clean(candidate) {
		t.Fatalf("got %q, want %q", got, filepath.Clean(candidate))
	}
}

func TestResolveUnderRejectsOutsideAndRelative(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere")

	if _, err := pathguard.ResolveUnder(base, outside); err == nil {
		t.Fatal("expected a path outside the base to be rejected")
	} else if !pathguard.IsEscape(err) {
		t.Fatalf("expected an escape error, got %v", err)
	}

	if _, err := pathguard.ResolveUnder(base, filepath.Join(base, "..", "escape")); err == nil {
		t.Fatal("expected a traversing path to be rejected")
	}

	if _, err := pathguard.ResolveUnder(base, "relative/path"); err == nil {
		t.Fatal("expected a relative candidate to be rejected")
	}

	if _, err := pathguard.ResolveUnder(base, ""); err == nil {
		t.Fatal("expected an empty candidate to be rejected")
	}
}

// A candidate reached through a symlink that leaves the base must be rejected
// even though its literal prefix matches the base.
func TestResolveUnderRejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(base, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if _, err := pathguard.ResolveUnder(base, filepath.Join(link, "payload")); err == nil {
		t.Fatal("expected a symlinked escape to be rejected")
	}
}

// ResolvePath is the dispatcher callers should reach for. It must behave the
// same whether the candidate arrives absolute or relative, because picking the
// wrong function fails in opposite directions: Resolve on an absolute path
// silently reinterprets it as root-relative.
func TestResolvePathAcceptsBothForms(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(mustAbs(t, root), "sub", "file.txt")

	rel, err := pathguard.ResolvePath(root, filepath.Join("sub", "file.txt"))
	if err != nil {
		t.Fatalf("relative form: %v", err)
	}
	if rel != want {
		t.Errorf("relative form: got %q, want %q", rel, want)
	}

	abs, err := pathguard.ResolvePath(root, want)
	if err != nil {
		t.Fatalf("absolute form: %v", err)
	}
	if abs != want {
		t.Errorf("absolute form: got %q, want %q", abs, want)
	}

	if _, err := pathguard.ResolvePath(root, filepath.Join(root, "..", "escape")); err == nil {
		t.Error("expected an absolute escape to be rejected")
	}
	if _, err := pathguard.ResolvePath(root, filepath.Join("..", "escape")); err == nil {
		t.Error("expected a relative escape to be rejected")
	}
}

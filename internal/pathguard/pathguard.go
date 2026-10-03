// Package pathguard resolves filesystem paths that are joined from a trusted
// base plus one or more untrusted components, and guarantees the result cannot
// escape the base.
//
// # Why this exists in one place
//
// Every caller here joins a path from two very different sources: a base the
// process legitimately owns (the repository the user is standing in, the state
// directory it created, the migration directory it shipped) and a component
// that came from outside the program. That component is variously a branch
// name, a file read out of a `.git` file, a directory entry, a session id, a
// user-supplied relative path, or an environment variable. filepath.Join cleans
// `..` segments, so it will happily produce `/etc/cron.d/x` from a base of
// `/repo` and a component of `../../etc/cron.d`, and nothing downstream notices.
//
// Three escape routes have to be closed, and all three are covered here:
//
//  1. `..` in the joined components. Caught by the containment check, which
//     compares the cleaned result against the base.
//  2. An absolute component. `filepath.Join(base, "/etc/passwd")` yields
//     `/base/etc/passwd`, so it cannot escape, but it is still rejected: a
//     caller passing an absolute path has made a mistake worth surfacing.
//  3. A symlink *inside* the base pointing outside it. This is the one that a
//     naive `filepath.Rel` check misses, and the one that matters most for the
//     git-hook installers: the hooks directory is routinely reached through a
//     symlink, and EvalSymlinks on a path that does not exist yet fails
//     silently, which would skip the check exactly when it is needed.
//
// This package deliberately has no dependency on the rest of the codebase so
// that low-level CLI code, HTTP controllers and the license loader can all use
// it without creating import cycles.
package pathguard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrEscapes is returned when a candidate path resolves outside its base. It is
// exported so callers can distinguish "you tried to escape" from "that path
// does not exist", which are different failures with different fixes.
type ErrEscapes struct {
	Base      string
	Candidate string
	Reason    string
}

func (e *ErrEscapes) Error() string {
	return fmt.Sprintf("path %q escapes %q: %s", e.Candidate, e.Base, e.Reason)
}

// IsEscape reports whether err is an escape rejection, so callers can branch on
// the cause without matching on the concrete type.
func IsEscape(err error) bool {
	_, ok := err.(*ErrEscapes)
	return ok
}

// Resolve joins elems onto base and returns the cleaned absolute path, only if
// the result stays inside base after following every symlink that already
// exists along the way. The path does not need to exist.
//
// The returned path is absolute and cleaned, so it is safe to hand to
// os.ReadFile, os.WriteFile, os.Remove or os.MkdirAll.
func Resolve(base string, elems ...string) (string, error) {
	if base == "" {
		return "", fmt.Errorf("base directory is empty")
	}
	for _, e := range elems {
		if e == "" {
			continue
		}
		if filepath.IsAbs(e) {
			return "", &ErrEscapes{
				Base:      base,
				Candidate: e,
				Reason:    "an absolute path cannot be resolved inside a base directory",
			}
		}
	}

	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolving base %q: %w", base, err)
	}

	joined := filepath.Clean(filepath.Join(append([]string{absBase}, elems...)...))

	if err := ensureWithin(absBase, joined, strings.Join(elems, string(filepath.Separator))); err != nil {
		return "", err
	}
	if err := ensureNoSymlinkEscape(absBase, joined); err != nil {
		return "", err
	}
	return joined, nil
}

// ResolveExisting is Resolve for a candidate that already exists. It fails if
// the path is absent, which lets a caller distinguish "would be a valid path"
// from "is a valid path".
func ResolveExisting(base, candidate string) (string, error) {
	resolved, err := Resolve(base, candidate)
	if err != nil {
		return "", err
	}
	if _, statErr := os.Lstat(resolved); statErr != nil {
		return "", fmt.Errorf("resolving %q: %w", candidate, statErr)
	}
	return resolved, nil
}

// ResolveUnder verifies that an already-absolute candidate lies inside base, and
// returns it cleaned. Use this when the candidate was produced elsewhere -- read
// out of a `.git` file, returned by a library, built from a URL -- and the only
// question is whether it may be used.
//
// Unlike Resolve, candidate is not joined onto base; it is checked against it.
func ResolveUnder(base, candidate string) (string, error) {
	if base == "" {
		return "", fmt.Errorf("base directory is empty")
	}
	if candidate == "" {
		return "", &ErrEscapes{Base: base, Candidate: candidate, Reason: "path is empty"}
	}

	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolving base %q: %w", base, err)
	}
	cleaned := filepath.Clean(candidate)
	if !filepath.IsAbs(cleaned) {
		return "", &ErrEscapes{Base: absBase, Candidate: candidate, Reason: "candidate is not an absolute path"}
	}

	if err := ensureWithin(absBase, cleaned, candidate); err != nil {
		return "", err
	}
	if err := ensureNoSymlinkEscape(absBase, cleaned); err != nil {
		return "", err
	}
	return cleaned, nil
}

// ResolvePath confines candidate to root, accepting either form.
//
// A relative candidate is joined onto root; an absolute one is checked against
// root as-is. Both are rejected if the result would leave root. Callers that
// receive a path from outside the program should prefer this over choosing
// between Resolve and ResolveUnder themselves, because picking wrong fails in
// opposite directions: Resolve on an absolute path is silently reinterpreted
// as root-relative, which hides the caller's intent.
func ResolvePath(root, candidate string) (string, error) {
	if filepath.IsAbs(candidate) {
		return ResolveUnder(root, candidate)
	}
	return Resolve(root, candidate)
}

// ensureWithin is the cheap, symlink-agnostic containment check. It runs first
// so a plain `..` escape is rejected before any filesystem access.
func ensureWithin(absBase, cleaned, candidate string) error {
	rel, err := filepath.Rel(absBase, cleaned)
	if err != nil {
		// Rel fails only when the two paths cannot be related at all, which on
		// any real system means different volumes. Treat as an escape.
		return &ErrEscapes{Base: absBase, Candidate: candidate, Reason: "path is not relative to the base directory"}
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return &ErrEscapes{Base: absBase, Candidate: candidate, Reason: "relative path traverses above the base directory"}
	}
	return nil
}

// ensureNoSymlinkEscape resolves the deepest existing ancestor of cleaned and
// re-checks containment against the resolved base.
//
// Evaluating only the ancestor that exists is what makes this correct for
// write-then-create flows: EvalSymlinks(cleaned) fails when the leaf is absent,
// which would silently skip the check for exactly the case where the hook
// installer is about to create a file.
func ensureNoSymlinkEscape(absBase, cleaned string) error {
	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		// A base that does not exist yet cannot be escaped from: there is
		// nothing under it to traverse into.
		return nil
	}

	// Walk down from the base, keeping the longest prefix that exists on disk.
	probe := cleaned
	var existing string
	for {
		if _, statErr := os.Lstat(probe); statErr == nil {
			existing = probe
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			// Reached the filesystem root without finding anything that exists.
			return nil
		}
		probe = parent
		// Stop once we are back at the base: it exists by definition.
		if probe == absBase || len(probe) < len(absBase) {
			break
		}
	}
	if existing == "" {
		return nil
	}

	resolvedExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return fmt.Errorf("resolving symlinks for %q: %w", cleaned, err)
	}

	rel, err := filepath.Rel(realBase, resolvedExisting)
	if err != nil {
		return &ErrEscapes{Base: absBase, Candidate: cleaned, Reason: "resolved path is not relative to the resolved base directory"}
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return &ErrEscapes{
			Base:      absBase,
			Candidate: cleaned,
			Reason:    "a symlink on the path points outside the base directory (resolves to " + resolvedExisting + ")",
		}
	}
	return nil
}

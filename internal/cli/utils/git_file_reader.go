// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	MaxDiffCharacters    = 500_000   // 500K characters max diff per file
	MaxContentCharacters = 2_000_000 // 2M characters max content per file
	BinaryCheckBufferSize = 8192
)

// SafeRepoFileReader reads repository files safely with security and boundary checks.
type SafeRepoFileReader struct {
	repoRoot string
}

// NewSafeRepoFileReader creates a reader scoped to a specific repository root.
func NewSafeRepoFileReader(repoRoot string) (*SafeRepoFileReader, error) {
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed resolving repository path: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("repository root does not exist: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("repository root is not a directory: %s", abs)
	}

	return &SafeRepoFileReader{repoRoot: abs}, nil
}

// ValidateRelativePath ensures relPath does not escape the repository boundary.
func (r *SafeRepoFileReader) ValidateRelativePath(relPath string) (string, error) {
	cleanRel := filepath.Clean(relPath)
	if strings.HasPrefix(cleanRel, "..") || filepath.IsAbs(cleanRel) {
		return "", fmt.Errorf("path escapes repository root: %s", relPath)
	}

	target := filepath.Join(r.repoRoot, cleanRel)
	// Check for symlink escaping
	evalTarget, err := filepath.EvalSymlinks(target)
	if err == nil {
		evalRoot, errRoot := filepath.EvalSymlinks(r.repoRoot)
		if errRoot == nil {
			relCheck, errRel := filepath.Rel(evalRoot, evalTarget)
			if errRel != nil || strings.HasPrefix(relCheck, "..") {
				return "", fmt.Errorf("symlink target escapes repository root: %s -> %s", relPath, evalTarget)
			}
		}
	}

	return target, nil
}

// IsBinaryFile inspects the leading bytes of a file to check for binary/non-text content.
func (r *SafeRepoFileReader) IsBinaryFile(relPath string) (bool, error) {
	target, err := r.ValidateRelativePath(relPath)
	if err != nil {
		return false, err
	}

	f, err := os.Open(target)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, BinaryCheckBufferSize)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return false, nil // Empty file is treated as text
	}

	return IsBinaryContent(buf[:n]), nil
}

// IsBinaryContent checks if a byte slice contains null bytes or non-text control codes.
func IsBinaryContent(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	// Null byte indicates binary
	if bytes.IndexByte(data, 0) != -1 {
		return true
	}
	// Check UTF-8 validity
	if !utf8.Valid(data) {
		return true
	}
	return false
}

// ReadFileContent reads file contents as UTF-8 string with size limit protection.
func (r *SafeRepoFileReader) ReadFileContent(relPath string) (string, error) {
	target, err := r.ValidateRelativePath(relPath)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("cannot read directory as file: %s", relPath)
	}

	// Size limit check (2MB)
	if info.Size() > MaxContentCharacters {
		return "", fmt.Errorf("file %s exceeds maximum content limit of %d bytes", relPath, MaxContentCharacters)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		return "", err
	}

	if IsBinaryContent(data) {
		return "", fmt.Errorf("file %s contains binary data", relPath)
	}

	return string(data), nil
}

// ReadFileAtRef reads file content at a specific Git ref using git show <ref>:<path>.
func (r *SafeRepoFileReader) ReadFileAtRef(ctx context.Context, ref, relPath string) (string, error) {
	if _, err := r.ValidateRelativePath(relPath); err != nil {
		return "", err
	}

	spec := fmt.Sprintf("%s:%s", ref, filepath.ToSlash(relPath))
	cmd := exec.CommandContext(ctx, "git", "show", spec)
	cmd.Dir = r.repoRoot

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git show %s failed: %w", spec, err)
	}

	if len(out) > MaxContentCharacters {
		return "", fmt.Errorf("file at %s exceeds maximum content limit of %d bytes", spec, MaxContentCharacters)
	}

	if IsBinaryContent(out) {
		return "", fmt.Errorf("file at %s contains binary data", spec)
	}

	return string(out), nil
}

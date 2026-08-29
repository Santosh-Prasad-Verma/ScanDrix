package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidatePathContainment asserts that targetRelPath resides strictly within sandboxRoot.
// It guards against directory traversal (../), absolute root injections, and symlink breakout.
func ValidatePathContainment(sandboxRoot, targetRelPath string) (string, error) {
	cleanRoot := filepath.Clean(sandboxRoot)
	
	// Ensure target is relative and not root injection
	if filepath.IsAbs(targetRelPath) {
		return "", fmt.Errorf("absolute path injection prohibited: %s", targetRelPath)
	}

	joined := filepath.Join(cleanRoot, targetRelPath)
	cleanTarget := filepath.Clean(joined)

	// Prefix check: cleanTarget must start with cleanRoot + separator
	expectedPrefix := cleanRoot + string(filepath.Separator)
	if cleanTarget != cleanRoot && !strings.HasPrefix(cleanTarget, expectedPrefix) {
		return "", fmt.Errorf("directory traversal attack detected: target '%s' escapes root '%s'", targetRelPath, sandboxRoot)
	}

	// Symlink resolution check (if target exists)
	if realPath, err := filepath.EvalSymlinks(cleanTarget); err == nil {
		cleanRealRoot, _ := filepath.EvalSymlinks(cleanRoot)
		if cleanRealRoot == "" {
			cleanRealRoot = cleanRoot
		}
		expectedRealPrefix := cleanRealRoot + string(filepath.Separator)
		if realPath != cleanRealRoot && !strings.HasPrefix(realPath, expectedRealPrefix) {
			return "", fmt.Errorf("symlink breakout attack detected: real path '%s' escapes '%s'", realPath, cleanRealRoot)
		}
		return realPath, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("path evaluation error: %w", err)
	}

	return cleanTarget, nil
}

// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package quickfix

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scandrix/backend/internal/pathguard"
)

// QuickFixResult documents the result of applying a fix.
type QuickFixResult struct {
	FilePath     string `json:"file_path"`
	Success      bool   `json:"success"`
	BackupPath   string `json:"backup_path,omitempty"`
	ErrorMsg     string `json:"error_msg,omitempty"`
	HunksApplied int    `json:"hunks_applied"`
}

// ApplyPatchToFile applies all hunks of a FilePatch to the specified target path.
//
// root bounds the write. The target path originates in model output, so without
// this a suggestion naming `../../.ssh/authorized_keys` would be applied
// verbatim: this function reads the file, writes a `.bak` beside it, and
// renames a temporary file over it.
func ApplyPatchToFile(root, targetPath string, patch FilePatch, createBackup bool) (*QuickFixResult, error) {
	result := &QuickFixResult{FilePath: targetPath}

	targetPath, err := pathguard.ResolvePath(root, targetPath)
	if err != nil {
		result.ErrorMsg = err.Error()
		return result, fmt.Errorf("refusing to patch %q: %w", targetPath, err)
	}

	contentBytes, err := os.ReadFile(targetPath)
	if err != nil {
		result.ErrorMsg = err.Error()
		return result, fmt.Errorf("failed reading file %s: %w", targetPath, err)
	}

	if createBackup {
		backupPath := targetPath + ".bak"
		if err := os.WriteFile(backupPath, contentBytes, 0600); err == nil { // #nosec G703 -- targetPath is re-resolved by pathguard.ResolvePath at the top of the function
			result.BackupPath = backupPath
		}
	}

	lines := strings.Split(string(contentBytes), "\n")
	appliedCount := 0

	for _, hunk := range patch.Hunks {
		updated, applyErr := ApplyHunkToLines(lines, hunk)
		if applyErr != nil {
			result.ErrorMsg = applyErr.Error()
			return result, fmt.Errorf("failed applying hunk to %s: %w", targetPath, applyErr)
		}
		lines = updated
		appliedCount++
	}

	newContent := strings.Join(lines, "\n")
	// Atomic write
	tempFile := filepath.Join(filepath.Dir(targetPath), fmt.Sprintf(".tmp_%s", filepath.Base(targetPath)))
	if err := os.WriteFile(tempFile, []byte(newContent), 0600); err != nil { // #nosec G703 -- backupPath is derived from the already-confined targetPath
		result.ErrorMsg = err.Error()
		return result, fmt.Errorf("failed writing temporary file: %w", err)
	}

	if err := os.Rename(tempFile, targetPath); err != nil {
		_ = os.Remove(tempFile)
		result.ErrorMsg = err.Error()
		return result, fmt.Errorf("failed replacing target file: %w", err)
	}

	result.Success = true
	result.HunksApplied = appliedCount
	return result, nil
}

// ApplySuggestedDiff applies a unified diff or replacement block directly to a
// file. root bounds the write, for the same reason as ApplyPatchToFile.
func ApplySuggestedDiff(root, targetPath string, diffText string, createBackup bool) (*QuickFixResult, error) {
	patches, err := ParseUnifiedDiff(diffText)
	if err == nil && len(patches) > 0 && len(patches[0].Hunks) > 0 {
		return ApplyPatchToFile(root, targetPath, patches[0], createBackup)
	}

	// Fallback for non-standard unified diffs: simple search and replace of clean lines
	targetPath, err = pathguard.ResolvePath(root, targetPath)
	if err != nil {
		return &QuickFixResult{FilePath: targetPath, ErrorMsg: err.Error()},
			fmt.Errorf("refusing to patch %q: %w", targetPath, err)
	}

	contentBytes, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, err
	}

	orig := string(contentBytes)
	lines := strings.Split(diffText, "\n")
	var oldLines []string
	var newLines []string

	for _, l := range lines {
		if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
			oldLines = append(oldLines, strings.TrimPrefix(l, "-"))
		} else if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			newLines = append(newLines, strings.TrimPrefix(l, "+"))
		}
	}

	if len(oldLines) > 0 && len(newLines) > 0 {
		oldBlock := strings.Join(oldLines, "\n")
		newBlock := strings.Join(newLines, "\n")
		if strings.Contains(orig, oldBlock) {
			if createBackup {
				_ = os.WriteFile(targetPath+".bak", contentBytes, 0600) // #nosec G703 -- targetPath is re-resolved by pathguard.ResolvePath at the top of the function
			}
			updated := strings.Replace(orig, oldBlock, newBlock, 1)
			_ = os.WriteFile(targetPath, []byte(updated), 0600) // #nosec G703 -- targetPath is re-resolved by pathguard.ResolvePath at the top of the function
			return &QuickFixResult{
				FilePath:     targetPath,
				Success:      true,
				HunksApplied: 1,
			}, nil
		}
	}

	return nil, fmt.Errorf("could not parse or apply suggested diff to %s", targetPath)
}

// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License").

package scan

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

var defaultExcludedDirs = map[string]bool{
	".git":         true,
	".github":      false, // inspect workflows if needed
	".svn":         true,
	".hg":          true,
	"node_modules": true,
	"vendor":       true,
	".next":        true,
	"dist":         true,
	"build":        true,
	"target":       true,
	"bin":          true,
	"obj":          true,
	".venv":        true,
	"venv":         true,
	"__pycache__":  true,
	".turbo":       true,
	"coverage":     true,
	".cache":       true,
	".idea":        true,
	".vscode":      true,
}

var languageExtensions = map[string]string{
	".go":    "go",
	".ts":    "typescript",
	".tsx":   "typescript-react",
	".js":    "javascript",
	".jsx":   "javascript-react",
	".py":    "python",
	".rs":    "rust",
	".java":  "java",
	".c":     "c",
	".cpp":   "cpp",
	".cc":    "cpp",
	".h":     "c-header",
	".hpp":   "cpp-header",
	".rb":    "ruby",
	".php":   "php",
	".cs":    "csharp",
	".swift": "swift",
	".kt":    "kotlin",
	".sql":   "sql",
	".sh":    "bash",
	".yaml":  "yaml",
	".yml":   "yaml",
	".json":  "json",
	".toml":  "toml",
	".proto": "protobuf",
}

// CollectFiles recursively scans rootDir according to ScanTarget rules.
func CollectFiles(target ScanTarget) ([]ScannedFile, error) {
	rootAbs, err := filepath.Abs(target.RootDir)
	if err != nil {
		return nil, err
	}

	maxSize := target.MaxFileSize
	if maxSize <= 0 {
		maxSize = 1048576 // 1MB default
	}

	langSet := make(map[string]bool)
	for _, l := range target.LanguageFilters {
		langSet[strings.ToLower(strings.TrimSpace(l))] = true
	}

	excludeSet := make(map[string]bool)
	for _, ex := range target.ExcludePaths {
		excludeSet[filepath.Clean(ex)] = true
	}

	var files []ScannedFile

	err = filepath.WalkDir(rootAbs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		name := d.Name()
		if d.IsDir() {
			if defaultExcludedDirs[name] {
				return filepath.SkipDir
			}
			rel, relErr := filepath.Rel(rootAbs, path)
			if relErr == nil && excludeSet[filepath.Clean(rel)] {
				return filepath.SkipDir
			}
			return nil
		}

		// File filtering
		relPath, relErr := filepath.Rel(rootAbs, path)
		if relErr != nil {
			return nil
		}
		relClean := filepath.Clean(relPath)
		if excludeSet[relClean] {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(name))
		lang, ok := languageExtensions[ext]
		if !ok {
			return nil
		}

		if len(langSet) > 0 && !langSet[lang] {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}

		if info.Size() > maxSize || info.Size() == 0 {
			return nil
		}

		// Read file content and verify it is not binary
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		if isBinary(data) {
			return nil
		}

		lines := bytes.Count(data, []byte("\n"))
		if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
			lines++
		}

		// Conservative token estimation: ~4 chars per token + structural bonus
		tokens := (len(data) / 4) + (lines * 2)

		files = append(files, ScannedFile{
			RelativePath:    filepath.ToSlash(relClean),
			AbsolutePath:    path,
			Language:        lang,
			LineCount:       lines,
			ByteSize:        info.Size(),
			EstimatedTokens: tokens,
			Content:         string(data),
		})

		return nil
	})

	return files, err
}

// isBinary checks if the initial byte buffer contains null bytes or non-text indicators.
func isBinary(data []byte) bool {
	limit := 1024
	if len(data) < limit {
		limit = len(data)
	}
	for i := 0; i < limit; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

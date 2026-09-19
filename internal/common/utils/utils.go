// Package utils provides bot user identification, concurrent batch execution, and in-memory ZIP archiving for ScanDrix.
package utils

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
)

var knownBotUsers = map[string]struct{}{
	"dependabot":             {},
	"dependabot[bot]":        {},
	"github-actions":         {},
	"github-actions[bot]":    {},
	"renovate":               {},
	"renovate[bot]":          {},
	"greenkeeper[bot]":       {},
	"snyk-bot":               {},
	"codecov[bot]":           {},
	"sonarcloud[bot]":        {},
	"scandrix":               {},
	"scandrix[bot]":          {},
	"drixy":                  {},
	"drixy[bot]":             {},
}

var botLoginFragments = []string{
	"[bot]",
	"dependabot",
	"renovate",
	"renovatebot",
	"github-actions",
	"gitlab-bot",
	"scandrix-bot",
	"drixy-bot",
	"mergify",
}

// IsBotUser determines whether a username or author represents an automated bot.
func IsBotUser(username string) bool {
	if username == "" {
		return false
	}
	u := strings.ToLower(strings.TrimSpace(username))
	if _, ok := knownBotUsers[u]; ok {
		return true
	}
	for _, fragment := range botLoginFragments {
		if strings.Contains(u, fragment) {
			return true
		}
	}
	if strings.HasSuffix(u, "[bot]") || strings.HasSuffix(u, "-bot") || strings.HasPrefix(u, "bot-") {
		return true
	}
	if strings.HasPrefix(u, "project_") && strings.HasSuffix(u, "_bot") {
		return true
	}
	return false
}

// BatchResult wraps output and error from concurrent worker batch execution.
type BatchResult[R any] struct {
	Index  int
	Output R
	Error  error
}

// BatchExecute executes fn concurrently for each item in items with max concurrency limit.
func BatchExecute[T any, R any](ctx context.Context, items []T, concurrency int, fn func(ctx context.Context, item T, index int) (R, error)) ([]R, error) {
	if len(items) == 0 {
		return nil, nil
	}
	if concurrency <= 0 {
		concurrency = 4
	}
	if concurrency > len(items) {
		concurrency = len(items)
	}

	results := make([]R, len(items))
	errs := make([]error, len(items))

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, item := range items {
		wg.Add(1)
		go func(idx int, itm T) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				errs[idx] = ctx.Err()
				return
			}

			out, err := fn(ctx, itm, idx)
			if err != nil {
				errs[idx] = err
			} else {
				results[idx] = out
			}
		}(i, item)
	}

	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

// CreateZipArchive bundles a map of relative file paths and byte content into an in-memory ZIP byte slice.
func CreateZipArchive(files map[string][]byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	for path, content := range files {
		cleanPath := strings.TrimPrefix(path, "/")
		fw, err := zw.Create(cleanPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create zip entry %q: %w", path, err)
		}
		if _, err := fw.Write(content); err != nil {
			return nil, fmt.Errorf("failed to write zip content for %q: %w", path, err)
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize zip archive: %w", err)
	}

	return buf.Bytes(), nil
}

// ExtractZipArchive extracts an in-memory ZIP byte slice into a map of relative file paths and bytes.
func ExtractZipArchive(zipData []byte) (map[string][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("failed to read zip header: %w", err)
	}

	extracted := make(map[string][]byte)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open zip file %q: %w", f.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read zip file %q: %w", f.Name, err)
		}
		extracted[f.Name] = content
	}

	return extracted, nil
}

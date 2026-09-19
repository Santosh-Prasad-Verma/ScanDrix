package utils

import (
	"context"
	"strings"
	"testing"
)

func TestIsBotUser(t *testing.T) {
	if !IsBotUser("dependabot[bot]") {
		t.Fatalf("expected dependabot[bot] to be recognized")
	}
	if !IsBotUser("renovate-bot") {
		t.Fatalf("expected renovate-bot to be recognized")
	}
	if !IsBotUser("project_123_bot") {
		t.Fatalf("expected gitlab project bot to be recognized")
	}
	if !IsBotUser("scandrix[bot]") {
		t.Fatalf("expected scandrix[bot] to be recognized")
	}
	if IsBotUser("tarun") {
		t.Fatalf("expected tarun not to be recognized as bot")
	}
}

func TestBatchExecute(t *testing.T) {
	items := []string{"apple", "banana", "cherry", "date"}

	results, err := BatchExecute(context.Background(), items, 2, func(ctx context.Context, item string, idx int) (string, error) {
		return strings.ToUpper(item), nil
	})

	if err != nil {
		t.Fatalf("BatchExecute failed: %v", err)
	}

	if len(results) != 4 || results[0] != "APPLE" || results[3] != "DATE" {
		t.Fatalf("unexpected results from BatchExecute: %v", results)
	}
}

func TestZipArchiveRoundTrip(t *testing.T) {
	files := map[string][]byte{
		".scandrix/config.yaml": []byte("version: 1\nreview:\n  strict: true\n"),
		"rules/security.md":     []byte("# Security rule\nCheck SQL injection.\n"),
	}

	zipBytes, err := CreateZipArchive(files)
	if err != nil {
		t.Fatalf("CreateZipArchive failed: %v", err)
	}
	if len(zipBytes) == 0 {
		t.Fatalf("expected non-empty zip bytes")
	}

	extracted, err := ExtractZipArchive(zipBytes)
	if err != nil {
		t.Fatalf("ExtractZipArchive failed: %v", err)
	}

	if len(extracted) != 2 {
		t.Fatalf("expected 2 files extracted, got %d", len(extracted))
	}

	if string(extracted[".scandrix/config.yaml"]) != string(files[".scandrix/config.yaml"]) {
		t.Fatalf("extracted content mismatch")
	}
}

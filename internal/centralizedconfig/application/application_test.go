package application

import (
	"context"
	"io"
	"testing"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
	commonUtils "github.com/scandrix/backend/internal/common/utils"
)

func TestDownloadAndZipUseCases(t *testing.T) {
	mockSvc := &MockCentralizedConfigService{
		Entries: []domain.ConfigFileMeta{
			{Path: ".scandrix/config.yaml", Content: "version: 1.2\n"},
			{Path: "rules/security.md", Content: "# Security Rule\n"},
		},
	}

	downloadUC := NewDownloadUseCase(mockSvc)
	entries, err := downloadUC.Execute(context.Background(), "org-1", "team-1")
	if err != nil {
		t.Fatalf("DownloadUseCase failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	zipUC := NewDownloadZipUseCase(downloadUC)
	reader, err := zipUC.Execute(context.Background(), "org-1", "team-1")
	if err != nil {
		t.Fatalf("DownloadZipUseCase failed: %v", err)
	}
	defer reader.Close()

	zipBytes, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read zip bytes: %v", err)
	}

	extracted, err := commonUtils.ExtractZipArchive(zipBytes)
	if err != nil {
		t.Fatalf("failed to extract zip: %v", err)
	}

	if len(extracted) != 2 {
		t.Fatalf("expected 2 extracted files, got %d", len(extracted))
	}
}

func TestInitAndSyncUseCases(t *testing.T) {
	mockSvc := &MockCentralizedConfigService{}

	initUC := NewInitUseCase(mockSvc)
	if err := initUC.Execute(context.Background(), "org-1", "team-1", "main"); err != nil {
		t.Fatalf("InitUseCase failed: %v", err)
	}

	syncUC := NewSyncUseCase(mockSvc)
	if err := syncUC.Execute(context.Background(), "org-1", "team-1"); err != nil {
		t.Fatalf("SyncUseCase failed: %v", err)
	}
}

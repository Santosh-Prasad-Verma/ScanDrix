package application

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
)

func TestDownloadZipUseCase_Execute(t *testing.T) {
	mockSvc := &MockCentralizedConfigService{
		Entries: []domain.ConfigFileMeta{
			{
				Path:    "repo-1/scandrix-config.yaml",
				Content: "version: '1.2'\nreviewMode: strict\n",
			},
			{
				Path:    "repo-1/.drixy-rules/review/security-rule.yml",
				Content: "title: Check SQL Injection\nseverity: critical\n",
			},
		},
	}

	fullUC := NewFullDownloadUseCase(mockSvc)
	zipUC := NewFullDownloadZipUseCase(fullUC)

	zipBytes, err := zipUC.Execute(context.Background(), "org-1", "team-1", DownloadOptions{})
	if err != nil {
		t.Fatalf("expected successful zip generation, got: %v", err)
	}

	if len(zipBytes) == 0 {
		t.Fatalf("expected non-empty zip bytes")
	}

	// Verify the generated zip archive structure
	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("failed reading generated zip: %v", err)
	}

	filesMap := make(map[string]string)
	for _, f := range zipReader.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("failed opening zip file %q: %v", f.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("failed reading zip file %q: %v", f.Name, err)
		}
		filesMap[f.Name] = string(content)
	}

	// Global config must be synthesized
	if _, ok := filesMap["scandrix-config.yaml"]; !ok {
		t.Errorf("missing scandrix-config.yaml in zip")
	}

	// Repo config
	repoCfg, ok := filesMap["repo-1/scandrix-config.yaml"]
	if !ok {
		t.Errorf("missing repo-1/scandrix-config.yaml in zip")
	} else if !strings.Contains(repoCfg, "reviewMode: strict") {
		t.Errorf("unexpected content in repo-1/scandrix-config.yaml: %s", repoCfg)
	}

	// Rule file
	ruleContent, ok := filesMap["repo-1/.drixy-rules/review/security-rule.yml"]
	if !ok {
		t.Errorf("missing rule in zip")
	} else if !strings.Contains(ruleContent, "Check SQL Injection") {
		t.Errorf("unexpected content in rule file: %s", ruleContent)
	}
}

type failingCentralizedConfigService struct {
	MockCentralizedConfigService
}

func (f *failingCentralizedConfigService) DownloadConfig(ctx context.Context, orgID, teamID string) ([]domain.ConfigFileMeta, error) {
	return nil, errors.New("simulated storage connectivity failure")
}

func TestDownloadZipUseCase_ErrorHandling(t *testing.T) {
	fullUC := NewFullDownloadUseCase(&failingCentralizedConfigService{})
	zipUC := NewFullDownloadZipUseCase(fullUC)

	_, err := zipUC.Execute(context.Background(), "org-1", "team-1", DownloadOptions{})
	if err == nil {
		t.Fatalf("expected error from failed download, got nil")
	}
	if !strings.Contains(err.Error(), "simulated storage connectivity failure") {
		t.Errorf("expected error message to be preserved: %v", err)
	}
}

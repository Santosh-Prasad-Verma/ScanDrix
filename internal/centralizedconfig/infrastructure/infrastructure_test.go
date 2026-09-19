package infrastructure

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
	commonUtils "github.com/scandrix/backend/internal/common/utils"
)

func TestCentralizedConfigService(t *testing.T) {
	prSvc := NewPRService(nil)
	svc := NewService(prSvc, nil, nil)

	status, err := svc.ValidateCentralizedConfig(context.Background(), "org-1", "team-1")
	if err != nil {
		t.Fatalf("ValidateCentralizedConfig failed: %v", err)
	}
	if !status.IsValid || status.ActiveRules < 1 {
		t.Fatalf("expected valid status with active rules, got: %+v", status)
	}

	reader, err := svc.DownloadConfigZip(context.Background(), "org-1", "team-1")
	if err != nil {
		t.Fatalf("DownloadConfigZip failed: %v", err)
	}
	defer reader.Close()

	zipData, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}

	extracted, err := commonUtils.ExtractZipArchive(zipData)
	if err != nil {
		t.Fatalf("failed to extract zip: %v", err)
	}

	if len(extracted) != 2 {
		t.Fatalf("expected 2 extracted files, got %d", len(extracted))
	}
}

func TestPRService(t *testing.T) {
	prSvc := NewPRService(nil)

	path := prSvc.BuildCentralizedPath("repo-a", ".scandrix/config.yaml")
	if path != "repo-a/.scandrix/config.yaml" {
		t.Fatalf("unexpected path: %s", path)
	}

	prURL, err := prSvc.CreateMutationPR(context.Background(), "org-1", "team-1", []domain.FileMutationOp{
		{Path: path, Operation: "upsert", Content: "version: 1.2\n"},
	}, "Update config", "PR description")

	if err != nil {
		t.Fatalf("CreateMutationPR failed: %v", err)
	}
	if prURL == "" {
		t.Fatalf("expected non-empty PR url")
	}
}

func TestSyncListener(t *testing.T) {
	prSvc := NewPRService(nil)
	svc := NewService(prSvc, nil, nil)
	listener := NewSyncListener(svc)

	event := SyncEvent{
		OrganizationID: "org-1",
		TeamID:         "team-1",
		TriggerSource:  "webhook",
	}
	payloadBytes, _ := json.Marshal(event)

	if err := listener.HandleSyncMessage(context.Background(), payloadBytes); err != nil {
		t.Fatalf("HandleSyncMessage failed: %v", err)
	}
}

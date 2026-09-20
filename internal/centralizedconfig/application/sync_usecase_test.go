package application

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
)

type mockSyncService struct {
	synced  bool
	syncErr error
}

func (m *mockSyncService) ValidateCentralizedConfig(ctx context.Context, orgID, teamID string) (*domain.CentralizedConfigStatus, error) {
	return &domain.CentralizedConfigStatus{IsValid: true}, nil
}

func (m *mockSyncService) DownloadConfig(ctx context.Context, orgID, teamID string) ([]domain.ConfigFileMeta, error) {
	return nil, nil
}

func (m *mockSyncService) DownloadConfigZip(ctx context.Context, orgID, teamID string) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockSyncService) InitCentralizedConfig(ctx context.Context, orgID, teamID, defaultBranch string) error {
	return nil
}

func (m *mockSyncService) SyncRepositoryConfig(ctx context.Context, orgID, teamID string) error {
	if m.syncErr != nil {
		return m.syncErr
	}
	m.synced = true
	return nil
}

func TestSyncUseCase_Execution(t *testing.T) {
	ctx := context.Background()
	orgID := "org-1"
	teamID := "team-1"

	// 1. Successful execution
	svc := &mockSyncService{}
	uc := NewSyncUseCase(&mockCentralizedConfigAdapter{svc: svc})

	err := uc.Execute(ctx, orgID, teamID)
	if err != nil {
		t.Fatalf("expected successful sync, got: %v", err)
	}
	if !svc.synced {
		t.Fatalf("expected SyncRepositoryConfig to be called")
	}

	// 2. Error propagation
	svcErr := &mockSyncService{syncErr: errors.New("git provider unreachable")}
	ucErr := NewSyncUseCase(&mockCentralizedConfigAdapter{svc: svcErr})
	err = ucErr.Execute(ctx, orgID, teamID)
	if err == nil {
		t.Fatalf("expected error from failed sync")
	}
}

type mockCentralizedConfigAdapter struct {
	svc *mockSyncService
}

func (a *mockCentralizedConfigAdapter) ValidateCentralizedConfig(ctx context.Context, orgID, teamID string) (*domain.CentralizedConfigStatus, error) {
	return a.svc.ValidateCentralizedConfig(ctx, orgID, teamID)
}

func (a *mockCentralizedConfigAdapter) DownloadConfig(ctx context.Context, orgID, teamID string) ([]domain.ConfigFileMeta, error) {
	return a.svc.DownloadConfig(ctx, orgID, teamID)
}

func (a *mockCentralizedConfigAdapter) DownloadConfigZip(ctx context.Context, orgID, teamID string) (io.ReadCloser, error) {
	return a.svc.DownloadConfigZip(ctx, orgID, teamID)
}

func (a *mockCentralizedConfigAdapter) InitCentralizedConfig(ctx context.Context, orgID, teamID, defaultBranch string) error {
	return a.svc.InitCentralizedConfig(ctx, orgID, teamID, defaultBranch)
}

func (a *mockCentralizedConfigAdapter) SyncRepositoryConfig(ctx context.Context, orgID, teamID string) error {
	return a.svc.SyncRepositoryConfig(ctx, orgID, teamID)
}

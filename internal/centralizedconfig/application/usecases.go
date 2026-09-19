// Package application implements use cases for downloading, ZIP packaging, initializing, and synchronizing centralized repository configurations.
package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
	commonUtils "github.com/scandrix/backend/internal/common/utils"
)

// DownloadUseCase retrieves all configuration files and custom rules for an organization/team.
type DownloadUseCase struct {
	configService domain.CentralizedConfigService
}

// NewDownloadUseCase creates a new DownloadUseCase.
func NewDownloadUseCase(svc domain.CentralizedConfigService) *DownloadUseCase {
	return &DownloadUseCase{configService: svc}
}

// Execute returns a list of ConfigFileMeta entries.
func (uc *DownloadUseCase) Execute(ctx context.Context, orgID, teamID string) ([]domain.ConfigFileMeta, error) {
	if orgID == "" {
		return nil, errors.New("centralizedconfig: organizationId is required")
	}
	return uc.configService.DownloadConfig(ctx, orgID, teamID)
}

// DownloadZipUseCase bundles all configuration files into a compressed ZIP stream.
type DownloadZipUseCase struct {
	downloadUseCase *DownloadUseCase
}

// NewDownloadZipUseCase creates a new DownloadZipUseCase.
func NewDownloadZipUseCase(downloadUC *DownloadUseCase) *DownloadZipUseCase {
	return &DownloadZipUseCase{downloadUseCase: downloadUC}
}

// Execute generates a readable ZIP archive stream of all configuration files.
func (uc *DownloadZipUseCase) Execute(ctx context.Context, orgID, teamID string) (io.ReadCloser, error) {
	entries, err := uc.downloadUseCase.Execute(ctx, orgID, teamID)
	if err != nil {
		return nil, err
	}

	files := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		files[entry.Path] = []byte(entry.Content)
	}

	zipBytes, err := commonUtils.CreateZipArchive(files)
	if err != nil {
		return nil, fmt.Errorf("failed to pack centralized config zip: %w", err)
	}

	return io.NopCloser(bytes.NewReader(zipBytes)), nil
}

// InitUseCase initializes a centralized repository structure with baseline templates.
type InitUseCase struct {
	configService domain.CentralizedConfigService
}

// NewInitUseCase creates a new InitUseCase.
func NewInitUseCase(svc domain.CentralizedConfigService) *InitUseCase {
	return &InitUseCase{configService: svc}
}

// Execute bootstraps the initial repository structure.
func (uc *InitUseCase) Execute(ctx context.Context, orgID, teamID, defaultBranch string) error {
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	return uc.configService.InitCentralizedConfig(ctx, orgID, teamID, defaultBranch)
}

// SyncUseCase reconciles the remote repository files with organization settings.
type SyncUseCase struct {
	configService domain.CentralizedConfigService
}

// NewSyncUseCase creates a new SyncUseCase.
func NewSyncUseCase(svc domain.CentralizedConfigService) *SyncUseCase {
	return &SyncUseCase{configService: svc}
}

// Execute performs bidirectional synchronization.
func (uc *SyncUseCase) Execute(ctx context.Context, orgID, teamID string) error {
	return uc.configService.SyncRepositoryConfig(ctx, orgID, teamID)
}

// MockCentralizedConfigService provides an in-memory test implementation of domain.CentralizedConfigService.
type MockCentralizedConfigService struct {
	Entries []domain.ConfigFileMeta
	Status  *domain.CentralizedConfigStatus
}

func (m *MockCentralizedConfigService) ValidateCentralizedConfig(ctx context.Context, orgID, teamID string) (*domain.CentralizedConfigStatus, error) {
	if m.Status != nil {
		return m.Status, nil
	}
	return &domain.CentralizedConfigStatus{
		IsValid:     true,
		LastSyncAt:  time.Now(),
		ActiveRules: len(m.Entries),
		TotalFiles:  len(m.Entries),
	}, nil
}

func (m *MockCentralizedConfigService) DownloadConfig(ctx context.Context, orgID, teamID string) ([]domain.ConfigFileMeta, error) {
	return m.Entries, nil
}

func (m *MockCentralizedConfigService) DownloadConfigZip(ctx context.Context, orgID, teamID string) (io.ReadCloser, error) {
	files := make(map[string][]byte, len(m.Entries))
	for _, e := range m.Entries {
		files[e.Path] = []byte(e.Content)
	}
	b, err := commonUtils.CreateZipArchive(files)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (m *MockCentralizedConfigService) InitCentralizedConfig(ctx context.Context, orgID, teamID, defaultBranch string) error {
	return nil
}

func (m *MockCentralizedConfigService) SyncRepositoryConfig(ctx context.Context, orgID, teamID string) error {
	return nil
}

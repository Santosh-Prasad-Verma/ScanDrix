// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package factory

import (
	"sync"
	"testing"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/pkg/models"
)

type mockCodeManagementService struct {
	contracts.ICodeManagementService
	provider models.SCMProvider
}

func (m *mockCodeManagementService) Provider() models.SCMProvider {
	return m.provider
}

func TestPlatformIntegrationFactory_RegistrationAndLookup(t *testing.T) {
	f := NewPlatformIntegrationFactory()

	ghMock := &mockCodeManagementService{provider: models.ProviderGitHub}
	glMock := &mockCodeManagementService{provider: models.ProviderGitLab}
	bbMock := &mockCodeManagementService{provider: models.ProviderBitbucket}
	azMock := &mockCodeManagementService{provider: models.ProviderAzure}
	fgMock := &mockCodeManagementService{provider: models.ProviderForgejo}

	f.RegisterCodeManagementService(models.ProviderGitHub, ghMock)
	f.RegisterCodeManagementService(models.ProviderGitLab, glMock)
	f.RegisterCodeManagementService(models.ProviderBitbucket, bbMock)
	f.RegisterCodeManagementService(models.ProviderAzure, azMock)
	f.RegisterCodeManagementService(models.ProviderForgejo, fgMock)

	// 1. Direct enum lookup
	svc, err := f.GetCodeManagementService(models.ProviderGitHub)
	if err != nil || svc != ghMock {
		t.Fatalf("expected GitHub service, got %v, err: %v", svc, err)
	}

	// 2. String alias lookup
	testCases := []struct {
		key      string
		expected contracts.ICodeManagementService
	}{
		{"github", ghMock},
		{"GH", ghMock},
		{"gitlab", glMock},
		{"gl", glMock},
		{"bitbucket", bbMock},
		{"bitbucket_cloud", bbMock},
		{"bitbucket_server", bbMock},
		{"bbs", bbMock},
		{"azure", azMock},
		{"azure_repos", azMock},
		{"ado", azMock},
		{"forgejo", fgMock},
		{"gitea", fgMock},
	}

	for _, tc := range testCases {
		res, err := f.GetCodeManagementServiceByString(tc.key)
		if err != nil || res != tc.expected {
			t.Errorf("lookup for alias '%s' failed: got %v, err: %v", tc.key, res, err)
		}
	}

	// 3. Error on unregistered or invalid provider
	if _, err := f.GetCodeManagementServiceByString("UNKNOWN"); err == nil {
		t.Error("expected error for UNKNOWN provider")
	}
	if _, err := f.GetCodeManagementServiceByString("null"); err == nil {
		t.Error("expected error for 'null' provider string")
	}
	if _, err := f.GetCodeManagementServiceByString(""); err == nil {
		t.Error("expected error for empty provider string")
	}

	// 4. HasCodeManagementService
	if !f.HasCodeManagementService(models.ProviderGitHub) {
		t.Error("expected HasCodeManagementService(GitHub) == true")
	}
	if f.HasCodeManagementService(models.SCMProvider("unregistered")) {
		t.Error("expected HasCodeManagementService(unregistered) == false")
	}

	// 5. ListRegisteredProviders
	providers := f.ListRegisteredProviders()
	if len(providers) != 5 {
		t.Fatalf("expected 5 registered providers, got %d", len(providers))
	}

	// 6. Unregister
	f.Unregister(models.ProviderGitHub)
	if f.HasCodeManagementService(models.ProviderGitHub) {
		t.Error("expected GitHub to be unregistered")
	}
	if _, err := f.GetCodeManagementService(models.ProviderGitHub); err == nil {
		t.Error("expected error fetching unregistered GitHub service")
	}
}

func TestPlatformIntegrationFactory_Concurrency(t *testing.T) {
	f := NewPlatformIntegrationFactory()
	mockSvc := &mockCodeManagementService{provider: models.ProviderForgejo}

	var wg sync.WaitGroup
	const iterations = 100

	// Concurrent registers
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.RegisterCodeManagementService(models.ProviderForgejo, mockSvc)
		}()
	}

	// Concurrent lookups
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = f.GetCodeManagementService(models.ProviderForgejo)
			_ = f.HasCodeManagementService(models.ProviderForgejo)
			_, _ = f.GetCodeManagementServiceByString("forgejo")
		}()
	}

	wg.Wait()
}

// Copyright 2026 ScanDrix AI. All rights reserved.

package orgparamusecases_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	orgparamusecases "github.com/scandrix/backend/internal/organization/application/usecases/organizationparameters"
	orgparams "github.com/scandrix/backend/internal/organization/domain/organizationparameters"
	"github.com/scandrix/backend/internal/organization/infrastructure/repositories"
)

func TestEncryptDecryptSecret(t *testing.T) {
	secret := "sk-ant-api03-very-secret-token-value-12345"

	encrypted, err := orgparamusecases.EncryptSecret(secret)
	if err != nil {
		t.Fatalf("EncryptSecret failed: %v", err)
	}

	decrypted, err := orgparamusecases.DecryptSecret(encrypted)
	if err != nil {
		t.Fatalf("DecryptSecret failed: %v", err)
	}

	if decrypted != secret {
		t.Errorf("expected decrypted secret %s, got %s", secret, decrypted)
	}
}

func TestBYOKProvidersAndModelsUseCases(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewPostgresOrganizationParametersRepository(nil)
	wsID := uuid.New()

	// Seed BYOK config with configured openai provider
	byokCfg := orgparams.BYOKConfigValue{
		Version: 2,
		Credentials: []orgparams.BYOKCredential{
			{ID: "c1", Provider: "openai", APIKey: "sk-proj-test12345"},
		},
		Models: []orgparams.BYOKModel{
			{ID: "m1", ModelID: "gpt-4o", Provider: "openai", CredentialID: "c1"},
		},
	}
	entity, _ := orgparams.NewOrganizationParametersEntity(wsID, orgparams.KeyBYOKConfig, byokCfg, "test")
	_, _ = repo.Create(ctx, entity)

	// 1. GetBYOKProvidersUseCase
	provUC := orgparamusecases.NewGetBYOKProvidersUseCase(repo)
	providers, err := provUC.Execute(ctx, wsID)
	if err != nil || len(providers) == 0 {
		t.Fatalf("failed to list providers: %v", err)
	}

	foundOpenAI := false
	for _, p := range providers {
		if p.ID == "openai" {
			foundOpenAI = true
			if !p.Configured {
				t.Errorf("expected openai to be marked configured")
			}
		}
	}
	if !foundOpenAI {
		t.Errorf("expected openai to be in provider list")
	}

	// 2. GetModelsByProviderUseCase
	modelsUC := orgparamusecases.NewGetModelsByProviderUseCase()
	models := modelsUC.Execute("anthropic")
	if len(models) == 0 {
		t.Errorf("expected non-empty models for anthropic")
	}

	// 3. GetModelCapabilitiesUseCase
	capsUC := orgparamusecases.NewGetModelCapabilitiesUseCase()
	caps := capsUC.Execute("claude-3-7-sonnet")
	if caps.ContextWindow <= 0 || !caps.SupportsReasoning {
		t.Errorf("expected valid capabilities for claude-3-7-sonnet, got %+v", caps)
	}

	// 4. DeleteBYOKConfigUseCase referential integrity
	delUC := orgparamusecases.NewDeleteBYOKConfigUseCase(repo)
	// Try deleting with no overrides
	err = delUC.Execute(ctx, wsID, "nonexistent-model")
	if err != nil {
		t.Errorf("expected no error deleting nonexistent model: %v", err)
	}
}

func TestAssertSafeURLValidation(t *testing.T) {
	invalidURLs := []string{
		"http://localhost",
		"http://localhost:8080/v1",
		"https://foo.localhost/v1",
		"http://127.0.0.1:8080",
		"https://127.0.0.1/v1",
		"http://10.0.0.1:9000",
		"http://192.168.1.50/v1",
		"http://172.16.0.5/v1",
		"http://169.254.169.254/latest/meta-data",
		"http://0.0.0.0:8000",
		"file:///etc/passwd",
		"ftp://evil.com/models",
		"https://non-existent-domain-for-unit-test-never-resolves-12345.scandrix.internal",
	}

	for _, raw := range invalidURLs {
		err := orgparamusecases.AssertSafeURL(raw)
		if err == nil {
			t.Errorf("expected AssertSafeURL to reject %q, but succeeded", raw)
		}
	}

	// Permitted: empty or public IP
	if err := orgparamusecases.AssertSafeURL(""); err != nil {
		t.Errorf("expected empty URL to pass, got: %v", err)
	}

	if err := orgparamusecases.AssertSafeURL("https://8.8.8.8/v1"); err != nil {
		t.Errorf("expected public IP to pass, got: %v", err)
	}
}


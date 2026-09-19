// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgparamusecases

import (
	"context"
	"strings"

	orgparams "github.com/scandrix/backend/internal/organization/domain/organizationparameters"
)

// TestBYOKModelUseCase tests invocation of an enrolled model against an authoritative provider.
type TestBYOKModelUseCase struct {
	connTester *TestBYOKConnectionUseCase
	repo       orgparams.IOrganizationParametersRepository
}

// NewTestBYOKModelUseCase instantiates the use case with optional parameters repository.
func NewTestBYOKModelUseCase(repo ...orgparams.IOrganizationParametersRepository) *TestBYOKModelUseCase {
	var r orgparams.IOrganizationParametersRepository
	if len(repo) > 0 {
		r = repo[0]
	}
	return &TestBYOKModelUseCase{
		connTester: NewTestBYOKConnectionUseCase(),
		repo:       r,
	}
}

// WithRepo injects parameters repository for credential resolution.
func (uc *TestBYOKModelUseCase) WithRepo(repo orgparams.IOrganizationParametersRepository) *TestBYOKModelUseCase {
	uc.repo = repo
	return uc
}

// Execute performs live probe verification of the model against the target provider.
func (uc *TestBYOKModelUseCase) Execute(ctx context.Context, input ByokTestInput) ByokTestResult {
	if strings.TrimSpace(input.ModelID) == "" {
		return ByokTestResult{
			Success: false,
			Code:    "bad_request",
			Message: "Model ID is required",
		}
	}

	// Dispatch live probe to provider
	return uc.connTester.Execute(ctx, input)
}

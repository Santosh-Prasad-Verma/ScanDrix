// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgusecases

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/scandrix/backend/internal/organization/domain/organization"
	"github.com/scandrix/backend/internal/organization/domain/organizationparameters"
)

// GetOrganizationsByDomainUseCase discovers organizations configured with auto-join for an email domain.
type GetOrganizationsByDomainUseCase struct {
	orgRepo   orgdomain.IOrganizationRepository
	paramRepo orgparams.IOrganizationParametersRepository
}

func NewGetOrganizationsByDomainUseCase(
	orgRepo orgdomain.IOrganizationRepository,
	paramRepo orgparams.IOrganizationParametersRepository,
) *GetOrganizationsByDomainUseCase {
	return &GetOrganizationsByDomainUseCase{
		orgRepo:   orgRepo,
		paramRepo: paramRepo,
	}
}

func (uc *GetOrganizationsByDomainUseCase) Execute(ctx context.Context, domain string) ([]*orgdomain.OrganizationEntity, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" || uc.paramRepo == nil || uc.orgRepo == nil {
		return []*orgdomain.OrganizationEntity{}, nil
	}

	params, err := uc.paramRepo.FindByKeyAndValue(ctx, orgparams.KeyAutoJoinConfig, map[string]any{
		"enabled": true,
	})
	if err != nil {
		return nil, err
	}

	var matchedOrgs []*orgdomain.OrganizationEntity
	for _, p := range params {
		var cfg orgparams.AutoJoinConfigValue
		if err := json.Unmarshal(p.ConfigValue, &cfg); err != nil || !cfg.Enabled {
			continue
		}
		for _, d := range cfg.Domains {
			if strings.EqualFold(strings.TrimSpace(d), domain) {
				org, err := uc.orgRepo.FindByID(ctx, p.WorkspaceID)
				if err == nil && org != nil && org.Status {
					matchedOrgs = append(matchedOrgs, org)
				}
				break
			}
		}
	}

	return matchedOrgs, nil
}

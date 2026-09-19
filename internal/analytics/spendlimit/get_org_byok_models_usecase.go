package spendlimit

import (
	"context"
	"encoding/json"
)

// GetOrgByokModelsUseCase discovers every distinct BYOK model an organization could run:
// the BYOK main/fallback models plus any per-repository/per-directory byokModel overrides
// in the code-review config. This is the set the spend-limit enablement gate must be able to price.
// Best-effort: a failed or absent lookup contributes nothing rather than failing the whole sweep.
type GetOrgByokModelsUseCase struct {
	orgParamsService  IOrganizationParametersService
	parametersService IParametersService
}

// NewGetOrgByokModelsUseCase creates a new GetOrgByokModelsUseCase.
func NewGetOrgByokModelsUseCase(
	orgParamsService IOrganizationParametersService,
	parametersService IParametersService,
) *GetOrgByokModelsUseCase {
	return &GetOrgByokModelsUseCase{
		orgParamsService:  orgParamsService,
		parametersService: parametersService,
	}
}

// Execute retrieves the BYOK models and code-review config overrides, returning a deduped list.
func (uc *GetOrgByokModelsUseCase) Execute(ctx context.Context, orgID, teamID string) []string {
	var byokConfig *BYOKConfigRef
	if uc.orgParamsService != nil {
		if raw, err := uc.orgParamsService.Get(ctx, orgID, teamID, ParamKeyBYOKConfig); err == nil && raw != nil {
			b, _ := json.Marshal(raw)
			var ref BYOKConfigRef
			if err := json.Unmarshal(b, &ref); err == nil {
				byokConfig = &ref
			}
		}
	}

	var extraModels []string
	if uc.parametersService != nil {
		if raw, err := uc.parametersService.Get(ctx, orgID, teamID, ParamKeyCodeReviewConfig); err == nil && raw != nil {
			extraModels = ExtractByokModelsFromConfig(raw)
		}
	}

	return CollectByokModels(byokConfig, extraModels)
}

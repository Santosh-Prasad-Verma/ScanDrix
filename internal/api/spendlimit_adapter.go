package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/analytics/spendlimit"
	orgparamdomain "github.com/scandrix/backend/internal/organization/domain/organizationparameters"
)

// OrgParamsSpendLimitAdapter adapts the organization parameters service to spendlimit.IOrganizationParametersService.
type OrgParamsSpendLimitAdapter struct {
	service orgparamdomain.IOrganizationParametersService
}

// NewOrgParamsSpendLimitAdapter constructs a new adapter.
func NewOrgParamsSpendLimitAdapter(svc orgparamdomain.IOrganizationParametersService) *OrgParamsSpendLimitAdapter {
	return &OrgParamsSpendLimitAdapter{service: svc}
}

func (a *OrgParamsSpendLimitAdapter) Get(ctx context.Context, orgID, teamID, key string) (any, error) {
	if a == nil || a.service == nil {
		return nil, nil
	}
	wsUUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, nil
	}
	entity, err := a.service.FindByKey(ctx, wsUUID, orgparamdomain.ParameterKey(key))
	if err != nil || entity == nil {
		return nil, err
	}
	return entity.ConfigValue, nil
}

func (a *OrgParamsSpendLimitAdapter) Set(ctx context.Context, orgID, teamID, key string, val any) error {
	if a == nil || a.service == nil {
		return errors.New("organization parameters persistence service is unavailable")
	}
	wsUUID, err := uuid.Parse(orgID)
	if err != nil {
		return fmt.Errorf("invalid organization UUID: %w", err)
	}
	_, err = a.service.CreateOrUpdateConfig(ctx, wsUUID, orgparamdomain.ParameterKey(key), val, "spend limit configuration")
	return err
}

func (a *OrgParamsSpendLimitAdapter) ListConfigured(ctx context.Context, key string) ([]spendlimit.OrgParameterRecord, error) {
	if a == nil || a.service == nil {
		return nil, nil
	}
	var pKey *orgparamdomain.ParameterKey
	if key != "" {
		pk := orgparamdomain.ParameterKey(key)
		pKey = &pk
	}
	entities, err := a.service.Find(ctx, orgparamdomain.OrganizationParametersFilter{
		ConfigKey: pKey,
	})
	if err != nil {
		return nil, err
	}
	records := make([]spendlimit.OrgParameterRecord, 0, len(entities))
	for _, e := range entities {
		records = append(records, spendlimit.OrgParameterRecord{
			OrganizationID: e.WorkspaceID.String(),
			ConfigKey:      string(e.ConfigKey),
			ConfigValue:    e.ConfigValue,
		})
	}
	return records, nil
}

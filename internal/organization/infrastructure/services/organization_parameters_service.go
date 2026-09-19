// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	orgparamdomain "github.com/scandrix/backend/internal/organization/domain/organizationparameters"
)

// OrganizationParametersService implements orgparamdomain.IOrganizationParametersService.
type OrganizationParametersService struct {
	repo orgparamdomain.IOrganizationParametersRepository
}

// NewOrganizationParametersService creates a new OrganizationParametersService.
func NewOrganizationParametersService(repo orgparamdomain.IOrganizationParametersRepository) *OrganizationParametersService {
	return &OrganizationParametersService{repo: repo}
}

func (s *OrganizationParametersService) Find(ctx context.Context, filter orgparamdomain.OrganizationParametersFilter) ([]*orgparamdomain.OrganizationParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.Find(ctx, filter)
}

func (s *OrganizationParametersService) FindOne(ctx context.Context, filter orgparamdomain.OrganizationParametersFilter) (*orgparamdomain.OrganizationParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindOne(ctx, filter)
}

func (s *OrganizationParametersService) FindByID(ctx context.Context, id uuid.UUID) (*orgparamdomain.OrganizationParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindByID(ctx, id)
}

func (s *OrganizationParametersService) FindByOrganizationName(ctx context.Context, orgName string) (*orgparamdomain.OrganizationParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindByOrganizationName(ctx, orgName)
}

func (s *OrganizationParametersService) FindByKey(ctx context.Context, wsID uuid.UUID, key orgparamdomain.ParameterKey) (*orgparamdomain.OrganizationParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindByKey(ctx, wsID, key)
}

func (s *OrganizationParametersService) FindByKeyAndValue(ctx context.Context, key orgparamdomain.ParameterKey, matchJSON map[string]any) ([]*orgparamdomain.OrganizationParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindByKeyAndValue(ctx, key, matchJSON)
}

func (s *OrganizationParametersService) FindByKeyAndValueFuzzy(ctx context.Context, key orgparamdomain.ParameterKey, matchJSON map[string]any, fuzzy bool) ([]*orgparamdomain.OrganizationParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindByKeyAndValueFuzzy(ctx, key, matchJSON, fuzzy)
}

func (s *OrganizationParametersService) Create(ctx context.Context, entity *orgparamdomain.OrganizationParametersEntity) (*orgparamdomain.OrganizationParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.Create(ctx, entity)
}

func (s *OrganizationParametersService) Update(ctx context.Context, filter orgparamdomain.OrganizationParametersFilter, data *orgparamdomain.OrganizationParametersEntity) (*orgparamdomain.OrganizationParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.Update(ctx, filter, data)
}

func (s *OrganizationParametersService) Delete(ctx context.Context, wsID uuid.UUID, key orgparamdomain.ParameterKey) error {
	if s.repo == nil {
		return errors.New("repository uninitialized")
	}
	return s.repo.Delete(ctx, wsID, key)
}

// CreateOrUpdateConfig creates a new parameter or updates an existing one for the given key.
func (s *OrganizationParametersService) CreateOrUpdateConfig(ctx context.Context, wsID uuid.UUID, key orgparamdomain.ParameterKey, val any, desc string) (*orgparamdomain.OrganizationParametersEntity, error) {
	bytes, err := json.Marshal(val)
	if err != nil {
		return nil, err
	}

	existing, err := s.FindByKey(ctx, wsID, key)
	if err != nil {
		return nil, err
	}

	if existing == nil {
		entity, err := orgparamdomain.NewOrganizationParametersEntity(wsID, key, val, desc)
		if err != nil {
			return nil, err
		}
		return s.Create(ctx, entity)
	}

	existing.ConfigValue = bytes
	if desc != "" {
		existing.Description = desc
	}
	return s.Create(ctx, existing)
}

// DeleteBYOKConfig removes main or fallback configuration, or removes provider credentials.
// If deleting main and there is no fallback, or deleting the last part,
// the entire BYOK configuration parameter is removed.
func (s *OrganizationParametersService) DeleteBYOKConfig(ctx context.Context, wsID uuid.UUID, configType string) error {
	existing, err := s.FindByKey(ctx, wsID, orgparamdomain.KeyBYOKConfig)
	if err != nil || existing == nil {
		return errors.New("BYOK configuration not found")
	}

	var byok orgparamdomain.BYOKConfigValue
	if err := json.Unmarshal(existing.ConfigValue, &byok); err != nil {
		return err
	}

	if configType == "main" {
		if byok.Main == nil {
			return errors.New("config main not found")
		}
		if byok.Fallback == nil && len(byok.Models) <= 1 {
			return s.Delete(ctx, wsID, orgparamdomain.KeyBYOKConfig)
		}
		byok.Main = nil
	} else if configType == "fallback" {
		if byok.Fallback == nil {
			return errors.New("config fallback not found")
		}
		if byok.Main == nil && len(byok.Models) <= 1 {
			return s.Delete(ctx, wsID, orgparamdomain.KeyBYOKConfig)
		}
		byok.Fallback = nil
	} else {
		// provider-specific credential removal fallback
		var updatedCreds []orgparamdomain.BYOKCredential
		for _, cred := range byok.Credentials {
			if cred.Provider != configType {
				updatedCreds = append(updatedCreds, cred)
			}
		}
		byok.Credentials = updatedCreds
	}

	_, err = s.CreateOrUpdateConfig(ctx, wsID, orgparamdomain.KeyBYOKConfig, byok, "BYOK Configuration")
	return err
}

// DeleteBYOKModel removes a specific model and drops any now-orphan non-managed credentials.
// If the final model is removed, the entire BYOK configuration parameter is cleaned up.
func (s *OrganizationParametersService) DeleteBYOKModel(ctx context.Context, wsID uuid.UUID, modelID string) error {
	existing, err := s.FindByKey(ctx, wsID, orgparamdomain.KeyBYOKConfig)
	if err != nil || existing == nil {
		return errors.New("BYOK configuration not found")
	}

	var byok orgparamdomain.BYOKConfigValue
	if err := json.Unmarshal(existing.ConfigValue, &byok); err != nil {
		return err
	}

	var targetFound bool
	var remainingModels []orgparamdomain.BYOKModel
	for _, m := range byok.Models {
		if m.ID == modelID || m.ModelID == modelID {
			targetFound = true
		} else {
			remainingModels = append(remainingModels, m)
		}
	}
	if !targetFound {
		return fmt.Errorf("model %s not found", modelID)
	}

	// Last-model disconnect: removing the final model tears down the whole config
	if len(remainingModels) == 0 {
		return s.Delete(ctx, wsID, orgparamdomain.KeyBYOKConfig)
	}

	// Drop orphan, non-managed credentials: ones that no remaining model references
	stillReferenced := make(map[string]bool)
	for _, m := range remainingModels {
		if m.CredentialID != "" {
			stillReferenced[m.CredentialID] = true
		}
	}

	var remainingCredentials []orgparamdomain.BYOKCredential
	for _, c := range byok.Credentials {
		if c.Managed || (c.ID != "" && stillReferenced[c.ID]) {
			remainingCredentials = append(remainingCredentials, c)
		}
	}

	byok.Models = remainingModels
	byok.Credentials = remainingCredentials

	if byok.Main != nil && (byok.Main.ID == modelID || byok.Main.ModelID == modelID) {
		byok.Main = nil
	}
	if byok.Fallback != nil && (byok.Fallback.ID == modelID || byok.Fallback.ModelID == modelID) {
		byok.Fallback = nil
	}

	_, err = s.CreateOrUpdateConfig(ctx, wsID, orgparamdomain.KeyBYOKConfig, byok, "BYOK Configuration")
	return err
}

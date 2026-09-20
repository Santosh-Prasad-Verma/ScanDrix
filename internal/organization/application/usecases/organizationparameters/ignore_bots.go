// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgparamusecases

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/organization/domain/organizationparameters"
)

var defaultKnownBots = []string{
	"dependabot",
	"dependabot[bot]",
	"renovate",
	"renovate[bot]",
	"snyk-bot",
	"codecov",
	"codecov[bot]",
	"greenkeeper",
	"greenkeeper[bot]",
	"github-actions",
	"github-actions[bot]",
	"scandrix-bot",
}

// IgnoreBotsUseCase detects bot accounts and updates the auto-license ignore list.
type IgnoreBotsUseCase struct {
	repo orgparams.IOrganizationParametersRepository
}

func NewIgnoreBotsUseCase(repo orgparams.IOrganizationParametersRepository) *IgnoreBotsUseCase {
	return &IgnoreBotsUseCase{repo: repo}
}

func (uc *IgnoreBotsUseCase) Execute(ctx context.Context, wsID uuid.UUID, detectedAuthors []string) ([]string, error) {
	botSet := make(map[string]bool)
	for _, b := range defaultKnownBots {
		botSet[strings.ToLower(b)] = true
	}

	for _, author := range detectedAuthors {
		lower := strings.ToLower(strings.TrimSpace(author))
		if strings.HasSuffix(lower, "[bot]") || botSet[lower] {
			botSet[lower] = true
		}
	}

	var botList []string
	for b := range botSet {
		botList = append(botList, b)
	}

	if wsID != uuid.Nil && uc.repo != nil {
		existing, _ := uc.repo.FindByKey(ctx, wsID, orgparams.KeyAutoLicenseAssignment)
		cfg := orgparams.AutoLicenseAssignmentConfigValue{
			Enabled:      false,
			IgnoredUsers: botList,
			AllowedUsers: []string{},
			SeededBotIDs: botList,
		}

		if existing != nil {
			var oldCfg orgparams.AutoLicenseAssignmentConfigValue
			if json.Unmarshal(existing.ConfigValue, &oldCfg) == nil {
				cfg.Enabled = oldCfg.Enabled
				cfg.AllowedUsers = oldCfg.AllowedUsers
				// Merge old ignored users with new bots
				existingSet := make(map[string]bool)
				for _, u := range oldCfg.IgnoredUsers {
					existingSet[strings.ToLower(u)] = true
				}
				for _, b := range botList {
					if !existingSet[b] {
						oldCfg.IgnoredUsers = append(oldCfg.IgnoredUsers, b)
					}
				}
				cfg.IgnoredUsers = oldCfg.IgnoredUsers
				cfg.SeededBotIDs = botList
			}
		}

		bytes, _ := json.Marshal(cfg)
		entity := &orgparams.OrganizationParametersEntity{
			UUID:        uuid.New(),
			WorkspaceID: wsID,
			ConfigKey:   orgparams.KeyAutoLicenseAssignment,
			ConfigValue: bytes,
			Description: "Auto-seeded bot suppression list for seat licensing",
			IsActive:    true,
		}

		if existing == nil {
			_, _ = uc.repo.Create(ctx, entity)
		} else {
			_, _ = uc.repo.Update(ctx, orgparams.OrganizationParametersFilter{
				WorkspaceID: &wsID,
				ConfigKey:   &entity.ConfigKey,
			}, entity)
		}
	}

	return botList, nil
}

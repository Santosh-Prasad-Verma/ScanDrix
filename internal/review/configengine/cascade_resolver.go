// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package configengine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
)

// CascadeResolver computes the final effective configuration by hierarchically merging
// Default -> Organization -> Team -> Repository -> In-Repo layers.
type CascadeResolver struct {
	provider IConfigPersistenceProvider
	parser   *InRepoConfigParser
}

// NewCascadeResolver constructs a cascade resolver.
func NewCascadeResolver(provider IConfigPersistenceProvider, parser *InRepoConfigParser) *CascadeResolver {
	if parser == nil {
		parser = NewInRepoConfigParser()
	}
	return &CascadeResolver{
		provider: provider,
		parser:   parser,
	}
}

// ResolveInput provides context keys for hierarchical resolution.
type ResolveInput struct {
	WorkspaceID  uuid.UUID
	RepositoryID uuid.UUID
	TeamID       *uuid.UUID
	InRepoConfig *InRepoConfiguration
}

// Resolve executes the full 5-tier configuration cascade and builds the inheritance audit trail.
func (r *CascadeResolver) Resolve(ctx context.Context, input ResolveInput) (CascadeResolutionResult, error) {
	result := CascadeResolutionResult{
		Config: domain.DefaultCodeReviewConfig(),
		AuditTrail: InheritanceAuditTrail{
			ReviewMode: CascadeConfigEntry[ReviewMode]{
				Value: ModeNormal,
				Scope: ScopeDefault,
			},
			Sensitivity: CascadeConfigEntry[ReviewSensitivity]{
				Value: SensitivityStandard,
				Scope: ScopeDefault,
			},
			MaxCommentsPerReview: CascadeConfigEntry[int]{
				Value: 15,
				Scope: ScopeDefault,
			},
			CommittableSuggestions: CascadeConfigEntry[bool]{
				Value: true,
				Scope: ScopeDefault,
			},
			RequireTicketContext: CascadeConfigEntry[bool]{
				Value: false,
				Scope: ScopeDefault,
			},
			AutoApproveCleanPRs: CascadeConfigEntry[bool]{
				Value: true,
				Scope: ScopeDefault,
			},
			EnableTraceDecisions: CascadeConfigEntry[bool]{
				Value: true,
				Scope: ScopeDefault,
			},
			IgnoredFilePatternsSources: make(map[string]ConfigScope),
			BranchFiltersSources:       make(map[string]ConfigScope),
			ResolvedAt:                 time.Now().UTC(),
		},
	}

	// 1. Organization Level
	if r.provider != nil && input.WorkspaceID != uuid.Nil {
		orgCfg, err := r.provider.GetOrganizationConfig(ctx, input.WorkspaceID)
		if err == nil && orgCfg != nil {
			r.applyConfigLayer(&result, *orgCfg, ScopeOrganization, input.WorkspaceID.String())
		} else if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("failed to fetch organization config: %v", err))
		}
	}

	// 2. Team Level (Explicit or look up repo team)
	var teamID *uuid.UUID = input.TeamID
	if teamID == nil && r.provider != nil && input.WorkspaceID != uuid.Nil && input.RepositoryID != uuid.Nil {
		foundTeam, err := r.provider.GetRepositoryTeamID(ctx, input.WorkspaceID, input.RepositoryID)
		if err == nil && foundTeam != nil {
			teamID = foundTeam
		}
	}

	if r.provider != nil && input.WorkspaceID != uuid.Nil && teamID != nil && *teamID != uuid.Nil {
		teamCfg, err := r.provider.GetTeamConfig(ctx, input.WorkspaceID, *teamID)
		if err == nil && teamCfg != nil {
			r.applyConfigLayer(&result, *teamCfg, ScopeTeam, teamID.String())
		} else if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("failed to fetch team config: %v", err))
		}
	}

	// 3. Repository Level
	if r.provider != nil && input.WorkspaceID != uuid.Nil && input.RepositoryID != uuid.Nil {
		repoCfg, err := r.provider.GetRepositoryConfig(ctx, input.WorkspaceID, input.RepositoryID)
		if err == nil && repoCfg != nil {
			r.applyConfigLayer(&result, *repoCfg, ScopeRepository, input.RepositoryID.String())
		} else if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("failed to fetch repository config: %v", err))
		}
	}

	// 4. In-Repo Level (.scandrix.yml / .scandrix/config.json)
	if input.InRepoConfig != nil {
		r.applyInRepoLayer(&result, input.InRepoConfig)
	}

	return result, nil
}

func (r *CascadeResolver) applyConfigLayer(
	res *CascadeResolutionResult,
	layer domain.CodeReviewConfig,
	scope ConfigScope,
	sourceID string,
) {
	// Review Mode
	if layer.ReviewMode != "" {
		m := ReviewMode(strings.ToLower(string(layer.ReviewMode)))
		if m == ModeFast || m == ModeNormal || m == ModeDeep {
			res.Config.ReviewMode = layer.ReviewMode
			res.AuditTrail.ReviewMode = CascadeConfigEntry[ReviewMode]{
				Value:    m,
				Scope:    scope,
				SourceID: sourceID,
			}
		}
	}

	// Sensitivity
	if layer.Sensitivity != "" {
		s := ReviewSensitivity(strings.ToUpper(string(layer.Sensitivity)))
		res.Config.Sensitivity = layer.Sensitivity
		res.AuditTrail.Sensitivity = CascadeConfigEntry[ReviewSensitivity]{
			Value:    s,
			Scope:    scope,
			SourceID: sourceID,
		}
	}

	// Max Comments Per Review
	if layer.MaxSuggestions > 0 {
		res.Config.MaxSuggestions = layer.MaxSuggestions
		res.AuditTrail.MaxCommentsPerReview = CascadeConfigEntry[int]{
			Value:    layer.MaxSuggestions,
			Scope:    scope,
			SourceID: sourceID,
		}
	}

	// Review Options
	rOpts := layer.ReviewOptions
	if rOpts.Bug || rOpts.Security || rOpts.Performance || rOpts.Architecture || rOpts.BusinessLogic {
		res.Config.ReviewOptions = rOpts
	}

	// Flags (only override if explicitly customized)
	if layer.RequireTicketContext {
		res.Config.RequireTicketContext = true
		res.AuditTrail.RequireTicketContext = CascadeConfigEntry[bool]{
			Value:    true,
			Scope:    scope,
			SourceID: sourceID,
		}
	}

	// Committable suggestions toggle
	res.Config.EnableCommittableSuggestions = layer.EnableCommittableSuggestions
	res.AuditTrail.CommittableSuggestions = CascadeConfigEntry[bool]{
		Value:    layer.EnableCommittableSuggestions,
		Scope:    scope,
		SourceID: sourceID,
	}

	// BYOK Model Slot
	if layer.ByokModel != "" {
		res.Config.ByokModel = layer.ByokModel
	}
	if layer.ByokModelID != "" {
		res.Config.ByokModelID = layer.ByokModelID
	}
	if layer.ResolvedModelSlot != nil {
		res.Config.ResolvedModelSlot = layer.ResolvedModelSlot
	}

	// Ignored file patterns (Union set across layers)
	for _, p := range layer.IgnorePaths {
		clean := strings.TrimSpace(p)
		if clean != "" {
			if _, exists := res.AuditTrail.IgnoredFilePatternsSources[clean]; !exists {
				res.Config.IgnorePaths = append(res.Config.IgnorePaths, clean)
			}
			res.AuditTrail.IgnoredFilePatternsSources[clean] = scope
		}
	}

	// Base branches / included branches
	for _, b := range layer.BaseBranches {
		clean := strings.TrimSpace(b)
		if clean != "" {
			if _, exists := res.AuditTrail.BranchFiltersSources[clean]; !exists {
				res.Config.BaseBranches = append(res.Config.BaseBranches, clean)
			}
			res.AuditTrail.BranchFiltersSources[clean] = scope
		}
	}
}

func (r *CascadeResolver) applyInRepoLayer(
	res *CascadeResolutionResult,
	inRepo *InRepoConfiguration,
) {
	sourceID := ".scandrix"

	// Review Mode Override
	if inRepo.ReviewMode != nil {
		res.Config.ReviewMode = string(*inRepo.ReviewMode)
		res.AuditTrail.ReviewMode = CascadeConfigEntry[ReviewMode]{
			Value:    *inRepo.ReviewMode,
			Scope:    ScopeInRepo,
			SourceID: sourceID,
		}
	}

	// Sensitivity Override
	if inRepo.Sensitivity != nil {
		res.Config.Sensitivity = string(*inRepo.Sensitivity)
		res.AuditTrail.Sensitivity = CascadeConfigEntry[ReviewSensitivity]{
			Value:    *inRepo.Sensitivity,
			Scope:    ScopeInRepo,
			SourceID: sourceID,
		}
	}

	// Max Comments Per Review Override
	if inRepo.MaxCommentsPerReview != nil {
		res.Config.MaxSuggestions = *inRepo.MaxCommentsPerReview
		res.AuditTrail.MaxCommentsPerReview = CascadeConfigEntry[int]{
			Value:    *inRepo.MaxCommentsPerReview,
			Scope:    ScopeInRepo,
			SourceID: sourceID,
		}
	}

	// Review Options Override
	if inRepo.ReviewOptions != nil {
		res.Config.ReviewOptions = *inRepo.ReviewOptions
	}

	// Committable Suggestions
	if inRepo.CommittableSuggestions != nil {
		res.Config.EnableCommittableSuggestions = *inRepo.CommittableSuggestions
		res.AuditTrail.CommittableSuggestions = CascadeConfigEntry[bool]{
			Value:    *inRepo.CommittableSuggestions,
			Scope:    ScopeInRepo,
			SourceID: sourceID,
		}
	}

	// Require Ticket Context
	if inRepo.RequireTicketContext != nil {
		res.Config.RequireTicketContext = *inRepo.RequireTicketContext
		res.AuditTrail.RequireTicketContext = CascadeConfigEntry[bool]{
			Value:    *inRepo.RequireTicketContext,
			Scope:    ScopeInRepo,
			SourceID: sourceID,
		}
	}

	// Ignored File Patterns (Union with In-Repo)
	for _, p := range inRepo.IgnoredFilePatterns {
		clean := strings.TrimSpace(p)
		if clean != "" {
			if _, exists := res.AuditTrail.IgnoredFilePatternsSources[clean]; !exists {
				res.Config.IgnorePaths = append(res.Config.IgnorePaths, clean)
			}
			res.AuditTrail.IgnoredFilePatternsSources[clean] = ScopeInRepo
		}
	}

	// Included Branch Patterns
	for _, b := range inRepo.IncludedBranchPatterns {
		clean := strings.TrimSpace(b)
		if clean != "" {
			if _, exists := res.AuditTrail.BranchFiltersSources[clean]; !exists {
				res.Config.BaseBranches = append(res.Config.BaseBranches, clean)
			}
			res.AuditTrail.BranchFiltersSources[clean] = ScopeInRepo
		}
	}
}

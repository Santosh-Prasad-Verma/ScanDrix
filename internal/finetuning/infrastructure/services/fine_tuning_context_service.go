// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Fine-Tuning Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"sync"

	"github.com/scandrix/backend/internal/finetuning/domain/interfaces"
	reviewdomain "github.com/scandrix/backend/internal/review/domain"
)

// IDrixyFineTuningContextPreparationService defines the interface for preparing review suggestions with fine-tuning.
type IDrixyFineTuningContextPreparationService interface {
	PrepareDrixyFineTuningContext(
		ctx context.Context,
		organizationID string,
		prNumber int,
		repository RepositoryRef,
		suggestionsToAnalyze []*reviewdomain.CodeSuggestion,
		isFineTuningEnabled bool,
		clusterizedSuggestions []interfaces.ClusterizedSuggestion,
	) (*interfaces.FineTuningAnalysisResult, error)
}

// DrixyFineTuningContextPreparationService coordinates suggestion filtering against fine-tuning clusters.
type DrixyFineTuningContextPreparationService struct {
	mu                sync.RWMutex
	fineTuningService *DrixyFineTuningService
}

// NewDrixyFineTuningContextPreparationService creates a new context preparation service.
func NewDrixyFineTuningContextPreparationService(
	fineTuningService *DrixyFineTuningService,
) *DrixyFineTuningContextPreparationService {
	return &DrixyFineTuningContextPreparationService{
		fineTuningService: fineTuningService,
	}
}

// PrepareDrixyFineTuningContext evaluates suggestions against fine-tuned cluster knowledge.
func (s *DrixyFineTuningContextPreparationService) PrepareDrixyFineTuningContext(
	ctx context.Context,
	organizationID string,
	prNumber int,
	repository RepositoryRef,
	suggestionsToAnalyze []*reviewdomain.CodeSuggestion,
	isFineTuningEnabled bool,
	clusterizedSuggestions []interfaces.ClusterizedSuggestion,
) (*interfaces.FineTuningAnalysisResult, error) {
	if len(suggestionsToAnalyze) == 0 {
		return &interfaces.FineTuningAnalysisResult{
			KeepedSuggestions:    []*reviewdomain.CodeSuggestion{},
			DiscardedSuggestions: []*reviewdomain.CodeSuggestion{},
		}, nil
	}

	// If fine tuning is disabled, preserve all suggestions
	if !isFineTuningEnabled {
		return &interfaces.FineTuningAnalysisResult{
			KeepedSuggestions:    suggestionsToAnalyze,
			DiscardedSuggestions: []*reviewdomain.CodeSuggestion{},
		}, nil
	}

	// If no clusters exist, preserve all suggestions
	if len(clusterizedSuggestions) == 0 {
		return &interfaces.FineTuningAnalysisResult{
			KeepedSuggestions:    suggestionsToAnalyze,
			DiscardedSuggestions: []*reviewdomain.CodeSuggestion{},
		}, nil
	}

	if s.fineTuningService == nil {
		return &interfaces.FineTuningAnalysisResult{
			KeepedSuggestions:    suggestionsToAnalyze,
			DiscardedSuggestions: []*reviewdomain.CodeSuggestion{},
		}, nil
	}

	result, err := s.fineTuningService.FineTuningAnalysis(
		ctx,
		organizationID,
		prNumber,
		repository,
		suggestionsToAnalyze,
		clusterizedSuggestions,
	)
	if err != nil {
		// Graceful degradation: never block code review if fine-tuning analysis errors
		return &interfaces.FineTuningAnalysisResult{
			KeepedSuggestions:    suggestionsToAnalyze,
			DiscardedSuggestions: []*reviewdomain.CodeSuggestion{},
		}, nil
	}

	return result, nil
}

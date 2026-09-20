// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Fine-Tuning Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package finetuning

import (
	"github.com/scandrix/backend/internal/finetuning/domain/enums"
	"github.com/scandrix/backend/internal/finetuning/domain/interfaces"
	suggestionembedded "github.com/scandrix/backend/internal/finetuning/domain/suggestion_embedded"
	"github.com/scandrix/backend/internal/finetuning/infrastructure/repositories"
	"github.com/scandrix/backend/internal/finetuning/infrastructure/services"
)

// Re-export domain enums
type FeedbackType = enums.FeedbackType
type FineTuningDecision = enums.FineTuningDecision
type FineTuningType = enums.FineTuningType

const (
	PositiveReaction      = enums.PositiveReaction
	NegativeReaction      = enums.NegativeReaction
	SuggestionImplemented = enums.SuggestionImplemented
	Neutral               = enums.Neutral

	DecisionKeep      = enums.DecisionKeep
	DecisionDiscard   = enums.DecisionDiscard
	DecisionUncertain = enums.DecisionUncertain

	FineTuningTypeGlobal     = enums.FineTuningTypeGlobal
	FineTuningTypeRepository = enums.FineTuningTypeRepository
)

// Re-export domain interfaces and structs
type DrixyFineTuning = interfaces.DrixyFineTuning
type DrixyFineTuningPullRequest = interfaces.DrixyFineTuningPullRequest
type DrixyFineTuningPullRequestRepository = interfaces.DrixyFineTuningPullRequestRepository
type EmbeddingResult = interfaces.EmbeddingResult
type ClusterAnalysis = interfaces.ClusterAnalysis
type ClusterizedSuggestion = interfaces.ClusterizedSuggestion
type ClusterDistribution = interfaces.ClusterDistribution
type FineTuningAnalysisResult = interfaces.FineTuningAnalysisResult

type SuggestionEmbedded = suggestionembedded.SuggestionEmbedded
type SuggestionToEmbed = suggestionembedded.SuggestionToEmbed
type SuggestionFilter = suggestionembedded.SuggestionFilter
type OrganizationRef = suggestionembedded.OrganizationRef
type SuggestionEmbeddedFeedbacks = suggestionembedded.SuggestionEmbeddedFeedbacks
type SuggestionEmbeddedFeedbacksWithLanguage = suggestionembedded.SuggestionEmbeddedFeedbacksWithLanguage
type SuggestionEmbeddedEntity = suggestionembedded.SuggestionEmbeddedEntity
type ISuggestionEmbeddedRepository = suggestionembedded.ISuggestionEmbeddedRepository
type ISuggestionEmbeddedService = suggestionembedded.ISuggestionEmbeddedService

// Re-export infrastructure services and types
type FineTuningConfig = services.FineTuningConfig
type RepositoryRef = services.RepositoryRef
type CodeReviewFeedbackReactions = services.CodeReviewFeedbackReactions
type CodeReviewFeedbackModel = services.CodeReviewFeedbackModel
type PullRequestDataModel = services.PullRequestDataModel
type PullRequestsProvider = services.PullRequestsProvider
type FeedbackProvider = services.FeedbackProvider
type ConfigProvider = services.ConfigProvider
type TextEmbedder = services.TextEmbedder
type DeterministicHasherEmbedder = services.DeterministicHasherEmbedder
type DrixyFineTuningService = services.DrixyFineTuningService
type DrixyFineTuningContextPreparationService = services.DrixyFineTuningContextPreparationService
type IDrixyFineTuningContextPreparationService = services.IDrixyFineTuningContextPreparationService
type SuggestionEmbeddedDatabaseRepository = repositories.SuggestionEmbeddedDatabaseRepository
type SuggestionEmbeddedService = services.SuggestionEmbeddedService
type KMeansOptions = services.KMeansOptions
type KMeansResult = services.KMeansResult

// Constructors and utility functions
var (
	NewSuggestionEmbeddedDatabaseRepository    = repositories.NewSuggestionEmbeddedDatabaseRepository
	NewDeterministicHasherEmbedder             = services.NewDeterministicHasherEmbedder
	NewSuggestionEmbeddedService               = services.NewSuggestionEmbeddedService
	NewDrixyFineTuningService                  = services.NewDrixyFineTuningService
	NewDrixyFineTuningContextPreparationService = services.NewDrixyFineTuningContextPreparationService
	KMeans                                     = services.KMeans
	CosineSimilarity                           = services.CosineSimilarity
	CalculateClusterCentroids                  = services.CalculateClusterCentroids
	DefaultFineTuningConfig                    = services.DefaultFineTuningConfig
)

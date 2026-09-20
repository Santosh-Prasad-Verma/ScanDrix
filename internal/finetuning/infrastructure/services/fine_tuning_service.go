// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Fine-Tuning Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/scandrix/backend/internal/finetuning/domain/enums"
	"github.com/scandrix/backend/internal/finetuning/domain/interfaces"
	suggestionembedded "github.com/scandrix/backend/internal/finetuning/domain/suggestion_embedded"
	reviewdomain "github.com/scandrix/backend/internal/review/domain"
)

const (
	DefaultMaxClusters                  = 50
	DefaultDivisorForClusterQuantity    = 4
	DefaultSimilarityThresholdNegative  = 0.6
	DefaultSimilarityThresholdPositive  = 0.6
	DefaultSimilarityThresholdCluster   = 0.6
	DefaultMinSuggestionsForClustering  = 50

	LabelDrixyRules       = "drixy_rules"
	LabelBreakingChanges = "breaking_changes"
)

var whitespaceRegex = regexp.MustCompile(`\s+`)
var nonAlphanumericRegex = regexp.MustCompile(`[^\w\s\-\_\.\(\)\{\}\[\]]`)

// FineTuningConfig holds tuning parameters.
type FineTuningConfig struct {
	MaxClusters                 int
	DivisorForClusterQuantity   int
	PositiveThreshold           float64
	NegativeThreshold           float64
	SimilarityThresholdCluster  float64
	MinSuggestionsForClustering int
}

func DefaultFineTuningConfig() FineTuningConfig {
	return FineTuningConfig{
		MaxClusters:                 DefaultMaxClusters,
		DivisorForClusterQuantity:   DefaultDivisorForClusterQuantity,
		PositiveThreshold:           DefaultSimilarityThresholdPositive,
		NegativeThreshold:           DefaultSimilarityThresholdNegative,
		SimilarityThresholdCluster:  DefaultSimilarityThresholdCluster,
		MinSuggestionsForClustering: DefaultMinSuggestionsForClustering,
	}
}

// RepositoryRef describes a source control repository.
type RepositoryRef struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
	Language string `json:"language,omitempty"`
}

// CodeReviewFeedbackReactions stores emoji reactions.
type CodeReviewFeedbackReactions struct {
	ThumbsUp   int `json:"thumbsUp"`
	ThumbsDown int `json:"thumbsDown"`
}

// CodeReviewFeedbackModel represents user feedback on a suggestion.
type CodeReviewFeedbackModel struct {
	SuggestionID string                      `json:"suggestionId"`
	Reactions    CodeReviewFeedbackReactions `json:"reactions"`
}

// PullRequestDataModel represents PR suggestions to be synchronized.
type PullRequestDataModel struct {
	UUID           string                                 `json:"uuid"`
	Number         int                                    `json:"number"`
	OrganizationID string                                 `json:"organizationId"`
	Repository     RepositoryRef                          `json:"repository"`
	Suggestions    []suggestionembedded.SuggestionToEmbed `json:"suggestions"`
}

// PullRequestsProvider defines pull request access for fine tuning sync.
type PullRequestsProvider interface {
	FindPullRequestsToSync(ctx context.Context, organizationID string, repo RepositoryRef) ([]PullRequestDataModel, error)
	UpdateSyncedSuggestionsFlag(ctx context.Context, prNumbers []int, repoID string, orgID string, synced bool) error
}

// FeedbackProvider defines feedback access for fine tuning sync.
type FeedbackProvider interface {
	FindFeedbackByOrgAndRepo(ctx context.Context, organizationID string, repoID string, synced bool) ([]CodeReviewFeedbackModel, error)
	UpdateSyncedFeedbackFlag(ctx context.Context, organizationID string, suggestionIDs []string, synced bool) error
}

// ConfigProvider provides dynamic parameter lookup.
type ConfigProvider interface {
	GetFineTuningConfig(ctx context.Context) (FineTuningConfig, error)
}

// DrixyFineTuningService implements complete fine-tuning analysis and suggestion clustering.
type DrixyFineTuningService struct {
	mu                         sync.RWMutex
	pullRequestsProvider       PullRequestsProvider
	feedbackProvider           FeedbackProvider
	suggestionEmbeddedService  suggestionembedded.ISuggestionEmbeddedService
	configProvider             ConfigProvider
	defaultConfig              FineTuningConfig
}

// NewDrixyFineTuningService creates a new DrixyFineTuningService instance.
func NewDrixyFineTuningService(
	pullRequestsProvider PullRequestsProvider,
	feedbackProvider FeedbackProvider,
	suggestionEmbeddedService suggestionembedded.ISuggestionEmbeddedService,
	configProvider ConfigProvider,
	customConfig *FineTuningConfig,
) *DrixyFineTuningService {
	cfg := DefaultFineTuningConfig()
	if customConfig != nil {
		if customConfig.MaxClusters > 0 {
			cfg.MaxClusters = customConfig.MaxClusters
		}
		if customConfig.DivisorForClusterQuantity > 0 {
			cfg.DivisorForClusterQuantity = customConfig.DivisorForClusterQuantity
		}
		if customConfig.PositiveThreshold > 0 {
			cfg.PositiveThreshold = customConfig.PositiveThreshold
		}
		if customConfig.NegativeThreshold > 0 {
			cfg.NegativeThreshold = customConfig.NegativeThreshold
		}
		if customConfig.SimilarityThresholdCluster > 0 {
			cfg.SimilarityThresholdCluster = customConfig.SimilarityThresholdCluster
		}
		if customConfig.MinSuggestionsForClustering > 0 {
			cfg.MinSuggestionsForClustering = customConfig.MinSuggestionsForClustering
		}
	}

	return &DrixyFineTuningService{
		pullRequestsProvider:      pullRequestsProvider,
		feedbackProvider:          feedbackProvider,
		suggestionEmbeddedService: suggestionEmbeddedService,
		configProvider:            configProvider,
		defaultConfig:             cfg,
	}
}

// StartAnalysis synchronizes historical data, checks dataset volume, and runs k-means clustering.
func (s *DrixyFineTuningService) StartAnalysis(
	ctx context.Context,
	organizationID string,
	repository RepositoryRef,
	prNumber int,
	language string,
) ([]interfaces.ClusterizedSuggestion, error) {
	// 1. Synchronize suggestions from closed PRs and feedbacks
	_, _ = s.SyncronizeSuggestions(ctx, organizationID, repository, prNumber)

	// 2. Verify fine tuning dataset availability
	suggestions, err := s.verifyFineTuningType(ctx, organizationID, repository, language)
	if err != nil || len(suggestions) == 0 {
		return []interfaces.ClusterizedSuggestion{}, nil
	}

	// 3. Clusterize suggestions
	return s.ClusterizeSuggestions(ctx, suggestions)
}

// FineTuningAnalysis categorizes candidate suggestions into Keep and Discard based on cluster similarity.
func (s *DrixyFineTuningService) FineTuningAnalysis(
	ctx context.Context,
	organizationID string,
	prNumber int,
	repository RepositoryRef,
	suggestionsToAnalyze []*reviewdomain.CodeSuggestion,
	mainClusterizedSuggestions []interfaces.ClusterizedSuggestion,
) (*interfaces.FineTuningAnalysisResult, error) {
	if len(suggestionsToAnalyze) == 0 || len(mainClusterizedSuggestions) == 0 {
		return &interfaces.FineTuningAnalysisResult{
			KeepedSuggestions:    suggestionsToAnalyze,
			DiscardedSuggestions: []*reviewdomain.CodeSuggestion{},
		}, nil
	}

	// Embed candidate suggestions
	embeddedToAnalyze, err := s.suggestionEmbeddedService.EmbedSuggestionsForSuggestionToEmbed(
		ctx,
		suggestionsToAnalyze,
		organizationID,
		prNumber,
		repository.ID,
		repository.FullName,
	)
	if err != nil || len(embeddedToAnalyze) == 0 {
		return &interfaces.FineTuningAnalysisResult{
			KeepedSuggestions:    suggestionsToAnalyze,
			DiscardedSuggestions: []*reviewdomain.CodeSuggestion{},
		}, nil
	}

	cfg := s.getConfig(ctx)
	var keeped []*reviewdomain.CodeSuggestion
	var discarded []*reviewdomain.CodeSuggestion

	for idx, candidate := range embeddedToAnalyze {
		originalCodeSuggestion := suggestionsToAnalyze[idx]

		// Special labels rule: drixy_rules and breaking_changes are ALWAYS kept
		if candidate.Label == LabelDrixyRules || candidate.Label == LabelBreakingChanges {
			keeped = append(keeped, originalCodeSuggestion)
			continue
		}

		clusterizedSuggestions, err := s.defineWhichClusterShouldBeUsed(
			ctx,
			organizationID,
			mainClusterizedSuggestions,
			candidate,
			repository,
			prNumber,
		)
		if err != nil || len(clusterizedSuggestions) < cfg.MinSuggestionsForClustering {
			keeped = append(keeped, originalCodeSuggestion)
			continue
		}

		decision := s.compareSuggestionsWithClusters(candidate, clusterizedSuggestions, cfg)
		if decision == enums.DecisionDiscard {
			discarded = append(discarded, originalCodeSuggestion)
		} else {
			// DecisionKeep or DecisionUncertain
			keeped = append(keeped, originalCodeSuggestion)
		}
	}

	return &interfaces.FineTuningAnalysisResult{
		KeepedSuggestions:    keeped,
		DiscardedSuggestions: discarded,
	}, nil
}

// ClusterizeSuggestions groups vectorized suggestions into clusters using k-means++.
func (s *DrixyFineTuningService) ClusterizeSuggestions(
	ctx context.Context,
	suggestions []*suggestionembedded.SuggestionEmbeddedEntity,
) ([]interfaces.ClusterizedSuggestion, error) {
	if len(suggestions) == 0 {
		return []interfaces.ClusterizedSuggestion{}, nil
	}

	// Filter suggestions with valid vector embeddings and matching dimensions
	var expectedDim int = -1
	var validSuggestions []*suggestionembedded.SuggestionEmbeddedEntity
	var validVectors [][]float64

	for _, item := range suggestions {
		vec := item.SuggestionEmbed()
		if len(vec) == 0 {
			continue
		}
		if expectedDim == -1 {
			expectedDim = len(vec)
		}
		if len(vec) != expectedDim {
			continue
		}
		validSuggestions = append(validSuggestions, item)
		validVectors = append(validVectors, vec)
	}

	if len(validSuggestions) == 0 {
		return []interfaces.ClusterizedSuggestion{}, nil
	}

	cfg := s.getConfig(ctx)
	divisor := cfg.DivisorForClusterQuantity
	if divisor <= 0 {
		divisor = DefaultDivisorForClusterQuantity
	}

	calculatedClusters := int(math.Ceil(float64(len(validVectors)) / float64(divisor)))
	k := len(validVectors)
	if calculatedClusters < k {
		k = calculatedClusters
	}
	if cfg.MaxClusters > 0 && cfg.MaxClusters < k {
		k = cfg.MaxClusters
	}
	if k < 1 {
		k = 1
	}

	result, err := KMeans(validVectors, k, &KMeansOptions{
		MaxIterations:  1,
		Initialization: "kmeans++",
	})
	if err != nil {
		return nil, err
	}

	clusterized := make([]interfaces.ClusterizedSuggestion, len(validSuggestions))
	for i, item := range validSuggestions {
		clusterID := 0
		if i < len(result.Clusters) {
			clusterID = result.Clusters[i]
		}

		clusterized[i] = interfaces.ClusterizedSuggestion{
			Cluster:            clusterID,
			OriginalSuggestion: item.ToObject(),
			Language:           item.Language(),
		}
	}

	return clusterized, nil
}

// SyncronizeSuggestions scans repository history for closed PRs, extracts feedbacks, and inserts embeddings.
func (s *DrixyFineTuningService) SyncronizeSuggestions(
	ctx context.Context,
	organizationID string,
	repository RepositoryRef,
	prNumber int,
) ([]*suggestionembedded.SuggestionEmbeddedEntity, error) {
	if s.pullRequestsProvider == nil || s.suggestionEmbeddedService == nil {
		return []*suggestionembedded.SuggestionEmbeddedEntity{}, nil
	}

	pullRequests, err := s.pullRequestsProvider.FindPullRequestsToSync(ctx, organizationID, repository)
	if err != nil || len(pullRequests) == 0 {
		return []*suggestionembedded.SuggestionEmbeddedEntity{}, nil
	}

	var allSuggestions []suggestionembedded.SuggestionToEmbed
	for _, pr := range pullRequests {
		allSuggestions = append(allSuggestions, pr.Suggestions...)
	}

	if len(allSuggestions) == 0 {
		return []*suggestionembedded.SuggestionEmbeddedEntity{}, nil
	}

	// 1. Get feedbacks
	var feedbacks []CodeReviewFeedbackModel
	if s.feedbackProvider != nil {
		feedbacks, _ = s.feedbackProvider.FindFeedbackByOrgAndRepo(ctx, organizationID, repository.ID, false)
	}

	feedbackMap := make(map[string]CodeReviewFeedbackModel)
	for _, fb := range feedbacks {
		feedbackMap[fb.SuggestionID] = fb
	}

	// 2. Separate into implemented and feedback-received suggestions
	var suggestionsWithFeedback []suggestionembedded.SuggestionToEmbed
	var implementedSuggestions []suggestionembedded.SuggestionToEmbed

	for _, sugg := range allSuggestions {
		if sugg.ImplementationStatus == "implemented" {
			implementedSuggestions = append(implementedSuggestions, sugg)
			continue
		}

		if fb, exists := feedbackMap[sugg.ID]; exists {
			sugg.FeedbackType = string(s.IdentifyFeedbackType(fb))
			suggestionsWithFeedback = append(suggestionsWithFeedback, sugg)
		}
	}

	if len(implementedSuggestions) == 0 && len(suggestionsWithFeedback) == 0 {
		return []*suggestionembedded.SuggestionEmbeddedEntity{}, nil
	}

	// 3. Remove duplicates and neutral suggestions
	refined := s.removeDuplicateAndNeutralSuggestions(suggestionsWithFeedback, implementedSuggestions)

	// 4. Filter out special labels (rules & breaking changes should not be trained into k-means)
	var toNormalize []suggestionembedded.SuggestionToEmbed
	for _, item := range append(refined.UniqueSuggestionsWithFeedback, refined.UniqueImplementedSuggestions...) {
		if item.Label == LabelDrixyRules || item.Label == LabelBreakingChanges {
			continue
		}
		if item.ImprovedCode == "" {
			continue
		}

		item.SuggestionContent = s.NormalizeText(item.SuggestionContent)
		item.Label = s.NormalizeText(item.Label)
		item.Severity = s.NormalizeText(item.Severity)
		toNormalize = append(toNormalize, item)
	}

	if len(toNormalize) == 0 {
		return []*suggestionembedded.SuggestionEmbeddedEntity{}, nil
	}

	// 5. Bulk insert embedded suggestions
	created, err := s.suggestionEmbeddedService.BulkCreateFromMongoData(ctx, toNormalize)
	if err != nil {
		return nil, err
	}

	// 6. Update synced flags
	var prNumbers []int
	seenPRs := make(map[int]bool)
	for _, pr := range pullRequests {
		if pr.Number != prNumber && !seenPRs[pr.Number] {
			seenPRs[pr.Number] = true
			prNumbers = append(prNumbers, pr.Number)
		}
	}

	if len(prNumbers) > 0 {
		_ = s.pullRequestsProvider.UpdateSyncedSuggestionsFlag(ctx, prNumbers, repository.ID, organizationID, true)
	}

	if s.feedbackProvider != nil && len(created) > 0 {
		var suggestionIDs []string
		for _, entity := range created {
			suggestionIDs = append(suggestionIDs, entity.SuggestionID())
		}
		_ = s.feedbackProvider.UpdateSyncedFeedbackFlag(ctx, organizationID, suggestionIDs, true)
	}

	return created, nil
}

// NormalizeText cleans text for vector embedding comparison.
func (s *DrixyFineTuningService) NormalizeText(text string) string {
	if text == "" {
		return ""
	}
	t := strings.ToLower(text)
	t = nonAlphanumericRegex.ReplaceAllString(t, " ")
	t = whitespaceRegex.ReplaceAllString(t, " ")
	return strings.TrimSpace(t)
}

// IdentifyFeedbackType categorizes reaction counts into FeedbackType.
func (s *DrixyFineTuningService) IdentifyFeedbackType(feedback CodeReviewFeedbackModel) enums.FeedbackType {
	up := feedback.Reactions.ThumbsUp
	down := feedback.Reactions.ThumbsDown

	if up > 0 && up > down {
		return enums.PositiveReaction
	}
	if down > 0 && down > up {
		return enums.NegativeReaction
	}
	return enums.Neutral
}

type refinedSuggestions struct {
	UniqueSuggestionsWithFeedback []suggestionembedded.SuggestionToEmbed
	UniqueImplementedSuggestions  []suggestionembedded.SuggestionToEmbed
}

func (s *DrixyFineTuningService) removeDuplicateAndNeutralSuggestions(
	suggestionsWithFeedback []suggestionembedded.SuggestionToEmbed,
	implementedSuggestions []suggestionembedded.SuggestionToEmbed,
) refinedSuggestions {
	implementedIDs := make(map[string]bool)
	for _, s := range implementedSuggestions {
		implementedIDs[s.ID] = true
	}

	var uniqueFeedback []suggestionembedded.SuggestionToEmbed
	for _, sugg := range suggestionsWithFeedback {
		if !implementedIDs[sugg.ID] && sugg.FeedbackType != string(enums.Neutral) {
			uniqueFeedback = append(uniqueFeedback, sugg)
		}
	}

	var uniqueImplemented []suggestionembedded.SuggestionToEmbed
	for _, sugg := range implementedSuggestions {
		sugg.FeedbackType = string(enums.SuggestionImplemented)
		uniqueImplemented = append(uniqueImplemented, sugg)
	}

	return refinedSuggestions{
		UniqueSuggestionsWithFeedback: uniqueFeedback,
		UniqueImplementedSuggestions:  uniqueImplemented,
	}
}

func (s *DrixyFineTuningService) verifyFineTuningType(
	ctx context.Context,
	organizationID string,
	repository RepositoryRef,
	language string,
) ([]*suggestionembedded.SuggestionEmbeddedEntity, error) {
	cfg := s.getConfig(ctx)
	minRequired := cfg.MinSuggestionsForClustering

	// 1. Check repository level
	repoFilter := suggestionembedded.SuggestionFilter{
		OrganizationID:     organizationID,
		RepositoryID:       repository.ID,
		RepositoryFullName: repository.FullName,
		Language:           strings.ToLower(language),
	}
	repoSuggestions, err := s.suggestionEmbeddedService.Find(ctx, repoFilter)
	if err == nil && len(repoSuggestions) >= minRequired {
		return repoSuggestions, nil
	}

	// 2. Fall back to global organization level
	globalFilter := suggestionembedded.SuggestionFilter{
		OrganizationID: organizationID,
		Language:       strings.ToLower(language),
	}
	globalSuggestions, err := s.suggestionEmbeddedService.Find(ctx, globalFilter)
	if err == nil && len(globalSuggestions) >= minRequired {
		return globalSuggestions, nil
	}

	return nil, nil
}

func (s *DrixyFineTuningService) defineWhichClusterShouldBeUsed(
	ctx context.Context,
	organizationID string,
	mainClusterizedSuggestions []interfaces.ClusterizedSuggestion,
	newSuggestion suggestionembedded.SuggestionToEmbed,
	repository RepositoryRef,
	prNumber int,
) ([]interfaces.ClusterizedSuggestion, error) {
	if len(mainClusterizedSuggestions) > 0 &&
		strings.EqualFold(newSuggestion.Language, mainClusterizedSuggestions[0].Language) {
		return mainClusterizedSuggestions, nil
	}

	return s.StartAnalysis(ctx, organizationID, repository, prNumber, newSuggestion.Language)
}

func (s *DrixyFineTuningService) compareSuggestionsWithClusters(
	newSuggestion suggestionembedded.SuggestionToEmbed,
	existingClusterizedSuggestions []interfaces.ClusterizedSuggestion,
	cfg FineTuningConfig,
) enums.FineTuningDecision {
	// 1. Calculate cluster centroids
	centroids := CalculateClusterCentroids(existingClusterizedSuggestions)
	if len(centroids) == 0 {
		return enums.DecisionUncertain
	}

	// 2. Compare with centroids
	type clusterSim struct {
		clusterID  int
		similarity float64
	}
	var sims []clusterSim

	for clusterID, centroid := range centroids {
		sim := CosineSimilarity(newSuggestion.SuggestionEmbed, centroid)
		sims = append(sims, clusterSim{clusterID: clusterID, similarity: sim})
	}

	sort.Slice(sims, func(i, j int) bool {
		return sims[i].similarity > sims[j].similarity
	})

	mostSimilar := sims[0]
	if mostSimilar.similarity < cfg.SimilarityThresholdCluster {
		return enums.DecisionUncertain
	}

	// 3. Analyze cluster feedback
	return s.analyzeClusterFeedback(existingClusterizedSuggestions, mostSimilar.clusterID, newSuggestion.SuggestionEmbed, cfg)
}

func (s *DrixyFineTuningService) analyzeClusterFeedback(
	existing []interfaces.ClusterizedSuggestion,
	clusterID int,
	newSuggestionEmbed []float64,
	cfg FineTuningConfig,
) enums.FineTuningDecision {
	var clusterSuggestions []interfaces.ClusterizedSuggestion
	for _, item := range existing {
		if item.Cluster == clusterID {
			clusterSuggestions = append(clusterSuggestions, item)
		}
	}

	if len(clusterSuggestions) == 0 {
		return enums.DecisionUncertain
	}

	// Check unanimous feedback
	unanimous := s.unanimousFeedbackInCluster(clusterSuggestions)
	if unanimous != enums.DecisionUncertain {
		return unanimous
	}

	// Analyze similarity distribution
	type scoredSuggestion struct {
		similarity float64
		isPositive bool
	}

	var scored []scoredSuggestion
	for _, item := range clusterSuggestions {
		sim := CosineSimilarity(newSuggestionEmbed, item.OriginalSuggestion.SuggestionEmbed)
		isPos := item.OriginalSuggestion.FeedbackType == string(enums.PositiveReaction) ||
			item.OriginalSuggestion.FeedbackType == string(enums.SuggestionImplemented)
		scored = append(scored, scoredSuggestion{similarity: sim, isPositive: isPos})
	}

	var keepCount, discardCount int
	for _, sc := range scored {
		if sc.isPositive && sc.similarity >= cfg.PositiveThreshold {
			keepCount++
		} else if !sc.isPositive && sc.similarity >= cfg.NegativeThreshold {
			discardCount++
		}
	}

	if keepCount > 0 && keepCount > discardCount {
		return enums.DecisionKeep
	}
	if discardCount > 0 && discardCount > keepCount {
		return enums.DecisionDiscard
	}

	return enums.DecisionUncertain
}

func (s *DrixyFineTuningService) unanimousFeedbackInCluster(
	clusterSuggestions []interfaces.ClusterizedSuggestion,
) enums.FineTuningDecision {
	allPositive := true
	allNegative := true

	for _, item := range clusterSuggestions {
		ft := item.OriginalSuggestion.FeedbackType
		if ft == string(enums.PositiveReaction) || ft == string(enums.SuggestionImplemented) {
			allNegative = false
		} else if ft == string(enums.NegativeReaction) {
			allPositive = false
		} else {
			allPositive = false
			allNegative = false
		}
	}

	if allPositive {
		return enums.DecisionKeep
	}
	if allNegative {
		return enums.DecisionDiscard
	}

	return enums.DecisionUncertain
}

func (s *DrixyFineTuningService) getConfig(ctx context.Context) FineTuningConfig {
	if s.configProvider != nil {
		if cfg, err := s.configProvider.GetFineTuningConfig(ctx); err == nil {
			return cfg
		}
	}
	return s.defaultConfig
}

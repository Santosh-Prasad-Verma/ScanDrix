// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Fine-Tuning Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/finetuning/domain/enums"
	suggestionembedded "github.com/scandrix/backend/internal/finetuning/domain/suggestion_embedded"
	reviewdomain "github.com/scandrix/backend/internal/review/domain"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// TextEmbedder defines the capability to compute vector embeddings from text.
type TextEmbedder interface {
	Embed(ctx context.Context, text string) ([]float64, error)
}

// DeterministicHasherEmbedder provides high-quality deterministic vector embeddings for testing & offline use.
type DeterministicHasherEmbedder struct {
	Dimensions int
}

func NewDeterministicHasherEmbedder(dimensions int) *DeterministicHasherEmbedder {
	if dimensions <= 0 {
		dimensions = 64
	}
	return &DeterministicHasherEmbedder{Dimensions: dimensions}
}

func (e *DeterministicHasherEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	vec := make([]float64, e.Dimensions)
	h := sha256.New()
	h.Write([]byte(text))
	baseHash := h.Sum(nil)

	for i := 0; i < e.Dimensions; i++ {
		// Use chunks of hash with index salt
		h.Reset()
		h.Write(baseHash)
		idxBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(idxBytes, uint32(i))
		h.Write(idxBytes)
		sum := h.Sum(nil)

		val := float64(binary.LittleEndian.Uint32(sum[:4])) / float64(math.MaxUint32)
		vec[i] = (val * 2.0) - 1.0 // Normalize to [-1.0, 1.0]
	}

	// L2 normalize
	var norm float64
	for _, v := range vec {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}

	return vec, nil
}

// SuggestionEmbeddedService implements ISuggestionEmbeddedService.
type SuggestionEmbeddedService struct {
	repo     suggestionembedded.ISuggestionEmbeddedRepository
	embedder TextEmbedder
}

func NewSuggestionEmbeddedService(
	repo suggestionembedded.ISuggestionEmbeddedRepository,
	embedder TextEmbedder,
) *SuggestionEmbeddedService {
	if embedder == nil {
		embedder = NewDeterministicHasherEmbedder(64)
	}
	return &SuggestionEmbeddedService{
		repo:     repo,
		embedder: embedder,
	}
}

// BulkInsert proxies bulk insert to the repository.
func (s *SuggestionEmbeddedService) BulkInsert(
	ctx context.Context,
	entities []suggestionembedded.SuggestionEmbedded,
) ([]*suggestionembedded.SuggestionEmbeddedEntity, error) {
	return s.repo.BulkInsert(ctx, entities)
}

// Create inserts a single entity.
func (s *SuggestionEmbeddedService) Create(
	ctx context.Context,
	entity suggestionembedded.SuggestionEmbedded,
) (*suggestionembedded.SuggestionEmbeddedEntity, error) {
	return s.repo.Create(ctx, entity)
}

// Find retrieves entities matching filter.
func (s *SuggestionEmbeddedService) Find(
	ctx context.Context,
	filter suggestionembedded.SuggestionFilter,
) ([]*suggestionembedded.SuggestionEmbeddedEntity, error) {
	return s.repo.Find(ctx, filter)
}

// FindOne retrieves by suggestion ID.
func (s *SuggestionEmbeddedService) FindOne(
	ctx context.Context,
	suggestionID string,
) (*suggestionembedded.SuggestionEmbeddedEntity, error) {
	return s.repo.FindOne(ctx, suggestionID)
}

// FindByID retrieves by UUID.
func (s *SuggestionEmbeddedService) FindByID(
	ctx context.Context,
	uuid string,
) (*suggestionembedded.SuggestionEmbeddedEntity, error) {
	return s.repo.FindByID(ctx, uuid)
}

// Update updates an existing record.
func (s *SuggestionEmbeddedService) Update(
	ctx context.Context,
	suggestionID string,
	data suggestionembedded.SuggestionEmbedded,
) (*suggestionembedded.SuggestionEmbeddedEntity, error) {
	return s.repo.Update(ctx, suggestionID, data)
}

// BulkCreateFromMongoData converts raw suggestions, computes vector embeddings, and stores them in bulk.
func (s *SuggestionEmbeddedService) BulkCreateFromMongoData(
	ctx context.Context,
	suggestions []suggestionembedded.SuggestionToEmbed,
) ([]*suggestionembedded.SuggestionEmbeddedEntity, error) {
	var cleanSuggestions []suggestionembedded.SuggestionToEmbed
	for _, sugg := range suggestions {
		if s.isValidSuggestion(sugg) {
			cleanSuggestions = append(cleanSuggestions, sugg)
		}
	}

	if len(cleanSuggestions) == 0 {
		return []*suggestionembedded.SuggestionEmbeddedEntity{}, nil
	}

	var toInsert []suggestionembedded.SuggestionEmbedded
	for _, sugg := range cleanSuggestions {
		embedded, err := s.embedSuggestionToSaveData(ctx, sugg)
		if err != nil || embedded == nil {
			continue
		}
		toInsert = append(toInsert, *embedded)
	}

	if len(toInsert) == 0 {
		return []*suggestionembedded.SuggestionEmbeddedEntity{}, nil
	}

	return s.repo.BulkInsert(ctx, toInsert)
}

// FindByLanguage finds suggestions for a given programming language.
func (s *SuggestionEmbeddedService) FindByLanguage(
	ctx context.Context,
	language string,
) ([]*suggestionembedded.SuggestionEmbeddedEntity, error) {
	return s.repo.Find(ctx, suggestionembedded.SuggestionFilter{
		Language: strings.ToLower(language),
	})
}

// FindByFeedbackType finds suggestions for a given feedback type.
func (s *SuggestionEmbeddedService) FindByFeedbackType(
	ctx context.Context,
	feedbackType string,
) ([]*suggestionembedded.SuggestionEmbeddedEntity, error) {
	return s.repo.Find(ctx, suggestionembedded.SuggestionFilter{
		FeedbackType: feedbackType,
	})
}

// GetByOrganization aggregates feedback statistics for an entire organization.
func (s *SuggestionEmbeddedService) GetByOrganization(
	ctx context.Context,
	organizationID string,
) (*suggestionembedded.SuggestionEmbeddedFeedbacks, error) {
	results, err := s.repo.Find(ctx, suggestionembedded.SuggestionFilter{
		OrganizationID: organizationID,
	})
	if err != nil {
		return nil, err
	}
	return s.countFeedbacks(results), nil
}

// GetByRepositoryAndOrganization aggregates feedback statistics for a specific repository.
func (s *SuggestionEmbeddedService) GetByRepositoryAndOrganization(
	ctx context.Context,
	repositoryID string,
	organizationID string,
) (*suggestionembedded.SuggestionEmbeddedFeedbacks, error) {
	results, err := s.repo.Find(ctx, suggestionembedded.SuggestionFilter{
		RepositoryID:   repositoryID,
		OrganizationID: organizationID,
	})
	if err != nil {
		return nil, err
	}
	return s.countFeedbacks(results), nil
}

// GetByOrganizationWithLanguages partitions feedback metrics by language for an organization.
func (s *SuggestionEmbeddedService) GetByOrganizationWithLanguages(
	ctx context.Context,
	organizationID string,
) (*suggestionembedded.SuggestionEmbeddedFeedbacksWithLanguage, error) {
	results, err := s.repo.Find(ctx, suggestionembedded.SuggestionFilter{
		OrganizationID: organizationID,
	})
	if err != nil {
		return nil, err
	}
	return s.countWithLanguages(results), nil
}

// GetByRepositoryAndOrganizationWithLanguages partitions feedback metrics by language for a repository.
func (s *SuggestionEmbeddedService) GetByRepositoryAndOrganizationWithLanguages(
	ctx context.Context,
	repositoryID string,
	organizationID string,
) (*suggestionembedded.SuggestionEmbeddedFeedbacksWithLanguage, error) {
	results, err := s.repo.Find(ctx, suggestionembedded.SuggestionFilter{
		RepositoryID:   repositoryID,
		OrganizationID: organizationID,
	})
	if err != nil {
		return nil, err
	}
	return s.countWithLanguages(results), nil
}

// EmbedSuggestionsForSuggestionToEmbed computes embeddings for code suggestions.
func (s *SuggestionEmbeddedService) EmbedSuggestionsForSuggestionToEmbed(
	ctx context.Context,
	codeSuggestions []*reviewdomain.CodeSuggestion,
	organizationID string,
	prNumber int,
	repositoryID string,
	repositoryFullName string,
) ([]suggestionembedded.SuggestionToEmbed, error) {
	var embeddedSuggestions []suggestionembedded.SuggestionToEmbed

	for _, suggestion := range codeSuggestions {
		if suggestion == nil {
			continue
		}

		content := suggestion.SuggestionContent
		if content == "" {
			content = suggestion.Description
		}
		summary := suggestion.OneSentenceSummary
		if summary == "" {
			summary = suggestion.Description
		}

		textToEmbed := fmt.Sprintf("%s %s %s", content, summary, suggestion.Label)
		vec, err := s.embedder.Embed(ctx, textToEmbed)
		if err != nil || len(vec) == 0 {
			continue
		}

		item := suggestionembedded.SuggestionToEmbed{
			ID:                 suggestion.ID.String(),
			SuggestionContent:  content,
			OneSentenceSummary: summary,
			Label:              suggestion.Label,
			Severity:           string(suggestion.Severity),
			FeedbackType:       string(enums.Neutral),
			ImprovedCode:       suggestion.ImprovedCode,
			Language:           suggestion.Language,
			SuggestionEmbed:    vec,
			OrganizationID:     organizationID,
			PullRequest: suggestionembedded.PullRequestRefToEmbed{
				Number: prNumber,
				Repository: struct {
					ID       string `json:"id"`
					FullName string `json:"fullName"`
				}{
					ID:       repositoryID,
					FullName: repositoryFullName,
				},
			},
		}

		embeddedSuggestions = append(embeddedSuggestions, item)
	}

	return embeddedSuggestions, nil
}

func (s *SuggestionEmbeddedService) embedSuggestionToSaveData(
	ctx context.Context,
	suggestion suggestionembedded.SuggestionToEmbed,
) (*suggestionembedded.SuggestionEmbedded, error) {
	if suggestion.SuggestionContent == "" ||
		suggestion.OneSentenceSummary == "" ||
		suggestion.Label == "" ||
		suggestion.Severity == "" ||
		suggestion.FeedbackType == "" {
		return nil, nil
	}

	textToEmbed := fmt.Sprintf("%s %s %s", suggestion.SuggestionContent, suggestion.OneSentenceSummary, suggestion.Label)
	vec, err := s.embedder.Embed(ctx, textToEmbed)
	if err != nil || len(vec) == 0 {
		return nil, err
	}

	return &suggestionembedded.SuggestionEmbedded{
		SuggestionID:       suggestion.ID,
		SuggestionEmbed:    vec,
		PullRequestNumber:  suggestion.PullRequest.Number,
		RepositoryID:       suggestion.PullRequest.Repository.ID,
		RepositoryFullName: suggestion.PullRequest.Repository.FullName,
		Organization: &suggestionembedded.OrganizationRef{
			UUID: suggestion.OrganizationID,
		},
		Label:              suggestion.Label,
		Severity:           suggestion.Severity,
		FeedbackType:       suggestion.FeedbackType,
		ImprovedCode:       suggestion.ImprovedCode,
		SuggestionContent:  suggestion.SuggestionContent,
		OneSentenceSummary: suggestion.OneSentenceSummary,
		Language:           strings.ToLower(suggestion.Language),
	}, nil
}

func (s *SuggestionEmbeddedService) isValidSuggestion(sugg suggestionembedded.SuggestionToEmbed) bool {
	return sugg.ID != "" &&
		sugg.SuggestionContent != "" &&
		sugg.OneSentenceSummary != "" &&
		sugg.Label != "" &&
		sugg.Severity != "" &&
		sugg.FeedbackType != ""
}

func (s *SuggestionEmbeddedService) countFeedbacks(
	suggestions []*suggestionembedded.SuggestionEmbeddedEntity,
) *suggestionembedded.SuggestionEmbeddedFeedbacks {
	posCount := 0
	negCount := 0

	for _, item := range suggestions {
		ft := item.FeedbackType()
		if ft == string(enums.PositiveReaction) || ft == string(enums.SuggestionImplemented) {
			posCount++
		} else if ft == string(enums.NegativeReaction) {
			negCount++
		}
	}

	return &suggestionembedded.SuggestionEmbeddedFeedbacks{
		PositiveFeedbacks: posCount,
		NegativeFeedbacks: negCount,
		Total:             len(suggestions),
	}
}

func (s *SuggestionEmbeddedService) countWithLanguages(
	suggestions []*suggestionembedded.SuggestionEmbeddedEntity,
) *suggestionembedded.SuggestionEmbeddedFeedbacksWithLanguage {
	posMap := make(map[string]int)
	negMap := make(map[string]int)
	posTotal := 0
	negTotal := 0

	for _, item := range suggestions {
		ft := item.FeedbackType()
		lang := item.Language()

		if ft == string(enums.PositiveReaction) || ft == string(enums.SuggestionImplemented) {
			posTotal++
			if lang != "" {
				posMap[lang]++
			}
		} else if ft == string(enums.NegativeReaction) {
			negTotal++
			if lang != "" {
				negMap[lang]++
			}
		}
	}

	posLangCounts := make([]suggestionembedded.LanguageCount, 0, len(posMap))
	for l, c := range posMap {
		posLangCounts = append(posLangCounts, suggestionembedded.LanguageCount{Language: l, Count: c})
	}

	negLangCounts := make([]suggestionembedded.LanguageCount, 0, len(negMap))
	for l, c := range negMap {
		negLangCounts = append(negLangCounts, suggestionembedded.LanguageCount{Language: l, Count: c})
	}

	return &suggestionembedded.SuggestionEmbeddedFeedbacksWithLanguage{
		PositiveFeedbacks: suggestionembedded.LanguageFeedbackGroup{
			Language: posLangCounts,
			Total:    posTotal,
		},
		NegativeFeedbacks: suggestionembedded.LanguageFeedbackGroup{
			Language: negLangCounts,
			Total:    negTotal,
		},
		Total: len(suggestions),
	}
}

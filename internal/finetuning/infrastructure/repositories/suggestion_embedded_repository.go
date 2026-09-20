// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Fine-Tuning Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package repositories

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/google/uuid"

	suggestionembedded "github.com/scandrix/backend/internal/finetuning/domain/suggestion_embedded"
)

var (
	ErrSuggestionNotFound = errors.New("suggestion embedding not found")
	ErrInvalidSuggestionID = errors.New("suggestion ID is required")
)

// SuggestionEmbeddedDatabaseRepository is a thread-safe repository implementing ISuggestionEmbeddedRepository.
type SuggestionEmbeddedDatabaseRepository struct {
	mu             sync.RWMutex
	byUUID         map[string]*suggestionembedded.SuggestionEmbeddedEntity
	bySuggestionID map[string]string // suggestionID -> UUID
}

// NewSuggestionEmbeddedDatabaseRepository creates a new repository instance.
func NewSuggestionEmbeddedDatabaseRepository() *SuggestionEmbeddedDatabaseRepository {
	return &SuggestionEmbeddedDatabaseRepository{
		byUUID:         make(map[string]*suggestionembedded.SuggestionEmbeddedEntity),
		bySuggestionID: make(map[string]string),
	}
}

// Create inserts a new suggestion embedded record.
func (r *SuggestionEmbeddedDatabaseRepository) Create(
	ctx context.Context,
	entity suggestionembedded.SuggestionEmbedded,
) (*suggestionembedded.SuggestionEmbeddedEntity, error) {
	if entity.SuggestionID == "" {
		return nil, ErrInvalidSuggestionID
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if entity.UUID == "" {
		entity.UUID = uuid.New().String()
	}

	domainEntity := suggestionembedded.NewSuggestionEmbeddedEntity(entity)
	r.byUUID[entity.UUID] = domainEntity
	r.bySuggestionID[entity.SuggestionID] = entity.UUID

	return domainEntity, nil
}

// Find retrieves suggestion entities matching the filter criteria.
func (r *SuggestionEmbeddedDatabaseRepository) Find(
	ctx context.Context,
	filter suggestionembedded.SuggestionFilter,
) ([]*suggestionembedded.SuggestionEmbeddedEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*suggestionembedded.SuggestionEmbeddedEntity

	for _, entity := range r.byUUID {
		if filter.OrganizationID != "" {
			org := entity.Organization()
			if org == nil || org.UUID != filter.OrganizationID {
				continue
			}
		}

		if filter.RepositoryID != "" && entity.RepositoryID() != filter.RepositoryID {
			continue
		}

		if filter.RepositoryFullName != "" && entity.RepositoryFullName() != filter.RepositoryFullName {
			continue
		}

		if filter.Language != "" && !strings.EqualFold(entity.Language(), filter.Language) {
			continue
		}

		if filter.FeedbackType != "" && entity.FeedbackType() != filter.FeedbackType {
			continue
		}

		results = append(results, entity)
	}

	// Apply Offset and Limit if specified
	if filter.Offset > 0 {
		if filter.Offset >= len(results) {
			return []*suggestionembedded.SuggestionEmbeddedEntity{}, nil
		}
		results = results[filter.Offset:]
	}

	if filter.Limit > 0 && len(results) > filter.Limit {
		results = results[:filter.Limit]
	}

	return results, nil
}

// FindOne retrieves a suggestion by suggestion ID.
func (r *SuggestionEmbeddedDatabaseRepository) FindOne(
	ctx context.Context,
	suggestionID string,
) (*suggestionembedded.SuggestionEmbeddedEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	u, exists := r.bySuggestionID[suggestionID]
	if !exists {
		return nil, nil
	}

	entity, exists := r.byUUID[u]
	if !exists {
		return nil, nil
	}

	return entity, nil
}

// FindByID retrieves a suggestion by internal UUID.
func (r *SuggestionEmbeddedDatabaseRepository) FindByID(
	ctx context.Context,
	id string,
) (*suggestionembedded.SuggestionEmbeddedEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entity, exists := r.byUUID[id]
	if !exists {
		return nil, nil
	}

	return entity, nil
}

// Update modifies an existing suggestion embedding record.
func (r *SuggestionEmbeddedDatabaseRepository) Update(
	ctx context.Context,
	suggestionID string,
	data suggestionembedded.SuggestionEmbedded,
) (*suggestionembedded.SuggestionEmbeddedEntity, error) {
	if suggestionID == "" {
		return nil, ErrInvalidSuggestionID
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	u, exists := r.bySuggestionID[suggestionID]
	if !exists {
		return nil, ErrSuggestionNotFound
	}

	existing := r.byUUID[u]
	existingObj := existing.ToObject()

	// Update fields if provided in data
	if len(data.SuggestionEmbed) > 0 {
		existingObj.SuggestionEmbed = data.SuggestionEmbed
	}
	if data.PullRequestNumber > 0 {
		existingObj.PullRequestNumber = data.PullRequestNumber
	}
	if data.RepositoryID != "" {
		existingObj.RepositoryID = data.RepositoryID
	}
	if data.RepositoryFullName != "" {
		existingObj.RepositoryFullName = data.RepositoryFullName
	}
	if data.Organization != nil {
		existingObj.Organization = data.Organization
	}
	if data.Label != "" {
		existingObj.Label = data.Label
	}
	if data.Severity != "" {
		existingObj.Severity = data.Severity
	}
	if data.FeedbackType != "" {
		existingObj.FeedbackType = data.FeedbackType
	}
	if data.ImprovedCode != "" {
		existingObj.ImprovedCode = data.ImprovedCode
	}
	if data.SuggestionContent != "" {
		existingObj.SuggestionContent = data.SuggestionContent
	}
	if data.OneSentenceSummary != "" {
		existingObj.OneSentenceSummary = data.OneSentenceSummary
	}
	if data.Language != "" {
		existingObj.Language = data.Language
	}

	updated := suggestionembedded.NewSuggestionEmbeddedEntity(existingObj)
	r.byUUID[u] = updated

	return updated, nil
}

// BulkInsert inserts multiple suggestion embeddings.
func (r *SuggestionEmbeddedDatabaseRepository) BulkInsert(
	ctx context.Context,
	entities []suggestionembedded.SuggestionEmbedded,
) ([]*suggestionembedded.SuggestionEmbeddedEntity, error) {
	if len(entities) == 0 {
		return []*suggestionembedded.SuggestionEmbeddedEntity{}, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	created := make([]*suggestionembedded.SuggestionEmbeddedEntity, 0, len(entities))
	for _, e := range entities {
		if e.UUID == "" {
			e.UUID = uuid.New().String()
		}
		domainEntity := suggestionembedded.NewSuggestionEmbeddedEntity(e)
		r.byUUID[e.UUID] = domainEntity
		if e.SuggestionID != "" {
			r.bySuggestionID[e.SuggestionID] = e.UUID
		}
		created = append(created, domainEntity)
	}

	return created, nil
}

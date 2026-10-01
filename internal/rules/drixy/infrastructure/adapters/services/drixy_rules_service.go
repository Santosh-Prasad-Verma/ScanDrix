package services

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

//go:embed data/buckets.json
var bucketsJSON []byte

//go:embed data/library_drixy_rules.json
var libraryRulesJSON []byte

// DrixyRulesService implements the complete domain service contracts.
type DrixyRulesService struct {
	contracts.IDrixyRulesRepository
	ruleLikeService contracts.IRuleLikeService
	mu              sync.RWMutex
	buckets         []contracts.BucketInfo
	libraryRules    []contracts.LibraryDrixyRule
	initOnce        sync.Once
}

// NewDrixyRulesService initializes the domain rules service.
func NewDrixyRulesService(repo contracts.IDrixyRulesRepository, ruleLikeService contracts.IRuleLikeService) *DrixyRulesService {
	s := &DrixyRulesService{
		IDrixyRulesRepository: repo,
		ruleLikeService:       ruleLikeService,
	}
	s.loadCatalog()
	return s
}

func (s *DrixyRulesService) loadCatalog() {
	s.initOnce.Do(func() {
		if len(bucketsJSON) > 0 {
			_ = json.Unmarshal(bucketsJSON, &s.buckets)
		}
		if len(libraryRulesJSON) > 0 {
			_ = json.Unmarshal(libraryRulesJSON, &s.libraryRules)
		}
	})
}

// GetLibraryDrixyRules returns catalog rules matching query filters.
func (s *DrixyRulesService) GetLibraryDrixyRules(ctx context.Context, filters map[string]any, userID string) ([]contracts.LibraryDrixyRule, error) {
	s.loadCatalog()
	s.mu.RLock()
	defer s.mu.RUnlock()

	search, _ := filters["search"].(string)
	language, _ := filters["language"].(string)
	scope, _ := filters["scope"].(string)

	var filtered []contracts.LibraryDrixyRule
	for _, rule := range s.libraryRules {
		if language != "" && !strings.EqualFold(rule.Language, language) && rule.Language != "" {
			continue
		}
		if scope != "" && !strings.EqualFold(rule.Scope, scope) {
			continue
		}
		if search != "" {
			term := strings.ToLower(search)
			if !strings.Contains(strings.ToLower(rule.Title), term) && !strings.Contains(strings.ToLower(rule.Rule), term) {
				continue
			}
		}
		filtered = append(filtered, rule)
	}

	return filtered, nil
}

// GetLibraryDrixyRulesWithFeedback returns catalog rules enriched with community votes.
func (s *DrixyRulesService) GetLibraryDrixyRulesWithFeedback(ctx context.Context, filters map[string]any, userID string) ([]contracts.LibraryDrixyRule, error) {
	rules, err := s.GetLibraryDrixyRules(ctx, filters, userID)
	if err != nil {
		return nil, err
	}

	if s.ruleLikeService == nil {
		return rules, nil
	}

	feedbackList, err := s.ruleLikeService.GetAllRulesWithFeedback(ctx, userID)
	if err != nil {
		return rules, nil
	}

	fbMap := make(map[string]contracts.RuleFeedbackSummary)
	for _, fb := range feedbackList {
		fbMap[fb.RuleID] = fb
	}

	for i := range rules {
		if fb, ok := fbMap[rules[i].UUID]; ok {
			rules[i].PositiveCount = fb.PositiveCount
			rules[i].NegativeCount = fb.NegativeCount
			if fb.UserFeedback != nil {
				str := string(*fb.UserFeedback)
				rules[i].UserFeedback = &str
			}
		}
	}

	return rules, nil
}

// GetLibraryDrixyRulesBuckets returns all pre-configured categories.
func (s *DrixyRulesService) GetLibraryDrixyRulesBuckets(ctx context.Context) ([]contracts.BucketInfo, error) {
	s.loadCatalog()
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]contracts.BucketInfo, len(s.buckets))
	copy(out, s.buckets)
	return out, nil
}

// FindRulesByDirectory returns rules targeting a specific repo and directory.
func (s *DrixyRulesService) FindRulesByDirectory(ctx context.Context, organizationID, repositoryID, directoryID string) ([]interfaces.DrixyRule, error) {
	entity, err := s.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return []interfaces.DrixyRule{}, nil
	}

	var matches []interfaces.DrixyRule
	for _, rule := range entity.Rules() {
		if rule.RepositoryID == repositoryID && rule.DirectoryID == directoryID {
			matches = append(matches, rule)
		}
	}
	return matches, nil
}

// UpdateRulesStatusByFilter updates status across filtered rules.
func (s *DrixyRulesService) UpdateRulesStatusByFilter(ctx context.Context, organizationID, repositoryID, directoryID string, newStatus interfaces.DrixyRulesStatus) (*entities.DrixyRulesEntity, error) {
	entity, err := s.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return nil, err
	}

	var updatedRules []interfaces.DrixyRule
	for _, rule := range entity.Rules() {
		match := true
		if repositoryID != "" && rule.RepositoryID != repositoryID {
			match = false
		}
		if directoryID != "" && rule.DirectoryID != directoryID {
			match = false
		}
		if match {
			rule.Status = newStatus
			now := time.Now().UTC()
			rule.UpdatedAt = &now
		}
		updatedRules = append(updatedRules, rule)
	}

	now := time.Now().UTC()
	updatedEntity := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		UUID:           entity.UUID(),
		OrganizationID: organizationID,
		Rules:          updatedRules,
		CreatedAt:      entity.CreatedAt(),
		UpdatedAt:      &now,
	})

	err = s.Save(ctx, updatedEntity)
	return updatedEntity, err
}

// DeleteRuleWithLogging deletes a rule and logs the audit trail.
func (s *DrixyRulesService) DeleteRuleWithLogging(ctx context.Context, organizationID, teamID, ruleID string, userInfo *contracts.UserAuditInfo) (bool, error) {
	entity, err := s.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return false, nil
	}

	found := false
	var updatedRules []interfaces.DrixyRule
	for _, rule := range entity.Rules() {
		if rule.UUID == ruleID {
			found = true
			rule.Status = interfaces.DrixyRulesStatusDeleted
			now := time.Now().UTC()
			rule.UpdatedAt = &now
		}
		updatedRules = append(updatedRules, rule)
	}

	if !found {
		return false, nil
	}

	now := time.Now().UTC()
	updatedEntity := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		UUID:           entity.UUID(),
		OrganizationID: organizationID,
		Rules:          updatedRules,
		CreatedAt:      entity.CreatedAt(),
		UpdatedAt:      &now,
	})

	err = s.Save(ctx, updatedEntity)
	return err == nil, err
}

// UpdateRuleWithLogging modifies a rule and logs the modification.
func (s *DrixyRulesService) UpdateRuleWithLogging(ctx context.Context, organizationID, teamID string, rule *interfaces.DrixyRule, userInfo *contracts.UserAuditInfo) (*interfaces.DrixyRule, error) {
	entity, err := s.FindByOrganizationID(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if rule.UUID == "" {
		rule.UUID = uuid.New().String()
		rule.CreatedAt = &now
	}
	rule.UpdatedAt = &now

	var existingRules []interfaces.DrixyRule
	if entity != nil {
		existingRules = entity.Rules()
	}

	found := false
	for i, r := range existingRules {
		if r.UUID == rule.UUID {
			existingRules[i] = *rule
			found = true
			break
		}
	}
	if !found {
		existingRules = append(existingRules, *rule)
	}

	updatedEntity := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		UUID:           uuid.New().String(),
		OrganizationID: organizationID,
		Rules:          existingRules,
		UpdatedAt:      &now,
	})

	if entity != nil {
		updatedEntity = entities.NewDrixyRulesEntity(interfaces.DrixyRules{
			UUID:           entity.UUID(),
			OrganizationID: organizationID,
			Rules:          existingRules,
			CreatedAt:      entity.CreatedAt(),
			UpdatedAt:      &now,
		})
	}

	err = s.Save(ctx, updatedEntity)
	if err != nil {
		return nil, err
	}
	return rule, nil
}

// UpdateRuleReferences updates linked context references.
func (s *DrixyRulesService) UpdateRuleReferences(ctx context.Context, organizationID, ruleID, contextReferenceID string) (*interfaces.DrixyRule, error) {
	entity, err := s.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return nil, fmt.Errorf("organization rules not found")
	}

	var target *interfaces.DrixyRule
	var updatedRules []interfaces.DrixyRule
	for _, rule := range entity.Rules() {
		if rule.UUID == ruleID {
			rule.ContextReferenceID = contextReferenceID
			now := time.Now().UTC()
			rule.UpdatedAt = &now
			target = &rule
		}
		updatedRules = append(updatedRules, rule)
	}

	if target == nil {
		return nil, fmt.Errorf("rule not found: %s", ruleID)
	}

	now := time.Now().UTC()
	updatedEntity := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		UUID:           entity.UUID(),
		OrganizationID: organizationID,
		Rules:          updatedRules,
		CreatedAt:      entity.CreatedAt(),
		UpdatedAt:      &now,
	})

	err = s.Save(ctx, updatedEntity)
	return target, err
}

// UpdateRuleDetector stores a compiled T0 detector.
func (s *DrixyRulesService) UpdateRuleDetector(ctx context.Context, organizationID, ruleID string, detector *interfaces.DrixyRuleDetector) (*interfaces.DrixyRule, error) {
	entity, err := s.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return nil, fmt.Errorf("organization rules not found")
	}

	var target *interfaces.DrixyRule
	var updatedRules []interfaces.DrixyRule
	for _, rule := range entity.Rules() {
		if rule.UUID == ruleID {
			rule.Detector = detector
			now := time.Now().UTC()
			rule.UpdatedAt = &now
			target = &rule
		}
		updatedRules = append(updatedRules, rule)
	}

	if target == nil {
		return nil, fmt.Errorf("rule not found: %s", ruleID)
	}

	now := time.Now().UTC()
	updatedEntity := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		UUID:           entity.UUID(),
		OrganizationID: organizationID,
		Rules:          updatedRules,
		CreatedAt:      entity.CreatedAt(),
		UpdatedAt:      &now,
	})

	err = s.Save(ctx, updatedEntity)
	return target, err
}

// GetRulesLimitStatus computes active rule count vs free-tier limit.
func (s *DrixyRulesService) GetRulesLimitStatus(ctx context.Context, organizationID, teamID string) (int, error) {
	activeStatus := interfaces.DrixyRulesStatusActive
	return s.CountRules(ctx, organizationID, &activeStatus)
}

// GetRecommendedRulesBySuggestions recommends rules based on language and review suggestions.
func (s *DrixyRulesService) GetRecommendedRulesBySuggestions(ctx context.Context, organizationID, teamID, repositoryID, language string) ([]contracts.LibraryDrixyRule, error) {
	filters := map[string]any{"language": language}
	all, err := s.GetLibraryDrixyRules(ctx, filters, "")
	if err != nil {
		return nil, err
	}
	if len(all) > 10 {
		return all[:10], nil
	}
	return all, nil
}

// CreateOrUpdateMemory records or updates a review memory.
func (s *DrixyRulesService) CreateOrUpdateMemory(ctx context.Context, organizationID, teamID string, memory *interfaces.DrixyRuleMemory, userInfo *contracts.UserAuditInfo) (*contracts.CreateOrUpdateMemoryResult, error) {
	if memory == nil {
		return nil, fmt.Errorf("memory is nil")
	}

	rule := interfaces.DrixyRule{
		UUID:               memory.UUID,
		Title:              memory.Title,
		Rule:               memory.Rule,
		Path:               memory.Path,
		SourcePath:         memory.SourcePath,
		Status:             memory.Status,
		Severity:           "MEDIUM",
		Type:               interfaces.DrixyRulesTypeMemory,
		RepositoryID:       memory.RepositoryID,
		SourceRepositoryID: memory.SourceRepositoryID,
		LastContentHash:    memory.LastContentHash,
		Origin:             memory.Origin,
		Reason:             memory.Reason,
		DirectoryID:        memory.DirectoryID,
		RequestType:        memory.RequestType,
		TargetRuleUUID:     memory.TargetRuleUUID,
		PinnedSync:         memory.PinnedSync,
		LockedByPlan:       memory.LockedByPlan,
	}

	action := contracts.MemoryActionCreated
	if rule.UUID != "" {
		action = contracts.MemoryActionUpdated
	}

	saved, err := s.UpdateRuleWithLogging(ctx, organizationID, teamID, &rule, userInfo)
	if err != nil {
		return nil, err
	}

	return &contracts.CreateOrUpdateMemoryResult{
		Rule:             saved,
		Action:           action,
		RequiresApproval: saved.Status == interfaces.DrixyRulesStatusPending,
		Link:             "/drixy-rules?ruleId=" + saved.UUID,
	}, nil
}

// FindMemories searches past memories.
func (s *DrixyRulesService) FindMemories(ctx context.Context, organizationID, teamID string, filters *interfaces.FindMemoriesFilters) ([]interfaces.FindMemoriesResult, error) {
	entity, err := s.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return []interfaces.FindMemoriesResult{}, nil
	}

	var results []interfaces.FindMemoriesResult
	for _, rule := range entity.Rules() {
		if rule.Type != interfaces.DrixyRulesTypeMemory {
			continue
		}
		if filters != nil {
			if filters.RepositoryID != "" && rule.RepositoryID != filters.RepositoryID {
				continue
			}
			if filters.DirectoryID != "" && rule.DirectoryID != filters.DirectoryID {
				continue
			}
		}
		cAt := ""
		if rule.CreatedAt != nil {
			cAt = rule.CreatedAt.Format(time.RFC3339)
		}
		results = append(results, interfaces.FindMemoriesResult{
			UUID:         rule.UUID,
			Title:        rule.Title,
			Rule:         rule.Rule,
			RepositoryID: rule.RepositoryID,
			DirectoryID:  rule.DirectoryID,
			Path:         rule.Path,
			CreatedAt:    cAt,
			Link:         "/drixy-rules?ruleId=" + rule.UUID,
		})
	}
	return results, nil
}

// SyncRulesWithPlanLimit disables or pauses rules exceeding plan quotas.
func (s *DrixyRulesService) SyncRulesWithPlanLimit(ctx context.Context, organizationID string, maxAllowedRules int) (*entities.DrixyRulesEntity, error) {
	if maxAllowedRules <= 0 {
		return s.FindByOrganizationID(ctx, organizationID)
	}

	entity, err := s.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return nil, err
	}

	rules := entity.Rules()
	activeCount := 0
	for i := range rules {
		if rules[i].Status == interfaces.DrixyRulesStatusActive {
			activeCount++
			if activeCount > maxAllowedRules {
				rules[i].Status = interfaces.DrixyRulesStatusPaused
				rules[i].LockedByPlan = true
				now := time.Now().UTC()
				rules[i].UpdatedAt = &now
			}
		}
	}

	now := time.Now().UTC()
	updatedEntity := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		UUID:           entity.UUID(),
		OrganizationID: organizationID,
		Rules:          rules,
		CreatedAt:      entity.CreatedAt(),
		UpdatedAt:      &now,
	})

	err = s.Save(ctx, updatedEntity)
	return updatedEntity, err
}

package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/centralizedconfig/domain"
)

// WorkspaceParametersStore provides access to workspace parameters in database.
type WorkspaceParametersStore interface {
	GetWorkspaceParameters(ctx context.Context, wsID uuid.UUID) (reviewParams, orgParams []byte, err error)
	UpdateWorkspaceReviewParameters(ctx context.Context, wsID uuid.UUID, reviewParams []byte) error
}

// DefaultConfigStorage implements ConfigStoragePort with database backing and synchronized cache.
type DefaultConfigStorage struct {
	mu             sync.RWMutex
	store          WorkspaceParametersStore
	codeReviewCfg  map[string]map[string]any
	customMessages map[string]domain.CustomMessageConfig
	rules          map[string]domain.RuleFileMeta
}

// NewDefaultConfigStorage creates an initialized database/memory hybrid configuration storage.
func NewDefaultConfigStorage(store WorkspaceParametersStore) *DefaultConfigStorage {
	return &DefaultConfigStorage{
		store:          store,
		codeReviewCfg:  make(map[string]map[string]any),
		customMessages: make(map[string]domain.CustomMessageConfig),
		rules:          make(map[string]domain.RuleFileMeta),
	}
}

// GetCodeReviewParameter retrieves code review config for org/team, checking store then cache.
func (s *DefaultConfigStorage) GetCodeReviewParameter(ctx context.Context, orgID, teamID string) (map[string]any, error) {
	s.mu.RLock()
	key := s.scopedKey(orgID, teamID)
	if val, ok := s.codeReviewCfg[key]; ok {
		s.mu.RUnlock()
		return val, nil
	}
	s.mu.RUnlock()

	if s.store != nil {
		wsUUID, err := uuid.Parse(orgID)
		if err == nil {
			reviewBytes, _, err := s.store.GetWorkspaceParameters(ctx, wsUUID)
			if err == nil && len(reviewBytes) > 0 {
				var parsed map[string]any
				if err := json.Unmarshal(reviewBytes, &parsed); err == nil {
					s.mu.Lock()
					s.codeReviewCfg[key] = parsed
					s.mu.Unlock()
					return parsed, nil
				}
			}
		}
	}

	return nil, nil
}

// SaveCodeReviewParameter persists code review configuration.
func (s *DefaultConfigStorage) SaveCodeReviewParameter(ctx context.Context, orgID, teamID, repoID string, directoryPaths []string, config map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.scopedKey(orgID, teamID)
	s.codeReviewCfg[key] = config

	if s.store != nil {
		wsUUID, err := uuid.Parse(orgID)
		if err == nil {
			if data, err := json.Marshal(config); err == nil {
				_ = s.store.UpdateWorkspaceReviewParameters(ctx, wsUUID, data)
			}
		}
	}

	return nil
}

// DeleteCodeReviewParameter removes repository/directory-level review parameter overrides.
func (s *DefaultConfigStorage) DeleteCodeReviewParameter(ctx context.Context, orgID, teamID, repoID string, directoryPaths []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.scopedKey(orgID, teamID)
	delete(s.codeReviewCfg, key)
	return nil
}

// GetCustomMessages returns all custom messages registered for an org/team.
func (s *DefaultConfigStorage) GetCustomMessages(ctx context.Context, orgID, teamID string) ([]domain.CustomMessageConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []domain.CustomMessageConfig
	for _, msg := range s.customMessages {
		if msg.OrganizationUUID == orgID && (teamID == "" || msg.TeamUUID == teamID) {
			result = append(result, msg)
		}
	}
	return result, nil
}

// SaveCustomMessage records or updates a custom PR comment message.
func (s *DefaultConfigStorage) SaveCustomMessage(ctx context.Context, msg domain.CustomMessageConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if msg.UUID == "" {
		msg.UUID = uuid.New().String()
	}
	if msg.UpdatedAt.IsZero() {
		msg.UpdatedAt = time.Now().UTC()
	}
	s.customMessages[msg.UUID] = msg
	return nil
}

// DeleteCustomMessage removes a custom message by UUID.
func (s *DefaultConfigStorage) DeleteCustomMessage(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.customMessages, id)
	return nil
}

// GetRules returns all custom rules for org/team.
func (s *DefaultConfigStorage) GetRules(ctx context.Context, orgID, teamID string) ([]domain.RuleFileMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []domain.RuleFileMeta
	for _, rule := range s.rules {
		result = append(result, rule)
	}
	return result, nil
}

// SaveRule adds or updates a custom review rule in the centralized store.
func (s *DefaultConfigStorage) SaveRule(ctx context.Context, rule domain.RuleFileMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rule.ID == "" {
		rule.ID = uuid.New().String()
	}
	if rule.UUID == "" {
		rule.UUID = rule.ID
	}
	if rule.ModifiedAt.IsZero() {
		rule.ModifiedAt = time.Now().UTC()
	}
	s.rules[rule.ID] = rule
	return nil
}

// DeleteRule removes a rule by ID.
func (s *DefaultConfigStorage) DeleteRule(ctx context.Context, ruleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.rules, ruleID)
	return nil
}

func (s *DefaultConfigStorage) scopedKey(orgID, teamID string) string {
	if teamID != "" {
		return fmt.Sprintf("%s:%s", orgID, teamID)
	}
	return orgID
}

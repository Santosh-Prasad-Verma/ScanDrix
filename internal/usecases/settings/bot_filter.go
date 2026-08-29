package settings

import (
	"strings"
	"sync"

	"github.com/google/uuid"
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
}

// BotFilterService manages bot suppression policies per workspace.
type BotFilterService struct {
	mu      sync.RWMutex
	configs map[uuid.UUID]BotIgnoreConfig
}

func NewBotFilterService() *BotFilterService {
	return &BotFilterService{
		configs: make(map[uuid.UUID]BotIgnoreConfig),
	}
}

// SetConfig updates bot filtering rules for a workspace.
func (s *BotFilterService) SetConfig(cfg BotIgnoreConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configs[cfg.WorkspaceID] = cfg
}

// IsBotAuthor checks if a pull request author should be suppressed from reviews.
func (s *BotFilterService) IsBotAuthor(workspaceID uuid.UUID, author string) bool {
	if strings.TrimSpace(author) == "" {
		return false
	}

	authorLower := strings.ToLower(author)

	s.mu.RLock()
	cfg, exists := s.configs[workspaceID]
	s.mu.RUnlock()

	// Default behavior: ignore standard bot suffixes
	if strings.HasSuffix(authorLower, "[bot]") {
		return true
	}

	if !exists || cfg.IgnoreKnownBots {
		for _, b := range defaultKnownBots {
			if authorLower == b {
				return true
			}
		}
	}

	if exists {
		// Custom bot names
		for _, custom := range cfg.CustomBotNames {
			if strings.EqualFold(authorLower, strings.TrimSpace(custom)) {
				return true
			}
		}

		// Explicit ignored user IDs/logins
		for _, ignored := range cfg.IgnoredUserIDs {
			if strings.EqualFold(authorLower, strings.TrimSpace(ignored)) {
				return true
			}
		}
	}

	return false
}

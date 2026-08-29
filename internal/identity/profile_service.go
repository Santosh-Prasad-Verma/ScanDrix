package identity

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ProfileService manages user profiles and localized environment preferences.
type ProfileService struct {
	mu       sync.RWMutex
	profiles map[uuid.UUID]*UserProfile
}

func NewProfileService() *ProfileService {
	return &ProfileService{
		profiles: make(map[uuid.UUID]*UserProfile),
	}
}

// CreateProfile registers a new user profile in the system.
func (s *ProfileService) CreateProfile(profile UserProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.profiles[profile.ID]; exists {
		return fmt.Errorf("profile with ID %s already exists", profile.ID)
	}

	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now
	if profile.Preferences == nil {
		profile.Preferences = make(map[string]string)
	}

	s.profiles[profile.ID] = &profile
	return nil
}

// GetProfile retrieves a profile by user UUID.
func (s *ProfileService) GetProfile(id uuid.UUID) (*UserProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, exists := s.profiles[id]
	if !exists {
		return nil, fmt.Errorf("profile not found: %s", id)
	}

	cpy := *p
	return &cpy, nil
}

// UpdatePreference sets or modifies an individual user configuration preference.
func (s *ProfileService) UpdatePreference(id uuid.UUID, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, exists := s.profiles[id]
	if !exists {
		return fmt.Errorf("profile not found: %s", id)
	}

	p.Preferences[key] = value
	p.UpdatedAt = time.Now().UTC()
	return nil
}

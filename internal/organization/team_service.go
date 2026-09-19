package organization

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TeamService manages team hierarchies, repo scoping, and reviewer auto-assignment.
type TeamService struct {
	mu         sync.RWMutex
	teams      map[uuid.UUID]*Team
	members    map[uuid.UUID][]*TeamMember // teamID -> members
	roundRobin map[uuid.UUID]int           // teamID -> currentIndex
}

func NewTeamService() *TeamService {
	return &TeamService{
		teams:      make(map[uuid.UUID]*Team),
		members:    make(map[uuid.UUID][]*TeamMember),
		roundRobin: make(map[uuid.UUID]int),
	}
}

// CreateTeam provisions a new team within a workspace.
func (s *TeamService) CreateTeam(workspaceID uuid.UUID, name, autoAssignMode string) (*Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	team := &Team{
		ID:             uuid.New(),
		WorkspaceID:    workspaceID,
		Name:           name,
		RepositoryIDs:  make([]uuid.UUID, 0),
		AutoAssignMode: autoAssignMode,
		CreatedAt:      time.Now().UTC(),
	}

	s.teams[team.ID] = team
	return team, nil
}

// AddMember attaches a user to a team.
func (s *TeamService) AddMember(teamID, userID uuid.UUID, role string) (*TeamMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.teams[teamID]; !exists {
		return nil, errors.New("team not found")
	}

	for _, m := range s.members[teamID] {
		if m.UserID == userID {
			return nil, fmt.Errorf("user %s is already a member of team %s", userID, teamID)
		}
	}

	member := &TeamMember{
		ID:          uuid.New(),
		TeamID:      teamID,
		UserID:      userID,
		Role:        role,
		ReviewCount: 0,
		JoinedAt:    time.Now().UTC(),
	}

	s.members[teamID] = append(s.members[teamID], member)
	return member, nil
}

// AutoAssignReviewer selects a team reviewer using round-robin or least-busy algorithm.
func (s *TeamService) AutoAssignReviewer(teamID uuid.UUID, excludeUserID *uuid.UUID) (*TeamMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	team, exists := s.teams[teamID]
	if !exists {
		return nil, errors.New("team not found")
	}

	members := s.members[teamID]
	if len(members) == 0 {
		return nil, errors.New("no members available in team for review assignment")
	}

	// Filter out PR author
	var candidates []*TeamMember
	for _, m := range members {
		if excludeUserID != nil && m.UserID == *excludeUserID {
			continue
		}
		candidates = append(candidates, m)
	}

	if len(candidates) == 0 {
		return nil, errors.New("all team members are excluded from review assignment")
	}

	var selected *TeamMember

	switch team.AutoAssignMode {
	case "least_busy":
		// Sort candidates by current review count ascending
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].ReviewCount < candidates[j].ReviewCount
		})
		selected = candidates[0]

	case "round_robin":
		fallthrough
	default:
		idx := s.roundRobin[teamID] % len(candidates)
		selected = candidates[idx]
		s.roundRobin[teamID] = idx + 1
	}

	selected.ReviewCount++
	return selected, nil
}

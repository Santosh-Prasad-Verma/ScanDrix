package organization

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// WorkspaceService manages organization tenants, seat quotas, and member onboarding.
type WorkspaceService struct {
	mu          sync.RWMutex
	workspaces  map[uuid.UUID]*WorkspaceTenant
	invitations map[string]*MemberInvitation // token -> invite
}

func NewWorkspaceService() *WorkspaceService {
	return &WorkspaceService{
		workspaces:  make(map[uuid.UUID]*WorkspaceTenant),
		invitations: make(map[string]*MemberInvitation),
	}
}

// CreateWorkspace registers a new tenant with designated seat allowances.
func (s *WorkspaceService) CreateWorkspace(slug, name, plan string, maxSeats int) (*WorkspaceTenant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, w := range s.workspaces {
		if w.Slug == slug {
			return nil, fmt.Errorf("workspace slug %q already in use", slug)
		}
	}

	now := time.Now().UTC()
	ws := &WorkspaceTenant{
		ID:        uuid.New(),
		Slug:      slug,
		Name:      name,
		Plan:      plan,
		MaxSeats:  maxSeats,
		UsedSeats: 1, // Creator occupies first seat
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.workspaces[ws.ID] = ws
	return ws, nil
}

// InviteMember issues an onboarding invitation asserting seat limits.
func (s *WorkspaceService) InviteMember(workspaceID uuid.UUID, email, role string) (*MemberInvitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws, exists := s.workspaces[workspaceID]
	if !exists {
		return nil, errors.New("workspace not found")
	}

	if ws.MaxSeats > 0 && ws.UsedSeats >= ws.MaxSeats {
		return nil, fmt.Errorf("seat limit reached (%d/%d seats used); upgrade plan to invite more members", ws.UsedSeats, ws.MaxSeats)
	}

	tokenBytes := make([]byte, 16)
	_, _ = rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)

	inv := &MemberInvitation{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		Email:       email,
		Role:        role,
		InviteToken: token,
		Accepted:    false,
		ExpiresAt:   time.Now().UTC().Add(7 * 24 * time.Hour),
	}

	s.invitations[token] = inv
	return inv, nil
}

// AcceptInvitation consumes an invitation token and claims a seat.
func (s *WorkspaceService) AcceptInvitation(token string) (*MemberInvitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	inv, exists := s.invitations[token]
	if !exists {
		return nil, errors.New("invalid or expired invitation token")
	}

	if inv.Accepted {
		return nil, errors.New("invitation has already been accepted")
	}

	if time.Now().UTC().After(inv.ExpiresAt) {
		return nil, errors.New("invitation token expired")
	}

	ws, wsExists := s.workspaces[inv.WorkspaceID]
	if !wsExists {
		return nil, errors.New("parent workspace no longer exists")
	}

	ws.UsedSeats++
	inv.Accepted = true
	return inv, nil
}

// GetWorkspace returns a tenant by ID.
func (s *WorkspaceService) GetWorkspace(id uuid.UUID) (*WorkspaceTenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ws, exists := s.workspaces[id]
	if !exists {
		return nil, errors.New("workspace not found")
	}
	cpy := *ws
	return &cpy, nil
}

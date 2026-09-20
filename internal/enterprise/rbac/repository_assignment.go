// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package rbac

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

var (
	ErrInvalidUserID       = errors.New("invalid user id")
	ErrInvalidWorkspaceID  = errors.New("invalid workspace id")
	ErrInvalidRepositoryID = errors.New("invalid repository id")
)

// UserRepositoryAssignment defines the specific repositories granted to a user within a workspace.
type UserRepositoryAssignment = models.UserRepositoryAssignment

// RepositoryAssignmentStore defines the persistence interface for fine-grained repository access control.
type RepositoryAssignmentStore interface {
	SaveAssignment(ctx context.Context, assignment *UserRepositoryAssignment) error
	GetAssignment(ctx context.Context, wsID, userID uuid.UUID) (*UserRepositoryAssignment, error)
	DeleteAssignment(ctx context.Context, wsID, userID uuid.UUID) error
}

// DatabaseAssignmentStore defines a persistence contract matching *database.Repository.
type DatabaseAssignmentStore interface {
	SaveRepositoryAssignment(ctx context.Context, assignment *models.UserRepositoryAssignment) error
	GetRepositoryAssignment(ctx context.Context, wsID, userID uuid.UUID) (*models.UserRepositoryAssignment, error)
	DeleteRepositoryAssignment(ctx context.Context, wsID, userID uuid.UUID) error
}

// PostgresRepositoryAssignmentStore provides a PostgreSQL database-backed assignment store.
type PostgresRepositoryAssignmentStore struct {
	db DatabaseAssignmentStore
}

// NewPostgresRepositoryAssignmentStore creates a database-backed repository assignment store.
func NewPostgresRepositoryAssignmentStore(db DatabaseAssignmentStore) *PostgresRepositoryAssignmentStore {
	return &PostgresRepositoryAssignmentStore{db: db}
}

func (s *PostgresRepositoryAssignmentStore) SaveAssignment(ctx context.Context, a *UserRepositoryAssignment) error {
	if s.db == nil {
		return errors.New("database repository unavailable")
	}
	return s.db.SaveRepositoryAssignment(ctx, a)
}

func (s *PostgresRepositoryAssignmentStore) GetAssignment(ctx context.Context, wsID, userID uuid.UUID) (*UserRepositoryAssignment, error) {
	if s.db == nil {
		return nil, errors.New("database repository unavailable")
	}
	return s.db.GetRepositoryAssignment(ctx, wsID, userID)
}

func (s *PostgresRepositoryAssignmentStore) DeleteAssignment(ctx context.Context, wsID, userID uuid.UUID) error {
	if s.db == nil {
		return errors.New("database repository unavailable")
	}
	return s.db.DeleteRepositoryAssignment(ctx, wsID, userID)
}

// MemoryRepositoryAssignmentStore provides an in-memory thread-safe implementation.
type MemoryRepositoryAssignmentStore struct {
	mu          sync.RWMutex
	assignments map[string]*UserRepositoryAssignment // "wsID:userID" -> assignment
}

func NewMemoryRepositoryAssignmentStore() *MemoryRepositoryAssignmentStore {
	return &MemoryRepositoryAssignmentStore{
		assignments: make(map[string]*UserRepositoryAssignment),
	}
}

func (s *MemoryRepositoryAssignmentStore) key(wsID, userID uuid.UUID) string {
	return fmt.Sprintf("%s:%s", wsID.String(), userID.String())
}

func (s *MemoryRepositoryAssignmentStore) SaveAssignment(_ context.Context, a *UserRepositoryAssignment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assignments[s.key(a.WorkspaceID, a.UserID)] = a
	return nil
}

func (s *MemoryRepositoryAssignmentStore) GetAssignment(_ context.Context, wsID, userID uuid.UUID) (*UserRepositoryAssignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, exists := s.assignments[s.key(wsID, userID)]
	if !exists {
		return nil, nil
	}
	return a, nil
}

func (s *MemoryRepositoryAssignmentStore) DeleteAssignment(_ context.Context, wsID, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.assignments, s.key(wsID, userID))
	return nil
}

// RepositoryAccessController enforces per-user repository boundaries.
type RepositoryAccessController struct {
	store        RepositoryAssignmentStore
	policyEngine *PolicyEngine
}

// NewRepositoryAccessController initializes the repository access controller.
func NewRepositoryAccessController(store RepositoryAssignmentStore, engine *PolicyEngine) *RepositoryAccessController {
	if store == nil {
		store = NewMemoryRepositoryAssignmentStore()
	}
	if engine == nil {
		engine = NewPolicyEngine()
	}
	return &RepositoryAccessController{
		store:        store,
		policyEngine: engine,
	}
}

// AssignRepositories sets or replaces the explicit repository assignments for a user.
func (c *RepositoryAccessController) AssignRepositories(
	ctx context.Context,
	wsID, userID uuid.UUID,
	repoIDs []uuid.UUID,
	assignedBy string,
) (*UserRepositoryAssignment, error) {
	if wsID == uuid.Nil {
		return nil, ErrInvalidWorkspaceID
	}
	if userID == uuid.Nil {
		return nil, ErrInvalidUserID
	}

	// Deduplicate repository UUIDs
	uniqueMap := make(map[uuid.UUID]bool)
	var cleanIDs []uuid.UUID
	for _, id := range repoIDs {
		if id != uuid.Nil && !uniqueMap[id] {
			uniqueMap[id] = true
			cleanIDs = append(cleanIDs, id)
		}
	}

	assignment := &UserRepositoryAssignment{
		WorkspaceID:   wsID,
		UserID:        userID,
		RepositoryIDs: cleanIDs,
		AssignedBy:    assignedBy,
		UpdatedAt:     time.Now().UTC(),
	}

	if err := c.store.SaveAssignment(ctx, assignment); err != nil {
		return nil, fmt.Errorf("failed saving repository assignment: %w", err)
	}

	return assignment, nil
}

// AddRepositories appends repositories to a user's existing assignment.
func (c *RepositoryAccessController) AddRepositories(
	ctx context.Context,
	wsID, userID uuid.UUID,
	repoIDs []uuid.UUID,
	assignedBy string,
) (*UserRepositoryAssignment, error) {
	existing, err := c.store.GetAssignment(ctx, wsID, userID)
	if err != nil {
		return nil, err
	}

	var combined []uuid.UUID
	if existing != nil {
		combined = append(combined, existing.RepositoryIDs...)
	}
	combined = append(combined, repoIDs...)

	return c.AssignRepositories(ctx, wsID, userID, combined, assignedBy)
}

// RevokeRepository removes a single repository from a user's assignment.
func (c *RepositoryAccessController) RevokeRepository(
	ctx context.Context,
	wsID, userID, repoID uuid.UUID,
) (*UserRepositoryAssignment, error) {
	existing, err := c.store.GetAssignment(ctx, wsID, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	var remaining []uuid.UUID
	for _, id := range existing.RepositoryIDs {
		if id != repoID {
			remaining = append(remaining, id)
		}
	}

	return c.AssignRepositories(ctx, wsID, userID, remaining, existing.AssignedBy)
}

// GetAssignedRepositories returns the list of explicit repository IDs assigned to a user.
func (c *RepositoryAccessController) GetAssignedRepositories(
	ctx context.Context,
	wsID, userID uuid.UUID,
) ([]uuid.UUID, error) {
	assignment, err := c.store.GetAssignment(ctx, wsID, userID)
	if err != nil {
		return nil, err
	}
	if assignment == nil {
		return []uuid.UUID{}, nil
	}
	return assignment.RepositoryIDs, nil
}

// CanAccessRepository checks whether a user has permission to view or review a specific repository.
func (c *RepositoryAccessController) CanAccessRepository(
	ctx context.Context,
	wsID, userID uuid.UUID,
	role models.UserRole,
	repoID uuid.UUID,
) bool {
	// 1. Workspace owners and admins have unrestricted access to all repositories
	if role == models.RoleOwner || role == models.RoleAdmin {
		return true
	}

	// 2. Base role must at least have repository read permissions
	if !c.policyEngine.Can(role, ActionRead, ResourceRepository) {
		return false
	}

	// 3. Check for fine-grained per-user repository restrictions
	assignment, err := c.store.GetAssignment(ctx, wsID, userID)
	if err != nil || assignment == nil || len(assignment.RepositoryIDs) == 0 {
		// If no explicit restrictions are configured for this member, default open workspace policy applies
		return true
	}

	// 4. If explicit repository restrictions are present, the target repo must be explicitly granted
	for _, assignedID := range assignment.RepositoryIDs {
		if assignedID == repoID {
			return true
		}
	}

	return false
}

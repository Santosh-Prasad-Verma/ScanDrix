package repositories_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
	"github.com/scandrix/backend/internal/core/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// In-Memory Thread-Safe Mock Repositories
// ============================================================================

// MockWorkspaceRepo implements in-memory Workspace storage.
type MockWorkspaceRepo struct {
	mu         sync.RWMutex
	workspaces map[uuid.UUID]*domain.Workspace
}

func NewMockWorkspaceRepo() *MockWorkspaceRepo {
	return &MockWorkspaceRepo{
		workspaces: make(map[uuid.UUID]*domain.Workspace),
	}
}

func (m *MockWorkspaceRepo) Create(ctx context.Context, ws *domain.Workspace) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ws.ID == uuid.Nil {
		ws.ID = uuid.New()
	}
	now := time.Now().UTC()
	if ws.CreatedAt.IsZero() {
		ws.CreatedAt = now
	}
	ws.UpdatedAt = now
	m.workspaces[ws.ID] = ws
	return nil
}

func (m *MockWorkspaceRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Workspace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ws, ok := m.workspaces[id]
	if !ok {
		return nil, nil
	}
	return ws, nil
}

func (m *MockWorkspaceRepo) Update(ctx context.Context, ws *domain.Workspace) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.workspaces[ws.ID]; !ok {
		return fmt.Errorf("workspace not found")
	}
	ws.UpdatedAt = time.Now().UTC()
	m.workspaces[ws.ID] = ws
	return nil
}

func (m *MockWorkspaceRepo) Delete(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.workspaces, id)
	return nil
}

func (m *MockWorkspaceRepo) List(ctx context.Context) ([]*domain.Workspace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*domain.Workspace, 0, len(m.workspaces))
	for _, ws := range m.workspaces {
		list = append(list, ws)
	}
	return list, nil
}

// MockUserRepo implements in-memory User storage.
type MockUserRepo struct {
	mu    sync.RWMutex
	users map[uuid.UUID]*domain.User
}

func NewMockUserRepo() *MockUserRepo {
	return &MockUserRepo{
		users: make(map[uuid.UUID]*domain.User),
	}
}

func (m *MockUserRepo) Create(ctx context.Context, u *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	now := time.Now().UTC()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	u.UpdatedAt = now
	m.users[u.ID] = u
	return nil
}

func (m *MockUserRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[id]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *MockUserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, nil
}

func (m *MockUserRepo) FindByWorkspace(ctx context.Context, wsID uuid.UUID) ([]*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*domain.User
	for _, u := range m.users {
		if u.WorkspaceID == wsID {
			list = append(list, u)
		}
	}
	return list, nil
}

// MockTrackedRepoRepo implements in-memory TrackedRepository storage.
type MockTrackedRepoRepo struct {
	mu    sync.RWMutex
	repos map[uuid.UUID]*domain.TrackedRepository
}

func NewMockTrackedRepoRepo() *MockTrackedRepoRepo {
	return &MockTrackedRepoRepo{
		repos: make(map[uuid.UUID]*domain.TrackedRepository),
	}
}

func (m *MockTrackedRepoRepo) Create(ctx context.Context, r *domain.TrackedRepository) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	m.repos[r.ID] = r
	return nil
}

func (m *MockTrackedRepoRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.TrackedRepository, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.repos[id]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func (m *MockTrackedRepoRepo) FindByWorkspace(ctx context.Context, wsID uuid.UUID) ([]*domain.TrackedRepository, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*domain.TrackedRepository
	for _, r := range m.repos {
		if r.WorkspaceID == wsID {
			list = append(list, r)
		}
	}
	return list, nil
}

// MockReviewRepo implements in-memory PullRequestReview storage.
type MockReviewRepo struct {
	mu      sync.RWMutex
	reviews map[uuid.UUID]*domain.PullRequestReview
}

func NewMockReviewRepo() *MockReviewRepo {
	return &MockReviewRepo{
		reviews: make(map[uuid.UUID]*domain.PullRequestReview),
	}
}

func (m *MockReviewRepo) Create(ctx context.Context, r *domain.PullRequestReview) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	m.reviews[r.ID] = r
	return nil
}

func (m *MockReviewRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.PullRequestReview, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.reviews[id]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func (m *MockReviewRepo) FindByRepository(ctx context.Context, repoID uuid.UUID) ([]*domain.PullRequestReview, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*domain.PullRequestReview
	for _, r := range m.reviews {
		if r.RepositoryID == repoID {
			list = append(list, r)
		}
	}
	return list, nil
}

// MockFindingRepo implements in-memory CodeFinding storage.
type MockFindingRepo struct {
	mu       sync.RWMutex
	findings map[uuid.UUID]*domain.CodeFinding
}

func NewMockFindingRepo() *MockFindingRepo {
	return &MockFindingRepo{
		findings: make(map[uuid.UUID]*domain.CodeFinding),
	}
}

func (m *MockFindingRepo) Create(ctx context.Context, f *domain.CodeFinding) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f.ID == uuid.Nil {
		f.ID = uuid.New()
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now().UTC()
	}
	m.findings[f.ID] = f
	return nil
}

func (m *MockFindingRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.CodeFinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.findings[id]
	if !ok {
		return nil, nil
	}
	return f, nil
}

func (m *MockFindingRepo) FindByReviewID(ctx context.Context, reviewID uuid.UUID) ([]*domain.CodeFinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*domain.CodeFinding
	for _, f := range m.findings {
		if f.ReviewID == reviewID {
			list = append(list, f)
		}
	}
	return list, nil
}

// MockTeamRepo implements in-memory Team storage.
type MockTeamRepo struct {
	mu    sync.RWMutex
	teams map[uuid.UUID]*domain.Team
}

func NewMockTeamRepo() *MockTeamRepo {
	return &MockTeamRepo{
		teams: make(map[uuid.UUID]*domain.Team),
	}
}

func (m *MockTeamRepo) Create(ctx context.Context, t *domain.Team) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	m.teams[t.ID] = t
	return nil
}

func (m *MockTeamRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.teams[id]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (m *MockTeamRepo) FindByWorkspace(ctx context.Context, wsID uuid.UUID) ([]*domain.Team, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*domain.Team
	for _, t := range m.teams {
		if t.WorkspaceID == wsID {
			list = append(list, t)
		}
	}
	return list, nil
}

// MockCliDeviceRepo implements in-memory CliDeviceRepository interface.
type MockCliDeviceRepo struct {
	mu      sync.RWMutex
	devices map[uuid.UUID]*domain.CliDevice
}

func NewMockCliDeviceRepo() *MockCliDeviceRepo {
	return &MockCliDeviceRepo{
		devices: make(map[uuid.UUID]*domain.CliDevice),
	}
}

func (m *MockCliDeviceRepo) Create(ctx context.Context, device *domain.CliDevice) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if device.ID == uuid.Nil {
		device.ID = uuid.New()
	}
	now := time.Now().UTC()
	if device.CreatedAt.IsZero() {
		device.CreatedAt = now
	}
	device.UpdatedAt = now
	m.devices[device.ID] = device
	return nil
}

func (m *MockCliDeviceRepo) FindByID(ctx context.Context, wsID, id uuid.UUID) (*domain.CliDevice, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.devices[id]
	if !ok || d.WorkspaceID != wsID {
		return nil, nil
	}
	return d, nil
}

func (m *MockCliDeviceRepo) FindByDeviceIdentifier(ctx context.Context, wsID uuid.UUID, identifier string) (*domain.CliDevice, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, d := range m.devices {
		if d.WorkspaceID == wsID && d.DeviceIdentifier == identifier {
			return d, nil
		}
	}
	return nil, nil
}

func (m *MockCliDeviceRepo) UpdateLastSeen(ctx context.Context, wsID, id uuid.UUID, clientVersion string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[id]
	if !ok || d.WorkspaceID != wsID {
		return fmt.Errorf("device not found")
	}
	now := time.Now().UTC()
	d.LastSeenAt = now
	d.ClientVersion = clientVersion
	d.UpdatedAt = now
	return nil
}

func (m *MockCliDeviceRepo) Revoke(ctx context.Context, wsID, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[id]
	if !ok || d.WorkspaceID != wsID {
		return fmt.Errorf("device not found")
	}
	d.IsRevoked = true
	d.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MockCliDeviceRepo) ListByUser(ctx context.Context, wsID, userID uuid.UUID) ([]*domain.CliDevice, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*domain.CliDevice
	for _, d := range m.devices {
		if d.WorkspaceID == wsID && d.UserID == userID {
			list = append(list, d)
		}
	}
	return list, nil
}

// MockCliSessionRepo implements in-memory CliSessionRepository interface.
type MockCliSessionRepo struct {
	mu       sync.RWMutex
	sessions map[uuid.UUID]*domain.CliAuthSession
}

func NewMockCliSessionRepo() *MockCliSessionRepo {
	return &MockCliSessionRepo{
		sessions: make(map[uuid.UUID]*domain.CliAuthSession),
	}
}

func (m *MockCliSessionRepo) Create(ctx context.Context, s *domain.CliAuthSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	s.UpdatedAt = time.Now().UTC()
	m.sessions[s.ID] = s
	return nil
}

func (m *MockCliSessionRepo) FindBySessionCode(ctx context.Context, sessionCode string) (*domain.CliAuthSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		if s.SessionCode == sessionCode {
			return s, nil
		}
	}
	return nil, nil
}

func (m *MockCliSessionRepo) FindByUserCode(ctx context.Context, userCode string) (*domain.CliAuthSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		if s.UserCode == userCode {
			return s, nil
		}
	}
	return nil, nil
}

func (m *MockCliSessionRepo) Authorize(ctx context.Context, userCode string, userID, wsID uuid.UUID, tokenPayload string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if s.UserCode == userCode {
			s.UserID = &userID
			s.WorkspaceID = &wsID
			s.TokenPayload = &tokenPayload
			s.Status = "AUTHORIZED"
			s.UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return fmt.Errorf("session not found for user code %s", userCode)
}

func (m *MockCliSessionRepo) Complete(ctx context.Context, sessionCode string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if s.SessionCode == sessionCode {
			s.Status = "COMPLETED"
			s.UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return fmt.Errorf("session not found for session code %s", sessionCode)
}

func (m *MockCliSessionRepo) PurgeExpired(ctx context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	now := time.Now().UTC()
	for id, s := range m.sessions {
		if s.ExpiresAt.Before(now) {
			delete(m.sessions, id)
			count++
		}
	}
	return count, nil
}

// MockTokenUsageRepo implements in-memory TokenUsageRepository interface.
type MockTokenUsageRepo struct {
	mu      sync.RWMutex
	records map[uuid.UUID]*domain.TokenUsageRecord
}

func NewMockTokenUsageRepo() *MockTokenUsageRepo {
	return &MockTokenUsageRepo{
		records: make(map[uuid.UUID]*domain.TokenUsageRecord),
	}
}

func (m *MockTokenUsageRepo) Record(ctx context.Context, usage *domain.TokenUsageRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if usage.ID == uuid.Nil {
		usage.ID = uuid.New()
	}
	if usage.CreatedAt.IsZero() {
		usage.CreatedAt = time.Now().UTC()
	}
	if usage.RecordedAt.IsZero() {
		usage.RecordedAt = time.Now().UTC()
	}
	m.records[usage.ID] = usage
	return nil
}

func (m *MockTokenUsageRepo) GetMonthlySpend(ctx context.Context, wsID uuid.UUID, year int, month time.Month) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var total float64
	for _, r := range m.records {
		if r.WorkspaceID == wsID && r.RecordedAt.Year() == year && r.RecordedAt.Month() == month {
			total += r.EstimatedCostUSD
		}
	}
	return total, nil
}

func (m *MockTokenUsageRepo) GetAggregatedUsage(ctx context.Context, wsID uuid.UUID, since time.Time) (map[string]int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	agg := make(map[string]int64)
	for _, r := range m.records {
		if r.WorkspaceID == wsID && !r.RecordedAt.Before(since) {
			agg[r.ModelName] += int64(r.TotalTokens)
		}
	}
	return agg, nil
}

// MockPlatformPRRepo implements in-memory PlatformPullRequestRepository interface.
type MockPlatformPRRepo struct {
	mu  sync.RWMutex
	prs map[uuid.UUID]*domain.PullRequest
}

func NewMockPlatformPRRepo() *MockPlatformPRRepo {
	return &MockPlatformPRRepo{
		prs: make(map[uuid.UUID]*domain.PullRequest),
	}
}

func (m *MockPlatformPRRepo) Upsert(ctx context.Context, pr *domain.PullRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pr.ID == uuid.Nil {
		pr.ID = uuid.New()
	}
	now := time.Now().UTC()
	if pr.CreatedAt.IsZero() {
		pr.CreatedAt = now
	}
	pr.UpdatedAt = now
	m.prs[pr.ID] = pr
	return nil
}

func (m *MockPlatformPRRepo) FindByNumber(ctx context.Context, wsID, repoID uuid.UUID, pullNumber int) (*domain.PullRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, pr := range m.prs {
		if pr.WorkspaceID == wsID && pr.RepositoryID == repoID && pr.PullNumber == pullNumber {
			return pr, nil
		}
	}
	return nil, nil
}

func (m *MockPlatformPRRepo) UpdateState(ctx context.Context, wsID, id uuid.UUID, state string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pr, ok := m.prs[id]
	if !ok || pr.WorkspaceID != wsID {
		return fmt.Errorf("pull request not found")
	}
	pr.State = state
	pr.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MockPlatformPRRepo) ListOpen(ctx context.Context, wsID, repoID uuid.UUID) ([]*domain.PullRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*domain.PullRequest
	for _, pr := range m.prs {
		if pr.WorkspaceID == wsID && pr.RepositoryID == repoID && pr.State == "open" {
			list = append(list, pr)
		}
	}
	return list, nil
}

// MockSandboxLeaseRepo implements in-memory SandboxLeaseRepository interface.
type MockSandboxLeaseRepo struct {
	mu     sync.RWMutex
	leases map[uuid.UUID]*repositories.SandboxLease
}

func NewMockSandboxLeaseRepo() *MockSandboxLeaseRepo {
	return &MockSandboxLeaseRepo{
		leases: make(map[uuid.UUID]*repositories.SandboxLease),
	}
}

func (m *MockSandboxLeaseRepo) AcquireLease(ctx context.Context, lease *repositories.SandboxLease) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if lease.ID == uuid.Nil {
		lease.ID = uuid.New()
	}
	now := time.Now().UTC()
	if lease.CreatedAt.IsZero() {
		lease.CreatedAt = now
	}
	lease.UpdatedAt = now
	m.leases[lease.ID] = lease
	return nil
}

func (m *MockSandboxLeaseRepo) FindByReviewID(ctx context.Context, wsID, reviewID uuid.UUID) (*repositories.SandboxLease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, l := range m.leases {
		if l.WorkspaceID == wsID && l.ReviewID == reviewID {
			return l, nil
		}
	}
	return nil, nil
}

func (m *MockSandboxLeaseRepo) ReleaseLease(ctx context.Context, wsID, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.leases[id]
	if !ok || l.WorkspaceID != wsID {
		return fmt.Errorf("lease not found")
	}
	l.Status = "RELEASED"
	now := time.Now().UTC()
	l.ReleasedAt = &now
	l.UpdatedAt = now
	return nil
}

func (m *MockSandboxLeaseRepo) PurgeExpired(ctx context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	var count int64
	for id, l := range m.leases {
		if l.Status == "ACTIVE" && l.ExpiresAt.Before(now) {
			delete(m.leases, id)
			count++
		}
	}
	return count, nil
}

// MockUserAssignmentRepo implements in-memory UserAssignmentRepository interface.
type MockUserAssignmentRepo struct {
	mu          sync.RWMutex
	assignments map[uuid.UUID]*repositories.UserRepositoryAssignment
}

func NewMockUserAssignmentRepo() *MockUserAssignmentRepo {
	return &MockUserAssignmentRepo{
		assignments: make(map[uuid.UUID]*repositories.UserRepositoryAssignment),
	}
}

func (m *MockUserAssignmentRepo) Assign(ctx context.Context, assignment *repositories.UserRepositoryAssignment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if assignment.ID == uuid.Nil {
		assignment.ID = uuid.New()
	}
	if assignment.CreatedAt.IsZero() {
		assignment.CreatedAt = time.Now().UTC()
	}
	assignment.UpdatedAt = time.Now().UTC()
	m.assignments[assignment.ID] = assignment
	return nil
}

func (m *MockUserAssignmentRepo) Revoke(ctx context.Context, wsID, userID, repoID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, a := range m.assignments {
		if a.WorkspaceID == wsID && a.UserID == userID && a.RepositoryID == repoID {
			delete(m.assignments, id)
			return nil
		}
	}
	return fmt.Errorf("assignment not found")
}

func (m *MockUserAssignmentRepo) ListByUser(ctx context.Context, wsID, userID uuid.UUID) ([]*repositories.UserRepositoryAssignment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*repositories.UserRepositoryAssignment
	for _, a := range m.assignments {
		if a.WorkspaceID == wsID && a.UserID == userID {
			list = append(list, a)
		}
	}
	return list, nil
}

func (m *MockUserAssignmentRepo) ListByRepository(ctx context.Context, wsID, repoID uuid.UUID) ([]*repositories.UserRepositoryAssignment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*repositories.UserRepositoryAssignment
	for _, a := range m.assignments {
		if a.WorkspaceID == wsID && a.RepositoryID == repoID {
			list = append(list, a)
		}
	}
	return list, nil
}

// MockDrixyRulesRepo implements in-memory DrixyRules storage.
type MockDrixyRulesRepo struct {
	mu    sync.RWMutex
	rules map[uuid.UUID]*domain.DrixyRules
}

func NewMockDrixyRulesRepo() *MockDrixyRulesRepo {
	return &MockDrixyRulesRepo{
		rules: make(map[uuid.UUID]*domain.DrixyRules),
	}
}

func (m *MockDrixyRulesRepo) Create(ctx context.Context, r *domain.DrixyRules) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	m.rules[r.ID] = r
	return nil
}

func (m *MockDrixyRulesRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.DrixyRules, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.rules[id]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func (m *MockDrixyRulesRepo) ListByWorkspace(ctx context.Context, wsID uuid.UUID) ([]*domain.DrixyRules, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*domain.DrixyRules
	for _, r := range m.rules {
		if r.WorkspaceID == wsID {
			list = append(list, r)
		}
	}
	return list, nil
}

// Verify interface compliance at compile time.
var (
	_ repositories.CliDeviceRepository           = (*MockCliDeviceRepo)(nil)
	_ repositories.CliSessionRepository          = (*MockCliSessionRepo)(nil)
	_ repositories.TokenUsageRepository          = (*MockTokenUsageRepo)(nil)
	_ repositories.PlatformPullRequestRepository = (*MockPlatformPRRepo)(nil)
	_ repositories.SandboxLeaseRepository        = (*MockSandboxLeaseRepo)(nil)
	_ repositories.UserAssignmentRepository      = (*MockUserAssignmentRepo)(nil)
)

// ============================================================================
// Comprehensive Concurrent & Multi-Tenant Tests
// ============================================================================

func TestMockWorkspaceRepo_ConcurrencyAndCRUD(t *testing.T) {
	repo := NewMockWorkspaceRepo()
	ctx := context.Background()

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	wsIDs := make([]uuid.UUID, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			ws := &domain.Workspace{
				BaseEntity: domain.BaseEntity{ID: uuid.New()},
				Name:       fmt.Sprintf("Tenant-%d", idx),
				Slug:       fmt.Sprintf("tenant-%d", idx),
				Tier:       "enterprise",
				Status:     "active",
			}
			err := repo.Create(ctx, ws)
			assert.NoError(t, err)
			wsIDs[idx] = ws.ID
		}(i)
	}
	wg.Wait()

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, numGoroutines)

	// Concurrent update
	var updateWg sync.WaitGroup
	updateWg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer updateWg.Done()
			found, err := repo.FindByID(ctx, wsIDs[idx])
			assert.NoError(t, err)
			require.NotNil(t, found)
			assert.Equal(t, fmt.Sprintf("Tenant-%d", idx), found.Name)

			found.Name = fmt.Sprintf("Updated-Tenant-%d", idx)
			err = repo.Update(ctx, found)
			assert.NoError(t, err)
		}(i)
	}
	updateWg.Wait()

	for i := 0; i < numGoroutines; i++ {
		found, err := repo.FindByID(ctx, wsIDs[i])
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, fmt.Sprintf("Updated-Tenant-%d", i), found.Name)
	}

	// Delete first 10
	for i := 0; i < 10; i++ {
		err := repo.Delete(ctx, wsIDs[i])
		assert.NoError(t, err)
		found, err := repo.FindByID(ctx, wsIDs[i])
		assert.NoError(t, err)
		assert.Nil(t, found)
	}

	remaining, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, remaining, numGoroutines-10)
}

func TestMockUserRepo_MultiTenantIsolation(t *testing.T) {
	repo := NewMockUserRepo()
	ctx := context.Background()

	ws1 := uuid.New()
	ws2 := uuid.New()

	for i := 0; i < 20; i++ {
		u := &domain.User{
			Email:       fmt.Sprintf("user%d@org1.com", i),
			FullName:    fmt.Sprintf("User %d Org1", i),
			IsActive:    true,
			WorkspaceID: ws1,
		}
		require.NoError(t, repo.Create(ctx, u))
	}

	for i := 0; i < 15; i++ {
		u := &domain.User{
			Email:       fmt.Sprintf("user%d@org2.com", i),
			FullName:    fmt.Sprintf("User %d Org2", i),
			IsActive:    true,
			WorkspaceID: ws2,
		}
		require.NoError(t, repo.Create(ctx, u))
	}

	org1Users, err := repo.FindByWorkspace(ctx, ws1)
	require.NoError(t, err)
	assert.Len(t, org1Users, 20)

	org2Users, err := repo.FindByWorkspace(ctx, ws2)
	require.NoError(t, err)
	assert.Len(t, org2Users, 15)

	found, err := repo.FindByEmail(ctx, "user5@org1.com")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, ws1, found.WorkspaceID)

	notFound, err := repo.FindByEmail(ctx, "nonexistent@nowhere.com")
	assert.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestMockCliDeviceRepo_Lifecycle(t *testing.T) {
	repo := NewMockCliDeviceRepo()
	ctx := context.Background()
	wsID := uuid.New()
	userID := uuid.New()

	device := &domain.CliDevice{
		TenantScopedEntity: domain.TenantScopedEntity{
			BaseEntity:  domain.BaseEntity{},
			WorkspaceID: wsID,
		},
		UserID:           userID,
		DeviceIdentifier: "sha256:abcd1234deadbeef",
		Hostname:         "developer-laptop",
		OS:               "linux",
		Arch:             "amd64",
		ClientVersion:    "1.4.0",
		IsRevoked:        false,
	}

	err := repo.Create(ctx, device)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, device.ID)

	found, err := repo.FindByDeviceIdentifier(ctx, wsID, "sha256:abcd1234deadbeef")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "developer-laptop", found.Hostname)

	err = repo.UpdateLastSeen(ctx, wsID, device.ID, "1.5.0")
	require.NoError(t, err)

	updated, err := repo.FindByID(ctx, wsID, device.ID)
	require.NoError(t, err)
	assert.Equal(t, "1.5.0", updated.ClientVersion)
	assert.False(t, updated.LastSeenAt.IsZero())

	err = repo.Revoke(ctx, wsID, device.ID)
	require.NoError(t, err)

	revoked, err := repo.FindByID(ctx, wsID, device.ID)
	require.NoError(t, err)
	assert.True(t, revoked.IsRevoked)

	devices, err := repo.ListByUser(ctx, wsID, userID)
	require.NoError(t, err)
	assert.Len(t, devices, 1)
}

func TestMockCliSessionRepo_ExpirationAndStatus(t *testing.T) {
	repo := NewMockCliSessionRepo()
	ctx := context.Background()

	now := time.Now().UTC()
	activeSession := &domain.CliAuthSession{
		SessionCode: "scandrix_auth_active_123",
		UserCode:    "ABCD-1234",
		Status:      "PENDING",
		ClientIP:    "127.0.0.1",
		UserAgent:   "ScanDrix-CLI/1.4.0",
		ExpiresAt:   now.Add(10 * time.Minute),
	}
	expiredSession := &domain.CliAuthSession{
		SessionCode: "scandrix_auth_expired_456",
		UserCode:    "EFGH-5678",
		Status:      "EXPIRED",
		ClientIP:    "10.0.0.1",
		UserAgent:   "ScanDrix-CLI/1.3.0",
		ExpiresAt:   now.Add(-5 * time.Minute),
	}

	require.NoError(t, repo.Create(ctx, activeSession))
	require.NoError(t, repo.Create(ctx, expiredSession))

	found, err := repo.FindBySessionCode(ctx, "scandrix_auth_active_123")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "PENDING", found.Status)

	foundByUser, err := repo.FindByUserCode(ctx, "ABCD-1234")
	require.NoError(t, err)
	require.NotNil(t, foundByUser)
	assert.Equal(t, activeSession.ID, foundByUser.ID)

	userID := uuid.New()
	wsID := uuid.New()
	err = repo.Authorize(ctx, "ABCD-1234", userID, wsID, "jwt_token_sample")
	require.NoError(t, err)

	authorized, err := repo.FindBySessionCode(ctx, "scandrix_auth_active_123")
	require.NoError(t, err)
	assert.Equal(t, "AUTHORIZED", authorized.Status)
	require.NotNil(t, authorized.TokenPayload)
	assert.Equal(t, "jwt_token_sample", *authorized.TokenPayload)
	require.NotNil(t, authorized.UserID)
	assert.Equal(t, userID, *authorized.UserID)

	err = repo.Complete(ctx, "scandrix_auth_active_123")
	require.NoError(t, err)

	completed, err := repo.FindBySessionCode(ctx, "scandrix_auth_active_123")
	require.NoError(t, err)
	assert.Equal(t, "COMPLETED", completed.Status)

	deletedCount, err := repo.PurgeExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deletedCount)

	expiredFound, err := repo.FindBySessionCode(ctx, "scandrix_auth_expired_456")
	assert.NoError(t, err)
	assert.Nil(t, expiredFound)
}

func TestMockTokenUsageRepo_Aggregation(t *testing.T) {
	repo := NewMockTokenUsageRepo()
	ctx := context.Background()
	wsID := uuid.New()

	now := time.Now().UTC()
	for i := 0; i < 10; i++ {
		record := &domain.TokenUsageRecord{
			WorkspaceID:      wsID,
			ModelName:        "claude-3-5-sonnet",
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
			EstimatedCostUSD: 0.005,
			OperationType:    "code_review",
			RecordedAt:       now.Add(time.Duration(i) * time.Minute),
		}
		require.NoError(t, repo.Record(ctx, record))
	}

	spend, err := repo.GetMonthlySpend(ctx, wsID, now.Year(), now.Month())
	require.NoError(t, err)
	assert.InDelta(t, 0.05, spend, 0.0001)

	agg, err := repo.GetAggregatedUsage(ctx, wsID, now.Add(-time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1500), agg["claude-3-5-sonnet"])

	emptyAgg, err := repo.GetAggregatedUsage(ctx, uuid.New(), now.Add(-time.Hour))
	require.NoError(t, err)
	assert.Empty(t, emptyAgg)
}

func TestMockPlatformPRRepo_UpsertAndQuery(t *testing.T) {
	repo := NewMockPlatformPRRepo()
	ctx := context.Background()
	wsID := uuid.New()
	repoID := uuid.New()

	for i := 1; i <= 5; i++ {
		pr := &domain.PullRequest{
			TenantScopedEntity: domain.TenantScopedEntity{
				WorkspaceID: wsID,
			},
			RepositoryID:   repoID,
			PullNumber:     i,
			Title:          fmt.Sprintf("PR #%d", i),
			AuthorUsername: "dev1",
			SourceBranch:   fmt.Sprintf("feature-%d", i),
			TargetBranch:   "main",
			HeadSHA:        fmt.Sprintf("sha-%d", i),
			BaseSHA:        "base-sha",
			State:          "open",
		}
		require.NoError(t, repo.Upsert(ctx, pr))
	}

	openPRs, err := repo.ListOpen(ctx, wsID, repoID)
	require.NoError(t, err)
	assert.Len(t, openPRs, 5)

	found, err := repo.FindByNumber(ctx, wsID, repoID, 3)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "PR #3", found.Title)

	err = repo.UpdateState(ctx, wsID, found.ID, "closed")
	require.NoError(t, err)

	openAfter, err := repo.ListOpen(ctx, wsID, repoID)
	require.NoError(t, err)
	assert.Len(t, openAfter, 4)

	notFound, err := repo.FindByNumber(ctx, uuid.New(), repoID, 1)
	assert.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestMockSandboxLeaseRepo_AcquisitionAndPurge(t *testing.T) {
	repo := NewMockSandboxLeaseRepo()
	ctx := context.Background()
	wsID := uuid.New()
	reviewID := uuid.New()

	now := time.Now().UTC()
	lease := &repositories.SandboxLease{
		TenantScopedEntity: domain.TenantScopedEntity{
			WorkspaceID: wsID,
		},
		ReviewID:        reviewID,
		SandboxProvider: "firecracker",
		ExternalLeaseID: "ext-lease-001",
		Port:            8080,
		Status:          "ACTIVE",
		ExpiresAt:       now.Add(5 * time.Minute),
	}

	require.NoError(t, repo.AcquireLease(ctx, lease))
	assert.NotEqual(t, uuid.Nil, lease.ID)

	found, err := repo.FindByReviewID(ctx, wsID, reviewID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "ACTIVE", found.Status)
	assert.Equal(t, "firecracker", found.SandboxProvider)

	err = repo.ReleaseLease(ctx, wsID, lease.ID)
	require.NoError(t, err)

	released, err := repo.FindByReviewID(ctx, wsID, reviewID)
	require.NoError(t, err)
	assert.Equal(t, "RELEASED", released.Status)
	assert.NotNil(t, released.ReleasedAt)

	// Create an already-expired lease
	expiredLease := &repositories.SandboxLease{
		TenantScopedEntity: domain.TenantScopedEntity{
			WorkspaceID: wsID,
		},
		ReviewID:        uuid.New(),
		SandboxProvider: "firecracker",
		ExternalLeaseID: "ext-lease-expired",
		Port:            8081,
		Status:          "ACTIVE",
		ExpiresAt:       now.Add(-10 * time.Minute),
	}
	require.NoError(t, repo.AcquireLease(ctx, expiredLease))

	purged, err := repo.PurgeExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), purged)
}

func TestMockUserAssignmentRepo_Assignments(t *testing.T) {
	repo := NewMockUserAssignmentRepo()
	ctx := context.Background()

	wsID := uuid.New()
	uID := uuid.New()
	repo1 := uuid.New()
	repo2 := uuid.New()

	a1 := &repositories.UserRepositoryAssignment{
		TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
		UserID:             uID,
		RepositoryID:       repo1,
		Role:               "ADMIN",
		IsActive:           true,
	}
	a2 := &repositories.UserRepositoryAssignment{
		TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
		UserID:             uID,
		RepositoryID:       repo2,
		Role:               "REVIEWER",
		IsActive:           true,
	}

	require.NoError(t, repo.Assign(ctx, a1))
	require.NoError(t, repo.Assign(ctx, a2))

	userAssignments, err := repo.ListByUser(ctx, wsID, uID)
	require.NoError(t, err)
	assert.Len(t, userAssignments, 2)

	repo1Assigns, err := repo.ListByRepository(ctx, wsID, repo1)
	require.NoError(t, err)
	assert.Len(t, repo1Assigns, 1)

	err = repo.Revoke(ctx, wsID, uID, repo1)
	require.NoError(t, err)

	afterRevoke, err := repo.ListByUser(ctx, wsID, uID)
	require.NoError(t, err)
	assert.Len(t, afterRevoke, 1)

	err = repo.Revoke(ctx, wsID, uuid.New(), uuid.New())
	assert.Error(t, err)
}

func TestMockDrixyRulesRepo_RulesManagement(t *testing.T) {
	repo := NewMockDrixyRulesRepo()
	ctx := context.Background()
	wsID := uuid.New()

	for i := 0; i < 5; i++ {
		rule := &domain.DrixyRules{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
			RuleKey:            fmt.Sprintf("SEC-%03d", i),
			Name:               fmt.Sprintf("Security-Rule-%d", i),
			Category:           "SECURITY",
			Severity:           "HIGH",
			Description:        fmt.Sprintf("Checks security vulnerability pattern %d", i),
			PromptInstructions: "Detect insecure patterns",
			IsActive:           true,
			WeightMultiplier:   1.0,
		}
		require.NoError(t, repo.Create(ctx, rule))
	}

	rules, err := repo.ListByWorkspace(ctx, wsID)
	require.NoError(t, err)
	assert.Len(t, rules, 5)

	for _, r := range rules {
		found, err := repo.FindByID(ctx, r.ID)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, "HIGH", found.Severity)
		assert.Equal(t, "SECURITY", found.Category)
		assert.True(t, found.IsActive)
	}

	// Isolation: other workspace sees nothing
	otherRules, err := repo.ListByWorkspace(ctx, uuid.New())
	require.NoError(t, err)
	assert.Empty(t, otherRules)
}

package infrastructure

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/domain"
)

// InMemoryUserRepository provides a concurrent-safe in-memory store for accounts.
type InMemoryUserRepository struct {
	mu    sync.RWMutex
	users map[uuid.UUID]domain.User
}

func NewInMemoryUserRepository() *InMemoryUserRepository {
	return &InMemoryUserRepository{users: make(map[uuid.UUID]domain.User)}
}

func (r *InMemoryUserRepository) Find(ctx context.Context, filter map[string]any) ([]domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []domain.User
	for _, u := range r.users {
		if matchesUserFilter(u, filter) {
			result = append(result, u)
		}
	}
	return result, nil
}

func (r *InMemoryUserRepository) FindOne(ctx context.Context, filter map[string]any) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, u := range r.users {
		if matchesUserFilter(u, filter) {
			cpy := u
			return &cpy, nil
		}
	}
	return nil, nil
}

func (r *InMemoryUserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, u := range r.users {
		if u.Email == email {
			cpy := u
			return &cpy, nil
		}
	}
	return nil, nil
}

func (r *InMemoryUserRepository) FindByUUID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	u, exists := r.users[id]
	if !exists {
		return nil, nil
	}
	cpy := u
	return &cpy, nil
}

func (r *InMemoryUserRepository) Create(ctx context.Context, user domain.User) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if user.UUID == uuid.Nil {
		user.UUID = uuid.New()
	}
	now := time.Now().UTC()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	user.UpdatedAt = now

	r.users[user.UUID] = user
	cpy := user
	return &cpy, nil
}

func (r *InMemoryUserRepository) Update(ctx context.Context, id uuid.UUID, updates map[string]any) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	u, exists := r.users[id]
	if !exists {
		return nil, errors.New("user not found")
	}

	if role, ok := updates["role"].(string); ok {
		u.Role = domain.Role(role)
	}
	if status, ok := updates["status"].(string); ok {
		u.Status = domain.UserStatus(status)
	}
	if pw, ok := updates["password"].(string); ok {
		u.Password = pw
	}
	u.UpdatedAt = time.Now().UTC()

	r.users[id] = u
	cpy := u
	return &cpy, nil
}

func (r *InMemoryUserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.users, id)
	return nil
}

func (r *InMemoryUserRepository) Count(ctx context.Context, filter map[string]any) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, u := range r.users {
		if matchesUserFilter(u, filter) {
			count++
		}
	}
	return count, nil
}

func matchesUserFilter(u domain.User, filter map[string]any) bool {
	if filter == nil {
		return true
	}
	if email, ok := filter["email"].(string); ok && u.Email != email {
		return false
	}
	if role, ok := filter["role"].(domain.Role); ok && u.Role != role {
		return false
	}
	if status, ok := filter["status"].(string); ok && string(u.Status) != status {
		return false
	}
	if id, ok := filter["uuid"].(uuid.UUID); ok && u.UUID != id {
		return false
	}
	return true
}

// InMemoryAuthRepository manages active refresh tokens in memory.
type InMemoryAuthRepository struct {
	mu       sync.RWMutex
	sessions map[string]domain.AuthSession // Keyed by refresh token
}

func NewInMemoryAuthRepository() *InMemoryAuthRepository {
	return &InMemoryAuthRepository{sessions: make(map[string]domain.AuthSession)}
}

func (r *InMemoryAuthRepository) SaveRefreshToken(ctx context.Context, session domain.AuthSession) (*domain.AuthSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if session.UUID == uuid.Nil {
		session.UUID = uuid.New()
	}
	r.sessions[session.RefreshToken] = session
	cpy := session
	return &cpy, nil
}

func (r *InMemoryAuthRepository) FindRefreshToken(ctx context.Context, token string) (*domain.AuthSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, exists := r.sessions[token]
	if !exists {
		return nil, nil
	}
	cpy := s
	return &cpy, nil
}

func (r *InMemoryAuthRepository) UpdateRefreshToken(ctx context.Context, session domain.AuthSession) (*domain.AuthSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sessions[session.RefreshToken] = session
	cpy := session
	return &cpy, nil
}

func (r *InMemoryAuthRepository) DeactivateRefreshToken(ctx context.Context, userUUID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for k, s := range r.sessions {
		if s.UserUUID == userUUID {
			s.Used = true
			r.sessions[k] = s
		}
	}
	return nil
}

// InMemoryCliAuthSessionRepository manages CLI device and loopback sessions in memory.
type InMemoryCliAuthSessionRepository struct {
	mu       sync.RWMutex
	sessions map[uuid.UUID]domain.CliAuthSession
}

func NewInMemoryCliAuthSessionRepository() *InMemoryCliAuthSessionRepository {
	return &InMemoryCliAuthSessionRepository{sessions: make(map[uuid.UUID]domain.CliAuthSession)}
}

func (r *InMemoryCliAuthSessionRepository) Create(ctx context.Context, session domain.CliAuthSession) (*domain.CliAuthSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if session.UUID == uuid.Nil {
		session.UUID = uuid.New()
	}
	r.sessions[session.UUID] = session
	cpy := session
	return &cpy, nil
}

func (r *InMemoryCliAuthSessionRepository) FindByState(ctx context.Context, state string) (*domain.CliAuthSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, s := range r.sessions {
		if s.State == state {
			cpy := s
			return &cpy, nil
		}
	}
	return nil, nil
}

func (r *InMemoryCliAuthSessionRepository) FindByDeviceCode(ctx context.Context, code string) (*domain.CliAuthSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, s := range r.sessions {
		if s.DeviceCode != nil && *s.DeviceCode == code {
			cpy := s
			return &cpy, nil
		}
	}
	return nil, nil
}

func (r *InMemoryCliAuthSessionRepository) FindByUserCode(ctx context.Context, code string) (*domain.CliAuthSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, s := range r.sessions {
		if s.UserCode != nil && *s.UserCode == code && s.Status == domain.CliAuthStatusPending {
			cpy := s
			return &cpy, nil
		}
	}
	return nil, nil
}

func (r *InMemoryCliAuthSessionRepository) Complete(
	ctx context.Context,
	id uuid.UUID,
	tokens domain.TokenResponse,
	userUUID uuid.UUID,
	userEmail string,
) (*domain.CliAuthSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, exists := r.sessions[id]
	if !exists {
		return nil, errors.New("CLI session not found")
	}

	now := time.Now().UTC()
	s.AccessToken = &tokens.AccessToken
	s.RefreshToken = &tokens.RefreshToken
	s.UserUUID = &userUUID
	s.UserEmail = &userEmail
	s.Status = domain.CliAuthStatusCompleted
	s.CompletedAt = &now
	s.UpdatedAt = now

	r.sessions[id] = s
	cpy := s
	return &cpy, nil
}

func (r *InMemoryCliAuthSessionRepository) MarkConsumed(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, exists := r.sessions[id]
	if !exists {
		return errors.New("CLI session not found")
	}
	now := time.Now().UTC()
	s.Status = domain.CliAuthStatusConsumed
	s.ConsumedAt = &now
	s.UpdatedAt = now
	r.sessions[id] = s
	return nil
}

func (r *InMemoryCliAuthSessionRepository) MarkDenied(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, exists := r.sessions[id]
	if !exists {
		return errors.New("CLI session not found")
	}
	s.Status = domain.CliAuthStatusDenied
	s.UpdatedAt = time.Now().UTC()
	r.sessions[id] = s
	return nil
}

func (r *InMemoryCliAuthSessionRepository) ExpirePending(ctx context.Context, now time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	for id, s := range r.sessions {
		if s.Status == domain.CliAuthStatusPending && now.After(s.ExpiresAt) {
			s.Status = domain.CliAuthStatusExpired
			s.UpdatedAt = now
			r.sessions[id] = s
			count++
		}
	}
	return count, nil
}

// InMemoryPermissionsRepository manages repository assignment records in memory.
type InMemoryPermissionsRepository struct {
	mu    sync.RWMutex
	perms map[uuid.UUID]domain.Permissions
}

func NewInMemoryPermissionsRepository() *InMemoryPermissionsRepository {
	return &InMemoryPermissionsRepository{perms: make(map[uuid.UUID]domain.Permissions)}
}

func (r *InMemoryPermissionsRepository) Create(ctx context.Context, perms domain.Permissions) (*domain.Permissions, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if perms.UUID == uuid.Nil {
		perms.UUID = uuid.New()
	}
	now := time.Now().UTC()
	perms.CreatedAt = now
	perms.UpdatedAt = now

	r.perms[perms.UUID] = perms
	cpy := perms
	return &cpy, nil
}

func (r *InMemoryPermissionsRepository) FindByUserUUID(ctx context.Context, userUUID uuid.UUID) (*domain.Permissions, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.perms {
		if p.UserUUID == userUUID {
			cpy := p
			return &cpy, nil
		}
	}
	return nil, nil
}

func (r *InMemoryPermissionsRepository) Update(ctx context.Context, id uuid.UUID, repoIDs []string) (*domain.Permissions, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.perms[id]
	if !exists {
		return nil, errors.New("permissions record not found")
	}

	p.AssignedRepositoryIDs = repoIDs
	p.UpdatedAt = time.Now().UTC()
	r.perms[id] = p
	cpy := p
	return &cpy, nil
}

func (r *InMemoryPermissionsRepository) Delete(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.perms, id)
	return nil
}

// InMemoryProfileRepository manages user profiles in memory.
type InMemoryProfileRepository struct {
	mu       sync.RWMutex
	profiles map[uuid.UUID]domain.UserProfile
}

func NewInMemoryProfileRepository() *InMemoryProfileRepository {
	return &InMemoryProfileRepository{profiles: make(map[uuid.UUID]domain.UserProfile)}
}

func (r *InMemoryProfileRepository) Create(ctx context.Context, profile domain.UserProfile) (*domain.UserProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if profile.UUID == uuid.Nil {
		profile.UUID = uuid.New()
	}
	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now

	r.profiles[profile.UUID] = profile
	cpy := profile
	return &cpy, nil
}

func (r *InMemoryProfileRepository) FindByUserUUID(ctx context.Context, userUUID uuid.UUID) (*domain.UserProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.profiles {
		if p.UserUUID == userUUID {
			cpy := p
			return &cpy, nil
		}
	}
	return nil, nil
}

func (r *InMemoryProfileRepository) Update(ctx context.Context, id uuid.UUID, updates map[string]any) (*domain.UserProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.profiles[id]
	if !exists {
		return nil, errors.New("profile not found")
	}

	if name, ok := updates["name"].(string); ok {
		p.Name = name
	}
	if phone, ok := updates["phone"].(string); ok {
		p.Phone = phone
	}
	if pos, ok := updates["position"].(string); ok {
		p.Position = pos
	}
	if img, ok := updates["img"].(string); ok {
		p.Img = img
	}
	if status, ok := updates["status"].(bool); ok {
		p.Status = status
	}
	if ref, ok := updates["referral_source"].(string); ok {
		p.ReferralSource = ref
	}
	if goal, ok := updates["primary_goal"].(string); ok {
		p.PrimaryGoal = goal
	}
	p.UpdatedAt = time.Now().UTC()

	r.profiles[id] = p
	cpy := p
	return &cpy, nil
}

func (r *InMemoryProfileRepository) UpdateByUserID(ctx context.Context, userUUID uuid.UUID, profile domain.UserProfile) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	for id, p := range r.profiles {
		if p.UserUUID == userUUID {
			if profile.Name != "" {
				p.Name = profile.Name
			}
			if profile.Phone != "" {
				p.Phone = profile.Phone
			}
			p.UpdatedAt = now
			r.profiles[id] = p
			return nil
		}
	}

	// Create if not found
	profile.UUID = uuid.New()
	profile.UserUUID = userUUID
	profile.Status = true
	profile.CreatedAt = now
	profile.UpdatedAt = now
	r.profiles[profile.UUID] = profile
	return nil
}

func (r *InMemoryProfileRepository) DeleteOne(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.profiles, id)
	return nil
}

// InMemoryProfileConfigRepository stores preference keys in memory.
type InMemoryProfileConfigRepository struct {
	mu      sync.RWMutex
	configs map[uuid.UUID]domain.ProfileConfig
}

func NewInMemoryProfileConfigRepository() *InMemoryProfileConfigRepository {
	return &InMemoryProfileConfigRepository{configs: make(map[uuid.UUID]domain.ProfileConfig)}
}

func (r *InMemoryProfileConfigRepository) Find(ctx context.Context, profileUUID uuid.UUID) ([]domain.ProfileConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []domain.ProfileConfig
	for _, c := range r.configs {
		if c.ProfileUUID == profileUUID {
			list = append(list, c)
		}
	}
	return list, nil
}

func (r *InMemoryProfileConfigRepository) FindOne(ctx context.Context, profileUUID uuid.UUID, key domain.ProfileConfigKey) (*domain.ProfileConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, c := range r.configs {
		if c.ProfileUUID == profileUUID && c.ConfigKey == key {
			cpy := c
			return &cpy, nil
		}
	}
	return nil, nil
}

func (r *InMemoryProfileConfigRepository) Create(ctx context.Context, config domain.ProfileConfig) (*domain.ProfileConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if config.UUID == uuid.Nil {
		config.UUID = uuid.New()
	}
	r.configs[config.UUID] = config
	cpy := config
	return &cpy, nil
}

func (r *InMemoryProfileConfigRepository) Update(ctx context.Context, id uuid.UUID, value any, status bool) (*domain.ProfileConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, exists := r.configs[id]
	if !exists {
		return nil, errors.New("profile config not found")
	}

	c.ConfigValue = value
	c.Status = status
	c.UpdatedAt = time.Now().UTC()
	r.configs[id] = c
	cpy := c
	return &cpy, nil
}

func (r *InMemoryProfileConfigRepository) Delete(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.configs, id)
	return nil
}

package cliauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// Standard RFC 8628 timing constants for OAuth 2.0 Device Authorization Grant
const (
	DeviceTTLSeconds    = 600 // 10 minutes
	PollIntervalSeconds = 5   // 5 seconds
	// User code alphabet excludes easily confused characters (0, O, 1, I)
	UserCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

var (
	ErrSessionNotFound = errors.New("cli auth session not found")
	ErrSessionExpired  = errors.New("cli auth session expired")
	ErrSessionConsumed = errors.New("cli auth session already consumed")
	ErrSessionPending  = errors.New("authorization pending")
	ErrSessionDenied   = errors.New("authorization denied by user")
	ErrSlowDown        = errors.New("slow_down")
	ErrTooManyAttempts = errors.New("too many invalid verification attempts")
)

// SessionStatus tracks state of RFC 8628 device authorization.
type SessionStatus string

const (
	StatusPending   SessionStatus = "pending"
	StatusCompleted SessionStatus = "completed"
	StatusConsumed  SessionStatus = "consumed"
	StatusDenied    SessionStatus = "denied"
	StatusExpired   SessionStatus = "expired"
)

// CLIDeviceSession encapsulates an in-flight or completed terminal login.
type CLIDeviceSession struct {
	UUID         uuid.UUID     `json:"uuid"`
	State        string        `json:"state"`
	DeviceCode   string        `json:"device_code"`
	UserCode     string        `json:"user_code"`
	RedirectURI  string        `json:"redirect_uri,omitempty"`
	Mode         string        `json:"mode"`
	Status       SessionStatus `json:"status"`
	AccessToken  string        `json:"access_token,omitempty"`
	RefreshToken string        `json:"refresh_token,omitempty"`
	UserID       *uuid.UUID    `json:"user_id,omitempty"`
	UserEmail    string        `json:"user_email,omitempty"`
	UserAgent    string        `json:"user_agent,omitempty"`
	ExpiresAt    time.Time     `json:"expires_at"`
	ConsumedAt   *time.Time    `json:"consumed_at,omitempty"`
	CompletedAt  *time.Time    `json:"completed_at,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// DeviceLoginInitiateResult is returned to the CLI client.
type DeviceLoginInitiateResult struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`

	// Standard camelCase serialization fields
	DeviceCodeCamel              string `json:"deviceCode,omitempty"`
	UserCodeCamel                string `json:"userCode,omitempty"`
	VerificationURICamel         string `json:"verificationUri,omitempty"`
	VerificationURICompleteCamel string `json:"verificationUriComplete,omitempty"`
	ExpiresInCamel               int    `json:"expiresIn,omitempty"`
}

// DeviceLoginPollResult is returned when polling the session state.
type DeviceLoginPollResult struct {
	Status       SessionStatus `json:"status"`
	AccessToken  string        `json:"access_token,omitempty"`
	RefreshToken string        `json:"refresh_token,omitempty"`
	UserEmail    string        `json:"user_email,omitempty"`
	Interval     int           `json:"interval,omitempty"`

	// Standard camelCase serialization fields
	AccessTokenCamel  string `json:"accessToken,omitempty"`
	RefreshTokenCamel string `json:"refreshToken,omitempty"`
	UserEmailCamel    string `json:"userEmail,omitempty"`
}

// SessionStore defines the storage contract for CLI auth sessions.
type SessionStore interface {
	CreateSession(ctx context.Context, session *CLIDeviceSession) error
	GetByDeviceCode(ctx context.Context, deviceCode string) (*CLIDeviceSession, error)
	GetByUserCode(ctx context.Context, userCode string) (*CLIDeviceSession, error)
	CompleteSession(ctx context.Context, userCode string, accessToken, refreshToken string, userID uuid.UUID, email string) error
	MarkConsumed(ctx context.Context, sessionID uuid.UUID) error
}

// InMemorySessionStore provides a thread-safe implementation of SessionStore.
type InMemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[uuid.UUID]*CLIDeviceSession
}

// NewInMemorySessionStore creates a new in-memory session store.
func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		sessions: make(map[uuid.UUID]*CLIDeviceSession),
	}
}

func (s *InMemorySessionStore) CreateSession(ctx context.Context, session *CLIDeviceSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.UUID] = session
	return nil
}

func (s *InMemorySessionStore) GetByDeviceCode(ctx context.Context, deviceCode string) (*CLIDeviceSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sess := range s.sessions {
		if sess.DeviceCode == deviceCode {
			cpy := *sess
			return &cpy, nil
		}
	}
	return nil, ErrSessionNotFound
}

func (s *InMemorySessionStore) GetByUserCode(ctx context.Context, userCode string) (*CLIDeviceSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(userCode), "-", ""))
	for _, sess := range s.sessions {
		sessNormalized := strings.ToUpper(strings.ReplaceAll(sess.UserCode, "-", ""))
		if sessNormalized == normalized {
			cpy := *sess
			return &cpy, nil
		}
	}
	return nil, ErrSessionNotFound
}

func (s *InMemorySessionStore) CompleteSession(ctx context.Context, userCode string, accessToken, refreshToken string, userID uuid.UUID, email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cleanCode := strings.ToUpper(strings.TrimSpace(userCode))
	for _, sess := range s.sessions {
		if sess.UserCode == cleanCode {
			if sess.Status != StatusPending {
				return fmt.Errorf("session is %s", sess.Status)
			}
			now := time.Now().UTC()
			sess.Status = StatusCompleted
			sess.AccessToken = accessToken
			sess.RefreshToken = refreshToken
			sess.UserID = &userID
			sess.UserEmail = email
			sess.CompletedAt = &now
			sess.UpdatedAt = now
			return nil
		}
	}
	return ErrSessionNotFound
}

func (s *InMemorySessionStore) MarkConsumed(ctx context.Context, sessionID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}
	now := time.Now().UTC()
	sess.Status = StatusConsumed
	sess.ConsumedAt = &now
	sess.UpdatedAt = now
	return nil
}

// DeviceFlowManager manages the RFC 8628 device authorization grant.
type DeviceFlowManager struct {
	store               SessionStore
	appBaseURL          string
	mu                  sync.RWMutex
	lastPollTime        map[string]time.Time
	pollIntervals       map[string]int
	failedAttempts      map[string]int
	failedByUser        map[string]int
	failedByClient      map[string]int
	consecutiveFailures int
	lastFailureTime     time.Time
}

type contextKey string

const ContextKeyClientIP contextKey = "device_flow_client_ip"

// WithClientIP binds a client IP address into the context for device code brute-force protection.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, ContextKeyClientIP, ip)
}

// NewDeviceFlowManager initializes the RFC 8628 manager.
func NewDeviceFlowManager(store SessionStore, appBaseURL string) *DeviceFlowManager {
	if store == nil {
		store = NewInMemorySessionStore()
	}
	if appBaseURL == "" {
		appBaseURL = "http://localhost:3000"
	}
	appBaseURL = strings.TrimSuffix(appBaseURL, "/")
	return &DeviceFlowManager{
		store:          store,
		appBaseURL:     appBaseURL,
		lastPollTime:   make(map[string]time.Time),
		pollIntervals:  make(map[string]int),
		failedAttempts: make(map[string]int),
		failedByUser:   make(map[string]int),
		failedByClient: make(map[string]int),
	}
}

// InitiateDeviceLogin begins a terminal login session.
func (m *DeviceFlowManager) InitiateDeviceLogin(ctx context.Context, userAgent string) (*DeviceLoginInitiateResult, error) {
	// 1. Generate 32 bytes of cryptographically secure random device code
	devBytes := make([]byte, 32)
	if _, err := rand.Read(devBytes); err != nil {
		return nil, fmt.Errorf("failed generating entropy: %w", err)
	}
	deviceCode := hex.EncodeToString(devBytes)

	// 2. Generate unbiased 8-character user code formatted as XXXX-XXXX
	userCode, err := GenerateUnbiasedUserCode()
	if err != nil {
		return nil, fmt.Errorf("failed generating user code: %w", err)
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("failed generating state entropy: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	now := time.Now().UTC()
	expiresAt := now.Add(DeviceTTLSeconds * time.Second)

	session := &CLIDeviceSession{
		UUID:       uuid.New(),
		State:      state,
		DeviceCode: deviceCode,
		UserCode:   userCode,
		Mode:       "device",
		Status:     StatusPending,
		UserAgent:  userAgent,
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := m.store.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("failed creating session: %w", err)
	}

	m.mu.Lock()
	if m.pollIntervals == nil {
		m.pollIntervals = make(map[string]int)
	}
	m.pollIntervals[deviceCode] = PollIntervalSeconds
	m.mu.Unlock()

	verificationURI := fmt.Sprintf("%s/cli/authorize", m.appBaseURL)
	verificationURIComplete := fmt.Sprintf("%s?code=%s", verificationURI, userCode)

	return &DeviceLoginInitiateResult{
		DeviceCode:                   deviceCode,
		UserCode:                     userCode,
		VerificationURI:              verificationURI,
		VerificationURIComplete:      verificationURIComplete,
		ExpiresIn:                    DeviceTTLSeconds,
		Interval:                     PollIntervalSeconds,
		DeviceCodeCamel:              deviceCode,
		UserCodeCamel:                userCode,
		VerificationURICamel:         verificationURI,
		VerificationURICompleteCamel: verificationURIComplete,
		ExpiresInCamel:               DeviceTTLSeconds,
	}, nil
}

// PollDeviceLogin checks session state and delivers tokens once upon completion.
// Enforces RFC 8628 §3.5 polling rate limiting with slow_down response and interval backoff.
func (m *DeviceFlowManager) PollDeviceLogin(ctx context.Context, deviceCode string) (*DeviceLoginPollResult, error) {
	if deviceCode == "" {
		return &DeviceLoginPollResult{Status: StatusExpired}, ErrSessionNotFound
	}

	sess, err := m.store.GetByDeviceCode(ctx, deviceCode)
	if err != nil {
		return &DeviceLoginPollResult{Status: StatusExpired}, ErrSessionNotFound
	}

	// Check expiration
	if sess.Status == StatusPending && time.Now().UTC().After(sess.ExpiresAt) {
		return &DeviceLoginPollResult{Status: StatusExpired}, nil
	}

	m.mu.Lock()
	if m.pollIntervals == nil {
		m.pollIntervals = make(map[string]int)
	}
	if m.lastPollTime == nil {
		m.lastPollTime = make(map[string]time.Time)
	}

	reqInterval := m.pollIntervals[deviceCode]
	if reqInterval <= 0 {
		reqInterval = PollIntervalSeconds
		m.pollIntervals[deviceCode] = reqInterval
	}

	// RFC 8628 §3.5: slow_down is returned when authorization is pending and client polls too fast
	if sess.Status == StatusPending {
		if last, exists := m.lastPollTime[deviceCode]; exists {
			elapsed := time.Since(last)
			if elapsed < time.Duration(reqInterval)*time.Second {
				// Polling too fast: increment interval by 5 seconds
				m.pollIntervals[deviceCode] += 5
				newInterval := m.pollIntervals[deviceCode]
				m.mu.Unlock()
				return &DeviceLoginPollResult{
					Status:   "slow_down",
					Interval: newInterval,
				}, ErrSlowDown
			}
		}
		m.lastPollTime[deviceCode] = time.Now()
	}
	m.mu.Unlock()

	if sess.Status != StatusCompleted {
		return &DeviceLoginPollResult{Status: sess.Status, Interval: reqInterval}, nil
	}

	// One-time consumption: mark consumed and clear re-fetchability
	_ = m.store.MarkConsumed(ctx, sess.UUID)

	return &DeviceLoginPollResult{
		Status:            StatusCompleted,
		AccessToken:       sess.AccessToken,
		RefreshToken:      sess.RefreshToken,
		UserEmail:         sess.UserEmail,
		Interval:          reqInterval,
		AccessTokenCamel:  sess.AccessToken,
		RefreshTokenCamel: sess.RefreshToken,
		UserEmailCamel:    sess.UserEmail,
	}, nil
}

// CompleteDeviceLogin is called by the browser when an authenticated user approves the user code.
// Enforces rate-limiting on user code verification attempts to prevent brute-forcing.
func (m *DeviceFlowManager) CompleteDeviceLogin(ctx context.Context, userCode string, accessToken, refreshToken string, user *models.AccountProfile) error {
	if user == nil {
		return errors.New("user profile required to complete device login")
	}

	cleanCode := strings.ToUpper(strings.TrimSpace(userCode))

	clientKey := ""
	if v := ctx.Value(ContextKeyClientIP); v != nil {
		if s, ok := v.(string); ok {
			clientKey = strings.TrimSpace(s)
		}
	}

	m.mu.Lock()
	if m.failedAttempts == nil {
		m.failedAttempts = make(map[string]int)
	}
	if m.failedByUser == nil {
		m.failedByUser = make(map[string]int)
	}
	if m.failedByClient == nil {
		m.failedByClient = make(map[string]int)
	}
	// 1. Code-specific lockout (prevents targeting a specific known code)
	if m.failedAttempts[cleanCode] >= 5 {
		m.mu.Unlock()
		return ErrTooManyAttempts
	}
	// 2. User-specific lockout (prevents authenticated account scanning)
	if user.Email != "" && m.failedByUser[user.Email] >= 5 {
		m.mu.Unlock()
		return ErrTooManyAttempts
	}
	// 3. Client IP lockout (prevents single origin from sweeping across codes)
	if clientKey != "" && m.failedByClient[clientKey] >= 5 {
		m.mu.Unlock()
		return ErrTooManyAttempts
	}
	// 4. Global burst lockout (prevents distributed dictionary attacks across codes)
	if time.Since(m.lastFailureTime) < time.Minute && m.consecutiveFailures >= 10 {
		m.mu.Unlock()
		return ErrTooManyAttempts
	}
	m.mu.Unlock()

	sess, err := m.store.GetByUserCode(ctx, cleanCode)
	if err != nil {
		m.mu.Lock()
		m.failedAttempts[cleanCode]++
		if user.Email != "" {
			m.failedByUser[user.Email]++
		}
		if clientKey != "" {
			m.failedByClient[clientKey]++
		}
		if time.Since(m.lastFailureTime) >= time.Minute {
			m.consecutiveFailures = 0
		}
		m.consecutiveFailures++
		m.lastFailureTime = time.Now()
		m.mu.Unlock()
		return ErrSessionNotFound
	}

	if sess.Status != StatusPending {
		return fmt.Errorf("session is %s", sess.Status)
	}

	if time.Now().UTC().After(sess.ExpiresAt) {
		return ErrSessionExpired
	}

	m.mu.Lock()
	delete(m.failedAttempts, cleanCode)
	if user.Email != "" {
		delete(m.failedByUser, user.Email)
	}
	if clientKey != "" {
		delete(m.failedByClient, clientKey)
	}
	m.consecutiveFailures = 0
	m.mu.Unlock()

	return m.store.CompleteSession(ctx, cleanCode, accessToken, refreshToken, user.ID, user.Email)
}

// GetSessionByUserCode retrieves a copy of the pending device session without modifying its state.
func (m *DeviceFlowManager) GetSessionByUserCode(ctx context.Context, userCode string) (*CLIDeviceSession, error) {
	cleanCode := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(userCode), "-", ""))
	if cleanCode == "" {
		return nil, ErrSessionNotFound
	}
	return m.store.GetByUserCode(ctx, cleanCode)
}

// GetSessionByDeviceCode retrieves a copy of the device session by device code.
func (m *DeviceFlowManager) GetSessionByDeviceCode(ctx context.Context, deviceCode string) (*CLIDeviceSession, error) {
	trimmed := strings.TrimSpace(deviceCode)
	if trimmed == "" {
		return nil, ErrSessionNotFound
	}
	return m.store.GetByDeviceCode(ctx, trimmed)
}

// GenerateUnbiasedUserCode generates an 8-character code formatted as XXXX-XXXX
// using rejection sampling to prevent modulo bias (CodeQL js/biased-cryptographic-random).
func GenerateUnbiasedUserCode() (string, error) {
	alphabetLength := len(UserCodeAlphabet)
	maxUnbiased := (256 / alphabetLength) * alphabetLength

	out := make([]byte, 8)
	filled := 0
	buf := make([]byte, 16)

	for filled < 8 {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if int(b) >= maxUnbiased {
				continue // Discard biased values in remainder range
			}
			out[filled] = UserCodeAlphabet[int(b)%alphabetLength]
			filled++
			if filled == 8 {
				break
			}
		}
	}

	return fmt.Sprintf("%s-%s", string(out[:4]), string(out[4:])), nil
}

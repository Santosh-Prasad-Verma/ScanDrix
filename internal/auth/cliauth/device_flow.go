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

// Standard RFC 8628 timing constants matching Kodus
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
}

// DeviceLoginPollResult is returned when polling the session state.
type DeviceLoginPollResult struct {
	Status       SessionStatus `json:"status"`
	AccessToken  string        `json:"access_token,omitempty"`
	RefreshToken string        `json:"refresh_token,omitempty"`
	UserEmail    string        `json:"user_email,omitempty"`
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
	cleanCode := strings.ToUpper(strings.TrimSpace(userCode))
	for _, sess := range s.sessions {
		if sess.UserCode == cleanCode {
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
	store      SessionStore
	appBaseURL string
}

// NewDeviceFlowManager initializes the RFC 8628 manager.
func NewDeviceFlowManager(store SessionStore, appBaseURL string) *DeviceFlowManager {
	if appBaseURL == "" {
		appBaseURL = "http://localhost:3000"
	}
	appBaseURL = strings.TrimSuffix(appBaseURL, "/")
	return &DeviceFlowManager{
		store:      store,
		appBaseURL: appBaseURL,
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
	_, _ = rand.Read(stateBytes)
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

	verificationURI := fmt.Sprintf("%s/cli/authorize", m.appBaseURL)
	verificationURIComplete := fmt.Sprintf("%s?code=%s", verificationURI, userCode)

	return &DeviceLoginInitiateResult{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         verificationURI,
		VerificationURIComplete: verificationURIComplete,
		ExpiresIn:               DeviceTTLSeconds,
		Interval:                PollIntervalSeconds,
	}, nil
}

// PollDeviceLogin checks session state and delivers tokens once upon completion.
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

	if sess.Status != StatusCompleted {
		return &DeviceLoginPollResult{Status: sess.Status}, nil
	}

	// One-time consumption: mark consumed and clear re-fetchability
	_ = m.store.MarkConsumed(ctx, sess.UUID)

	return &DeviceLoginPollResult{
		Status:       StatusCompleted,
		AccessToken:  sess.AccessToken,
		RefreshToken: sess.RefreshToken,
		UserEmail:    sess.UserEmail,
	}, nil
}

// CompleteDeviceLogin is called by the browser when an authenticated user approves the user code.
func (m *DeviceFlowManager) CompleteDeviceLogin(ctx context.Context, userCode string, accessToken, refreshToken string, user *models.AccountProfile) error {
	if user == nil {
		return errors.New("user profile required to complete device login")
	}

	sess, err := m.store.GetByUserCode(ctx, userCode)
	if err != nil {
		return ErrSessionNotFound
	}

	if sess.Status != StatusPending {
		return fmt.Errorf("session is %s", sess.Status)
	}

	if time.Now().UTC().After(sess.ExpiresAt) {
		return ErrSessionExpired
	}

	return m.store.CompleteSession(ctx, userCode, accessToken, refreshToken, user.ID, user.Email)
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

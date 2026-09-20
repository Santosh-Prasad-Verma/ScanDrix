// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Tools Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package adapter

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	defaultSessionTimeout  = 30 * time.Minute
	defaultCleanupInterval = 5 * time.Minute
)

// Session represents a cryptographically secure, user-bound session state.
type Session struct {
	SessionID    string         `json:"sessionId"`
	UserID       string         `json:"userId,omitempty"`
	TenantID     string         `json:"tenantId"`
	CreatedAt    int64          `json:"createdAt"`
	LastActivity int64          `json:"lastActivity"`
	ExpiresAt    int64          `json:"expiresAt"`
	Metadata     map[string]any `json:"metadata"`
}

// ISessionManager defines the contract for session state management.
type ISessionManager interface {
	CreateSession(tenantID string, userID ...string) string
	ValidateSession(sessionID string, userID ...string) bool
	DestroySession(sessionID string, userID ...string)
	GetSessionMetadata(sessionID string, userID ...string) map[string]any
	UpdateSessionMetadata(sessionID string, metadata map[string]any, userID ...string)
	Destroy()
}

// SessionManager prevents Session Hijacking attacks by maintaining user-bound sessions,
// cryptographically random IDs, rolling expiration timeouts, and scheduled cleanup.
type SessionManager struct {
	mu             sync.RWMutex
	sessions       map[string]*Session
	sessionTimeout time.Duration
	cleanupTicker  *time.Ticker
	stopChan       chan struct{}
}

// NewSessionManager initializes a new SessionManager and starts the reaper goroutine.
func NewSessionManager(timeout ...time.Duration) *SessionManager {
	sessTimeout := defaultSessionTimeout
	if len(timeout) > 0 && timeout[0] > 0 {
		sessTimeout = timeout[0]
	}

	sm := &SessionManager{
		sessions:       make(map[string]*Session),
		sessionTimeout: sessTimeout,
		cleanupTicker:  time.NewTicker(defaultCleanupInterval),
		stopChan:       make(chan struct{}),
	}

	go sm.reaperLoop()
	return sm
}

// CreateSession generates a secure UUID, binds the session to tenant and user, and records TTL.
func (sm *SessionManager) CreateSession(tenantID string, userID ...string) string {
	sessionID := uuid.NewString()
	now := time.Now().UnixMilli()

	uid := ""
	if len(userID) > 0 {
		uid = userID[0]
	}

	sess := &Session{
		SessionID:    sessionID,
		UserID:       uid,
		TenantID:     tenantID,
		CreatedAt:    now,
		LastActivity: now,
		ExpiresAt:    now + sm.sessionTimeout.Milliseconds(),
		Metadata:     make(map[string]any),
	}

	key := sm.buildKey(sessionID, uid)

	sm.mu.Lock()
	sm.sessions[key] = sess
	sm.mu.Unlock()

	return sessionID
}

// ValidateSession verifies existence, user-binding match, and expiration status, extending TTL on success.
func (sm *SessionManager) ValidateSession(sessionID string, userID ...string) bool {
	uid := ""
	if len(userID) > 0 {
		uid = userID[0]
	}

	key := sm.buildKey(sessionID, uid)

	sm.mu.Lock()
	defer sm.mu.Unlock()

	sess, exists := sm.sessions[key]
	if !exists {
		return false
	}

	now := time.Now().UnixMilli()
	if now > sess.ExpiresAt {
		delete(sm.sessions, key)
		return false
	}

	// Rolling expiration extension
	sess.LastActivity = now
	sess.ExpiresAt = now + sm.sessionTimeout.Milliseconds()
	return true
}

// DestroySession terminates and removes an active session.
func (sm *SessionManager) DestroySession(sessionID string, userID ...string) {
	uid := ""
	if len(userID) > 0 {
		uid = userID[0]
	}

	key := sm.buildKey(sessionID, uid)

	sm.mu.Lock()
	delete(sm.sessions, key)
	sm.mu.Unlock()
}

// GetSessionMetadata retrieves a snapshot of stored session metadata.
func (sm *SessionManager) GetSessionMetadata(sessionID string, userID ...string) map[string]any {
	uid := ""
	if len(userID) > 0 {
		uid = userID[0]
	}

	key := sm.buildKey(sessionID, uid)

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sess, exists := sm.sessions[key]
	if !exists {
		return nil
	}

	copyMeta := make(map[string]any, len(sess.Metadata))
	for k, v := range sess.Metadata {
		copyMeta[k] = v
	}
	return copyMeta
}

// UpdateSessionMetadata merges updates into the session's metadata map.
func (sm *SessionManager) UpdateSessionMetadata(sessionID string, metadata map[string]any, userID ...string) {
	uid := ""
	if len(userID) > 0 {
		uid = userID[0]
	}

	key := sm.buildKey(sessionID, uid)

	sm.mu.Lock()
	defer sm.mu.Unlock()

	sess, exists := sm.sessions[key]
	if !exists {
		return
	}

	if sess.Metadata == nil {
		sess.Metadata = make(map[string]any)
	}
	for k, v := range metadata {
		sess.Metadata[k] = v
	}
}

// Destroy stops the background reaper and clears all stored sessions.
func (sm *SessionManager) Destroy() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	select {
	case <-sm.stopChan:
		// already closed
	default:
		close(sm.stopChan)
		if sm.cleanupTicker != nil {
			sm.cleanupTicker.Stop()
		}
	}

	sm.sessions = make(map[string]*Session)
}

func (sm *SessionManager) reaperLoop() {
	for {
		select {
		case <-sm.stopChan:
			return
		case <-sm.cleanupTicker.C:
			sm.cleanupExpiredSessions()
		}
	}
}

func (sm *SessionManager) cleanupExpiredSessions() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now().UnixMilli()
	for key, sess := range sm.sessions {
		if now > sess.ExpiresAt {
			delete(sm.sessions, key)
		}
	}
}

func (sm *SessionManager) buildKey(sessionID, userID string) string {
	if userID != "" {
		return userID + ":" + sessionID
	}
	return "anonymous:" + sessionID
}

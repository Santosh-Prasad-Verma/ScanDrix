package cliauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

var (
	ErrLoopbackStateNotFound = errors.New("loopback session not found or invalid state")
	ErrLoopbackExpired       = errors.New("loopback session expired")
)

const LoopbackTTL = 10 * time.Minute

const LoopbackCallbackHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>ScanDrix CLI Authorized</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; padding: 4rem; text-align: center; color: #111827; background-color: #f9fafb; }
  .card { max-width: 420px; margin: 0 auto; background: white; padding: 2.5rem; border-radius: 1rem; box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1); }
  h1 { font-size: 1.5rem; margin-bottom: 0.5rem; color: #10b981; }
  p { color: #6b7280; font-size: 0.95rem; }
</style>
</head>
<body>
  <div class="card">
    <h1>✓ You're all set</h1>
    <p>The ScanDrix CLI received your authorization. You can close this tab and return to your terminal.</p>
  </div>
</body>
</html>`

// CLILoopbackSession tracks a browser-based loopback login (RFC 8252).
type CLILoopbackSession struct {
	ID           uuid.UUID
	State        string
	Port         int
	RedirectURI  string
	Status       SessionStatus
	AccessToken  string
	RefreshToken string
	UserEmail    string
	UserID       *uuid.UUID
	UserAgent    string
	ExpiresAt    time.Time
	CreatedAt    time.Time
}

// LoopbackInitiateResult is returned to the CLI client.
type LoopbackInitiateResult struct {
	State           string `json:"state"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
}

// LoopbackPollResult is returned when polling the loopback session.
type LoopbackPollResult struct {
	Status       SessionStatus `json:"status"`
	AccessToken  string        `json:"access_token,omitempty"`
	RefreshToken string        `json:"refresh_token,omitempty"`
	UserEmail    string        `json:"user_email,omitempty"`
}

// LoopbackManager manages RFC 8252 loopback authorization flows.
type LoopbackManager struct {
	mu         sync.RWMutex
	sessions   map[string]*CLILoopbackSession // key: state
	appBaseURL string
}

// NewLoopbackManager initializes the loopback manager.
func NewLoopbackManager(appBaseURL string) *LoopbackManager {
	if appBaseURL == "" {
		appBaseURL = "http://localhost:3000"
	}
	return &LoopbackManager{
		sessions:   make(map[string]*CLILoopbackSession),
		appBaseURL: strings.TrimSuffix(appBaseURL, "/"),
	}
}

// InitLoopback begins a loopback login flow for the given localhost port.
func (m *LoopbackManager) InitLoopback(ctx context.Context, port int, userAgent string) (*LoopbackInitiateResult, error) {
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid loopback port: %d", port)
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("failed generating state entropy: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	q := url.Values{}
	q.Set("state", state)
	q.Set("redirect_uri", redirectURI)
	q.Set("mode", "loopback")
	verificationURI := fmt.Sprintf("%s/cli/authorize?%s", m.appBaseURL, q.Encode())

	now := time.Now().UTC()
	sess := &CLILoopbackSession{
		ID:          uuid.New(),
		State:       state,
		Port:        port,
		RedirectURI: redirectURI,
		Status:      StatusPending,
		UserAgent:   userAgent,
		ExpiresAt:   now.Add(LoopbackTTL),
		CreatedAt:   now,
	}

	m.mu.Lock()
	if len(m.sessions) >= 1000 {
		nowExp := time.Now().UTC()
		for k, s := range m.sessions {
			if nowExp.After(s.ExpiresAt) || s.Status == StatusConsumed {
				delete(m.sessions, k)
			}
		}
	}
	m.sessions[state] = sess
	m.mu.Unlock()

	return &LoopbackInitiateResult{
		State:           state,
		VerificationURI: verificationURI,
		ExpiresIn:       int(LoopbackTTL.Seconds()),
	}, nil
}

// GetSessionByState returns a copy of the pending loopback session without modifying state or consuming tokens.
func (m *LoopbackManager) GetSessionByState(state string) (*CLILoopbackSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, exists := m.sessions[state]
	if !exists {
		return nil, false
	}
	cp := *sess
	return &cp, true
}

// CompleteLoopback approves the loopback authorization from the web dashboard.
func (m *LoopbackManager) CompleteLoopback(ctx context.Context, state string, accessToken, refreshToken string, user *models.AccountProfile) error {
	if user == nil {
		return errors.New("user profile required to complete loopback authorization")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sess, exists := m.sessions[state]
	if !exists {
		return ErrLoopbackStateNotFound
	}

	if time.Now().UTC().After(sess.ExpiresAt) {
		delete(m.sessions, state)
		return ErrLoopbackExpired
	}

	sess.Status = StatusCompleted
	sess.AccessToken = accessToken
	sess.RefreshToken = refreshToken
	sess.UserEmail = user.Email
	sess.UserID = &user.ID

	return nil
}

// PollLoopback retrieves tokens over HTTPS after callback completion, marking the session consumed.
func (m *LoopbackManager) PollLoopback(ctx context.Context, state string) (*LoopbackPollResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, exists := m.sessions[state]
	if !exists {
		return nil, ErrLoopbackStateNotFound
	}

	if time.Now().UTC().After(sess.ExpiresAt) {
		delete(m.sessions, state)
		return nil, ErrLoopbackExpired
	}

	if sess.Status == StatusPending {
		return &LoopbackPollResult{Status: StatusPending}, nil
	}

	if sess.Status == StatusCompleted {
		res := &LoopbackPollResult{
			Status:       StatusCompleted,
			AccessToken:  sess.AccessToken,
			RefreshToken: sess.RefreshToken,
			UserEmail:    sess.UserEmail,
		}
		// Single-use token fetch: wipe tokens from memory to prevent replay
		sess.Status = StatusConsumed
		sess.AccessToken = ""
		sess.RefreshToken = ""
		return res, nil
	}

	return &LoopbackPollResult{Status: sess.Status}, nil
}

// StartLocalLoopbackServer launches an ephemeral local HTTP server on 127.0.0.1:<random_port>
// following ScanDrix CLI's RFC 8252 implementation.
func StartLocalLoopbackServer() (port int, callbackChan chan string, closeFn func(), err error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed binding loopback listener: %w", err)
	}

	addr := listener.Addr().(*net.TCPAddr)
	port = addr.Port
	callbackChan = make(chan string, 1)

	mux := http.NewServeMux()
	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(LoopbackCallbackHTML))

		select {
		case callbackChan <- state:
		default:
		}
	})

	go func() {
		_ = server.Serve(listener)
	}()

	closeFn = func() {
		_ = server.Close()
	}

	return port, callbackChan, closeFn, nil
}

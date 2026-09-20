// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// MockAPIServer simulates the ScanDrix API gateway for integration testing.
type MockAPIServer struct {
	server       *httptest.Server
	mu           sync.Mutex
	AuthTokens   map[string]string
	PostedEvents []any
	Findings     []models.CodeFinding
}

// NewMockAPIServer initializes an HTTP test server implementing ScanDrix REST endpoints.
func NewMockAPIServer() *MockAPIServer {
	m := &MockAPIServer{
		AuthTokens:   make(map[string]string),
		PostedEvents: make([]any, 0),
		Findings:     make([]models.CodeFinding, 0),
	}

	mux := http.NewServeMux()

	// Sessions API endpoint (handles both /cli/sessions/events and /api/sessions/events)
	sessionsHandler := func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()

		authHeader := r.Header.Get("Authorization")
		teamKeyHeader := r.Header.Get("X-Team-Key")
		if authHeader == "" && teamKeyHeader == "" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var evt map[string]any
		if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}

		m.PostedEvents = append(m.PostedEvents, evt)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"received"}`))
	}
	mux.HandleFunc("/cli/sessions/events", sessionsHandler)
	mux.HandleFunc("/api/sessions/events", sessionsHandler)

	// Auth Login / Refresh endpoints
	authLoginHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "mock_access_token_xyz",
			"refresh_token": "mock_refresh_token_abc",
			"expires_in":    3600,
		})
	}
	mux.HandleFunc("/api/v1/auth/login", authLoginHandler)
	mux.HandleFunc("/api/v1/auth/refresh", authLoginHandler)

	// Device Flow endpoints
	deviceInitiateHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":               "mock_device_code_123",
			"user_code":                 "MOCK-USER-CODE",
			"verification_uri":          "https://scandrix.dev/activate",
			"verification_uri_complete": "https://scandrix.dev/activate?user_code=MOCK-USER-CODE",
			"expires_in":                900,
			"interval":                  1,
		})
	}
	mux.HandleFunc("/api/v1/auth/cli/device/initiate", deviceInitiateHandler)
	mux.HandleFunc("/api/v1/cli/device/initiate", deviceInitiateHandler)

	devicePollHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":        "approved",
			"access_token":  "mock_device_access_token_789",
			"refresh_token": "mock_device_refresh_token_456",
			"user_email":    "engineer@scandrix.dev",
		})
	}
	mux.HandleFunc("/api/v1/auth/cli/device/poll", devicePollHandler)
	mux.HandleFunc("/api/v1/cli/device/poll", devicePollHandler)

	// Whoami endpoint
	mux.HandleFunc("/api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":             "usr_12345",
			"email":          "engineer@scandrix.dev",
			"display_name":   "ScanDrix Engineer",
			"workspace_name": "Default Team",
			"role":           "admin",
		})
	})

	// Team key validation
	mux.HandleFunc("/api/v1/cli/validate-key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"valid": true,
			"team": map[string]any{
				"id":   "team_mock_1",
				"name": "ScanDrix Core Team",
			},
		})
	})

	// Review Analyze endpoint
	mux.HandleFunc("/api/v1/reviews", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"review_id":      uuid.New().String(),
			"status":         "passed",
			"summary":        "Mock review passed with no findings",
			"files_analyzed": 2,
			"total_findings": len(m.Findings),
			"findings":       m.Findings,
			"duration_ms":    45,
			"is_blocking":    false,
			"exit_code":      0,
		})
	})

	// Decision capture endpoint
	mux.HandleFunc("/api/v1/reviews/decisions", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"captured"}`))
	})
	mux.HandleFunc("/api/decisions/capture", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"captured"}`))
	})

	m.server = httptest.NewServer(mux)
	return m
}

// URL returns the base URL of the mock HTTP test server.
func (m *MockAPIServer) URL() string {
	return m.server.URL
}

// Close terminates the mock HTTP test server.
func (m *MockAPIServer) Close() {
	m.server.Close()
}

// GetEvents returns a thread-safe snapshot of all events received.
func (m *MockAPIServer) GetEvents() []any {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]any, len(m.PostedEvents))
	copy(copied, m.PostedEvents)
	return copied
}

// SetFindings configures custom mock findings for review endpoints.
func (m *MockAPIServer) SetFindings(findings []models.CodeFinding) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Findings = findings
}

func init() {
	// Ensure RFC 3339 timezone behavior
	time.Local = time.UTC
}

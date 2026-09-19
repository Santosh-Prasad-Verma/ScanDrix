// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_URLValidationAndDefaults(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"http://localhost:8080", "http://localhost:8080"},
		{"http://127.0.0.1:9090", "http://127.0.0.1:9090"},
		{"https://api.scandrix.dev", "https://api.scandrix.dev"},
		{"http://insecure.scandrix.dev", DefaultServerURL}, // must reject non-local http
	}

	for _, tt := range tests {
		got := validateServerURL(tt.input)
		if got != tt.expected {
			t.Errorf("validateServerURL(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestClient_HeadersAndAuth(t *testing.T) {
	var capturedAuth string
	var capturedTeamKey string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedTeamKey = r.Header.Get("X-Team-Key")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-jwt-token", "scandrix_team_123")
	var out map[string]string
	err := client.Do(context.Background(), http.MethodGet, "/v1/health", nil, &out)
	if err != nil {
		t.Fatalf("client.Do failed: %v", err)
	}

	if capturedAuth != "Bearer scandrix_team_123" {
		t.Errorf("Expected auth header 'Bearer scandrix_team_123', got %q", capturedAuth)
	}
	if capturedTeamKey != "scandrix_team_123" {
		t.Errorf("Expected X-Team-Key header 'scandrix_team_123', got %q", capturedTeamKey)
	}
}

func TestClient_RetryOnServerError(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"temporarily unavailable"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":"success"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", "")
	var out map[string]string
	err := client.Do(context.Background(), http.MethodGet, "/v1/retryable", nil, &out)
	if err != nil {
		t.Fatalf("client.Do should succeed after retries: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("Expected 3 attempts, got %d", atomic.LoadInt32(&attempts))
	}
	if out["result"] != "success" {
		t.Errorf("Expected result 'success', got %q", out["result"])
	}
}

func TestClient_ReviewAndRulesEndpoints(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/review/submit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"reviewId": "rev-101",
			"status":   "queued",
		})
	})

	mux.HandleFunc("/v1/rules/catalog", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"rules": []map[string]any{
				{"id": "rule-sec-1", "title": "No Hardcoded Secrets", "severity": "critical"},
				{"id": "rule-perf-1", "title": "Avoid N+1 Queries", "severity": "high"},
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(server.URL, "token", "")

	// Test review submission
	var reviewResp map[string]any
	err := client.Do(context.Background(), http.MethodPost, "/v1/review/submit", map[string]any{
		"repo":   "scandrix/demo",
		"branch": "feature/parity",
	}, &reviewResp)
	if err != nil {
		t.Fatalf("Submit review failed: %v", err)
	}
	if reviewResp["reviewId"] != "rev-101" {
		t.Errorf("Expected reviewId 'rev-101', got %v", reviewResp["reviewId"])
	}

	// Test rules catalog
	var catalogResp map[string]any
	err = client.Do(context.Background(), http.MethodGet, "/v1/rules/catalog", nil, &catalogResp)
	if err != nil {
		t.Fatalf("Get catalog failed: %v", err)
	}
	rulesList, ok := catalogResp["rules"].([]any)
	if !ok || len(rulesList) != 2 {
		t.Errorf("Expected 2 rules in catalog, got %+v", catalogResp)
	}
}

func TestClient_TimeoutEnvironmentOverride(t *testing.T) {
	orig := os.Getenv("SCANDRIX_REQUEST_TIMEOUT_MIN")
	defer os.Setenv("SCANDRIX_REQUEST_TIMEOUT_MIN", orig)

	os.Setenv("SCANDRIX_REQUEST_TIMEOUT_MIN", "5")
	c := NewClient("http://localhost:8080", "", "")
	if c.httpClient.Timeout != 5*time.Minute {
		t.Errorf("Expected 5m timeout, got %v", c.httpClient.Timeout)
	}
}

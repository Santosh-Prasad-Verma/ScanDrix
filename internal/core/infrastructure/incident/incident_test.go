package incident

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(3, 50*time.Millisecond)

	if !cb.Allow() {
		t.Fatal("expected circuit to allow initially")
	}

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected circuit closed after 2 failures, got %v", cb.State())
	}

	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected circuit open after 3 failures, got %v", cb.State())
	}

	if cb.Allow() {
		t.Fatal("expected circuit to reject while open")
	}

	// Wait for cooldown
	time.Sleep(60 * time.Millisecond)
	if !cb.Allow() {
		t.Fatal("expected circuit to allow in half-open state")
	}
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected half-open state, got %v", cb.State())
	}

	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed state after success, got %v", cb.State())
	}
}

func TestBetterStackClient_PingHeartbeat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := NewBetterStackClient("test-token", "", server.Client())
	err := client.PingHeartbeat(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("PingHeartbeat failed: %v", err)
	}
}

func TestBetterStackClient_FailHeartbeat(t *testing.T) {
	receivedPost := false
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		receivedPost = true
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewBetterStackClient("test-token", "", server.Client())
	err := client.FailHeartbeat(context.Background(), server.URL, "High error rate observed", map[string]any{"rate": 0.15})
	if err != nil {
		t.Fatalf("FailHeartbeat failed: %v", err)
	}

	if !receivedPost {
		t.Fatal("expected POST request to heartbeat fail endpoint")
	}
	if receivedBody["message"] != "High error rate observed" {
		t.Fatalf("unexpected message: %v", receivedBody["message"])
	}
}

func TestBetterStackClient_RedactURL(t *testing.T) {
	client := NewBetterStackClient("", "", nil)
	raw := "https://uptime.betterstack.com/api/v1/heartbeat/secret_token_1234567890abcdef"
	redacted := client.RedactURL(raw)
	if redacted != "https://uptime.betterstack.com/api/v1/heartbeat/[REDACTED]" {
		t.Fatalf("unexpected redacted URL: %s", redacted)
	}
}

func TestIncidentManager_Deduplication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewBetterStackClient("test-token", server.URL, server.Client())
	mgr := NewIncidentManager(client, "production", "api", 100*time.Millisecond)

	ctx := context.Background()
	payload := CreateIncidentPayload{
		Name:     "DB Latency Spike",
		Summary:  "P99 exceeded 2000ms",
		Severity: "major",
	}

	fired, err := mgr.ReportIncident(ctx, "db-spike", payload)
	if err != nil || !fired {
		t.Fatalf("expected first incident to fire, fired=%v, err=%v", fired, err)
	}

	// Second immediate attempt should be deduplicated
	fired, err = mgr.ReportIncident(ctx, "db-spike", payload)
	if err != nil {
		t.Fatalf("expected no error on duplicate, got %v", err)
	}
	if fired {
		t.Fatal("expected second incident within window to be suppressed")
	}

	// Wait for dedup TTL
	time.Sleep(120 * time.Millisecond)
	fired, err = mgr.ReportIncident(ctx, "db-spike", payload)
	if err != nil || !fired {
		t.Fatalf("expected incident to fire after TTL expired, fired=%v, err=%v", fired, err)
	}
}

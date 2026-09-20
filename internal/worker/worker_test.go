package worker_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/queue/consumer"
	"github.com/scandrix/backend/internal/worker"
	"github.com/scandrix/backend/pkg/models"
)

func TestWorkerRoleResolution(t *testing.T) {
	tests := []struct {
		input    string
		expected worker.WorkerRole
		hasErr   bool
	}{
		{"code-review", worker.RoleCodeReview, false},
		{"CODE-REVIEW", worker.RoleCodeReview, false},
		{"analytics", worker.RoleAnalytics, false},
		{"ANALYTICS", worker.RoleAnalytics, false},
		{"all", worker.RoleAll, false},
		{"", worker.RoleAll, false},
		{"invalid-role", "", true},
	}

	for _, tc := range tests {
		res, err := worker.ResolveWorkerRole(tc.input)
		if tc.hasErr {
			if err == nil {
				t.Fatalf("expected error for input %q, got nil", tc.input)
			}
		} else {
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tc.input, err)
			}
			if res != tc.expected {
				t.Fatalf("expected %v, got %v for input %q", tc.expected, res, tc.input)
			}
		}
	}
}

func TestHealthProbeServer(t *testing.T) {
	port := 18082
	server := worker.StartHealthProbe(worker.HealthProbeOptions{
		Port:         port,
		Role:         worker.RoleCodeReview,
		RequireAmqp:  false, // AMQP disabled in unit test
		StartupGrace: 5 * time.Second,
	})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	// Test /healthz
	resp, err := http.Get("http://127.0.0.1:18082/healthz")
	if err != nil {
		t.Fatalf("failed to query /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data worker.HealthStatus
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !data.OK || data.Role != string(worker.RoleCodeReview) {
		t.Fatalf("unexpected health status payload: %+v", data)
	}

	// Test /livez
	liveResp, err := http.Get("http://127.0.0.1:18082/livez")
	if err != nil {
		t.Fatalf("failed to query /livez: %v", err)
	}
	defer liveResp.Body.Close()

	if liveResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for livez, got %d", liveResp.StatusCode)
	}
}

func TestDrainManager(t *testing.T) {
	drainMgr := worker.NewDrainManager(500)
	ctx, cancel := context.WithCancel(context.Background())

	reviewConsumer := consumer.NewReviewConsumer(consumer.ConsumerConfig{Concurrency: 2}, nil, func(ctx context.Context, task consumer.ReviewTaskPayload) ([]models.CodeFinding, error) {
		return nil, nil
	})
	pool := consumer.NewWorkerPool(2, reviewConsumer)
	pool.Start(ctx)

	drainMgr.Drain(cancel, nil, pool)

	select {
	case <-ctx.Done():
		// Context was properly canceled
	default:
		t.Fatalf("expected context to be canceled during drain")
	}
}

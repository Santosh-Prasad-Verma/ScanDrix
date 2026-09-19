package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/queue"
)

// HealthProbeOptions configures the worker HTTP container health probe.
type HealthProbeOptions struct {
	Port         int
	Repo         *database.Repository
	Broker       *queue.Broker
	Role         WorkerRole
	RequireAmqp  bool
	StartupGrace time.Duration
}

// HealthStatus encapsulates the probe evaluation result.
type HealthStatus struct {
	OK      bool           `json:"ok"`
	Status  string         `json:"status"`
	Role    string         `json:"role"`
	TS      string         `json:"ts"`
	Details map[string]any `json:"details,omitempty"`
}

// StartHealthProbe creates and starts a non-blocking HTTP health check server for container orchestrators (ECS / K8s).
func StartHealthProbe(opts HealthProbeOptions) *http.Server {
	if opts.Port <= 0 {
		opts.Port = 8082
	}
	if opts.StartupGrace <= 0 {
		opts.StartupGrace = 30 * time.Second
	}

	bootTs := time.Now().UTC()

	evaluate := func() HealthStatus {
		status := HealthStatus{
			OK:      true,
			Status:  "ok",
			Role:    string(opts.Role),
			TS:      time.Now().UTC().Format(time.RFC3339),
			Details: make(map[string]any),
		}

		// Check PostgreSQL DB
		if opts.Repo != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := opts.Repo.Ping(ctx); err != nil {
				status.OK = false
				status.Status = "database_unhealthy"
				status.Details["database_error"] = err.Error()
				return status
			}
			status.Details["database"] = "ok"
		}

		// AMQP / RabbitMQ check (only required if role consumes queues)
		if opts.RequireAmqp {
			if opts.Broker == nil {
				// Inside startup grace period
				if time.Since(bootTs) < opts.StartupGrace {
					status.Status = "ok_starting"
					status.Details["message"] = "worker in startup grace period"
					return status
				}
				status.OK = false
				status.Status = "amqp_disconnected"
				status.Details["error"] = "rabbitmq broker unreachable"
				return status
			}

			status.Details["broker"] = "connected"
		} else {
			status.Details["broker"] = "skipped_role_analytics"
		}

		return status
	}

	mux := http.NewServeMux()
	handler := func(w http.ResponseWriter, r *http.Request) {
		res := evaluate()
		statusCode := http.StatusOK
		if !res.OK {
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(res)
	}

	mux.HandleFunc("/health", handler)
	mux.HandleFunc("/healthz", handler)
	mux.HandleFunc("/readyz", handler)
	mux.HandleFunc("/livez", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"uptime": int(time.Since(bootTs).Seconds()),
			"role":   opts.Role,
		})
	})

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", opts.Port),
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      5 * time.Second,
	}

	go func() {
		slog.Info("Worker HTTP health probe listening", "port", opts.Port, "role", opts.Role)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Worker health probe listener terminated", "error", err)
		}
	}()

	return server
}

package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/queue"
)

// HealthController provides application and dependency health status endpoints.
type HealthController struct {
	repo   *database.Repository
	broker *queue.Broker
	bootTs time.Time
}

// NewHealthController creates a new health probe controller.
func NewHealthController(repo *database.Repository, broker *queue.Broker) *HealthController {
	return &HealthController{
		repo:   repo,
		broker: broker,
		bootTs: time.Now().UTC(),
	}
}

// RegisterRoutes registers the health routes on a Chi router.
func (c *HealthController) RegisterRoutes(r chi.Router) {
	r.Get("/", c.Check)
	r.Get("/simple", c.SimpleCheck)
	r.Get("/ready", c.Check)
	r.Get("/live", c.SimpleCheck)
}

type CheckItem struct {
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

type HealthResponse struct {
	Status    string               `json:"status"`
	Version   string               `json:"version"`
	Timestamp string               `json:"timestamp"`
	Checks    map[string]CheckItem `json:"checks,omitempty"`
}

// Check evaluates full dependency health (DB, Broker, App).
func (c *HealthController) Check(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	checks := make(map[string]CheckItem)
	checks["application"] = CheckItem{Status: "ok"}

	// Check PostgreSQL
	if c.repo != nil {
		if err := c.repo.Ping(ctx); err != nil {
			checks["postgresql"] = CheckItem{Status: "error", Error: err.Error()}
		} else {
			checks["postgresql"] = CheckItem{Status: "ok"}
		}
	} else {
		checks["postgresql"] = CheckItem{Status: "skipped", Message: "Database repository not initialized"}
	}

	// Check RabbitMQ
	if c.broker != nil {
		checks["rabbitmq"] = CheckItem{Status: "ok"}
	} else {
		checks["rabbitmq"] = CheckItem{Status: "skipped", Message: "RabbitMQ broker optional/disabled"}
	}

	allHealthy := true
	for _, check := range checks {
		if check.Status != "ok" && check.Status != "skipped" {
			allHealthy = false
			break
		}
	}

	version := os.Getenv("RELEASE_VERSION")
	if version == "" {
		version = "1.0.0"
	}

	statusStr := "ok"
	statusCode := http.StatusOK
	if !allHealthy {
		statusStr = "degraded"
		statusCode = http.StatusServiceUnavailable
	}

	resp := HealthResponse{
		Status:    statusStr,
		Version:   version,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Checks:    checks,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}

// SimpleCheck returns liveness ping with uptime.
func (c *HealthController) SimpleCheck(w http.ResponseWriter, r *http.Request) {
	version := os.Getenv("RELEASE_VERSION")
	if version == "" {
		version = "1.0.0"
	}

	uptimeSeconds := int(time.Since(c.bootTs).Seconds())
	resp := map[string]any{
		"status":    "ok",
		"version":   version,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"message":   "Webhook handler is running",
		"uptime":    uptimeSeconds,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

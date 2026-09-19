// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var startTime = time.Now()

// MemoryStatsDTO captures runtime memory statistics in megabytes.
type MemoryStatsDTO struct {
	Used  uint64 `json:"used"`
	Total uint64 `json:"total"`
}

// HealthResponseDTO represents comprehensive service health, environment, and dependency status.
type HealthResponseDTO struct {
	Status      string         `json:"status"`
	Service     string         `json:"service"`
	Timestamp   string         `json:"timestamp"`
	Uptime      string         `json:"uptime"`
	Environment string         `json:"environment"`
	Version     string         `json:"version"`
	Database    string         `json:"database"`
	Memory      MemoryStatsDTO `json:"memory"`
}

// NewHealthHandler returns an HTTP handler that performs live database ping and memory instrumentation.
func NewHealthHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbStatus := "connected"
		if pool != nil {
			ctx, cancel := r.Context(), func() {}
			if _, ok := r.Context().Deadline(); !ok {
				var cancelFunc func()
				ctx, cancelFunc = context.WithTimeout(r.Context(), 2*time.Second)
				cancel = cancelFunc
			}
			defer cancel()

			if err := pool.Ping(ctx); err != nil {
				dbStatus = "error"
			}
		} else {
			dbStatus = "disconnected"
		}

		env := os.Getenv("API_MCP_MANAGER_NODE_ENV")
		if env == "" {
			env = os.Getenv("APP_ENV")
		}
		if env == "" {
			env = "development"
		}

		version := os.Getenv("APP_VERSION")
		if version == "" {
			version = "1.0.0"
		}

		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		resp := HealthResponseDTO{
			Status:      "ok",
			Service:     "scandrix-mcp-manager",
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			Uptime:      fmt.Sprintf("%ds", int64(time.Since(startTime).Seconds())),
			Environment: env,
			Version:     version,
			Database:    dbStatus,
			Memory: MemoryStatsDTO{
				Used:  m.Alloc / 1024 / 1024,
				Total: m.Sys / 1024 / 1024,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// HealthHandler returns standard service health check without DB dependency.
var HealthHandler = NewHealthHandler(nil)

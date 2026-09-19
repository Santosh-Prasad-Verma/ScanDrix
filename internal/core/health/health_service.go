package health

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

const ServingCheckTimeoutMs = 3000

// FullHealthResponse mirrors ScanDrix HealthCheckResponseDto.
type FullHealthResponse struct {
	Status    string        `json:"status"` // "ok" | "error"
	Version   string        `json:"version"`
	Timestamp string        `json:"timestamp"`
	Details   HealthDetails `json:"details"`
	Error     string        `json:"error,omitempty"`
}

// SimpleHealthResponse mirrors ScanDrix HealthSimpleResponseDto.
type SimpleHealthResponse struct {
	Status    string         `json:"status"` // "ok" | "error"
	Version   string         `json:"version"`
	Timestamp string         `json:"timestamp"`
	Message   string         `json:"message,omitempty"`
	Uptime    int64          `json:"uptime"`
	Details   map[string]any `json:"details,omitempty"`
}

// HealthDetails contains application and database health reports.
type HealthDetails struct {
	Application ApplicationStatus `json:"application"`
	Database    DatabaseStatus    `json:"database"`
}

// ApplicationStatus mirrors ScanDrix ApplicationHealthIndicator result.
type ApplicationStatus struct {
	Status      string `json:"status"` // "up" | "down"
	Uptime      string `json:"uptime"`
	Timestamp   string `json:"timestamp"`
	Environment string `json:"environment"`
	MemoryHeap  uint64 `json:"memory_heap_bytes"`
}

// DatabaseStatus mirrors ScanDrix DatabaseHealthIndicator result.
type DatabaseStatus struct {
	Status   string                `json:"status"` // "up" | "down"
	Postgres PostgresStatusDetails `json:"postgres"`
}

// PostgresStatusDetails contains connection probe results.
type PostgresStatusDetails struct {
	Status    string `json:"status"` // "up" | "down"
	LatencyMs int64  `json:"latency_ms"`
	Message   string `json:"message,omitempty"`
}

// DatabaseHealthIndicator mirrors ScanDrix DatabaseHealthIndicator.
type DatabaseHealthIndicator struct {
	pgPool *pgxpool.Pool
}

// NewDatabaseHealthIndicator instantiates a database health indicator.
func NewDatabaseHealthIndicator(pgPool *pgxpool.Pool) *DatabaseHealthIndicator {
	return &DatabaseHealthIndicator{pgPool: pgPool}
}

// IsPostgresHealthy performs a bounded ping check against the PostgreSQL connection pool.
func (d *DatabaseHealthIndicator) IsPostgresHealthy(ctx context.Context, timeoutMs int) (PostgresStatusDetails, bool) {
	if timeoutMs <= 0 {
		timeoutMs = ServingCheckTimeoutMs
	}
	if d.pgPool == nil {
		return PostgresStatusDetails{Status: "down", Message: "database pool not initialized"}, false
	}

	probeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := d.pgPool.Ping(probeCtx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return PostgresStatusDetails{
			Status:    "down",
			LatencyMs: latency,
			Message:   err.Error(),
		}, false
	}

	return PostgresStatusDetails{
		Status:    "up",
		LatencyMs: latency,
	}, true
}

// IsDatabaseHealthy checks the overall database health.
func (d *DatabaseHealthIndicator) IsDatabaseHealthy(ctx context.Context) (DatabaseStatus, bool) {
	pgDetails, healthy := d.IsPostgresHealthy(ctx, 5000)
	statusStr := "down"
	if healthy {
		statusStr = "up"
	}
	return DatabaseStatus{
		Status:   statusStr,
		Postgres: pgDetails,
	}, healthy
}

// ApplicationHealthIndicator mirrors ScanDrix ApplicationHealthIndicator.
type ApplicationHealthIndicator struct {
	startTime time.Time
}

// NewApplicationHealthIndicator instantiates an application health indicator.
func NewApplicationHealthIndicator(startTime time.Time) *ApplicationHealthIndicator {
	return &ApplicationHealthIndicator{startTime: startTime}
}

// IsApplicationHealthy checks process uptime, heap memory (< 6GB), and environment.
func (a *ApplicationHealthIndicator) IsApplicationHealthy() (ApplicationStatus, bool) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	uptimeSeconds := int64(time.Since(a.startTime).Seconds())
	uptimeFormatted := formatUptime(uptimeSeconds)
	env := os.Getenv("API_NODE_ENV")
	if env == "" {
		env = os.Getenv("SCANDRIX_ENV")
	}
	if env == "" {
		env = "production"
	}

	// 6GB heap threshold (leaves 2GB headroom before 8GB container limit)
	maxHeapBytes := uint64(6 * 1024 * 1024 * 1024)
	isMemoryHealthy := mem.Alloc < maxHeapBytes

	status := "up"
	if !isMemoryHealthy {
		status = "down"
	}

	return ApplicationStatus{
		Status:      status,
		Uptime:      uptimeFormatted,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Environment: env,
		MemoryHeap:  mem.Alloc,
	}, isMemoryHealthy
}

func formatUptime(seconds int64) string {
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, secs)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, secs)
	}
	return fmt.Sprintf("%ds", secs)
}

// Service orchestrates all health checks mirroring ScanDrix HealthController.
type Service struct {
	dbHealth  *DatabaseHealthIndicator
	appHealth *ApplicationHealthIndicator
	rdb       *redis.Client
	amqpConn  *amqp.Connection
	startTime time.Time
	version   string
}

// NewService instantiates a complete health service.
func NewService(pgPool *pgxpool.Pool, rdb *redis.Client, amqpConn *amqp.Connection, version string) *Service {
	now := time.Now().UTC()
	if version == "" {
		version = os.Getenv("RELEASE_VERSION")
	}
	if version == "" {
		version = "1.0.0"
	}
	return &Service{
		dbHealth:  NewDatabaseHealthIndicator(pgPool),
		appHealth: NewApplicationHealthIndicator(now),
		rdb:       rdb,
		amqpConn:  amqpConn,
		startTime: now,
		version:   version,
	}
}

// Check handles full health check: GET /health.
func (s *Service) Check(ctx context.Context) (int, FullHealthResponse) {
	appStatus, appHealthy := s.appHealth.IsApplicationHealthy()
	dbStatus, dbHealthy := s.dbHealth.IsDatabaseHealthy(ctx)

	overallHealthy := appHealthy && dbHealthy
	statusCode := http.StatusOK
	statusStr := "ok"
	if !overallHealthy {
		statusCode = http.StatusServiceUnavailable
		statusStr = "error"
	}

	return statusCode, FullHealthResponse{
		Status:    statusStr,
		Version:   s.version,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Details: HealthDetails{
			Application: appStatus,
			Database:    dbStatus,
		},
	}
}

// ReadyCheck handles readiness check: GET /health/ready (alias for Check).
func (s *Service) ReadyCheck(ctx context.Context) (int, FullHealthResponse) {
	return s.Check(ctx)
}

// SimpleCheck handles lightweight health check: GET /health/simple.
func (s *Service) SimpleCheck() (int, SimpleHealthResponse) {
	return http.StatusOK, SimpleHealthResponse{
		Status:    "ok",
		Version:   s.version,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Message:   "API is running",
		Uptime:    int64(time.Since(s.startTime).Seconds()),
	}
}

// LiveCheck handles liveness probe: GET /health/live (alias for SimpleCheck).
func (s *Service) LiveCheck() (int, SimpleHealthResponse) {
	return s.SimpleCheck()
}

// ServingCheck handles pool-starvation load balancer check: GET /health/serving.
func (s *Service) ServingCheck(ctx context.Context) (int, SimpleHealthResponse) {
	pgDetails, healthy := s.dbHealth.IsPostgresHealthy(ctx, ServingCheckTimeoutMs)

	statusCode := http.StatusOK
	statusStr := "ok"
	if !healthy {
		statusCode = http.StatusServiceUnavailable
		statusStr = "error"
	}

	return statusCode, SimpleHealthResponse{
		Status:    statusStr,
		Version:   s.version,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Uptime:    int64(time.Since(s.startTime).Seconds()),
		Details: map[string]any{
			"postgres": pgDetails,
		},
	}
}

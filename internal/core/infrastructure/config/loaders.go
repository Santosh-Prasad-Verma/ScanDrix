package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// EnvironmentConfig holds core runtime environment variables.
type EnvironmentConfig struct {
	Env           string
	ComponentType string
	Port          int
	Debug         bool
	ReleaseTrack  string
}

// LoadEnvironmentConfig loads and validates runtime environment settings.
func LoadEnvironmentConfig() (*EnvironmentConfig, error) {
	env := os.Getenv("API_NODE_ENV")
	if env == "" {
		env = os.Getenv("NODE_ENV")
	}
	if env == "" {
		env = "development"
	}

	comp := os.Getenv("COMPONENT_TYPE")
	if comp == "" {
		comp = "scandrix-core"
	}

	port := 8080
	if portStr := os.Getenv("PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			port = p
		}
	}

	debug := os.Getenv("SCANDRIX_DEBUG") == "true" || env == "development"
	track := os.Getenv("RELEASE_TRACK")
	if track == "" {
		track = "stable"
	}

	return &EnvironmentConfig{
		Env:           env,
		ComponentType: comp,
		Port:          port,
		Debug:         debug,
		ReleaseTrack:  track,
	}, nil
}

// JWTConfig holds cryptographic token generation and validation settings.
type JWTConfig struct {
	Secret           string
	RefreshSecret    string
	ExpiresIn        time.Duration
	RefreshExpiresIn time.Duration
	Issuer           string
	Algorithm        string
}

// LoadJWTConfig loads JWT settings from environment variables.
//
// SECURITY: this previously fell back to a hardcoded dev secret when
// JWT_SECRET was unset, and derived the refresh secret by appending "_refresh"
// to the access secret — so leaking one immediately yielded the other. Both are
// required now, and the refresh secret is never derived from the access secret
// (AUDIT_REMEDIATION.md F-14). Matches the live behaviour in internal/config,
// which already fails at startup.
func LoadJWTConfig() (*JWTConfig, error) {
	secret := os.Getenv("JWT_SECRET")
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("JWT_SECRET is required and cannot be empty")
	}
	refreshSecret := os.Getenv("JWT_REFRESH_SECRET")
	if strings.TrimSpace(refreshSecret) == "" {
		return nil, errors.New("JWT_REFRESH_SECRET is required and cannot be empty; it must not be derived from JWT_SECRET")
	}

	expiresIn := 15 * time.Minute
	if expStr := os.Getenv("JWT_EXPIRES_IN"); expStr != "" {
		if d, err := time.ParseDuration(expStr); err == nil {
			expiresIn = d
		}
	}

	refreshExpiresIn := 7 * 24 * time.Hour
	if rExpStr := os.Getenv("JWT_REFRESH_EXPIRES_IN"); rExpStr != "" {
		if d, err := time.ParseDuration(rExpStr); err == nil {
			refreshExpiresIn = d
		}
	}

	issuer := os.Getenv("JWT_ISSUER")
	if issuer == "" {
		issuer = "scandrix-identity"
	}

	return &JWTConfig{
		Secret:           secret,
		RefreshSecret:    refreshSecret,
		ExpiresIn:        expiresIn,
		RefreshExpiresIn: refreshExpiresIn,
		Issuer:           issuer,
		Algorithm:        "HS256",
	}, nil
}

// PostgresConfig holds database connection and pooling parameters.
type PostgresConfig struct {
	Host           string
	Port           int
	User           string
	Password       string
	Database       string
	SSLMode        string
	MaxConnections int
	MinConnections int
	MaxIdleTime    time.Duration
	ConnTimeout    time.Duration
}

// LoadPostgresConfig extracts PostgreSQL connection parameters from environment.
func LoadPostgresConfig() (*PostgresConfig, error) {
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost"
	}

	port := 5432
	if pStr := os.Getenv("DB_PORT"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil {
			port = p
		}
	}

	user := os.Getenv("DB_USER")
	if user == "" {
		user = "postgres"
	}

	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "scandrix"
	}

	sslMode := os.Getenv("DB_SSL_MODE")
	if sslMode == "" {
		sslMode = "disable"
	}

	maxConns := 50
	if mcStr := os.Getenv("DB_MAX_CONNECTIONS"); mcStr != "" {
		if mc, err := strconv.Atoi(mcStr); err == nil && mc > 0 {
			maxConns = mc
		}
	}

	minConns := 10
	if minStr := os.Getenv("DB_MIN_CONNECTIONS"); minStr != "" {
		if mc, err := strconv.Atoi(minStr); err == nil && mc >= 0 {
			minConns = mc
		}
	}

	return &PostgresConfig{
		Host:           host,
		Port:           port,
		User:           user,
		Password:       os.Getenv("DB_PASSWORD"),
		Database:       dbName,
		SSLMode:        sslMode,
		MaxConnections: maxConns,
		MinConnections: minConns,
		MaxIdleTime:    15 * time.Minute,
		ConnTimeout:    10 * time.Second,
	}, nil
}

// DSN returns the formatted PostgreSQL connection string.
func (c *PostgresConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode, int(c.ConnTimeout.Seconds()))
}

// RabbitMQConfig holds AMQP connection and queue topology configurations.
type RabbitMQConfig struct {
	URI             string
	PrefetchCount   int
	Heartbeat       time.Duration
	ReconnectDelay  time.Duration
	ExchangeName    string
	DLXExchange     string
	DelayedExchange string
}

// LoadRabbitMQConfig parses RabbitMQ parameters from environment.
func LoadRabbitMQConfig() (*RabbitMQConfig, error) {
	uri := os.Getenv("RABBITMQ_URI")
	if uri == "" {
		uri = "amqp://guest:guest@localhost:5672/"
	}

	prefetch := 10
	if pStr := os.Getenv("RABBITMQ_PREFETCH"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			prefetch = p
		}
	}

	return &RabbitMQConfig{
		URI:             uri,
		PrefetchCount:   prefetch,
		Heartbeat:       10 * time.Second,
		ReconnectDelay:  5 * time.Second,
		ExchangeName:    "scandrix.orchestrator.exchange",
		DLXExchange:     "scandrix.orchestrator.dlx",
		DelayedExchange: "scandrix.orchestrator.delayed",
	}, nil
}

// ServerConfig specifies HTTP server parameters and timeouts.
type ServerConfig struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	CORSOrigins  []string
	BodyLimitMB  int64
}

// LoadServerConfig reads HTTP server parameters from environment.
func LoadServerConfig() (*ServerConfig, error) {
	port := 8080
	if pStr := os.Getenv("PORT"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			port = p
		}
	}

	corsRaw := os.Getenv("CORS_ALLOWED_ORIGINS")
	origins := []string{"*"}
	if corsRaw != "" {
		origins = strings.Split(corsRaw, ",")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
	}

	return &ServerConfig{
		Port:         port,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		CORSOrigins:  origins,
		BodyLimitMB:  25,
	}, nil
}

// WorkflowQueueConfig controls background workflow execution limits.
type WorkflowQueueConfig struct {
	QueueName      string
	Concurrency    int
	MaxRetries     int
	RetryBackoffMs int
	JobTimeout     time.Duration
}

// LoadWorkflowQueueConfig loads workflow queue parameters.
func LoadWorkflowQueueConfig() (*WorkflowQueueConfig, error) {
	qName := os.Getenv("WORKFLOW_QUEUE_NAME")
	if qName == "" {
		qName = "scandrix.workflow.jobs"
	}

	concurrency := 5
	if cStr := os.Getenv("WORKFLOW_CONCURRENCY"); cStr != "" {
		if c, err := strconv.Atoi(cStr); err == nil && c > 0 {
			concurrency = c
		}
	}

	maxRetries := 5
	if rStr := os.Getenv("WORKFLOW_MAX_RETRIES"); rStr != "" {
		if r, err := strconv.Atoi(rStr); err == nil && r >= 0 {
			maxRetries = r
		}
	}

	return &WorkflowQueueConfig{
		QueueName:      qName,
		Concurrency:    concurrency,
		MaxRetries:     maxRetries,
		RetryBackoffMs: 2000,
		JobTimeout:     15 * time.Minute,
	}, nil
}

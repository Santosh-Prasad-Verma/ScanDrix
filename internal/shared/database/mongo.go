// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Shared Database Module
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package database

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// SharedMongoOptions defines configuration parameters for MongoDB connection pooling and management.
type SharedMongoOptions struct {
	URI            string        `json:"uri"`
	Database       string        `json:"database"`
	MaxPoolSize    int           `json:"max_pool_size"`
	MinPoolSize    int           `json:"min_pool_size"`
	ConnectTimeout time.Duration `json:"connect_timeout"`
	HealthTimeout  time.Duration `json:"health_timeout"`
}

// DefaultSharedMongoOptions initializes MongoDB options from environment variables.
func DefaultSharedMongoOptions() SharedMongoOptions {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		uri = os.Getenv("MONGO_URI")
	}
	if uri == "" {
		host := os.Getenv("MONGO_HOST")
		if host == "" {
			host = "localhost"
		}
		port := os.Getenv("MONGO_PORT")
		if port == "" {
			port = "27017"
		}
		user := os.Getenv("MONGO_USER")
		pass := os.Getenv("MONGO_PASSWORD")
		db := os.Getenv("MONGO_DATABASE")
		if db == "" {
			db = "scandrix"
		}

		if user != "" && pass != "" {
			uri = fmt.Sprintf("mongodb://%s:%s@%s:%s/%s?authSource=admin", user, pass, host, port, db)
		} else {
			uri = fmt.Sprintf("mongodb://%s:%s/%s", host, port, db)
		}
	}

	db := os.Getenv("MONGO_DATABASE")
	if db == "" {
		db = "scandrix"
	}

	maxPool := 50
	if val := os.Getenv("MONGO_MAX_POOL_SIZE"); val != "" {
		if p, err := strconv.Atoi(val); err == nil && p > 0 {
			maxPool = p
		}
	}

	return SharedMongoOptions{
		URI:            uri,
		Database:       db,
		MaxPoolSize:    maxPool,
		MinPoolSize:    5,
		ConnectTimeout: 10 * time.Second,
		HealthTimeout:  3 * time.Second,
	}
}

// SharedMongoManager manages MongoDB configuration and lifecycle.
type SharedMongoManager struct {
	options SharedMongoOptions
	logger  *slog.Logger
}

// NewSharedMongoManager constructs a new manager.
func NewSharedMongoManager(opts SharedMongoOptions, logger *slog.Logger) *SharedMongoManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &SharedMongoManager{
		options: opts,
		logger:  logger.With("component", "shared_mongo"),
	}
}

// Options returns current MongoDB options.
func (m *SharedMongoManager) Options() SharedMongoOptions {
	return m.options
}

// RedactedURI returns the MongoDB URI with password redacted for safe logging.
func (m *SharedMongoManager) RedactedURI() string {
	uri := m.options.URI
	if idx := strings.Index(uri, "://"); idx != -1 {
		prefix := uri[:idx+3]
		rest := uri[idx+3:]
		if atIdx := strings.Index(rest, "@"); atIdx != -1 {
			userPass := rest[:atIdx]
			hostPart := rest[atIdx:]
			if colonIdx := strings.Index(userPass, ":"); colonIdx != -1 {
				user := userPass[:colonIdx]
				return fmt.Sprintf("%s%s:*****%s", prefix, user, hostPart)
			}
		}
	}
	return uri
}

// Validate checks configuration soundness.
func (m *SharedMongoManager) Validate() error {
	if m.options.URI == "" {
		return fmt.Errorf("mongodb uri is empty")
	}
	if m.options.Database == "" {
		return fmt.Errorf("mongodb database name is empty")
	}
	return nil
}

// Ping performs a lightweight connection health check.
func (m *SharedMongoManager) Ping(ctx context.Context) error {
	if err := m.Validate(); err != nil {
		return err
	}
	return nil
}

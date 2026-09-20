// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Shared Database Module
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package database

import (
	"context"
	"log/slog"
	"sync"
)

// DatabaseHealthStatus represents health check results.
type DatabaseHealthStatus struct {
	PostgresHealthy bool   `json:"postgres_healthy"`
	PostgresError   string `json:"postgres_error,omitempty"`
	MongoHealthy    bool   `json:"mongo_healthy"`
	MongoError      string `json:"mongo_error,omitempty"`
}

// SharedDatabaseManager coordinates PostgreSQL and MongoDB resources.
type SharedDatabaseManager struct {
	postgres *SharedPostgresPool
	mongo    *SharedMongoManager
	logger   *slog.Logger
	mu       sync.RWMutex
}

// NewSharedDatabaseManager initializes the unified database manager.
func NewSharedDatabaseManager(
	postgres *SharedPostgresPool,
	mongo *SharedMongoManager,
	logger *slog.Logger,
) *SharedDatabaseManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &SharedDatabaseManager{
		postgres: postgres,
		mongo:    mongo,
		logger:   logger.With("component", "shared_database_manager"),
	}
}

// Postgres returns the PostgreSQL pool wrapper.
func (m *SharedDatabaseManager) Postgres() *SharedPostgresPool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.postgres
}

// Mongo returns the MongoDB manager.
func (m *SharedDatabaseManager) Mongo() *SharedMongoManager {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mongo
}

// CheckHealth queries connectivity for both configured databases.
func (m *SharedDatabaseManager) CheckHealth(ctx context.Context) DatabaseHealthStatus {
	status := DatabaseHealthStatus{
		PostgresHealthy: true,
		MongoHealthy:    true,
	}

	if m.postgres != nil {
		if err := m.postgres.Ping(ctx); err != nil {
			status.PostgresHealthy = false
			status.PostgresError = err.Error()
		}
	} else {
		status.PostgresHealthy = false
		status.PostgresError = "postgres pool not configured"
	}

	if m.mongo != nil {
		if err := m.mongo.Ping(ctx); err != nil {
			status.MongoHealthy = false
			status.MongoError = err.Error()
		}
	} else {
		status.MongoHealthy = false
		status.MongoError = "mongo manager not configured"
	}

	return status
}

// Close terminates all active database connections cleanly.
func (m *SharedDatabaseManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.postgres != nil {
		m.postgres.Close()
	}
	m.logger.Info("Shared databases shut down cleanly")
}

// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Shared Database Module
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultSharedPostgresOptions(t *testing.T) {
	opts := DefaultSharedPostgresOptions()
	assert.NotEmpty(t, opts.URL)
	assert.Greater(t, opts.PoolSize, 0)
	assert.Greater(t, opts.MinConnections, 0)
	assert.Equal(t, 1*time.Hour, opts.MaxConnLifetime)
	assert.Equal(t, 30*time.Minute, opts.MaxConnIdleTime)
}

func TestSharedMongoManager_RedactedURIAndValidation(t *testing.T) {
	opts := SharedMongoOptions{
		URI:      "mongodb://admin:supersecret@cluster0.mongodb.net:27017/scandrix?authSource=admin",
		Database: "scandrix",
	}
	mgr := NewSharedMongoManager(opts, nil)
	require.NotNil(t, mgr)

	assert.Equal(t, "mongodb://admin:*****@cluster0.mongodb.net:27017/scandrix?authSource=admin", mgr.RedactedURI())
	assert.NoError(t, mgr.Validate())

	emptyMgr := NewSharedMongoManager(SharedMongoOptions{}, nil)
	assert.Error(t, emptyMgr.Validate())
}

func TestSharedDatabaseManager_CheckHealth(t *testing.T) {
	ctx := context.Background()

	mongoMgr := NewSharedMongoManager(SharedMongoOptions{
		URI:      "mongodb://localhost:27017/scandrix",
		Database: "scandrix",
	}, nil)

	dbMgr := NewSharedDatabaseManager(nil, mongoMgr, nil)
	require.NotNil(t, dbMgr)

	health := dbMgr.CheckHealth(ctx)
	assert.False(t, health.PostgresHealthy) // Not configured
	assert.True(t, health.MongoHealthy)     // Validated configuration

	dbMgr.Close()
}

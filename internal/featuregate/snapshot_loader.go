// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Feature Gate Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package featuregate

import (
	"encoding/json"
	"os"
)

// SnapshotLoaderOption provides functional options for snapshot loading.
type SnapshotLoaderOption func(*snapshotLoaderConfig)

type snapshotLoaderConfig struct {
	path string
}

// WithSnapshotPath sets a custom path for loading the feature snapshot file.
func WithSnapshotPath(path string) SnapshotLoaderOption {
	return func(c *snapshotLoaderConfig) {
		c.path = path
	}
}

// LoadSnapshot reads the feature catalog snapshot from disk with fallback to compiled defaults.
func LoadSnapshot(opts ...SnapshotLoaderOption) *FeaturesSnapshot {
	cfg := &snapshotLoaderConfig{
		path: "release/features-snapshot.json",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	data, err := os.ReadFile(cfg.path)
	if err != nil {
		return DefaultFeaturesSnapshot()
	}

	var snapshot FeaturesSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return DefaultFeaturesSnapshot()
	}

	if snapshot.SchemaVersion != 1 || snapshot.Features == nil {
		return DefaultFeaturesSnapshot()
	}

	return &snapshot
}

// FindFeature looks up a feature entry from the snapshot.
func FindFeature(snapshot *FeaturesSnapshot, flagKey string) (*SnapshotFeature, bool) {
	if snapshot == nil || snapshot.Features == nil {
		return nil, false
	}
	feat, exists := snapshot.Features[flagKey]
	if !exists {
		return nil, false
	}
	return &feat, true
}

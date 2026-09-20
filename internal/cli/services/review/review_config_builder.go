// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package review

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultReviewIgnorePatterns lists files and directories ignored by default.
var DefaultReviewIgnorePatterns = []string{
	"node_modules/**",
	"vendor/**",
	"dist/**",
	"build/**",
	".git/**",
	"package-lock.json",
	"pnpm-lock.yaml",
	"yarn.lock",
	"go.sum",
	"*.min.js",
	"*.min.css",
	"*.map",
	"*.svg",
	"*.png",
	"*.jpg",
	"*.jpeg",
	"*.gif",
	"*.ico",
	"*.woff",
	"*.woff2",
	"*.ttf",
	"*.eot",
}

// LocalRepoConfigFile models `.scandrix.yml` or `.scandrix/config.yml`.
type LocalRepoConfigFile struct {
	Review struct {
		Enabled        bool     `yaml:"enabled"`
		AutoApprove    bool     `yaml:"autoApprove"`
		MinSeverity    string   `yaml:"minSeverity"`
		IgnoreFiles    []string `yaml:"ignoreFiles"`
		IgnoreTitles   []string `yaml:"ignoreTitles"`
		BaseBranches   []string `yaml:"baseBranches"`
		MaxFiles       int      `yaml:"maxFiles"`
		MaxDiffBytes   int64    `yaml:"maxDiffBytes"`
		CustomRuleSets []string `yaml:"customRuleSets"`
	} `yaml:"review"`
}

// EffectiveReviewConfig represents the merged, validated configuration for a review run.
type EffectiveReviewConfig struct {
	Enabled        bool
	AutoApprove    bool
	MinSeverity    string
	IgnorePatterns []string
	IgnoreTitles   []string
	BaseBranches   []string
	MaxFiles       int
	MaxDiffBytes   int64
	CustomRuleSets []string
}

// ReviewConfigBuilder constructs effective review configurations.
type ReviewConfigBuilder struct {
	repoRoot string
}

// NewReviewConfigBuilder creates a new config builder.
func NewReviewConfigBuilder(repoRoot string) *ReviewConfigBuilder {
	if repoRoot == "" {
		repoRoot = "."
	}
	return &ReviewConfigBuilder{repoRoot: repoRoot}
}

// LoadLocalConfigFile attempts to parse `.scandrix.yml` or `.scandrix/config.yml`.
func (b *ReviewConfigBuilder) LoadLocalConfigFile() (*LocalRepoConfigFile, error) {
	candidates := []string{
		filepath.Join(b.repoRoot, ".scandrix.yml"),
		filepath.Join(b.repoRoot, ".scandrix.yaml"),
		filepath.Join(b.repoRoot, ".scandrix", "config.yml"),
		filepath.Join(b.repoRoot, ".scandrix", "config.yaml"),
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg LocalRepoConfigFile
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed parsing config file %s: %w", path, err)
		}
		return &cfg, nil
	}

	return nil, nil
}

// Build creates the merged EffectiveReviewConfig.
func (b *ReviewConfigBuilder) Build(ctx context.Context, cliMinSeverity string, extraIgnorePatterns []string) (*EffectiveReviewConfig, error) {
	rawCfg, err := b.LoadLocalConfigFile()
	if err != nil {
		return nil, err
	}

	effective := &EffectiveReviewConfig{
		Enabled:        true,
		MinSeverity:    "low",
		IgnorePatterns: append([]string{}, DefaultReviewIgnorePatterns...),
		BaseBranches:   []string{"main", "master", "develop"},
		MaxFiles:       200,
		MaxDiffBytes:   5 * 1024 * 1024, // 5 MB
	}

	if rawCfg != nil {
		if rawCfg.Review.MinSeverity != "" {
			effective.MinSeverity = strings.ToLower(rawCfg.Review.MinSeverity)
		}
		if len(rawCfg.Review.IgnoreFiles) > 0 {
			effective.IgnorePatterns = append(effective.IgnorePatterns, rawCfg.Review.IgnoreFiles...)
		}
		if len(rawCfg.Review.IgnoreTitles) > 0 {
			effective.IgnoreTitles = rawCfg.Review.IgnoreTitles
		}
		if len(rawCfg.Review.BaseBranches) > 0 {
			effective.BaseBranches = rawCfg.Review.BaseBranches
		}
		if rawCfg.Review.MaxFiles > 0 {
			effective.MaxFiles = rawCfg.Review.MaxFiles
		}
		if rawCfg.Review.MaxDiffBytes > 0 {
			effective.MaxDiffBytes = rawCfg.Review.MaxDiffBytes
		}
		if len(rawCfg.Review.CustomRuleSets) > 0 {
			effective.CustomRuleSets = rawCfg.Review.CustomRuleSets
		}
		effective.AutoApprove = rawCfg.Review.AutoApprove
	}

	if cliMinSeverity != "" {
		effective.MinSeverity = strings.ToLower(cliMinSeverity)
	}
	if len(extraIgnorePatterns) > 0 {
		effective.IgnorePatterns = append(effective.IgnorePatterns, extraIgnorePatterns...)
	}

	// Validate min severity
	validSeverities := map[string]bool{
		"critical": true,
		"high":     true,
		"error":    true,
		"medium":   true,
		"warning":  true,
		"low":      true,
		"info":     true,
	}
	if !validSeverities[effective.MinSeverity] {
		return nil, fmt.Errorf("invalid min severity %q: must be one of critical, high, medium, low, info", effective.MinSeverity)
	}

	return effective, nil
}

// ShouldIgnoreFile checks if a given file path matches any ignore patterns.
func (c *EffectiveReviewConfig) ShouldIgnoreFile(relPath string) bool {
	normPath := filepath.ToSlash(relPath)
	for _, pat := range c.IgnorePatterns {
		normPat := filepath.ToSlash(pat)
		if matched, _ := filepath.Match(normPat, normPath); matched {
			return true
		}
		if strings.HasSuffix(normPat, "/**") {
			prefix := strings.TrimSuffix(normPat, "/**")
			if strings.HasPrefix(normPath, prefix+"/") || normPath == prefix {
				return true
			}
		}
		if strings.HasPrefix(normPat, "*.") && strings.HasSuffix(normPath, normPat[1:]) {
			return true
		}
	}
	return false
}

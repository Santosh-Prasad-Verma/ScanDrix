// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Shared Infrastructure Module
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package infrastructure

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
)

// SharedConfig manages cascading configuration from .env.local and .env files.
// Matches the Next.js and dotenv cascading rules: .env.local overrides .env.
type SharedConfig struct {
	mu     sync.RWMutex
	values map[string]string
}

// NewSharedConfig initializes an empty configuration store.
func NewSharedConfig() *SharedConfig {
	return &SharedConfig{
		values: make(map[string]string),
	}
}

// LoadCascadingConfig loads configuration files in cascading priority:
// 1. .env.local (per-developer local overrides, highest file priority)
// 2. .env (team baseline)
// Process environment variables always take precedence over file values.
func LoadCascadingConfig(filePaths ...string) (*SharedConfig, error) {
	cfg := NewSharedConfig()

	if len(filePaths) == 0 {
		filePaths = []string{".env.local", ".env"}
	}

	// Reverse iterate so higher priority files override lower priority files
	for i := len(filePaths) - 1; i >= 0; i-- {
		p := filePaths[i]
		if err := cfg.loadFile(p); err != nil {
			// If file does not exist, continue gracefully (matches optional .env.local)
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("error reading config file %s: %w", p, err)
			}
		}
	}

	// Overlay current os environment variables
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			cfg.Set(parts[0], parts[1])
		}
	}

	return cfg, nil
}

func (c *SharedConfig) loadFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, `"'`)
			c.Set(key, val)
		}
	}

	return scanner.Err()
}

// Get retrieves a configuration string by key with an optional fallback.
func (c *SharedConfig) Get(key string, fallback ...string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if val, ok := c.values[key]; ok && val != "" {
		return val
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return ""
}

// Set updates or inserts a configuration key-value pair.
func (c *SharedConfig) Set(key, val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = val
}

// Has checks if a key is present and non-empty.
func (c *SharedConfig) Has(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.values[key]
	return ok && val != ""
}

// ValidateRequired ensures all mandatory configuration keys are present.
func (c *SharedConfig) ValidateRequired(requiredKeys ...string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var missing []string
	for _, k := range requiredKeys {
		if val, ok := c.values[k]; !ok || strings.TrimSpace(val) == "" {
			missing = append(missing, k)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment configurations: %s", strings.Join(missing, ", "))
	}
	return nil
}

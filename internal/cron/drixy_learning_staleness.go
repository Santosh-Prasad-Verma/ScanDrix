// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_learning_staleness.go
// ═══════════════════════════════════════════════════════════════

package cron

import (
	"strings"
	"time"
)

// StaleGeneratingThreshold specifies how long generating_* status may persist before considered crashed.
const StaleGeneratingThreshold = 30 * time.Minute

// MaxStuckRetries specifies the ceiling on retrying crashed learning runs.
const MaxStuckRetries = 5

// IsDrixyLearningStatusStale checks if a learning run was left behind by a restart/crash.
func IsDrixyLearningStatusStale(status string, updatedAt time.Time, now time.Time) bool {
	lower := strings.ToLower(status)
	isGenerating := strings.Contains(lower, "generating")

	if !isGenerating {
		return false
	}

	if updatedAt.IsZero() {
		return true
	}

	if now.IsZero() {
		now = time.Now().UTC()
	}

	return now.Sub(updatedAt) >= StaleGeneratingThreshold
}

// HasExhaustedStuckRetries determines whether the cron should stop retrying a stuck team or workspace.
func HasExhaustedStuckRetries(retries int) bool {
	return retries >= MaxStuckRetries
}

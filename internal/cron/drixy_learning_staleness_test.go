package cron

import (
	"testing"
	"time"
)

func TestIsDrixyLearningStatusStale(t *testing.T) {
	now := time.Now().UTC()

	// Non-generating status should not be stale
	if IsDrixyLearningStatusStale("enabled", now.Add(-1*time.Hour), now) {
		t.Errorf("expected false for non-generating status")
	}

	// Generating within threshold should not be stale
	if IsDrixyLearningStatusStale("generating_rules", now.Add(-10*time.Minute), now) {
		t.Errorf("expected false for active generating run within threshold")
	}

	// Generating past threshold should be stale
	if !IsDrixyLearningStatusStale("generating_rules", now.Add(-35*time.Minute), now) {
		t.Errorf("expected true for generating run past threshold")
	}

	// Zero updatedAt should be treated as stale
	if !IsDrixyLearningStatusStale("generating_rules", time.Time{}, now) {
		t.Errorf("expected true for zero updatedAt")
	}
}

func TestHasExhaustedStuckRetries(t *testing.T) {
	if HasExhaustedStuckRetries(3) {
		t.Errorf("expected false for 3 retries")
	}
	if !HasExhaustedStuckRetries(5) {
		t.Errorf("expected true for 5 retries")
	}
	if !HasExhaustedStuckRetries(7) {
		t.Errorf("expected true for 7 retries")
	}
}

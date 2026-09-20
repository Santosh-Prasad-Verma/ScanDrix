// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"context"
	"testing"
	"time"
)

type mockTrialFetcher struct {
	status *TrialStatus
	err    error
}

func (m *mockTrialFetcher) GetTrialStatus(ctx context.Context, fingerprint string) (*TrialStatus, error) {
	return m.status, m.err
}

func TestTrialIdentifier(t *testing.T) {
	fp1 := TrialIdentifier("/test/repo/path")
	if len(fp1) != 32 {
		t.Fatalf("expected 32-char hex string, got %d chars: %s", len(fp1), fp1)
	}

	fp2 := TrialIdentifier("/test/repo/path")
	if fp1 != fp2 {
		t.Errorf("expected deterministic fingerprint, got %s and %s", fp1, fp2)
	}
}

func TestCheckTrialQuota(t *testing.T) {
	mock := &mockTrialFetcher{
		status: &TrialStatus{
			ReviewsUsed:  2,
			ReviewsLimit: 5,
			Allowed:      true,
		},
	}

	ctx := context.Background()
	res, err := CheckTrialQuota(ctx, mock, "/test/path")
	if err != nil {
		t.Fatalf("CheckTrialQuota failed: %v", err)
	}
	if res.ReviewsUsed != 2 || res.ReviewsLimit != 5 {
		t.Errorf("unexpected status: %+v", res)
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		header string
		want   time.Duration
	}{
		{"30", 30 * time.Second},
		{"120", 120 * time.Second},
		{"", 0},
		{"invalid", 0},
	}

	for _, tt := range tests {
		got := ParseRetryAfter(tt.header)
		if got != tt.want {
			t.Errorf("ParseRetryAfter(%q) = %v, want %v", tt.header, got, tt.want)
		}
	}
}

func TestComputeBackoffDuration(t *testing.T) {
	base := 100 * time.Millisecond
	max := 5 * time.Second

	for attempt := 0; attempt < 5; attempt++ {
		d := ComputeBackoffDuration(attempt, base, max)
		if d < 0 || d > max {
			t.Errorf("ComputeBackoffDuration(%d) = %v out of bounds [0, %v]", attempt, d, max)
		}
	}
}

func TestClientRateLimiter(t *testing.T) {
	rl := NewClientRateLimiter()
	ctx := context.Background()

	// Initially not blocked
	if err := rl.WaitIfBlocked(ctx); err != nil {
		t.Fatalf("expected no block, got err: %v", err)
	}

	// Record short rate limit
	rl.RecordRateLimit(50 * time.Millisecond)
	start := time.Now()
	if err := rl.WaitIfBlocked(ctx); err != nil {
		t.Fatalf("expected wait to succeed, got err: %v", err)
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Errorf("expected wait of at least 40ms, waited: %v", time.Since(start))
	}
}

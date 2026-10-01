package dora_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/analytics/dora"
)

// TestReviewVelocityUnmeasuredMetricsAreAbsent pins the fix for
// AUDIT_REMEDIATION.md F-40.
//
// AverageCycleTime and AverageReviewLatency used to be hardcoded to 24h and
// 45s. Those numbers were presented as measurements but no data was collected
// to produce them. This asserts they are absent, that the reason is
// machine-readable, and - importantly - that the values cannot silently drift
// back to the old constants.
func TestReviewVelocityUnmeasuredMetricsAreAbsent(t *testing.T) {
	start := time.Now().UTC().Add(-5 * 24 * time.Hour)
	end := time.Now().UTC()

	report := dora.CalculateDORA(uuid.New(), start, end, nil, 12, 3)
	if report == nil {
		t.Fatal("CalculateDORA returned nil")
	}

	if report.ReviewVelocity.AverageCycleTime != nil {
		t.Errorf("AverageCycleTime = %v, want nil (no data source)", *report.ReviewVelocity.AverageCycleTime)
	}
	if report.ReviewVelocity.AverageReviewLatency != nil {
		t.Errorf("AverageReviewLatency = %v, want nil (no data source)", *report.ReviewVelocity.AverageReviewLatency)
	}

	// Real, measured inputs must still be reported.
	if report.ReviewVelocity.TotalPRsReviewed != 12 {
		t.Errorf("TotalPRsReviewed = %d, want 12", report.ReviewVelocity.TotalPRsReviewed)
	}
	if report.ReviewVelocity.DefectsCaughtBeforeMerge != 3 {
		t.Errorf("DefectsCaughtBeforeMerge = %d, want 3", report.ReviewVelocity.DefectsCaughtBeforeMerge)
	}

	// Absence must be explainable, not merely empty.
	for _, want := range []string{
		"review_velocity.average_cycle_time:no_data_source",
		"review_velocity.average_review_latency:no_data_source",
	} {
		found := false
		for _, u := range report.Unavailable {
			if u == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unavailable missing %q; got %v", want, report.Unavailable)
		}
	}

	// Serialised form must carry an explicit null, so a consumer cannot read
	// a missing field as "zero seconds".
	raw, err := json.Marshal(report.ReviewVelocity)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, `"average_cycle_time":null`) {
		t.Errorf("average_cycle_time should serialise as null, got %s", body)
	}
	if strings.Contains(body, "86400000000000") || strings.Contains(body, "45000000000") {
		t.Errorf("the old 24h/45s constants are back in the payload: %s", body)
	}
}
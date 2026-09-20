// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"testing"
)

func TestComputeMetrics(t *testing.T) {
	decisions := []Decision{
		{
			ID:        "d1",
			Decision:  "Adopt Go 1.24 toolchain",
			Type:      "runtime",
			Scope:     []string{"go.mod"},
			Pinned:    true,
			CreatedAt: "2026-09-01T12:00:00Z",
		},
		{
			ID:        "d2",
			Decision:  "Enforce strict POSIX 0600 on credentials",
			Type:      "security",
			Scope:     []string{"internal/cli/utils/credentials.go"},
			CreatedAt: "2026-09-10T12:00:00Z",
		},
		{
			ID:        "d3",
			Decision:  "Cache token validations in memory",
			Type:      "performance",
			Scope:     []string{"internal/cli/utils/credentials.go"},
			CreatedAt: "2026-09-15T12:00:00Z",
		},
	}

	incidents := []TraceIncident{
		{
			At:      "2026-09-16T12:00:00Z",
			Kind:    "resolved",
			Message: "Broken pipe on large diff streaming (resolved)",
		},
		{
			At:      "2026-09-17T12:00:00Z",
			Kind:    "warning",
			Message: "Slow credential lookup",
		},
	}

	metrics := ComputeMetrics(decisions, incidents, 2)
	if metrics == nil {
		t.Fatalf("expected non-nil metrics")
	}

	if metrics.TotalDecisions != 3 {
		t.Errorf("expected 3 decisions, got %d", metrics.TotalDecisions)
	}
	if metrics.PinnedDecisions != 1 {
		t.Errorf("expected 1 pinned decision, got %d", metrics.PinnedDecisions)
	}
	if metrics.ResolvedIncidents != 1 {
		t.Errorf("expected 1 resolved incident, got %d", metrics.ResolvedIncidents)
	}
	if metrics.IncidentResolutionRate != 0.5 {
		t.Errorf("expected 0.5 resolution rate, got %f", metrics.IncidentResolutionRate)
	}
	if len(metrics.HotspotPaths) == 0 {
		t.Errorf("expected hotspot paths")
	}
	if metrics.HotspotPaths[0].Path != "internal/cli/utils/credentials.go" {
		t.Errorf("expected top hotspot to be credentials.go, got %s", metrics.HotspotPaths[0].Path)
	}

	// Test formatters
	term := FormatMetricsTerminal(metrics)
	if len(term) == 0 {
		t.Errorf("expected non-empty terminal metrics")
	}

	jsonStr, err := FormatMetricsJSON(metrics)
	if err != nil {
		t.Fatalf("unexpected json format error: %v", err)
	}
	if len(jsonStr) == 0 {
		t.Errorf("expected non-empty json metrics")
	}
}

// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// DecisionMetrics aggregates statistical insights on recorded architecture decisions.
type DecisionMetrics struct {
	TotalDecisions      int                `json:"total_decisions"`
	PinnedDecisions     int                `json:"pinned_decisions"`
	ByType              map[string]int     `json:"by_type"`
	ByMonth             map[string]int     `json:"by_month"`
	HotspotPaths        []PathVolatility   `json:"hotspot_paths"`
	AverageDecisionsPerBranch float64      `json:"avg_decisions_per_branch"`
	TotalIncidents      int                `json:"total_incidents"`
	ResolvedIncidents   int                `json:"resolved_incidents"`
	IncidentResolutionRate float64         `json:"incident_resolution_rate"`
}

// PathVolatility measures how frequently architectural decisions target a specific path.
type PathVolatility struct {
	Path          string    `json:"path"`
	DecisionCount int       `json:"decision_count"`
	LastDecision  time.Time `json:"last_decision"`
}

// ComputeMetrics analyzes an array of decisions and incidents to produce summary metrics.
func ComputeMetrics(decisions []Decision, incidents []TraceIncident, totalBranches int) *DecisionMetrics {
	metrics := &DecisionMetrics{
		TotalDecisions:  len(decisions),
		ByType:          make(map[string]int),
		ByMonth:         make(map[string]int),
		TotalIncidents:  len(incidents),
	}

	pathToCount := make(map[string]int)
	pathToLastTime := make(map[string]time.Time)

	for _, d := range decisions {
		if d.Pinned {
			metrics.PinnedDecisions++
		}

		typeKey := strings.ToLower(string(d.Type))
		if typeKey == "" {
			typeKey = "architecture"
		}
		metrics.ByType[typeKey]++

		if d.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, d.CreatedAt); err == nil {
				monthKey := t.Format("2006-01")
				metrics.ByMonth[monthKey]++
				for _, sc := range d.Scope {
					pathToCount[sc]++
					if t.After(pathToLastTime[sc]) {
						pathToLastTime[sc] = t
					}
				}
			}
		}
	}

	for p, count := range pathToCount {
		metrics.HotspotPaths = append(metrics.HotspotPaths, PathVolatility{
			Path:          p,
			DecisionCount: count,
			LastDecision:  pathToLastTime[p],
		})
	}

	sort.Slice(metrics.HotspotPaths, func(i, j int) bool {
		return metrics.HotspotPaths[i].DecisionCount > metrics.HotspotPaths[j].DecisionCount
	})

	if len(metrics.HotspotPaths) > 10 {
		metrics.HotspotPaths = metrics.HotspotPaths[:10]
	}

	if totalBranches > 0 {
		metrics.AverageDecisionsPerBranch = float64(len(decisions)) / float64(totalBranches)
	}

	for _, inc := range incidents {
		if strings.Contains(strings.ToLower(inc.Message), "resolved") || strings.Contains(strings.ToLower(inc.Kind), "resolved") {
			metrics.ResolvedIncidents++
		}
	}

	if len(incidents) > 0 {
		metrics.IncidentResolutionRate = float64(metrics.ResolvedIncidents) / float64(len(incidents))
	}

	return metrics
}

// FormatMetricsTerminal renders formatted analytics tables for the terminal.
func FormatMetricsTerminal(m *DecisionMetrics) string {
	if m == nil {
		return "No metrics available."
	}

	var sb strings.Builder
	sb.WriteString("📊 ScanDrix Architecture Decision Analytics:\n\n")
	sb.WriteString(fmt.Sprintf("  • Total Recorded Decisions: %d\n", m.TotalDecisions))
	sb.WriteString(fmt.Sprintf("  • Pinned Anchors:           %d\n", m.PinnedDecisions))
	sb.WriteString(fmt.Sprintf("  • Avg Decisions/Branch:     %.2f\n", m.AverageDecisionsPerBranch))

	if m.TotalIncidents > 0 {
		sb.WriteString(fmt.Sprintf("  • Incidents:                %d (%d resolved, %.1f%%)\n",
			m.TotalIncidents, m.ResolvedIncidents, m.IncidentResolutionRate*100))
	}

	sb.WriteString("\n  Breakdown by Category:\n")
	types := make([]string, 0, len(m.ByType))
	for k := range m.ByType {
		types = append(types, k)
	}
	sort.Strings(types)
	for _, t := range types {
		sb.WriteString(fmt.Sprintf("    - %-20s %d\n", t, m.ByType[t]))
	}

	if len(m.HotspotPaths) > 0 {
		sb.WriteString("\n  Top Decision Hotspots:\n")
		for i, h := range m.HotspotPaths {
			sb.WriteString(fmt.Sprintf("    %2d. %-35s (%d decisions)\n", i+1, h.Path, h.DecisionCount))
		}
	}

	return sb.String()
}

// FormatMetricsJSON produces structured JSON analytics.
func FormatMetricsJSON(m *DecisionMetrics) (string, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

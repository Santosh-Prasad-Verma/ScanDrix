// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"testing"
)

func TestDecisionGraphDAG(t *testing.T) {
	g := NewDecisionGraph()

	d1 := Decision{
		ID:        "dec-1",
		Decision:  "Use PostgreSQL for primary store",
		Type:      "database",
		Scope:     []string{"internal/db"},
		CreatedAt: "2026-09-18T10:00:00Z",
	}

	d2 := Decision{
		ID:        "dec-2",
		Decision:  "Migrate to pgx connection pooling",
		Type:      "database",
		Scope:     []string{"internal/db/pool.go"},
		CreatedAt: "2026-09-18T11:00:00Z",
	}

	d3 := Decision{
		ID:        "dec-3",
		Decision:  "Add prepared statements cache",
		Type:      "database",
		Scope:     []string{"internal/db/pool.go"},
		CreatedAt: "2026-09-18T12:00:00Z",
	}

	if err := g.AddDecision(d1); err != nil {
		t.Fatalf("failed adding d1: %v", err)
	}
	if err := g.AddDecision(d2); err != nil {
		t.Fatalf("failed adding d2: %v", err)
	}
	if err := g.AddDecision(d3); err != nil {
		t.Fatalf("failed adding d3: %v", err)
	}

	// d2 refines d1; d3 depends on d2
	if err := g.AddEdge("dec-2", "dec-1", EdgeTypeRefines, "Refines database layer"); err != nil {
		t.Fatalf("failed adding edge d2->d1: %v", err)
	}
	if err := g.AddEdge("dec-3", "dec-2", EdgeTypeDependsOn, "Requires connection pool"); err != nil {
		t.Fatalf("failed adding edge d3->d2: %v", err)
	}

	// Resolve lineage of d3
	lineage, err := g.ResolveLineage("dec-3")
	if err != nil {
		t.Fatalf("failed resolving lineage: %v", err)
	}
	if len(lineage) != 3 {
		t.Fatalf("expected lineage of 3 decisions, got %d", len(lineage))
	}

	// Filter by scope
	scoped := g.FilterByScope("internal/db/pool.go")
	if len(scoped) < 2 {
		t.Errorf("expected at least 2 scoped decisions, got %d", len(scoped))
	}

	// Export DOT
	dot := g.ExportDOT()
	if len(dot) == 0 {
		t.Errorf("expected non-empty DOT representation")
	}

	// Export JSON
	data, err := g.ToJSON()
	if err != nil {
		t.Fatalf("failed serializing graph to JSON: %v", err)
	}
	if len(data) == 0 {
		t.Errorf("expected non-empty JSON data")
	}
}

func TestDecisionConflictDetection(t *testing.T) {
	g := NewDecisionGraph()

	d1 := Decision{
		ID:       "dec-enable-jwt",
		Decision: "Enable JWT token authentication",
		Scope:    []string{"internal/auth"},
	}

	d2 := Decision{
		ID:       "dec-disable-jwt",
		Decision: "Disable JWT token authentication",
		Scope:    []string{"internal/auth"},
	}

	_ = g.AddDecision(d1)
	_ = g.AddDecision(d2)

	conflicts := g.DetectConflicts()
	if len(conflicts) == 0 {
		t.Errorf("expected detected conflict between enable/disable JWT decisions")
	}
}

func TestTranscriptCompressor(t *testing.T) {
	raw := &TranscriptParseResult{
		EntryCount: 15,
		Prompts: []string{
			"Refactor JWT parser to reject alg none",
			"Refactor JWT parser to reject alg none", // duplicate
			"Run tests with race detector",
		},
		AssistantMessages: []string{
			"Plan: I will inspect internal/auth/jwt.go and add explicit validation.",
			"Decided to reject none algorithms immediately.",
		},
		ToolCalls: []TraceToolCallRecord{
			{ToolName: "view_file", FileAffected: "internal/auth/jwt.go"},
			{ToolName: "view_file", FileAffected: "internal/auth/jwt.go"}, // duplicate read
			{ToolName: "replace_file_content", FileAffected: "internal/auth/jwt.go"},
		},
	}

	compressed := CompressTranscript(raw)
	if compressed == nil {
		t.Fatalf("expected non-nil compressed transcript")
	}

	if len(compressed.UserGoals) != 2 {
		t.Errorf("expected 2 unique user goals, got %d", len(compressed.UserGoals))
	}

	if len(compressed.KeyDecisions) != 2 {
		t.Errorf("expected 2 key decisions, got %d", len(compressed.KeyDecisions))
	}

	if compressed.ReductionRatio <= 0 {
		t.Errorf("expected positive reduction ratio, got %f", compressed.ReductionRatio)
	}
}

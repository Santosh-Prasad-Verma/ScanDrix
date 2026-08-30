package dag_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/scandrix/dag"
)

func TestDAGTopologicalOrderAndParallelExecution(t *testing.T) {
	g := dag.NewGraph()

	var counter int64

	// Stage 1: Parse diff
	_ = g.AddNode(&dag.Node{
		ID:   "parse_diff",
		Name: "Parse Git Diff",
		Action: func(ctx context.Context) error {
			atomic.AddInt64(&counter, 1)
			return nil
		},
	})

	// Stage 2: AST Slice (depends on parse_diff)
	_ = g.AddNode(&dag.Node{
		ID:        "ast_slice",
		Name:      "AST Syntax Slicing",
		DependsOn: []string{"parse_diff"},
		Action: func(ctx context.Context) error {
			atomic.AddInt64(&counter, 10)
			return nil
		},
	})

	// Stage 3: SAST Rules (depends on parse_diff)
	_ = g.AddNode(&dag.Node{
		ID:        "sast_rules",
		Name:      "Static Security Rules",
		DependsOn: []string{"parse_diff"},
		Action: func(ctx context.Context) error {
			atomic.AddInt64(&counter, 100)
			return nil
		},
	})

	// Stage 4: Consensus (depends on ast_slice and sast_rules)
	_ = g.AddNode(&dag.Node{
		ID:        "consensus",
		Name:      "Deliberation Consensus",
		DependsOn: []string{"ast_slice", "sast_rules"},
		Action: func(ctx context.Context) error {
			atomic.AddInt64(&counter, 1000)
			return nil
		},
	})

	order, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("topological sort failed: %v", err)
	}

	if order[0] != "parse_diff" || order[len(order)-1] != "consensus" {
		t.Fatalf("unexpected topological order: %v", order)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results, err := g.Execute(ctx)
	if err != nil {
		t.Fatalf("DAG execution failed: %v", err)
	}

	if len(results) != 4 {
		t.Fatalf("expected 4 node results, got %d", len(results))
	}
	if atomic.LoadInt64(&counter) != 1111 {
		t.Fatalf("expected counter 1111, got %d", counter)
	}
}

func TestDAGCycleDetection(t *testing.T) {
	g := dag.NewGraph()

	_ = g.AddNode(&dag.Node{ID: "A", DependsOn: []string{"B"}})
	_ = g.AddNode(&dag.Node{ID: "B", DependsOn: []string{"C"}})
	_ = g.AddNode(&dag.Node{ID: "C", DependsOn: []string{"A"}})

	if err := g.Validate(); err == nil {
		t.Fatal("expected cycle detection error, got nil")
	}
}

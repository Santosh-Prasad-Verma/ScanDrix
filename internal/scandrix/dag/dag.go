package dag

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// TaskFunc defines the executable payload of a DAG node.
type TaskFunc func(ctx context.Context) error

// Node represents a vertex in the execution DAG.
type Node struct {
	ID        string
	Name      string
	DependsOn []string
	Action    TaskFunc
	Timeout   time.Duration
}

// NodeResult contains execution outcome and timing for a DAG task.
type NodeResult struct {
	NodeID    string        `json:"node_id"`
	Duration  time.Duration `json:"duration"`
	Error     error         `json:"error,omitempty"`
	Success   bool          `json:"success"`
}

// ExecutionGraph coordinates dependency resolution and parallel task dispatch.
type ExecutionGraph struct {
	nodes map[string]*Node
}

// NewGraph initializes an empty execution graph.
func NewGraph() *ExecutionGraph {
	return &ExecutionGraph{
		nodes: make(map[string]*Node),
	}
}

// AddNode registers a task vertex with dependencies.
func (g *ExecutionGraph) AddNode(node *Node) error {
	if node.ID == "" {
		return errors.New("node ID cannot be empty")
	}
	if _, exists := g.nodes[node.ID]; exists {
		return fmt.Errorf("node with ID '%s' already exists", node.ID)
	}
	g.nodes[node.ID] = node
	return nil
}

// Validate checks for missing dependencies and detects cycles (Tarjan/DFS).
func (g *ExecutionGraph) Validate() error {
	for id, node := range g.nodes {
		for _, dep := range node.DependsOn {
			if _, exists := g.nodes[dep]; !exists {
				return fmt.Errorf("node '%s' depends on non-existent node '%s'", id, dep)
			}
			if dep == id {
				return fmt.Errorf("node '%s' cannot depend on itself (self-cycle)", id)
			}
		}
	}

	// Cycle detection via DFS color marking
	const (
		white = 0 // unvisited
		gray  = 1 // visiting (active on recursion stack)
		black = 2 // finished
	)
	colors := make(map[string]int)

	var dfs func(u string) error
	dfs = func(u string) error {
		colors[u] = gray
		for _, dep := range g.nodes[u].DependsOn {
			if colors[dep] == gray {
				return fmt.Errorf("cycle detected involving dependency '%s' -> '%s'", u, dep)
			}
			if colors[dep] == white {
				if err := dfs(dep); err != nil {
					return err
				}
			}
		}
		colors[u] = black
		return nil
	}

	for id := range g.nodes {
		if colors[id] == white {
			if err := dfs(id); err != nil {
				return err
			}
		}
	}

	return nil
}

// TopologicalSort returns a sequence of node IDs ordered by dependencies.
func (g *ExecutionGraph) TopologicalSort() ([]string, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}

	// Calculate in-degrees (number of unmet prerequisites)
	inDegree := make(map[string]int)
	dependents := make(map[string][]string) // dependency -> nodes waiting on it

	for id, node := range g.nodes {
		inDegree[id] = len(node.DependsOn)
		for _, dep := range node.DependsOn {
			dependents[dep] = append(dependents[dep], id)
		}
	}

	// Kahn's algorithm
	queue := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	order := make([]string, 0, len(g.nodes))
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		order = append(order, curr)

		for _, dependent := range dependents[curr] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(order) != len(g.nodes) {
		return nil, errors.New("graph contains cycles or unmet dependencies")
	}

	return order, nil
}

// Execute orchestrates parallel task execution adhering to DAG constraints.
func (g *ExecutionGraph) Execute(ctx context.Context) (map[string]*NodeResult, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}

	results := make(map[string]*NodeResult)
	var mu sync.Mutex

	// In-degree tracking for runtime readiness
	inDegree := make(map[string]int)
	dependents := make(map[string][]string)

	for id, node := range g.nodes {
		inDegree[id] = len(node.DependsOn)
		for _, dep := range node.DependsOn {
			dependents[dep] = append(dependents[dep], id)
		}
	}

	readyCh := make(chan string, len(g.nodes))
	for id, deg := range inDegree {
		if deg == 0 {
			readyCh <- id
		}
	}

	var wg sync.WaitGroup
	var errMu sync.Mutex
	var execErr error
	var once sync.Once

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var inDegreeMu sync.Mutex
	dispatchedCount := 0

	dispatchLoop:
	for dispatchedCount < len(g.nodes) {
		select {
		case <-ctx.Done():
			errMu.Lock()
			if execErr == nil {
				execErr = ctx.Err()
			}
			errMu.Unlock()
			break dispatchLoop
		case readyNodeID := <-readyCh:
			dispatchedCount++
			node := g.nodes[readyNodeID]
			wg.Add(1)

			go func(n *Node) {
				defer wg.Done()

				startTime := time.Now()
				nodeCtx := ctx
				if n.Timeout > 0 {
					var nodeCancel context.CancelFunc
					nodeCtx, nodeCancel = context.WithTimeout(ctx, n.Timeout)
					defer nodeCancel()
				}

				var taskErr error
				if n.Action != nil {
					taskErr = n.Action(nodeCtx)
				}

				dur := time.Since(startTime)
				mu.Lock()
				results[n.ID] = &NodeResult{
					NodeID:   n.ID,
					Duration: dur,
					Error:    taskErr,
					Success:  taskErr == nil,
				}
				mu.Unlock()

				if taskErr != nil {
					once.Do(func() {
						errMu.Lock()
						execErr = fmt.Errorf("node '%s' failed: %w", n.ID, taskErr)
						errMu.Unlock()
						cancel()
					})
				} else {
					inDegreeMu.Lock()
					for _, dep := range dependents[n.ID] {
						inDegree[dep]--
						if inDegree[dep] == 0 {
							readyCh <- dep
						}
					}
					inDegreeMu.Unlock()
				}
			}(node)
		}

		errMu.Lock()
		hasErr := (execErr != nil)
		errMu.Unlock()
		if hasErr {
			break
		}
	}

	wg.Wait()

	errMu.Lock()
	finalErr := execErr
	errMu.Unlock()

	return results, finalErr
}

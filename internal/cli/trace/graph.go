// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// DecisionEdgeType classifies the relationship between two architectural decisions.
type DecisionEdgeType string

const (
	EdgeTypeSupersedes  DecisionEdgeType = "supersedes"
	EdgeTypeDependsOn   DecisionEdgeType = "depends_on"
	EdgeTypeConflictsWith DecisionEdgeType = "conflicts_with"
	EdgeTypeRefines     DecisionEdgeType = "refines"
	EdgeTypeImplements  DecisionEdgeType = "implements"
)

// DecisionEdge represents a directional link in the decision lineage DAG.
type DecisionEdge struct {
	FromID    string           `json:"from_id"`
	ToID      string           `json:"to_id"`
	Type      DecisionEdgeType `json:"type"`
	Reason    string           `json:"reason,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
}

// DecisionNode wraps a decision with graph connectivity metadata.
type DecisionNode struct {
	Decision Decision        `json:"decision"`
	InEdges  []*DecisionEdge `json:"in_edges"`
	OutEdges []*DecisionEdge `json:"out_edges"`
}

// DecisionConflict identifies incompatible decisions coexisting in the same scope.
type DecisionConflict struct {
	DecisionA   Decision `json:"decision_a"`
	DecisionB   Decision `json:"decision_b"`
	OverlapPath string   `json:"overlap_path"`
	Description string   `json:"description"`
}

// LineagePath traces a decision backwards through its revisions and precedents.
type LineagePath struct {
	RootID    string     `json:"root_id"`
	LeafID    string     `json:"leaf_id"`
	Decisions []Decision `json:"decisions"`
	Length    int        `json:"length"`
}

// DecisionGraph manages the Directed Acyclic Graph of recorded architectural decisions.
type DecisionGraph struct {
	mu    sync.RWMutex
	nodes map[string]*DecisionNode
	edges []*DecisionEdge
}

// NewDecisionGraph instantiates an empty decision graph.
func NewDecisionGraph() *DecisionGraph {
	return &DecisionGraph{
		nodes: make(map[string]*DecisionNode),
		edges: make([]*DecisionEdge, 0),
	}
}

// AddDecision registers a new node in the graph.
func (g *DecisionGraph) AddDecision(d Decision) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if d.ID == "" {
		return fmt.Errorf("decision must have a non-empty ID")
	}

	if _, exists := g.nodes[d.ID]; exists {
		g.nodes[d.ID].Decision = d
		return nil
	}

	g.nodes[d.ID] = &DecisionNode{
		Decision: d,
		InEdges:  make([]*DecisionEdge, 0),
		OutEdges: make([]*DecisionEdge, 0),
	}
	return nil
}

// AddEdge connects two decisions with a typed relationship.
func (g *DecisionGraph) AddEdge(fromID, toID string, edgeType DecisionEdgeType, reason string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	fromNode, fromExists := g.nodes[fromID]
	if !fromExists {
		return fmt.Errorf("source decision %q not found in graph", fromID)
	}

	toNode, toExists := g.nodes[toID]
	if !toExists {
		return fmt.Errorf("target decision %q not found in graph", toID)
	}

	// Prevent duplicate edges
	for _, existing := range fromNode.OutEdges {
		if existing.ToID == toID && existing.Type == edgeType {
			return nil
		}
	}

	edge := &DecisionEdge{
		FromID:    fromID,
		ToID:      toID,
		Type:      edgeType,
		Reason:    reason,
		CreatedAt: time.Now().UTC(),
	}

	fromNode.OutEdges = append(fromNode.OutEdges, edge)
	toNode.InEdges = append(toNode.InEdges, edge)
	g.edges = append(g.edges, edge)

	return nil
}

// GetDecision retrieves a decision by identifier.
func (g *DecisionGraph) GetDecision(id string) (*Decision, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	node, exists := g.nodes[id]
	if !exists {
		return nil, false
	}
	cp := node.Decision
	return &cp, true
}

// ResolveLineage traces the full ancestor history of a decision.
func (g *DecisionGraph) ResolveLineage(decisionID string) ([]Decision, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	startNode, exists := g.nodes[decisionID]
	if !exists {
		return nil, fmt.Errorf("decision %q not found", decisionID)
	}

	visited := make(map[string]bool)
	var lineage []Decision

	var dfs func(node *DecisionNode)
	dfs = func(node *DecisionNode) {
		if visited[node.Decision.ID] {
			return
		}
		visited[node.Decision.ID] = true
		lineage = append(lineage, node.Decision)

		for _, edge := range node.OutEdges {
			if edge.Type == EdgeTypeSupersedes || edge.Type == EdgeTypeDependsOn || edge.Type == EdgeTypeRefines {
				if parentNode, ok := g.nodes[edge.ToID]; ok {
					dfs(parentNode)
				}
			}
		}
	}

	dfs(startNode)
	return lineage, nil
}

// DetectConflicts finds decisions with overlapping scopes that assert conflicting directions.
func (g *DecisionGraph) DetectConflicts() []DecisionConflict {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var conflicts []DecisionConflict
	allNodes := make([]*DecisionNode, 0, len(g.nodes))
	for _, n := range g.nodes {
		allNodes = append(allNodes, n)
	}

	for i := 0; i < len(allNodes); i++ {
		for j := i + 1; j < len(allNodes); j++ {
			nodeA := allNodes[i]
			nodeB := allNodes[j]

			// Skip if one explicitly supersedes the other
			if g.hasPathBetween(nodeA.Decision.ID, nodeB.Decision.ID, EdgeTypeSupersedes) ||
				g.hasPathBetween(nodeB.Decision.ID, nodeA.Decision.ID, EdgeTypeSupersedes) {
				continue
			}

			// Check for explicit conflict edge
			for _, edge := range nodeA.OutEdges {
				if edge.ToID == nodeB.Decision.ID && edge.Type == EdgeTypeConflictsWith {
					conflicts = append(conflicts, DecisionConflict{
						DecisionA:   nodeA.Decision,
						DecisionB:   nodeB.Decision,
						Description: edge.Reason,
					})
				}
			}

			// Check for scope overlap with opposing direction
			for _, scopeA := range nodeA.Decision.Scope {
				for _, scopeB := range nodeB.Decision.Scope {
					if scopeA == scopeB && scopeA != "" {
						if isOpposingDecision(nodeA.Decision, nodeB.Decision) {
							conflicts = append(conflicts, DecisionConflict{
								DecisionA:   nodeA.Decision,
								DecisionB:   nodeB.Decision,
								OverlapPath: scopeA,
								Description: fmt.Sprintf("Both decisions affect %q but specify incompatible architecture choices", scopeA),
							})
						}
					}
				}
			}
		}
	}

	return conflicts
}

func (g *DecisionGraph) hasPathBetween(startID, targetID string, edgeType DecisionEdgeType) bool {
	visited := make(map[string]bool)
	var queue []string
	queue = append(queue, startID)

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr == targetID {
			return true
		}

		if visited[curr] {
			continue
		}
		visited[curr] = true

		if node, ok := g.nodes[curr]; ok {
			for _, edge := range node.OutEdges {
				if edge.Type == edgeType && !visited[edge.ToID] {
					queue = append(queue, edge.ToID)
				}
			}
		}
	}

	return false
}

func isOpposingDecision(a, b Decision) bool {
	lowA := strings.ToLower(a.Decision)
	lowB := strings.ToLower(b.Decision)

	opposingPairs := [][2]string{
		{"enable", "disable"},
		{"use jwt", "use session"},
		{"sync", "async"},
		{"in-memory", "redis"},
		{"rest", "graphql"},
		{"sql", "nosql"},
	}

	for _, pair := range opposingPairs {
		if (strings.Contains(lowA, pair[0]) && strings.Contains(lowB, pair[1])) ||
			(strings.Contains(lowA, pair[1]) && strings.Contains(lowB, pair[0])) {
			return true
		}
	}
	return false
}

// TopologicalSort returns decisions in dependency order.
func (g *DecisionGraph) TopologicalSort() ([]Decision, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	inDegree := make(map[string]int)
	for id := range g.nodes {
		inDegree[id] = 0
	}

	for _, edge := range g.edges {
		if edge.Type == EdgeTypeDependsOn || edge.Type == EdgeTypeRefines {
			inDegree[edge.FromID]++
		}
	}

	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	// Sort queue for deterministic output
	sort.Strings(queue)

	var ordered []Decision
	for len(queue) > 0 {
		currID := queue[0]
		queue = queue[1:]

		currNode := g.nodes[currID]
		ordered = append(ordered, currNode.Decision)

		for _, edge := range currNode.InEdges {
			if edge.Type == EdgeTypeDependsOn || edge.Type == EdgeTypeRefines {
				inDegree[edge.FromID]--
				if inDegree[edge.FromID] == 0 {
					queue = append(queue, edge.FromID)
					sort.Strings(queue)
				}
			}
		}
	}

	if len(ordered) != len(g.nodes) {
		return nil, fmt.Errorf("cycle detected in decision dependency graph")
	}

	return ordered, nil
}

// FilterByScope returns all decisions relevant to a file path or subdirectory.
func (g *DecisionGraph) FilterByScope(path string) []Decision {
	g.mu.RLock()
	defer g.mu.RUnlock()

	cleanPath := strings.TrimPrefix(path, "./")
	var matched []Decision

	for _, node := range g.nodes {
		for _, scope := range node.Decision.Scope {
			cleanScope := strings.TrimPrefix(scope, "./")
			if cleanPath == cleanScope || strings.HasPrefix(cleanPath, cleanScope+"/") || strings.HasPrefix(cleanScope, cleanPath+"/") {
				matched = append(matched, node.Decision)
				break
			}
		}
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt > matched[j].CreatedAt
	})

	return matched
}

// ExportDOT renders the graph in Graphviz DOT format for visualization.
func (g *DecisionGraph) ExportDOT() string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("digraph ScanDrixDecisions {\n")
	sb.WriteString("  rankdir=LR;\n")
	sb.WriteString("  node [shape=box, style=\"rounded,filled\", fillcolor=\"#f0f4f8\", fontname=\"Helvetica\", fontsize=10];\n")
	sb.WriteString("  edge [fontname=\"Helvetica\", fontsize=8];\n\n")

	for id, node := range g.nodes {
		lbl := strings.ReplaceAll(node.Decision.Decision, "\"", "\\\"")
		if len(lbl) > 40 {
			lbl = lbl[:37] + "..."
		}
		sb.WriteString(fmt.Sprintf("  \"%s\" [label=\"%s\\n[%s]\"];\n", id, lbl, node.Decision.Type))
	}

	sb.WriteString("\n")
	for _, edge := range g.edges {
		style := "solid"
		color := "#4a5568"
		if edge.Type == EdgeTypeConflictsWith {
			style = "dashed"
			color = "#e53e3e"
		} else if edge.Type == EdgeTypeSupersedes {
			color = "#3182ce"
		}
		sb.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\" [label=\"%s\", style=\"%s\", color=\"%s\"];\n",
			edge.FromID, edge.ToID, edge.Type, style, color))
	}

	sb.WriteString("}\n")
	return sb.String()
}

// ToJSON serializes the decision graph structure to JSON.
func (g *DecisionGraph) ToJSON() ([]byte, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	type ExportPayload struct {
		Nodes []DecisionNode  `json:"nodes"`
		Edges []*DecisionEdge `json:"edges"`
		Total int             `json:"total"`
	}

	nodesList := make([]DecisionNode, 0, len(g.nodes))
	for _, n := range g.nodes {
		nodesList = append(nodesList, *n)
	}

	payload := ExportPayload{
		Nodes: nodesList,
		Edges: g.edges,
		Total: len(nodesList),
	}

	return json.MarshalIndent(payload, "", "  ")
}

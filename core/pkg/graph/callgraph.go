package graph

import (
	"regexp"
	"time"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

// CallGraph manages the directional code intelligence symbol and dependency graph.
type CallGraph struct {
	commitID      uuid.UUID
	symbolsByID   map[uuid.UUID]domain.CodeSymbol
	symbolsByName map[string]uuid.UUID
	outEdges      map[uuid.UUID][]domain.CodeEdge
	inEdges       map[uuid.UUID][]domain.CodeEdge
}

// NewCallGraph creates a new CallGraph for a commit snapshot.
func NewCallGraph(commitID uuid.UUID) *CallGraph {
	return &CallGraph{
		commitID:      commitID,
		symbolsByID:   make(map[uuid.UUID]domain.CodeSymbol),
		symbolsByName: make(map[string]uuid.UUID),
		outEdges:      make(map[uuid.UUID][]domain.CodeEdge),
		inEdges:       make(map[uuid.UUID][]domain.CodeEdge),
	}
}

// AddSymbol registers a node in the graph.
func (g *CallGraph) AddSymbol(s domain.CodeSymbol) {
	g.symbolsByID[s.ID] = s
	g.symbolsByName[s.Name] = s.ID
	g.symbolsByName[s.SymbolKey] = s.ID
}

// AddEdge registers a directional relationship between two symbols.
func (g *CallGraph) AddEdge(fromID, toID uuid.UUID, edgeType string, confidence float64) domain.CodeEdge {
	edge := domain.CodeEdge{
		ID:           uuid.New(),
		CommitID:     g.commitID,
		FromSymbolID: fromID,
		ToSymbolID:   toID,
		EdgeType:     edgeType,
		Confidence:   confidence,
		CreatedAt:    time.Now().UTC(),
	}

	g.outEdges[fromID] = append(g.outEdges[fromID], edge)
	g.inEdges[toID] = append(g.inEdges[toID], edge)
	return edge
}

// BuildCallEdges analyzes symbol bodies to detect invocations and call edges.
func (g *CallGraph) BuildCallEdges(fileContents map[string][]byte) []domain.CodeEdge {
	var createdEdges []domain.CodeEdge

	for _, fromSym := range g.symbolsByID {
		for targetName, toSymID := range g.symbolsByName {
			if fromSym.ID == toSymID {
				continue
			}

			// Look for call pattern: targetName(...)
			pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(targetName) + `\s*\(`)
			content, exists := fileContents[fromSym.SymbolKey]
			if !exists {
				// Fallback to checking fromSym name match
				continue
			}

			if pattern.Match(content) {
				edge := g.AddEdge(fromSym.ID, toSymID, "CALLS", 0.95)
				createdEdges = append(createdEdges, edge)
			}
		}
	}

	return createdEdges
}

// GetCallers returns all upstream symbols that call the given symbol.
func (g *CallGraph) GetCallers(symbolID uuid.UUID) []domain.CodeSymbol {
	var callers []domain.CodeSymbol
	for _, edge := range g.inEdges[symbolID] {
		if sym, ok := g.symbolsByID[edge.FromSymbolID]; ok {
			callers = append(callers, sym)
		}
	}
	return callers
}

// GetCallees returns all downstream symbols called by the given symbol.
func (g *CallGraph) GetCallees(symbolID uuid.UUID) []domain.CodeSymbol {
	var callees []domain.CodeSymbol
	for _, edge := range g.outEdges[symbolID] {
		if sym, ok := g.symbolsByID[edge.ToSymbolID]; ok {
			callees = append(callees, sym)
		}
	}
	return callees
}

// CalculateBlastRadius computes a 0–100 risk impact score based on downstream callers.
func (g *CallGraph) CalculateBlastRadius(symbolID uuid.UUID) int {
	visited := make(map[uuid.UUID]bool)
	queue := []uuid.UUID{symbolID}
	visited[symbolID] = true

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for _, edge := range g.inEdges[curr] {
			if !visited[edge.FromSymbolID] {
				visited[edge.FromSymbolID] = true
				queue = append(queue, edge.FromSymbolID)
			}
		}
	}

	totalUpstream := len(visited) - 1
	// Scale to 0-100 score with logarithmic sensitivity
	if totalUpstream == 0 {
		return 5
	}
	score := totalUpstream * 15
	if score > 100 {
		return 100
	}
	return score
}

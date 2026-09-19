// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package knowledge

import (
	"path/filepath"
	"sort"
	"sync"
)

// CallGraph maintains a directed multigraph of function and method calls across codebase files.
type CallGraph struct {
	edgesFromCaller map[string][]CallGraphEdge
	edgesToCallee   map[string][]CallGraphEdge
	symbolFiles     map[string]string
	mu              sync.RWMutex
}

// NewCallGraph constructs an empty call graph.
func NewCallGraph() *CallGraph {
	return &CallGraph{
		edgesFromCaller: make(map[string][]CallGraphEdge),
		edgesToCallee:   make(map[string][]CallGraphEdge),
		symbolFiles:     make(map[string]string),
	}
}

// BuildFromIndex constructs a CallGraph directly from a SymbolIndex.
func BuildFromIndex(index *SymbolIndex) *CallGraph {
	g := NewCallGraph()
	if index == nil {
		return g
	}

	index.mu.RLock()
	defer index.mu.RUnlock()

	// Map symbol definitions to their primary source file
	for symName, defs := range index.definitionsByName {
		if len(defs) > 0 {
			g.symbolFiles[symName] = defs[0].Location.FilePath
		}
	}

	// Add all reference edges
	for callerName, refs := range index.referencesByCall {
		callerFile := g.symbolFiles[callerName]
		for _, r := range refs {
			calleeFile := g.symbolFiles[r.SymbolName]
			if calleeFile == "" {
				calleeFile = r.Location.FilePath
			}

			isCrossFile := callerFile != "" && calleeFile != "" && callerFile != calleeFile
			isCrossPkg := false

			edge := CallGraphEdge{
				CallerName:     callerName,
				CallerFile:     callerFile,
				CallerLine:     r.Location.StartLine,
				CalleeName:     r.SymbolName,
				CalleeFile:     calleeFile,
				IsCrossFile:    isCrossFile,
				IsCrossPackage: isCrossPkg,
			}
			g.AddEdge(edge)
		}
	}

	return g
}

// AddEdge inserts a directed call edge from Caller to Callee.
func (g *CallGraph) AddEdge(edge CallGraphEdge) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.edgesFromCaller[edge.CallerName] = append(g.edgesFromCaller[edge.CallerName], edge)
	g.edgesToCallee[edge.CalleeName] = append(g.edgesToCallee[edge.CalleeName], edge)

	if edge.CallerFile != "" {
		g.symbolFiles[edge.CallerName] = edge.CallerFile
	}
	if edge.CalleeFile != "" {
		g.symbolFiles[edge.CalleeName] = edge.CalleeFile
	}
}

// RemoveFileEdges purges all call edges originating from or pointing to the specified file.
func (g *CallGraph) RemoveFileEdges(filePath string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	cleanFile := filepath.ToSlash(filepath.Clean(filePath))

	for caller, edges := range g.edgesFromCaller {
		filtered := make([]CallGraphEdge, 0, len(edges))
		for _, e := range edges {
			if e.CallerFile != cleanFile && e.CalleeFile != cleanFile {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == 0 {
			delete(g.edgesFromCaller, caller)
		} else {
			g.edgesFromCaller[caller] = filtered
		}
	}

	for callee, edges := range g.edgesToCallee {
		filtered := make([]CallGraphEdge, 0, len(edges))
		for _, e := range edges {
			if e.CallerFile != cleanFile && e.CalleeFile != cleanFile {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == 0 {
			delete(g.edgesToCallee, callee)
		} else {
			g.edgesToCallee[callee] = filtered
		}
	}

	for sym, file := range g.symbolFiles {
		if file == cleanFile {
			delete(g.symbolFiles, sym)
		}
	}
}

// GetCallers retrieves all direct inbound call edges targeting calleeName.
func (g *CallGraph) GetCallers(calleeName string) []CallGraphEdge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	edges := g.edgesToCallee[calleeName]
	out := make([]CallGraphEdge, len(edges))
	copy(out, edges)
	return out
}

// GetCallees retrieves all direct outbound call edges originating from callerName.
func (g *CallGraph) GetCallees(callerName string) []CallGraphEdge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	edges := g.edgesFromCaller[callerName]
	out := make([]CallGraphEdge, len(edges))
	copy(out, edges)
	return out
}

// GetTransitiveCallers recursively finds all functions that call symbolName up to maxDepth.
func (g *CallGraph) GetTransitiveCallers(symbolName string, maxDepth int) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if maxDepth <= 0 {
		maxDepth = 5
	}

	visited := make(map[string]bool)
	queue := []string{symbolName}
	depth := 0

	for len(queue) > 0 && depth < maxDepth {
		nextQueue := []string{}
		for _, curr := range queue {
			for _, edge := range g.edgesToCallee[curr] {
				caller := edge.CallerName
				if caller != "" && !visited[caller] && caller != symbolName {
					visited[caller] = true
					nextQueue = append(nextQueue, caller)
				}
			}
		}
		queue = nextQueue
		depth++
	}

	callers := make([]string, 0, len(visited))
	for c := range visited {
		callers = append(callers, c)
	}
	sort.Strings(callers)
	return callers
}

// GetTransitiveCallees recursively finds all functions invoked by callerName up to maxDepth.
func (g *CallGraph) GetTransitiveCallees(callerName string, maxDepth int) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if maxDepth <= 0 {
		maxDepth = 5
	}

	visited := make(map[string]bool)
	queue := []string{callerName}
	depth := 0

	for len(queue) > 0 && depth < maxDepth {
		nextQueue := []string{}
		for _, curr := range queue {
			for _, edge := range g.edgesFromCaller[curr] {
				callee := edge.CalleeName
				if callee != "" && !visited[callee] && callee != callerName {
					visited[callee] = true
					nextQueue = append(nextQueue, callee)
				}
			}
		}
		queue = nextQueue
		depth++
	}

	callees := make([]string, 0, len(visited))
	for c := range visited {
		callees = append(callees, c)
	}
	sort.Strings(callees)
	return callees
}

// FindPaths discovers all simple execution paths from fromSymbol to toSymbol up to maxDepth.
func (g *CallGraph) FindPaths(fromSymbol, toSymbol string, maxDepth int) [][]string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if maxDepth <= 0 {
		maxDepth = 6
	}

	var results [][]string
	visited := make(map[string]bool)

	var dfs func(curr string, path []string, depth int)
	dfs = func(curr string, path []string, depth int) {
		if depth > maxDepth {
			return
		}
		if curr == toSymbol {
			pCopy := make([]string, len(path))
			copy(pCopy, path)
			results = append(results, pCopy)
			return
		}

		visited[curr] = true
		for _, edge := range g.edgesFromCaller[curr] {
			next := edge.CalleeName
			if !visited[next] {
				dfs(next, append(path, next), depth+1)
			}
		}
		visited[curr] = false
	}

	dfs(fromSymbol, []string{fromSymbol}, 0)
	return results
}

// DetectCycles finds recursive or circular call loops in the call graph using Tarjan's/DFS.
func (g *CallGraph) DetectCycles() [][]string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := make(map[string]bool)
	onStack := make(map[string]bool)
	var cycles [][]string

	var dfs func(node string, path []string)
	dfs = func(node string, path []string) {
		visited[node] = true
		onStack[node] = true
		path = append(path, node)

		for _, edge := range g.edgesFromCaller[node] {
			neighbor := edge.CalleeName
			if !visited[neighbor] {
				dfs(neighbor, path)
			} else if onStack[neighbor] {
				// Found cycle: slice path from neighbor to end
				cycleStart := -1
				for i, n := range path {
					if n == neighbor {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycle := make([]string, len(path[cycleStart:]))
					copy(cycle, path[cycleStart:])
					cycle = append(cycle, neighbor) // close the loop
					cycles = append(cycles, cycle)
				}
			}
		}

		onStack[node] = false
	}

	for node := range g.edgesFromCaller {
		if !visited[node] {
			dfs(node, nil)
		}
	}

	return cycles
}

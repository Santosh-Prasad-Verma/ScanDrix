// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package knowledge

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// IncrementalSyncRequest specifies the files changed in a commit or PR for AST synchronization.
type IncrementalSyncRequest struct {
	RepositoryID string            `json:"repository_id"`
	ChangedFiles []string          `json:"changed_files"`
	FileContents map[string]string `json:"file_contents"` // filePath -> content (empty string indicates deleted file)
	NewSha       string            `json:"new_sha"`
}

// IncrementalSyncReport details the outcome and blast radius of an incremental AST update.
type IncrementalSyncReport struct {
	RepositoryID        string        `json:"repository_id"`
	NewSha              string        `json:"new_sha"`
	ModifiedFilesCount  int           `json:"modified_files_count"`
	DeletedFilesCount   int           `json:"deleted_files_count"`
	AddedSymbols        []string      `json:"added_symbols"`
	RemovedSymbols      []string      `json:"removed_symbols"`
	ModifiedSymbols     []string      `json:"modified_symbols"`
	ImpactedCallerCount int           `json:"impacted_caller_count"`
	ImpactedCallers     []string      `json:"impacted_callers"`
	DirectBlastRadius   []string      `json:"direct_blast_radius"`
	TotalNodes          int           `json:"total_nodes"`
	TotalEdges          int           `json:"total_edges"`
	Duration            time.Duration `json:"duration"`
	Timestamp           time.Time     `json:"timestamp"`
}

// IncrementalGraphSynchronizer manages localized incremental updates to the codebase
// AST symbol index and directed call graph without needing a full-repo re-indexing.
type IncrementalGraphSynchronizer struct {
	index *SymbolIndex
	graph *CallGraph
	mu    sync.RWMutex
}

// NewIncrementalGraphSynchronizer constructs a synchronizer attached to an index and call graph.
func NewIncrementalGraphSynchronizer(index *SymbolIndex, graph *CallGraph) *IncrementalGraphSynchronizer {
	if index == nil {
		index = NewSymbolIndex()
	}
	if graph == nil {
		graph = NewCallGraph()
	}

	return &IncrementalGraphSynchronizer{
		index: index,
		graph: graph,
	}
}

// GetIndex returns the underlying symbol index.
func (s *IncrementalGraphSynchronizer) GetIndex() *SymbolIndex {
	return s.index
}

// GetGraph returns the underlying call graph.
func (s *IncrementalGraphSynchronizer) GetGraph() *CallGraph {
	return s.graph
}

// Sync performs an atomic incremental AST graph synchronization for the specified changed files.
func (s *IncrementalGraphSynchronizer) Sync(ctx context.Context, req IncrementalSyncRequest) (*IncrementalSyncReport, error) {
	startTime := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	if req.RepositoryID == "" {
		return nil, fmt.Errorf("repository_id is required")
	}

	report := &IncrementalSyncReport{
		RepositoryID:    req.RepositoryID,
		NewSha:          req.NewSha,
		AddedSymbols:    make([]string, 0),
		RemovedSymbols:  make([]string, 0),
		ModifiedSymbols: make([]string, 0),
		ImpactedCallers: make([]string, 0),
		Timestamp:       startTime,
	}

	// 1. Snapshot previous definitions and existing callers in changed files
	oldDefsBySymbol := make(map[string]SymbolDefinition)
	oldCallersBySymbol := make(map[string][]CallGraphEdge)
	for _, file := range req.ChangedFiles {
		cleanFile := filepath.ToSlash(filepath.Clean(file))
		defs := s.index.GetDefinitionsInFile(cleanFile)
		for _, d := range defs {
			oldDefsBySymbol[d.Name] = d
			oldCallersBySymbol[d.Name] = s.graph.GetCallers(d.Name)
		}
	}

	// 2. Purge old definitions and edges for all changed files
	for _, file := range req.ChangedFiles {
		cleanFile := filepath.ToSlash(filepath.Clean(file))
		s.index.RemoveFile(cleanFile)
		s.graph.RemoveFileEdges(cleanFile)
	}

	// 3. Re-index updated files
	newDefsBySymbol := make(map[string]SymbolDefinition)
	for _, file := range req.ChangedFiles {
		cleanFile := filepath.ToSlash(filepath.Clean(file))
		content, hasContent := req.FileContents[file]
		if !hasContent {
			content = req.FileContents[cleanFile]
		}

		if content == "" {
			// Deleted file
			report.DeletedFilesCount++
			continue
		}

		report.ModifiedFilesCount++
		s.index.IndexFile(cleanFile, content)

		// Record newly parsed definitions
		newDefs := s.index.GetDefinitionsInFile(cleanFile)
		for _, d := range newDefs {
			newDefsBySymbol[d.Name] = d
		}
	}

	// 4. Re-link call graph edges for the newly indexed files
	for _, file := range req.ChangedFiles {
		cleanFile := filepath.ToSlash(filepath.Clean(file))
		defs := s.index.GetDefinitionsInFile(cleanFile)
		for _, d := range defs {
			callees := s.index.FindCallees(d.Name)
			for _, calleeName := range callees {
				calleeDefs, ok := s.index.GetDefinition(calleeName)
				calleeFile := ""
				if ok && len(calleeDefs) > 0 {
					calleeFile = calleeDefs[0].Location.FilePath
				}

				edge := CallGraphEdge{
					CallerName:     d.Name,
					CallerFile:     cleanFile,
					CallerLine:     d.Location.StartLine,
					CalleeName:     calleeName,
					CalleeFile:     calleeFile,
					IsCrossFile:    calleeFile != "" && calleeFile != cleanFile,
					IsCrossPackage: false,
				}
				s.graph.AddEdge(edge)
			}
		}
	}

	// 5. Diff Analysis: classify Added, Removed, and Modified symbols
	for symName, oldDef := range oldDefsBySymbol {
		newDef, exists := newDefsBySymbol[symName]
		if !exists {
			report.RemovedSymbols = append(report.RemovedSymbols, symName)
		} else if oldDef.Signature != newDef.Signature || oldDef.Kind != newDef.Kind {
			report.ModifiedSymbols = append(report.ModifiedSymbols, symName)
		}
	}

	for symName := range newDefsBySymbol {
		if _, exists := oldDefsBySymbol[symName]; !exists {
			report.AddedSymbols = append(report.AddedSymbols, symName)
		}
	}

	sort.Strings(report.AddedSymbols)
	sort.Strings(report.RemovedSymbols)
	sort.Strings(report.ModifiedSymbols)

	// 6. Impact Analysis: determine callers affected by removed or modified symbols
	impactedCallersMap := make(map[string]struct{})
	directBlastFilesMap := make(map[string]struct{})

	symbolsToTrace := append(append([]string{}, report.RemovedSymbols...), report.ModifiedSymbols...)
	for _, sym := range symbolsToTrace {
		for _, edge := range oldCallersBySymbol[sym] {
			impactedCallersMap[edge.CallerName] = struct{}{}
			if edge.CallerFile != "" {
				directBlastFilesMap[edge.CallerFile] = struct{}{}
			}
		}
		for _, edge := range s.graph.GetCallers(sym) {
			impactedCallersMap[edge.CallerName] = struct{}{}
			if edge.CallerFile != "" {
				directBlastFilesMap[edge.CallerFile] = struct{}{}
			}
		}
	}

	for caller := range impactedCallersMap {
		report.ImpactedCallers = append(report.ImpactedCallers, caller)
	}
	sort.Strings(report.ImpactedCallers)
	report.ImpactedCallerCount = len(report.ImpactedCallers)

	for file := range directBlastFilesMap {
		report.DirectBlastRadius = append(report.DirectBlastRadius, file)
	}
	sort.Strings(report.DirectBlastRadius)

	// 7. Graph totals
	report.TotalNodes = len(s.graph.symbolFiles)
	totalEdges := 0
	for _, edges := range s.graph.edgesFromCaller {
		totalEdges += len(edges)
	}
	report.TotalEdges = totalEdges

	report.Duration = time.Since(startTime)
	return report, nil
}

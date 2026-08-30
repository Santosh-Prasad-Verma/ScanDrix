package graph

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/codeanalysis/languages"
)

// ASTNodeKind categorizes code structural entities in the repository graph.
type ASTNodeKind string

const (
	NodeFunction ASTNodeKind = "FUNCTION"
	NodeClass    ASTNodeKind = "CLASS"
	NodePackage  ASTNodeKind = "PACKAGE"
	NodeEndpoint ASTNodeKind = "ENDPOINT"
)

// ASTEdgeKind defines the semantic relationship between two code symbols.
type ASTEdgeKind string

const (
	EdgeCalls     ASTEdgeKind = "CALLS"
	EdgeImports   ASTEdgeKind = "IMPORTS"
	EdgeDependsOn ASTEdgeKind = "DEPENDS_ON"
)

// ASTNode represents a persistent code symbol in the repository architecture.
type ASTNode struct {
	ID           uuid.UUID   `json:"id" db:"id"`
	RepositoryID uuid.UUID   `json:"repository_id" db:"repository_id"`
	Kind         ASTNodeKind `json:"kind" db:"kind"`
	SymbolName   string      `json:"symbol_name" db:"symbol_name"`
	FilePath     string      `json:"file_path" db:"file_path"`
	StartLine    int         `json:"start_line" db:"start_line"`
	EndLine      int         `json:"end_line" db:"end_line"`
	Signature    string      `json:"signature" db:"signature"`
	Language     string      `json:"language" db:"language"`
	UpdatedAt    time.Time   `json:"updated_at" db:"updated_at"`
}

// ASTEdge connects two code symbols to model architectural dependencies and call graphs.
type ASTEdge struct {
	ID           uuid.UUID   `json:"id" db:"id"`
	RepositoryID uuid.UUID   `json:"repository_id" db:"repository_id"`
	FromNodeID   uuid.UUID   `json:"from_node_id" db:"from_node_id"`
	ToNodeID     uuid.UUID   `json:"to_node_id" db:"to_node_id"`
	Kind         ASTEdgeKind `json:"kind" db:"kind"`
}

// GraphIndexer manages the incremental persistence and traversal of repository AST graphs.
type GraphIndexer struct {
	mu    sync.RWMutex
	nodes map[uuid.UUID]ASTNode
	edges []ASTEdge
}

func NewGraphIndexer() *GraphIndexer {
	return &GraphIndexer{
		nodes: make(map[uuid.UUID]ASTNode),
		edges: make([]ASTEdge, 0),
	}
}

// Lock acquires the write lock for batch operations.
func (g *GraphIndexer) Lock() {
	g.mu.Lock()
}

// Unlock releases the write lock.
func (g *GraphIndexer) Unlock() {
	g.mu.Unlock()
}

// GetEdges returns a copy of all collected edges (caller must hold lock).
func (g *GraphIndexer) GetEdges() []ASTEdge {
	out := make([]ASTEdge, len(g.edges))
	copy(out, g.edges)
	return out
}

// GetNodes returns a copy of all collected nodes (caller must hold lock).
func (g *GraphIndexer) GetNodes() []ASTNode {
	out := make([]ASTNode, 0, len(g.nodes))
	for _, n := range g.nodes {
		out = append(out, n)
	}
	return out
}

// UpsertNode records or updates a symbol in the graph.
func (g *GraphIndexer) UpsertNode(node ASTNode) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nodes[node.ID] = node
}

// AddEdge registers a relationship between two symbols.
func (g *GraphIndexer) AddEdge(edge ASTEdge) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.edges = append(g.edges, edge)
}

// FindNodeBySymbol retrieves a node by symbol name and file path.
func (g *GraphIndexer) FindNodeBySymbol(repoID uuid.UUID, symbolName, filePath string) (*ASTNode, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, n := range g.nodes {
		if n.RepositoryID == repoID && n.SymbolName == symbolName && n.FilePath == filePath {
			return &n, true
		}
	}
	return nil, false
}

// QueryCallers finds all symbols that directly call the target symbol.
func (g *GraphIndexer) QueryCallers(targetNodeID uuid.UUID) []ASTNode {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var callers []ASTNode
	for _, edge := range g.edges {
		if edge.ToNodeID == targetNodeID && edge.Kind == EdgeCalls {
			if callerNode, ok := g.nodes[edge.FromNodeID]; ok {
				callers = append(callers, callerNode)
			}
		}
	}
	return callers
}

// IndexGoFile parses a Go source string, extracts declarations and calls, and updates the graph.
func (g *GraphIndexer) IndexGoFile(ctx context.Context, repoID uuid.UUID, filePath, content string) ([]ASTNode, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, content, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var extractedNodes []ASTNode
	symbolMap := make(map[string]uuid.UUID)

	// Step 1: Extract Function Declarations
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			return true
		}

		startPos := fset.Position(fn.Pos())
		endPos := fset.Position(fn.End())

		nodeID := uuid.New()
		symbolName := fn.Name.Name
		symbolMap[symbolName] = nodeID

		node := ASTNode{
			ID:           nodeID,
			RepositoryID: repoID,
			Kind:         NodeFunction,
			SymbolName:   symbolName,
			FilePath:     filePath,
			StartLine:    startPos.Line,
			EndLine:      endPos.Line,
			Language:     "go",
			UpdatedAt:    time.Now().UTC(),
		}

		g.UpsertNode(node)
		extractedNodes = append(extractedNodes, node)
		return true
	})

	// Step 2: Extract Inter-Function Calls
	for _, fromNode := range extractedNodes {
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Name == nil || fn.Name.Name != fromNode.SymbolName {
				return true
			}

			// Traverse call expressions within this function body
			ast.Inspect(fn.Body, func(inner ast.Node) bool {
				call, ok := inner.(*ast.CallExpr)
				if !ok {
					return true
				}

				var calleeName string
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					calleeName = fun.Name
				case *ast.SelectorExpr:
					calleeName = fun.Sel.Name
				}

				if targetNodeID, exists := symbolMap[calleeName]; exists && targetNodeID != fromNode.ID {
					g.AddEdge(ASTEdge{
						ID:           uuid.New(),
						RepositoryID: repoID,
						FromNodeID:   fromNode.ID,
						ToNodeID:     targetNodeID,
						Kind:         EdgeCalls,
					})
				}
				return true
			})
			return false
		})
	}

	return extractedNodes, nil
}

// IndexTSFile parses TypeScript/JavaScript sources, extracting declarations and call graph edges.
func (g *GraphIndexer) IndexTSFile(ctx context.Context, repoID uuid.UUID, filePath, content string) ([]ASTNode, error) {
	analysis := languages.AnalyzeTSSource(content)
	var extractedNodes []ASTNode
	symbolMap := make(map[string]uuid.UUID)

	now := time.Now().UTC()

	// 1. Index Functions
	for _, fn := range analysis.Functions {
		nodeID := uuid.New()
		symbolMap[fn.Name] = nodeID

		node := ASTNode{
			ID:           nodeID,
			RepositoryID: repoID,
			Kind:         NodeFunction,
			SymbolName:   fn.Name,
			FilePath:     filePath,
			StartLine:    fn.StartLine,
			EndLine:      fn.EndLine,
			Signature:    fn.Signature,
			Language:     "typescript",
			UpdatedAt:    now,
		}
		g.UpsertNode(node)
		extractedNodes = append(extractedNodes, node)
	}

	// 2. Index Classes
	for _, cls := range analysis.Classes {
		nodeID := uuid.New()
		node := ASTNode{
			ID:           nodeID,
			RepositoryID: repoID,
			Kind:         NodeClass,
			SymbolName:   cls,
			FilePath:     filePath,
			StartLine:    1,
			EndLine:      1,
			Signature:    "class " + cls,
			Language:     "typescript",
			UpdatedAt:    now,
		}
		g.UpsertNode(node)
		extractedNodes = append(extractedNodes, node)
	}

	// 3. Connect Function Calls
	for _, fn := range analysis.Functions {
		fromNodeID, ok := symbolMap[fn.Name]
		if !ok {
			continue
		}
		for _, callee := range fn.Calls {
			if toNodeID, exists := symbolMap[callee]; exists && toNodeID != fromNodeID {
				g.AddEdge(ASTEdge{
					ID:           uuid.New(),
					RepositoryID: repoID,
					FromNodeID:   fromNodeID,
					ToNodeID:     toNodeID,
					Kind:         EdgeCalls,
				})
			}
		}
	}

	return extractedNodes, nil
}

// IndexPythonFile parses Python sources, extracting function/class declarations and call graph edges.
func (g *GraphIndexer) IndexPythonFile(ctx context.Context, repoID uuid.UUID, filePath, content string) ([]ASTNode, error) {
	analysis := languages.AnalyzePySource(content)
	var extractedNodes []ASTNode
	symbolMap := make(map[string]uuid.UUID)

	now := time.Now().UTC()

	// 1. Index Functions
	for _, fn := range analysis.Functions {
		nodeID := uuid.New()
		symbolMap[fn.Name] = nodeID

		node := ASTNode{
			ID:           nodeID,
			RepositoryID: repoID,
			Kind:         NodeFunction,
			SymbolName:   fn.Name,
			FilePath:     filePath,
			StartLine:    fn.StartLine,
			EndLine:      fn.EndLine,
			Signature:    fn.Signature,
			Language:     "python",
			UpdatedAt:    now,
		}
		g.UpsertNode(node)
		extractedNodes = append(extractedNodes, node)
	}

	// 2. Index Classes
	for _, cls := range analysis.Classes {
		nodeID := uuid.New()
		node := ASTNode{
			ID:           nodeID,
			RepositoryID: repoID,
			Kind:         NodeClass,
			SymbolName:   cls,
			FilePath:     filePath,
			StartLine:    1,
			EndLine:      1,
			Signature:    "class " + cls,
			Language:     "python",
			UpdatedAt:    now,
		}
		g.UpsertNode(node)
		extractedNodes = append(extractedNodes, node)
	}

	// 3. Connect Function Calls
	for _, fn := range analysis.Functions {
		fromNodeID, ok := symbolMap[fn.Name]
		if !ok {
			continue
		}
		for _, callee := range fn.Calls {
			if toNodeID, exists := symbolMap[callee]; exists && toNodeID != fromNodeID {
				g.AddEdge(ASTEdge{
					ID:           uuid.New(),
					RepositoryID: repoID,
					FromNodeID:   fromNodeID,
					ToNodeID:     toNodeID,
					Kind:         EdgeCalls,
				})
			}
		}
	}

	return extractedNodes, nil
}

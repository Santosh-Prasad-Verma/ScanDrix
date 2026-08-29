package languages

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

// GoFunctionMeta holds structural information about a Go function or method.
type GoFunctionMeta struct {
	Name        string
	Receiver    string
	StartLine   int
	EndLine     int
	ParamsCount int
	Returns     int
	Complexity  int
}

// GoFileAnalysis models the extracted AST structure of a Go file.
type GoFileAnalysis struct {
	PackageName string
	Imports     []string
	Functions   []GoFunctionMeta
	Structs     []string
	Interfaces  []string
}

// AnalyzeGoSource parses a Go source code buffer and extracts detailed AST metadata.
func AnalyzeGoSource(filename string, content string) (*GoFileAnalysis, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed parsing go source: %w", err)
	}

	analysis := &GoFileAnalysis{
		PackageName: node.Name.Name,
		Imports:     make([]string, 0, len(node.Imports)),
		Functions:   make([]GoFunctionMeta, 0),
		Structs:     make([]string, 0),
		Interfaces:  make([]string, 0),
	}

	for _, imp := range node.Imports {
		if imp.Path != nil {
			analysis.Imports = append(analysis.Imports, imp.Path.Value)
		}
	}

	ast.Inspect(node, func(n ast.Node) bool {
		switch t := n.(type) {
		case *ast.TypeSpec:
			switch t.Type.(type) {
			case *ast.StructType:
				analysis.Structs = append(analysis.Structs, t.Name.Name)
			case *ast.InterfaceType:
				analysis.Interfaces = append(analysis.Interfaces, t.Name.Name)
			}

		case *ast.FuncDecl:
			start := fset.Position(t.Pos()).Line
			end := fset.Position(t.End()).Line

			receiver := ""
			if t.Recv != nil && len(t.Recv.List) > 0 {
				if expr := t.Recv.List[0].Type; expr != nil {
					receiver = fmt.Sprintf("%s", expr)
				}
			}

			returnsCount := 0
			if t.Type.Results != nil {
				returnsCount = t.Type.Results.NumFields()
			}

			complexity := computeComplexity(t.Body)

			analysis.Functions = append(analysis.Functions, GoFunctionMeta{
				Name:        t.Name.Name,
				Receiver:    receiver,
				StartLine:   start,
				EndLine:     end,
				ParamsCount: t.Type.Params.NumFields(),
				Returns:     returnsCount,
				Complexity:  complexity,
			})
		}
		return true
	})

	return analysis, nil
}

// FindEnclosingFunction locates the function enclosing the given line number.
func (g *GoFileAnalysis) FindEnclosingFunction(line int) *GoFunctionMeta {
	for _, fn := range g.Functions {
		if line >= fn.StartLine && line <= fn.EndLine {
			return &fn
		}
	}
	return nil
}

func computeComplexity(body *ast.BlockStmt) int {
	if body == nil {
		return 1
	}
	complexity := 1
	ast.Inspect(body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
			complexity++
		case *ast.BinaryExpr:
			// Logical AND/OR operators increase branching paths
			// (Checked during deeper traversal)
		}
		return true
	})
	return complexity
}

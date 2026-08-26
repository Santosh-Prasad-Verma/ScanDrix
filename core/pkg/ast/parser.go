package ast

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

// Parser handles multilingual source code parsing and symbol extraction.
type Parser struct{}

// NewParser creates a new AST Parser instance.
func NewParser() *Parser {
	return &Parser{}
}

// ParseFile inspects a source code file and extracts all structural symbols.
func (p *Parser) ParseFile(codeFileID uuid.UUID, filePath, language string, content []byte) ([]domain.CodeSymbol, error) {
	switch strings.ToLower(language) {
	case "go", "golang":
		return p.parseGo(codeFileID, filePath, content)
	case "typescript", "javascript", "ts", "js":
		return p.parseTypeScript(codeFileID, filePath, content)
	case "python", "py":
		return p.parsePython(codeFileID, filePath, content)
	case "rust", "rs":
		return p.parseRust(codeFileID, filePath, content)
	default:
		return p.parseGeneric(codeFileID, filePath, language, content)
	}
}

// parseGo extracts Go AST symbols using standard go/parser and go/ast.
func (p *Parser) parseGo(codeFileID uuid.UUID, filePath string, content []byte) ([]domain.CodeSymbol, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, content, parser.ParseComments)
	if err != nil {
		// Fallback to generic line-based extractor if compilation units have syntax errors
		return p.parseGeneric(codeFileID, filePath, "go", content)
	}

	var symbols []domain.CodeSymbol
	pkgName := node.Name.Name

	ast.Inspect(node, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.FuncDecl:
			pos := fset.Position(decl.Pos())
			end := fset.Position(decl.End())

			name := decl.Name.Name
			kind := "FUNCTION"
			symbolKey := fmt.Sprintf("%s.%s", pkgName, name)

			if decl.Recv != nil && len(decl.Recv.List) > 0 {
				kind = "METHOD"
				recvType := fmt.Sprintf("%v", decl.Recv.List[0].Type)
				symbolKey = fmt.Sprintf("%s.%s.%s", pkgName, recvType, name)
			}

			sig := fmt.Sprintf("func %s(...)", name)
			vis := "PRIVATE"
			if ast.IsExported(name) {
				vis = "PUBLIC"
			}

			symbols = append(symbols, domain.CodeSymbol{
				ID:            uuid.New(),
				CodeFileID:    codeFileID,
				SymbolKey:     symbolKey,
				Name:          name,
				Kind:          kind,
				StartLine:     pos.Line,
				EndLine:       end.Line,
				StartColumn:   pos.Column,
				EndColumn:     end.Column,
				Signature:     &sig,
				Visibility:    vis,
				TaintedInputs: []string{},
			})

		case *ast.TypeSpec:
			pos := fset.Position(decl.Pos())
			end := fset.Position(decl.End())
			name := decl.Name.Name
			kind := "TYPE"

			switch decl.Type.(type) {
			case *ast.StructType:
				kind = "STRUCT"
			case *ast.InterfaceType:
				kind = "INTERFACE"
			}

			symbolKey := fmt.Sprintf("%s.%s", pkgName, name)
			vis := "PRIVATE"
			if ast.IsExported(name) {
				vis = "PUBLIC"
			}

			symbols = append(symbols, domain.CodeSymbol{
				ID:            uuid.New(),
				CodeFileID:    codeFileID,
				SymbolKey:     symbolKey,
				Name:          name,
				Kind:          kind,
				StartLine:     pos.Line,
				EndLine:       end.Line,
				StartColumn:   pos.Column,
				EndColumn:     end.Column,
				Visibility:    vis,
				TaintedInputs: []string{},
			})
		}
		return true
	})

	return symbols, nil
}

// parseTypeScript extracts TS/JS functions, classes, and route declarations.
func (p *Parser) parseTypeScript(codeFileID uuid.UUID, filePath string, content []byte) ([]domain.CodeSymbol, error) {
	var symbols []domain.CodeSymbol
	lines := strings.Split(string(content), "\n")

	funcRegex := regexp.MustCompile(`(?:export\s+)?(?:async\s+)?function\s+([a-zA-Z0-9_$]+)\s*\((.*?)\)`)
	arrowFuncRegex := regexp.MustCompile(`(?:export\s+)?(?:const|let|var)\s+([a-zA-Z0-9_$]+)\s*=\s*(?:async\s+)?\((.*?)\)\s*=>`)
	classRegex := regexp.MustCompile(`(?:export\s+)?class\s+([a-zA-Z0-9_$]+)`)
	routeRegex := regexp.MustCompile(`(?:router|app)\.(get|post|put|delete|patch)\s*\(\s*['"]([^'"]+)['"]`)

	for i, line := range lines {
		lineNum := i + 1

		if m := funcRegex.FindStringSubmatch(line); len(m) > 1 {
			name := m[1]
			sig := m[0]
			vis := "PRIVATE"
			if strings.Contains(line, "export") {
				vis = "PUBLIC"
			}
			symbols = append(symbols, domain.CodeSymbol{
				ID:            uuid.New(),
				CodeFileID:    codeFileID,
				SymbolKey:     fmt.Sprintf("%s:%s", filePath, name),
				Name:          name,
				Kind:          "FUNCTION",
				StartLine:     lineNum,
				EndLine:       lineNum + 5, // Estimated boundary
				StartColumn:   1,
				EndColumn:     len(line),
				Signature:     &sig,
				Visibility:    vis,
				TaintedInputs: []string{},
			})
		} else if m := arrowFuncRegex.FindStringSubmatch(line); len(m) > 1 {
			name := m[1]
			sig := m[0]
			symbols = append(symbols, domain.CodeSymbol{
				ID:            uuid.New(),
				CodeFileID:    codeFileID,
				SymbolKey:     fmt.Sprintf("%s:%s", filePath, name),
				Name:          name,
				Kind:          "FUNCTION",
				StartLine:     lineNum,
				EndLine:       lineNum + 5,
				StartColumn:   1,
				EndColumn:     len(line),
				Signature:     &sig,
				Visibility:    "PUBLIC",
				TaintedInputs: []string{},
			})
		} else if m := classRegex.FindStringSubmatch(line); len(m) > 1 {
			name := m[1]
			symbols = append(symbols, domain.CodeSymbol{
				ID:            uuid.New(),
				CodeFileID:    codeFileID,
				SymbolKey:     fmt.Sprintf("%s:%s", filePath, name),
				Name:          name,
				Kind:          "CLASS",
				StartLine:     lineNum,
				EndLine:       lineNum + 20,
				StartColumn:   1,
				EndColumn:     len(line),
				Visibility:    "PUBLIC",
				TaintedInputs: []string{},
			})
		} else if m := routeRegex.FindStringSubmatch(line); len(m) > 2 {
			method := strings.ToUpper(m[1])
			path := m[2]
			routeName := fmt.Sprintf("%s %s", method, path)
			symbols = append(symbols, domain.CodeSymbol{
				ID:            uuid.New(),
				CodeFileID:    codeFileID,
				SymbolKey:     fmt.Sprintf("%s:route:%s_%s", filePath, method, path),
				Name:          routeName,
				Kind:          "API_ROUTE",
				StartLine:     lineNum,
				EndLine:       lineNum + 10,
				StartColumn:   1,
				EndColumn:     len(line),
				Visibility:    "PUBLIC",
				TaintedInputs: []string{"params", "body", "query"},
			})
		}
	}

	return symbols, nil
}

// parsePython extracts Python functions, classes, and FastAPI/Flask route decorators.
func (p *Parser) parsePython(codeFileID uuid.UUID, filePath string, content []byte) ([]domain.CodeSymbol, error) {
	var symbols []domain.CodeSymbol
	lines := strings.Split(string(content), "\n")

	defRegex := regexp.MustCompile(`^\s*(?:async\s+)?def\s+([a-zA-Z0-9_]+)\s*\((.*?)\):`)
	classRegex := regexp.MustCompile(`^\s*class\s+([a-zA-Z0-9_]+)(?:\((.*?)\))?:`)
	routeRegex := regexp.MustCompile(`@(?:app|router)\.(get|post|put|delete|patch)\s*\(\s*['"]([^'"]+)['"]`)

	for i, line := range lines {
		lineNum := i + 1

		if m := routeRegex.FindStringSubmatch(line); len(m) > 2 {
			method := strings.ToUpper(m[1])
			path := m[2]
			symbols = append(symbols, domain.CodeSymbol{
				ID:            uuid.New(),
				CodeFileID:    codeFileID,
				SymbolKey:     fmt.Sprintf("%s:route:%s_%s", filePath, method, path),
				Name:          fmt.Sprintf("%s %s", method, path),
				Kind:          "API_ROUTE",
				StartLine:     lineNum,
				EndLine:       lineNum + 10,
				StartColumn:   1,
				EndColumn:     len(line),
				Visibility:    "PUBLIC",
				TaintedInputs: []string{"request", "payload", "query"},
			})
		} else if m := defRegex.FindStringSubmatch(line); len(m) > 1 {
			name := m[1]
			sig := m[0]
			symbols = append(symbols, domain.CodeSymbol{
				ID:            uuid.New(),
				CodeFileID:    codeFileID,
				SymbolKey:     fmt.Sprintf("%s.%s", filePath, name),
				Name:          name,
				Kind:          "FUNCTION",
				StartLine:     lineNum,
				EndLine:       lineNum + 8,
				StartColumn:   1,
				EndColumn:     len(line),
				Signature:     &sig,
				Visibility:    "PUBLIC",
				TaintedInputs: []string{},
			})
		} else if m := classRegex.FindStringSubmatch(line); len(m) > 1 {
			name := m[1]
			symbols = append(symbols, domain.CodeSymbol{
				ID:            uuid.New(),
				CodeFileID:    codeFileID,
				SymbolKey:     fmt.Sprintf("%s.%s", filePath, name),
				Name:          name,
				Kind:          "CLASS",
				StartLine:     lineNum,
				EndLine:       lineNum + 25,
				StartColumn:   1,
				EndColumn:     len(line),
				Visibility:    "PUBLIC",
				TaintedInputs: []string{},
			})
		}
	}

	return symbols, nil
}

// parseRust extracts Rust functions, structs, traits, and impl blocks.
func (p *Parser) parseRust(codeFileID uuid.UUID, filePath string, content []byte) ([]domain.CodeSymbol, error) {
	var symbols []domain.CodeSymbol
	lines := strings.Split(string(content), "\n")

	fnRegex := regexp.MustCompile(`^\s*(?:pub\s+)?(?:async\s+)?fn\s+([a-zA-Z0-9_]+)\s*\(`)
	structRegex := regexp.MustCompile(`^\s*(?:pub\s+)?struct\s+([a-zA-Z0-9_]+)`)
	traitRegex := regexp.MustCompile(`^\s*(?:pub\s+)?trait\s+([a-zA-Z0-9_]+)`)

	for i, line := range lines {
		lineNum := i + 1
		vis := "PRIVATE"
		if strings.Contains(line, "pub ") {
			vis = "PUBLIC"
		}

		if m := fnRegex.FindStringSubmatch(line); len(m) > 1 {
			symbols = append(symbols, domain.CodeSymbol{
				ID:            uuid.New(),
				CodeFileID:    codeFileID,
				SymbolKey:     fmt.Sprintf("%s::%s", filePath, m[1]),
				Name:          m[1],
				Kind:          "FUNCTION",
				StartLine:     lineNum,
				EndLine:       lineNum + 10,
				StartColumn:   1,
				EndColumn:     len(line),
				Visibility:    vis,
				TaintedInputs: []string{},
			})
		} else if m := structRegex.FindStringSubmatch(line); len(m) > 1 {
			symbols = append(symbols, domain.CodeSymbol{
				ID:            uuid.New(),
				CodeFileID:    codeFileID,
				SymbolKey:     fmt.Sprintf("%s::%s", filePath, m[1]),
				Name:          m[1],
				Kind:          "STRUCT",
				StartLine:     lineNum,
				EndLine:       lineNum + 15,
				StartColumn:   1,
				EndColumn:     len(line),
				Visibility:    vis,
				TaintedInputs: []string{},
			})
		} else if m := traitRegex.FindStringSubmatch(line); len(m) > 1 {
			symbols = append(symbols, domain.CodeSymbol{
				ID:            uuid.New(),
				CodeFileID:    codeFileID,
				SymbolKey:     fmt.Sprintf("%s::%s", filePath, m[1]),
				Name:          m[1],
				Kind:          "TRAIT",
				StartLine:     lineNum,
				EndLine:       lineNum + 15,
				StartColumn:   1,
				EndColumn:     len(line),
				Visibility:    vis,
				TaintedInputs: []string{},
			})
		}
	}

	return symbols, nil
}

// parseGeneric fallback extractor.
func (p *Parser) parseGeneric(codeFileID uuid.UUID, filePath, language string, content []byte) ([]domain.CodeSymbol, error) {
	hash := sha256.Sum256(content)
	symbolKey := fmt.Sprintf("%s:root:%s", filePath, hex.EncodeToString(hash[:8]))

	return []domain.CodeSymbol{
		{
			ID:            uuid.New(),
			CodeFileID:    codeFileID,
			SymbolKey:     symbolKey,
			Name:          filePath,
			Kind:          "MODULE",
			StartLine:     1,
			EndLine:       len(strings.Split(string(content), "\n")),
			StartColumn:   1,
			EndColumn:     1,
			Visibility:    "PUBLIC",
			TaintedInputs: []string{},
		},
	}, nil
}

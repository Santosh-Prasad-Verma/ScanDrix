package ast

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// GoASTAnalyzer performs deep syntax inspection and metrics computation on Go source code.
type GoASTAnalyzer struct{}

// NewGoASTAnalyzer initializes the analyzer.
func NewGoASTAnalyzer() *GoASTAnalyzer {
	return &GoASTAnalyzer{}
}

// Analyze parses Go source and calculates structural complexity and code smell findings.
func (a *GoASTAnalyzer) Analyze(filePath, src string) (*ComplexityReport, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("ast parse error in %s: %w", filePath, err)
	}

	lines := strings.Split(src, "\n")
	loc := len(lines)
	commentLines := 0
	blankLines := 0

	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			blankLines++
		} else if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			commentLines++
		}
	}

	report := &ComplexityReport{
		FilePath:     filePath,
		Language:     "go",
		LinesOfCode:  loc,
		CommentLines: commentLines,
		BlankLines:   blankLines,
		Functions:    make([]FunctionComplexity, 0),
		Findings:     make([]models.CodeFinding, 0),
		Halstead:     CalculateHalstead(src),
	}

	totalCyclomatic := 0
	totalCognitive := 0

	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}

		startPos := fset.Position(fn.Pos())
		endPos := fset.Position(fn.End())

		paramCount := 0
		if fn.Type.Params != nil {
			for _, field := range fn.Type.Params.List {
				if len(field.Names) == 0 {
					paramCount++
				} else {
					paramCount += len(field.Names)
				}
			}
		}

		returnCount := 0
		if fn.Type.Results != nil {
			for _, field := range fn.Type.Results.List {
				if len(field.Names) == 0 {
					returnCount++
				} else {
					returnCount += len(field.Names)
				}
			}
		}

		cyclomatic := computeCyclomatic(fn.Body)
		cognitive, maxDepth := computeCognitive(fn.Body, 0)

		totalCyclomatic += cyclomatic
		totalCognitive += cognitive

		fnComp := FunctionComplexity{
			Name:                 fn.Name.Name,
			StartLine:            startPos.Line,
			EndLine:              endPos.Line,
			CyclomaticComplexity: cyclomatic,
			CognitiveComplexity:  cognitive,
			ParamCount:           paramCount,
			ReturnCount:          returnCount,
			MaxNestingDepth:      maxDepth,
		}
		report.Functions = append(report.Functions, fnComp)

		// Rule 1: High Cyclomatic Complexity
		if cyclomatic > 15 {
			sev := models.SeverityMedium
			if cyclomatic > 25 {
				sev = models.SeverityHigh
			}
			report.Findings = append(report.Findings, models.CodeFinding{
				ID:          uuid.New(),
				FilePath:    filePath,
				StartLine:   startPos.Line,
				EndLine:     endPos.Line,
				Title:       fmt.Sprintf("High Cyclomatic Complexity in %s (%d)", fn.Name.Name, cyclomatic),
				Severity:    sev,
				Category:    "CODE_COMPLEXITY",
				Description: fmt.Sprintf("Function %s has a cyclomatic complexity score of %d (threshold is 15). Consider breaking it into smaller modular subroutines.", fn.Name.Name, cyclomatic),
				Remediation: "Extract independent branches into dedicated private helper methods.",
			})
		}

		// Rule 2: Excessive Parameters (> 5)
		if paramCount > 5 {
			report.Findings = append(report.Findings, models.CodeFinding{
				ID:          uuid.New(),
				FilePath:    filePath,
				StartLine:   startPos.Line,
				EndLine:     startPos.Line,
				Title:       fmt.Sprintf("Excessive Function Parameters in %s (%d)", fn.Name.Name, paramCount),
				Severity:    models.SeverityLow,
				Category:    "CLEAN_CODE",
				Description: fmt.Sprintf("Function %s accepts %d parameters. Functions accepting more than 5 arguments harm readability and testability.", fn.Name.Name, paramCount),
				Remediation: "Group related arguments into a structured Options/Config struct.",
			})
		}

		// Rule 3: Dead Code Detection
		deadCodeFindings := detectDeadCode(fset, filePath, fn.Body)
		report.Findings = append(report.Findings, deadCodeFindings...)

		return false // don't recurse into nested function bodies twice
	})

	report.FileCyclomaticComplexity = totalCyclomatic
	report.FileCognitiveComplexity = totalCognitive

	return report, nil
}

func computeCyclomatic(body *ast.BlockStmt) int {
	complexity := 1 // Base complexity

	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
			complexity++
		case *ast.BinaryExpr:
			if node.Op == token.LAND || node.Op == token.LOR {
				complexity++
			}
		}
		return true
	})

	return complexity
}

func computeCognitive(body *ast.BlockStmt, currentDepth int) (int, int) {
	cognitive := 0
	maxDepth := currentDepth

	for _, stmt := range body.List {
		switch s := stmt.(type) {
		case *ast.IfStmt:
			cognitive += 1 + currentDepth
			subCog, subDepth := computeCognitive(s.Body, currentDepth+1)
			cognitive += subCog
			if subDepth > maxDepth {
				maxDepth = subDepth
			}
			if s.Else != nil {
				if elseBlock, ok := s.Else.(*ast.BlockStmt); ok {
					cognitive += 1
					subElse, elseDepth := computeCognitive(elseBlock, currentDepth)
					cognitive += subElse
					if elseDepth > maxDepth {
						maxDepth = elseDepth
					}
				}
			}
		case *ast.ForStmt:
			cognitive += 1 + currentDepth
			subCog, subDepth := computeCognitive(s.Body, currentDepth+1)
			cognitive += subCog
			if subDepth > maxDepth {
				maxDepth = subDepth
			}
		case *ast.RangeStmt:
			cognitive += 1 + currentDepth
			subCog, subDepth := computeCognitive(s.Body, currentDepth+1)
			cognitive += subCog
			if subDepth > maxDepth {
				maxDepth = subDepth
			}
		case *ast.SwitchStmt:
			cognitive += 1 + currentDepth
			if s.Body != nil {
				subCog, subDepth := computeCognitive(s.Body, currentDepth+1)
				cognitive += subCog
				if subDepth > maxDepth {
					maxDepth = subDepth
				}
			}
		}
	}

	return cognitive, maxDepth
}

func detectDeadCode(fset *token.FileSet, filePath string, body *ast.BlockStmt) []models.CodeFinding {
	var findings []models.CodeFinding

	for i, stmt := range body.List {
		isTerminating := false
		switch stmt.(type) {
		case *ast.ReturnStmt, *ast.BranchStmt:
			isTerminating = true
		}

		// If this statement terminates and there are subsequent statements in the same block
		if isTerminating && i < len(body.List)-1 {
			nextStmt := body.List[i+1]
			pos := fset.Position(nextStmt.Pos())
			findings = append(findings, models.CodeFinding{
				ID:          uuid.New(),
				FilePath:    filePath,
				StartLine:   pos.Line,
				EndLine:     pos.Line,
				Title:       "Unreachable Dead Code Detected",
				Severity:    models.SeverityMedium,
				Category:    "DEAD_CODE",
				Description: fmt.Sprintf("Statements following unconditional termination at line %d will never execute.", fset.Position(stmt.Pos()).Line),
				Remediation: "Remove or relocate dead unreachable statements.",
			})
			break
		}
	}

	return findings
}

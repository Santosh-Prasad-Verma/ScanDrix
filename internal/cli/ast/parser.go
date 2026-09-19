// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ast

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DetectLanguage resolves the language based on file extension.
func DetectLanguage(filePath string) Language {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return LangGo
	case ".ts", ".tsx":
		return LangTypeScript
	case ".js", ".jsx", ".mjs", ".cjs":
		return LangJavaScript
	case ".py":
		return LangPython
	default:
		return LangUnknown
	}
}

// ParseFile inspects a file and extracts all function symbols and complexity metrics.
func ParseFile(filePath string) (*FileAnalysis, error) {
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	content := string(contentBytes)
	lines := strings.Split(content, "\n")
	lang := DetectLanguage(filePath)

	analysis := &FileAnalysis{
		FilePath:   filePath,
		Language:   lang,
		TotalLines: len(lines),
	}

	switch lang {
	case LangGo:
		parseGoFile(filePath, contentBytes, lines, analysis)
	case LangTypeScript, LangJavaScript:
		parseJSFile(lines, analysis)
	case LangPython:
		parsePythonFile(lines, analysis)
	default:
		// Fallback generic scan
		parseGenericFile(lines, analysis)
	}

	// Calculate aggregate statistics
	totalComplexity := 0
	maxComp := 0
	for _, fn := range analysis.Functions {
		totalComplexity += fn.CyclomaticComplexity
		if fn.CyclomaticComplexity > maxComp {
			maxComp = fn.CyclomaticComplexity
		}
	}
	analysis.MaxComplexity = maxComp
	if len(analysis.Functions) > 0 {
		analysis.AvgComplexity = float64(totalComplexity) / float64(len(analysis.Functions))
	}

	// Run semantic rules to detect code smells
	analysis.Smells = DetectCodeSmells(analysis)

	return analysis, nil
}

func parseGoFile(filePath string, src []byte, lines []string, analysis *FileAnalysis) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		parseGenericFile(lines, analysis)
		return
	}

	for _, decl := range node.Decls {
		fnDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		startPos := fset.Position(fnDecl.Pos())
		endPos := fset.Position(fnDecl.End())

		startLine := startPos.Line
		endLine := endPos.Line
		if endLine < startLine {
			endLine = startLine
		}

		receiver := ""
		if fnDecl.Recv != nil && len(fnDecl.Recv.List) > 0 {
			var buf bytes.Buffer
			_ = printer.Fprint(&buf, fset, fnDecl.Recv.List[0].Type)
			receiver = buf.String()
		}

		fnLines := extractLines(lines, startLine, endLine)
		cyclo, cogn, maxDepth := CalculateComplexity(fnLines)

		analysis.Functions = append(analysis.Functions, FunctionSymbol{
			Name:                 fnDecl.Name.Name,
			Receiver:             receiver,
			Signature:            formatGoSignature(fnDecl, fset),
			StartLine:            startLine,
			EndLine:              endLine,
			LinesOfCode:          endLine - startLine + 1,
			CyclomaticComplexity: cyclo,
			CognitiveComplexity:  cogn,
			NestingDepth:         maxDepth,
			IsExported:           fnDecl.Name.IsExported(),
		})
	}
}

func formatGoSignature(fn *ast.FuncDecl, fset *token.FileSet) string {
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, fset, fn.Type)
	sig := buf.String()
	sig = strings.TrimPrefix(sig, "func")
	return fn.Name.Name + sig
}

var jsFuncRegex = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:async\s+)?function\s+([a-zA-Z0-9_$]+)\s*\((.*?)\)`)
var jsMethodRegex = regexp.MustCompile(`(?m)^\s*(?:async\s+)?([a-zA-Z0-9_$]+)\s*\((.*?)\)\s*\{`)
var pyFuncRegex = regexp.MustCompile(`(?m)^\s*(?:async\s+)?def\s+([a-zA-Z0-9_]+)\s*\((.*?)\):`)

func parseJSFile(lines []string, analysis *FileAnalysis) {
	for i, line := range lines {
		lineNum := i + 1
		matches := jsFuncRegex.FindStringSubmatch(line)
		if len(matches) >= 2 {
			name := matches[1]
			endLine := findMatchingBraceEnd(lines, i)
			fnLines := extractLines(lines, lineNum, endLine)
			cyclo, cogn, depth := CalculateComplexity(fnLines)

			analysis.Functions = append(analysis.Functions, FunctionSymbol{
				Name:                 name,
				Signature:            strings.TrimSpace(line),
				StartLine:            lineNum,
				EndLine:              endLine,
				LinesOfCode:          endLine - lineNum + 1,
				CyclomaticComplexity: cyclo,
				CognitiveComplexity:  cogn,
				NestingDepth:         depth,
				IsExported:           strings.Contains(line, "export"),
			})
		}
	}
}

func parsePythonFile(lines []string, analysis *FileAnalysis) {
	for i, line := range lines {
		lineNum := i + 1
		matches := pyFuncRegex.FindStringSubmatch(line)
		if len(matches) >= 2 {
			name := matches[1]
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			endLine := lineNum

			// Scan until dedent
			for j := i + 1; j < len(lines); j++ {
				curLine := lines[j]
				if strings.TrimSpace(curLine) == "" {
					continue
				}
				curIndent := len(curLine) - len(strings.TrimLeft(curLine, " \t"))
				if curIndent <= indent {
					break
				}
				endLine = j + 1
			}

			fnLines := extractLines(lines, lineNum, endLine)
			cyclo, cogn, depth := CalculateComplexity(fnLines)

			analysis.Functions = append(analysis.Functions, FunctionSymbol{
				Name:                 name,
				Signature:            strings.TrimSpace(line),
				StartLine:            lineNum,
				EndLine:              endLine,
				LinesOfCode:          endLine - lineNum + 1,
				CyclomaticComplexity: cyclo,
				CognitiveComplexity:  cogn,
				NestingDepth:         depth,
				IsExported:           !strings.HasPrefix(name, "_"),
			})
		}
	}
}

func parseGenericFile(lines []string, analysis *FileAnalysis) {
	cyclo, cogn, depth := CalculateComplexity(lines)
	if len(lines) > 0 {
		analysis.Functions = append(analysis.Functions, FunctionSymbol{
			Name:                 filepath.Base(analysis.FilePath),
			Signature:            "generic",
			StartLine:            1,
			EndLine:              len(lines),
			LinesOfCode:          len(lines),
			CyclomaticComplexity: cyclo,
			CognitiveComplexity:  cogn,
			NestingDepth:         depth,
			IsExported:           true,
		})
	}
}

func findMatchingBraceEnd(lines []string, startIndex int) int {
	openCount := 0
	foundOpen := false

	for i := startIndex; i < len(lines); i++ {
		line := lines[i]
		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		if opens > 0 {
			foundOpen = true
		}
		openCount += (opens - closes)
		if foundOpen && openCount <= 0 {
			return i + 1
		}
	}

	if len(lines) > startIndex+30 {
		return startIndex + 30
	}
	return len(lines)
}

func extractLines(lines []string, start, end int) []string {
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end || start > len(lines) {
		return nil
	}
	return lines[start-1 : end]
}

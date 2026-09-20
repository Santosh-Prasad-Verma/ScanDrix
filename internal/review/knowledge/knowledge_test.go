// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package knowledge

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSymbolIndex_GoAST(t *testing.T) {
	idx := NewSymbolIndex()

	goCode := `package auth

import "fmt"

type TokenManager interface {
	ValidateToken(token string) bool
}

type AuthHandler struct {
	secret string
}

func (h *AuthHandler) Login(user, pass string) string {
	if checkCredentials(user, pass) {
		return h.generateToken(user)
	}
	return ""
}

func (h *AuthHandler) generateToken(user string) string {
	return fmt.Sprintf("token-%s", user)
}

func checkCredentials(user, pass string) bool {
	return len(user) > 0 && len(pass) > 0
}
`

	idx.IndexFile("internal/auth/handler.go", goCode)

	defs := idx.GetDefinitionsInFile("internal/auth/handler.go")
	require.NotEmpty(t, defs)

	names := make(map[string]SymbolKind)
	for _, d := range defs {
		names[d.Name] = d.Kind
	}

	assert.Equal(t, KindInterface, names["TokenManager"])
	assert.Equal(t, KindStruct, names["AuthHandler"])
	assert.Equal(t, KindMethod, names["Login"])
	assert.Equal(t, KindMethod, names["generateToken"])
	assert.Equal(t, KindFunction, names["checkCredentials"])

	// Check calls
	callees := idx.FindCallees("Login")
	assert.Contains(t, callees, "checkCredentials")
	assert.Contains(t, callees, "generateToken")

	callers := idx.FindCallers("checkCredentials")
	assert.Contains(t, callers, "Login")
}

func TestSymbolIndex_TypeScript(t *testing.T) {
	idx := NewSymbolIndex()

	tsCode := `export function parseJwt(token: string) {
	return decode(token);
}

export class AuthService {
	login(credentials: any) {
		return parseJwt(credentials.token);
	}
}
`

	idx.IndexFile("src/auth.ts", tsCode)

	defs := idx.GetDefinitionsInFile("src/auth.ts")
	require.NotEmpty(t, defs)

	hasFunc := false
	hasClass := false
	for _, d := range defs {
		if d.Name == "parseJwt" && d.Kind == KindFunction {
			hasFunc = true
		}
		if d.Name == "AuthService" && d.Kind == KindClass {
			hasClass = true
		}
	}
	assert.True(t, hasFunc)
	assert.True(t, hasClass)
}

func TestSymbolIndex_Python(t *testing.T) {
	idx := NewSymbolIndex()

	pyCode := `class SecurityValidator:
    def validate(self, payload):
        return check_hash(payload)

def check_hash(data):
    return hash(data)
`

	idx.IndexFile("app/security.py", pyCode)

	defs := idx.GetDefinitionsInFile("app/security.py")
	require.NotEmpty(t, defs)

	hasFunc := false
	for _, d := range defs {
		if d.Name == "check_hash" {
			hasFunc = true
		}
	}
	assert.True(t, hasFunc)
}

func TestSymbolIndex_ConcurrentAccess(t *testing.T) {
	idx := NewSymbolIndex()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			code := `package worker
func Process() { DoWork() }
func DoWork() {}`
			idx.IndexFile("worker.go", code)
			_ = idx.GetDefinitionsInFile("worker.go")
			_ = idx.FindCallers("DoWork")
			_ = idx.FindCallees("Process")
		}(i)
	}
	wg.Wait()
}

func TestCallGraph_TransitiveAndCycles(t *testing.T) {
	g := NewCallGraph()

	// A -> B -> C -> D
	g.AddEdge(CallGraphEdge{CallerName: "A", CalleeName: "B", CallerFile: "a.go", CalleeFile: "b.go"})
	g.AddEdge(CallGraphEdge{CallerName: "B", CalleeName: "C", CallerFile: "b.go", CalleeFile: "c.go"})
	g.AddEdge(CallGraphEdge{CallerName: "C", CalleeName: "D", CallerFile: "c.go", CalleeFile: "d.go"})

	// Direct callees
	calleesA := g.GetCallees("A")
	require.Len(t, calleesA, 1)
	assert.Equal(t, "B", calleesA[0].CalleeName)

	// Transitive callees of A
	transCalleesA := g.GetTransitiveCallees("A", 5)
	assert.ElementsMatch(t, []string{"B", "C", "D"}, transCalleesA)

	// Transitive callers of D
	transCallersD := g.GetTransitiveCallers("D", 5)
	assert.ElementsMatch(t, []string{"A", "B", "C"}, transCallersD)

	// FindPaths from A to D
	paths := g.FindPaths("A", "D", 5)
	require.Len(t, paths, 1)
	assert.Equal(t, []string{"A", "B", "C", "D"}, paths[0])

	// Introduce cycle: D -> B
	g.AddEdge(CallGraphEdge{CallerName: "D", CalleeName: "B", CallerFile: "d.go", CalleeFile: "b.go"})
	cycles := g.DetectCycles()
	require.NotEmpty(t, cycles)
}

func TestBlastRadiusCalculator_ImpactAnalysis(t *testing.T) {
	idx := NewSymbolIndex()

	dbCode := `package db
func QueryUser(id string) string { return "user" }
`
	serviceCode := `package service
func GetUserProfile(id string) string { return QueryUser(id) }
`
	apiCode := `package api
func HandleUserRequest(id string) string { return GetUserProfile(id) }
`

	idx.IndexFile("db/user.go", dbCode)
	idx.IndexFile("service/profile.go", serviceCode)
	idx.IndexFile("api/handler.go", apiCode)

	graph := BuildFromIndex(idx)
	calc := NewBlastRadiusCalculator()

	changedFiles := []string{"db/user.go"}
	lineRanges := map[string][][2]int{
		"db/user.go": {{1, 3}},
	}

	report := calc.CalculateBlastRadius(changedFiles, lineRanges, idx, graph)
	require.NotNil(t, report)

	assert.ElementsMatch(t, []string{"db/user.go"}, report.DirectlyModifiedFiles)
	assert.Contains(t, report.DirectlyModifiedSymbols, "QueryUser")
	assert.Contains(t, report.Tier1ImpactedSymbols, "GetUserProfile")
	assert.Contains(t, report.Tier1ImpactedFiles, "service/profile.go")
	assert.Contains(t, report.Tier2ImpactedFiles, "api/handler.go")
	assert.GreaterOrEqual(t, report.TotalImpactedFiles, 3)
	assert.NotEqual(t, RiskLow, report.RiskRating)
}

func TestGraphFormatter_MarkdownAndMermaid(t *testing.T) {
	report := &BlastRadiusReport{
		DirectlyModifiedFiles:   []string{"pkg/auth/token.go"},
		DirectlyModifiedSymbols: []string{"SignToken"},
		Tier1ImpactedSymbols:    []string{"LoginHandler"},
		Tier1ImpactedFiles:      []string{"api/login.go"},
		TotalImpactedFiles:      2,
		ImpactScore:             0.35,
		RiskRating:              RiskMedium,
	}

	graph := NewCallGraph()
	graph.AddEdge(CallGraphEdge{CallerName: "LoginHandler", CalleeName: "SignToken"})

	formatter := NewGraphFormatter()

	md := formatter.FormatBlastRadiusMarkdown(report)
	assert.Contains(t, md, "Architectural Blast Radius Analysis")
	assert.Contains(t, md, "MEDIUM RISK")
	assert.Contains(t, md, "SignToken")
	assert.Contains(t, md, "LoginHandler")

	mermaid := formatter.FormatMermaidDiagram(report, graph)
	assert.Contains(t, mermaid, "graph TD")
	assert.Contains(t, mermaid, "SignToken")
	assert.Contains(t, mermaid, "LoginHandler")

	prompt := formatter.FormatForReviewerPrompt(report, 500)
	assert.Contains(t, prompt, "CROSS-FILE KNOWLEDGE GRAPH & CALL CONTEXT")
	assert.Contains(t, prompt, "SignToken")
}

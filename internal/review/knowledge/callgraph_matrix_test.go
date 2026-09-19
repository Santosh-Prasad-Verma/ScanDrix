// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package knowledge

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCallGraphMatrix_MultiLanguageASTExtraction tests symbol indexing and call resolution across Go, TS, Python, and Rust.
func TestCallGraphMatrix_MultiLanguageASTExtraction(t *testing.T) {
	idx := NewSymbolIndex()

	t.Run("Go Complex Architecture AST", func(t *testing.T) {
		goCode := `package payment

import "context"

type PaymentGateway interface {
	Authorize(ctx context.Context, amount int64) (string, error)
	Capture(ctx context.Context, authID string) error
}

type StripeGateway struct {
	apiKey string
}

func (s *StripeGateway) Authorize(ctx context.Context, amount int64) (string, error) {
	if s.validateAmount(amount) {
		return s.postStripeAuth(amount)
	}
	return "", nil
}

func (s *StripeGateway) Capture(ctx context.Context, authID string) error {
	return s.postStripeCapture(authID)
}

func (s *StripeGateway) validateAmount(amt int64) bool {
	return amt > 0
}

func (s *StripeGateway) postStripeAuth(amt int64) (string, error) {
	s.signRequest()
	return "auth_123", nil
}

func (s *StripeGateway) postStripeCapture(id string) error {
	s.signRequest()
	return nil
}

func (s *StripeGateway) signRequest() {
	computeHMAC(s.apiKey)
}

func computeHMAC(key string) string {
	return "hmac_sig"
}
`
		idx.IndexFile("internal/payment/stripe.go", goCode)
		defs := idx.GetDefinitionsInFile("internal/payment/stripe.go")
		require.NotEmpty(t, defs)

		defMap := make(map[string]SymbolKind)
		for _, d := range defs {
			defMap[d.Name] = d.Kind
		}

		assert.Equal(t, KindInterface, defMap["PaymentGateway"])
		assert.Equal(t, KindStruct, defMap["StripeGateway"])
		assert.Equal(t, KindMethod, defMap["Authorize"])
		assert.Equal(t, KindMethod, defMap["Capture"])
		assert.Equal(t, KindMethod, defMap["validateAmount"])
		assert.Equal(t, KindMethod, defMap["postStripeAuth"])
		assert.Equal(t, KindMethod, defMap["postStripeCapture"])
		assert.Equal(t, KindMethod, defMap["signRequest"])
		assert.Equal(t, KindFunction, defMap["computeHMAC"])

		// Verify caller/callee links
		calleesOfAuth := idx.FindCallees("Authorize")
		assert.Contains(t, calleesOfAuth, "validateAmount")
		assert.Contains(t, calleesOfAuth, "postStripeAuth")

		callersOfHMAC := idx.FindCallers("computeHMAC")
		assert.Contains(t, callersOfHMAC, "signRequest")
	})

	t.Run("TypeScript Classes and Async Functions", func(t *testing.T) {
		tsCode := `export interface SessionPayload {
	userId: string;
	roles: string[];
}

export async function verifySession(token: string): Promise<SessionPayload> {
	const raw = decodeToken(token);
	return validateClaims(raw);
}

export class SessionManager {
	async handleLogin(req: any) {
		const token = issueToken(req.user);
		return verifySession(token);
	}
}

function decodeToken(t: string) { return JSON.parse(t); }
function validateClaims(p: any) { return p; }
function issueToken(u: any) { return "jwt." + u; }
`
		idx.IndexFile("src/auth/session.ts", tsCode)
		defs := idx.GetDefinitionsInFile("src/auth/session.ts")
		require.NotEmpty(t, defs)

		names := make(map[string]bool)
		for _, d := range defs {
			names[d.Name] = true
		}

		assert.True(t, names["verifySession"])
		assert.True(t, names["SessionManager"])
	})

	t.Run("Python Object-Oriented Call Graph", func(t *testing.T) {
		pyCode := `class ModelEvaluator:
    def __init__(self, model):
        self.model = model

    def evaluate_batch(self, items):
        scores = []
        for it in items:
            s = self.compute_single_score(it)
            scores.append(s)
        return aggregate_scores(scores)

    def compute_single_score(self, item):
        cleaned = sanitize_input(item)
        return self.model.predict(cleaned)

def sanitize_input(raw):
    return raw.strip()

def aggregate_scores(scores):
    return sum(scores) / max(len(scores), 1)
`
		idx.IndexFile("ml/evaluator.py", pyCode)
		defs := idx.GetDefinitionsInFile("ml/evaluator.py")
		require.NotEmpty(t, defs)

		pyNames := make(map[string]bool)
		for _, d := range defs {
			pyNames[d.Name] = true
		}
		assert.True(t, pyNames["ModelEvaluator"])
		assert.True(t, pyNames["evaluate_batch"])
		assert.True(t, pyNames["compute_single_score"])
		assert.True(t, pyNames["sanitize_input"])
		assert.True(t, pyNames["aggregate_scores"])
	})
}

// TestCallGraphMatrix_DeepTransitiveBlastRadius tests multi-tier caller resolution and risk scoring.
func TestCallGraphMatrix_DeepTransitiveBlastRadius(t *testing.T) {
	cg := NewCallGraph()

	// Construct deep dependency chain:
	// lowLevelStore -> serviceAdapter -> apiHandler -> routerEndpoint -> publicGateway
	edges := []CallGraphEdge{
		{CallerName: "ServiceAdapter", CallerFile: "internal/adapter/service.go", CalleeName: "LowLevelStore", CalleeFile: "internal/store/db.go", IsCrossFile: true},
		{CallerName: "APIHandler", CallerFile: "internal/api/handler.go", CalleeName: "ServiceAdapter", CalleeFile: "internal/adapter/service.go", IsCrossFile: true},
		{CallerName: "RouterEndpoint", CallerFile: "internal/api/router.go", CalleeName: "APIHandler", CalleeFile: "internal/api/handler.go", IsCrossFile: true},
		{CallerName: "PublicGateway", CallerFile: "cmd/gateway/main.go", CalleeName: "RouterEndpoint", CalleeFile: "internal/api/router.go", IsCrossFile: true},
		// Side branch: BackgroundWorker -> LowLevelStore
		{CallerName: "BackgroundWorker", CallerFile: "internal/worker/job.go", CalleeName: "LowLevelStore", CalleeFile: "internal/store/db.go", IsCrossFile: true},
	}

	for _, e := range edges {
		cg.AddEdge(e)
	}

	// 1. Direct callers of LowLevelStore
	directCallers := cg.GetCallers("LowLevelStore")
	assert.Len(t, directCallers, 2)
	callerNames := []string{directCallers[0].CallerName, directCallers[1].CallerName}
	assert.Contains(t, callerNames, "ServiceAdapter")
	assert.Contains(t, callerNames, "BackgroundWorker")

	// 2. Transitive callers of LowLevelStore (should include everything upstream)
	transitive := cg.GetTransitiveCallers("LowLevelStore", 10)
	assert.Contains(t, transitive, "ServiceAdapter")
	assert.Contains(t, transitive, "BackgroundWorker")
	assert.Contains(t, transitive, "APIHandler")
	assert.Contains(t, transitive, "RouterEndpoint")
	assert.Contains(t, transitive, "PublicGateway")

	// 3. Blast radius evaluation
	idx := NewSymbolIndex()
	idx.IndexFile("internal/store/db.go", `package store
func LowLevelStore() {}`)
	idx.IndexFile("internal/adapter/service.go", `package adapter
func ServiceAdapter() {}`)
	idx.IndexFile("internal/worker/job.go", `package worker
func BackgroundWorker() {}`)
	idx.IndexFile("internal/api/handler.go", `package api
func APIHandler() {}`)

	calc := NewBlastRadiusCalculator()
	report := calc.CalculateBlastRadius(
		[]string{"internal/store/db.go"},
		map[string][][2]int{
			"internal/store/db.go": {{1, 10}},
		},
		idx,
		cg,
	)

	assert.Equal(t, []string{"internal/store/db.go"}, report.DirectlyModifiedFiles)
	assert.Contains(t, report.DirectlyModifiedSymbols, "LowLevelStore")
	assert.Contains(t, report.Tier1ImpactedSymbols, "ServiceAdapter")
	assert.Contains(t, report.Tier1ImpactedSymbols, "BackgroundWorker")
	assert.Contains(t, report.Tier1ImpactedFiles, "internal/adapter/service.go")

	// Total impacted files should include direct + tier1
	assert.GreaterOrEqual(t, report.TotalImpactedFiles, 2)
	assert.NotEmpty(t, report.RiskRating)
}

// TestCallGraphMatrix_CyclicDependencyHandling ensures no infinite recursion on circular graphs.
func TestCallGraphMatrix_CyclicDependencyHandling(t *testing.T) {
	cg := NewCallGraph()

	// Mutual recursion: NodeA -> NodeB -> NodeC -> NodeA
	cg.AddEdge(CallGraphEdge{CallerName: "NodeA", CalleeName: "NodeB"})
	cg.AddEdge(CallGraphEdge{CallerName: "NodeB", CalleeName: "NodeC"})
	cg.AddEdge(CallGraphEdge{CallerName: "NodeC", CalleeName: "NodeA"})

	// Self-loop: NodeD -> NodeD
	cg.AddEdge(CallGraphEdge{CallerName: "NodeD", CalleeName: "NodeD"})

	// Detect cycles
	cycles := cg.DetectCycles()
	assert.NotEmpty(t, cycles)

	// Transitive callers with cycle must terminate safely and deduplicate
	transA := cg.GetTransitiveCallers("NodeA", 10)
	assert.Contains(t, transA, "NodeC")
	assert.Contains(t, transA, "NodeB")

	transCalleesA := cg.GetTransitiveCallees("NodeA", 10)
	assert.Contains(t, transCalleesA, "NodeB")
	assert.Contains(t, transCalleesA, "NodeC")

	// Self loop verification via GetCallers
	callersD := cg.GetCallers("NodeD")
	require.NotEmpty(t, callersD)
	assert.Equal(t, "NodeD", callersD[0].CallerName)

	// Path lookup
	paths := cg.FindPaths("NodeA", "NodeC", 5)
	require.NotEmpty(t, paths)
	assert.Equal(t, "NodeA", paths[0][0])
	assert.Equal(t, "NodeC", paths[0][len(paths[0])-1])
}

// TestCallGraphMatrix_DynamicFileIncrementalMutations tests adding, updating, and removing file edges.
func TestCallGraphMatrix_DynamicFileIncrementalMutations(t *testing.T) {
	cg := NewCallGraph()

	// Version 1 of fileA.go
	cg.AddEdge(CallGraphEdge{CallerName: "FuncA1", CallerFile: "pkg/fileA.go", CalleeName: "FuncB", CalleeFile: "pkg/fileB.go"})
	cg.AddEdge(CallGraphEdge{CallerName: "FuncA2", CallerFile: "pkg/fileA.go", CalleeName: "FuncC", CalleeFile: "pkg/fileC.go"})

	callersOfB := cg.GetCallers("FuncB")
	require.Len(t, callersOfB, 1)
	assert.Equal(t, "FuncA1", callersOfB[0].CallerName)

	// Update: fileA.go changes in PR - remove old edges and add new edges
	cg.RemoveFileEdges("pkg/fileA.go")

	// Verify old edges purged
	assert.Empty(t, cg.GetCallers("FuncB"))
	assert.Empty(t, cg.GetCallers("FuncC"))

	// Add Version 2 of fileA.go
	cg.AddEdge(CallGraphEdge{CallerName: "FuncA_New", CallerFile: "pkg/fileA.go", CalleeName: "FuncB", CalleeFile: "pkg/fileB.go"})
	cg.AddEdge(CallGraphEdge{CallerName: "FuncA_New", CallerFile: "pkg/fileA.go", CalleeName: "FuncD", CalleeFile: "pkg/fileD.go"})

	updatedCallersB := cg.GetCallers("FuncB")
	require.Len(t, updatedCallersB, 1)
	assert.Equal(t, "FuncA_New", updatedCallersB[0].CallerName)

	callersD := cg.GetCallers("FuncD")
	require.Len(t, callersD, 1)
	assert.Equal(t, "FuncA_New", callersD[0].CallerName)
}

// TestCallGraphMatrix_CrossRepoDependencyContractMatrix validates multi-repo graph contracts.
func TestCallGraphMatrix_CrossRepoDependencyContractMatrix(t *testing.T) {
	engine := NewLinkedRepoGraphEngine()

	// Register 4 packages across repositories
	engine.RegisterPackage(PackageNode{
		ID:          "pkg-contracts",
		RepoName:    "org/shared-contracts",
		PackagePath: "github.com/org/shared-contracts/user",
		Contracts: []ContractSignature{
			{
				Name:       "UserClaimsV1",
				Package:    "github.com/org/shared-contracts/user",
				Kind:       "STRUCT",
				Fields:     []string{"UserID", "TenantID", "Scope"},
				IsExported: true,
			},
			{
				Name:       "InvoicePlanV1",
				Package:    "github.com/org/shared-contracts/billing",
				Kind:       "STRUCT",
				Fields:     []string{"PlanID", "PriceCents"},
				IsExported: true,
			},
		},
	})

	engine.RegisterPackage(PackageNode{
		ID:          "pkg-auth",
		RepoName:    "org/auth-service",
		PackagePath: "internal/auth",
		DependsOn:   []string{"pkg-contracts"},
		Contracts: []ContractSignature{
			{
				Name:       "ValidateTokenRPC",
				Package:    "internal/auth",
				Kind:       "FUNCTION",
				Params:     []string{"BearerToken"},
				Returns:    []string{"UserClaimsV1", "error"},
				IsExported: true,
			},
		},
	})

	engine.RegisterPackage(PackageNode{
		ID:          "pkg-billing",
		RepoName:    "org/billing-service",
		PackagePath: "internal/billing",
		DependsOn:   []string{"pkg-contracts"},
	})

	engine.RegisterPackage(PackageNode{
		ID:          "pkg-gateway",
		RepoName:    "org/api-gateway",
		PackagePath: "internal/proxy",
		DependsOn:   []string{"pkg-auth"},
	})

	// 1. Analyze blast radius when pkg-contracts changes without contract modifications
	report := engine.AnalyzeCrossRepoBlastRadius(
		context.Background(),
		"org/shared-contracts",
		[]string{"pkg-contracts"},
		nil,
	)

	assert.Contains(t, report.DirectDependents, "pkg-auth")
	assert.Contains(t, report.DirectDependents, "pkg-billing")
	assert.Contains(t, report.TransitiveDependents, "pkg-gateway")
	assert.False(t, report.HasCyclicDependency)
	assert.Greater(t, report.CrossRepoRiskScore, 0.0)

	// 2. Breaking change detection: removal of a required field in UserClaimsV1
	modifiedContracts := []ContractSignature{
		{
			Name:       "UserClaimsV1",
			Package:    "github.com/org/shared-contracts/user",
			Kind:       "STRUCT",
			Fields:     []string{"UserID"}, // TenantID and Scope removed!
			IsExported: true,
		},
	}

	breakingReport := engine.AnalyzeCrossRepoBlastRadius(
		context.Background(),
		"org/shared-contracts",
		[]string{"pkg-contracts"},
		modifiedContracts,
	)

	assert.NotEmpty(t, breakingReport.BreakingChanges)
	hasFieldRemoval := false
	for _, bc := range breakingReport.BreakingChanges {
		if bc.Kind == BreakingFieldRemoval && strings.HasPrefix(bc.SymbolName, "UserClaimsV1.") {
			hasFieldRemoval = true
			break
		}
	}
	assert.True(t, hasFieldRemoval)

	// 3. Add cyclic dependency: pkg-contracts -> pkg-gateway
	engine.RegisterPackage(PackageNode{
		ID:          "pkg-contracts",
		RepoName:    "org/shared-contracts",
		PackagePath: "github.com/org/shared-contracts/user",
		DependsOn:   []string{"pkg-gateway"},
	})

	cycleReport := engine.AnalyzeCrossRepoBlastRadius(
		context.Background(),
		"org/shared-contracts",
		[]string{"pkg-contracts"},
		nil,
	)
	assert.True(t, cycleReport.HasCyclicDependency)
	assert.NotEmpty(t, cycleReport.CycleTrace)
}

// TestCallGraphMatrix_MermaidAndMarkdownFormatting validates visual diagram output.
func TestCallGraphMatrix_MermaidAndMarkdownFormatting(t *testing.T) {
	cg := NewCallGraph()

	cg.AddEdge(CallGraphEdge{
		CallerName:     "CreateOrderHandler",
		CallerFile:     "api/order.go",
		CallerLine:     45,
		CalleeName:     "ProcessPayment",
		CalleeFile:     "services/billing.go",
		IsCrossFile:    true,
		IsCrossPackage: true,
	})

	cg.AddEdge(CallGraphEdge{
		CallerName:     "ProcessPayment",
		CallerFile:     "services/billing.go",
		CallerLine:     112,
		CalleeName:     "AuthorizeCard",
		CalleeFile:     "services/stripe.go",
		IsCrossFile:    true,
	})

	formatter := NewGraphFormatter()

	report := &BlastRadiusReport{
		DirectlyModifiedFiles:   []string{"api/order.go"},
		DirectlyModifiedSymbols: []string{"CreateOrderHandler"},
		Tier1ImpactedSymbols:    []string{"AuditLogger"},
		Tier1ImpactedFiles:      []string{"services/audit.go"},
		TotalImpactedFiles:      3,
		ImpactScore:             0.65,
		RiskRating:              RiskHigh,
		GeneratedAt:             time.Now().UTC(),
	}

	// 1. Markdown summary format
	reportMD := formatter.FormatBlastRadiusMarkdown(report)
	assert.Contains(t, reportMD, "Architectural Blast Radius Analysis")
	assert.Contains(t, reportMD, "HIGH RISK")
	assert.Contains(t, reportMD, "services/audit.go")
	assert.Contains(t, reportMD, "CreateOrderHandler")

	// 2. Mermaid diagram format
	mermaid := formatter.FormatMermaidDiagram(report, cg)
	assert.True(t, strings.HasPrefix(mermaid, "```mermaid\ngraph TD") || strings.Contains(mermaid, "graph TD"))
	assert.Contains(t, mermaid, "CreateOrderHandler")
	assert.Contains(t, mermaid, "AuditLogger")
	assert.True(t, strings.HasSuffix(strings.TrimSpace(mermaid), "```"))

	// 3. Reviewer prompt format
	promptText := formatter.FormatForReviewerPrompt(report, 200)
	assert.Contains(t, promptText, "CROSS-FILE KNOWLEDGE GRAPH & CALL CONTEXT")
	assert.Contains(t, promptText, "CreateOrderHandler")
	assert.Contains(t, promptText, "AuditLogger")
}

// TestCallGraphMatrix_HighThroughputConcurrencyStress verifies thread safety across all operations.
func TestCallGraphMatrix_HighThroughputConcurrencyStress(t *testing.T) {
	cg := NewCallGraph()
	idx := NewSymbolIndex()
	calc := NewBlastRadiusCalculator()

	workers := 16
	opsPerWorker := 100

	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				filePath := fmt.Sprintf("internal/mod_%d/file_%d.go", workerID, i%5)
				funcCaller := fmt.Sprintf("WorkerFunc_%d_%d", workerID, i)
				funcCallee := fmt.Sprintf("TargetFunc_%d_%d", workerID, (i+1)%5)

				// 1. Add edge
				cg.AddEdge(CallGraphEdge{
					CallerName:  funcCaller,
					CallerFile:  filePath,
					CalleeName:  funcCallee,
					CalleeFile:  fmt.Sprintf("internal/mod_%d/file_%d.go", workerID, (i+1)%5),
					IsCrossFile: true,
				})

				// 2. Index symbol
				idx.IndexFile(filePath, fmt.Sprintf("package mod\nfunc %s() { %s() }\nfunc %s() {}", funcCaller, funcCallee, funcCallee))

				// 3. Query
				_ = cg.GetCallers(funcCallee)
				_ = cg.GetCallees(funcCaller)
				_ = cg.GetTransitiveCallers(funcCallee, 3)
				_ = cg.DetectCycles()

				// 4. Calculate blast radius occasionally
				if i%20 == 0 {
					_ = calc.CalculateBlastRadius(
						[]string{filePath},
						map[string][][2]int{filePath: {{1, 10}}},
						idx,
						cg,
					)
				}
			}
		}(w)
	}

	wg.Wait()
}

// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package stages_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/orchestrator"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/review/pipeline/stages"
	"github.com/scandrix/backend/internal/review/priority"
	"github.com/scandrix/backend/pkg/models"
)

func TestBuildOrchestratorInput_ReviewDirectiveForwarding(t *testing.T) {
	directive := "Focus exclusively on authentication, JWT validation, and session cookies."
	pCtx := &pipeline.PipelineContext{
		ReviewDirective: directive,
		PullNumber:      101,
		RepoNamespace:   "scandrix/gateway",
		Title:           "Refactor Auth Interceptor",
	}

	computed := stages.OrchestratorInputComputed{
		PRNumber: 101,
	}

	input := stages.BuildOrchestratorInput(pCtx, computed)

	if !strings.Contains(input.ExternalContext, "### Review Steering Directive:") {
		t.Fatalf("expected directive header in external context, got:\n%s", input.ExternalContext)
	}
	if !strings.Contains(input.ExternalContext, directive) {
		t.Fatalf("expected directive text '%s' in external context, got:\n%s", directive, input.ExternalContext)
	}
}

func TestBuildOrchestratorInput_NoDirective(t *testing.T) {
	pCtx := &pipeline.PipelineContext{
		PullNumber:    102,
		RepoNamespace: "scandrix/worker",
		Title:         "Worker queue concurrency",
	}

	computed := stages.OrchestratorInputComputed{
		PRNumber: 102,
	}

	input := stages.BuildOrchestratorInput(pCtx, computed)

	if strings.Contains(input.ExternalContext, "### Review Steering Directive:") {
		t.Fatalf("expected no directive header when directive is empty, got:\n%s", input.ExternalContext)
	}
}

func TestBuildOrchestratorInput_PromptFieldsMapping(t *testing.T) {
	repoID := uuid.New()
	wsID := uuid.New()

	pCtx := &pipeline.PipelineContext{
		PullNumber:    42,
		Title:         "Add Distributed Rate Limiter",
		Description:   "Implements Redis token bucket limiter for all API routes",
		RepoNamespace: "scandrix/api-gateway",
		BaseSHA:       "base-sha-123456",
		HeadSHA:       "head-sha-abcdef",
		BaseBranch:    "main",
		Branch:        "feat/rate-limiter",
		Author:        "lead-architect",
		RepositoryID:  repoID,
		WorkspaceID:   wsID,
		ResolvedConfig: domain.CodeReviewConfig{
			ReviewMode: "deep",
		},
	}

	computed := stages.OrchestratorInputComputed{}

	input := stages.BuildOrchestratorInput(pCtx, computed)

	if input.PRNumber != 42 {
		t.Errorf("expected PRNumber 42, got %d", input.PRNumber)
	}
	if input.Title != "Add Distributed Rate Limiter" {
		t.Errorf("expected Title 'Add Distributed Rate Limiter', got '%s'", input.Title)
	}
	if input.Description != "Implements Redis token bucket limiter for all API routes" {
		t.Errorf("expected Description match, got '%s'", input.Description)
	}
	if input.RepositoryName != "scandrix/api-gateway" {
		t.Errorf("expected RepositoryName 'scandrix/api-gateway', got '%s'", input.RepositoryName)
	}
	if input.BaseSHA != "base-sha-123456" || input.HeadSHA != "head-sha-abcdef" {
		t.Errorf("SHA mismatch: base=%s head=%s", input.BaseSHA, input.HeadSHA)
	}
	if input.BaseBranch != "main" || input.HeadBranch != "feat/rate-limiter" {
		t.Errorf("Branch mismatch: base=%s head=%s", input.BaseBranch, input.HeadBranch)
	}
	if input.AuthorUsername != "lead-architect" {
		t.Errorf("expected Author 'lead-architect', got '%s'", input.AuthorUsername)
	}
	if input.RepositoryID != repoID || input.WorkspaceID != wsID {
		t.Errorf("UUID mismatch: repo=%s ws=%s", input.RepositoryID, input.WorkspaceID)
	}
	if input.ReviewOptions.ReviewMode != orchestrator.ReviewModeDeep {
		t.Errorf("expected ReviewModeDeep, got %v", input.ReviewOptions.ReviewMode)
	}
}

func TestBuildOrchestratorInput_TraceDecisionsForwarding(t *testing.T) {
	decisions := []pipeline.TraceDecisionInfo{
		{
			ID:          "dec-1",
			DecisionKey: "ADR-004",
			Title:       "Use MurmurHash3 for Sharding",
			Summary:     "Distribute jobs uniformly across 64 ring partitions",
			Rationale:   "Minimizes hot spots under heavy webhook ingress bursts",
			Files:       []string{"pkg/hash/murmur.go", "internal/queue/ring.go"},
		},
		{
			ID:          "dec-2",
			DecisionKey: "ADR-009",
			Title:       "Bounded In-Memory Ring Buffer",
			Summary:     "Cap trace buffer at 10,000 items per tenant",
			Rationale:   "Avoids OutOfMemory killed under noisy multi-tenant load",
			Files:       []string{"internal/review/trace/tracer.go"},
		},
	}

	pCtx := &pipeline.PipelineContext{
		PullNumber:     77,
		RepoNamespace:  "scandrix/core",
		TraceDecisions: decisions,
	}

	computed := stages.OrchestratorInputComputed{PRNumber: 77}
	input := stages.BuildOrchestratorInput(pCtx, computed)

	if !strings.Contains(input.ExternalContext, "### Recorded Architectural Decisions (Trace Context):") {
		t.Fatalf("expected trace decisions header in external context")
	}
	if !strings.Contains(input.ExternalContext, "ADR-004") || !strings.Contains(input.ExternalContext, "ADR-009") {
		t.Errorf("expected both ADR keys in external context, got:\n%s", input.ExternalContext)
	}
	if !strings.Contains(input.ExternalContext, "pkg/hash/murmur.go") {
		t.Errorf("expected file references in trace context")
	}
}

func TestBuildOrchestratorInput_ReviewModes(t *testing.T) {
	modes := []struct {
		configuredMode string
		expectedMode   orchestrator.ReviewMode
	}{
		{"", orchestrator.ReviewModeNormal},
		{"normal", orchestrator.ReviewModeNormal},
		{"fast", orchestrator.ReviewModeFast},
		{"FAST", orchestrator.ReviewModeFast},
		{"deep", orchestrator.ReviewModeDeep},
		{"DEEP", orchestrator.ReviewModeDeep},
		{"unknown_custom", orchestrator.ReviewModeNormal},
	}

	for _, tc := range modes {
		t.Run(fmt.Sprintf("mode_%s", tc.configuredMode), func(t *testing.T) {
			pCtx := &pipeline.PipelineContext{
				ResolvedConfig: domain.CodeReviewConfig{
					ReviewMode: tc.configuredMode,
				},
			}
			input := stages.BuildOrchestratorInput(pCtx, stages.OrchestratorInputComputed{})
			if input.ReviewOptions.ReviewMode != tc.expectedMode {
				t.Errorf("for config '%s', expected mode %v, got %v",
					tc.configuredMode, tc.expectedMode, input.ReviewOptions.ReviewMode)
			}
		})
	}
}

func TestBuildOrchestratorInput_DrixyRulesPrecedence(t *testing.T) {
	orgID := uuid.New()
	rule1 := orchestrator.DrixyRule{
		ID:          uuid.New(),
		OrgID:       orgID,
		Name:        "Context Rule 1",
		Description: "Raw long description from database",
		Prompt:      "Check SQL injection",
		Severity:    models.SeverityHigh,
		IsActive:    true,
	}
	rule2 := orchestrator.DrixyRule{
		ID:          uuid.New(),
		OrgID:       orgID,
		Name:        "Computed Rule 2 (Summary Swapped)",
		Description: "Concise token-optimized AST checklist",
		Prompt:      "WHAT TO VALIDATE:\n- Verify prepared statement parameters",
		Severity:    models.SeverityHigh,
		IsActive:    true,
	}

	pCtx := &pipeline.PipelineContext{
		DrixyRules: []orchestrator.DrixyRule{rule1},
	}

	// 1. Stage computed rules take precedence
	computedWithRules := stages.OrchestratorInputComputed{
		DrixyRules: []orchestrator.DrixyRule{rule2},
	}
	input1 := stages.BuildOrchestratorInput(pCtx, computedWithRules)
	if len(input1.DrixyRules) != 1 || input1.DrixyRules[0].Name != "Computed Rule 2 (Summary Swapped)" {
		t.Errorf("expected computed rule to take precedence, got %+v", input1.DrixyRules)
	}

	// 2. Fallback to context rules when computed is empty
	computedEmpty := stages.OrchestratorInputComputed{}
	input2 := stages.BuildOrchestratorInput(pCtx, computedEmpty)
	if len(input2.DrixyRules) != 1 || input2.DrixyRules[0].Name != "Context Rule 1" {
		t.Errorf("expected fallback to context rule, got %+v", input2.DrixyRules)
	}
}

func TestBuildOrchestratorInput_ChangedFilesPrecedence(t *testing.T) {
	fileContext := pipeline.FileChangeInfo{
		Filename:  "pkg/auth/jwt.go",
		Status:    "modified",
		Additions: 12,
		Deletions: 3,
		Patch:     "@@ -10,3 +10,12 @@\n+func VerifyToken...",
	}

	fileComputed := orchestrator.ChangedFile{
		Filename:  "pkg/auth/jwt_computed.go",
		Status:    "modified",
		Additions: 20,
		Deletions: 5,
		Patch:     "@@ -1,5 +1,20 @@",
	}

	pCtx := &pipeline.PipelineContext{
		ChangedFiles: []pipeline.FileChangeInfo{fileContext},
	}

	// 1. Stage computed files take precedence
	computed := stages.OrchestratorInputComputed{
		ChangedFiles: []orchestrator.ChangedFile{fileComputed},
	}
	input1 := stages.BuildOrchestratorInput(pCtx, computed)
	if len(input1.ChangedFiles) != 1 || input1.ChangedFiles[0].Filename != "pkg/auth/jwt_computed.go" {
		t.Errorf("expected computed file to take precedence, got %+v", input1.ChangedFiles)
	}

	// 2. Fallback to pCtx.ChangedFiles
	computedEmpty := stages.OrchestratorInputComputed{}
	input2 := stages.BuildOrchestratorInput(pCtx, computedEmpty)
	if len(input2.ChangedFiles) != 1 || input2.ChangedFiles[0].Filename != "pkg/auth/jwt.go" {
		t.Errorf("expected fallback to context changed files, got %+v", input2.ChangedFiles)
	}

	// 3. Fallback to pCtx.Files if ChangedFiles is empty
	pCtxFilesOnly := &pipeline.PipelineContext{
		Files: []pipeline.FileChangeInfo{fileContext},
	}
	input3 := stages.BuildOrchestratorInput(pCtxFilesOnly, computedEmpty)
	if len(input3.ChangedFiles) != 1 || input3.ChangedFiles[0].Filename != "pkg/auth/jwt.go" {
		t.Errorf("expected fallback to context files, got %+v", input3.ChangedFiles)
	}
}

func TestBuildOrchestratorInput_CallGraphAndAdaptiveProfile(t *testing.T) {
	graphContent := "```mermaid\ngraph TD\n  AuthInterceptor --> TokenValidator\n```"

	pCtx := &pipeline.PipelineContext{PullNumber: 1}

	// Case 1: Call graph included when profile does NOT drop it
	computedInclude := stages.OrchestratorInputComputed{
		CallGraph: graphContent,
		AdaptiveProfile: priority.AdaptiveProfile{
			DropCallGraph: false,
		},
	}
	input1 := stages.BuildOrchestratorInput(pCtx, computedInclude)
	if !strings.Contains(input1.ExternalContext, "### Call Graph Topology Context:") {
		t.Errorf("expected call graph in external context")
	}
	if !strings.Contains(input1.ExternalContext, "AuthInterceptor --> TokenValidator") {
		t.Errorf("expected graph body in external context")
	}

	// Case 2: Call graph dropped when profile requests DropCallGraph
	computedDrop := stages.OrchestratorInputComputed{
		CallGraph: graphContent,
		AdaptiveProfile: priority.AdaptiveProfile{
			DropCallGraph: true,
		},
	}
	input2 := stages.BuildOrchestratorInput(pCtx, computedDrop)
	if strings.Contains(input2.ExternalContext, "### Call Graph Topology Context:") {
		t.Errorf("expected call graph to be dropped when DropCallGraph is true")
	}
}

func TestBuildOrchestratorInput_NilContextSafety(t *testing.T) {
	computed := stages.OrchestratorInputComputed{
		PRNumber: 88,
		ChangedFiles: []orchestrator.ChangedFile{
			{Filename: "service.go", Status: "modified"},
		},
		ReviewOptions: orchestrator.ReviewOptions{
			MaxTokens:  16000,
			ReviewMode: orchestrator.ReviewModeFast,
		},
	}

	input := stages.BuildOrchestratorInput(nil, computed)
	if input.PRNumber != 88 {
		t.Errorf("expected PRNumber 88, got %d", input.PRNumber)
	}
	if len(input.ChangedFiles) != 1 || input.ChangedFiles[0].Filename != "service.go" {
		t.Errorf("expected computed changed files preserved on nil context")
	}
	if input.ReviewOptions.ReviewMode != orchestrator.ReviewModeFast {
		t.Errorf("expected fast review mode preserved on nil context")
	}
}

func TestBuildOrchestratorInput_ConcurrencyRaceSafety(t *testing.T) {
	pCtx := &pipeline.PipelineContext{
		PullNumber:      999,
		Title:           "Concurrent Stress PR",
		RepoNamespace:   "scandrix/platform",
		ReviewDirective: "Perform security and concurrency audit",
		TraceDecisions: []pipeline.TraceDecisionInfo{
			{DecisionKey: "ADR-001", Title: "Thread Safety Guarantee", Summary: "Read-write locks on registries"},
		},
		DrixyRules: []orchestrator.DrixyRule{
			{ID: uuid.New(), Name: "Rule Alpha", Prompt: "Audit locks", IsActive: true},
		},
		Files: []pipeline.FileChangeInfo{
			{Filename: "mutex.go", Status: "modified", Additions: 5, Deletions: 1},
		},
	}

	var wg sync.WaitGroup
	workers := 30
	iterations := 50

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				computed := stages.OrchestratorInputComputed{
					PRNumber: 999,
					CallGraph: fmt.Sprintf("Graph-W%d-I%d", workerID, i),
					AdaptiveProfile: priority.AdaptiveProfile{
						DropCallGraph: i%2 == 0,
					},
					ParentWarnings: []orchestrator.ReviewWarning{
						{Kind: orchestrator.WarningContextTruncation, Message: fmt.Sprintf("Warning %d", i), AgentName: "security_agent"},
					},
				}

				res := stages.BuildOrchestratorInput(pCtx, computed)
				if res.PRNumber != 999 {
					t.Errorf("unexpected PR number %d", res.PRNumber)
				}
				if !strings.Contains(res.ExternalContext, "Perform security and concurrency audit") {
					t.Errorf("missing directive in concurrency test")
				}
			}
		}(w)
	}

	wg.Wait()
}

// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package aiengine

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

func TestReferenceDetector_Detection(t *testing.T) {
	detector := NewReferenceDetector()

	text := `This PR addresses [SEC-404] and closes #128.
Also mitigates CVE-2026-1122 and tracks ENG-88.
Reference documentation at https://scandrix.dev/docs/security`

	refs := detector.DetectReferences(text)

	if len(refs) != 5 {
		t.Fatalf("expected 5 detected references, got %d: %+v", len(refs), refs)
	}

	kinds := make(map[ReferenceKind]bool)
	for _, r := range refs {
		kinds[r.Kind] = true
	}

	if !kinds[RefKindJira] {
		t.Errorf("expected Jira ticket SEC-404")
	}
	if !kinds[RefKindGitHub] {
		t.Errorf("expected GitHub issue #128")
	}
	if !kinds[RefKindCVE] {
		t.Errorf("expected CVE-2026-1122")
	}
	if !kinds[RefKindLinear] {
		t.Errorf("expected Linear ticket ENG-88")
	}
	if !kinds[RefKindWebURL] {
		t.Errorf("expected external URL")
	}

	md := detector.FormatContextMarkdown(refs)
	if !strings.Contains(md, "CVE-2026-1122") || !strings.Contains(md, "SEC-404") {
		t.Errorf("expected markdown to contain formatted references: %s", md)
	}
}

func TestContextPackAssembler_AssemblyAndTrimming(t *testing.T) {
	// 1. Normal assembly within budget
	assembler := NewContextPackAssembler(64000)

	pCtx := &pipeline.PipelineContext{
		PullNumber:  10,
		Title:       "Add auth module",
		Description: "Implements JWT validator",
		TraceDecisions: []pipeline.TraceDecisionInfo{
			{DecisionKey: "ADR-001", Title: "Auth", Summary: "Use JWT"},
		},
		ChangedFiles: []pipeline.FileChangeInfo{
			{Filename: "pkg/auth/jwt.go", Additions: 30, Deletions: 2, Patch: "@@ -1,2 +1,4 @@\n+token := jwt.New()"},
		},
	}

	pack := assembler.AssemblePack(pCtx)

	if pack.DigestToken == "" {
		t.Fatalf("expected non-empty digest token")
	}
	if pack.WasTrimmed {
		t.Errorf("expected pack to not be trimmed within large budget")
	}
	if len(pack.Layers) < 3 {
		t.Errorf("expected at least 3 layers (persona, ADRs, diff), got %d", len(pack.Layers))
	}

	// 2. Budget constraint triggering adaptive trimming
	tightAssembler := NewContextPackAssembler(50) // Extremely tight token budget (50 tokens = ~200 bytes)
	tightPack := tightAssembler.AssemblePack(pCtx)

	if !tightPack.WasTrimmed {
		t.Errorf("expected tight pack to trigger adaptive trimming")
	}
	if tightPack.TrimmedBytes <= 0 {
		t.Errorf("expected trimmed bytes to be positive, got %d", tightPack.TrimmedBytes)
	}
}

func TestLLMResponseProcessor_ProcessingAndRepair(t *testing.T) {
	processor := NewLLMResponseProcessor()
	reviewID := uuid.New()
	workspaceID := uuid.New()

	// 1. Standard markdown code fence with valid JSON array
	validFence := "```json\n" + `[
  {
    "file_path": "pkg/auth/token.go",
    "start_line": 42,
    "end_line": 45,
    "severity": "CRITICAL",
    "category": "SECURITY",
    "title": "Hardcoded JWT Signing Secret",
    "description": "Never commit secret keys in source files",
    "remediation": "Load from environment variables"
  }
]` + "\n```"

	res1 := processor.ProcessCompletion(validFence, reviewID, workspaceID)
	if res1.Error != nil {
		t.Fatalf("unexpected error parsing valid fence: %v", res1.Error)
	}
	if len(res1.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(res1.Findings))
	}
	if res1.Findings[0].Severity != models.SeverityCritical {
		t.Errorf("expected CRITICAL severity, got %v", res1.Findings[0].Severity)
	}

	// 2. Wrapper object with "findings" key
	wrapperJSON := `{
  "summary": "Completed review",
  "findings": [
    {
      "file_path": "pkg/db/user.go",
      "start_line": 10,
      "end_line": 10,
      "severity": "HIGH",
      "title": "Unbounded Query"
    }
  ]
}`
	res2 := processor.ProcessCompletion(wrapperJSON, reviewID, workspaceID)
	if res2.Error != nil || len(res2.Findings) != 1 {
		t.Fatalf("expected 1 finding from wrapper object, got %d (err: %v)", len(res2.Findings), res2.Error)
	}

	// 3. Malformed JSON with trailing commas and unclosed brackets
	malformed := `[
  {
    "file_path": "pkg/api/client.go",
    "start_line": 5,
    "end_line": 8,
    "severity": "MEDIUM",
    "title": "Missing Context Timeout",
  },
` // Truncated mid-response with trailing comma and unclosed array!

	res3 := processor.ProcessCompletion(malformed, reviewID, workspaceID)
	if res3.Error != nil {
		t.Fatalf("expected repair to salvage malformed JSON: %v", res3.Error)
	}
	if !res3.WasRepaired {
		t.Errorf("expected WasRepaired to be true")
	}
	if len(res3.Findings) != 1 {
		t.Fatalf("expected 1 finding salvaged from truncated JSON, got %d", len(res3.Findings))
	}
}

func TestAIReviewEngine_ConcurrentStress(t *testing.T) {
	detector := NewReferenceDetector()
	assembler := NewContextPackAssembler(10000)
	processor := NewLLMResponseProcessor()

	var wg sync.WaitGroup
	workers := 25

	for w := 0; w < workers; w++ {
		wg.Add(3)

		// Goroutine 1: Detect references
		go func(id int) {
			defer wg.Done()
			_ = detector.DetectReferences(fmt.Sprintf("Fixes [PROJ-%d] and #%d", id, id))
		}(w)

		// Goroutine 2: Assemble pack
		go func(id int) {
			defer wg.Done()
			pCtx := &pipeline.PipelineContext{
				PullNumber: id,
				Title:      fmt.Sprintf("PR %d", id),
			}
			_ = assembler.AssemblePack(pCtx)
		}(w)

		// Goroutine 3: Process completion
		go func(id int) {
			defer wg.Done()
			payload := fmt.Sprintf(`[{"file_path":"f%d.go","start_line":%d,"title":"Issue","severity":"LOW"}]`, id, id)
			_ = processor.ProcessCompletion(payload, uuid.New(), uuid.New())
		}(w)
	}

	wg.Wait()
}

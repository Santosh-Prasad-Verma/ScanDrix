package prompts

import (
	"strings"
	"testing"
)

func TestPromptCodeReviewSystemGeminiV2_Default(t *testing.T) {
	payload := CodeReviewPayload{
		LanguageResultPrompt: "es-ES",
	}

	prompt := PromptCodeReviewSystemGeminiV2(payload)

	if !strings.Contains(prompt, "ScanDrix Bug-Hunter") && !strings.Contains(prompt, "Drixy Bug-Hunter") {
		t.Errorf("Expected prompt to contain Bug-Hunter identity, got:\n%s", prompt[:200])
	}
	if !strings.Contains(prompt, "es-ES") {
		t.Errorf("Expected prompt to specify language es-ES")
	}
	if !strings.Contains(prompt, "## Core Method: Mental Simulation") {
		t.Errorf("Expected prompt to include mental simulation section")
	}
	if !strings.Contains(prompt, "### BUG") || !strings.Contains(prompt, "### PERFORMANCE") || !strings.Contains(prompt, "### SECURITY") {
		t.Errorf("Expected prompt to include bug, performance, and security categories")
	}
	if !strings.Contains(prompt, "**CRITICAL**") || !strings.Contains(prompt, "**HIGH**") {
		t.Errorf("Expected prompt to include severity tiers")
	}
}

func TestPromptCodeReviewSystemGeminiV2_WithExternalContext(t *testing.T) {
	payload := CodeReviewPayload{
		LanguageResultPrompt: "en-US",
		Memories: []MemoryItem{
			{Title: "Avoid raw queries", Rule: "Use parameter bindings everywhere"},
		},
		DocumentationContext: []DocumentationContext{
			{Title: "Go Concurrency", URL: "https://golang.org/doc", Content: "Use sync.Mutex"},
		},
		TraceDecisions: []TraceDecision{
			{Decision: "Use buffered channels", Rationale: "Prevent goroutine leaks", RuleID: "trace-1"},
		},
		CrossFileSnippets: []CrossFileSnippet{
			{FilePath: "pkg/service.go", Rationale: "Consumer of handler", Content: "func Call()"},
		},
		ContextAugmentations: []ContextAugmentation{
			{Key: "categories.descriptions.bug", Title: "Static Analysis", Content: "Nil pointer detected"},
		},
	}

	prompt := PromptCodeReviewSystemGeminiV2(payload)

	if !strings.Contains(prompt, "Avoid raw queries") {
		t.Errorf("Expected prompt to include memories")
	}
	if !strings.Contains(prompt, "Go Concurrency") {
		t.Errorf("Expected prompt to include documentation context")
	}
	if !strings.Contains(prompt, "Use buffered channels") {
		t.Errorf("Expected prompt to include trace decisions")
	}
	if !strings.Contains(prompt, "pkg/service.go") {
		t.Errorf("Expected prompt to include cross-file snippets")
	}
	if !strings.Contains(prompt, "## External Context & Injected Knowledge") {
		t.Errorf("Expected prompt to include external context header")
	}
}

func TestPromptCodeReviewUserGeminiV2(t *testing.T) {
	payload := CodeReviewPayload{
		PRSummary:         "Fix concurrency deadlock in worker queue",
		RelevantContent:   "package worker\n\nfunc Run() {}",
		PatchWithLinesStr: "12 + lock.Lock()\n13 + defer lock.Unlock()",
	}

	userPrompt := PromptCodeReviewUserGeminiV2(payload)

	if !strings.Contains(userPrompt, "Fix concurrency deadlock in worker queue") {
		t.Errorf("Expected user prompt to include PR summary")
	}
	if !strings.Contains(userPrompt, "package worker") {
		t.Errorf("Expected user prompt to include relevant content")
	}
	if !strings.Contains(userPrompt, "defer lock.Unlock()") {
		t.Errorf("Expected user prompt to include patch lines")
	}
}

func TestPromptCodeReviewSystemGemini_V1(t *testing.T) {
	payload := CodeReviewPayload{
		LanguageResultPrompt: "en-US",
		Memories: []MemoryItem{
			{Title: "Format memory", Rule: "Use strict schema"},
		},
	}

	prompt := PromptCodeReviewSystemGemini(payload)

	if !strings.Contains(prompt, "PR-Reviewer: Code Analysis System") {
		t.Errorf("Expected prompt v1 header")
	}
	if !strings.Contains(prompt, "Format memory") {
		t.Errorf("Expected prompt to include memory item")
	}
}

func TestPromptCodeReviewUserGemini_V1(t *testing.T) {
	payload := CodeReviewPayload{
		FileContent:       "const x = 10;",
		PatchWithLinesStr: "1 + const x = 20;",
	}

	res := PromptCodeReviewUserGemini(payload)
	if !strings.Contains(res, "const x = 10;") || !strings.Contains(res, "const x = 20;") {
		t.Errorf("Expected user prompt to contain file content and patch lines")
	}
}

func TestPromptCodeReviewSystemMain_And_UserMain(t *testing.T) {
	sys := PromptCodeReviewSystemMain()
	if !strings.Contains(sys, "PR-Reviewer") || !strings.Contains(sys, "security") {
		t.Errorf("Expected system prompt to contain review categories")
	}

	payload := CodeReviewPayload{
		LimitationType:       "file",
		MaxSuggestionsParams: 5,
		LanguageResultPrompt: "pt-BR",
	}
	user := PromptCodeReviewUserMain(payload)
	if !strings.Contains(user, "Provide up to 5 code suggestions") {
		t.Errorf("Expected user prompt to include max suggestions limit")
	}
	if !strings.Contains(user, "pt-BR") {
		t.Errorf("Expected user prompt to include language pt-BR")
	}
}

func TestPromptCodeReviewUserTool(t *testing.T) {
	payload := map[string]interface{}{
		"files": []string{"main.go", "util.go"},
	}
	res := PromptCodeReviewUserTool(payload)
	if !strings.Contains(res, "## Code Review") || !strings.Contains(res, "main.go") {
		t.Errorf("Expected tool prompt to contain review template and payload content")
	}
}

func TestSeverityAnalysisUser(t *testing.T) {
	suggestions := []SuggestionSeverityInput{
		{
			ID:                "sugg-1",
			RelevantFile:      "auth.go",
			SuggestionContent: "SQL injection vulnerability in raw query",
			ExistingCode:      "db.Query(fmt.Sprintf(\"SELECT * FROM users WHERE id = %s\", id))",
			ImprovedCode:      "db.Query(\"SELECT * FROM users WHERE id = ?\", id)",
			Label:             "security",
		},
	}

	prompt := PromptSeverityAnalysisUser(suggestions)
	if !strings.Contains(prompt, "# Code Review Severity Analyzer") {
		t.Errorf("Expected severity analyzer header")
	}
	if !strings.Contains(prompt, "sugg-1") || !strings.Contains(prompt, "SQL injection") {
		t.Errorf("Expected suggestion content inside prompt")
	}
	if !strings.Contains(prompt, "CRITICAL FLAGS") || !strings.Contains(prompt, "HIGH FLAGS") {
		t.Errorf("Expected severity flag descriptions")
	}
}

func TestDocumentationPlannerAndFormatter(t *testing.T) {
	planSys := PromptCodeReviewDocumentationPlannerSystem()
	if !strings.Contains(planSys, "expert software documentation planner") {
		t.Errorf("Expected documentation planner system prompt")
	}

	planPayload := DocPlannerPayload{
		Packages: []DocPackageDependency{
			{Name: "gorm.io/gorm", Version: "1.25.0", Ecosystem: "golang", SourceFile: "go.mod"},
		},
		File: DocPlannerFilePayload{
			FilePath:    "repo/user.go",
			Language:    "go",
			FileContent: "package repo\n",
			Diff:        "+ db.Preload(\"Profile\").Find(&users)",
		},
	}
	planUser := PromptCodeReviewDocumentationPlannerUser(planPayload)
	if !strings.Contains(planUser, "gorm.io/gorm@1.25.0") || !strings.Contains(planUser, "repo/user.go") {
		t.Errorf("Expected documentation planner user prompt to contain package and file info")
	}

	formatSys := PromptCodeReviewDocumentationFormatterSystem()
	if !strings.Contains(formatSys, "documentation distillation assistant") {
		t.Errorf("Expected documentation formatter system prompt")
	}

	formatUser := PromptCodeReviewDocumentationFormatterUser(DocumentationFormatterInput{
		PackageName:      "gorm.io/gorm",
		Query:            "How to preload associations",
		RawSearchContent: "Preload loads associations eagerly...",
	})
	if !strings.Contains(formatUser, "gorm.io/gorm") || !strings.Contains(formatUser, "Preload loads associations eagerly") {
		t.Errorf("Expected documentation formatter user prompt to contain query and content")
	}
}

func TestSafeguardVerificationPrompt(t *testing.T) {
	params := SafeguardVerificationParams{
		SuggestionContent:    "Missing mutex lock causes race condition",
		ClaimedDefectType:    "Race condition",
		ExistingCode:         "counter++",
		FilePath:             "pkg/counter.go",
		LanguageResultPrompt: "en-US",
	}

	prompt := PromptCodeReviewSafeguardAgentVerification(params)
	if !strings.Contains(prompt, "STRICT BUDGET of 4 tool calls") {
		t.Errorf("Expected tool budget specification")
	}
	if !strings.Contains(prompt, "counter++") || !strings.Contains(prompt, "pkg/counter.go") {
		t.Errorf("Expected code and file path in safeguard prompt")
	}
	if !strings.Contains(prompt, "{\"tool\": \"search\"") {
		t.Errorf("Expected tool definitions")
	}
}

func TestSemanticsValidationPrompt(t *testing.T) {
	payload := ValidateCodeSemanticsPayload{
		Code:     "func foo() { x := 1\n return x }",
		FilePath: "foo.go",
		Language: "go",
		Diff:     "+ return x",
	}

	prompt := PromptValidateCodeSemanticsFull(payload)
	if !strings.Contains(prompt, "identifying \"code breakage\" issues") {
		t.Errorf("Expected semantic validator task description")
	}
	if !strings.Contains(prompt, "foo.go") || !strings.Contains(prompt, "func foo()") {
		t.Errorf("Expected file path and code in prompt")
	}
}

func TestImplementedValidationPrompt(t *testing.T) {
	payload := ValidateImplementedSuggestionsPayload{
		CodePatch: "+ const timeout = 5000;",
		CodeSuggestions: []SuggestionSeverityInput{
			{
				ID:           "s-1",
				RelevantFile: "client.js",
				ImprovedCode: "const timeout = 5000;",
			},
		},
	}

	prompt := PromptValidateImplementedSuggestions(payload)
	if !strings.Contains(prompt, "matching implemented code review suggestions") {
		t.Errorf("Expected implemented suggestions task description")
	}
	if !strings.Contains(prompt, "const timeout = 5000;") {
		t.Errorf("Expected patch content in prompt")
	}
}

func TestClusteringPrompt(t *testing.T) {
	prompt := PromptRepeatedSuggestionClusteringSystem("en-US")
	if !strings.Contains(prompt, "identify repeated suggestions") {
		t.Errorf("Expected clustering prompt instructions")
	}
	if !strings.Contains(prompt, "sameSuggestionsId") {
		t.Errorf("Expected sameSuggestionsId schema requirement")
	}
	// Verify zero brand leaks!
	if strings.Contains(strings.ToLower(prompt), "kod"+"us") || strings.Contains(strings.ToLower(prompt), "kod"+"y") {
		t.Errorf("Brand leak detected in clustering prompt!")
	}
	if !strings.Contains(prompt, "drixy_rules") {
		t.Errorf("Expected drixy_rules label in clustering prompt")
	}
}

func TestDrixyMemoryResolutionAndSimplicity(t *testing.T) {
	memSys := PromptDrixyMemoryResolutionSystem()
	if !strings.Contains(memSys, "memory curator for engineering preferences") {
		t.Errorf("Expected memory resolution system prompt")
	}

	memUser := PromptDrixyMemoryResolutionUser(DrixyMemoryResolutionPayload{
		IncomingMemory: DrixyMemoryItem{Title: "New Memory", Rule: "Rule text"},
		ExistingMemories: []DrixyMemoryItem{
			{Title: "Existing Memory", Rule: "Existing rule"},
		},
	})
	if !strings.Contains(memUser, "New Memory") || !strings.Contains(memUser, "Existing Memory") {
		t.Errorf("Expected memory resolution user prompt to contain memories")
	}

	simpSys := PromptCheckSuggestionSimplicitySystem()
	if !strings.Contains(simpSys, "determine if it is \"simple\" and safe to apply") {
		t.Errorf("Expected simplicity check system prompt")
	}

	simpUser := PromptCheckSuggestionSimplicityUser(SimplicityCheckPayload{
		Language:     "go",
		ExistingCode: "x := 1",
		ImprovedCode: "x := 2",
	})
	if !strings.Contains(simpUser, "Original Code:") || !strings.Contains(simpUser, "Improved Code:") {
		t.Errorf("Expected simplicity check user prompt format")
	}

	recSys := PromptDrixyRulesRecommendationSystem()
	if !strings.Contains(recSys, "recommend Drixy Rules from the library") {
		t.Errorf("Expected rules recommendation system prompt")
	}
	if strings.Contains(strings.ToLower(recSys), "kod"+"us") || strings.Contains(strings.ToLower(recSys), "kod"+"y") {
		t.Errorf("Brand leak detected in rules recommendation prompt!")
	}
}

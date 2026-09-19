package prompts

import (
	"strings"
	"testing"
)

func TestCodeReviewPrompts(t *testing.T) {
	sysMain := PromptCodeReviewSystemMain()
	if !strings.Contains(sysMain, "Drixy PR-Reviewer") {
		t.Fatalf("expected Drixy PR-Reviewer in system prompt")
	}

	userMain := PromptCodeReviewUserMain(CodeReviewPayload{
		LimitationType:       "file",
		MaxSuggestionsParams: 5,
		LanguageResultPrompt: "en-US",
	})
	if !strings.Contains(userMain, "Provide up to 5 code suggestions") {
		t.Fatalf("expected 5 suggestions limit in user prompt")
	}

	geminiV2Sys := PromptCodeReviewSystemGeminiV2(CodeReviewPayload{
		LanguageResultPrompt: "en-US",
		Memories: []MemoryItem{
			{Title: "No console.log", Content: "Never allow console.log in production"},
		},
		CrossFileSnippets: []CrossFileSnippet{
			{FilePath: "pkg/api.go", Rationale: "Consumer of removed endpoint", Content: "api.Call()"},
		},
	})
	if !strings.Contains(geminiV2Sys, "### Codebase Context (REAL CODE — treat as visible evidence)") {
		t.Fatalf("expected codebase context section in gemini v2 prompt")
	}
	if !strings.Contains(geminiV2Sys, "No console.log") {
		t.Fatalf("expected memory in external context section")
	}
}

func TestDrixyRulesPrompts(t *testing.T) {
	classSys := PromptDrixyRulesClassifierSystem()
	if !strings.Contains(classSys, "Alice, Bob, and Charles") || !strings.Contains(classSys, "drixyRules") {
		t.Fatalf("unexpected classifier system prompt: %s", classSys)
	}

	guardianSys := PromptDrixyRulesGuardianSystem()
	if !strings.Contains(guardianSys, "DrixyGuardian") {
		t.Fatalf("unexpected guardian system prompt")
	}

	extractIDSys := PromptDrixyRulesExtractIDSystem()
	if !strings.Contains(extractIDSys, "UUID v4 format") {
		t.Fatalf("unexpected extract ID system prompt")
	}
}

func TestSafeguardPrompts(t *testing.T) {
	safeSys := PromptCodeReviewSafeguardSystem(SafeguardParams{
		LanguageResultPrompt: "en-US",
		Memories: []MemoryItem{
			{Title: "Strict Auth", Content: "Always enforce bearer token check"},
		},
	})
	if !strings.Contains(safeSys, "Edward (Special Cases Guardian)") {
		t.Fatalf("expected Edward in safeguard prompt")
	}
	if !strings.Contains(safeSys, "Strict Auth") {
		t.Fatalf("expected memories in safeguard prompt")
	}
}

func TestCrossFileAndExtraPrompts(t *testing.T) {
	plannerSys := PromptCrossFileContextPlannerSystem()
	if !strings.Contains(plannerSys, "cross-file context planner") {
		t.Fatalf("unexpected planner prompt")
	}

	matcherSys := PromptDrixyIssuesMergeSuggestionsIntoIssuesSystem()
	if !strings.Contains(matcherSys, "Drixy‐Matcher") {
		t.Fatalf("unexpected matcher prompt")
	}

	catSys := CommentCategorizerSystemPrompt()
	if !strings.Contains(catSys, "code review suggestion categorization expert") {
		t.Fatalf("unexpected comment categorizer prompt")
	}

	featSys := CodeReviewSafeguardFeatureExtractionPrompt("en-US")
	if !strings.Contains(featSys, "has_resource_leak") || !strings.Contains(featSys, "has_unsafe_data_flow") {
		t.Fatalf("unexpected safeguard feature prompt")
	}

	refSys := DetectExternalReferencesSystemPrompt()
	if !strings.Contains(refSys, "identify file references") {
		t.Fatalf("unexpected reference prompt")
	}
}

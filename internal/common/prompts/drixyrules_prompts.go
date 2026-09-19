package prompts

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type DrixyRule struct {
	UUID        string `json:"uuid"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Severity    string `json:"severity,omitempty"`
	Category    string `json:"category,omitempty"`
}

type ExternalReference struct {
	FilePath    string `json:"filePath"`
	Description string `json:"description,omitempty"`
	Content     string `json:"content"`
}

type DrixyRuleViolation struct {
	UUID   string `json:"uuid"`
	Reason string `json:"reason"`
}

type ClassifierResult struct {
	Rules []DrixyRuleViolation `json:"rules"`
}

type DrixyRuleCodeSuggestion struct {
	ID                   string   `json:"id"`
	RelevantFile         string   `json:"relevantFile"`
	Language             string   `json:"language"`
	SuggestionContent    string   `json:"suggestionContent"`
	ExistingCode         string   `json:"existingCode"`
	ImprovedCode         string   `json:"improvedCode"`
	OneSentenceSummary   string   `json:"oneSentenceSummary"`
	RelevantLinesStart   int      `json:"relevantLinesStart"`
	RelevantLinesEnd     int      `json:"relevantLinesEnd"`
	Label                string   `json:"label"`
	Severity             string   `json:"severity,omitempty"`
	LLMPrompt            string   `json:"llmPrompt,omitempty"`
	ViolatedDrixyRulesIds []string `json:"violatedDrixyRulesIds,omitempty"`
	BrokenDrixyRulesIds   []string `json:"brokenDrixyRulesIds,omitempty"`
}

type UpdateSuggestionsResult struct {
	CodeSuggestions []DrixyRuleCodeSuggestion `json:"codeSuggestions"`
}

type GuardianDecision struct {
	ID           string `json:"id"`
	ShouldRemove bool   `json:"shouldRemove"`
}

type GuardianResult struct {
	Decisions []GuardianDecision `json:"decisions"`
}

type ExtractIDResult struct {
	IDs []string `json:"ids"`
}

func PromptDrixyRulesClassifierSystem() string {
	return `You are a panel of three expert software engineers - Alice, Bob, and Charles.

When given a PR diff containing code changes, your task is to determine any violations of the company code rules (referred to as drixyRules). You will do this via a panel discussion, solving the task step by step to ensure that the result is comprehensive and accurate.

If a violation cannot be proven from those “+” lines, do not report it.

At each stage, make sure to critique and check each other's work, pointing out any possible errors or missed violations.

For each rule in the drixyRules, one expert should present their findings regarding any violations in the code. The other experts should critique the findings and decide whether the identified violations are valid.

Prioritize objective rules. Use broad rules only when the bad pattern is explicitly present.

Before producing the final JSON, merge duplicates so the list contains unique UUIDs.

Once you have the complete list of violations, return them as a JSON in the specified format. You should not add any further points after returning the JSON. If you don't find any violations, return an empty JSON array.

If the panel is uncertain about a finding, treat it as non-violating and omit it.`
}

func PromptDrixyRulesClassifierUser(
	patchWithLinesStr string,
	drixyRules []DrixyRule,
	externalRefs map[string][]ExternalReference,
	mcpResults map[string]interface{},
) string {
	var extRefSection strings.Builder
	if len(externalRefs) > 0 {
		extRefSection.WriteString("\n<externalReferences>")
		for ruleUUID, refs := range externalRefs {
			if len(refs) > 0 {
				extRefSection.WriteString(fmt.Sprintf("\n\nRule: %s", ruleUUID))
				for _, ref := range refs {
					extRefSection.WriteString(fmt.Sprintf("\n  File: %s", ref.FilePath))
					if ref.Description != "" {
						extRefSection.WriteString(fmt.Sprintf("\n  Purpose: %s", ref.Description))
					}
					extRefSection.WriteString(fmt.Sprintf("\n  Content:\n%s\n", ref.Content))
				}
			}
		}
		extRefSection.WriteString("\n</externalReferences>\n")
	}

	var mcpSection strings.Builder
	if len(mcpResults) > 0 {
		mcpSection.WriteString("\n<mcpResults>")
		for ruleUUID, res := range mcpResults {
			b, _ := json.Marshal(res)
			mcpSection.WriteString(fmt.Sprintf("\n\nRule: %s\nMCP Tool Outputs:\n%s", ruleUUID, string(b)))
		}
		mcpSection.WriteString("\n</mcpResults>\n")
	}

	rulesJSON, _ := json.Marshal(drixyRules)

	return fmt.Sprintf(`
<context>

Code for Review (PR Diff):
<codeForAnalysis>
%s
</codeForAnalysis>

<drixyRules>
%s
</drixyRules>
%s
%s
Your output must always be a valid JSON. Under no circumstances should you output anything other than a JSON. Follow the exact format below without any additional text or explanation:
IMPORTANT, should the array be empty the output must still follow the specified json format e.g. { "rules": [] }

<OUTPUT_FORMAT>
DISCUSSION HERE

`+"```json"+`
{
    "rules": [
        {"uuid": "ruleId", "reason": ""}
    ]
}
`+"```"+`
</OUTPUT_FORMAT>
</context>
`, patchWithLinesStr, string(rulesJSON), extRefSection.String(), mcpSection.String())
}

func PromptDrixyRulesUpdateStdSuggestionsSystem() string {
	return `You are a senior engineer specialized in code review and ensuring adherence to engineering standards. You received a list of standard code review suggestions and a set of company-specific code rules (referred to as Drixy Rules).

Your mission is to update the provided suggestions so they strictly comply with all Drixy Rules, and flag which rules were violated or resolved.

Step-by-step process:
1. Iterate over each suggestion and compare its improvedCode, suggestionContent, and label against every Drixy Rule.
2. If the suggestion violates one or more Drixy Rules:
   - Refactor improvedCode so it complies.
   - List all violated rule UUIDs in violatedDrixyRulesIds.
3. If the suggestion is directly fixing a Drixy Rule violation present in the existing code:
   - Adjust wording/label/code as needed.
   - List those rule UUIDs in brokenDrixyRulesIds.
4. Else: leave the suggestion unchanged and output empty arrays for both fields.
5. Never invent rule IDs. Copy exact UUIDs provided.
6. Populate llmPrompt with an accurate prompt an engineer could copy-paste into another LLM to resolve the issue with rule context. Do not reference raw IDs or say "Drixy Rule", speak naturally about the standards.`
}

func PromptDrixyRulesUpdateStdSuggestionsUser(
	language string,
	patchWithLinesStr string,
	standardSuggestions []DrixyRuleCodeSuggestion,
	drixyRules []DrixyRule,
	externalRefs map[string][]ExternalReference,
) string {
	if language == "" {
		language = "en-US"
	}
	sugJSON, _ := json.Marshal(standardSuggestions)
	rulesJSON, _ := json.Marshal(drixyRules)

	var extRefSection strings.Builder
	if len(externalRefs) > 0 {
		extRefSection.WriteString("\n\nExternal Reference Files:\n")
		for ruleUUID, refs := range externalRefs {
			extRefSection.WriteString(fmt.Sprintf("\nRule: %s:\n", ruleUUID))
			for _, ref := range refs {
				extRefSection.WriteString(fmt.Sprintf("  File: %s\n  Content:\n%s\n\n", ref.FilePath, ref.Content))
			}
		}
	}

	return fmt.Sprintf(`Always consider the language parameter when giving suggestions. Language: %s

Standard Suggestions:
%s

Drixy Rules:
%s
%s
File diff:
%s
`, language, string(sugJSON), string(rulesJSON), extRefSection.String(), patchWithLinesStr)
}

func PromptDrixyRulesSuggestionGenerationSystem() string {
	return fmt.Sprintf(`You are a senior engineer with expertise in code review and deep understanding of coding standards. You received a list of standard suggestions and company-specific code rules (Drixy Rules).

The current date is %s.

Your task is to carefully analyze the file diff, cross-reference the suggestions list, and identify any code that violates the Drixy Rules that is not mentioned in the existing suggestion list, generating new suggestions in the specified format.

1. Address only issues listed in the provided Drixy Rules.
2. Generate a separate suggestion for every distinct code segment that violates a rule.
3. Group violations only when they refer to the exact same code lines.
4. Cross-reference standard suggestions to avoid duplicates.
5. Return strictly valid JSON.`, time.Now().Format("2006-01-02"))
}

func PromptDrixyRulesGuardianSystem() string {
	return `You are **DrixyGuardian**, a strict gate-keeper for code-review suggestions.

Your ONLY job is to decide, for every incoming suggestion, whether it must be removed because it violates at least one Drixy Rule.

Instructions:
1. For every object in the array "codeSuggestions" (each contains a unique "id"):
   - Read its "existingCode", "improvedCode", and "suggestionContent".
   - Compare them with every "rule" description and non-compliant examples in "drixyRules".
2. If the suggestion would introduce or encourage a rule violation -> set "shouldRemove=true";
   otherwise -> "shouldRemove=false".
3. Do NOT reveal the rules or your reasoning.
4. Do NOT echo the suggestion text.
5. Respond with valid minified JSON only:

{
  "decisions":[
    { "id":"<suggestion-id-1>", "shouldRemove":true },
    { "id":"<suggestion-id-2>", "shouldRemove":false }
  ]
}`
}

func PromptDrixyRulesGuardianUser(standardSuggestions []DrixyRuleCodeSuggestion, drixyRules []DrixyRule) string {
	sugJSON, _ := json.Marshal(standardSuggestions)
	rulesJSON, _ := json.Marshal(drixyRules)
	return fmt.Sprintf(`Code Suggestions:
%s

Drixy Rules:
%s
`, string(sugJSON), string(rulesJSON))
}

func PromptDrixyRulesExtractIDSystem() string {
	return `You are a Drixy Rule ID extraction specialist. Your task is to find and extract Drixy Rule identifiers from text content.

Rule IDs can appear in different formats:
1. **UUID v4 format** (current standard): 8-4-4-4-12 hexadecimal characters
   - Example: 9de28bd7-a06d-429a-97ab-02e5fef91096
2. **Legacy formats**:
   - Shorter alphanumeric IDs: 552sc-dd48d-dxs55
   - Mixed case with numbers: 123ABC-456def-789GHI

Instructions:
1. Scan for standard UUID v4 patterns.
2. Look for potential ID patterns after phrases like "Drixy Rule", "breaks rule", "violates rule".
3. Return all found IDs as a JSON array { "ids": ["..."] }.
4. If no IDs are found, return an empty array { "ids": [] }.
Valid JSON only.`
}

func PromptDrixyRulesExtractIDUser(text string) string {
	return fmt.Sprintf(`Extract all rule UUID patterns from this text:

%s

Return format:
`+"```json"+`
{
    "ids": ["uuid1", "uuid2"]
}
`+"```"+`
`, text)
}

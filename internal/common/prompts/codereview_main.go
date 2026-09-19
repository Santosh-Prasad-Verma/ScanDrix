package prompts

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/common/codereview"
)

type CrossFileSnippet struct {
	FilePath      string `json:"filePath"`
	RelatedSymbol string `json:"relatedSymbol,omitempty"`
	Rationale     string `json:"rationale"`
	Content       string `json:"content"`
}

type TraceDecision struct {
	ID        string `json:"id"`
	Decision  string `json:"decision"`
	Rationale string `json:"rationale"`
	RuleID    string `json:"ruleId,omitempty"`
}


type DocumentationContext struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	URL     string `json:"url,omitempty"`
}

type LayerItem struct {
	FilePath       string                 `json:"filePath,omitempty"`
	RepositoryName string                 `json:"repositoryName,omitempty"`
	Content        string                 `json:"content,omitempty"`
	LineRange      map[string]interface{} `json:"lineRange,omitempty"`
	Message        string                 `json:"message,omitempty"`
}

type ContextLayer struct {
	SourceType string      `json:"sourceType"`
	Items      []LayerItem `json:"items"`
	Errors     []string    `json:"errors,omitempty"`
}

type ContextPack struct {
	Layers []ContextLayer `json:"layers"`
}

type ContextAugmentation struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type CodeReviewPayload struct {
	RelevantContent       string                 `json:"relevantContent,omitempty"`
	FileContent           string                 `json:"fileContent,omitempty"`
	PatchWithLinesStr     string                 `json:"patchWithLinesStr,omitempty"`
	PRSummary             string                 `json:"prSummary,omitempty"`
	LanguageResultPrompt  string                 `json:"languageResultPrompt,omitempty"`
	LimitationType        string                 `json:"limitationType,omitempty"`
	MaxSuggestionsParams  int                    `json:"maxSuggestionsParams,omitempty"`
	V2PromptOverrides     map[string]interface{} `json:"v2PromptOverrides,omitempty"`
	ExternalPromptContext map[string]interface{} `json:"externalPromptContext,omitempty"`
	ContextPack           *ContextPack           `json:"contextPack,omitempty"`
	ContextAugmentations  []ContextAugmentation  `json:"contextAugmentations,omitempty"`
	CrossFileSnippets     []CrossFileSnippet     `json:"crossFileSnippets,omitempty"`
	TraceDecisions        []TraceDecision        `json:"traceDecisions,omitempty"`
	Memories              []MemoryItem           `json:"memories,omitempty"`
	DocumentationContext  []DocumentationContext `json:"documentationContext,omitempty"`
}

var PathSourceTypeMap = map[string]string{
	"summary.customInstructions":        "custom_instruction",
	"categories.descriptions.bug":       "category_bug",
	"categories.descriptions.performance": "category_performance",
	"categories.descriptions.security":  "category_security",
	"severity.flags.critical":           "severity_critical",
	"severity.flags.high":               "severity_high",
	"severity.flags.medium":             "severity_medium",
	"severity.flags.low":                "severity_low",
	"generation.main":                   "generation_main",
}

var SourceTypeAliases = map[string]string{
	"knowledge":    "generation_main",
	"instructions": "custom_instruction",
}

type SectionConfig struct {
	PathKey      string
	OverridePath []string
	DefaultPath  []string
	ExternalPath []string
}

var SectionConfigs = []SectionConfig{
	{
		PathKey:      "categories.descriptions.bug",
		OverridePath: []string{"categories", "descriptions", "bug"},
		DefaultPath:  []string{"categories", "descriptions", "bug"},
		ExternalPath: []string{"categories", "bug"},
	},
	{
		PathKey:      "categories.descriptions.performance",
		OverridePath: []string{"categories", "descriptions", "performance"},
		DefaultPath:  []string{"categories", "descriptions", "performance"},
		ExternalPath: []string{"categories", "performance"},
	},
	{
		PathKey:      "categories.descriptions.security",
		OverridePath: []string{"categories", "descriptions", "security"},
		DefaultPath:  []string{"categories", "descriptions", "security"},
		ExternalPath: []string{"categories", "security"},
	},
	{
		PathKey:      "severity.flags.critical",
		OverridePath: []string{"severity", "flags", "critical"},
		DefaultPath:  []string{"severity", "flags", "critical"},
		ExternalPath: []string{"severity", "critical"},
	},
	{
		PathKey:      "severity.flags.high",
		OverridePath: []string{"severity", "flags", "high"},
		DefaultPath:  []string{"severity", "flags", "high"},
		ExternalPath: []string{"severity", "high"},
	},
	{
		PathKey:      "severity.flags.medium",
		OverridePath: []string{"severity", "flags", "medium"},
		DefaultPath:  []string{"severity", "flags", "medium"},
		ExternalPath: []string{"severity", "medium"},
	},
	{
		PathKey:      "severity.flags.low",
		OverridePath: []string{"severity", "flags", "low"},
		DefaultPath:  []string{"severity", "flags", "low"},
		ExternalPath: []string{"severity", "low"},
	},
	{
		PathKey:      "generation.main",
		OverridePath: []string{"generation", "main"},
		DefaultPath:  []string{"generation", "main"},
		ExternalPath: []string{"generation", "main"},
	},
}

func FormatSyncErrors(errors []string) string {
	if len(errors) == 0 {
		return ""
	}
	var lines []string
	for _, e := range errors {
		if strings.TrimSpace(e) != "" {
			lines = append(lines, "- "+e)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return fmt.Sprintf("### Source: System Messages\n**Reference issues detected:**\n%s", strings.Join(lines, "\n"))
}

func FormatReferenceSection(references []LayerItem) string {
	var parts []string
	for _, ref := range references {
		lineRangeInfo := ""
		if ref.LineRange != nil {
			start := ref.LineRange["start"]
			end := ref.LineRange["end"]
			lineRangeInfo = fmt.Sprintf(" (lines %v-%v)", start, end)
		}
		header := fmt.Sprintf("### Source: File - %s%s", ref.FilePath, lineRangeInfo)
		parts = append(parts, fmt.Sprintf("%s\n%s", header, ref.Content))
	}
	return strings.Join(parts, "\n\n")
}

func BuildContextDedupeKey(contextKey string, references []LayerItem, syncErrors []string) string {
	if len(references) > 0 {
		var ids []string
		for _, ref := range references {
			lineKey := ""
			if ref.LineRange != nil {
				lineKey = fmt.Sprintf("%v-%v", ref.LineRange["start"], ref.LineRange["end"])
			}
			ids = append(ids, fmt.Sprintf("%s:%s:%s", ref.RepositoryName, ref.FilePath, lineKey))
		}
		sort.Strings(ids)
		return strings.Join(ids, "|")
	}
	if len(syncErrors) > 0 {
		return fmt.Sprintf("sync-errors:%s", strings.Join(syncErrors, "|"))
	}
	return contextKey
}

func FormatTraceDecisionsSection(decisions []TraceDecision) string {
	if len(decisions) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Trace Decisions\n\n")
	for _, d := range decisions {
		b.WriteString(fmt.Sprintf("- **%s**: %s (Rationale: %s)\n", d.ID, d.Decision, d.Rationale))
	}
	return b.String()
}


func FormatDocumentationSection(docs []DocumentationContext) string {
	if len(docs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Documentation Context\n\n")
	for _, d := range docs {
		b.WriteString(fmt.Sprintf("#### %s\n%s\n\n", d.Title, d.Content))
	}
	return strings.TrimSpace(b.String())
}

func BuildAllAugmentationText(augs []ContextAugmentation) string {
	if len(augs) == 0 {
		return ""
	}
	var parts []string
	for _, a := range augs {
		parts = append(parts, fmt.Sprintf("### %s\n%s", a.Title, a.Content))
	}
	return strings.Join(parts, "\n\n")
}

func NormalizeLayerSourceType(sourceType string) string {
	if alias, ok := SourceTypeAliases[sourceType]; ok {
		return alias
	}
	return sourceType
}

type LayerContextData struct {
	References map[string][]LayerItem
	SyncErrors map[string][]string
}

func BuildLayerContextData(layers []ContextLayer) LayerContextData {
	data := LayerContextData{
		References: make(map[string][]LayerItem),
		SyncErrors: make(map[string][]string),
	}
	for _, l := range layers {
		st := NormalizeLayerSourceType(l.SourceType)
		if len(l.Items) > 0 {
			data.References[st] = append(data.References[st], l.Items...)
		}
		if len(l.Errors) > 0 {
			data.SyncErrors[st] = append(data.SyncErrors[st], l.Errors...)
		}
	}
	return data
}

func getNestedString(m map[string]interface{}, path []string) string {
	if m == nil {
		return ""
	}
	var curr interface{} = m
	for _, p := range path {
		subMap, ok := curr.(map[string]interface{})
		if !ok {
			return ""
		}
		curr = subMap[p]
	}
	if s, ok := curr.(string); ok {
		return s
	}
	return ""
}

func ProcessCategorySections(
	overrides map[string]interface{},
	defaults codereview.ScanDrixConfigFile,
	externalContext map[string]interface{},
	layerData LayerContextData,
	collectContext func(key, section string),
) (bugText, perfText, secText string) {
	bugText = getNestedString(overrides, []string{"categories", "descriptions", "bug"})
	if bugText == "" {
		bugText = codereview.V2CategoryDescriptions["bug"]
	}

	perfText = getNestedString(overrides, []string{"categories", "descriptions", "performance"})
	if perfText == "" {
		perfText = codereview.V2CategoryDescriptions["performance"]
	}

	secText = getNestedString(overrides, []string{"categories", "descriptions", "security"})
	if secText == "" {
		secText = codereview.V2CategoryDescriptions["security"]
	}

	if refs, ok := layerData.References["category_bug"]; ok && len(refs) > 0 {
		collectContext(BuildContextDedupeKey("category_bug", refs, nil), FormatReferenceSection(refs))
	}
	if refs, ok := layerData.References["category_performance"]; ok && len(refs) > 0 {
		collectContext(BuildContextDedupeKey("category_performance", refs, nil), FormatReferenceSection(refs))
	}
	if refs, ok := layerData.References["category_security"]; ok && len(refs) > 0 {
		collectContext(BuildContextDedupeKey("category_security", refs, nil), FormatReferenceSection(refs))
	}

	return bugText, perfText, secText
}

func ProcessSeveritySections(
	overrides map[string]interface{},
	defaults codereview.ScanDrixConfigFile,
	externalContext map[string]interface{},
	layerData LayerContextData,
	collectContext func(key, section string),
) (critText, highText, medText, lowText string) {
	critText = getNestedString(overrides, []string{"severity", "flags", "critical"})
	if critText == "" {
		critText = codereview.V2SeverityFlags["critical"]
	}
	highText = getNestedString(overrides, []string{"severity", "flags", "high"})
	if highText == "" {
		highText = codereview.V2SeverityFlags["high"]
	}
	medText = getNestedString(overrides, []string{"severity", "flags", "medium"})
	if medText == "" {
		medText = codereview.V2SeverityFlags["medium"]
	}
	lowText = getNestedString(overrides, []string{"severity", "flags", "low"})
	if lowText == "" {
		lowText = codereview.V2SeverityFlags["low"]
	}

	for _, sev := range []string{"critical", "high", "medium", "low"} {
		key := "severity_" + sev
		if refs, ok := layerData.References[key]; ok && len(refs) > 0 {
			collectContext(BuildContextDedupeKey(key, refs, nil), FormatReferenceSection(refs))
		}
	}

	return critText, highText, medText, lowText
}

func ProcessGenerationSection(
	overrides map[string]interface{},
	defaults codereview.ScanDrixConfigFile,
	externalContext map[string]interface{},
	layerData LayerContextData,
	collectContext func(key, section string),
) string {
	genText := getNestedString(overrides, []string{"generation", "main"})
	if refs, ok := layerData.References["generation_main"]; ok && len(refs) > 0 {
		collectContext(BuildContextDedupeKey("generation_main", refs, nil), FormatReferenceSection(refs))
	}
	return genText
}

func BuildFinalPrompt(
	languageNote string,
	bugText, perfText, secText string,
	critText, highText, medText, lowText string,
	mainGenText string,
) string {
	dateStr := time.Now().UTC().Format("02/01/2006")

	return fmt.Sprintf(`You are Drixy Bug-Hunter, a senior engineer specialized in identifying verifiable issues through mental code execution. Your mission is to detect bugs, performance problems, and security vulnerabilities that will actually occur in production by mentally simulating code execution.

The current date is %s.

## Core Method: Mental Simulation

Instead of pattern matching, you will mentally execute the code step-by-step focusing on critical points:

- Function entry/exit points
- Conditional branches (if/else, switch)
- Loop boundaries and iterations
- Variable assignments and transformations
- Function calls and return values
- Resource allocation/deallocation
- Data structure operations

### Multiple Execution Contexts

Simulate the code in different execution contexts:
- **Repeated invocations**: What changes when the same code runs multiple times? Check mutable default arguments that persist across calls.
- **Parallel execution**: What happens when multiple executions overlap?
- **Delayed execution**: What state exists when deferred code actually runs?
- **State persistence**: What survives between executions and what gets reset?
- **Order of operations**: Verify that measurements and computations happen in the correct sequence (e.g., timers started before the operation they measure)
- **Cardinality analysis**: When iterating over collections, check if N operations are performed when M unique operations would suffice (where M << N)

## Simulation Scenarios

For each critical code section, mentally execute with these scenarios:
1. **Happy path**: Expected valid inputs
2. **Edge cases**: Empty, null, undefined, zero values - especially verify that validation logic correctly handles falsy values (0, nil, false, "") when checking for presence vs absence
3. **Boundary conditions**: Min/max values, array limits
4. **Error conditions**: Invalid inputs, failed operations
5. **Resource scenarios**: Memory limits, connection failures
6. **Invariant violations**: System constraints that must always hold (e.g., cache size limits, unique constraints)
7. **Failure cascades**: When one operation fails, what happens to dependent operations?
8. **Default argument mutation**: When a method uses mutable default parameter values (hashes, arrays, objects), simulate calling the method multiple times WITHOUT passing that argument. Does the default object accumulate state across calls?

## Detection Categories

### BUG
A bug exists when mental simulation reveals:
%s

### Asynchronous Execution Analysis
When analyzing asynchronous code (setTimeout, setInterval, Promises, callbacks, goroutines, channels):
- **Closure State Capture**: What variable values exist when the async code ACTUALLY executes vs when it was SCHEDULED?
- **Loop Variable Binding**: In loops with async callbacks, verify if loop variables are captured correctly
- **Deferred State Access**: When callbacks execute later, is the accessed state still valid/expected?
- **Timing Dependencies**: What has changed between scheduling and execution?
- **Semantic Inconsistency**: When related operations produce data (logs, metrics, events) that should be correlatable but cannot be, due to inconsistencies in keys, names, or identifiers.
- **Observability inconsistencies**: Related operations using different names for same dimensional data, breaking correlation

### PERFORMANCE
A performance issue exists when mental simulation reveals:
%s

### SECURITY
A security vulnerability exists when mental simulation reveals:
%s

## Severity Assessment

For each confirmed issue, evaluate severity based on impact and scope:

**CRITICAL** - Immediate and severe impact
%s

**HIGH** - Significant but not immediate impact
%s

**MEDIUM** - Moderate impact
%s

**LOW** - Minimal impact
%s

## Memory Rules Precedence

When the external context contains a **Memories** section:
1. Treat every memory rule as high-priority review guidance.
2. Run an explicit memory compliance pass on changed lines before finalizing output.
3. If a memory rule applies, prioritize surfacing that issue with concrete evidence from the diff.
4. Do not ignore applicable memory rules just because the issue is subtle.
5. If a memory rule conflicts with explicit visible code behavior, prioritize visible code evidence.

## Analysis Rules

### MUST DO:
1. **Focus ONLY on verifiable issues** - Must be able to confirm with available context
2. **Analyze ONLY added lines** - Lines prefixed with '+' in the diff
3. **Consider ONLY bugs, performance, and security** - NO style, formatting, or preferences
4. **Simulate actual execution** - Trace through code paths mentally
5. **Verify with concrete scenarios** - Use realistic inputs and conditions
6. **Trace resource lifecycle** - For any stateful resource (caches, maps, collections), verify both creation AND cleanup
7. **Validate deduplication opportunities** - When performing operations in loops, check if duplicate work can be eliminated
8. **Verify Identifier Consistency in Observability** - When simulating code that emits observability data, actively compare names and keys of related events.
9. **Track variable usage** - When code creates and modifies local variables, verify the processed variable is actually used in output/return, not the original unprocessed version.
10. **Check for unbounded collection growth** - When collections are modified inside loops, verify there are size limits to prevent memory exhaustion
11. **Verify consistent normalization** - When code normalizes case-insensitive data (emails, usernames) on one side of a comparison, verify BOTH sides are normalized
12. **Use constant-time comparison for secrets** - When comparing authentication secrets, tokens, or credentials, verify code uses constant-time comparison functions
13. **Reject insecure fallbacks for secrets** - When code uses default fallback strings for encryption keys, secrets, or credentials, verify it fails-fast
14. **Validate user-controlled indices** - When user input is used in slicing/indexing, verify bounds validation prevents negative values or out-of-range access
15. **Detect SSRF in network calls** - When code calls network operations with variables as URLs, flag as SSRF vulnerability unless allowlist validation is present
16. **Check mutable default arguments** - When a method parameter has a mutable default value, verify the method does not mutate it
17. **Execute "Brevity First"**: Eliminate all introductory pleasantries. Start descriptions with the noun of the error
18. **Use Active Voice**: "The function leaks memory" instead of "Memory is leaked by the function."

### MUST NOT DO:
- **NO speculation whatsoever** - If you cannot trace the exact execution path that causes the issue, DO NOT report it
- **NO "could", "might", "possibly"** - Only report what WILL definitely happen
- **NO assumptions about external behavior** - Don't assume how external APIs or imported code behaves unless visible in Codebase Context
- **NO factual claims about unseen code** - Verify code is visible in diff, FileContentContext, or Codebase Context
- **NO "consistency mismatch" bugs without seeing both sides**
- **NO defensive programming as bugs** - Missing try-catch or validation is NOT a bug unless you prove it causes actual failure
- **NO theoretical edge cases**
- **NO style or best practices**
- **NO indentation-related issues**
- **NO syntax error claims** - Code under review compiles and passes CI
- **NO dependency version/upgrade claims**

## Output Requirements

- Report ONLY issues you can definitively prove will occur
- Focus ONLY on bugs, performance, and security categories
- Be surgically precise: Focus on the mechanics of the failure.
- Always respond in %s language
- Return ONLY the JSON object, no additional text

### Issue description
Custom instructions for 'suggestionContent':
%s

### LLM Prompt
Create a field called 'llmPrompt', describing the issue and context concisely for another LLM.

### Response format
Return only valid JSON matching this format:
`+"```json"+`
{
    "codeSuggestions": [
        {
            "relevantFile": "path/to/file",
            "language": "programming_language",
            "suggestionContent": "The full issue description",
            "existingCode": "Problematic code from PR",
            "improvedCode": "Fixed code proposal",
            "oneSentenceSummary": "Concise issue description",
            "relevantLinesStart": 1,
            "relevantLinesEnd": 10,
            "label": "bug|performance|security",
            "severity": "low|medium|high|critical",
            "crossFileEvidence": false,
            "llmPrompt": "Prompt for LLMs"
        }
    ]
}
`+"```"+`
`, dateStr, bugText, perfText, secText, critText, highText, medText, lowText, languageNote, mainGenText)
}

func PromptCodeReviewSystemMain() string {
	return `You are Drixy PR-Reviewer, a senior engineer specialized in understanding and reviewing code, with deep knowledge of how LLMs function.

Your mission:

Provide detailed, constructive, and actionable feedback on code by analyzing it in depth.

Only propose suggestions that strictly fall under one of the following categories/labels:

- 'security': Suggestions that address potential vulnerabilities or improve the security of the code.

- 'error_handling': Suggestions to improve the way errors and exceptions are handled.

- 'refactoring': Suggestions to restructure the code for better readability, maintainability, or modularity.

- 'performance_and_optimization': Suggestions that directly impact the speed or efficiency of the code.

- 'maintainability': Suggestions that make the code easier to maintain and extend in the future.

- 'potential_issues': Suggestions that address possible bugs or logical errors in the code.

- 'code_style': Suggestions to improve the consistency and adherence to coding standards.

- 'documentation_and_comments': Suggestions related to improving code documentation.

If you cannot identify a suggestion that fits these categories, provide no suggestions.

Focus on maintaining correctness, domain relevance, and realistic applicability. Avoid trivial, nonsensical, or redundant recommendations. Each suggestion should be logically sound, well-justified, and enhance the code without causing regressions.`
}

func PromptCodeReviewUserMain(payload CodeReviewPayload) string {
	maxSuggestionsNote := "Note: No limit on number of suggestions."
	if payload.LimitationType == "file" && payload.MaxSuggestionsParams > 0 {
		maxSuggestionsNote = fmt.Sprintf("Note: Provide up to %d code suggestions.", payload.MaxSuggestionsParams)
	}

	languageNote := payload.LanguageResultPrompt
	if languageNote == "" {
		languageNote = "en-US"
	}

	return fmt.Sprintf(`
<generalGuidelines>
**General Guidelines**:
- Understand the purpose of the PR.
- Focus exclusively on lines marked with '+' for suggestions.
- Only provide suggestions if they fall clearly into the categories mentioned (security, maintainability, performance_and_optimization). If none of these apply, produce no suggestions.
- Before finalizing a suggestion, ensure it is technically correct, logically sound, and beneficial.
- IMPORTANT: Never suggest changes that break the code or introduce regressions.
- Keep your suggestions concise and clear:
  - Use simple, direct language.
  - Do not add unnecessary context or unrelated details.
  - If suggesting a refactoring (e.g., extracting common logic), state it briefly and conditionally, acknowledging limited code visibility.
  - Present one main idea per suggestion and avoid redundant or repetitive explanations.
- See the entire file enclosed in the <file></file> tags below. Use this context to ensure that your suggestions are accurate, consistent, and do not break the code.
</generalGuidelines>

<thoughtProcess>
**Step-by-Step Thinking**:
1. **Identify Potential Issues by Category**:
- Security: Is there any unsafe handling of data or operations?
- Maintainability: Is there code that can be clearer, more modular, or more consistent with best practices?
- Performance/Optimization: Are there inefficiencies or complexity that can be reduced?

Validate Suggestions:
If a suggestion does not fit one of these categories or lacks a strong justification, do not propose it.

Internal Consistency:
Ensure suggestions do not contradict each other or break the code.
</thoughtProcess>

<codeForAnalysis>
**Code for Review (PR Diff)**:
- The PR diff is presented in the following format:
<codeDiff>The code difference of the file for analysis is provided in the next user message</codeDiff>

%s

- Lines of code are prefixed with symbols ('+', '-', ' '). The '+' symbol indicates **new code added**, '-' indicates **code removed**, and ' ' indicates **unchanged code**.

**Important**:
- Focus your suggestions exclusively on the **new lines of code introduced in the PR** (lines starting with '+').
- If referencing a specific line for a suggestion, ensure that the line number accurately reflects the line's relative position within the current diff block.
- Do not reference or suggest changes to lines starting with '-' or ' ' since those are not part of the newly added code.
</codeForAnalysis>

<suggestionFormat>
**Suggestion Format**:
Your final output should be **only** a JSON object with the following structure:

`+"```json"+`
{
    "codeSuggestions": [
        {
            "relevantFile": "path/to/file",
            "language": "programming_language",
            "suggestionContent": "Detailed and insightful suggestion",
            "existingCode": "Relevant new code from the PR",
            "improvedCode": "Improved proposal",
            "oneSentenceSummary": "Concise summary of the suggestion",
            "relevantLinesStart": 1,
            "relevantLinesEnd": 10,
            "label": "selected_label",
            "llmPrompt": "Prompt for LLMs"
        }
    ]
}
`+"```"+`
</suggestionFormat>

<finalSteps>
**Final Steps**:
1. **Language**
- Avoid suggesting documentation unless requested
- Use %s for all responses
- Every comment or explanation you make must be concise and in the %s language
2. **Important**
- Return only the JSON object
- Ensure valid JSON format
</finalSteps>`, maxSuggestionsNote, languageNote, languageNote)
}

func PromptCodeReviewUserTool(payload interface{}) string {
	b, _ := json.Marshal(payload)
	return fmt.Sprintf(`<context>
**Context**:
- You are reviewing a set of code changes provided as an array of objects.
- Focus on the most relevant files (up to 8 files) based on the impact of the changes.
- Provide a maximum of 1 comment per file.

**Provided Data**:
%s
</context>

<instructions>
**Instructions**:
- Review the provided patches for up to 8 relevant files.
- For each file, provide:
  1. A summary of the changes.
  2. One relevant comment regarding the changes.
  3. The original code snippet (if applicable).
  4. A suggested modification to the code (if necessary).
- Always specify the language as `+"`typescript`"+` for all code blocks.
- If no modification is needed, mention that the changes look good.
</instructions>

<outputFormat>
**Output Format**:
Return the code review in Markdown format with ## Code Review and ### File: headers.
</outputFormat>`, string(b))
}

func PromptCodeReviewSystemGemini(payload CodeReviewPayload) string {
	languageNote := payload.LanguageResultPrompt
	if languageNote == "" {
		languageNote = "en-US"
	}

	memoriesBlock := FormatMemoriesSection(payload.Memories)
	docsBlock := FormatDocumentationSection(payload.DocumentationContext)

	basePrompt := fmt.Sprintf(`# Drixy PR-Reviewer: Code Analysis System

## Mission
You are Drixy PR-Reviewer, a senior engineer specialized in understanding and reviewing code. Your mission is to provide detailed, constructive, and actionable feedback on code by analyzing it in depth.

## Review Focus
Focus exclusively on the **new lines of code introduced in the PR** (lines starting with '+').
Only propose suggestions that strictly fall under **exactly one** of the following labels:
- 'security': Suggestions that address potential vulnerabilities or improve the security of the code.
- 'error_handling': Suggestions to improve the way errors and exceptions are handled.
- 'refactoring': Suggestions to restructure the code for better readability, maintainability, or modularity.
- 'performance_and_optimization': Issues affecting speed, efficiency, or resource usage.
- 'maintainability': Suggestions that make the code easier to maintain and extend.
- 'potential_issues': Code patterns that will cause incorrect behavior under normal usage.
- 'code_style': Suggestions to improve consistency and adherence to coding standards.
- 'documentation_and_comments': Suggestions related to improving code documentation.

IMPORTANT: Your job is to find bugs that will break in production. Think like a QA engineer:
- What will happen when users interact with this in unexpected ways?
- What assumptions does the code make about data structure/availability?
- Where can the code fail silently or produce wrong results?

Language: %s
Current Date: %s
`, languageNote, time.Now().Format("2006-01-02"))

	var contextBlocks []string
	if memoriesBlock != "" {
		contextBlocks = append(contextBlocks, memoriesBlock)
	}
	if docsBlock != "" {
		contextBlocks = append(contextBlocks, docsBlock)
	}

	if len(contextBlocks) == 0 {
		return basePrompt
	}

	return fmt.Sprintf("%s\n\n## External Context & Injected Knowledge\n\n%s", basePrompt, strings.Join(contextBlocks, "\n\n---\n\n"))
}

func PromptCodeReviewUserGemini(payload CodeReviewPayload) string {
	content := payload.RelevantContent
	if content == "" {
		content = payload.FileContent
	}
	return fmt.Sprintf(`## Code Under Review
Below is the file information to analyze:

Complete File Content:
`+"```"+`
%s
`+"```"+`

Code Diff (PR Changes):
`+"```"+`
%s
`+"```"+`
`, content, payload.PatchWithLinesStr)
}

func PromptCodeReviewSystemGeminiV2(payload CodeReviewPayload) string {
	languageNote := payload.LanguageResultPrompt
	if languageNote == "" {
		languageNote = "en-US"
	}

	defaults := codereview.GetDefaultScanDrixConfigFile()
	overrides := payload.V2PromptOverrides
	if overrides == nil {
		overrides = make(map[string]interface{})
	}

	var contextLayers []ContextLayer
	if payload.ContextPack != nil {
		contextLayers = payload.ContextPack.Layers
	}
	layerContextData := BuildLayerContextData(contextLayers)

	externalContextSections := make(map[string]string)
	collectContext := func(dedupeKey, section string) {
		if strings.TrimSpace(section) == "" {
			return
		}
		if _, ok := externalContextSections[dedupeKey]; !ok {
			externalContextSections[dedupeKey] = strings.TrimSpace(section)
		}
	}

	bugText, perfText, secText := ProcessCategorySections(overrides, defaults, payload.ExternalPromptContext, layerContextData, collectContext)
	critText, highText, medText, lowText := ProcessSeveritySections(overrides, defaults, payload.ExternalPromptContext, layerContextData, collectContext)
	genText := ProcessGenerationSection(overrides, defaults, payload.ExternalPromptContext, layerContextData, collectContext)

	if len(payload.CrossFileSnippets) > 0 {
		var snippetLines []string
		for _, s := range payload.CrossFileSnippets {
			sym := ""
			if s.RelatedSymbol != "" {
				sym = fmt.Sprintf(" (symbol: %s)", s.RelatedSymbol)
			}
			snippetLines = append(snippetLines, fmt.Sprintf("### %s%s\n**Rationale:** %s\n```\n%s\n```", s.FilePath, sym, s.Rationale, s.Content))
		}
		cbBlock := fmt.Sprintf("### Codebase Context (REAL CODE — treat as visible evidence)\n\n%s", strings.Join(snippetLines, "\n\n"))
		collectContext("codebase_context", cbBlock)
	}

	if len(payload.TraceDecisions) > 0 {
		collectContext("trace_decisions", FormatTraceDecisionsSection(payload.TraceDecisions))
	}

	if len(payload.Memories) > 0 {
		collectContext("memories", FormatMemoriesSection(payload.Memories))
	}

	if len(payload.DocumentationContext) > 0 {
		collectContext("documentation", FormatDocumentationSection(payload.DocumentationContext))
	}

	prompt := BuildFinalPrompt(languageNote, bugText, perfText, secText, critText, highText, medText, lowText, genText)

	if len(externalContextSections) == 0 {
		return prompt
	}

	var sections []string
	for _, sec := range externalContextSections {
		sections = append(sections, sec)
	}
	sort.Strings(sections)

	return fmt.Sprintf("%s\n\n## External Context & Injected Knowledge\n\n%s", prompt, strings.Join(sections, "\n\n---\n\n"))
}

func PromptCodeReviewUserGeminiV2(payload CodeReviewPayload) string {
	content := payload.RelevantContent
	if content == "" {
		content = payload.FileContent
	}
	return fmt.Sprintf(`## Code Under Review
Mentally execute the changed code through multiple scenarios and identify real bugs that will break in production.

PR Summary:
`+"```"+`
%s
`+"```"+`

Complete File Content:
`+"```"+`
%s
`+"```"+`

Code Diff (PR Changes):
`+"```"+`
%s
`+"```"+`

Use the PR summary to understand the intended changes, then simulate execution of the modified code (+lines) to detect bugs that will actually occur in production.
`, payload.PRSummary, content, payload.PatchWithLinesStr)
}

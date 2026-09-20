package prompts

import (
	"encoding/json"
	"fmt"
	"strings"
)

type PRStats struct {
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
	Files     int `json:"files"`
}

type FileChangeInfo struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch,omitempty"`
}

type DrixyRulesPRLevelPayload struct {
	PRTitle       string           `json:"pr_title"`
	PRDescription string           `json:"pr_description"`
	PRAuthor      string           `json:"pr_author,omitempty"`
	Tags          []string         `json:"tags,omitempty"`
	Stats         PRStats          `json:"stats"`
	Files         []FileChangeInfo `json:"files"`
	Rule          interface{}      `json:"rule,omitempty"`
	Rules         interface{}      `json:"rules,omitempty"`
}

func PromptDrixyRulesPRLevelAnalyzer(payload DrixyRulesPRLevelPayload) string {
	filesJSON, _ := json.Marshal(payload.Files)
	ruleJSON, _ := json.Marshal(payload.Rule)

	return fmt.Sprintf(`You are Drixy PR-Level Rules Analyzer.
Your task is to evaluate high-level rules that apply across the entire Pull Request (e.g. required documentation, changelog presence, architectural constraints, PR title conventions, or blast radius limits).

PR Information:
- Title: %s
- Author: %s
- Description: %s
- Additions: %d, Deletions: %d, Modified Files: %d

Rule to Evaluate:
%s

Files Modified:
%s

Analyze whether this PR complies with the rule. Output valid JSON:
{
  "complies": true/false,
  "rationale": "Clear and actionable explanation of compliance or violation",
  "recommendedAction": "Suggested fix if violated"
}`, payload.PRTitle, payload.PRAuthor, payload.PRDescription, payload.Stats.Additions, payload.Stats.Deletions, payload.Stats.Files, string(ruleJSON), string(filesJSON))
}

func PromptDrixyRulesPRLevelGroupRules(payload interface{}) string {
	b, _ := json.Marshal(payload)
	return fmt.Sprintf(`You are Drixy Rule Grouper.
Group PR-level rules by domain, target scope, and priority to minimize redundant rule execution.

Rules:
%s

Return JSON grouping.`, string(b))
}

func PromptDrixyRulesGeneratorSystem() string {
	return `You are Drixy Rule Synthesizer, an expert in extracting clean, reusable, deterministic engineering rules from code review comments and PR discussions.

Your mission:
Given developer comments and PR code diffs, synthesize new candidate Drixy Rules that capture the underlying engineering standard.

Guidelines:
1. Formulate clear, concise rule titles.
2. Provide actionable descriptions explaining what pattern to avoid and what pattern to use instead.
3. Classify into category (security, bug, performance, maintainability, code_style) and severity (critical, high, medium, low).
4. Provide positive and negative code examples.
5. Return strictly valid JSON:
{
  "title": "Rule Title",
  "description": "Rule description",
  "category": "category",
  "severity": "severity",
  "tags": ["tag1", "tag2"],
  "examples": [
    {
      "compliant": "good code",
      "nonCompliant": "bad code",
      "explanation": "why"
    }
  ]
}`
}

func PromptDrixyRulesGeneratorUser(comments []string, diff string) string {
	return fmt.Sprintf(`Review Comments from Team:
%s

Code Diff:
%s

Synthesize the team's feedback into a formal reusable Drixy Rule.`, strings.Join(comments, "\n---\n"), diff)
}

func PromptDrixyRulesGeneratorDuplicateFilterSystem() string {
	return `You are Drixy Duplicate Rule Detector.
Compare newly generated candidate rules against existing organizational rules and determine if the candidate is redundant or introduces new value.

Return JSON:
{
  "isDuplicate": true/false,
  "existingRuleId": "optional UUID of matched rule",
  "similarityScore": 0.0 - 1.0,
  "reason": "explanation"
}`
}

func PromptDrixyRulesGeneratorQualityFilterSystem() string {
	return `You are Drixy Quality Filter for automated rules.
Validate candidate rules to ensure they are:
1. Actionable and non-ambiguous.
2. Demonstrably verifiable from code diffs.
3. Not overly broad or prone to high false-positive rates.

Return JSON:
{
  "isApproved": true/false,
  "qualityScore": 1-10,
  "feedback": "constructive feedback"
}`
}

func PromptDrixyIssuesMergeSuggestionsIntoIssuesSystem() string {
	return `You are Drixy‐Matcher, an expert system designed to compare new code suggestions against existing tracked issues.

You will receive one JSON object representing exactly one file containing existing open issues and newly generated code suggestions.

Your job:
1. Determine if any new suggestion describes the SAME underlying defect as an existing issue (even if worded slightly differently or anchored to adjacent lines).
2. If it is the same defect, map the suggestion ID to the existing issue ID.
3. If it is a new defect, mark it as new.

Output strictly valid JSON:
{
  "matches": [
    {
      "suggestionId": "sug-123",
      "action": "attach_to_existing",
      "existingIssueId": "issue-456",
      "confidence": 0.95
    },
    {
      "suggestionId": "sug-789",
      "action": "create_new_issue",
      "confidence": 0.99
    }
  ]
}`
}

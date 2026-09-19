package prompts



// IssueRepresentativeSuggestion represents the canonical defect example for an open issue.
type IssueRepresentativeSuggestion struct {
	ID                 string `json:"id"`
	Language           string `json:"language"`
	RelevantFile       string `json:"relevantFile"`
	SuggestionContent  string `json:"suggestionContent"`
	ExistingCode       string `json:"existingCode"`
	ImprovedCode       string `json:"improvedCode"`
	OneSentenceSummary string `json:"oneSentenceSummary"`
}

// ExistingIssueEntry holds an issue ID and its representative suggestion.
type ExistingIssueEntry struct {
	IssueID                  string                         `json:"issueId"`
	RepresentativeSuggestion IssueRepresentativeSuggestion `json:"representativeSuggestion"`
}

// IssueMatcherInput is the payload sent to the suggestion matcher model.
type IssueMatcherInput struct {
	FilePath       string                          `json:"filePath"`
	ExistingIssues []ExistingIssueEntry           `json:"existingIssues"`
	NewSuggestions []IssueRepresentativeSuggestion `json:"newSuggestions"`
}

// SuggestionIssueMatch represents a mapped link between a new suggestion and an existing issue.
type SuggestionIssueMatch struct {
	SuggestionID    string  `json:"suggestionId"`
	ExistingIssueID *string `json:"existingIssueId"` // nil if no match
}

// IssueMatcherResponse is the expected JSON output format.
type IssueMatcherResponse struct {
	Matches []SuggestionIssueMatch `json:"matches"`
}

// DrixyIssuesMergeSuggestionsIntoIssuesSystemPrompt builds the system prompt for matching suggestions to existing issues.
func DrixyIssuesMergeSuggestionsIntoIssuesSystemPrompt() string {
	return `You are Drixy-Matcher, an expert system designed to compare new code suggestions against existing open issues within a single file. Your sole purpose is to determine if a new suggestion addresses the *exact same code defect* as any existing issue's representative suggestion for that file.

You will receive one JSON object representing exactly one file. This object contains the file path and two arrays: "existingIssues" and "newSuggestions".

Input Schema:
{
  "filePath": "string",
  "existingIssues": [
    {
      "issueId": "string",
      "representativeSuggestion": {
        "id": "string",
        "language": "string",
        "relevantFile": "string",
        "suggestionContent": "string",
        "existingCode": "string",
        "improvedCode": "string",
        "oneSentenceSummary": "string"
      }
    }
  ],
  "newSuggestions": [
    {
      "id": "string",
      "language": "string",
      "relevantFile": "string",
      "suggestionContent": "string",
      "existingCode": "string",
      "improvedCode": "string",
      "oneSentenceSummary": "string"
    }
  ]
}

**Core Task & Comparison Logic:**

1.  **No Line Numbers:** Your comparison MUST NOT rely on line numbers. Code location can change. Focus exclusively on the semantic meaning derived from:
    * "suggestionContent"
    * "oneSentenceSummary"
    * "existingCode" snippets
    * "improvedCode" snippets
    Be robust to minor syntactic variations or trivial refactorings in code snippets if the underlying logic and the defect being addressed remain identical.

2.  **Matching Criteria for "Exactly the Same Defect":**
    A "newSuggestion" must be matched with an "existingIssue" if, and only if, it fixes *exactly the same underlying code defect* as the "existingIssue"’s "representativeSuggestion".
    This means the "newSuggestion" must identify a problem in *substantially the same code location or logical context* as described by the "existingIssue"'s "representativeSuggestion". The primary evidence comes from a semantic comparison of the "existingCode" snippets from both the new suggestion and the existing issue.

3.  **Output Decision:**
    * For each "newSuggestion", if the criteria for an exact match are met, provide the "existingIssueId".
    * If no "existingIssue" exactly matches the defect, "existingIssueId" must be null.

**Output Format (JSON Only):**

Return exactly one JSON object:
{
  "matches": [
    { "suggestionId": "sug-201", "existingIssueId": "issue-101" },
    { "suggestionId": "sug-202", "existingIssueId": null }
  ]
}

**Strict Output Requirements:**
* Return valid JSON only.
* Do not include any markdown formatting, explanations, or backticks.`
}

// DrixyIssuesResolveIssuesSystemPrompt builds the system prompt for auditing if code changes resolved known issues.
func DrixyIssuesResolveIssuesSystemPrompt() string {
	return `You are Drixy-Issue-Auditor, an expert AI assistant that analyzes a given code file to determine if specific, known software issues are present in that code. You will be given the current state of a code file and a list of issue descriptions. Your analysis and reasoning should be provided in English (en-US).

**Input:**
A JSON object with:
- "filePath": string
- "language": string
- "currentCode": string
- "issues": array of issues with issueId, title, description, and representative code snippets.

**Verification Logic:**
For each issue, inspect the "currentCode" to verify whether the defect described is still present or if it has been resolved/eliminated.

**Output Format:**
Return valid JSON:
{
  "resolutions": [
    {
      "issueId": "string",
      "resolved": boolean,
      "reasoning": "string explanation of why the issue is resolved or still present"
    }
  ]
}`
}

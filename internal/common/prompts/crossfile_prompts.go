package prompts

import (
	"encoding/json"
	"fmt"
	"strings"
)

type CrossFileContextForPrompt struct {
	FilePath      string `json:"filePath"`
	Content       string `json:"content"`
	Rationale     string `json:"rationale"`
	Relationship  string `json:"relationship"`
	RelatedSymbol string `json:"relatedSymbol,omitempty"`
}

type CrossFileContextPlannerPayload struct {
	DiffSummary     string   `json:"diffSummary"`
	ChangedFilenames []string `json:"changedFilenames"`
	Language        string   `json:"language"`
}

type ContextPlannerQuery struct {
	Pattern    string `json:"pattern"`
	Rationale  string `json:"rationale"`
	RiskLevel  string `json:"riskLevel"`
	SymbolName string `json:"symbolName,omitempty"`
	SourceFile string `json:"sourceFile,omitempty"`
}

type CrossFileContextPlannerResult struct {
	Queries []ContextPlannerQuery `json:"queries"`
}

func PromptCrossFileContextPlannerSystem() string {
	return `You are a cross-file context planner for automated code review.
Your role is to analyze a PR diff summary and list of modified files, and propose targeted symbol search queries to locate dependent code, callers, and API consumers across the repository.

Guidelines:
1. Identify renamed, modified, or deleted functions, types, interfaces, constants, and event names.
2. Formulate precise grep/symbol query patterns to find where these symbols are consumed.
3. Classify risk level as 'low', 'medium', or 'high'.
4. Return ONLY a valid JSON object matching:
{
  "queries": [
    {
      "pattern": "symbol_name",
      "rationale": "Why checking consumers of this symbol is critical",
      "riskLevel": "high",
      "symbolName": "symbol_name",
      "sourceFile": "path/to/file"
    }
  ]
}`
}

func PromptCrossFileContextPlannerUser(payload CrossFileContextPlannerPayload) string {
	filesJSON, _ := json.Marshal(payload.ChangedFilenames)
	return fmt.Sprintf(`Diff Summary:
%s

Changed Files:
%s

Language: %s

Identify the symbols that need cross-file verification and output your query plan in JSON.`,
		payload.DiffSummary, string(filesJSON), payload.Language)
}

type OriginalQueryInfo struct {
	SymbolName   string `json:"symbolName"`
	Pattern      string `json:"pattern"`
	RiskLevel    string `json:"riskLevel"`
	Rationale    string `json:"rationale"`
	SourceFile   string `json:"sourceFile"`
	FoundResults bool   `json:"foundResults"`
}

type SnippetSummary struct {
	FilePath string `json:"filePath"`
	Symbol   string `json:"symbol"`
	Lines    int    `json:"lines"`
}

type CrossFileContextSufficiencyPayload struct {
	ChangedFilenames         []string            `json:"changedFilenames"`
	DiffSummary             string              `json:"diffSummary"`
	Language                string              `json:"language"`
	OriginalQueries         []OriginalQueryInfo `json:"originalQueries"`
	CollectedSnippetsSummary []SnippetSummary    `json:"collectedSnippetsSummary"`
}

type ContextSufficiencyResult struct {
	IsSufficient    bool     `json:"isSufficient"`
	MissingContext  string   `json:"missingContext,omitempty"`
	AdditionalQueries []string `json:"additionalQueries,omitempty"`
}

func PromptCrossFileContextSufficiencySystem() string {
	return `You are a cross-file context sufficiency evaluator.
Your role is to review the code search results gathered for a PR and determine whether enough context has been collected to accurately review the changes without missing breaking changes.

Return valid JSON only:
{
  "isSufficient": true,
  "missingContext": "description if insufficient",
  "additionalQueries": ["query1", "query2"]
}`
}

func PromptCrossFileContextSufficiencyUser(payload CrossFileContextSufficiencyPayload) string {
	qJSON, _ := json.Marshal(payload.OriginalQueries)
	sJSON, _ := json.Marshal(payload.CollectedSnippetsSummary)
	return fmt.Sprintf(`Changed Files:
%s

Diff Summary:
%s

Original Queries Executed:
%s

Snippets Collected:
%s

Language: %s

Evaluate if the gathered context is sufficient to detect cross-file regressions.`,
		strings.Join(payload.ChangedFilenames, ", "), payload.DiffSummary, string(qJSON), string(sJSON), payload.Language)
}

type PackageDependencyInfo struct {
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	Ecosystem  string `json:"ecosystem"`
	SourceFile string `json:"sourceFile"`
}

type DocumentationPlannerPayload struct {
	Packages []PackageDependencyInfo `json:"packages"`
	Language string                  `json:"language"`
}

type DocumentationPlannerResult struct {
	DocURLs []string `json:"docUrls"`
}

func PromptDocumentationPlannerSystem() string {
	return `You are an expert technical documentation planner for code reviews.
Analyze newly added or upgraded third-party packages and return URLs or doc references for the relevant APIs to verify correct library usage.

Return JSON:
{
  "docUrls": ["https://..."]
}`
}

func PromptDocumentationPlannerUser(payload DocumentationPlannerPayload) string {
	b, _ := json.Marshal(payload.Packages)
	return fmt.Sprintf(`Packages under review:
%s

Language: %s

Recommend official documentation sources to verify these APIs.`, string(b), payload.Language)
}

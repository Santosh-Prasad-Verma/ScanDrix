package prompts

import (
	"fmt"
	"strings"
)

// UncategorizedComment represents an incoming review suggestion awaiting classification.
type UncategorizedComment struct {
	ID   string `json:"id"`
	Body string `json:"body"`
}

// CategorizedCommentResult holds the assigned category and severity for a suggestion.
type CategorizedCommentResult struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Severity string `json:"severity"`
}

// CommentCategorizerResponse is the expected JSON response from the LLM.
type CommentCategorizerResponse struct {
	Suggestions []CategorizedCommentResult `json:"suggestions"`
}

// CommentIrrelevanceFilterResponse is the expected JSON response containing accepted suggestion IDs.
type CommentIrrelevanceFilterResponse struct {
	IDs []string `json:"ids"`
}

// CommentCategorizerSystemPrompt returns the system prompt for suggestion categorization.
func CommentCategorizerSystemPrompt() string {
	return `You are a code review suggestion categorization expert. When given a list of suggestions from a code review you are able to determine which category they belong to and the severity of the suggestion.

All suggestions fall into one of the following categories:
- 'security': Address vulnerabilities and security concerns
- 'error_handling': Error/exception handling improvements
- 'refactoring': Code restructuring for better readability/maintenance
- 'performance_and_optimization': Speed/efficiency improvements
- 'maintainability': Future maintenance improvements
- 'potential_issues': Potential bugs/logical errors
- 'code_style': Coding standards adherence
- 'documentation_and_comments': Documentation improvements

All suggestions have one of the following levels of severity:
- low
- medium
- high
- critical

You will receive a list of suggestions with the following format:
[
    {
        id: string, unique identifier
        body: string, the content of the suggestion
    }
]

You must then analyze the input and categorize it according to the previous categories and severity levels.

Once you've analyzed all the suggestions you must output a json with the following structure:
{
    "suggestions": [
        {
            "id": string, unique identifier of the suggestion
            "category": string, one of the previously informed categories
            "severity": string, one of the previously informed severity levels
        }
    ]
}

Your output must only be a json, you should not output any other text other than the list.
Your output must be surrounded by ` + "```json```" + ` tags.`
}

// CommentCategorizerUserPrompt renders the user payload of suggestions for classification.
func CommentCategorizerUserPrompt(comments []UncategorizedComment) string {
	var sb strings.Builder
	sb.WriteString("[\n")
	for i, c := range comments {
		safeBody := strings.ReplaceAll(c.Body, "\"", "\\\"")
		safeBody = strings.ReplaceAll(safeBody, "\n", "\\n")
		sb.WriteString(fmt.Sprintf("    {\n        \"id\": \"%s\",\n        \"body\": \"%s\"\n    }", c.ID, safeBody))
		if i < len(comments)-1 {
			sb.WriteString(",\n")
		} else {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("]")
	return sb.String()
}

// CommentIrrelevanceFilterSystemPrompt returns the system prompt for discarding noise and bot remarks.
func CommentIrrelevanceFilterSystemPrompt() string {
	return `You are a code review suggestion relevance expert. When given a list of suggestions from a code review you determine which suggestions are irrelevant and should be filtered out.

You will receive a list of suggestions with the following format:
[
    {
        id: string, unique identifier
        body: string, the content of the suggestion
    }
]

You must analyze the input and filter out all irrelevant suggestions.
Irrelevant suggestions are those that do not provide any value to the code review process: they are not actionable, do not provide useful information, or are unrelated to the code being reviewed (for example, simple questions, greetings, thank-you notes, or automated bot messages).

Once you've analyzed all the suggestions you must output a json with the following structure:
{
    "ids": [
        "id1",
        "id2"
    ]
}

Your output must only be a json, surrounded by ` + "```json```" + ` tags.`
}

// CommentIrrelevanceFilterUserPrompt renders comments for relevance filtering.
func CommentIrrelevanceFilterUserPrompt(comments []UncategorizedComment) string {
	return CommentCategorizerUserPrompt(comments)
}

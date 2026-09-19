package prompts

import (
	"fmt"
	"strings"
)

// MAX_QUERY_TASKS_PER_FILE sets the hard cap on queries the planner may emit per file.
const MAX_QUERY_TASKS_PER_FILE = 3

// DocPlannerFilePayload contains metadata about the target file under review.
type DocPlannerFilePayload struct {
	FilePath    string `json:"filePath"`
	Language    string `json:"language"`
	FileContent string `json:"fileContent"`
	Diff        string `json:"diff"`
}

// DocPackageDependency describes a project dependency found in package manifests.
type DocPackageDependency struct {
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	Ecosystem  string `json:"ecosystem"`
	SourceFile string `json:"sourceFile"`
}

// DocPlannerPayload encapsulates all inputs needed for planning documentation queries.
type DocPlannerPayload struct {
	Packages []DocPackageDependency `json:"packages"`
	File     DocPlannerFilePayload  `json:"file"`
}

// DocumentationQueryTask pairs a package name with a specific search query.
type DocumentationQueryTask struct {
	PackageName string `json:"packageName"`
	Query       string `json:"query"`
}

// DocPlannerResult is the schema expected from the documentation planner LLM.
type DocPlannerResult struct {
	QueryTasks []DocumentationQueryTask `json:"queryTasks"`
}

// PromptCodeReviewDocumentationPlannerSystem returns the system instructions for query planning.
func PromptCodeReviewDocumentationPlannerSystem() string {
	return `You are an expert software documentation planner. Given a source code file, a diff of changes, and a list of repository packages, you pinpoint which packages are most relevant specifically to the proposed code change (the diff) and generate laser-focused documentation search queries.

Your core philosophy is "less is more". Your absolute focus is the provided diff of changes. You must only generate queries for APIs, functions, or concepts that are actively being added, modified, or uniquely impacted in the diff.

Always bias selections toward complex, high-leverage dependencies (frameworks, ORMs, cloud SDKs) that are present in the diff, and ignore packages that are just part of the surrounding file context but not part of the actual code change.

Focus strictly on finding practical implementation guidance for the specific syntax, methods, or API usage introduced or altered in the diff.`
}

// PromptCodeReviewDocumentationPlannerUser formats the user prompt for documentation query generation.
func PromptCodeReviewDocumentationPlannerUser(payload DocPlannerPayload) string {
	var pkgLines []string
	for _, pkg := range payload.Packages {
		v := ""
		if pkg.Version != "" {
			v = "@" + pkg.Version
		}
		pkgLines = append(pkgLines, fmt.Sprintf("- %s%s (%s) from %s", pkg.Name, v, pkg.Ecosystem, pkg.SourceFile))
	}
	packagesPreview := strings.Join(pkgLines, "\n")
	if packagesPreview == "" {
		packagesPreview = "- (no packages provided)"
	}

	fileContentPreview := payload.File.FileContent
	if fileContentPreview == "" {
		fileContentPreview = "(empty)"
	}

	diffPreview := payload.File.Diff
	if diffPreview == "" {
		diffPreview = "(empty)"
	}

	return fmt.Sprintf(`Analyze the target diff and repository package dependencies to propose highly targeted documentation searches.

Rules:
- Return JSON only following the configured parser schema.
- Focus strictly on the changed lines (diff excerpt). Do not generate queries for existing code in the file content excerpt that was not modified.
- Only include a package if its API, method, or class is directly added, altered, or manipulated within the diff excerpt.
- Prioritize complex/runtime-defining packages first (frameworks, platforms, ORMs, cloud/infra SDKs, auth).
- Ignore low-complexity dependencies (utilities, linters, types) unless they are the primary subject of the diff.
- Queries should be highly specific to the exact functions, hooks, or classes changed in the diff rather than broad conceptual overviews.
- Use 'en-US' for query text.
- Each queryTask must contain both packageName and query. Do not return unpaired package or query arrays.
- Every query string must include: the language, the package name, and an explicit preference for official documentation.
- Preferred query template: "Language: <language>. Package: <package>. <specific API/use-case>. Prefer official vendor/maintainer documentation."
- Hard cap: return AT MOST %d queryTasks for this file. If more candidates would qualify, drop the lowest-priority ones (utilities, types, low-complexity deps). Quality over quantity.

Target file: %s
Language: %s

Repository packages:
%s

Target file content excerpt (for context only):
%s

Target diff excerpt (YOUR SOLE FOCUS):
%s

Output instructions: Return queryTasks only. Each queryTask must include packageName and documentation-oriented query, explicitly paired in the same item. Base your queryTasks *exclusively* on the additions and modifications shown in the target diff excerpt. Avoid generic package documentation searches.

Even if there's no queryTasks to return, respond with an empty array for queryTasks, not an empty object or null.
There must always be an object with a queryTasks property in the output, even if it's an empty array. Do not omit the queryTasks property.

Output JSON schema:
`+"```"+`
{
    "queryTasks": [
        {
            "packageName": "string",
            "query": "string"
        }
    ]
}
`+"```"+`
`, MAX_QUERY_TASKS_PER_FILE, payload.File.FilePath, payload.File.Language, packagesPreview, fileContentPreview, diffPreview)
}

// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Business Rules Validation Agent
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package businessrules

import (
	"fmt"
	"regexp"
	"strings"
)

const defaultPromptLanguage = "en-US"

var (
	taskIdRegex     = regexp.MustCompile(`(?i)\b(?:task|ticket|issue)\s*(?:id|key)\s*[:#-]?\s*([A-Z][A-Z0-9]+-\d+|\d+)\b`)
	issueKeyRegex   = regexp.MustCompile(`\b([A-Z][A-Z0-9]+-\d+)\b`)
	taskTitleRegex  = regexp.MustCompile(`(?im)^\s*title\s*:\s*(.+)$`)
	urlRegex        = regexp.MustCompile(`https?://[^\s)]+`)
	bulletListRegex = regexp.MustCompile(`(?i)^(?:[-*]\s*(?:\[[ x]]\s*)?|\d+\.\s+)(.+)$`)
)

// BuildBusinessRulesAnalysisPrompt constructs the structured gap analysis prompt for LLM evaluation.
func BuildBusinessRulesAnalysisPrompt(ctx BusinessRulesContext) string {
	criteria := formatAcceptanceCriteria(ctx)
	meta := resolveTaskMetadata(ctx)

	lang := ctx.UserLanguage
	if strings.TrimSpace(lang) == "" {
		lang = defaultPromptLanguage
	}

	taskQuality := ctx.TaskQuality
	if taskQuality == "" {
		taskQuality = "adequate"
	}

	var sections []string
	sections = append(sections,
		"Perform business rules gap analysis.",
		"",
		fmt.Sprintf("TASK_QUALITY: %s", taskQuality),
	)

	taskLabel := strings.TrimSpace(fmt.Sprintf("%s — %s", meta.ID, meta.Title))
	taskLabel = strings.Trim(taskLabel, "— ")
	if taskLabel != "" {
		sections = append(sections, "", fmt.Sprintf("TASK: %s", taskLabel))
	}

	if len(meta.Links) > 0 {
		sections = append(sections, "", "TASK_LINKS:", strings.Join(meta.Links, "\n"))
	}

	sections = append(sections,
		"",
		"ACCEPTANCE_CRITERIA:",
		criteria,
		"",
		"FULL_TASK_CONTEXT:",
		formatPromptValue(ctx.TaskContext, "(none)"),
		"",
		"PR_DIFF:",
		formatPromptValue(ctx.PRDiff, "(not available)"),
		"",
		"PR_DESCRIPTION:",
		formatPromptValue(ctx.PRBody, "(not available)"),
		"",
		fmt.Sprintf("USER LANGUAGE: %s", lang),
		"",
		"INSTRUCTIONS:",
		"1. Check EACH acceptance criterion against the PR_DIFF. For each one, determine: IMPLEMENTED, MISSING, or PARTIAL.",
		"2. Scan for any task requirements in FULL_TASK_CONTEXT not covered by the acceptance criteria list.",
		"3. Check for regressions or violated architectural business rules.",
		fmt.Sprintf("4. Write ALL generated prose (summary, findings, suggestions) in %s.", lang),
		"5. Return a strict JSON response adhering to the output format: is_compliant, needs_more_info, summary, missing_requirements, violated_rules, suggestions.",
	)

	return strings.Join(sections, "\n")
}

func resolveTaskMetadata(ctx BusinessRulesContext) TaskMetadata {
	meta := TaskMetadata{}
	if ctx.TaskContextNormalized != nil {
		meta = *ctx.TaskContextNormalized
	}

	if meta.ID == "" {
		meta.ID = ExtractTaskIDFromText(ctx.TaskContext)
	}
	if meta.Title == "" {
		meta.Title = ExtractTaskTitleFromText(ctx.TaskContext)
	}
	if meta.Description == "" {
		meta.Description = ctx.TaskContext
	}
	if len(meta.AcceptanceCriteria) == 0 {
		meta.AcceptanceCriteria = ExtractCriteriaFromText(ctx.TaskContext)
	}

	linksFromText := ExtractLinksFromText(ctx.TaskContext)
	allLinks := append(meta.Links, linksFromText...)
	meta.Links = uniqueNonEmpty(allLinks)

	return meta
}

func formatPromptValue(val, fallback string) string {
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func formatAcceptanceCriteria(ctx BusinessRulesContext) string {
	if ctx.TaskContextNormalized != nil && len(ctx.TaskContextNormalized.AcceptanceCriteria) > 0 {
		var lines []string
		for i, ac := range ctx.TaskContextNormalized.AcceptanceCriteria {
			lines = append(lines, fmt.Sprintf("%d. %q", i+1, ac))
		}
		return strings.Join(lines, "\n")
	}

	extracted := ExtractCriteriaFromText(ctx.TaskContext)
	if len(extracted) > 0 {
		var lines []string
		for i, ac := range extracted {
			lines = append(lines, fmt.Sprintf("%d. %q (extracted from task description)", i+1, ac))
		}
		return strings.Join(lines, "\n")
	}

	return "(no structured acceptance criteria available — use FULL_TASK_CONTEXT to identify requirements)"
}

// ExtractCriteriaFromText extracts bullet-point requirements from markdown text.
func ExtractCriteriaFromText(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	var criteria []string
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		match := bulletListRegex.FindStringSubmatch(trimmed)
		if len(match) > 1 {
			content := strings.TrimSpace(match[1])
			if urlRegex.MatchString(content) && len(content) < 80 && !strings.Contains(content, " ") {
				continue
			}
			if len(content) > 10 && !strings.HasPrefix(content, "#") {
				criteria = append(criteria, content)
			}
		}
	}

	return criteria
}

// ExtractTaskIDFromText searches for Jira/Linear/issue keys like PROJ-123.
func ExtractTaskIDFromText(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	match := taskIdRegex.FindStringSubmatch(text)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}

	match2 := issueKeyRegex.FindStringSubmatch(text)
	if len(match2) > 1 {
		return strings.TrimSpace(match2[1])
	}

	return ""
}

// ExtractTaskTitleFromText finds 'Title: ...' in task descriptions.
func ExtractTaskTitleFromText(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	match := taskTitleRegex.FindStringSubmatch(text)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}

	return ""
}

// ExtractLinksFromText finds URLs in task text.
func ExtractLinksFromText(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	matches := urlRegex.FindAllString(text, -1)
	return uniqueNonEmpty(matches)
}

func uniqueNonEmpty(items []string) []string {
	seen := make(map[string]bool)
	var res []string
	for _, item := range items {
		clean := strings.TrimSpace(item)
		if clean != "" && !seen[clean] {
			seen[clean] = true
			res = append(res, clean)
		}
	}
	return res
}

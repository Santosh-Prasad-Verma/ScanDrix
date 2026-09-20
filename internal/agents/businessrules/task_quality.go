// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package businessrules

import (
	"regexp"
	"strings"
)

// TaskQuality defines the completeness level of the task context.
type TaskQuality string

const (
	TaskQualityEmpty    TaskQuality = "EMPTY"
	TaskQualityMinimal  TaskQuality = "MINIMAL"
	TaskQualityPartial  TaskQuality = "PARTIAL"
	TaskQualityComplete TaskQuality = "COMPLETE"
)

// TaskContextStatus indicates whether the task context is usable for evaluation.
type TaskContextStatus string

const (
	TaskContextStatusMissing TaskContextStatus = "missing"
	TaskContextStatusWeak    TaskContextStatus = "weak"
	TaskContextStatusUsable  TaskContextStatus = "usable"
)

// PrDiffStatus indicates whether the pull request diff is present.
type PrDiffStatus string

const (
	PrDiffStatusMissing PrDiffStatus = "missing"
	PrDiffStatusUsable  PrDiffStatus = "usable"
)

// EligibilityMode dictates whether the engine should perform full analysis or return limitation guidance.
type EligibilityMode string

const (
	EligibilityModeFullAnalysis       EligibilityMode = "full_analysis"
	EligibilityModeLimitationResponse EligibilityMode = "limitation_response"
)

// BusinessLogicEligibility encapsulates the pre-flight evaluation of task and diff readiness.
type BusinessLogicEligibility struct {
	Mode              EligibilityMode   `json:"mode"`
	Reason            string            `json:"reason"`
	TaskContextStatus TaskContextStatus `json:"task_context_status"`
	PrDiffStatus      PrDiffStatus      `json:"pr_diff_status"`
}

const (
	TaskQualityClassificationGuide = "EMPTY (no task found), MINIMAL (title only), PARTIAL (some description), COMPLETE (description + acceptance criteria)"
	TaskQualityAnalyzerPolicy      = "- EMPTY => needsMoreInfo = true\n- MINIMAL => needsMoreInfo = true\n- PARTIAL => proceed with full gap analysis\n- COMPLETE => proceed with full gap analysis\n- Never proceed using only PR description as task context."
)

var (
	statusRegex = regexp.MustCompile(`(?i)status:\s*\d{3}`)
)

// NormalizeTaskQuality validates and casts a string into a known TaskQuality enum value.
func NormalizeTaskQuality(raw string) TaskQuality {
	upper := strings.ToUpper(strings.TrimSpace(raw))
	switch upper {
	case "MINIMAL":
		return TaskQualityMinimal
	case "PARTIAL":
		return TaskQualityPartial
	case "COMPLETE":
		return TaskQualityComplete
	default:
		return TaskQualityEmpty
	}
}

// CanProceedWithBusinessRulesAnalysis determines if the task quality is rich enough for analysis.
func CanProceedWithBusinessRulesAnalysis(quality TaskQuality) bool {
	q := NormalizeTaskQuality(string(quality))
	return q == TaskQualityPartial || q == TaskQualityComplete
}

// HasUsablePullRequestDiff checks whether diff content exists.
func HasUsablePullRequestDiff(prDiff string) bool {
	return strings.TrimSpace(prDiff) != ""
}

// ClassifyTaskContextStatus assesses whether task context is usable, weak, or missing.
func ClassifyTaskContextStatus(quality TaskQuality, taskContext string, meta *TaskMetadata) TaskContextStatus {
	q := NormalizeTaskQuality(string(quality))
	if q == TaskQualityEmpty {
		return TaskContextStatusMissing
	}
	if q == TaskQualityMinimal {
		return TaskContextStatusWeak
	}

	description := taskContext
	if meta != nil && strings.TrimSpace(meta.Description) != "" {
		description = meta.Description
	}

	if looksLikeStructuredMetadata(description) {
		return TaskContextStatusWeak
	}
	if looksLikeTaskFetchFailure(description) {
		return TaskContextStatusWeak
	}

	return TaskContextStatusUsable
}

// ClassifyPrDiffStatus assesses whether the PR diff is usable.
func ClassifyPrDiffStatus(prDiff string) PrDiffStatus {
	if HasUsablePullRequestDiff(prDiff) {
		return PrDiffStatusUsable
	}
	return PrDiffStatusMissing
}

// BuildBusinessLogicEligibility checks both task context and PR diff to determine the execution mode.
func BuildBusinessLogicEligibility(
	quality TaskQuality,
	taskContext string,
	prDiff string,
	meta *TaskMetadata,
) BusinessLogicEligibility {
	taskContextStatus := ClassifyTaskContextStatus(quality, taskContext, meta)
	diffStatus := ClassifyPrDiffStatus(prDiff)

	if taskContextStatus == TaskContextStatusMissing {
		return BusinessLogicEligibility{
			Mode:              EligibilityModeLimitationResponse,
			Reason:            "task_context_missing",
			TaskContextStatus: taskContextStatus,
			PrDiffStatus:      diffStatus,
		}
	}

	if taskContextStatus == TaskContextStatusWeak {
		return BusinessLogicEligibility{
			Mode:              EligibilityModeLimitationResponse,
			Reason:            "task_context_weak",
			TaskContextStatus: taskContextStatus,
			PrDiffStatus:      diffStatus,
		}
	}

	if diffStatus == PrDiffStatusMissing {
		return BusinessLogicEligibility{
			Mode:              EligibilityModeLimitationResponse,
			Reason:            "pr_diff_missing",
			TaskContextStatus: taskContextStatus,
			PrDiffStatus:      diffStatus,
		}
	}

	return BusinessLogicEligibility{
		Mode:              EligibilityModeFullAnalysis,
		Reason:            "analysis_ready",
		TaskContextStatus: taskContextStatus,
		PrDiffStatus:      diffStatus,
	}
}

// GetTaskContextMissingInfoMessage constructs a formatted markdown guide when task information is deficient.
func GetTaskContextMissingInfoMessage(quality TaskQuality) string {
	q := NormalizeTaskQuality(string(quality))
	if q == TaskQualityMinimal {
		return buildMinimalContextMessage()
	}
	if q == TaskQualityEmpty {
		return buildEmptyContextMessage()
	}
	return buildWeakContextMessage()
}

// GetPullRequestDiffMissingInfoMessage constructs markdown advice when PR diff is absent.
func GetPullRequestDiffMissingInfoMessage() string {
	return `## 🤔 Need Pull Request Diff

I found enough task context to understand the expected behavior, but I couldn't load the pull request diff. Without the actual code changes, I can't validate whether the implementation matches the business requirements.

### 🔍 What I need to validate:
- The files and code paths changed in this PR
- The exact implementation compared to the task requirements
- Any regressions or missing business-rule coverage

### 💡 How to fix it:
- Ensure the PR diff tool is available and returns the patch content
- Retry the validation after the pull request diff is fetched successfully

### ⚠️ Important:
Business rules validation requires both the task context and the code diff.`
}

func buildEmptyContextMessage() string {
	return `## 🤔 Need Task Information

I couldn't find any task information associated with this pull request. To perform a proper business rules validation, I need context about what this PR is supposed to implement.

### 🔍 What I need to validate:
- Task title and description
- Acceptance criteria or business requirements
- Expected behavior and business rules

### 💡 Examples of how to provide it:
- Link a Jira/Linear/GitHub issue in the PR description
- Add a task URL in the PR body
- Include acceptance criteria directly in the PR description

### ⚠️ Important:
Business rules validation requires understanding **what** should be implemented, not just **what** was changed.`
}

func buildMinimalContextMessage() string {
	return `## 🤔 Insufficient Task Context

I found a task linked to this PR, but it only contains minimal information (title only, no description or acceptance criteria). To perform a meaningful business rules validation, I need more details.

### 🔍 What I need to validate:
- Business requirements and acceptance criteria
- Expected behavior and business rules
- Edge cases and constraints to consider

### 💡 How to improve the task context:
- Add a description to the linked ticket
- Include acceptance criteria or business rules
- Describe the expected behavior after the change

### ⚠️ Important:
A task title alone is not sufficient to determine whether the implementation is correct or complete.`
}

func buildWeakContextMessage() string {
	return `## 🤔 Limited Task Context

I found task-related information for this pull request, but the available context is still too weak or too ambiguous to support a reliable business rules validation.

### 🔍 What I still need to validate:
- Explicit business requirements or acceptance criteria
- Expected behavior and scope boundaries
- Enough detail to compare intended behavior against the PR diff

### 💡 How to improve the task context:
- Link the canonical Jira/Linear/GitHub issue instead of only a smart link or metadata card
- Add a clear task description and acceptance criteria
- Include the expected behavior directly in the PR description when the task is incomplete

### ⚠️ Important:
Without grounded requirement details, any business-rules finding would be speculative.`
}

func looksLikeStructuredMetadata(value string) bool {
	trimmed := strings.TrimSpace(value)
	if !(strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) {
		return false
	}

	return strings.Contains(trimmed, `"inlineCard"`) ||
		strings.Contains(trimmed, `"blockCard"`) ||
		strings.Contains(trimmed, `"application"`) ||
		strings.Contains(trimmed, `"attrs"`) ||
		strings.Contains(trimmed, `"url"`)
}

func looksLikeTaskFetchFailure(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return false
	}

	hasErrorMarkers := strings.Contains(normalized, `"error":true`) ||
		strings.Contains(normalized, "failed to fetch") ||
		statusRegex.MatchString(value)

	if !hasErrorMarkers {
		return false
	}

	return strings.Contains(normalized, "tenant info") ||
		strings.Contains(normalized, "task context") ||
		strings.Contains(normalized, "cloud id") ||
		strings.Contains(normalized, "jira") ||
		strings.Contains(normalized, "linear")
}

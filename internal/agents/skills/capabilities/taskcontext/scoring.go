package taskcontext

import (
	"strings"
)

// ScoreNormalizedContext calculates a weighted richness score for task context.
func ScoreNormalizedContext(value *TaskContextNormalized) int {
	if value == nil {
		return 0
	}
	score := 0
	if value.ID != "" {
		score += 1
	}
	if value.Title != "" {
		score += 3
	}
	if value.Description != "" {
		score += 4
	}
	if len(value.AcceptanceCriteria) > 0 {
		score += 2
	}
	if len(value.Links) > 0 {
		score += 1
	}
	return score
}

// IsUsableTaskContext checks whether extracted context is substantive and not an error.
func IsUsableTaskContext(value *TaskContextNormalized) bool {
	if value == nil {
		return false
	}
	if len(value.AcceptanceCriteria) > 0 {
		return true
	}
	if strings.TrimSpace(value.Description) == "" {
		return false
	}
	if looksLikeTaskContextFailure(value) {
		return false
	}
	return !looksLikeStructuredMetadata(value.Description)
}

func looksLikeStructuredMetadata(value string) bool {
	trimmed := strings.TrimSpace(value)
	if !(strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) {
		return false
	}

	return strings.Contains(trimmed, "\"inlineCard\"") ||
		strings.Contains(trimmed, "\"blockCard\"") ||
		strings.Contains(trimmed, "\"application\"") ||
		strings.Contains(trimmed, "\"attrs\"") ||
		strings.Contains(trimmed, "\"url\"")
}

func looksLikeTaskContextFailure(value *TaskContextNormalized) bool {
	combined := strings.ToLower(strings.TrimSpace(value.Title + " " + value.Description))
	if combined == "" {
		return false
	}

	failureIndicators := []string{
		"failed to fetch",
		"status: 404",
		"status 404",
		"status: 403",
		"status 403",
		"status: 401",
		"status 401",
		"not found",
		"unauthorized",
		"forbidden",
		"tenant info",
		"mcp error",
		"input validation error",
		"invalid arguments",
		"-32602",
	}

	for _, ind := range failureIndicators {
		if strings.Contains(combined, ind) {
			return true
		}
	}
	return false
}

// LooksLikeToolErrorCandidate inspects raw payload for error structures.
func LooksLikeToolErrorCandidate(payload any) bool {
	if payload == nil {
		return true
	}

	m, ok := payload.(map[string]any)
	if !ok {
		return false
	}

	if isErr, _ := m["isError"].(bool); isErr {
		return true
	}
	if _, hasErr := m["error"]; hasErr {
		return true
	}

	msg := FirstNonEmptyString(m["message"], m["errorMessage"], m["detail"])
	if msg != "" {
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "error") ||
			strings.Contains(lower, "failed") ||
			strings.Contains(lower, "not found") ||
			strings.Contains(lower, "unauthorized") {
			return true
		}
	}

	return false
}

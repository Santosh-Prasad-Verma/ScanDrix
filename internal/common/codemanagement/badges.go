// Package codemanagement provides shields badge generation and severity markers.
package codemanagement

import (
	"fmt"
	"strings"
)

// Shield colors for severity badges.
const (
	ShieldColorLowBlue     = "1A8EBC"
	ShieldColorMediumBlue  = "1A7BBE"
	ShieldColorHighPurple  = "6B6B92"
	ShieldColorCriticalRed = "FF3D3D"
)

// SeverityLevel represents the normalized issue severity.
type SeverityLevel string

const (
	SeverityCritical SeverityLevel = "critical"
	SeverityHigh     SeverityLevel = "high"
	SeverityMedium   SeverityLevel = "medium"
	SeverityLow      SeverityLevel = "low"
)

// NormalizeSeverityLevel returns the standardized SeverityLevel or empty string.
func NormalizeSeverityLevel(raw string) SeverityLevel {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "critical":
		return SeverityCritical
	case "high":
		return SeverityHigh
	case "medium":
		return SeverityMedium
	case "low":
		return SeverityLow
	default:
		return ""
	}
}

// GetSeverityLevelShield generates a Shields.io Markdown badge for the given severity.
func GetSeverityLevelShield(severity string) string {
	normalized := NormalizeSeverityLevel(severity)
	if normalized == "" {
		return ""
	}

	labelTitle := "severity_level"
	shieldBase := fmt.Sprintf("![%s](https://img.shields.io/badge/%s-%s-", normalized, labelTitle, normalized)

	switch normalized {
	case SeverityLow:
		return shieldBase + ShieldColorLowBlue + ")"
	case SeverityMedium:
		return shieldBase + ShieldColorMediumBlue + ")"
	case SeverityHigh:
		return shieldBase + ShieldColorHighPurple + ")"
	case SeverityCritical:
		return shieldBase + ShieldColorCriticalRed + ")"
	default:
		return ""
	}
}

// GetCodeReviewBadge returns the standard ScanDrix code review markdown badge.
func GetCodeReviewBadge() string {
	return `![scandrix code-review](https://img.shields.io/badge/scandrix-code--review-312B4B?labelColor=C9BBF2)`
}

// CalculateRankScore calculates the numeric priority rank score based on severity.
func CalculateRankScore(severity string) int {
	switch NormalizeSeverityLevel(severity) {
	case SeverityCritical:
		return 100
	case SeverityHigh:
		return 75
	case SeverityMedium:
		return 50
	case SeverityLow:
		return 25
	default:
		return 10
	}
}

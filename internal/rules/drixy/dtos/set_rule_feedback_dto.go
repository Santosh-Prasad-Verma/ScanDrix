// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: set_rule_feedback_dto.go
// ═══════════════════════════════════════════════════════════════

package dtos

// SetRuleFeedbackDTO captures user sentiment ("positive" or "negative") on a catalog or custom rule.
type SetRuleFeedbackDTO struct {
	Feedback string `json:"feedback"` // "positive" or "negative"
}

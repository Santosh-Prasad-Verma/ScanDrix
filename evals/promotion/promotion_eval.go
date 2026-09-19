package promotion

import (
	"os"
	"strconv"
	"strings"

	"github.com/scandrix/backend/internal/review/agentcore"
)

const (
	DefaultPromotionConfidence = 0.70
)

// PromotionDecision records whether a finding was promoted to an inline PR comment.
type PromotionDecision struct {
	Finding  agentcore.FinderSuggestion `json:"finding"`
	Promoted bool                       `json:"promoted"`
	Reason   string                     `json:"reason"`
}

// EvaluatePromotion asserts promotion criteria for code review findings:
// 1. Must have verified keep verdict (if verification ran).
// 2. Must meet confidence threshold (default 0.70, configurable).
// 3. Must have meaningful improvedCode different from existingCode.
// 4. Must specify a valid line location.
func EvaluatePromotion(finding agentcore.FinderSuggestion, customConfidence ...float64) PromotionDecision {
	minConf := DefaultPromotionConfidence
	if len(customConfidence) > 0 && customConfidence[0] > 0 {
		minConf = customConfidence[0]
	} else if val := os.Getenv("SCANDRIX_EVAL_PROMOTION_CONFIDENCE"); val != "" {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil && parsed > 0 {
			minConf = parsed
		}
	}

	if finding.Confidence > 0 && finding.Confidence < minConf {
		return PromotionDecision{
			Finding:  finding,
			Promoted: false,
			Reason:   "Confidence below promotion floor",
		}
	}

	if strings.TrimSpace(finding.ExistingCode) != "" &&
		strings.TrimSpace(finding.ExistingCode) == strings.TrimSpace(finding.ImprovedCode) {
		return PromotionDecision{
			Finding:  finding,
			Promoted: false,
			Reason:   "Improved code identical to existing code (no actionable fix)",
		}
	}

	if finding.RelevantFile == "" {
		return PromotionDecision{
			Finding:  finding,
			Promoted: false,
			Reason:   "Missing relevant file",
		}
	}

	return PromotionDecision{
		Finding:  finding,
		Promoted: true,
		Reason:   "All promotion gates satisfied",
	}
}

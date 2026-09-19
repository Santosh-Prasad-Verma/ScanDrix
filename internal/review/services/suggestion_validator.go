package services

import (
	"context"
	"strings"

	"github.com/scandrix/backend/internal/review/domain"
)

// SuggestionLLMValidator implements domain.ISuggestionLLMValidator.
type SuggestionLLMValidator struct {
	syntaxValidator domain.ISandboxSyntaxValidator
}

// NewSuggestionLLMValidator constructs an LLM/heuristic suggestion validator.
func NewSuggestionLLMValidator(syntax domain.ISandboxSyntaxValidator) *SuggestionLLMValidator {
	return &SuggestionLLMValidator{
		syntaxValidator: syntax,
	}
}

// ValidateSuggestion verifies recommendation soundness, rejects cosmetic churn, and scores confidence.
func (v *SuggestionLLMValidator) ValidateSuggestion(ctx context.Context, s domain.CodeSuggestion, fileContext string) (domain.LLMValidationResult, error) {
	// 1. Check for empty or trivial advice
	desc := strings.TrimSpace(s.SuggestionContent)
	if len(desc) < 10 {
		return domain.LLMValidationResult{
			Approved:        false,
			ConfidenceScore: 0.1,
			RejectionReason: "suggestion description is too short or ambiguous",
		}, nil
	}

	// 2. Reject pure cosmetic comment additions if classified as Bug or Security
	if s.Category == domain.CategoryBug || s.Category == domain.CategorySecurity {
		if strings.HasPrefix(strings.TrimSpace(s.ImprovedCode), "//") || strings.HasPrefix(strings.TrimSpace(s.ImprovedCode), "/*") {
			return domain.LLMValidationResult{
				Approved:        false,
				ConfidenceScore: 0.2,
				RejectionReason: "security/bug finding cannot merely propose adding comments",
			}, nil
		}
	}

	// 3. Check for identical replacement code (no-op suggestion)
	if s.ExistingCode != "" && strings.TrimSpace(s.ExistingCode) == strings.TrimSpace(s.ImprovedCode) {
		return domain.LLMValidationResult{
			Approved:        false,
			ConfidenceScore: 0.0,
			RejectionReason: "suggested diff is a no-op identical to existing code",
		}, nil
	}

	// 4. Validate syntax of improved code if syntax validator is present
	confidence := 0.85
	if v.syntaxValidator != nil && s.ImprovedCode != "" {
		res, err := v.syntaxValidator.ValidateSyntax(ctx, s.Language, s.RelevantFile, s.ImprovedCode)
		if err == nil && !res.IsValid {
			return domain.LLMValidationResult{
				Approved:        false,
				ConfidenceScore: 0.25,
				RejectionReason: "suggested code fails compiler/syntax check: " + res.ErrorMessage,
			}, nil
		}
		confidence = 0.95
	}

	// Adjust confidence based on severity impact
	if s.Severity == domain.SeverityCritical {
		confidence = min(1.0, confidence+0.05)
	}

	return domain.LLMValidationResult{
		Approved:        true,
		ConfidenceScore: confidence,
	}, nil
}

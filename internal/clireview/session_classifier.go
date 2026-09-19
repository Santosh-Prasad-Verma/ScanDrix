package clireview

import (
	"context"
	"regexp"
	"strings"
)

// SessionClassifier extracts structured decisions from coding sessions.
type SessionClassifier struct{}

// NewSessionClassifier creates a new session classifier.
func NewSessionClassifier() *SessionClassifier {
	return &SessionClassifier{}
}

// ClassifySession processes a session capture and populates its classified decisions.
func (sc *SessionClassifier) ClassifySession(ctx context.Context, capture *CliSessionCapture) {
	if capture == nil {
		return
	}

	if capture.Event != "stop" && capture.Event != "commit" && capture.Event != "review" {
		capture.Status = "skipped"
		capture.ErrorMessage = "Unsupported event: " + capture.Event
		return
	}

	var textParts []string
	if capture.Summary != "" {
		textParts = append(textParts, capture.Summary)
	}
	if capture.Signals != nil {
		if capture.Signals.Prompt != "" {
			textParts = append(textParts, capture.Signals.Prompt)
		}
		if capture.Signals.AssistantMessage != "" {
			textParts = append(textParts, capture.Signals.AssistantMessage)
		}
	}

	if len(textParts) == 0 {
		capture.Status = "skipped"
		capture.ErrorMessage = "No textual context for classification"
		return
	}

	capture.Status = "processing"

	// Extract decisions via heuristic classifier
	decisions := sc.extractWithHeuristics(capture)
	capture.ClassifiedDecisions = decisions
	if len(decisions) > 0 {
		capture.ClassificationSource = "heuristic"
	} else {
		capture.ClassificationSource = "empty"
	}
	capture.Status = "completed"
}

func (sc *SessionClassifier) extractWithHeuristics(capture *CliSessionCapture) []CliSessionClassifiedDecision {
	var parts []string
	if capture.Summary != "" {
		parts = append(parts, capture.Summary)
	}
	if capture.Signals != nil {
		if capture.Signals.Prompt != "" {
			parts = append(parts, capture.Signals.Prompt)
		}
		if capture.Signals.AssistantMessage != "" {
			parts = append(parts, capture.Signals.AssistantMessage)
		}
	}

	fullText := strings.Join(parts, "\n")
	if strings.TrimSpace(fullText) == "" {
		return nil
	}

	// Split into sentences
	sentenceRegex := regexp.MustCompile(`[.\n!?]+`)
	rawSentences := sentenceRegex.Split(fullText, -1)

	var candidateSentences []string
	keywordRegex := regexp.MustCompile(`(?i)(decid|because|trade[- ]?off|prefer|chose|choose|adopt|use|convention|pattern|standard|architect|refactor)`)

	for _, s := range rawSentences {
		trimmed := strings.TrimSpace(s)
		if len(trimmed) > 10 && keywordRegex.MatchString(trimmed) {
			candidateSentences = append(candidateSentences, trimmed)
		}
	}

	selected := candidateSentences
	if len(selected) > 8 {
		selected = selected[:8]
	}

	var evidence []string
	if capture.Signals != nil && len(capture.Signals.ModifiedFiles) > 0 {
		evidence = capture.Signals.ModifiedFiles
		if len(evidence) > 3 {
			evidence = evidence[:3]
		}
	}

	var out []CliSessionClassifiedDecision
	for _, sentence := range selected {
		dType := sc.inferDecisionType(sentence)
		confidence := 0.35
		if len(candidateSentences) == 0 {
			confidence = 0.2
		}

		out = append(out, CliSessionClassifiedDecision{
			Type:                 dType,
			Origin:               OriginCollaborative,
			Decision:             trimString(sentence, 500),
			Confidence:           confidence,
			Evidence:             evidence,
			AutoPromoteCandidate: sc.shouldAutoPromote(dType, confidence),
		})
	}

	return out
}

func (sc *SessionClassifier) inferDecisionType(text string) CliSessionDecisionType {
	lower := strings.ToLower(text)

	if matchAny(lower, "architecture", "architectural", "layer", "module", "schema", "database", "queue", "event", "service boundary", "system design") {
		return DecisionArchitecturalDetail
	}
	if matchAny(lower, "convention", "style", "naming", "format", "lint", "folder structure") {
		return DecisionConvention
	}
	if matchAny(lower, "trade-off", "tradeoff", "versus", "vs.", "instead of", "however", "but") {
		return DecisionTradeoff
	}
	if matchAny(lower, "tool", "framework", "library", "package", "cli", "sdk", "dependency") {
		return DecisionTooling
	}
	if matchAny(lower, "implement", "refactor", "validation", "jwt", "cache", "middleware", "repository", "endpoint", "handler") {
		return DecisionImplementation
	}
	return DecisionOther
}

func (sc *SessionClassifier) shouldAutoPromote(dType CliSessionDecisionType, confidence float64) bool {
	if confidence < 0.7 {
		return false
	}
	return dType == DecisionArchitecturalDetail || dType == DecisionConvention || dType == DecisionTradeoff
}

func matchAny(s string, keywords ...string) bool {
	for _, k := range keywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func trimString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

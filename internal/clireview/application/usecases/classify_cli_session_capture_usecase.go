package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/clireview/domain"
)

// ClassifyCliSessionCaptureUseCase processes atomic IDE/CLI session captures into decisions.
type ClassifyCliSessionCaptureUseCase struct {
	captureRepo domain.ICliSessionCaptureRepository
	llmClient   domain.ILLMDecisionClient
	logger      *slog.Logger
}

// NewClassifyCliSessionCaptureUseCase initializes the usecase with repository and LLM client.
func NewClassifyCliSessionCaptureUseCase(
	captureRepo domain.ICliSessionCaptureRepository,
	llmClient domain.ILLMDecisionClient,
) *ClassifyCliSessionCaptureUseCase {
	return &ClassifyCliSessionCaptureUseCase{
		captureRepo: captureRepo,
		llmClient:   llmClient,
		logger:      slog.Default().With("usecase", "ClassifyCliSessionCaptureUseCase"),
	}
}

// Execute performs classification on a single capture record.
func (uc *ClassifyCliSessionCaptureUseCase) Execute(ctx context.Context, captureID string) error {
	capture, err := uc.captureRepo.FindByCaptureID(ctx, captureID)
	if err != nil || capture == nil {
		uc.logger.Warn("Capture not found for classification", "captureId", captureID)
		return nil
	}

	if capture.Event != "stop" {
		_ = uc.captureRepo.MarkSkipped(ctx, captureID, fmt.Sprintf("Unsupported event: %s", capture.Event))
		return nil
	}

	var promptText, assistantText string
	if capture.Signals != nil {
		promptText = capture.Signals.Prompt
		assistantText = capture.Signals.AssistantMessage
	}

	textParts := []string{
		strings.TrimSpace(capture.Summary),
		strings.TrimSpace(promptText),
		strings.TrimSpace(assistantText),
	}

	hasContent := false
	for _, p := range textParts {
		if p != "" {
			hasContent = true
			break
		}
	}

	if !hasContent {
		_ = uc.captureRepo.MarkSkipped(ctx, captureID, "No textual context for classification")
		return nil
	}

	_ = uc.captureRepo.MarkProcessing(ctx, captureID)

	var decisions []domain.CliSessionClassifiedDecision
	var source string

	if uc.llmClient != nil {
		llmDecisions, llmErr := uc.ExtractWithLLM(ctx, capture)
		if llmErr == nil && len(llmDecisions) > 0 {
			decisions = llmDecisions
			source = "llm"
		} else if llmErr != nil {
			uc.logger.Warn("LLM classification failed for CLI session capture, using fallback",
				"captureId", captureID,
				"error", llmErr,
			)
		}
	}

	if len(decisions) == 0 {
		fallback := uc.ExtractWithHeuristics(capture)
		decisions = fallback
		if len(fallback) > 0 {
			if uc.llmClient != nil {
				source = "heuristic-fallback"
			} else {
				source = "heuristic"
			}
		} else {
			source = "empty"
		}
	}

	return uc.captureRepo.MarkCompleted(ctx, captureID, decisions, source)
}

// ExtractWithLLM uses structured LLM inference to extract decisions from capture signals.
func (uc *ClassifyCliSessionCaptureUseCase) ExtractWithLLM(
	ctx context.Context,
	capture *domain.CliSessionCapture,
) ([]domain.CliSessionClassifiedDecision, error) {
	systemPrompt := `You are classifying coding session captures into reusable decisions.

Return ONLY JSON with shape:
{ "decisions": [ { "type": "...", "origin": "...", "decision": "...", "rationale": "...", "confidence": 0.0, "evidence": ["..."] } ] }

Allowed decision types:
- architectural_decision: high-level structure or system choice
- convention: team style/naming/process convention
- tradeoff: explicit compromise between options
- implementation_detail: concrete technical implementation choice
- tooling: tool or framework choice
- other: valid but uncategorized decision

Allowed origin values:
- human: the human explicitly requested or decided this
- agent: the agent proposed and implemented this without the human asking
- collaborative: the agent suggested and the human confirmed/refined

Rules:
- Extract only concrete choices, not generic statements.
- Keep each "decision" concise and self-contained.
- confidence must be between 0 and 1.
- If nothing useful exists, return { "decisions": [] }.`

	var promptText, assistantText string
	var modifiedFiles []string
	var toolUses []domain.CliSessionToolUse
	if capture.Signals != nil {
		promptText = capture.Signals.Prompt
		assistantText = capture.Signals.AssistantMessage
		modifiedFiles = capture.Signals.ModifiedFiles
		toolUses = capture.Signals.ToolUses
	}

	payloadObj := map[string]any{
		"summary":          capture.Summary,
		"prompt":           promptText,
		"assistantMessage": assistantText,
		"modifiedFiles":    modifiedFiles,
		"toolUses":         toolUses,
	}

	payloadBytes, _ := json.Marshal(payloadObj)
	rawDecisions, err := uc.llmClient.ExtractDecisions(ctx, systemPrompt, string(payloadBytes))
	if err != nil {
		return nil, err
	}

	var results []domain.CliSessionClassifiedDecision
	for _, d := range rawDecisions {
		conf := uc.NormalizeConfidence(d.Confidence)
		dType := d.Type
		origin := d.Origin

		results = append(results, domain.CliSessionClassifiedDecision{
			Type:                 dType,
			Origin:               origin,
			Decision:             uc.trim(d.Decision, 500),
			Rationale:            uc.trim(d.Rationale, 1000),
			Confidence:           conf,
			Evidence:             uc.sliceStrings(d.Evidence, 5),
			AutoPromoteCandidate: uc.ShouldAutoPromote(dType, conf),
		})
	}

	return results, nil
}

// ExtractWithHeuristics generates decisions without requiring model inference.
func (uc *ClassifyCliSessionCaptureUseCase) ExtractWithHeuristics(capture *domain.CliSessionCapture) []domain.CliSessionClassifiedDecision {
	var promptText, assistantText string
	var modifiedFiles []string
	if capture.Signals != nil {
		promptText = capture.Signals.Prompt
		assistantText = capture.Signals.AssistantMessage
		modifiedFiles = capture.Signals.ModifiedFiles
	}

	var parts []string
	if strings.TrimSpace(capture.Summary) != "" {
		parts = append(parts, capture.Summary)
	}
	if strings.TrimSpace(promptText) != "" {
		parts = append(parts, promptText)
	}
	if strings.TrimSpace(assistantText) != "" {
		parts = append(parts, assistantText)
	}

	sourceText := strings.Join(parts, "\n")
	if strings.TrimSpace(sourceText) == "" {
		return []domain.CliSessionClassifiedDecision{}
	}

	var sentences []string
	for _, line := range strings.Split(sourceText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var cur strings.Builder
		for i := 0; i < len(line); i++ {
			ch := line[i]
			cur.WriteByte(ch)
			if (ch == '.' || ch == '!' || ch == '?') && (i+1 == len(line) || line[i+1] == ' ') {
				s := strings.TrimSpace(cur.String())
				if s != "" {
					sentences = append(sentences, s)
				}
				cur.Reset()
				if i+1 < len(line) && line[i+1] == ' ' {
					i++
				}
			}
		}
		if rem := strings.TrimSpace(cur.String()); rem != "" {
			sentences = append(sentences, rem)
		}
		if len(sentences) >= 80 {
			break
		}
	}

	reMatch := regexp.MustCompile(`(?i)(decid|because|trade[- ]?off|prefer|chose|choose|adopt|use|convention|pattern|standard)`)
	var candidateSentences []string
	for _, s := range sentences {
		if reMatch.MatchString(s) {
			candidateSentences = append(candidateSentences, s)
		}
	}

	var selected []string
	confidence := 0.2
	if len(candidateSentences) > 0 {
		limit := len(candidateSentences)
		if limit > 8 {
			limit = 8
		}
		selected = candidateSentences[:limit]
		confidence = 0.35
	} else {
		limit := len(sentences)
		if limit > 3 {
			limit = 3
		}
		selected = sentences[:limit]
	}

	evidence := uc.sliceStrings(modifiedFiles, 3)

	var results []domain.CliSessionClassifiedDecision
	for _, sentence := range selected {
		dType := uc.InferDecisionType(sentence)
		results = append(results, domain.CliSessionClassifiedDecision{
			Type:                 dType,
			Decision:             uc.trim(sentence, 500),
			Confidence:           confidence,
			Evidence:             evidence,
			AutoPromoteCandidate: uc.ShouldAutoPromote(dType, confidence),
		})
	}

	return results
}

// InferDecisionType maps sentence keywords to a decision category.
func (uc *ClassifyCliSessionCaptureUseCase) InferDecisionType(text string) domain.CliSessionDecisionType {
	val := strings.ToLower(text)

	if regexp.MustCompile(`(architecture|architectural|layer|module|schema|database|queue|event|service boundary|system design)`).MatchString(val) {
		return "architectural_decision"
	}
	if regexp.MustCompile(`(convention|style|naming|format|lint|folder structure)`).MatchString(val) {
		return "convention"
	}
	if regexp.MustCompile(`(trade[- ]?off|versus|vs\.|instead of|however|but)`).MatchString(val) {
		return "tradeoff"
	}
	if regexp.MustCompile(`(tool|framework|library|package|cursor|codex|claude|cli|sdk|dependency)`).MatchString(val) {
		return "tooling"
	}
	if regexp.MustCompile(`(implement|refactor|validation|jwt|cache|middleware|repository|endpoint|handler)`).MatchString(val) {
		return "implementation_detail"
	}

	return "other"
}

// ShouldAutoPromote checks if decision meets confidence threshold for auto-promotion.
func (uc *ClassifyCliSessionCaptureUseCase) ShouldAutoPromote(decisionType domain.CliSessionDecisionType, confidence float64) bool {
	if confidence < 0.7 {
		return false
	}
	return decisionType == "architectural_decision" ||
		decisionType == "convention" ||
		decisionType == "tradeoff"
}

// NormalizeConfidence bounds value between 0 and 1.
func (uc *ClassifyCliSessionCaptureUseCase) NormalizeConfidence(val float64) float64 {
	if math.IsNaN(val) || val < 0 {
		return 0
	}
	if val > 1 {
		return 1
	}
	return val
}

func (uc *ClassifyCliSessionCaptureUseCase) trim(s string, maxLen int) string {
	clean := strings.TrimSpace(s)
	if len(clean) <= maxLen {
		return clean
	}
	if maxLen <= 3 {
		return clean[:maxLen]
	}
	return clean[:maxLen-3] + "..."
}

func (uc *ClassifyCliSessionCaptureUseCase) sliceStrings(list []string, maxCount int) []string {
	if len(list) == 0 {
		return []string{}
	}
	var res []string
	for _, item := range list {
		trimmed := uc.trim(item, 300)
		if trimmed != "" {
			res = append(res, trimmed)
		}
		if len(res) >= maxCount {
			break
		}
	}
	if res == nil {
		return []string{}
	}
	return res
}

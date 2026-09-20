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

// SessionTurnPair correlates human prompt with agent response and tooling.
type SessionTurnPair struct {
	Prompt        string   `json:"prompt,omitempty"`
	Response      string   `json:"response,omitempty"`
	ToolCalls     []string `json:"toolCalls"`
	FilesModified []string `json:"filesModified"`
}

// SubagentInfo describes delegated sub-agent invocations.
type SubagentInfo struct {
	Type string `json:"type,omitempty"`
	Task string `json:"task,omitempty"`
}

// AggregatedSession aggregates turn telemetry for holistic evaluation.
type AggregatedSession struct {
	AgentType     string            `json:"agentType,omitempty"`
	GitRemote     string            `json:"gitRemote,omitempty"`
	Turns         []SessionTurnPair `json:"turns"`
	Prompts       []string          `json:"prompts"`
	Responses     []string          `json:"responses"`
	ToolCalls     []string          `json:"toolCalls"`
	FilesModified []string          `json:"filesModified"`
	FilesRead     []string          `json:"filesRead"`
	Commands      []string          `json:"commands"`
	Subagents     []SubagentInfo    `json:"subagents"`
}

// ClassifySessionUseCase transforms session events into structured architectural decisions.
type ClassifySessionUseCase struct {
	eventRepo domain.ISessionEventRepository
	llmClient domain.ILLMDecisionClient
	logger    *slog.Logger
}

// NewClassifySessionUseCase constructs an initialized ClassifySessionUseCase.
func NewClassifySessionUseCase(
	eventRepo domain.ISessionEventRepository,
	llmClient domain.ILLMDecisionClient,
) *ClassifySessionUseCase {
	return &ClassifySessionUseCase{
		eventRepo: eventRepo,
		llmClient: llmClient,
		logger:    slog.Default().With("usecase", "ClassifySessionUseCase"),
	}
}

// Execute aggregates session events and extracts classified decisions.
func (uc *ClassifySessionUseCase) Execute(ctx context.Context, sessionEndEventUUID string) error {
	sessionEndEvent, err := uc.eventRepo.FindByUUID(ctx, sessionEndEventUUID)
	if err != nil || sessionEndEvent == nil {
		uc.logger.Warn("Session end event not found for classification", "uuid", sessionEndEventUUID)
		return nil
	}

	if sessionEndEvent.EventType != "session_end" {
		_ = uc.eventRepo.MarkClassificationSkipped(
			ctx,
			sessionEndEventUUID,
			fmt.Sprintf("Unsupported event type: %s", sessionEndEvent.EventType),
		)
		return nil
	}

	allEvents, err := uc.eventRepo.FindBySessionID(ctx, sessionEndEvent.SessionID, sessionEndEvent.OrganizationID)
	if err != nil {
		uc.logger.Error("Failed to fetch session events", "error", err, "sessionId", sessionEndEvent.SessionID)
		return err
	}

	aggregated := uc.AggregateEvents(allEvents)

	if !uc.HasUsefulContent(aggregated) {
		_ = uc.eventRepo.MarkClassificationSkipped(
			ctx,
			sessionEndEventUUID,
			"No textual context for classification",
		)
		return nil
	}

	_ = uc.eventRepo.MarkClassificationProcessing(ctx, sessionEndEventUUID)

	var decisions []domain.CliSessionClassifiedDecision
	var source string

	if uc.llmClient != nil {
		llmDecisions, llmErr := uc.ExtractWithLLM(ctx, &aggregated, sessionEndEvent.OrganizationID)
		if llmErr == nil && len(llmDecisions) > 0 {
			decisions = llmDecisions
			source = "llm"
		} else if llmErr != nil {
			uc.logger.Warn("LLM classification failed, using fallback", "uuid", sessionEndEventUUID, "error", llmErr)
		}
	}

	if len(decisions) == 0 {
		fallback := uc.ExtractWithHeuristics(aggregated)
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

	err = uc.eventRepo.MarkClassificationCompleted(ctx, sessionEndEventUUID, decisions, source)
	if err != nil {
		_ = uc.eventRepo.MarkClassificationFailed(ctx, sessionEndEventUUID, err.Error())
		return err
	}
	return nil
}

// AggregateEvents merges chronological events into structured session history.
func (uc *ClassifySessionUseCase) AggregateEvents(events []*domain.SessionEvent) AggregatedSession {
	agg := AggregatedSession{
		Turns:         []SessionTurnPair{},
		Prompts:       []string{},
		Responses:     []string{},
		ToolCalls:     []string{},
		FilesModified: []string{},
		FilesRead:     []string{},
		Commands:      []string{},
		Subagents:     []SubagentInfo{},
	}

	for _, e := range events {
		p := e.Payload
		if p == nil {
			continue
		}

		if agg.AgentType == "" {
			if at, ok := p["agentType"].(string); ok && at != "" {
				agg.AgentType = at
			}
		}
		if agg.GitRemote == "" {
			if gr, ok := p["gitRemote"].(string); ok && gr != "" {
				agg.GitRemote = gr
			}
		}

		switch e.EventType {
		case "turn_start":
			if prompt, ok := p["prompt"].(string); ok && strings.TrimSpace(prompt) != "" {
				agg.Prompts = append(agg.Prompts, prompt)
			}
		case "turn_end":
			turn := SessionTurnPair{
				ToolCalls:     []string{},
				FilesModified: []string{},
			}
			if prompt, ok := p["prompt"].(string); ok && strings.TrimSpace(prompt) != "" {
				turn.Prompt = prompt
				agg.Prompts = append(agg.Prompts, prompt)
			}
			if resp, ok := p["response"].(string); ok && strings.TrimSpace(resp) != "" {
				turn.Response = resp
				agg.Responses = append(agg.Responses, resp)
			}
			if tools, ok := p["toolCalls"].([]any); ok {
				for _, t := range tools {
					if ts, ok := t.(string); ok && ts != "" {
						turn.ToolCalls = append(turn.ToolCalls, ts)
						agg.ToolCalls = append(agg.ToolCalls, ts)
					} else if tm, ok := t.(map[string]any); ok {
						if toolName, ok := tm["tool"].(string); ok && toolName != "" {
							turn.ToolCalls = append(turn.ToolCalls, toolName)
							agg.ToolCalls = append(agg.ToolCalls, toolName)
						}
					}
				}
			}
			if files, ok := p["filesModified"].([]any); ok {
				for _, f := range files {
					if fs, ok := f.(string); ok && fs != "" {
						turn.FilesModified = append(turn.FilesModified, fs)
						agg.FilesModified = append(agg.FilesModified, fs)
					}
				}
			}
			if files, ok := p["filesRead"].([]any); ok {
				for _, f := range files {
					if fs, ok := f.(string); ok && fs != "" {
						agg.FilesRead = append(agg.FilesRead, fs)
					}
				}
			}
			if cmds, ok := p["commands"].([]any); ok {
				for _, c := range cmds {
					if cs, ok := c.(string); ok && cs != "" {
						agg.Commands = append(agg.Commands, cs)
					}
				}
			}
			agg.Turns = append(agg.Turns, turn)
		case "subagent_start", "subagent_end":
			sub := SubagentInfo{}
			if st, ok := p["subagentType"].(string); ok {
				sub.Type = st
			}
			if task, ok := p["taskDescription"].(string); ok {
				sub.Task = task
			} else if task, ok := p["task"].(string); ok {
				sub.Task = task
			}
			agg.Subagents = append(agg.Subagents, sub)
		}
	}

	return agg
}

// HasUsefulContent determines if session history contains actionable textual telemetry.
func (uc *ClassifySessionUseCase) HasUsefulContent(agg AggregatedSession) bool {
	if len(agg.Prompts) > 0 || len(agg.Responses) > 0 || len(agg.Subagents) > 0 {
		return true
	}
	return false
}

// ExtractWithLLM uses structured LLM inference to extract decisions.
func (uc *ClassifySessionUseCase) ExtractWithLLM(
	ctx context.Context,
	agg *AggregatedSession,
	orgID string,
) ([]domain.CliSessionClassifiedDecision, error) {
	if agg == nil {
		return []domain.CliSessionClassifiedDecision{}, nil
	}
	systemPrompt := `You are classifying coding session histories into reusable decisions.

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

	clonedAgg := *agg
	if len(clonedAgg.Turns) > 20 {
		clonedAgg.Turns = clonedAgg.Turns[len(clonedAgg.Turns)-20:]
	}
	slicedTurns := make([]SessionTurnPair, len(clonedAgg.Turns))
	for i, t := range clonedAgg.Turns {
		slicedTurn := t
		if slicedTurn.ToolCalls == nil {
			slicedTurn.ToolCalls = []string{}
		} else if len(slicedTurn.ToolCalls) > 5 {
			slicedTurn.ToolCalls = slicedTurn.ToolCalls[:5]
		}
		if slicedTurn.FilesModified == nil {
			slicedTurn.FilesModified = []string{}
		} else if len(slicedTurn.FilesModified) > 5 {
			slicedTurn.FilesModified = slicedTurn.FilesModified[:5]
		}
		slicedTurns[i] = slicedTurn
	}
	clonedAgg.Turns = slicedTurns

	if clonedAgg.FilesModified == nil {
		clonedAgg.FilesModified = []string{}
	} else if len(clonedAgg.FilesModified) > 30 {
		clonedAgg.FilesModified = clonedAgg.FilesModified[:30]
	}
	if clonedAgg.FilesRead == nil {
		clonedAgg.FilesRead = []string{}
	} else if len(clonedAgg.FilesRead) > 20 {
		clonedAgg.FilesRead = clonedAgg.FilesRead[:20]
	}
	if clonedAgg.Commands == nil {
		clonedAgg.Commands = []string{}
	} else if len(clonedAgg.Commands) > 20 {
		clonedAgg.Commands = clonedAgg.Commands[:20]
	}
	if clonedAgg.ToolCalls == nil {
		clonedAgg.ToolCalls = []string{}
	}
	if clonedAgg.Subagents == nil {
		clonedAgg.Subagents = []SubagentInfo{}
	}
	if clonedAgg.Prompts == nil {
		clonedAgg.Prompts = []string{}
	}
	if clonedAgg.Responses == nil {
		clonedAgg.Responses = []string{}
	}

	payloadBytes, _ := json.Marshal(clonedAgg)
	rawDecisions, err := uc.llmClient.ExtractDecisions(ctx, systemPrompt, string(payloadBytes))
	if err != nil {
		return nil, err
	}

	var results []domain.CliSessionClassifiedDecision
	for _, d := range rawDecisions {
		conf := uc.NormalizeConfidence(d.Confidence)
		dType := d.Type
		origin := d.Origin

		scope := d.Scope
		if len(scope) == 0 && len(agg.FilesModified) > 0 {
			scope = agg.FilesModified
		}

		results = append(results, domain.CliSessionClassifiedDecision{
			Type:                 dType,
			Origin:               origin,
			Decision:             uc.trim(d.Decision, 500),
			Rationale:            uc.trim(d.Rationale, 1000),
			Confidence:           conf,
			Evidence:             uc.sliceStrings(d.Evidence, 5),
			AutoPromoteCandidate: uc.ShouldAutoPromote(dType, conf),
			Scope:                scope,
		})
	}

	return results, nil
}

// ExtractWithHeuristics generates decisions without requiring model inference.
func (uc *ClassifySessionUseCase) ExtractWithHeuristics(agg AggregatedSession) []domain.CliSessionClassifiedDecision {
	var parts []string
	parts = append(parts, agg.Prompts...)
	parts = append(parts, agg.Responses...)
	for _, sub := range agg.Subagents {
		if sub.Task != "" {
			parts = append(parts, sub.Task)
		}
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

	evidence := uc.sliceStrings(agg.FilesModified, 3)

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
func (uc *ClassifySessionUseCase) InferDecisionType(text string) domain.CliSessionDecisionType {
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
func (uc *ClassifySessionUseCase) ShouldAutoPromote(decisionType domain.CliSessionDecisionType, confidence float64) bool {
	if confidence < 0.7 {
		return false
	}
	return decisionType == "architectural_decision" ||
		decisionType == "convention" ||
		decisionType == "tradeoff"
}

// NormalizeConfidence bounds value between 0 and 1.
func (uc *ClassifySessionUseCase) NormalizeConfidence(val float64) float64 {
	if math.IsNaN(val) || val < 0 {
		return 0
	}
	if val > 1 {
		return 1
	}
	return val
}

func (uc *ClassifySessionUseCase) trim(s string, maxLen int) string {
	clean := strings.TrimSpace(s)
	if len(clean) <= maxLen {
		return clean
	}
	if maxLen <= 3 {
		return clean[:maxLen]
	}
	return clean[:maxLen-3] + "..."
}

func (uc *ClassifySessionUseCase) sliceStrings(list []string, maxCount int) []string {
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

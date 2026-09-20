// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package compression

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// CompressionConfig holds tuning parameters for context compaction.
type CompressionConfig struct {
	CompressionThresholdRatio float64
	RecentTailMessages        int
	RecentMaxCharsPerResult   int
	OlderMaxCharsPerResult    int
	SummaryMaxCharsPerEntry   int
	HardClampMaxChars         int
}

// DefaultCompressionConfig loads calibrated defaults with environment overrides.
func DefaultCompressionConfig() CompressionConfig {
	cfg := CompressionConfig{
		CompressionThresholdRatio: 0.7,
		RecentTailMessages:        4,
		RecentMaxCharsPerResult:   3000,
		OlderMaxCharsPerResult:    400,
		SummaryMaxCharsPerEntry:   200,
		HardClampMaxChars:         500,
	}

	if v := os.Getenv("COMPRESSION_THRESHOLD_RATIO"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f < 1 {
			cfg.CompressionThresholdRatio = f
		}
	}
	if v := os.Getenv("COMPRESSION_RECENT_TAIL_MESSAGES"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.RecentTailMessages = i
		}
	}
	if v := os.Getenv("COMPRESSION_RECENT_MAX_CHARS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.RecentMaxCharsPerResult = i
		}
	}
	if v := os.Getenv("COMPRESSION_OLDER_MAX_CHARS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.OlderMaxCharsPerResult = i
		}
	}
	if v := os.Getenv("COMPRESSION_HARD_CLAMP_MAX_CHARS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.HardClampMaxChars = i
		}
	}

	return cfg
}

// EstimateMessagesTokens computes token usage across all messages in history.
func EstimateMessagesTokens(messages []contracts.AgentMessage) int {
	total := 0
	for _, msg := range messages {
		total += EstimateValueTokens(msg.Content)
		for _, tc := range msg.ToolCalls {
			total += EstimateValueTokens(tc.Input)
			total += EstimateTextTokens(tc.Output)
		}
	}
	return total
}

// ShouldCompressResult reports whether the history requires compression.
type ShouldCompressResult struct {
	Should          bool
	CurrentTokens   int
	ThresholdTokens int
}

// ShouldCompress evaluates if message tokens cross the trigger threshold.
func ShouldCompress(messages []contracts.AgentMessage, contextWindowTokens int, cfg ...CompressionConfig) ShouldCompressResult {
	config := DefaultCompressionConfig()
	if len(cfg) > 0 {
		config = cfg[0]
	}

	currentTokens := EstimateMessagesTokens(messages)
	thresholdTokens := int(float64(contextWindowTokens) * config.CompressionThresholdRatio)
	return ShouldCompressResult{
		Should:          currentTokens > thresholdTokens,
		CurrentTokens:   currentTokens,
		ThresholdTokens: thresholdTokens,
	}
}

func truncateText(text string, maxChars int) string {
	if len(text) <= maxChars {
		return text
	}
	return text[:maxChars] + "…[truncated]"
}

func truncateToolMessage(msg contracts.AgentMessage, maxChars int) (contracts.AgentMessage, bool) {
	truncated := false
	newMsg := msg

	if str, ok := msg.Content.(string); ok {
		if len(str) > maxChars {
			newMsg.Content = truncateText(str, maxChars)
			truncated = true
		}
	} else if parts, ok := msg.Content.([]any); ok {
		newParts := make([]any, len(parts))
		for i, part := range parts {
			if m, ok := part.(map[string]any); ok {
				cloned := make(map[string]any, len(m))
				for k, v := range m {
					if textStr, isStr := v.(string); isStr && len(textStr) > maxChars {
						cloned[k] = truncateText(textStr, maxChars)
						truncated = true
					} else {
						cloned[k] = v
					}
				}
				newParts[i] = cloned
			} else {
				newParts[i] = part
			}
		}
		newMsg.Content = newParts
	}

	// Also check tool calls output
	if len(msg.ToolCalls) > 0 {
		newTCs := make([]contracts.ToolCallRecord, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			newTC := tc
			if len(tc.Output) > maxChars {
				newTC.Output = truncateText(tc.Output, maxChars)
				truncated = true
			}
			newTCs[i] = newTC
		}
		newMsg.ToolCalls = newTCs
	}

	return newMsg, truncated
}

func buildInvestigationSummary(allToolCalls []contracts.ToolCallRecord, maxEntryChars int) string {
	if len(allToolCalls) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Previously investigated (older tool results truncated to save context):\n")
	for _, tc := range allToolCalls {
		name := tc.Name
		if name == "" {
			name = "unknown"
		}
		preview := strings.ReplaceAll(tc.Output, "\n", " ")
		preview = strings.TrimSpace(preview)
		if len(preview) > maxEntryChars {
			preview = preview[:maxEntryChars] + "…"
		}
		sb.WriteString(fmt.Sprintf("- %s → %s\n", name, preview))
	}
	sb.WriteString("\nUse this recap to avoid redundant tool calls. Continue your investigation from the most recent tool results shown below.")
	return sb.String()
}

// SplitHead separates leading system turns + the first user turn (which holds <Diffs>).
func SplitHead(messages []contracts.AgentMessage) ([]contracts.AgentMessage, []contracts.AgentMessage) {
	var head []contracts.AgentMessage
	idx := 0
	for idx < len(messages) && messages[idx].Role == contracts.RoleSystem {
		head = append(head, messages[idx])
		idx++
	}
	if idx < len(messages) && messages[idx].Role == contracts.RoleUser {
		head = append(head, messages[idx])
		idx++
	}
	return head, messages[idx:]
}

// CompressMessages preserves the head (diff), truncates older tail results,
// and injects a structured investigation recap.
func CompressMessages(
	messages []contracts.AgentMessage,
	allToolCalls []contracts.ToolCallRecord,
	cfg ...CompressionConfig,
) []contracts.AgentMessage {
	if len(messages) == 0 {
		return messages
	}

	config := DefaultCompressionConfig()
	if len(cfg) > 0 {
		config = cfg[0]
	}

	head, tail := SplitHead(messages)
	if len(tail) == 0 {
		return messages
	}

	recentStart := len(tail) - config.RecentTailMessages
	if recentStart < 0 {
		recentStart = 0
	}

	anyTruncated := false
	compressedTail := make([]contracts.AgentMessage, len(tail))

	for i, msg := range tail {
		if msg.Role != contracts.RoleTool && len(msg.ToolCalls) == 0 {
			compressedTail[i] = msg
			continue
		}

		maxChars := config.OlderMaxCharsPerResult
		if i >= recentStart {
			maxChars = config.RecentMaxCharsPerResult
		}

		newMsg, truncated := truncateToolMessage(msg, maxChars)
		if truncated {
			anyTruncated = true
		}
		compressedTail[i] = newMsg
	}

	if anyTruncated && len(allToolCalls) > 0 {
		summary := buildInvestigationSummary(allToolCalls, config.SummaryMaxCharsPerEntry)
		if summary != "" {
			recapMsg := contracts.AgentMessage{
				Role:    contracts.RoleUser, // role user for Gemini compatibility
				Content: fmt.Sprintf("[investigation recap]\n%s", summary),
			}
			out := append(head, recapMsg)
			return append(out, compressedTail...)
		}
	}

	return append(head, compressedTail...)
}

// GroupRounds groups turns into atomic rounds: each non-tool turn plus its answering tool results.
func GroupRounds(messages []contracts.AgentMessage) [][]contracts.AgentMessage {
	var rounds [][]contracts.AgentMessage
	var current []contracts.AgentMessage

	for _, m := range messages {
		if m.Role == contracts.RoleTool && len(current) > 0 {
			current = append(current, m)
		} else {
			if len(current) > 0 {
				rounds = append(rounds, current)
			}
			current = []contracts.AgentMessage{m}
		}
	}
	if len(current) > 0 {
		rounds = append(rounds, current)
	}
	return rounds
}

func evictOldestRounds(messages []contracts.AgentMessage, budgetTokens int) []contracts.AgentMessage {
	head, rest := SplitHead(messages)
	rounds := GroupRounds(rest)
	if len(rounds) == 0 {
		return messages
	}

	running := EstimateMessagesTokens(head)
	var keptReversed [][]contracts.AgentMessage

	for i := len(rounds) - 1; i >= 0; i-- {
		roundTokens := EstimateMessagesTokens(rounds[i])
		if len(keptReversed) == 0 || running+roundTokens <= budgetTokens {
			keptReversed = append(keptReversed, rounds[i])
			running += roundTokens
		} else {
			break
		}
	}

	var flatRest []contracts.AgentMessage
	for i := len(keptReversed) - 1; i >= 0; i-- {
		flatRest = append(flatRest, keptReversed[i]...)
	}

	return append(head, flatRest...)
}

func hardTruncateToFit(messages []contracts.AgentMessage, budgetTokens int, hardClampChars int) []contracts.AgentMessage {
	capChars := hardClampChars
	work := messages

	for iter := 0; iter < 12; iter++ {
		stepWork := make([]contracts.AgentMessage, len(work))
		for i, m := range work {
			newM, _ := truncateToolMessage(m, capChars)
			stepWork[i] = newM
		}
		work = stepWork

		if EstimateMessagesTokens(work) <= budgetTokens {
			return work
		}
		if capChars <= 80 {
			break
		}
		capChars = capChars / 2
		if capChars < 80 {
			capChars = 80
		}
	}
	return work
}

// ClampMessagesToBudget guarantees the message history fits within budgetTokens.
// Order of operations:
// 1. Aggressive tool-result truncation across the entire window.
// 2. Round eviction (preserving head and most recent turns).
// 3. Last-resort geometric shrinking.
func ClampMessagesToBudget(messages []contracts.AgentMessage, budgetTokens int, cfg ...CompressionConfig) []contracts.AgentMessage {
	if len(messages) == 0 || budgetTokens <= 0 {
		return messages
	}
	if EstimateMessagesTokens(messages) <= budgetTokens {
		return messages
	}

	config := DefaultCompressionConfig()
	if len(cfg) > 0 {
		config = cfg[0]
	}

	// Phase 1: Aggressive tool-result truncation
	work := make([]contracts.AgentMessage, len(messages))
	for i, m := range messages {
		if m.Role == contracts.RoleTool || len(m.ToolCalls) > 0 {
			newM, _ := truncateToolMessage(m, config.HardClampMaxChars)
			work[i] = newM
		} else {
			work[i] = m
		}
	}
	if EstimateMessagesTokens(work) <= budgetTokens {
		return work
	}

	// Phase 2: Evict oldest rounds
	work = evictOldestRounds(work, budgetTokens)
	if EstimateMessagesTokens(work) <= budgetTokens {
		return work
	}

	// Phase 3: Geometric hard truncate
	return hardTruncateToFit(work, budgetTokens, config.HardClampMaxChars)
}

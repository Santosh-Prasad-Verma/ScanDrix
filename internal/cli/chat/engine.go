// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/internal/llm"
)

// AgentEventHandler receives lifecycle events during autonomous agent execution.
type AgentEventHandler interface {
	OnThinking(thought string)
	OnToolCallStart(toolName string, args map[string]any)
	OnToolCallComplete(result *ToolResult)
	OnFinalAnswer(answer string)
}

// DefaultAgentEventHandler provides a no-op handler implementation.
type DefaultAgentEventHandler struct{}

func (h *DefaultAgentEventHandler) OnThinking(thought string)                            {}
func (h *DefaultAgentEventHandler) OnToolCallStart(toolName string, args map[string]any) {}
func (h *DefaultAgentEventHandler) OnToolCallComplete(result *ToolResult)                 {}
func (h *DefaultAgentEventHandler) OnFinalAnswer(answer string)                          {}

// AgentEngine coordinates autonomous ReAct reasoning and tool dispatch.
type AgentEngine struct {
	gateway   *llm.Gateway
	registry  *ToolRegistry
	maxTurns  int
	modelName string
}

// NewAgentEngine instantiates the agent loop engine.
func NewAgentEngine(gw *llm.Gateway, registry *ToolRegistry, modelName string) *AgentEngine {
	if gw == nil {
		gw = engine.InitLocalGateway()
	}
	return &AgentEngine{
		gateway:   gw,
		registry:  registry,
		maxTurns:  10,
		modelName: modelName,
	}
}

var toolCallRegex = regexp.MustCompile(`(?s)<tool_call>\s*({.*?})\s*</tool_call>`)

type toolCallPayload struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// BuildToolsSystemPrompt constructs the tool instruction header for LLM context.
func (e *AgentEngine) BuildToolsSystemPrompt() string {
	var sb strings.Builder
	sb.WriteString("You are ScanDrix, an elite autonomous software engineering and security agent.\n")
	sb.WriteString("You have access to the following tools to inspect, analyze, and test code:\n\n")

	for _, tool := range e.registry.All() {
		sb.WriteString(fmt.Sprintf("### Tool: %s\n", tool.Name()))
		sb.WriteString(fmt.Sprintf("Description: %s\n", tool.Description()))
		sb.WriteString(fmt.Sprintf("Parameters Schema:\n```json\n%s\n```\n\n", tool.ParametersJSON()))
	}

	sb.WriteString("When you need to use a tool, output exactly:\n")
	sb.WriteString("<tool_call>\n{\n  \"name\": \"tool_name\",\n  \"arguments\": {\"param\": \"value\"}\n}\n</tool_call>\n\n")
	sb.WriteString("After the tool executes, you will receive <tool_result>...</tool_result>.\n")
	sb.WriteString("Provide a clear, helpful final answer once you have sufficient context.\n")

	return sb.String()
}

// RunAutonomousTurn executes the ReAct loop until the agent produces a final answer or reaches turn limit.
func (e *AgentEngine) RunAutonomousTurn(ctx context.Context, history []llm.ChatMessage, handler AgentEventHandler) (string, []llm.ChatMessage, error) {
	if handler == nil {
		handler = &DefaultAgentEventHandler{}
	}

	systemPrompt := e.BuildToolsSystemPrompt()
	messages := make([]llm.ChatMessage, len(history))
	copy(messages, history)

	// Extract prompt from latest user message or default
	lastUserPrompt := "Analyze workspace"
	if len(messages) > 0 && messages[len(messages)-1].Role == "user" {
		lastUserPrompt = messages[len(messages)-1].Content
		messages = messages[:len(messages)-1]
	}

	for turn := 0; turn < e.maxTurns; turn++ {
		select {
		case <-ctx.Done():
			return "", messages, ctx.Err()
		default:
		}

		handler.OnThinking(fmt.Sprintf("Reasoning (Turn %d/%d)...", turn+1, e.maxTurns))

		var reply string
		var err error
		if e.gateway != nil {
			reply, err = e.gateway.GenerateChatResponseWithModel(ctx, e.modelName, systemPrompt, messages, lastUserPrompt)
		} else {
			err = fmt.Errorf("no LLM gateway available")
		}

		if err != nil {
			fallbackMsg := "ScanDrix Local Agent: Context analyzed. Run `scandrix review` to inspect local diffs or `scandrix pentest` to verify security vulnerabilities."
			handler.OnFinalAnswer(fallbackMsg)
			messages = append(messages,
				llm.ChatMessage{Role: "user", Content: lastUserPrompt},
				llm.ChatMessage{Role: "assistant", Content: fallbackMsg},
			)
			return fallbackMsg, messages, nil
		}

		matches := toolCallRegex.FindStringSubmatch(reply)

		// If no tool call, this is the final answer
		if len(matches) < 2 {
			handler.OnFinalAnswer(reply)
			messages = append(messages,
				llm.ChatMessage{Role: "user", Content: lastUserPrompt},
				llm.ChatMessage{Role: "assistant", Content: reply},
			)
			return reply, messages, nil
		}

		// Extract and parse tool invocation
		var payload toolCallPayload
		if err := json.Unmarshal([]byte(matches[1]), &payload); err != nil {
			errResult := fmt.Sprintf("Error: Invalid tool call payload: %v", err)
			messages = append(messages,
				llm.ChatMessage{Role: "user", Content: lastUserPrompt},
				llm.ChatMessage{Role: "assistant", Content: reply},
			)
			lastUserPrompt = fmt.Sprintf("<tool_result>\n%s\n</tool_result>", errResult)
			continue
		}

		var parsedArgs map[string]any
		_ = json.Unmarshal(payload.Arguments, &parsedArgs)
		handler.OnToolCallStart(payload.Name, parsedArgs)

		// Execute tool via registry
		result, execErr := e.registry.Execute(ctx, payload.Name, payload.Arguments)
		if execErr != nil {
			result = &ToolResult{
				ToolName: payload.Name,
				Output:   fmt.Sprintf("Execution error: %v", execErr),
				IsError:  true,
			}
		}

		handler.OnToolCallComplete(result)

		// Append turn messages
		messages = append(messages,
			llm.ChatMessage{Role: "user", Content: lastUserPrompt},
			llm.ChatMessage{Role: "assistant", Content: reply},
		)
		lastUserPrompt = fmt.Sprintf("<tool_result tool=%q>\n%s\n</tool_result>", payload.Name, result.Output)
	}

	exhaustedMsg := "Agent reached maximum execution turn limit."
	handler.OnFinalAnswer(exhaustedMsg)
	return exhaustedMsg, messages, nil
}

// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package conversation

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/domain"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
	agentTools "github.com/scandrix/backend/internal/agents/tools"
	mcpClient "github.com/scandrix/backend/internal/mcp/manager/client"
)

const (
	defaultMaxSteps         = 12
	defaultMaxOutputTokens   = 20000
	defaultLanguage         = "en-US"
)

// ConversationRequest models the input payload for an interactive agent conversation turn.
type ConversationRequest struct {
	Prompt         string                 `json:"prompt"`
	OrganizationID string                 `json:"organization_id"`
	TeamID         string                 `json:"team_id"`
	UserID         string                 `json:"user_id,omitempty"`
	RepositoryID   string                 `json:"repository_id,omitempty"`
	ThreadID       string                 `json:"thread_id"`
	Language       string                 `json:"language,omitempty"`
	SandboxRoot    string                 `json:"sandbox_root,omitempty"`
	Tools          contracts.ToolRegistry `json:"-"`
	PrepareContext map[string]any         `json:"prepare_context,omitempty"`
}

// ConversationResponse encapsulates the normalized answer, metrics, and thread status.
type ConversationResponse struct {
	Response     string               `json:"response"`
	ThreadID     string               `json:"thread_id"`
	FinishReason string               `json:"finish_reason"`
	StepCount    int                  `json:"step_count"`
	Usage        contracts.TokenUsage `json:"usage"`
	Duration     time.Duration        `json:"duration"`
}

// ConversationAgentOptions configures limits, language, and token ceilings.
type ConversationAgentOptions struct {
	MaxSteps           int
	MaxOutputTokens    int
	DefaultLanguage    string
	Store              contracts.ConversationStore
	MCPClient          *mcpClient.MCPClient
	DefaultSandboxRoot string
}

// ConversationAgentProvider coordinates conversational turns through GoAgentRunner.
type ConversationAgentProvider struct {
	runner contracts.AgentRunner
	store  contracts.ConversationStore
	opts   ConversationAgentOptions
}

// NewConversationAgentProvider initializes the provider with environment defaults.
func NewConversationAgentProvider(
	runner contracts.AgentRunner,
	store contracts.ConversationStore,
	customOpts ...ConversationAgentOptions,
) *ConversationAgentProvider {
	opts := ConversationAgentOptions{
		MaxSteps:        defaultMaxSteps,
		MaxOutputTokens: defaultMaxOutputTokens,
		DefaultLanguage: defaultLanguage,
		Store:           store,
	}

	if envSteps := os.Getenv("SCANDRIX_AGENT_MAX_STEPS"); envSteps != "" {
		if parsed, err := strconv.Atoi(envSteps); err == nil && parsed > 0 {
			opts.MaxSteps = parsed
		}
	}
	if envTokens := os.Getenv("SCANDRIX_AGENT_MAX_OUTPUT_TOKENS"); envTokens != "" {
		if parsed, err := strconv.Atoi(envTokens); err == nil && parsed > 0 {
			opts.MaxOutputTokens = parsed
		}
	}
	if envLang := os.Getenv("SCANDRIX_AGENT_DEFAULT_LANGUAGE"); envLang != "" {
		opts.DefaultLanguage = envLang
	}

	if len(customOpts) > 0 {
		userOpt := customOpts[0]
		if userOpt.MaxSteps > 0 {
			opts.MaxSteps = userOpt.MaxSteps
		}
		if userOpt.MaxOutputTokens > 0 {
			opts.MaxOutputTokens = userOpt.MaxOutputTokens
		}
		if userOpt.DefaultLanguage != "" {
			opts.DefaultLanguage = userOpt.DefaultLanguage
		}
		if userOpt.Store != nil {
			opts.Store = userOpt.Store
		}
		if userOpt.MCPClient != nil {
			opts.MCPClient = userOpt.MCPClient
		}
		if userOpt.DefaultSandboxRoot != "" {
			opts.DefaultSandboxRoot = userOpt.DefaultSandboxRoot
		}
	}

	return &ConversationAgentProvider{
		runner: runner,
		store:  opts.Store,
		opts:   opts,
	}
}

// Execute executes an interactive conversation turn with multi-step reasoning,
// fallback unwrapping, minimal retry on empty answers, and background history storage.
func (p *ConversationAgentProvider) Execute(ctx context.Context, req ConversationRequest) (*ConversationResponse, error) {
	startTime := time.Now()

	userLang := req.Language
	if userLang == "" {
		userLang = p.opts.DefaultLanguage
	}

	var toolList []contracts.AgentTool
	if req.Tools != nil {
		toolList = append(toolList, req.Tools.List()...)
	}

	// 1. Auto-attach repository sandbox tools if sandbox directory exists
	sandboxDir := req.SandboxRoot
	if sandboxDir == "" {
		sandboxDir = p.opts.DefaultSandboxRoot
	}
	if sandboxDir != "" {
		if fi, err := os.Stat(sandboxDir); err == nil && fi.IsDir() {
			toolList = append(toolList,
				agentTools.BuildGrepTool(sandboxDir),
				agentTools.BuildReadFileTool(sandboxDir),
				agentTools.BuildListDirTool(sandboxDir),
				agentTools.BuildExecTool(sandboxDir),
			)
		}
	}

	// 2. Auto-attach remote MCP tools if MCP client is configured
	if p.opts.MCPClient != nil {
		if mcpToolsList, err := p.opts.MCPClient.GetTools(ctx); err == nil {
			toolList = append(toolList, agentTools.AdaptMCPTools(p.opts.MCPClient, mcpToolsList)...)
		}
	}

	toolRegistry := tools.NewInMemoryToolRegistry(toolList...)

	maxTokens := p.opts.MaxOutputTokens
	// 1. Build AgentSpec
	spec := contracts.AgentSpec{
		ID:              "conversation",
		AgentName:       "ConversationalAgent",
		RunName:         "conversationAgent",
		Phase:           "conversation",
		SpanName:        "ConversationalAgent::conversationAgent",
		SystemPrompt:    p.buildSystemPrompt(userLang),
		Tools:           toolRegistry,
		MaxSteps:        p.opts.MaxSteps,
		MaxOutputTokens: &maxTokens,
	}

	// 2. Load prior conversation history if store is available
	var priorMessages []contracts.AgentMessage
	if p.store != nil && req.ThreadID != "" {
		history, _ := p.store.Load(ctx, req.ThreadID)
		for _, msg := range history {
			priorMessages = append(priorMessages, contracts.AgentMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	userPrompt := p.buildUserPrompt(req.Prompt, userLang, req.PrepareContext)

	// 3. Assemble run input and context
	runInput := contracts.AgentRunInput{
		Prompt: userPrompt,
		TelemetryMetadata: map[string]any{
			"organization_id": req.OrganizationID,
			"team_id":         req.TeamID,
			"thread_id":       req.ThreadID,
			"repository_id":   req.RepositoryID,
		},
	}

	toolCtx := contracts.ToolContext{
		RunID:   fmt.Sprintf("conv-%s-%d", req.OrganizationID, time.Now().UnixNano()),
		Context: ctx,
		Services: map[string]any{
			"organization_id": req.OrganizationID,
			"team_id":         req.TeamID,
		},
	}

	// 4. Run harness agent loop
	state, err := p.runner.Run(ctx, spec, runInput, toolCtx)
	if err != nil {
		slog.Error("ConversationAgent execution failed", "error", err, "thread_id", req.ThreadID)
		return &ConversationResponse{
			Response:     ConversationProviderErrorMessage,
			ThreadID:     req.ThreadID,
			FinishReason: "error",
			Duration:     time.Since(startTime),
		}, nil
	}

	// 5. Extract and normalize answer
	answer := domain.FinalText(state)
	normalized := NormalizeConversationResponse(answer)

	// 6. Minimal retry if model produced no usable content (Never-empty guard)
	if normalized == "" {
		slog.Warn("Conversation agent produced no usable response; retrying minimal",
			"thread_id", req.ThreadID,
			"steps", len(state.Steps),
		)
		minimalAnswer := p.forceAnswer(ctx, req.Prompt, userLang)
		normalized = NormalizeConversationResponse(minimalAnswer)
	}

	// 7. Select user-facing response
	userFacing := normalized
	if userFacing == "" {
		if state.Status == contracts.StatusError {
			userFacing = ConversationProviderErrorMessage
		} else {
			userFacing = ConversationFallbackMessage
		}
	}

	// 8. Persist conversation turn into store (asynchronous/best-effort)
	if p.store != nil && req.ThreadID != "" {
		turns := []contracts.ConversationMessage{
			{Role: contracts.RoleUser, Content: req.Prompt},
			{Role: contracts.RoleAssistant, Content: userFacing},
		}
		meta := &contracts.ConversationAppendMeta{
			OrganizationID: req.OrganizationID,
			TeamID:         req.TeamID,
			RepositoryID:   req.RepositoryID,
			Channel:        "chat",
			CorrelationID:  toolCtx.RunID,
		}
		_ = p.store.Append(ctx, req.ThreadID, turns, meta)
	}

	finishReason := string(state.Status)
	if state.StopReason != "" {
		finishReason = state.StopReason
	}

	return &ConversationResponse{
		Response:     userFacing,
		ThreadID:     req.ThreadID,
		FinishReason: finishReason,
		StepCount:    len(state.Steps),
		Usage:        state.Usage,
		Duration:     time.Since(startTime),
	}, nil
}

// forceAnswer executes a single minimal prompt retry without tools.
func (p *ConversationAgentProvider) forceAnswer(ctx context.Context, prompt, userLanguage string) string {
	fallbackTokens := 2000
	spec := contracts.AgentSpec{
		ID:              "conversation-minimal",
		AgentName:       "ConversationalAgent",
		RunName:         "conversationMinimalFallback",
		SystemPrompt:    fmt.Sprintf("Answer the user's question directly and concisely in %s. Do not use tools.", userLanguage),
		Tools:           tools.NewInMemoryToolRegistry(),
		MaxSteps:        1,
		MaxOutputTokens: &fallbackTokens,
	}

	state, err := p.runner.Run(ctx, spec, contracts.AgentRunInput{Prompt: prompt}, contracts.ToolContext{
		RunID:   "conv-fallback",
		Context: ctx,
	})
	if err != nil {
		return ""
	}
	return domain.FinalText(state)
}

func (p *ConversationAgentProvider) buildSystemPrompt(language string) string {
	return fmt.Sprintf(`You are ScanDrix AI's intelligent code review assistant ("ScanDrix").
You collaborate with software engineers on pull requests, architectural reviews, and code refactorings.

Guidelines:
1. Provide accurate, clear, and direct answers in %s.
2. When investigating codebases, make effective use of exploration tools (grep, readFile, listDir).
3. If unsure about specific implementation details, inspect the files before concluding.
4. Keep answers focused on the engineer's question without unsolicited verbosity.
5. Format code blocks with language syntax highlighting.`, language)
}

func (p *ConversationAgentProvider) buildUserPrompt(userPrompt, language string, prepareCtx map[string]any) string {
	var builder strings.Builder

	if len(prepareCtx) > 0 {
		builder.WriteString("### Context Information:\n")
		for k, v := range prepareCtx {
			builder.WriteString(fmt.Sprintf("- **%s**: %v\n", k, v))
		}
		builder.WriteString("\n")
	}

	builder.WriteString(fmt.Sprintf("### User Question (%s):\n%s", language, userPrompt))
	return builder.String()
}

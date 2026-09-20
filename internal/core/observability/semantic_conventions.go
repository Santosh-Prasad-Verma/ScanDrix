package observability

// Semantic conventions matching OpenTelemetry GenAI standards from ScanDrix semantic-conventions.ts.

const (
	GenAISystem                 = "gen_ai.system"
	GenAIOperationName          = "gen_ai.operation.name"
	GenAIRequestModel           = "gen_ai.request.model"
	GenAIRequestTemperature     = "gen_ai.request.temperature"
	GenAIRequestMaxTokens       = "gen_ai.request.max_tokens"
	GenAIRequestTopP            = "gen_ai.request.top_p"
	GenAIRequestStopSequences   = "gen_ai.request.stop_sequences"
	GenAIResponseModel          = "gen_ai.response.model"
	GenAIUsageInputTokens       = "gen_ai.usage.input_tokens"
	GenAIUsageOutputTokens      = "gen_ai.usage.output_tokens"
	GenAIUsageTotalTokens       = "gen_ai.usage.total_tokens"
	GenAIUsageReasoningTokens   = "gen_ai.usage.reasoning_tokens"
	GenAIUsageCacheReadTokens   = "gen_ai.usage.cache_read_input_tokens"
	GenAIUsageCacheWriteTokens  = "gen_ai.usage.cache_creation_input_tokens"
	GenAIUsageCostUSD           = "gen_ai.usage.cost_usd"
	GenAIRunName                = "gen_ai.run.name"

	AgentName                   = "agent.name"
	AgentVersion                = "agent.version"
	AgentType                   = "agent.type"
	AgentExecutionID            = "agent.execution.id"
	AgentConversationID         = "agent.conversation.id"
	AgentPhase                  = "agent.phase"

	ToolName                    = "tool.name"
	ToolType                    = "tool.type"
	ToolInput                   = "tool.input"
	ToolOutput                  = "tool.output"

	TenantID                    = "scandrix.tenant_id"
	WorkspaceID                 = "scandrix.workspace_id"
	OrganizationID              = "scandrix.organization_id"
	TeamID                      = "scandrix.team_id"
	RepositoryID                = "scandrix.repository_id"
	PullRequestNumber           = "scandrix.pr_number"
	CorrelationID               = "scandrix.correlation_id"
)

// Supported GenAI systems.
const (
	SystemOpenAI     = "openai"
	SystemAnthropic  = "anthropic"
	SystemGoogle     = "google"
	SystemGroq       = "groq"
	SystemTogether   = "together"
	SystemVertexAI   = "vertex_ai"
)

// Standard operations.
const (
	OperationChat       = "chat"
	OperationCompletion = "text_completion"
	OperationEmbedding  = "embedding"
)

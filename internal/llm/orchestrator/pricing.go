package orchestrator

import (
	"math"
	"strings"
	"sync"
)

// NOTE ON PROVIDER CONSTANTS:
// Supported generative AI inference clouds & direct frontier APIs:
//   ProviderOpenAI, ProviderAnthropic, ProviderGemini, ProviderNovita,
//   ProviderDeepSeek, ProviderBedrock, ProviderVertex, ProviderOpenRouter,
//   ProviderOllama, ProviderVLLM,
//   ProviderMoonshot   ("moonshot")   // Kimi / Moonshot AI direct API
//   ProviderAlibaba    ("alibaba")    // Qwen direct API (Model Studio)
//   ProviderMiniMax    ("minimax")    // MiniMax direct API
//   ProviderTencent    ("tencent")    // Tencent Hunyuan direct API
//   ProviderXAI        ("xai")        // xAI Grok direct API
//   ProviderMistral    ("mistral")    // Mistral direct API
//
// Pricing changes weekly on frontier models right now (price wars are
// active across OpenAI/Anthropic/Google/Alibaba/Moonshot as of Aug 2026).
// Treat everything below as a snapshot — wire this catalog to a periodic
// refresh job or override via RegisterModelProfile() in production.

var defaultCatalog = map[string]ModelProfile{

	// =========================================================================
	// OPENAI — verified against OpenAI's pricing page + OpenRouter, Aug 2026
	// GPT-5.6 GA'd July 9 2026 with a 1.05M context window across all 3 tiers;
	// Terra/Luna got cut 20%/80% on July 30 2026.
	// =========================================================================
	"gpt-5.6-sol": {
		ModelID:          "gpt-5.6-sol",
		Provider:         ProviderOpenAI,
		ContextWindow:    1050000,
		InputPerMillion:  5.00,
		OutputPerMillion: 30.00,
		SupportsThinking: true,
	},
	"gpt-5.6-terra": {
		ModelID:          "gpt-5.6-terra",
		Provider:         ProviderOpenAI,
		ContextWindow:    1050000,
		InputPerMillion:  2.00,
		OutputPerMillion: 12.00,
		SupportsThinking: true,
	},
	"gpt-5.6-luna": {
		ModelID:          "gpt-5.6-luna",
		Provider:         ProviderOpenAI,
		ContextWindow:    1050000,
		InputPerMillion:  0.20,
		OutputPerMillion: 1.20,
		SupportsThinking: true,
	},
	"gpt-5.5": {
		ModelID:          "gpt-5.5",
		Provider:         ProviderOpenAI,
		ContextWindow:    1050000,
		InputPerMillion:  5.00,
		OutputPerMillion: 30.00,
		SupportsThinking: true,
	},
	"gpt-5.4": {
		ModelID:          "gpt-5.4",
		Provider:         ProviderOpenAI,
		ContextWindow:    400000,
		InputPerMillion:  2.50,
		OutputPerMillion: 15.00,
		SupportsThinking: true,
	},
	"gpt-5.4-mini": {
		ModelID:          "gpt-5.4-mini",
		Provider:         ProviderOpenAI,
		ContextWindow:    400000,
		InputPerMillion:  0.75,
		OutputPerMillion: 4.50,
		SupportsThinking: true,
	},
	"gpt-5.4-nano": {
		ModelID:          "gpt-5.4-nano",
		Provider:         ProviderOpenAI,
		ContextWindow:    400000,
		InputPerMillion:  0.20,
		OutputPerMillion: 1.25,
		SupportsThinking: false,
	},
	"gpt-5": {
		ModelID:          "gpt-5",
		Provider:         ProviderOpenAI,
		ContextWindow:    400000,
		InputPerMillion:  1.25,
		OutputPerMillion: 10.00,
		SupportsThinking: true,
	},
	"gpt-4o": {
		ModelID:          "gpt-4o",
		Provider:         ProviderOpenAI,
		ContextWindow:    128000,
		InputPerMillion:  2.50,
		OutputPerMillion: 10.00,
		SupportsThinking: false,
	},
	"gpt-4o-mini": {
		ModelID:          "gpt-4o-mini",
		Provider:         ProviderOpenAI,
		ContextWindow:    128000,
		InputPerMillion:  0.15,
		OutputPerMillion: 0.60,
		SupportsThinking: false,
	},

	// =========================================================================
	// ANTHROPIC — verified against Anthropic's pricing page, Aug 2026
	// Current lineup: Fable 5, Opus 5, Sonnet 5 (intro rate through Aug 31,
	// now made standard), Haiku 4.5. Opus 4.8/Sonnet 4.6 still served.
	// =========================================================================
	"claude-fable-5": {
		ModelID:          "claude-fable-5",
		Provider:         ProviderAnthropic,
		ContextWindow:    1000000,
		InputPerMillion:  10.00,
		OutputPerMillion: 50.00,
		SupportsThinking: true,
	},
	"claude-opus-5": {
		ModelID:          "claude-opus-5",
		Provider:         ProviderAnthropic,
		ContextWindow:    1000000,
		InputPerMillion:  5.00,
		OutputPerMillion: 25.00,
		SupportsThinking: true,
	},
	"claude-sonnet-5": {
		ModelID:          "claude-sonnet-5",
		Provider:         ProviderAnthropic,
		ContextWindow:    1000000,
		InputPerMillion:  2.00,
		OutputPerMillion: 10.00,
		SupportsThinking: true,
	},
	"claude-haiku-4-5": {
		ModelID:          "claude-haiku-4-5",
		Provider:         ProviderAnthropic,
		ContextWindow:    200000,
		InputPerMillion:  1.00,
		OutputPerMillion: 5.00,
		SupportsThinking: true,
	},
	"claude-opus-4-8": {
		ModelID:          "claude-opus-4-8",
		Provider:         ProviderAnthropic,
		ContextWindow:    1000000,
		InputPerMillion:  5.00,
		OutputPerMillion: 25.00,
		SupportsThinking: true,
	},
	"claude-sonnet-4-6": {
		ModelID:          "claude-sonnet-4-6",
		Provider:         ProviderAnthropic,
		ContextWindow:    1000000,
		InputPerMillion:  3.00,
		OutputPerMillion: 15.00,
		SupportsThinking: true,
	},
	"claude-opus-4-1": { // legacy, 3x premium — keep only for pinned compatibility
		ModelID:          "claude-opus-4-1",
		Provider:         ProviderAnthropic,
		ContextWindow:    200000,
		InputPerMillion:  15.00,
		OutputPerMillion: 75.00,
		SupportsThinking: true,
	},
	"claude-3-5-sonnet": { // legacy
		ModelID:          "claude-3-5-sonnet",
		Provider:         ProviderAnthropic,
		ContextWindow:    200000,
		InputPerMillion:  3.00,
		OutputPerMillion: 15.00,
		SupportsThinking: false,
	},

	// =========================================================================
	// GOOGLE GEMINI — verified against ai.google.dev + Google pricing, Aug 2026
	// All Gemini 3 tiers get a 1,048,576-token context window (official).
	// Pro pricing steps up above 200K input tokens.
	// =========================================================================
	"gemini-3.1-pro": {
		ModelID:          "gemini-3.1-pro",
		Provider:         ProviderGemini,
		ContextWindow:    1048576,
		InputPerMillion:  2.00, // steps to $4.00 above 200K input tokens
		OutputPerMillion: 12.00, // steps to $18.00 above 200K input tokens
		SupportsThinking: true,
	},
	"gemini-3-flash": {
		ModelID:          "gemini-3-flash",
		Provider:         ProviderGemini,
		ContextWindow:    1048576,
		InputPerMillion:  0.50,
		OutputPerMillion: 3.00,
		SupportsThinking: true,
	},
	// Gemini 3.5 Flash — launched Google I/O, May 19, 2026
	"gemini-3.5-flash": {
		ModelID:          "gemini-3.5-flash",
		Provider:         ProviderGemini,
		ContextWindow:    1048576,
		InputPerMillion:  1.50,
		OutputPerMillion: 9.00,
		SupportsThinking: true,
	},
	// Gemini 3.5 Flash-Lite — GA July 21, 2026, fastest in the 3.5 series
	"gemini-3.5-flash-lite": {
		ModelID:          "gemini-3.5-flash-lite",
		Provider:         ProviderGemini,
		ContextWindow:    1048576,
		InputPerMillion:  0.30,
		OutputPerMillion: 2.50,
		SupportsThinking: false,
	},
	// Gemini 3.6 Flash — GA July 21, 2026; moved to $0.75/$3.75 intro rate
	// alongside 3.7 Flash's launch; reverts to $1.50/$7.50 on Jan 1, 2027
	"gemini-3.6-flash": {
		ModelID:          "gemini-3.6-flash",
		Provider:         ProviderGemini,
		ContextWindow:    1048576,
		InputPerMillion:  0.75,
		OutputPerMillion: 3.75,
		SupportsThinking: true,
	},
	// Gemini 3.7 Flash — Google's newest/best workhorse model, launched
	// August 13, 2026; intro price expires Dec 31, 2026 → then $1.50/$7.50
	"gemini-3.7-flash": {
		ModelID:          "gemini-3.7-flash",
		Provider:         ProviderGemini,
		ContextWindow:    1048576,
		InputPerMillion:  0.75,
		OutputPerMillion: 3.75,
		SupportsThinking: true,
	},
	// Gemini 3.1 Flash-Lite — GA, half the cost of Gemini 3 Flash
	"gemini-3.1-flash-lite": {
		ModelID:          "gemini-3.1-flash-lite",
		Provider:         ProviderGemini,
		ContextWindow:    1048576,
		InputPerMillion:  0.25,
		OutputPerMillion: 1.50,
		SupportsThinking: true,
	},
	// Gemini 2.5 Flash-Lite — cheapest current-gen fallback model
	"gemini-2.5-flash-lite": {
		ModelID:          "gemini-2.5-flash-lite",
		Provider:         ProviderGemini,
		ContextWindow:    1048576,
		InputPerMillion:  0.10,
		OutputPerMillion: 0.40,
		SupportsThinking: false,
	},
	// Gemini 2.5 Flash — legacy mid-tier, still active fallback
	"gemini-2.5-flash": {
		ModelID:          "gemini-2.5-flash",
		Provider:         ProviderGemini,
		ContextWindow:    1048576,
		InputPerMillion:  0.30,
		OutputPerMillion: 2.50,
		SupportsThinking: true,
	},
	"gemini-2.5-pro": { // legacy, retiring Oct 16 2026
		ModelID:          "gemini-2.5-pro",
		Provider:         ProviderGemini,
		ContextWindow:    1048576,
		InputPerMillion:  1.25,
		OutputPerMillion: 10.00,
		SupportsThinking: true,
	},

	// =========================================================================
	// QWEN / ALIBABA — verified against Alibaba Model Studio pricing, Aug 2026
	// Qwen3.8-Max GA'd Aug 3 2026; Qwen3.7-Max/Plus/Flash still active.
	// =========================================================================
	"qwen3.8-max": {
		ModelID:          "qwen3.8-max",
		Provider:         ProviderAlibaba,
		ContextWindow:    1000000,
		InputPerMillion:  2.00,
		OutputPerMillion: 6.00,
		SupportsThinking: true,
	},
	"qwen3.7-max": {
		ModelID:          "qwen3.7-max",
		Provider:         ProviderAlibaba,
		ContextWindow:    1000000,
		InputPerMillion:  2.50,
		OutputPerMillion: 7.50,
		SupportsThinking: true,
	},
	"qwen3.5-plus": {
		ModelID:          "qwen3.5-plus",
		Provider:         ProviderAlibaba,
		ContextWindow:    1000000,
		InputPerMillion:  0.40,
		OutputPerMillion: 2.40,
		SupportsThinking: true,
	},
	"qwen3.7-flash": {
		ModelID:          "qwen3.7-flash",
		Provider:         ProviderAlibaba,
		ContextWindow:    1000000,
		InputPerMillion:  0.03,
		OutputPerMillion: 0.13,
		SupportsThinking: false,
	},
	"qwen2.5-coder-32b": { // open-weights, common self-host/OpenRouter target
		ModelID:          "qwen2.5-coder-32b",
		Provider:         ProviderOpenRouter,
		ContextWindow:    131072,
		InputPerMillion:  0.07,
		OutputPerMillion: 0.16,
		SupportsThinking: false,
	},

	// =========================================================================
	// KIMI / MOONSHOT AI — verified against Moonshot pricing docs, Aug 2026
	// K3 (July 2026) is a distinct, pricier tier vs. the K2 line.
	// =========================================================================
	"kimi-k3": {
		ModelID:          "kimi-k3",
		Provider:         ProviderMoonshot,
		ContextWindow:    1048576,
		InputPerMillion:  3.00,
		OutputPerMillion: 15.00,
		SupportsThinking: true,
	},
	"kimi-k2.7-code": {
		ModelID:          "kimi-k2.7-code",
		Provider:         ProviderMoonshot,
		ContextWindow:    262144,
		InputPerMillion:  0.95,
		OutputPerMillion: 4.00,
		SupportsThinking: true,
	},
	"kimi-k2.6": {
		ModelID:          "kimi-k2.6",
		Provider:         ProviderMoonshot,
		ContextWindow:    262144,
		InputPerMillion:  0.95,
		OutputPerMillion: 4.00,
		SupportsThinking: true,
	},
	"kimi-k2.5": {
		ModelID:          "kimi-k2.5",
		Provider:         ProviderMoonshot,
		ContextWindow:    262144,
		InputPerMillion:  0.60,
		OutputPerMillion: 3.00,
		SupportsThinking: true,
	},

	// =========================================================================
	// DEEPSEEK — carried over from prior catalog; re-verify against DeepSeek's
	// live pricing page periodically (V3.x/R2-class updates ship frequently).
	// =========================================================================
	"deepseek-chat": {
		ModelID:          "deepseek-chat",
		Provider:         ProviderDeepSeek,
		ContextWindow:    128000,
		InputPerMillion:  0.14,
		OutputPerMillion: 0.28,
		SupportsThinking: false,
	},
	"deepseek-reasoner": {
		ModelID:          "deepseek-reasoner",
		Provider:         ProviderDeepSeek,
		ContextWindow:    128000,
		InputPerMillion:  0.55,
		OutputPerMillion: 2.19,
		SupportsThinking: true,
	},

	// =========================================================================
	// MINIMAX, TENCENT, XAI, MISTRAL — ⚠️ NOT re-verified with live search this
	// pass. These are last-known-good approximations; confirm against each
	// vendor's pricing page before relying on them for billing.
	// =========================================================================
	"minimax-m2": {
		ModelID:          "minimax-m2",
		Provider:         ProviderMiniMax,
		ContextWindow:    1000000,
		InputPerMillion:  0.30,
		OutputPerMillion: 1.20,
		SupportsThinking: true,
	},
	"minimax-text-01": {
		ModelID:          "minimax-text-01",
		Provider:         ProviderMiniMax,
		ContextWindow:    1000000,
		InputPerMillion:  0.20,
		OutputPerMillion: 1.10,
		SupportsThinking: false,
	},
	"hunyuan-turbo": {
		ModelID:          "hunyuan-turbo",
		Provider:         ProviderTencent,
		ContextWindow:    256000,
		InputPerMillion:  0.15,
		OutputPerMillion: 0.60,
		SupportsThinking: false,
	},
	"hunyuan-large": {
		ModelID:          "hunyuan-large",
		Provider:         ProviderTencent,
		ContextWindow:    256000,
		InputPerMillion:  0.30,
		OutputPerMillion: 1.00,
		SupportsThinking: true,
	},
	"grok-4": {
		ModelID:          "grok-4",
		Provider:         ProviderXAI,
		ContextWindow:    256000,
		InputPerMillion:  3.00,
		OutputPerMillion: 15.00,
		SupportsThinking: true,
	},
	"grok-4-fast": {
		ModelID:          "grok-4-fast",
		Provider:         ProviderXAI,
		ContextWindow:    2000000,
		InputPerMillion:  0.20,
		OutputPerMillion: 0.50,
		SupportsThinking: true,
	},
	"mistral-large": {
		ModelID:          "mistral-large",
		Provider:         ProviderMistral,
		ContextWindow:    128000,
		InputPerMillion:  2.00,
		OutputPerMillion: 6.00,
		SupportsThinking: false,
	},
	"mistral-small": {
		ModelID:          "mistral-small",
		Provider:         ProviderMistral,
		ContextWindow:    128000,
		InputPerMillion:  0.10,
		OutputPerMillion: 0.30,
		SupportsThinking: false,
	},
	"codestral": {
		ModelID:          "codestral",
		Provider:         ProviderMistral,
		ContextWindow:    256000,
		InputPerMillion:  0.30,
		OutputPerMillion: 0.90,
		SupportsThinking: false,
	},

	// =========================================================================
	// AWS BEDROCK / GCP VERTEX — mirror the underlying model's rate card
	// =========================================================================
	"bedrock-claude-opus-5": {
		ModelID:          "bedrock-claude-opus-5",
		Provider:         ProviderBedrock,
		ContextWindow:    1000000,
		InputPerMillion:  5.00,
		OutputPerMillion: 25.00,
		SupportsThinking: true,
	},
	"bedrock-nova-pro": {
		ModelID:          "bedrock-nova-pro",
		Provider:         ProviderBedrock,
		ContextWindow:    300000,
		InputPerMillion:  0.80,
		OutputPerMillion: 3.20,
		SupportsThinking: false,
	},
	"vertex-gemini-3.1-pro": {
		ModelID:          "vertex-gemini-3.1-pro",
		Provider:         ProviderVertex,
		ContextWindow:    1048576,
		InputPerMillion:  2.00,
		OutputPerMillion: 12.00,
		SupportsThinking: true,
	},
	"vertex-gemini-3-flash": {
		ModelID:          "vertex-gemini-3-flash",
		Provider:         ProviderVertex,
		ContextWindow:    1048576,
		InputPerMillion:  0.50,
		OutputPerMillion: 3.00,
		SupportsThinking: true,
	},

	// =========================================================================
	// OPENROUTER / MULTI-MODEL FALLBACK CHAIN MODELS
	// =========================================================================
	"minimax/minimax-m3:free": {
		ModelID:          "minimax/minimax-m3:free",
		Provider:         ProviderOpenRouter,
		ContextWindow:    1048576,
		InputPerMillion:  0.00,
		OutputPerMillion: 0.00,
		SupportsThinking: false,
	},
	"nvidia/nemotron-3-ultra-550b-a55b:free": {
		ModelID:          "nvidia/nemotron-3-ultra-550b-a55b:free",
		Provider:         ProviderOpenRouter,
		ContextWindow:    131072,
		InputPerMillion:  0.00,
		OutputPerMillion: 0.00,
		SupportsThinking: false,
	},
	"stealth/ox-alpha": {
		ModelID:          "stealth/ox-alpha",
		Provider:         ProviderOpenRouter,
		ContextWindow:    1048576,
		InputPerMillion:  1.00,
		OutputPerMillion: 3.00,
		SupportsThinking: true,
	},
	"thinkingmachines/inkling:free": {
		ModelID:          "thinkingmachines/inkling:free",
		Provider:         ProviderOpenRouter,
		ContextWindow:    1048576,
		InputPerMillion:  0.00,
		OutputPerMillion: 0.00,
		SupportsThinking: true,
	},
	"nvidia/llama-3.1-nemotron-70b-instruct": {
		ModelID:          "nvidia/llama-3.1-nemotron-70b-instruct",
		Provider:         ProviderOpenRouter,
		ContextWindow:    131072,
		InputPerMillion:  0.35,
		OutputPerMillion: 0.40,
		SupportsThinking: false,
	},
	"nvidia/nemotron-4-340b-instruct": {
		ModelID:          "nvidia/nemotron-4-340b-instruct",
		Provider:         ProviderOpenRouter,
		ContextWindow:    4096,
		InputPerMillion:  1.20,
		OutputPerMillion: 1.20,
		SupportsThinking: false,
	},

	// =========================================================================
	// SELF-HOSTED / LOCAL INFERENCE
	// =========================================================================
	"ollama-deepseek-r1-70b": {
		ModelID:          "ollama-deepseek-r1-70b",
		Provider:         ProviderOllama,
		ContextWindow:    128000,
		InputPerMillion:  0.00,
		OutputPerMillion: 0.00,
		SupportsThinking: true,
	},
	"vllm-qwen-2.5-coder": {
		ModelID:          "vllm-qwen-2.5-coder",
		Provider:         ProviderVLLM,
		ContextWindow:    131072,
		InputPerMillion:  0.00,
		OutputPerMillion: 0.00,
		SupportsThinking: false,
	},

	// =========================================================================
	// Z.AI (ZHIPU AI) — GLM model family
	// Verified against Z.ai official API docs, OpenRouter, and Requesty,
	// August 2026. Direct API endpoint: https://api.z.ai/api/openai/v1
	// Z.ai exposes both an OpenAI-compatible and Anthropic-compatible endpoint,
	// so your existing provider client wiring can reuse either adapter.
	// =========================================================================

	// GLM-5.3 — current flagship, announced August 14, 2026.
	// Reasoning always enabled; same base as GLM-5.2, improved post-training.
	"glm-5.3": {
		ModelID:          "glm-5.3",
		Provider:         ProviderZAI,
		ContextWindow:    1000000,
		InputPerMillion:  1.40,
		OutputPerMillion: 4.40,
		SupportsThinking: true,
	},

	// GLM-5.3-Flash — multimodal, hybrid sparse/linear attention,
	// 1.31M context on OpenRouter, $0.075/$0.25 per 1M tokens. Released Aug 26 2026.
	"glm-5.3-flash": {
		ModelID:          "glm-5.3-flash",
		Provider:         ProviderZAI,
		ContextWindow:    1310000,
		InputPerMillion:  0.075,
		OutputPerMillion: 0.25,
		SupportsThinking: true,
	},

	// GLM-5.2 — large-scale reasoning model, 1M context, released June 16 2026.
	// Strong at coding, tool use, and long-horizon agentic workflows.
	"glm-5.2": {
		ModelID:          "glm-5.2",
		Provider:         ProviderZAI,
		ContextWindow:    1048576,
		InputPerMillion:  1.40,
		OutputPerMillion: 4.40,
		SupportsThinking: true,
	},

	// GLM-5.1 — 745B parameters, 204 800-token context, released April 7 2026.
	// Capable of autonomous execution for 8+ hours on a single task.
	"glm-5.1": {
		ModelID:          "glm-5.1",
		Provider:         ProviderZAI,
		ContextWindow:    204800,
		InputPerMillion:  0.91,
		OutputPerMillion: 2.86,
		SupportsThinking: true,
	},

	// GLM-5 — open-source flagship, 203K context, $1.20/$4.00 on OpenRouter.
	"glm-5": {
		ModelID:          "glm-5",
		Provider:         ProviderZAI,
		ContextWindow:    203000,
		InputPerMillion:  1.20,
		OutputPerMillion: 4.00,
		SupportsThinking: true,
	},

	// GLM-5V-Turbo — first native multimodal agent model from Z.ai.
	// Handles image, video, and text; built for vision-based coding + agent tasks.
	"glm-5v-turbo": {
		ModelID:          "glm-5v-turbo",
		Provider:         ProviderZAI,
		ContextWindow:    204800,
		InputPerMillion:  1.40,
		OutputPerMillion: 4.40,
		SupportsThinking: true,
	},

	// GLM-4.7 — balanced cost-performance tier, 200K context.
	"glm-4.7": {
		ModelID:          "glm-4.7",
		Provider:         ProviderZAI,
		ContextWindow:    200000,
		InputPerMillion:  0.60,
		OutputPerMillion: 2.20,
		SupportsThinking: false,
	},

	// GLM-4.7-Flash — FREE for all registered Z.ai accounts.
	// 203K context window — largest free context window from any major provider.
	"glm-4.7-flash": {
		ModelID:          "glm-4.7-flash",
		Provider:         ProviderZAI,
		ContextWindow:    203000,
		InputPerMillion:  0.00,
		OutputPerMillion: 0.00,
		SupportsThinking: false,
	},

	// GLM-4.6 — previous balanced tier, 200K context, same price as GLM-4.7.
	"glm-4.6": {
		ModelID:          "glm-4.6",
		Provider:         ProviderZAI,
		ContextWindow:    200000,
		InputPerMillion:  0.60,
		OutputPerMillion: 2.20,
		SupportsThinking: false,
	},

	// GLM-4.5 — top-rated ZAI model for vision-language and AI agent tasks.
	"glm-4.5": {
		ModelID:          "glm-4.5",
		Provider:         ProviderZAI,
		ContextWindow:    200000,
		InputPerMillion:  0.60,
		OutputPerMillion: 2.20,
		SupportsThinking: false,
	},

	// GLM-4.5-Air — lighter MoE variant of GLM-4.5; cost-effective for agents.
	"glm-4.5-air": {
		ModelID:          "glm-4.5-air",
		Provider:         ProviderZAI,
		ContextWindow:    200000,
		InputPerMillion:  0.30,
		OutputPerMillion: 1.00,
		SupportsThinking: false,
	},

	// GLM-4.5-Flash — FREE for all registered Z.ai accounts, good for prototyping.
	"glm-4.5-flash": {
		ModelID:          "glm-4.5-flash",
		Provider:         ProviderZAI,
		ContextWindow:    128000,
		InputPerMillion:  0.00,
		OutputPerMillion: 0.00,
		SupportsThinking: false,
	},

	// GLM-4.5V — multimodal variant; excels at vision-language understanding.
	"glm-4.5v": {
		ModelID:          "glm-4.5v",
		Provider:         ProviderZAI,
		ContextWindow:    200000,
		InputPerMillion:  0.60,
		OutputPerMillion: 2.20,
		SupportsThinking: false,
	},

	// =========================================================================
	// TENCENT HUNYUAN — verified against OpenRouter + tokenmix.ai, Aug 2026
	// =========================================================================
	"hunyuan-hy3-preview": {
		ModelID:          "hunyuan-hy3-preview",
		Provider:         ProviderTencent,
		ContextWindow:    256000,
		InputPerMillion:  0.063,
		OutputPerMillion: 0.21,
		SupportsThinking: true,
	},
	"hunyuan-hy3-preview-nonreasoning": {
		ModelID:          "hunyuan-hy3-preview-nonreasoning",
		Provider:         ProviderTencent,
		ContextWindow:    256000,
		InputPerMillion:  0.13,
		OutputPerMillion: 0.52,
		SupportsThinking: false,
	},
	"hunyuan-turbo-s": {
		ModelID:          "hunyuan-turbo-s",
		Provider:         ProviderTencent,
		ContextWindow:    256000,
		InputPerMillion:  0.11,
		OutputPerMillion: 0.28,
		SupportsThinking: false,
	},
	"hunyuan-t1": {
		ModelID:          "hunyuan-t1",
		Provider:         ProviderTencent,
		ContextWindow:    256000,
		InputPerMillion:  0.14,
		OutputPerMillion: 0.57,
		SupportsThinking: true,
	},
	"hunyuan-a13b-instruct": {
		ModelID:          "hunyuan-a13b-instruct",
		Provider:         ProviderTencent,
		ContextWindow:    131072,
		InputPerMillion:  0.14,
		OutputPerMillion: 0.57,
		SupportsThinking: false,
	},

	// =========================================================================
	// META LLAMA — verified against aicostcheck.com + amnic.com, Aug 2026
	// =========================================================================
	"llama-4-scout": {
		ModelID:          "llama-4-scout",
		Provider:         ProviderMeta,
		ContextWindow:    10000000,
		InputPerMillion:  0.08,
		OutputPerMillion: 0.30,
		SupportsThinking: false,
	},
	"llama-4-maverick": {
		ModelID:          "llama-4-maverick",
		Provider:         ProviderMeta,
		ContextWindow:    1000000,
		InputPerMillion:  0.27,
		OutputPerMillion: 0.85,
		SupportsThinking: false,
	},
	"llama-3.3-70b-instruct": {
		ModelID:          "llama-3.3-70b-instruct",
		Provider:         ProviderMeta,
		ContextWindow:    131072,
		InputPerMillion:  0.23,
		OutputPerMillion: 0.40,
		SupportsThinking: false,
	},
	"llama-3.1-405b-instruct": {
		ModelID:          "llama-3.1-405b-instruct",
		Provider:         ProviderMeta,
		ContextWindow:    131072,
		InputPerMillion:  3.50,
		OutputPerMillion: 3.50,
		SupportsThinking: false,
	},
	"llama-3.1-8b-instruct": {
		ModelID:          "llama-3.1-8b-instruct",
		Provider:         ProviderMeta,
		ContextWindow:    131072,
		InputPerMillion:  0.02,
		OutputPerMillion: 0.05,
		SupportsThinking: false,
	},
	"llama-3.2-3b-instruct": {
		ModelID:          "llama-3.2-3b-instruct",
		Provider:         ProviderMeta,
		ContextWindow:    131072,
		InputPerMillion:  0.02,
		OutputPerMillion: 0.05,
		SupportsThinking: false,
	},
	"llama-3.2-11b-vision-instruct": {
		ModelID:          "llama-3.2-11b-vision-instruct",
		Provider:         ProviderMeta,
		ContextWindow:    128000,
		InputPerMillion:  0.16,
		OutputPerMillion: 0.16,
		SupportsThinking: false,
	},
	"llama-3.2-90b-vision-instruct": {
		ModelID:          "llama-3.2-90b-vision-instruct",
		Provider:         ProviderMeta,
		ContextWindow:    128000,
		InputPerMillion:  1.20,
		OutputPerMillion: 1.20,
		SupportsThinking: false,
	},

	// =========================================================================
	// NVIDIA NIM — verified against aipricing.org + tokenando.ai, Aug 2026
	// =========================================================================
	"nvidia-llama-3.1-nemotron-ultra-253b": {
		ModelID:          "nvidia-llama-3.1-nemotron-ultra-253b",
		Provider:         ProviderNvidia,
		ContextWindow:    131072,
		InputPerMillion:  1.60,
		OutputPerMillion: 1.60,
		SupportsThinking: true,
	},
	"nvidia-llama-3.1-nemotron-70b-instruct": {
		ModelID:          "nvidia-llama-3.1-nemotron-70b-instruct",
		Provider:         ProviderNvidia,
		ContextWindow:    131072,
		InputPerMillion:  0.35,
		OutputPerMillion: 0.40,
		SupportsThinking: false,
	},
	"nvidia-nemotron-nano-12b-vl": {
		ModelID:          "nvidia-nemotron-nano-12b-vl",
		Provider:         ProviderNvidia,
		ContextWindow:    131072,
		InputPerMillion:  0.10,
		OutputPerMillion: 0.10,
		SupportsThinking: false,
	},
	"nvidia-nemotron-nano-8b-instruct": {
		ModelID:          "nvidia-nemotron-nano-8b-instruct",
		Provider:         ProviderNvidia,
		ContextWindow:    131072,
		InputPerMillion:  0.04,
		OutputPerMillion: 0.04,
		SupportsThinking: false,
	},
	"nvidia-deepseek-v4": {
		ModelID:          "nvidia-deepseek-v4",
		Provider:         ProviderNvidia,
		ContextWindow:    128000,
		InputPerMillion:  0.00,
		OutputPerMillion: 0.00,
		SupportsThinking: true,
	},

	// =========================================================================
	// GROQ — ultra-fast inference, token-billed. Llama + Mixtral family
	// =========================================================================
	"groq-llama-4-scout": {
		ModelID:          "groq-llama-4-scout",
		Provider:         ProviderGroq,
		ContextWindow:    131072,
		InputPerMillion:  0.11,
		OutputPerMillion: 0.34,
		SupportsThinking: false,
	},
	"groq-llama-4-maverick": {
		ModelID:          "groq-llama-4-maverick",
		Provider:         ProviderGroq,
		ContextWindow:    131072,
		InputPerMillion:  0.50,
		OutputPerMillion: 0.77,
		SupportsThinking: false,
	},
	"groq-llama-3.3-70b": {
		ModelID:          "groq-llama-3.3-70b",
		Provider:         ProviderGroq,
		ContextWindow:    131072,
		InputPerMillion:  0.59,
		OutputPerMillion: 0.79,
		SupportsThinking: false,
	},
	"groq-llama-3.1-8b": {
		ModelID:          "groq-llama-3.1-8b",
		Provider:         ProviderGroq,
		ContextWindow:    131072,
		InputPerMillion:  0.05,
		OutputPerMillion: 0.08,
		SupportsThinking: false,
	},
	"groq-mixtral-8x7b": {
		ModelID:          "groq-mixtral-8x7b",
		Provider:         ProviderGroq,
		ContextWindow:    32768,
		InputPerMillion:  0.24,
		OutputPerMillion: 0.24,
		SupportsThinking: false,
	},

	// =========================================================================
	// TOGETHER AI — strong open-weights host
	// =========================================================================
	"together-llama-4-scout": {
		ModelID:          "together-llama-4-scout",
		Provider:         ProviderTogether,
		ContextWindow:    10000000,
		InputPerMillion:  0.08,
		OutputPerMillion: 0.30,
		SupportsThinking: false,
	},
	"together-llama-4-maverick": {
		ModelID:          "together-llama-4-maverick",
		Provider:         ProviderTogether,
		ContextWindow:    1000000,
		InputPerMillion:  0.27,
		OutputPerMillion: 0.85,
		SupportsThinking: false,
	},
	"together-qwen-2.5-72b": {
		ModelID:          "together-qwen-2.5-72b",
		Provider:         ProviderTogether,
		ContextWindow:    131072,
		InputPerMillion:  0.12,
		OutputPerMillion: 0.12,
		SupportsThinking: false,
	},
	"together-deepseek-r1": {
		ModelID:          "together-deepseek-r1",
		Provider:         ProviderTogether,
		ContextWindow:    128000,
		InputPerMillion:  0.55,
		OutputPerMillion: 2.19,
		SupportsThinking: true,
	},

	// =========================================================================
	// DEEPINFRA — cheapest verified per-token rates for most open-weight models
	// =========================================================================
	"deepinfra-llama-4-maverick": {
		ModelID:          "deepinfra-llama-4-maverick",
		Provider:         ProviderDeepInfra,
		ContextWindow:    1000000,
		InputPerMillion:  0.15,
		OutputPerMillion: 0.60,
		SupportsThinking: false,
	},
	"deepinfra-llama-3.3-70b": {
		ModelID:          "deepinfra-llama-3.3-70b",
		Provider:         ProviderDeepInfra,
		ContextWindow:    131072,
		InputPerMillion:  0.23,
		OutputPerMillion: 0.40,
		SupportsThinking: false,
	},
	"deepinfra-qwen-2.5-72b": {
		ModelID:          "deepinfra-qwen-2.5-72b",
		Provider:         ProviderDeepInfra,
		ContextWindow:    131072,
		InputPerMillion:  0.07,
		OutputPerMillion: 0.16,
		SupportsThinking: false,
	},
	"deepinfra-deepseek-r1": {
		ModelID:          "deepinfra-deepseek-r1",
		Provider:         ProviderDeepInfra,
		ContextWindow:    128000,
		InputPerMillion:  0.55,
		OutputPerMillion: 2.19,
		SupportsThinking: true,
	},
	"deepinfra-mistral-7b": {
		ModelID:          "deepinfra-mistral-7b",
		Provider:         ProviderDeepInfra,
		ContextWindow:    32768,
		InputPerMillion:  0.04,
		OutputPerMillion: 0.04,
		SupportsThinking: false,
	},

	// =========================================================================
	// CEREBRAS — ultra-fast inference on Cerebras wafer-scale chips
	// =========================================================================
	"cerebras-llama-4-scout": {
		ModelID:          "cerebras-llama-4-scout",
		Provider:         ProviderCerebras,
		ContextWindow:    131072,
		InputPerMillion:  0.10,
		OutputPerMillion: 0.10,
		SupportsThinking: false,
	},
	"cerebras-llama-3.3-70b": {
		ModelID:          "cerebras-llama-3.3-70b",
		Provider:         ProviderCerebras,
		ContextWindow:    131072,
		InputPerMillion:  0.59,
		OutputPerMillion: 0.99,
		SupportsThinking: false,
	},
	"cerebras-llama-3.1-8b": {
		ModelID:          "cerebras-llama-3.1-8b",
		Provider:         ProviderCerebras,
		ContextWindow:    131072,
		InputPerMillion:  0.10,
		OutputPerMillion: 0.10,
		SupportsThinking: false,
	},

	// =========================================================================
	// SAMBANOVA CLOUD — fast enterprise inference on SambaNova RDU chips
	// =========================================================================
	"sambanova-llama-4-maverick": {
		ModelID:          "sambanova-llama-4-maverick",
		Provider:         ProviderSambaNova,
		ContextWindow:    131072,
		InputPerMillion:  0.50,
		OutputPerMillion: 1.50,
		SupportsThinking: false,
	},
	"sambanova-llama-3.3-70b": {
		ModelID:          "sambanova-llama-3.3-70b",
		Provider:         ProviderSambaNova,
		ContextWindow:    131072,
		InputPerMillion:  0.60,
		OutputPerMillion: 1.20,
		SupportsThinking: false,
	},
	"sambanova-deepseek-r1": {
		ModelID:          "sambanova-deepseek-r1",
		Provider:         ProviderSambaNova,
		ContextWindow:    128000,
		InputPerMillion:  0.50,
		OutputPerMillion: 2.00,
		SupportsThinking: true,
	},

	// =========================================================================
	// COHERE — Command R family, strong for RAG and enterprise search
	// =========================================================================
	"command-r-plus": {
		ModelID:          "command-r-plus",
		Provider:         ProviderCohere,
		ContextWindow:    128000,
		InputPerMillion:  2.50,
		OutputPerMillion: 10.00,
		SupportsThinking: false,
	},
	"command-r": {
		ModelID:          "command-r",
		Provider:         ProviderCohere,
		ContextWindow:    128000,
		InputPerMillion:  0.15,
		OutputPerMillion: 0.60,
		SupportsThinking: false,
	},
	"command-r7b": {
		ModelID:          "command-r7b",
		Provider:         ProviderCohere,
		ContextWindow:    128000,
		InputPerMillion:  0.04,
		OutputPerMillion: 0.15,
		SupportsThinking: false,
	},
	"command-a": {
		ModelID:          "command-a",
		Provider:         ProviderCohere,
		ContextWindow:    256000,
		InputPerMillion:  2.50,
		OutputPerMillion: 10.00,
		SupportsThinking: false,
	},

	// =========================================================================
	// PERPLEXITY — Sonar models (search-augmented inference)
	// =========================================================================
	"sonar-pro": {
		ModelID:          "sonar-pro",
		Provider:         ProviderPerplexity,
		ContextWindow:    200000,
		InputPerMillion:  3.00,
		OutputPerMillion: 15.00,
		SupportsThinking: false,
	},
	"sonar": {
		ModelID:          "sonar",
		Provider:         ProviderPerplexity,
		ContextWindow:    128000,
		InputPerMillion:  1.00,
		OutputPerMillion: 1.00,
		SupportsThinking: false,
	},
	"sonar-reasoning-pro": {
		ModelID:          "sonar-reasoning-pro",
		Provider:         ProviderPerplexity,
		ContextWindow:    128000,
		InputPerMillion:  2.00,
		OutputPerMillion: 8.00,
		SupportsThinking: true,
	},
	"sonar-reasoning": {
		ModelID:          "sonar-reasoning",
		Provider:         ProviderPerplexity,
		ContextWindow:    128000,
		InputPerMillion:  1.00,
		OutputPerMillion: 5.00,
		SupportsThinking: true,
	},

	// =========================================================================
	// FIREWORKS AI — fast open-weights hosting, competitive with Together AI
	// =========================================================================
	"fireworks-llama-4-maverick": {
		ModelID:          "fireworks-llama-4-maverick",
		Provider:         ProviderFireworks,
		ContextWindow:    1000000,
		InputPerMillion:  0.22,
		OutputPerMillion: 0.88,
		SupportsThinking: false,
	},
	"fireworks-llama-3.3-70b": {
		ModelID:          "fireworks-llama-3.3-70b",
		Provider:         ProviderFireworks,
		ContextWindow:    131072,
		InputPerMillion:  0.90,
		OutputPerMillion: 0.90,
		SupportsThinking: false,
	},
	"fireworks-qwen-2.5-72b": {
		ModelID:          "fireworks-qwen-2.5-72b",
		Provider:         ProviderFireworks,
		ContextWindow:    131072,
		InputPerMillion:  0.90,
		OutputPerMillion: 0.90,
		SupportsThinking: false,
	},
	"fireworks-deepseek-r1": {
		ModelID:          "fireworks-deepseek-r1",
		Provider:         ProviderFireworks,
		ContextWindow:    128000,
		InputPerMillion:  0.55,
		OutputPerMillion: 2.19,
		SupportsThinking: true,
	},
}

var (
	catalogLock   sync.RWMutex
	customCatalog = make(map[string]ModelProfile)
)

// RegisterModelProfile allows runtime registration of custom, enterprise, or
// newly released foundation models — the recommended way to keep pricing
// current between code deploys.
func RegisterModelProfile(profile ModelProfile) {
	catalogLock.Lock()
	defer catalogLock.Unlock()
	customCatalog[profile.ModelID] = profile
}

// GetModelProfile returns technical specs and pricing for a model. If not in
// the static catalog, it heuristically resolves provider/params from naming
// conventions so new/unknown model IDs still get a sane default.
func GetModelProfile(modelID string) (ModelProfile, bool) {
	catalogLock.RLock()
	p, ok := customCatalog[modelID]
	catalogLock.RUnlock()
	if ok {
		return p, true
	}

	if p, ok = defaultCatalog[modelID]; ok {
		return p, true
	}

	lower := strings.ToLower(modelID)

	// 1. OpenRouter-style slugs (vendor/model:variant)
	if strings.Contains(modelID, "/") {
		return ModelProfile{
			ModelID: modelID, Provider: ProviderOpenRouter,
			ContextWindow: 1000000, InputPerMillion: 1.00, OutputPerMillion: 3.00,
			SupportsThinking: true,
		}, true
	}

	switch {
	case strings.Contains(lower, "claude") || strings.Contains(lower, "opus") ||
		strings.Contains(lower, "sonnet") || strings.Contains(lower, "haiku") || strings.Contains(lower, "fable"):
		return ModelProfile{ModelID: modelID, Provider: ProviderAnthropic,
			ContextWindow: 1000000, InputPerMillion: 3.00, OutputPerMillion: 15.00, SupportsThinking: true}, true

	case strings.Contains(lower, "gemini"):
		return ModelProfile{ModelID: modelID, Provider: ProviderGemini,
			ContextWindow: 1048576, InputPerMillion: 2.00, OutputPerMillion: 12.00, SupportsThinking: true}, true

	case strings.Contains(lower, "gpt") || strings.HasPrefix(lower, "o1") || strings.HasPrefix(lower, "o2") ||
		strings.HasPrefix(lower, "o3") || strings.HasPrefix(lower, "o4") || strings.HasPrefix(lower, "o5") ||
		strings.Contains(lower, "sol") || strings.Contains(lower, "tera") || strings.Contains(lower, "terra") || strings.Contains(lower, "luna"):
		return ModelProfile{ModelID: modelID, Provider: ProviderOpenAI,
			ContextWindow: 1050000, InputPerMillion: 2.00, OutputPerMillion: 12.00, SupportsThinking: true}, true

	case strings.Contains(lower, "deepseek"):
		return ModelProfile{ModelID: modelID, Provider: ProviderDeepSeek,
			ContextWindow: 128000, InputPerMillion: 0.55, OutputPerMillion: 2.19, SupportsThinking: true}, true

	case strings.Contains(lower, "qwen"):
		return ModelProfile{ModelID: modelID, Provider: ProviderAlibaba,
			ContextWindow: 1000000, InputPerMillion: 0.60, OutputPerMillion: 2.40, SupportsThinking: true}, true

	case strings.Contains(lower, "kimi") || strings.Contains(lower, "moonshot"):
		return ModelProfile{ModelID: modelID, Provider: ProviderMoonshot,
			ContextWindow: 262144, InputPerMillion: 0.95, OutputPerMillion: 4.00, SupportsThinking: true}, true

	case strings.Contains(lower, "minimax"):
		return ModelProfile{ModelID: modelID, Provider: ProviderMiniMax,
			ContextWindow: 1000000, InputPerMillion: 0.30, OutputPerMillion: 1.20, SupportsThinking: true}, true

	case strings.Contains(lower, "hunyuan") || strings.HasPrefix(lower, "hy3") || strings.HasPrefix(lower, "hy2") || strings.Contains(lower, "tencent"):
		return ModelProfile{ModelID: modelID, Provider: ProviderTencent,
			ContextWindow: 256000, InputPerMillion: 0.13, OutputPerMillion: 0.52, SupportsThinking: false}, true

	case strings.Contains(lower, "glm") || strings.HasPrefix(lower, "z-ai") || strings.HasPrefix(lower, "zai"):
		return ModelProfile{ModelID: modelID, Provider: ProviderZAI,
			ContextWindow: 1000000, InputPerMillion: 1.40, OutputPerMillion: 4.40, SupportsThinking: true}, true

	case strings.Contains(lower, "llama") || strings.Contains(lower, "maverick") || strings.Contains(lower, "scout"):
		return ModelProfile{ModelID: modelID, Provider: ProviderMeta,
			ContextWindow: 131072, InputPerMillion: 0.27, OutputPerMillion: 0.85, SupportsThinking: false}, true

	case strings.Contains(lower, "nemotron") || strings.HasPrefix(lower, "nvidia"):
		return ModelProfile{ModelID: modelID, Provider: ProviderNvidia,
			ContextWindow: 131072, InputPerMillion: 0.35, OutputPerMillion: 0.40, SupportsThinking: false}, true

	case strings.HasPrefix(lower, "groq-"):
		return ModelProfile{ModelID: modelID, Provider: ProviderGroq,
			ContextWindow: 131072, InputPerMillion: 0.30, OutputPerMillion: 0.50, SupportsThinking: false}, true

	case strings.HasPrefix(lower, "together-"):
		return ModelProfile{ModelID: modelID, Provider: ProviderTogether,
			ContextWindow: 131072, InputPerMillion: 0.20, OutputPerMillion: 0.60, SupportsThinking: false}, true

	case strings.HasPrefix(lower, "deepinfra-"):
		return ModelProfile{ModelID: modelID, Provider: ProviderDeepInfra,
			ContextWindow: 131072, InputPerMillion: 0.15, OutputPerMillion: 0.40, SupportsThinking: false}, true

	case strings.HasPrefix(lower, "cerebras-"):
		return ModelProfile{ModelID: modelID, Provider: ProviderCerebras,
			ContextWindow: 131072, InputPerMillion: 0.20, OutputPerMillion: 0.40, SupportsThinking: false}, true

	case strings.HasPrefix(lower, "sambanova-"):
		return ModelProfile{ModelID: modelID, Provider: ProviderSambaNova,
			ContextWindow: 131072, InputPerMillion: 0.50, OutputPerMillion: 1.50, SupportsThinking: false}, true

	case strings.Contains(lower, "command-") || strings.HasPrefix(lower, "cohere"):
		return ModelProfile{ModelID: modelID, Provider: ProviderCohere,
			ContextWindow: 128000, InputPerMillion: 1.00, OutputPerMillion: 4.00, SupportsThinking: false}, true

	case strings.Contains(lower, "sonar") || strings.HasPrefix(lower, "perplexity"):
		return ModelProfile{ModelID: modelID, Provider: ProviderPerplexity,
			ContextWindow: 128000, InputPerMillion: 1.00, OutputPerMillion: 5.00, SupportsThinking: true}, true

	case strings.HasPrefix(lower, "fireworks-"):
		return ModelProfile{ModelID: modelID, Provider: ProviderFireworks,
			ContextWindow: 131072, InputPerMillion: 0.50, OutputPerMillion: 1.00, SupportsThinking: false}, true

	case strings.Contains(lower, "grok"):
		return ModelProfile{ModelID: modelID, Provider: ProviderXAI,
			ContextWindow: 256000, InputPerMillion: 2.00, OutputPerMillion: 10.00, SupportsThinking: true}, true

	case strings.Contains(lower, "mistral") || strings.Contains(lower, "codestral"):
		return ModelProfile{ModelID: modelID, Provider: ProviderMistral,
			ContextWindow: 128000, InputPerMillion: 1.00, OutputPerMillion: 3.00, SupportsThinking: false}, true

	case strings.HasPrefix(lower, "bedrock"):
		return ModelProfile{ModelID: modelID, Provider: ProviderBedrock,
			ContextWindow: 1000000, InputPerMillion: 1.00, OutputPerMillion: 3.00, SupportsThinking: true}, true

	case strings.HasPrefix(lower, "vertex"):
		return ModelProfile{ModelID: modelID, Provider: ProviderVertex,
			ContextWindow: 1048576, InputPerMillion: 2.00, OutputPerMillion: 12.00, SupportsThinking: true}, true

	case strings.HasPrefix(lower, "ollama"):
		return ModelProfile{ModelID: modelID, Provider: ProviderOllama,
			ContextWindow: 128000, InputPerMillion: 0.00, OutputPerMillion: 0.00, SupportsThinking: true}, true

	case strings.HasPrefix(lower, "vllm"):
		return ModelProfile{ModelID: modelID, Provider: ProviderVLLM,
			ContextWindow: 128000, InputPerMillion: 0.00, OutputPerMillion: 0.00, SupportsThinking: true}, true
	}

	// Fallback for unknown BYOK/custom/enterprise models
	return ModelProfile{
		ModelID: modelID, Provider: ProviderOpenRouter,
		ContextWindow: 1000000, InputPerMillion: 1.00, OutputPerMillion: 3.00,
		SupportsThinking: true,
	}, true
}

// CalculateCost computes the USD cost of an inference call, rounded to 6 decimals.
func CalculateCost(modelID string, promptTokens, completionTokens int) float64 {
	profile, ok := GetModelProfile(modelID)
	if !ok {
		profile = defaultCatalog["gpt-4o"]
	}

	inputCost := (float64(promptTokens) / 1_000_000.0) * profile.InputPerMillion
	outputCost := (float64(completionTokens) / 1_000_000.0) * profile.OutputPerMillion

	total := inputCost + outputCost
	return math.Round(total*1_000_000) / 1_000_000
}

package chunking

import (
	"encoding/json"
	"math"
	"strings"
)

// DefaultTokenChunkingMaxTokens is the standard window limit (64,000 tokens).
const DefaultTokenChunkingMaxTokens = 64000

// DefaultTokenChunkingUsagePercentage is the safety utilization headroom (60%).
const DefaultTokenChunkingUsagePercentage = 60

// TokenChunkingOptions mirrors ScanDrix TokenChunkingOptions.
type TokenChunkingOptions struct {
	Model             string `json:"model"`
	Data              []any  `json:"data"`
	UsagePercentage   int    `json:"usagePercentage,omitempty"`
	DefaultMaxTokens  int    `json:"defaultMaxTokens,omitempty"`
	OverrideMaxTokens int    `json:"overrideMaxTokens,omitempty"`
}

// TokenChunkingResult mirrors ScanDrix TokenChunkingResult.
type TokenChunkingResult struct {
	Chunks         [][]any `json:"chunks"`
	TotalItems     int     `json:"totalItems"`
	TotalChunks    int     `json:"totalChunks"`
	TokensPerChunk []int   `json:"tokensPerChunk"`
	TokenLimit     int     `json:"tokenLimit"`
	ModelUsed      string  `json:"modelUsed"`
}

// TokenChunkingService mirrors ScanDrix TokenChunkingService: splits data payloads based on model token limits.
type TokenChunkingService struct{}

// NewTokenChunkingService instantiates a token chunking service.
func NewTokenChunkingService() *TokenChunkingService {
	return &TokenChunkingService{}
}

// GetMaxTokensForModel returns maximum context window capacity per model family.
func (s *TokenChunkingService) GetMaxTokensForModel(model string, defaultMaxTokens int) int {
	if defaultMaxTokens <= 0 {
		defaultMaxTokens = DefaultTokenChunkingMaxTokens
	}

	m := strings.ToLower(model)
	if strings.Contains(m, "gemini") {
		return 1000000 // 1M tokens
	}
	if strings.Contains(m, "claude-3") || strings.Contains(m, "claude-sonnet") || strings.Contains(m, "claude-opus") {
		return 200000 // 200K tokens
	}
	if strings.Contains(m, "gpt-4o") || strings.Contains(m, "gpt-4-turbo") || strings.Contains(m, "o1") || strings.Contains(m, "o3") {
		return 128000 // 128K tokens
	}
	if strings.Contains(m, "gpt-4") {
		return 32768
	}
	if strings.Contains(m, "gpt-3.5") {
		return 16384
	}

	return defaultMaxTokens
}

// EstimateTokens provides accurate token count estimation based on byte density and characters.
func (s *TokenChunkingService) EstimateTokens(item any) int {
	if item == nil {
		return 0
	}

	var text string
	switch v := item.(type) {
	case string:
		text = v
	case []byte:
		text = string(v)
	default:
		bytes, err := json.Marshal(item)
		if err != nil {
			return 0
		}
		text = string(bytes)
	}

	if len(text) == 0 {
		return 0
	}

	// 1 token ~= 3.5 characters average across mixed code and JSON
	tokens := int(math.Ceil(float64(len(text)) / 3.5))
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

// ChunkDataByTokens splits data into chunks based on the LLM model's token limit.
func (s *TokenChunkingService) ChunkDataByTokens(options TokenChunkingOptions) TokenChunkingResult {
	model := options.Model
	if model == "" {
		model = "default"
	}

	if len(options.Data) == 0 {
		return TokenChunkingResult{
			Chunks:         [][]any{},
			TotalItems:     0,
			TotalChunks:    0,
			TokensPerChunk: []int{},
			TokenLimit:     0,
			ModelUsed:      model,
		}
	}

	usagePct := options.UsagePercentage
	if usagePct <= 0 || usagePct > 100 {
		usagePct = DefaultTokenChunkingUsagePercentage
	}

	defaultMax := options.DefaultMaxTokens
	if defaultMax <= 0 {
		defaultMax = DefaultTokenChunkingMaxTokens
	}

	maxTokens := options.OverrideMaxTokens
	if maxTokens <= 0 {
		maxTokens = s.GetMaxTokensForModel(model, defaultMax)
	}

	tokenLimit := int(math.Floor(float64(maxTokens) * (float64(usagePct) / 100.0)))
	if tokenLimit <= 0 {
		tokenLimit = 1000
	}

	var chunks [][]any
	var tokensPerChunk []int

	var currentChunk []any
	currentTokens := 0

	for _, item := range options.Data {
		itemTokens := s.EstimateTokens(item)

		// Edge case: single item exceeds entire chunk limit, isolate in dedicated chunk
		if itemTokens >= tokenLimit {
			if len(currentChunk) > 0 {
				chunks = append(chunks, currentChunk)
				tokensPerChunk = append(tokensPerChunk, currentTokens)
				currentChunk = nil
				currentTokens = 0
			}
			chunks = append(chunks, []any{item})
			tokensPerChunk = append(tokensPerChunk, itemTokens)
			continue
		}

		if currentTokens+itemTokens > tokenLimit && len(currentChunk) > 0 {
			chunks = append(chunks, currentChunk)
			tokensPerChunk = append(tokensPerChunk, currentTokens)
			currentChunk = []any{item}
			currentTokens = itemTokens
		} else {
			currentChunk = append(currentChunk, item)
			currentTokens += itemTokens
		}
	}

	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
		tokensPerChunk = append(tokensPerChunk, currentTokens)
	}

	return TokenChunkingResult{
		Chunks:         chunks,
		TotalItems:     len(options.Data),
		TotalChunks:    len(chunks),
		TokensPerChunk: tokensPerChunk,
		TokenLimit:     tokenLimit,
		ModelUsed:      model,
	}
}

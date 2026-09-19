package document

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Document struct {
	PageContent string                 `json:"pageContent"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type OpenAIEmbeddingItem struct {
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
	Object    string    `json:"object"`
}

type OpenAIEmbeddingResponse struct {
	Data   []OpenAIEmbeddingItem `json:"data"`
	Model  string                `json:"model"`
	Object string                `json:"object"`
}

type EmbeddingOptions struct {
	Model      string `json:"model"`
	APIKey     string `json:"apiKey"`
	BaseURL    string `json:"baseUrl"`
	HTTPClient *http.Client
}

const DefaultEmbeddingModel = "text-embedding-3-small"

type PlatformEmbedder struct {
	APIKey     string
	Model      string
	BaseURL    string
	httpClient *http.Client
}

var (
	embedderCacheMu sync.RWMutex
	embedderCache   = make(map[string]*PlatformEmbedder)
)

func CreateDocument(formattedData string, metaData map[string]interface{}) Document {
	md := make(map[string]interface{})
	for k, v := range metaData {
		md[k] = v
	}
	return Document{
		PageContent: formattedData,
		Metadata:    md,
	}
}

func EstimateTokenCount(text string) int {
	byteCount := len([]byte(text))
	// Estimate token count based on average of 4 bytes per token
	return byteCount / 4
}

func BuildPlatformEmbedder(opts ...EmbeddingOptions) *PlatformEmbedder {
	var opt EmbeddingOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	apiKey := opt.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("API_OPEN_AI_API_KEY")
	}
	if apiKey == "" {
		return nil
	}

	model := opt.Model
	if model == "" {
		model = DefaultEmbeddingModel
	}

	baseURL := opt.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	cacheKey := fmt.Sprintf("%s:%s:%s", apiKey, model, baseURL)

	embedderCacheMu.RLock()
	cached, ok := embedderCache[cacheKey]
	embedderCacheMu.RUnlock()
	if ok {
		return cached
	}

	embedderCacheMu.Lock()
	defer embedderCacheMu.Unlock()
	if cached, ok = embedderCache[cacheKey]; ok {
		return cached
	}

	client := opt.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	embedder := &PlatformEmbedder{
		APIKey:     apiKey,
		Model:      model,
		BaseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: client,
	}

	embedderCache[cacheKey] = embedder
	return embedder
}

func (e *PlatformEmbedder) Embed(ctx context.Context, input string) (*OpenAIEmbeddingResponse, error) {
	// Strip newlines as per LangChain OpenAIEmbeddings default
	value := strings.ReplaceAll(input, "\n", " ")

	reqBody := map[string]interface{}{
		"model": e.Model,
		"input": value,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding request: %w", err)
	}

	url := fmt.Sprintf("%s/embeddings", e.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.APIKey))

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding API returned error HTTP %d: %s", resp.StatusCode, string(body))
	}

	var res OpenAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode embedding response: %w", err)
	}

	return &res, nil
}

func GetOpenAIEmbedding(ctx context.Context, input string, opts ...EmbeddingOptions) (*OpenAIEmbeddingResponse, error) {
	embedder := BuildPlatformEmbedder(opts...)
	if embedder == nil {
		return nil, fmt.Errorf("no platform OpenAI key configured for embeddings (API_OPEN_AI_API_KEY)")
	}
	return embedder.Embed(ctx, input)
}

// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgparamusecases

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ByokTestInput carries credentials to probe.
type ByokTestInput struct {
	Provider        string   `json:"provider"`
	APIKey          string   `json:"apiKey"`
	BaseURL         string   `json:"baseUrl,omitempty"`
	Region          string   `json:"region,omitempty"`
	ModelID         string   `json:"modelId,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
	ReasoningEffort string   `json:"reasoningEffort,omitempty"`
}

// ByokTestResult details probe outcome.
type ByokTestResult struct {
	Success         bool   `json:"success"`
	Code            string `json:"code"` // ok, auth, not_found, bad_request, rate_limit, server_error, network, unknown
	LatencyMs       int64  `json:"latencyMs"`
	Message         string `json:"message"`
	ProviderMessage string `json:"providerMessage,omitempty"`
	HTTPStatus      int    `json:"httpStatus,omitempty"`
}

// TestBYOKConnectionUseCase executes live authenticated probe against the target AI provider.
type TestBYOKConnectionUseCase struct {
	httpClient *http.Client
}

func NewTestBYOKConnectionUseCase() *TestBYOKConnectionUseCase {
	return &TestBYOKConnectionUseCase{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// WithHTTPClient injects the HTTP client used for the live probe. The default
// client is used when none is supplied; this exists so the probe can be
// exercised against a local test server rather than a real provider.
func (uc *TestBYOKConnectionUseCase) WithHTTPClient(c *http.Client) *TestBYOKConnectionUseCase {
	if c != nil {
		uc.httpClient = c
	}
	return uc
}

// Execute performs a real authenticated probe against the provider. It never
// reports success without a successful provider response: the returned Success
// flag is true only for a 2xx from the provider.
func (uc *TestBYOKConnectionUseCase) Execute(ctx context.Context, input ByokTestInput) ByokTestResult {
	start := time.Now()
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	apiKey := strings.TrimSpace(input.APIKey)
	baseURL := strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	model := strings.TrimSpace(input.ModelID)

	if apiKey == "" && provider != "bedrock" && provider != "vertex" {
		return ByokTestResult{
			Success:   false,
			Code:      "auth",
			LatencyMs: 0,
			Message:   "API key cannot be empty",
		}
	}

	if baseURL != "" {
		if err := AssertSafeURL(baseURL); err != nil {
			return ByokTestResult{
				Success:   false,
				Code:      "bad_request",
				LatencyMs: 0,
				Message:   err.Error(),
			}
		}
	}

	req, err := uc.buildProbeRequest(ctx, provider, apiKey, baseURL, model)
	if err != nil {
		return ByokTestResult{
			Success:   false,
			Code:      "bad_request",
			LatencyMs: time.Since(start).Milliseconds(),
			Message:   err.Error(),
		}
	}

	resp, err := uc.httpClient.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return ByokTestResult{
			Success:   false,
			Code:      "network",
			LatencyMs: latency,
			Message:   fmt.Sprintf("Failed to connect to provider %s: %v", input.Provider, err),
		}
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 16384))
	bodyStr := strings.TrimSpace(string(bodyBytes))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return ByokTestResult{
			Success:    true,
			Code:       "ok",
			LatencyMs:  latency,
			HTTPStatus: resp.StatusCode,
			Message:    fmt.Sprintf("Provider %s connection validated successfully", input.Provider),
		}
	}

	// Classify error code based on HTTP response
	code := "unknown"
	message := fmt.Sprintf("Provider returned HTTP status %d", resp.StatusCode)
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		code = "auth"
		message = "Authentication failed: invalid or unauthorized API key"
	case http.StatusNotFound:
		code = "not_found"
		message = "Requested model or provider endpoint not found"
	case http.StatusBadRequest:
		code = "bad_request"
		message = "Invalid request format or model parameters"
	case http.StatusTooManyRequests, 402:
		code = "rate_limit"
		message = "Rate limit exceeded or insufficient API credit quota"
	default:
		if resp.StatusCode >= 500 {
			code = "server_error"
			message = "Provider upstream server error"
		}
	}

	return ByokTestResult{
		Success:         false,
		Code:            code,
		LatencyMs:       latency,
		HTTPStatus:      resp.StatusCode,
		Message:         message,
		ProviderMessage: bodyStr,
	}
}

func (uc *TestBYOKConnectionUseCase) buildProbeRequest(ctx context.Context, provider, apiKey, baseURL, model string) (*http.Request, error) {
	switch provider {
	case "anthropic", "anthropic_compatible":
		root := baseURL
		if root == "" {
			root = "https://api.anthropic.com"
		}
		if model != "" {
			payload := map[string]any{
				"model":      model,
				"max_tokens": 1,
				"messages":   []map[string]string{{"role": "user", "content": "ping"}},
			}
			body, _ := json.Marshal(payload)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, root+"/v1/messages", bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			req.Header.Set("x-api-key", apiKey)
			req.Header.Set("anthropic-version", "2023-06-01")
			req.Header.Set("Content-Type", "application/json")
			return req, nil
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, root+"/v1/models", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		return req, nil

	case "gemini", "google":
		if model != "" {
			url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
			payload := map[string]any{
				"contents": []map[string]any{
					{"parts": []map[string]string{{"text": "ping"}}},
				},
			}
			body, _ := json.Marshal(payload)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			return req, nil
		}
		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", apiKey)
		return http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	case "azure":
		if baseURL == "" {
			return nil, errors.New("azure provider requires a baseUrl endpoint")
		}
		url := fmt.Sprintf("%s/models?api-version=2024-02-01", baseURL)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("api-key", apiKey)
		return req, nil

	default: // openai, openrouter, novita, openai_compatible, and other compatible providers
		root := baseURL
		if root == "" {
			switch provider {
			case "openrouter":
				root = "https://openrouter.ai/api/v1"
			case "novita":
				root = "https://api.novita.ai/v3/openai"
			default:
				root = "https://api.openai.com/v1"
			}
		}
		if model != "" {
			payload := map[string]any{
				"model":      model,
				"max_tokens": 1,
				"messages":   []map[string]string{{"role": "user", "content": "ping"}},
			}
			body, _ := json.Marshal(payload)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, root+"/chat/completions", bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+apiKey)
			req.Header.Set("Content-Type", "application/json")
			return req, nil
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, root+"/models", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		return req, nil
	}
}

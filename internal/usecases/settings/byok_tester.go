package settings

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

// BYOKConnectionTester verifies live customer LLM provider API credentials.
type BYOKConnectionTester struct {
	httpClient *http.Client
}

func NewBYOKConnectionTester(timeout time.Duration) *BYOKConnectionTester {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &BYOKConnectionTester{
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// TestConnection executes a live authenticated probe against the target AI provider.
func (t *BYOKConnectionTester) TestConnection(ctx context.Context, input ByokTestInput) ByokTestResult {
	start := time.Now()

	// 1. Parameter Validation
	if strings.TrimSpace(input.APIKey) == "" {
		return ByokTestResult{
			Success:   false,
			Code:      ByokResultAuth,
			LatencyMs: 0,
			Message:   "API key cannot be empty",
		}
	}

	if err := AssertSafeRegion(input.Region); err != nil {
		return ByokTestResult{
			Success:   false,
			Code:      ByokResultBadRequest,
			LatencyMs: 0,
			Message:   err.Error(),
		}
	}

	// 2. Resolve Probe Request
	provider := strings.ToLower(input.Provider)
	req, err := t.buildProbeRequest(ctx, provider, input)
	if err != nil {
		return ByokTestResult{
			Success:   false,
			Code:      ByokResultBadRequest,
			LatencyMs: 0,
			Message:   err.Error(),
		}
	}

	// 3. Execute HTTP Probe
	resp, err := t.httpClient.Do(req)
	latency := time.Since(start)
	latencyMs := latency.Milliseconds()

	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || strings.Contains(err.Error(), "timeout") {
			return ByokTestResult{
				Success:   false,
				Code:      ByokResultTimeout,
				Latency:   latency,
				LatencyMs: latencyMs,
				Message:   "Connection timed out while probing provider",
			}
		}
		return ByokTestResult{
			Success:   false,
			Code:      ByokResultNetwork,
			Latency:   latency,
			LatencyMs: latencyMs,
			Message:   fmt.Sprintf("Network connection failed: %v", err),
		}
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	providerMsg := string(bodyBytes)

	// 4. Map Status Code
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return ByokTestResult{
			Success:    true,
			Code:       ByokResultOK,
			Latency:    latency,
			LatencyMs:  latencyMs,
			Message:    "Credentials verified successfully",
			HTTPStatus: resp.StatusCode,
		}
	}

	code := ByokResultServerError
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		code = ByokResultAuth
	case http.StatusNotFound:
		code = ByokResultNotFound
	case http.StatusPaymentRequired:
		code = ByokResultPayment
	case http.StatusTooManyRequests:
		code = ByokResultRateLimit
	case http.StatusBadRequest:
		code = ByokResultBadRequest
	}

	return ByokTestResult{
		Success:         false,
		Code:            code,
		Latency:         latency,
		LatencyMs:       latencyMs,
		Message:         fmt.Sprintf("Provider rejected probe with HTTP %d", resp.StatusCode),
		ProviderMessage: providerMsg,
		HTTPStatus:      resp.StatusCode,
	}
}

func (t *BYOKConnectionTester) buildProbeRequest(ctx context.Context, provider string, input ByokTestInput) (*http.Request, error) {
	switch provider {
	case "openai":
		url := "https://api.openai.com/v1/models"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+input.APIKey)
		return req, nil

	case "anthropic":
		// Minimal 1-token probe
		url := "https://api.anthropic.com/v1/messages"
		payload := map[string]any{
			"model":      "claude-3-5-haiku-20241022",
			"max_tokens": 1,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		}
		data, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-api-key", input.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("Content-Type", "application/json")
		return req, nil

	case "gemini":
		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", input.APIKey)
		return http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	case "novita":
		url := "https://api.novita.ai/v3/openai/models"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+input.APIKey)
		return req, nil

	case "openai_compatible", "custom":
		if strings.TrimSpace(input.BaseURL) == "" {
			return nil, errors.New("baseURL is required for custom OpenAI-compatible providers")
		}
		// SSRF Guard
		if err := AssertSafeEndpoint(input.BaseURL); err != nil {
			return nil, err
		}
		modelsURL := strings.TrimRight(input.BaseURL, "/") + "/models"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+input.APIKey)
		return req, nil

	default:
		return nil, fmt.Errorf("unsupported AI provider: %s", provider)
	}
}

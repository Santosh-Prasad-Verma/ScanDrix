package llm

import (
	"fmt"
	"net/http"
	"strings"
)

// ErrorCategory classifies AI provider failures into actionable domains.
type ErrorCategory string

const (
	CategoryAuthInvalid       ErrorCategory = "AUTH_INVALID"
	CategoryQuotaExceeded     ErrorCategory = "QUOTA_EXCEEDED"
	CategoryRateLimit         ErrorCategory = "RATE_LIMIT"
	CategoryModelNotFound     ErrorCategory = "MODEL_NOT_FOUND"
	CategoryModelAccessDenied ErrorCategory = "MODEL_ACCESS_DENIED"
	CategoryContextOverflow   ErrorCategory = "CONTEXT_OVERFLOW"
	CategoryTransient         ErrorCategory = "TRANSIENT"
	CategoryUnknown           ErrorCategory = "UNKNOWN"
)

// ClassifiedError standardizes provider error responses across OpenAI, Anthropic, Gemini, Vertex, and Novita.
type ClassifiedError struct {
	Category        ErrorCategory `json:"category"`
	IsTerminal      bool          `json:"is_terminal"`
	IsTransient     bool          `json:"is_transient"`
	HTTPStatus      int           `json:"http_status,omitempty"`
	FriendlyMessage string        `json:"friendly_message"`
	RawMessage      string        `json:"raw_message"`
}

func (e ClassifiedError) Error() string {
	return fmt.Sprintf("[%s] %s (HTTP %d): %s", e.Category, e.FriendlyMessage, e.HTTPStatus, e.RawMessage)
}

// ClassifyLLMError analyzes raw provider messages and HTTP status codes to determine retry and recovery actions.
func ClassifyLLMError(err error, httpStatus int) ClassifiedError {
	raw := ""
	if err != nil {
		raw = err.Error()
	}
	lower := strings.ToLower(raw)

	// 1. Context Overflow (Token budget exceeded)
	if strings.Contains(lower, "context_length_exceeded") ||
		strings.Contains(lower, "maximum context length") ||
		strings.Contains(lower, "prompt is too long") ||
		strings.Contains(lower, "token limit exceeded") {
		return ClassifiedError{
			Category:        CategoryContextOverflow,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "The code review diff exceeds the model's token context window. File pruning or chunking required.",
			RawMessage:      raw,
		}
	}

	// 2. HTTP Status Code Mapping
	switch httpStatus {
	case http.StatusUnauthorized:
		return ClassifiedError{
			Category:        CategoryAuthInvalid,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Invalid or expired AI provider API key. Please update your BYOK credentials.",
			RawMessage:      raw,
		}
	case http.StatusPaymentRequired:
		return ClassifiedError{
			Category:        CategoryQuotaExceeded,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Provider balance depleted or spending quota reached. Please refill your provider account balance.",
			RawMessage:      raw,
		}
	case http.StatusTooManyRequests:
		return ClassifiedError{
			Category:        CategoryRateLimit,
			IsTerminal:      false,
			IsTransient:     true,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "AI provider rate limit reached (requests or tokens per minute). Retrying with backoff.",
			RawMessage:      raw,
		}
	case http.StatusNotFound:
		return ClassifiedError{
			Category:        CategoryModelNotFound,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Specified AI model ID was not found or is discontinued by the provider.",
			RawMessage:      raw,
		}
	case http.StatusForbidden:
		if strings.Contains(lower, "model garden") || strings.Contains(lower, "access") || strings.Contains(lower, "permission") {
			return ClassifiedError{
				Category:        CategoryModelAccessDenied,
				IsTerminal:      true,
				IsTransient:     false,
				HTTPStatus:      httpStatus,
				FriendlyMessage: "Model exists but your cloud project lacks entitlement. Enable it in your provider's Model Garden.",
				RawMessage:      raw,
			}
		}
		return ClassifiedError{
			Category:        CategoryAuthInvalid,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Access forbidden by AI provider. Verify project permissions.",
			RawMessage:      raw,
		}
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return ClassifiedError{
			Category:        CategoryTransient,
			IsTerminal:      false,
			IsTransient:     true,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Temporary provider outage. Retrying automatically.",
			RawMessage:      raw,
		}
	}

	// 3. Fallback Substring Matching
	switch {
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests"):
		return ClassifiedError{
			Category:        CategoryRateLimit,
			IsTerminal:      false,
			IsTransient:     true,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Rate limit reached. Retrying with backoff.",
			RawMessage:      raw,
		}
	case strings.Contains(lower, "quota") || strings.Contains(lower, "billing"):
		return ClassifiedError{
			Category:        CategoryQuotaExceeded,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "AI provider quota exceeded.",
			RawMessage:      raw,
		}
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "connection reset") || strings.Contains(lower, "econnrefused"):
		return ClassifiedError{
			Category:        CategoryTransient,
			IsTerminal:      false,
			IsTransient:     true,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Network connection blip while communicating with AI provider.",
			RawMessage:      raw,
		}
	default:
		return ClassifiedError{
			Category:        CategoryUnknown,
			IsTerminal:      false,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Unexpected error during AI model execution.",
			RawMessage:      raw,
		}
	}
}

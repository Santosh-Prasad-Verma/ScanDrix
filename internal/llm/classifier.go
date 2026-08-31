package llm

import (
	"fmt"
	"net/http"
	"strings"
)

// ErrorCategory classifies AI provider failures into actionable domains.
type ErrorCategory string

const (
	CategoryAuthInvalid            ErrorCategory = "AUTH_INVALID"
	CategoryQuotaExceeded          ErrorCategory = "QUOTA_EXCEEDED"
	CategoryRateLimit              ErrorCategory = "RATE_LIMIT"
	CategoryModelNotFound          ErrorCategory = "MODEL_NOT_FOUND"
	CategoryModelAccessDenied      ErrorCategory = "MODEL_ACCESS_DENIED"
	CategoryContextOverflow        ErrorCategory = "CONTEXT_OVERFLOW"
	CategoryContentFilterBlocked   ErrorCategory = "CONTENT_FILTER_BLOCKED"
	CategoryTransient              ErrorCategory = "TRANSIENT"
	CategoryUnknown                ErrorCategory = "UNKNOWN"
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

func extractStatusFromError(err error) int {
	if err == nil {
		return 0
	}
	msg := err.Error()
	idx := strings.Index(msg, "HTTP ")
	if idx != -1 && len(msg) >= idx+8 {
		statusStr := msg[idx+5 : idx+8]
		var status int
		if n, _ := fmt.Sscanf(statusStr, "%d", &status); n == 1 && status >= 100 && status <= 599 {
			return status
		}
	}
	return 0
}

// ClassifyLLMError analyzes raw provider messages and HTTP status codes to determine retry and recovery actions.
func ClassifyLLMError(err error, httpStatus int) ClassifiedError {
	if httpStatus == 0 {
		httpStatus = extractStatusFromError(err)
	}

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
	case strings.Contains(lower, "content_filter") ||
		strings.Contains(lower, "safety_ratings") ||
		strings.Contains(lower, "responsible ai") ||
		strings.Contains(lower, "moderation"):
		return ClassifiedError{
			Category:        CategoryContentFilterBlocked,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "AI provider content safety filter blocked the diff payload.",
			RawMessage:      raw,
		}
	case strings.Contains(lower, "invalid_api_key") ||
		strings.Contains(lower, "incorrect api key") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "authentication failed") ||
		strings.Contains(lower, "permission_denied"):
		return ClassifiedError{
			Category:        CategoryAuthInvalid,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Invalid or expired AI provider API key. Please update your BYOK credentials.",
			RawMessage:      raw,
		}
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests"):
		return ClassifiedError{
			Category:        CategoryRateLimit,
			IsTerminal:      false,
			IsTransient:     true,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Rate limit reached. Retrying with backoff.",
			RawMessage:      raw,
		}
	case strings.Contains(lower, "quota") || strings.Contains(lower, "billing") || strings.Contains(lower, "insufficient_quota"):
		return ClassifiedError{
			Category:        CategoryQuotaExceeded,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "AI provider quota exceeded.",
			RawMessage:      raw,
		}
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "connection reset") || strings.Contains(lower, "econnrefused") || strings.Contains(lower, "etimedout"):
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

// ShouldFailover determines whether a provider failure warrants attempting the next candidate in the cascade.
func ShouldFailover(classified ClassifiedError) bool {
	switch classified.Category {
	case CategoryTransient:
		return true
	case CategoryAuthInvalid, CategoryQuotaExceeded, CategoryModelAccessDenied, CategoryModelNotFound:
		// Terminal errors on specific model credentials allow cascading to backup providers,
		// but should be logged with alert-level provenance.
		return true
	case CategoryRateLimit:
		// Rate limit is managed by cooldown backoff on the provider's circuit breaker
		return false
	case CategoryContextOverflow, CategoryContentFilterBlocked:
		// Do not burn other providers on context overflow or content filter blocks
		return false
	default:
		return false
	}
}

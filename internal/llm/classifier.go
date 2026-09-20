// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package llm

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/llm/byok"
)

// ErrorCategory classifies AI provider failures into actionable domains in the ScanDrix LLM subsystem.
type ErrorCategory string

const (
	CategoryAuthInvalid          ErrorCategory = "AUTH_INVALID"
	CategoryQuotaExceeded        ErrorCategory = "QUOTA_EXCEEDED"
	CategoryRateLimit            ErrorCategory = "RATE_LIMIT"
	CategoryModelNotFound        ErrorCategory = "MODEL_NOT_FOUND"
	CategoryModelAccessDenied    ErrorCategory = "MODEL_ACCESS_DENIED"
	CategoryContextOverflow      ErrorCategory = "CONTEXT_OVERFLOW"
	CategoryContentFilterBlocked ErrorCategory = "CONTENT_FILTER_BLOCKED"
	CategoryAbortTimeout         ErrorCategory = "ABORT_TIMEOUT"
	CategoryUnsupportedParam     ErrorCategory = "UNSUPPORTED_PARAM"
	CategoryTransient            ErrorCategory = "TRANSIENT"
	CategoryUnknown              ErrorCategory = "UNKNOWN"
)

// AttemptedSlotProvider represents any error that carries the model and provider that actually ran on the attempt.
type AttemptedSlotProvider interface {
	AttemptedSlot() (model string, provider string)
}

// AttemptStampedError wraps an underlying error with the specific slot attempt that produced it.
type AttemptStampedError struct {
	Err      error
	Model    string
	Provider string
}

func (e *AttemptStampedError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *AttemptStampedError) Unwrap() error {
	return e.Err
}

func (e *AttemptStampedError) AttemptedSlot() (string, string) {
	return e.Model, e.Provider
}

// AttachAttemptedSlot records the model and provider of the active attempt on the error.
func AttachAttemptedSlot(err error, model, provider string) error {
	if err == nil {
		return nil
	}
	if model == "" && provider == "" {
		return err
	}
	if ce, ok := err.(ClassifiedError); ok {
		if model != "" {
			ce.Model = model
		}
		if provider != "" {
			ce.Provider = provider
		}
		return ce
	}
	return &AttemptStampedError{
		Err:      err,
		Model:    model,
		Provider: provider,
	}
}

// ReadAttemptedSlot extracts the attempted model and provider stamped on the error, if any.
func ReadAttemptedSlot(err error) (model string, provider string) {
	if err == nil {
		return "", ""
	}
	var asp AttemptedSlotProvider
	if errors.As(err, &asp) {
		return asp.AttemptedSlot()
	}
	var ce ClassifiedError
	if errors.As(err, &ce) {
		return ce.Model, ce.Provider
	}
	return "", ""
}

// ResolvedModel returns the model that actually ran on the failed attempt, or falls back to the configured slot model.
func ResolvedModel(slot *byok.NormalizedModel, err error) string {
	if m, _ := ReadAttemptedSlot(err); m != "" {
		return m
	}
	if slot != nil && slot.Model != "" {
		return slot.Model
	}
	return ""
}

// ResolvedProvider returns the provider that actually answered the failed attempt, or falls back to the configured slot provider.
func ResolvedProvider(slot *byok.NormalizedModel, err error) string {
	if _, p := ReadAttemptedSlot(err); p != "" {
		return p
	}
	if slot != nil && slot.Provider != "" {
		return string(slot.Provider)
	}
	return ""
}

// ClassifiedError standardizes provider error responses across OpenAI, Anthropic, Gemini, Vertex, Novita, etc.
type ClassifiedError struct {
	Category        ErrorCategory `json:"category"`
	Provider        string        `json:"provider,omitempty"`
	Model           string        `json:"model,omitempty"`
	IsTerminal      bool          `json:"is_terminal"`
	IsTransient     bool          `json:"is_transient"`
	HTTPStatus      int           `json:"http_status,omitempty"`
	FriendlyMessage string        `json:"friendly_message"`
	RawMessage      string        `json:"raw_message"`
	ProviderMessage string        `json:"provider_message,omitempty"`
}

func (e ClassifiedError) AttemptedSlot() (string, string) {
	return e.Model, e.Provider
}

func (e ClassifiedError) BuildReviewErrorMessage() string {
	return BuildReviewErrorMessage(ReviewErrorDiagnostics{
		FriendlyMessage: e.FriendlyMessage,
		Provider:        e.Provider,
		Model:           e.Model,
		HTTPStatus:      e.HTTPStatus,
		ProviderMessage: e.ProviderMessage,
	})
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
	idx = strings.Index(msg, "status ")
	if idx != -1 && len(msg) >= idx+10 {
		statusStr := msg[idx+7 : idx+10]
		var status int
		if n, _ := fmt.Sscanf(statusStr, "%d", &status); n == 1 && status >= 100 && status <= 599 {
			return status
		}
	}
	return 0
}

var abortOrTimeoutRegex = regexp.MustCompile(`(?i)(\[HARD-TIMEOUT\]|context canceled|context deadline exceeded|timed?\s*out|timeout|aborted)`)

// IsAbortOrHardTimeout returns true if the error represents a timeout or abort rather than network instability.
func IsAbortOrHardTimeout(err error) bool {
	if err == nil {
		return false
	}
	return abortOrTimeoutRegex.MatchString(err.Error())
}

// IsTerminalCategory returns true for categories where retrying without admin/user intervention is pointless.
func IsTerminalCategory(category ErrorCategory) bool {
	return category == CategoryAuthInvalid ||
		category == CategoryQuotaExceeded ||
		category == CategoryModelNotFound ||
		category == CategoryModelAccessDenied ||
		category == CategoryUnsupportedParam
}

func looksLikeQuota(lower string) bool {
	return strings.Contains(lower, "quota") ||
		strings.Contains(lower, "credit") ||
		strings.Contains(lower, "billing") ||
		strings.Contains(lower, "payment") ||
		strings.Contains(lower, "balance") ||
		strings.Contains(lower, "suspended") ||
		strings.Contains(lower, "spending limit")
}

func looksLikeModelAccessDenied(lower string) bool {
	return strings.Contains(lower, "does not have access") ||
		strings.Contains(lower, "doesn't have access") ||
		(strings.Contains(lower, "publisher model") && strings.Contains(lower, "not found")) ||
		strings.Contains(lower, "not been allowlisted") ||
		strings.Contains(lower, "enable the model") ||
		strings.Contains(lower, "model garden")
}

// ClassifyLLMError analyzes raw provider messages and HTTP status codes to determine retry and recovery actions.
func ClassifyLLMError(err error, httpStatus int, provider ...string) (res ClassifiedError) {
	attemptedModel, attemptedProvider := ReadAttemptedSlot(err)
	prov := ""
	if len(provider) > 0 && provider[0] != "" {
		prov = provider[0]
	} else if attemptedProvider != "" {
		prov = attemptedProvider
	}
	model := attemptedModel
	providerMsg := ExtractProviderMessage(err)

	defer func() {
		if res.Provider == "" {
			res.Provider = prov
		}
		if res.Model == "" {
			res.Model = model
		}
		if res.ProviderMessage == "" {
			res.ProviderMessage = providerMsg
		}
	}()

	if httpStatus == 0 {
		httpStatus = extractStatusFromError(err)
	}

	raw := ""
	if err != nil {
		raw = err.Error()
	}
	lower := strings.ToLower(raw)

	if prov == "" {
		switch {
		case strings.Contains(lower, "openrouter") || strings.Contains(lower, "open_router"):
			prov = "open_router"
		case strings.Contains(lower, "anthropic"):
			prov = "anthropic"
		case strings.Contains(lower, "openai"):
			prov = "openai"
		case strings.Contains(lower, "gemini"):
			prov = "gemini"
		case strings.Contains(lower, "vertex"):
			prov = "vertex"
		case strings.Contains(lower, "novita"):
			prov = "novita"
		case strings.Contains(lower, "moonshot"):
			prov = "moonshot"
		case strings.Contains(lower, "bedrock"):
			prov = "bedrock"
		case strings.Contains(lower, "azure"):
			prov = "azure"
		case strings.Contains(lower, "zai"):
			prov = "zai"
		}
	}

	// 1. Abort / hard timeout check
	if IsAbortOrHardTimeout(err) {
		return ClassifiedError{
			Category:        CategoryAbortTimeout,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Request timed out or was cancelled before completing.",
			RawMessage:      raw,
		}
	}

	// 2. Context Overflow (Token budget exceeded)
	if strings.Contains(lower, "context_length_exceeded") ||
		strings.Contains(lower, "maximum context length") ||
		strings.Contains(lower, "prompt is too long") ||
		strings.Contains(lower, "too many tokens") ||
		strings.Contains(lower, "token limit exceeded") {
		return ClassifiedError{
			Category:        CategoryContextOverflow,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "The review diff exceeds the maximum context size accepted by the model. Switch to a recommended model (e.g. Claude Sonnet 4.5, Gemini 2.5 Pro) or split the PR.",
			RawMessage:      raw,
		}
	}

	// 3. HTTP Status Code Mapping
	switch httpStatus {
	case http.StatusUnauthorized:
		return ClassifiedError{
			Category:        CategoryAuthInvalid,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Invalid or expired AI provider API key. Check the key in your ScanDrix BYOK settings.",
			RawMessage:      raw,
		}
	case http.StatusPaymentRequired:
		return ClassifiedError{
			Category:        CategoryQuotaExceeded,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "The configured API key is out of credits or has reached its billing limit. Top up the provider account or adjust the plan.",
			RawMessage:      raw,
		}
	case http.StatusTooManyRequests:
		if looksLikeQuota(lower) {
			return ClassifiedError{
				Category:        CategoryQuotaExceeded,
				IsTerminal:      true,
				IsTransient:     false,
				HTTPStatus:      httpStatus,
				FriendlyMessage: "Provider quota limit exceeded. Top up your provider balance.",
				RawMessage:      raw,
			}
		}
		return ClassifiedError{
			Category:        CategoryRateLimit,
			IsTerminal:      false,
			IsTransient:     true,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Rate limit reached on the provider. Backing off and retrying automatically.",
			RawMessage:      raw,
		}
	case http.StatusNotFound:
		if looksLikeModelAccessDenied(lower) {
			return ClassifiedError{
				Category:        CategoryModelAccessDenied,
				IsTerminal:      true,
				IsTransient:     false,
				HTTPStatus:      httpStatus,
				FriendlyMessage: "Your cloud project doesn't have access to this model on Vertex AI. Enable it in the project's Vertex AI Model Garden.",
				RawMessage:      raw,
			}
		}
		return ClassifiedError{
			Category:        CategoryModelNotFound,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "The configured model is not available on the provider. Verify the model name in your ScanDrix settings.",
			RawMessage:      raw,
		}
	case http.StatusForbidden:
		if looksLikeModelAccessDenied(lower) {
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
			FriendlyMessage: "Transient error reaching the provider. Retrying automatically.",
			RawMessage:      raw,
		}
	}

	// 4. Fallback Substring Matching
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
		if looksLikeQuota(lower) {
			return ClassifiedError{
				Category:        CategoryQuotaExceeded,
				IsTerminal:      true,
				IsTransient:     false,
				HTTPStatus:      httpStatus,
				FriendlyMessage: "Provider account balance depleted or quota limit reached.",
				RawMessage:      raw,
			}
		}
		return ClassifiedError{
			Category:        CategoryRateLimit,
			IsTerminal:      false,
			IsTransient:     true,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Rate limit reached. Retrying with backoff.",
			RawMessage:      raw,
		}
	case looksLikeQuota(lower):
		return ClassifiedError{
			Category:        CategoryQuotaExceeded,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "AI provider balance or quota depleted.",
			RawMessage:      raw,
		}
	case looksLikeModelAccessDenied(lower):
		return ClassifiedError{
			Category:        CategoryModelAccessDenied,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Account or cloud project does not have access to the specified model.",
			RawMessage:      raw,
		}
	case strings.Contains(lower, "model_not_found") || strings.Contains(lower, "model not found") || strings.Contains(lower, "does not exist"):
		return ClassifiedError{
			Category:        CategoryModelNotFound,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Specified AI model ID was not found on the provider.",
			RawMessage:      raw,
		}
	case strings.Contains(lower, "temperature") && (strings.Contains(lower, "unsupported") || strings.Contains(lower, "not supported")):
		return ClassifiedError{
			Category:        CategoryUnsupportedParam,
			IsTerminal:      true,
			IsTransient:     false,
			HTTPStatus:      httpStatus,
			FriendlyMessage: "Configured model does not support the requested temperature parameter.",
			RawMessage:      raw,
		}
	case strings.Contains(lower, "econnreset") || strings.Contains(lower, "etimedout") || strings.Contains(lower, "socket hang up") || strings.Contains(lower, "network error"):
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
		// Terminal errors on specific model credentials allow cascading to backup providers
		return true
	case CategoryRateLimit:
		return false
	case CategoryContextOverflow, CategoryContentFilterBlocked, CategoryAbortTimeout:
		return false
	default:
		return false
	}
}

// IsContextOverflowError returns true when an error indicates that the prompt exceeded the context window.
func IsContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	return ClassifyLLMError(err, 0).Category == CategoryContextOverflow
}

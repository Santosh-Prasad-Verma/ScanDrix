// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package llm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"
)

// MaxProviderMessage is the maximum rune length of a provider sentence published to PR comments.
const MaxProviderMessage = 400

var (
	// Bearer / Basic authorization headers echoed back in an error body.
	bearerPattern = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[\w\-._~+/]+=*`)

	// Provider key prefixes: OpenAI, OpenRouter, Anthropic (sk-ant-), Google (AIza), AWS access keys, GitHub tokens.
	skPattern   = regexp.MustCompile(`\bsk-[A-Za-z0-9\-_]{8,}`)
	aizaPattern = regexp.MustCompile(`\bAIza[A-Za-z0-9\-_]{10,}`)
	awsPattern  = regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{12,}`)
	ghPattern   = regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}`)

	// A key named in a JSON body or query parameter: {"api_key":"..."} / "authorization": "..."
	namedKeyPattern = regexp.MustCompile(`(?i)("?(?:api[_-]?key|authorization|access[_-]?token|secret)"?\s*[:=]\s*"?)[^"\s,}]+`)

	whitespaceRegex = regexp.MustCompile(`\s+`)
	jsonPairRegex   = regexp.MustCompile(`"\s*:`)
)

// RedactSecrets strips credential shapes from text destined for public PR comments.
// It preserves field names in JSON structures (e.g. "api_key":"[redacted]") while eliminating values.
func RedactSecrets(text string) string {
	out := text
	// 1. Bearer / Basic headers
	out = bearerPattern.ReplaceAllString(out, "[redacted]")
	// 2. OpenAI / OpenRouter / Anthropic keys
	out = skPattern.ReplaceAllString(out, "[redacted]")
	// 3. Google AIza keys
	out = aizaPattern.ReplaceAllString(out, "[redacted]")
	// 4. AWS access keys
	out = awsPattern.ReplaceAllString(out, "[redacted]")
	// 5. GitHub personal access tokens
	out = ghPattern.ReplaceAllString(out, "[redacted]")
	// 6. Named key / secret fields in JSON payloads or query strings
	out = namedKeyPattern.ReplaceAllStringFunc(out, func(match string) string {
		submatches := namedKeyPattern.FindStringSubmatch(match)
		if len(submatches) > 1 {
			return submatches[1] + "[redacted]"
		}
		return "[redacted]"
	})
	return out
}

// tidy collapses whitespace and caps text at MaxProviderMessage runes with an ellipsis.
func tidy(text string) string {
	collapsed := strings.TrimSpace(whitespaceRegex.ReplaceAllString(text, " "))
	runes := []rune(collapsed)
	if len(runes) > MaxProviderMessage {
		trimmed := strings.TrimRight(string(runes[:MaxProviderMessage]), " ")
		return trimmed + "…"
	}
	return collapsed
}

// messageIn performs depth-limited search (up to 4 levels) inside a parsed JSON object for error messages.
func messageIn(val any, depth int) string {
	if val == nil || depth > 4 {
		return ""
	}
	switch v := val.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if len(trimmed) > 0 {
			return trimmed
		}
		return ""
	case map[string]any:
		if m, ok := v["message"]; ok {
			if found := messageIn(m, depth+1); found != "" {
				return found
			}
		}
		if e, ok := v["error"]; ok {
			if found := messageIn(e, depth+1); found != "" {
				return found
			}
		}
		if d, ok := v["detail"]; ok {
			if found := messageIn(d, depth+1); found != "" {
				return found
			}
		}
		if dat, ok := v["data"]; ok {
			if found := messageIn(dat, depth+1); found != "" {
				return found
			}
		}
	}
	return ""
}

// parsedJSON parses JSON string into any, or returns nil if invalid.
func parsedJSON(text string) any {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		return nil
	}
	var res any
	if err := json.Unmarshal([]byte(text), &res); err == nil {
		return res
	}
	return nil
}

// looksLikeProse verifies if a raw response body is safe to publish as explanatory prose in a public comment.
// It drops payloads, code, HTML, stack traces, and multi-line dumps.
func looksLikeProse(text string) bool {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) == 0 || len(trimmed) > 200 {
		return false
	}
	// Multi-backticks indicate code blocks or snippets rather than inline identifier quotes
	if strings.Contains(trimmed, "```") {
		return false
	}
	// Structural characters indicate payload, HTML, or code rather than plain human-readable sentences
	if strings.ContainsAny(trimmed, "{}[]<>") {
		return false
	}
	// A quoted key/value pair indicates a JSON fragment
	if jsonPairRegex.MatchString(trimmed) {
		return false
	}
	// Multi-line output indicates a stack trace or log dump
	lines := strings.Split(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\n")
	if len(lines) > 2 {
		return false
	}
	return true
}

func isStandardHTTPStatus(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	for code := 400; code <= 599; code++ {
		statusText := strings.ToLower(http.StatusText(code))
		if statusText != "" && (lower == statusText || lower == fmt.Sprintf("%d %s", code, statusText)) {
			return true
		}
	}
	return false
}

// ExtractProviderMessage retrieves the provider's own sentence from an error or response object.
// Returns an empty string if the provider gave no actionable message beyond standard HTTP statuses.
func ExtractProviderMessage(err any) string {
	if err == nil {
		return ""
	}

	var structuredBodies []any
	var rawBodies []string

	// Case 1: map[string]any representation (common in testing and generic adapters)
	if m, ok := err.(map[string]any); ok {
		if d, ok := m["data"]; ok {
			structuredBodies = append(structuredBodies, d)
		}
		if rb, ok := m["responseBody"].(string); ok {
			if p := parsedJSON(rb); p != nil {
				structuredBodies = append(structuredBodies, p)
			}
			rawBodies = append(rawBodies, rb)
		}
		if b, ok := m["body"]; ok {
			if bs, ok := b.(string); ok {
				if p := parsedJSON(bs); p != nil {
					structuredBodies = append(structuredBodies, p)
				}
				rawBodies = append(rawBodies, bs)
			} else {
				structuredBodies = append(structuredBodies, b)
			}
		}
	} else if errVal := reflect.ValueOf(err); errVal.IsValid() && (errVal.Kind() == reflect.Struct || (errVal.Kind() == reflect.Pointer && errVal.Elem().Kind() == reflect.Struct)) {
		// Case 2: struct with Data, ResponseBody, or Body fields
		elem := errVal
		if elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}

		if f := elem.FieldByName("Data"); f.IsValid() && f.CanInterface() {
			structuredBodies = append(structuredBodies, f.Interface())
		}
		if f := elem.FieldByName("ResponseBody"); f.IsValid() && f.Kind() == reflect.String {
			rb := f.String()
			if p := parsedJSON(rb); p != nil {
				structuredBodies = append(structuredBodies, p)
			}
			rawBodies = append(rawBodies, rb)
		}
		if f := elem.FieldByName("Body"); f.IsValid() {
			if f.Kind() == reflect.String {
				b := f.String()
				if p := parsedJSON(b); p != nil {
					structuredBodies = append(structuredBodies, p)
				}
				rawBodies = append(rawBodies, b)
			} else if f.CanInterface() {
				structuredBodies = append(structuredBodies, f.Interface())
			}
		}
	}

	// Case 3: Error interface or string with embedded JSON / provider sentence
	if e, ok := err.(error); ok {
		errMsg := e.Error()
		start := strings.Index(errMsg, "{")
		end := strings.LastIndex(errMsg, "}")
		if start != -1 && end > start {
			jsonSub := errMsg[start : end+1]
			if p := parsedJSON(jsonSub); p != nil {
				structuredBodies = append(structuredBodies, p)
			}
		}

		// Also check if there is a provider suffix after status: e.g. "status 404): ..."
		if idx := strings.LastIndex(errMsg, "): "); idx != -1 {
			rawBodies = append(rawBodies, errMsg[idx+3:])
		} else if idx := strings.LastIndex(errMsg, ": "); idx != -1 {
			rawBodies = append(rawBodies, errMsg[idx+2:])
		}
	}

	// 1. Prioritize structured JSON message (labelled as message/error/detail by provider)
	for _, body := range structuredBodies {
		if body == nil {
			continue
		}
		if found := messageIn(body, 0); found != "" {
			return tidy(RedactSecrets(found))
		}
	}

	// 2. Fall back to plain-text body if and only if it qualifies as prose
	for _, raw := range rawBodies {
		trimmed := strings.TrimSpace(raw)
		if trimmed != "" && looksLikeProse(trimmed) && !isStandardHTTPStatus(trimmed) {
			return tidy(RedactSecrets(trimmed))
		}
	}

	return ""
}

// ReviewErrorDiagnostics encapsulates facts and explanations surrounding a review execution failure.
type ReviewErrorDiagnostics struct {
	// FriendlyMessage is the high-level classified action item (always present).
	FriendlyMessage string `json:"friendly_message"`
	// Provider is the AI provider that handled the failing attempt.
	Provider string `json:"provider,omitempty"`
	// Model is the model ID that actually executed on the failed attempt.
	Model string `json:"model,omitempty"`
	// HTTPStatus is the HTTP status code returned by the provider, if any.
	HTTPStatus int `json:"http_status,omitempty"`
	// ProviderMessage is the redacted, capped sentence returned directly by the provider.
	ProviderMessage string `json:"provider_message,omitempty"`
	// AgentName specifies the failing agent when failure is attributable to one.
	AgentName string `json:"agent_name,omitempty"`
}

// BuildReviewErrorMessage formats user-facing diagnostics for PR comments and summaries.
// Format:
//
//	<Friendly Message>
//
//	<provider> · <model> · HTTP <status> · <agent>
//
//	Provider said: <Provider Message>
func BuildReviewErrorMessage(diag ReviewErrorDiagnostics) string {
	friendly := strings.TrimSpace(RedactSecrets(diag.FriendlyMessage))
	provMsg := strings.TrimSpace(RedactSecrets(diag.ProviderMessage))

	var facts []string
	if diag.Provider != "" {
		facts = append(facts, diag.Provider)
	}
	if diag.Model != "" {
		facts = append(facts, diag.Model)
	}
	if diag.HTTPStatus > 0 {
		facts = append(facts, fmt.Sprintf("HTTP %d", diag.HTTPStatus))
	}
	if diag.AgentName != "" {
		facts = append(facts, diag.AgentName)
	}

	var lines []string
	if friendly != "" {
		lines = append(lines, friendly)
	}
	if len(facts) > 0 {
		lines = append(lines, strings.Join(facts, " · "))
	}
	if provMsg != "" && !strings.Contains(friendly, provMsg) {
		lines = append(lines, fmt.Sprintf("Provider said: %s", provMsg))
	}

	return strings.Join(lines, "\n\n")
}

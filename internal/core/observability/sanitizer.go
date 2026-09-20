package observability

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
)

// RedactionStyle defines how sensitive text is masked.
type RedactionStyle string

const (
	RedactionStyleFull    RedactionStyle = "full"
	RedactionStylePartial RedactionStyle = "partial"
	DefaultRedactionToken                = "[REDACTED]"
)

var (
	jwtRegex       = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)
	aiKeyRegex     = regexp.MustCompile(`(?:sk-[a-zA-Z0-9_-]{20,}|anthropic-key-[a-zA-Z0-9_-]{20,}|ghp_[a-zA-Z0-9]{36}|glpat-[a-zA-Z0-9_-]{20,})`)
	bearerRegex    = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9_\-\.]{15,}`)
	scandrixKeyReg = regexp.MustCompile(`scandrix_live_[a-zA-Z0-9]{20,}`)
)

// SanitizationConfig configures the SanitizationProcessor.
type SanitizationConfig struct {
	SensitiveKeys  []string
	RedactionToken string
	RedactionStyle RedactionStyle
	MaxDepth       int
}

// SanitizationProcessor recursively scrubs sensitive tokens, keys, and PII from telemetry spans.
type SanitizationProcessor struct {
	sensitiveKeys  map[string]struct{}
	redactionToken string
	redactionStyle RedactionStyle
	maxDepth       int
	nextProcessor  SpanProcessor
	mu             sync.RWMutex
}

// NewSanitizationProcessor instantiates a processor with enterprise-default sensitive key rules.
func NewSanitizationProcessor(cfg ...func(*SanitizationConfig)) *SanitizationProcessor {
	c := SanitizationConfig{
		RedactionToken: DefaultRedactionToken,
		RedactionStyle: RedactionStyleFull,
		MaxDepth:       16,
	}
	for _, fn := range cfg {
		fn(&c)
	}

	keys := []string{
		"password", "token", "apikey", "api_key", "secret", "authorization",
		"bearer", "creditcard", "cvv", "ssn", "clientsecret", "client_secret",
		"privatekey", "private_key", "refresh", "refresh_token", "auth",
		"bearertoken", "jwt", "credential", "cookie", "set-cookie",
		"x-api-key", "x-team-key", "webhook_secret", "encryption_key",
	}
	keys = append(keys, c.SensitiveKeys...)

	keyMap := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		keyMap[normalizeKey(k)] = struct{}{}
	}

	return &SanitizationProcessor{
		sensitiveKeys:  keyMap,
		redactionToken: c.RedactionToken,
		redactionStyle: c.RedactionStyle,
		maxDepth:       c.MaxDepth,
	}
}

// SetNext binds a downstream processor to receive sanitized spans.
func (p *SanitizationProcessor) SetNext(next SpanProcessor) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nextProcessor = next
}

func (p *SanitizationProcessor) OnStart(parent context.Context, span Span) {
	p.mu.RLock()
	next := p.nextProcessor
	p.mu.RUnlock()
	if next != nil {
		next.OnStart(parent, span)
	}
}

// OnEnd sanitizes the span data in-place before forwarding.
func (p *SanitizationProcessor) OnEnd(span *SpanData) {
	if span == nil {
		return
	}

	// Sanitize attributes
	span.Attributes = p.SanitizeMap(span.Attributes)

	// Sanitize events
	for i := range span.Events {
		span.Events[i].Attributes = p.SanitizeMap(span.Events[i].Attributes)
	}

	// Sanitize links
	for i := range span.Links {
		span.Links[i].Attributes = p.SanitizeMap(span.Links[i].Attributes)
	}

	// Sanitize status description if it leaked credentials
	if span.Status.Description != "" {
		span.Status.Description = p.SanitizeString(span.Status.Description)
	}

	p.mu.RLock()
	next := p.nextProcessor
	p.mu.RUnlock()
	if next != nil {
		next.OnEnd(span)
	}
}

func (p *SanitizationProcessor) ForceFlush(ctx context.Context) error {
	p.mu.RLock()
	next := p.nextProcessor
	p.mu.RUnlock()
	if next != nil {
		return next.ForceFlush(ctx)
	}
	return nil
}

func (p *SanitizationProcessor) Shutdown(ctx context.Context) error {
	p.mu.RLock()
	next := p.nextProcessor
	p.mu.RUnlock()
	if next != nil {
		return next.Shutdown(ctx)
	}
	return nil
}

// SanitizeMap recursively scrubs a map of attributes.
func (p *SanitizationProcessor) SanitizeMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	result := make(map[string]any, len(input))
	for k, v := range input {
		norm := normalizeKey(k)
		if p.isSensitive(norm) {
			result[k] = p.redactValue(v)
		} else {
			result[k] = p.sanitizeValue(v, 0)
		}
	}
	return result
}

// SanitizeString scrubs regex-matched tokens from raw strings or JSON strings.
func (p *SanitizationProcessor) SanitizeString(s string) string {
	if s == "" {
		return ""
	}
	trimmed := strings.TrimSpace(s)
	// Check if this is an embedded JSON object or array
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		var decoded any
		if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
			sanitized := p.sanitizeValue(decoded, 0)
			if outBytes, err := json.Marshal(sanitized); err == nil {
				return string(outBytes)
			}
		}
	}

	// Regex-based redaction
	s = jwtRegex.ReplaceAllString(s, p.redactionToken)
	s = aiKeyRegex.ReplaceAllString(s, p.redactionToken)
	s = bearerRegex.ReplaceAllString(s, "Bearer "+p.redactionToken)
	s = scandrixKeyReg.ReplaceAllString(s, p.redactionToken)
	return s
}

// SanitizeAny scrubs arbitrary data types.
func (p *SanitizationProcessor) SanitizeAny(val any) any {
	return p.sanitizeValue(val, 0)
}

func (p *SanitizationProcessor) sanitizeValue(val any, depth int) any {
	if val == nil || depth > p.maxDepth {
		return val
	}

	switch v := val.(type) {
	case string:
		return p.SanitizeString(v)
	case map[string]any:
		return p.SanitizeMap(v)
	case []any:
		arr := make([]any, len(v))
		for i, item := range v {
			arr[i] = p.sanitizeValue(item, depth+1)
		}
		return arr
	default:
		return val
	}
}

func (p *SanitizationProcessor) isSensitive(normalizedKey string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, found := p.sensitiveKeys[normalizedKey]
	return found
}

func (p *SanitizationProcessor) redactValue(v any) any {
	if p.redactionStyle == RedactionStyleFull {
		return p.redactionToken
	}

	str, ok := v.(string)
	if !ok {
		return p.redactionToken
	}

	if len(str) <= 8 {
		return p.redactionToken
	}
	return str[:3] + "..." + str[len(str)-3:]
}

func normalizeKey(key string) string {
	lower := strings.ToLower(key)
	var b strings.Builder
	for i := 0; i < len(lower); i++ {
		c := lower[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteByte(c)
		}
	}
	return b.String()
}

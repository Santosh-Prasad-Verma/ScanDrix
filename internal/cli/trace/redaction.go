// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"encoding/json"
	"reflect"
	"regexp"
)

const RedactionPlaceholder = "[REDACTED]"

type secretPattern struct {
	name    string
	pattern *regexp.Regexp
	group   int // 0 means replace full match, >0 means replace only capture group
}

// secretPatterns defines regex filters for enterprise secrets ordered most-specific first.
var secretPatterns = []secretPattern{
	{
		name:    "pem-private-key",
		pattern: regexp.MustCompile(`(?s)-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY-----.*?-----END (?:RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY-----`),
		group:   0,
	},
	{
		name:    "anthropic",
		pattern: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{8,}`),
		group:   0,
	},
	{
		name:    "openai-project",
		pattern: regexp.MustCompile(`\bsk-proj-[A-Za-z0-9_-]{8,}`),
		group:   0,
	},
	{
		name:    "openai",
		pattern: regexp.MustCompile(`\bsk-[A-Za-z0-9]{16,}`),
		group:   0,
	},
	{
		name:    "github-pat",
		pattern: regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`),
		group:   0,
	},
	{
		name:    "github-token",
		pattern: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{16,}`),
		group:   0,
	},
	{
		name:    "gitlab-token",
		pattern: regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{16,}`),
		group:   0,
	},
	{
		name:    "slack-token",
		pattern: regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`),
		group:   0,
	},
	{
		name:    "google-api-key",
		pattern: regexp.MustCompile(`\bAIza[A-Za-z0-9_-]{30,}`),
		group:   0,
	},
	{
		name:    "aws-access-key-id",
		pattern: regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),
		group:   0,
	},
	{
		name:    "stripe",
		pattern: regexp.MustCompile(`\b[rs]k_(?:live|test)_[A-Za-z0-9]{16,}`),
		group:   0,
	},
	{
		name:    "npm-token",
		pattern: regexp.MustCompile(`\bnpm_[A-Za-z0-9]{30,}`),
		group:   0,
	},
	{
		name:    "jwt",
		pattern: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`),
		group:   0,
	},
	{
		name:    "authorization-header",
		pattern: regexp.MustCompile(`(?i)\b(?:Bearer|Basic|Token)\s+([A-Za-z0-9._~+/=-]{12,})`),
		group:   1,
	},
	{
		name:    "url-credentials",
		pattern: regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://[^\s/@:]+:)([^\s/@]{3,})@`),
		group:   2,
	},
	{
		name:    "secret-assignment",
		pattern: regexp.MustCompile(`(?i)\b([A-Za-z0-9_.-]*(?:secret|token|passwd|password|api[_-]?key|apikey|access[_-]?key|private[_-]?key|client[_-]?secret|auth)[A-Za-z0-9_.-]*)\s*[:=]\s*["'` + "`" + `]?([^\s"'` + "`" + `,;]{6,})["'` + "`" + `]?`),
		group:   2,
	},
}

// Redact strips credentials out of free text. It is idempotent and safe to call on already redacted text.
func Redact(value string) string {
	if value == "" {
		return ""
	}

	result := value
	for _, sp := range secretPatterns {
		if sp.group == 0 {
			result = sp.pattern.ReplaceAllString(result, RedactionPlaceholder)
		} else {
			result = replaceCaptureGroup(result, sp.pattern, sp.group)
		}
	}
	return result
}

func replaceCaptureGroup(input string, pattern *regexp.Regexp, group int) string {
	matches := pattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return input
	}

	var output []byte
	lastIdx := 0

	for _, m := range matches {
		if group*2+1 >= len(m) {
			continue
		}
		start := m[group*2]
		end := m[group*2+1]
		if start < 0 || end < 0 {
			continue
		}

		// Don't re-redact if already redacted
		matchVal := input[start:end]
		if matchVal == RedactionPlaceholder {
			continue
		}

		output = append(output, input[lastIdx:start]...)
		output = append(output, RedactionPlaceholder...)
		lastIdx = end
	}

	if lastIdx < len(input) {
		output = append(output, input[lastIdx:]...)
	}

	return string(output)
}

// RedactDeep recursively scrubs string fields in JSON-compatible nested maps, slices, and structs.
func RedactDeep(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case string:
		return Redact(val)
	case []any:
		res := make([]any, len(val))
		for i, item := range val {
			res[i] = RedactDeep(item)
		}
		return res
	case map[string]any:
		res := make(map[string]any, len(val))
		for k, item := range val {
			res[k] = RedactDeep(item)
		}
		return res
	default:
		valType := reflect.TypeOf(v)
		kind := valType.Kind()
		if kind == reflect.Struct || kind == reflect.Pointer || kind == reflect.Map || kind == reflect.Slice {
			data, err := json.Marshal(v)
			if err == nil {
				var generic any
				if err := json.Unmarshal(data, &generic); err == nil {
					return RedactDeep(generic)
				}
			}
		}
		return v
	}
}

// ContainsSecret returns true if any pattern matches the string.
func ContainsSecret(value string) bool {
	for _, sp := range secretPatterns {
		if sp.pattern.MatchString(value) {
			return true
		}
	}
	return false
}

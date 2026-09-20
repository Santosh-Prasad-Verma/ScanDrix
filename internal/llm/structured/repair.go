// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package structured

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	fenceRegex         = regexp.MustCompile("(?i)```(?:json)?\\s*([\\s\\S]*?)```")
	trailingCommaRegex = regexp.MustCompile(`,(\s*[}\]])`)
)

// SliceBalancedJSON slices out the outermost balanced { ... } or [ ... ],
// string-aware so braces inside string literals don't skew the depth count.
func SliceBalancedJSON(s string) string {
	start := -1
	var openChar, closeChar byte

	for i := 0; i < len(s); i++ {
		if s[i] == '{' {
			start = i
			openChar = '{'
			closeChar = '}'
			break
		} else if s[i] == '[' {
			start = i
			openChar = '['
			closeChar = ']'
			break
		}
	}

	if start < 0 {
		return ""
	}

	depth := 0
	inStr := false
	escaped := false

	for i := start; i < len(s); i++ {
		ch := s[i]
		if inStr {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inStr = false
			}
			continue
		}

		if ch == '"' {
			inStr = true
		} else if ch == openChar {
			depth++
		} else if ch == closeChar {
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}

	return ""
}

// ExtractJSONFromText pulls the JSON value out of free-form text:
// unwraps markdown code fences, drops prose wrappers, and removes trailing commas.
func ExtractJSONFromText(text string) string {
	s := strings.TrimSpace(text)
	if s == "" {
		return ""
	}

	// 1. Unwrap markdown code fence
	if match := fenceRegex.FindStringSubmatch(s); len(match) > 1 {
		s = strings.TrimSpace(match[1])
	}

	// 2. Drop prose before and after outermost balanced JSON
	if balanced := SliceBalancedJSON(s); balanced != "" {
		s = balanced
	}

	// 3. Remove trailing commas before } or ]
	s = trailingCommaRegex.ReplaceAllString(s, "$1")

	// Ensure it starts with valid JSON delimiter
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return s
	}

	return ""
}

// RepairJSONText performs deterministic, model-free repair of "almost JSON".
// Returns the cleaned string ONLY when it differs from input and parses as valid JSON.
func RepairJSONText(text string) string {
	candidate := ExtractJSONFromText(text)
	if candidate == "" || candidate == text {
		return ""
	}

	var dummy any
	if err := json.Unmarshal([]byte(candidate), &dummy); err != nil {
		return ""
	}

	return candidate
}

// SalvageStructuredError attempts deterministic extraction and parsing into the target struct.
func SalvageStructuredError[T any](rawText string, target *T) bool {
	candidate := ExtractJSONFromText(rawText)
	if candidate == "" {
		return false
	}

	if err := json.Unmarshal([]byte(candidate), target); err != nil {
		return false
	}

	return true
}

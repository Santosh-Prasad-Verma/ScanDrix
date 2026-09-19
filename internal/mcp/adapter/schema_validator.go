// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Tools Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package adapter

import (
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"regexp"
	"time"

	"github.com/google/uuid"
)

var (
	timeFormatRegex = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d):([0-5]\d)$`)
	ipv4Regex       = regexp.MustCompile(`^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`)
	ipv6Regex       = regexp.MustCompile(`^(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$`)
)

// ValidateMCPSchema verifies that the schema object contains either a type or properties.
func ValidateMCPSchema(schema any) bool {
	if schema == nil {
		return false
	}
	s, ok := schema.(map[string]any)
	if !ok {
		return false
	}

	hasType := false
	if t, exists := s["type"]; exists {
		if _, isStr := t.(string); isStr {
			hasType = true
		}
	}

	hasProps := false
	if p, exists := s["properties"]; exists {
		if _, isObj := p.(map[string]any); isObj {
			hasProps = true
		}
	}

	return hasType || hasProps
}

// ValidateInputAgainstSchema performs strict JSON-Schema validation on input arguments.
func ValidateInputAgainstSchema(schema map[string]any, input map[string]any) error {
	if schema == nil {
		return nil
	}

	properties, _ := schema["properties"].(map[string]any)
	requiredList, _ := schema["required"].([]any)
	if requiredList == nil {
		if strList, ok := schema["required"].([]string); ok {
			requiredList = make([]any, len(strList))
			for i, v := range strList {
				requiredList[i] = v
			}
		}
	}

	// 1. Check required properties
	for _, req := range requiredList {
		reqKey, ok := req.(string)
		if !ok {
			continue
		}
		if input == nil {
			return fmt.Errorf("missing required property: %s", reqKey)
		}
		val, exists := input[reqKey]
		if !exists || val == nil {
			return fmt.Errorf("missing required property: %s", reqKey)
		}
	}

	if input == nil || properties == nil {
		return nil
	}

	// 2. Validate individual properties
	for key, val := range input {
		propSchemaRaw, exists := properties[key]
		if !exists {
			continue
		}
		propSchema, ok := propSchemaRaw.(map[string]any)
		if !ok {
			continue
		}

		if err := validatePropertyValue(key, propSchema, val); err != nil {
			return err
		}
	}

	return nil
}

func validatePropertyValue(field string, propSchema map[string]any, val any) error {
	if val == nil {
		return nil
	}

	// Check enum
	if enumRaw, ok := propSchema["enum"].([]any); ok && len(enumRaw) > 0 {
		matched := false
		for _, e := range enumRaw {
			if fmt.Sprintf("%v", e) == fmt.Sprintf("%v", val) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("field '%s' has value '%v' which is not in allowed enum: %v", field, val, enumRaw)
		}
	}

	expectedType, _ := propSchema["type"].(string)

	switch expectedType {
	case "string":
		strVal, ok := val.(string)
		if !ok {
			return fmt.Errorf("field '%s' expected string, got %T", field, val)
		}

		if minLen, ok := propSchema["minLength"].(float64); ok && float64(len(strVal)) < minLen {
			return fmt.Errorf("field '%s' length %d is less than minLength %d", field, len(strVal), int(minLen))
		}
		if maxLen, ok := propSchema["maxLength"].(float64); ok && float64(len(strVal)) > maxLen {
			return fmt.Errorf("field '%s' length %d exceeds maxLength %d", field, len(strVal), int(maxLen))
		}
		if pat, ok := propSchema["pattern"].(string); ok && pat != "" {
			matched, err := regexp.MatchString(pat, strVal)
			if err != nil || !matched {
				return fmt.Errorf("field '%s' does not match required pattern %s", field, pat)
			}
		}
		if format, ok := propSchema["format"].(string); ok && format != "" {
			if err := validateFormat(field, format, strVal); err != nil {
				return err
			}
		}

	case "number", "integer":
		var numVal float64
		switch v := val.(type) {
		case float64:
			numVal = v
		case int:
			numVal = float64(v)
		case int64:
			numVal = float64(v)
		default:
			return fmt.Errorf("field '%s' expected number, got %T", field, val)
		}

		if expectedType == "integer" && math.Floor(numVal) != numVal {
			return fmt.Errorf("field '%s' expected integer, got decimal %f", field, numVal)
		}
		if minVal, ok := propSchema["minimum"].(float64); ok && numVal < minVal {
			return fmt.Errorf("field '%s' value %f is less than minimum %f", field, numVal, minVal)
		}
		if maxVal, ok := propSchema["maximum"].(float64); ok && numVal > maxVal {
			return fmt.Errorf("field '%s' value %f exceeds maximum %f", field, numVal, maxVal)
		}
		if multVal, ok := propSchema["multipleOf"].(float64); ok && multVal > 0 {
			if math.Mod(numVal, multVal) != 0 {
				return fmt.Errorf("field '%s' value %f is not a multiple of %f", field, numVal, multVal)
			}
		}

	case "boolean":
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("field '%s' expected boolean, got %T", field, val)
		}

	case "array":
		arr, ok := val.([]any)
		if !ok {
			return fmt.Errorf("field '%s' expected array, got %T", field, val)
		}
		if minItems, ok := propSchema["minItems"].(float64); ok && float64(len(arr)) < minItems {
			return fmt.Errorf("field '%s' has %d items, minimum is %d", field, len(arr), int(minItems))
		}
		if maxItems, ok := propSchema["maxItems"].(float64); ok && float64(len(arr)) > maxItems {
			return fmt.Errorf("field '%s' has %d items, maximum is %d", field, len(arr), int(maxItems))
		}
		if uniq, ok := propSchema["uniqueItems"].(bool); ok && uniq {
			seen := make(map[string]bool)
			for _, item := range arr {
				strKey := fmt.Sprintf("%v", item)
				if seen[strKey] {
					return fmt.Errorf("field '%s' contains duplicate items", field)
				}
				seen[strKey] = true
			}
		}
		if itemsSchema, ok := propSchema["items"].(map[string]any); ok {
			for i, item := range arr {
				itemField := fmt.Sprintf("%s[%d]", field, i)
				if err := validatePropertyValue(itemField, itemsSchema, item); err != nil {
					return err
				}
			}
		}

	case "object":
		obj, ok := val.(map[string]any)
		if !ok {
			return fmt.Errorf("field '%s' expected object, got %T", field, val)
		}
		return ValidateInputAgainstSchema(propSchema, obj)
	}

	return nil
}

func validateFormat(field, format, val string) error {
	switch format {
	case "uri", "uri-reference":
		if _, err := url.ParseRequestURI(val); err != nil {
			return fmt.Errorf("field '%s' is not a valid URI: %s", field, val)
		}
	case "email":
		if _, err := mail.ParseAddress(val); err != nil {
			return fmt.Errorf("field '%s' is not a valid email: %s", field, val)
		}
	case "date-time":
		if _, err := time.Parse(time.RFC3339, val); err != nil {
			return fmt.Errorf("field '%s' is not a valid RFC3339 date-time: %s", field, val)
		}
	case "date":
		if _, err := time.Parse("2006-01-02", val); err != nil {
			return fmt.Errorf("field '%s' is not a valid date (YYYY-MM-DD): %s", field, val)
		}
	case "time":
		if !timeFormatRegex.MatchString(val) {
			return fmt.Errorf("field '%s' is not a valid time (HH:MM:SS): %s", field, val)
		}
	case "uuid":
		if _, err := uuid.Parse(val); err != nil {
			return fmt.Errorf("field '%s' is not a valid UUID: %s", field, val)
		}
	case "ipv4":
		if !ipv4Regex.MatchString(val) {
			return fmt.Errorf("field '%s' is not a valid IPv4 address: %s", field, val)
		}
	case "ipv6":
		if !ipv6Regex.MatchString(val) {
			return fmt.Errorf("field '%s' is not a valid IPv6 address: %s", field, val)
		}
	}
	return nil
}

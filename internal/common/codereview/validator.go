// Package codereview provides validation and version migration for ScanDrix configuration files.
package codereview

import (
	"fmt"
	"strings"
)

// CodeReviewParameter mirrors the database entity for global code review configuration.
type CodeReviewParameter struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	IsSelected   bool           `json:"isSelected"`
	Configs      map[string]any `json:"configs"`
	Repositories []string       `json:"repositories"`
}

// BuildDefaultGlobalCodeReviewConfig creates a pristine global code review parameter for new teams.
func BuildDefaultGlobalCodeReviewConfig() CodeReviewParameter {
	return CodeReviewParameter{
		ID:           "global",
		Name:         "Global",
		IsSelected:   true,
		Configs:      make(map[string]any),
		Repositories: []string{},
	}
}

// ValidationResult represents the output of validating a configuration file.
type ValidationResult struct {
	IsValid       bool     `json:"isValid"`
	ErrorMessages []string `json:"errorMessages,omitempty"`
	IsDeprecated  bool     `json:"isDeprecated"`
}

// ValidateScanDrixConfigFile validates and migrates configuration fields.
func ValidateScanDrixConfigFile(cfg *ScanDrixConfigFile) ValidationResult {
	if cfg == nil {
		return ValidationResult{
			IsValid:       false,
			ErrorMessages: []string{"Configuration file is null or undefined"},
		}
	}

	var errors []string
	isDeprecated := false
	currentVersion := "1.2"

	version := cfg.Version
	if version == "" {
		version = currentVersion
	}

	if version != currentVersion {
		isDeprecated = true
		// Version 1.0/1.1 migrations
		if version == "1.0" || version == "1.1" {
			cfg.AutomaticPRApprovalActive = false
		}
	}

	if cfg.MaxReviewLines <= 0 {
		cfg.MaxReviewLines = 1000
	} else if cfg.MaxReviewLines > 10000 {
		errors = append(errors, "maxReviewLines exceeds maximum allowed threshold of 10,000 lines")
	}

	cadence := strings.ToLower(cfg.ReviewCadence)
	if cadence != "" && cadence != "automatic" && cadence != "manual" && cadence != "autopause" {
		errors = append(errors, fmt.Sprintf("invalid reviewCadence %q: must be automatic, manual, or autopause", cfg.ReviewCadence))
	}

	return ValidationResult{
		IsValid:       len(errors) == 0,
		ErrorMessages: errors,
		IsDeprecated:  isDeprecated,
	}
}

// IsParameterValidInConfigFile checks if a specific parameter key is valid or flagged in error sets.
func IsParameterValidInConfigFile(parameterKey string, errors []string) bool {
	if len(errors) == 0 {
		return true
	}
	for _, err := range errors {
		if strings.Contains(strings.ToLower(err), strings.ToLower(parameterKey)) {
			return false
		}
	}
	return true
}

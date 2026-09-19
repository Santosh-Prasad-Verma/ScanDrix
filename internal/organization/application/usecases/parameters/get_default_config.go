// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package paramusecases

import (
	"os"

	paramdomain "github.com/scandrix/backend/internal/organization/domain/parameters"
	"gopkg.in/yaml.v3"
)

// GetDefaultConfigUseCase produces standard ScanDrix review configurations.
type GetDefaultConfigUseCase struct{}

func NewGetDefaultConfigUseCase() *GetDefaultConfigUseCase {
	return &GetDefaultConfigUseCase{}
}

func (uc *GetDefaultConfigUseCase) DefaultCodeReviewConfig() paramdomain.CodeReviewConfigValue {
	// Attempt to load from default-scandrix-config.yml
	paths := []string{
		"default-scandrix-config.yml",
		"ScanDrix/default-scandrix-config.yml",
		"../default-scandrix-config.yml",
		"../../default-scandrix-config.yml",
	}

	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			var parsed map[string]any
			if err := yaml.Unmarshal(data, &parsed); err == nil {
				cfg := paramdomain.CodeReviewConfigValue{
					MaxFiles:                50,
					IncludedPaths:           []string{"**/*"},
					AutomatedReviewLabels:  []string{"scandrix-review"},
					SeverityThreshold:      "medium",
					EnableInlineSuggestions: true,
					DrixyRulesEnabled:      true,
				}
				if ig, ok := parsed["ignorePaths"].([]any); ok {
					var ignores []string
					for _, item := range ig {
						if s, ok := item.(string); ok {
							ignores = append(ignores, s)
						}
					}
					if len(ignores) > 0 {
						cfg.IgnoredPaths = ignores
					}
				}
				return cfg
			}
		}
	}

	return paramdomain.CodeReviewConfigValue{
		MaxFiles: 50,
		IgnoredPaths: []string{
			"vendor/**",
			"node_modules/**",
			"*.min.js",
			"*.min.css",
			"package-lock.json",
			"pnpm-lock.yaml",
			"go.sum",
		},
		IncludedPaths:           []string{"**/*"},
		AutomatedReviewLabels:  []string{"scandrix-review"},
		SeverityThreshold:      "medium",
		EnableInlineSuggestions: true,
		DrixyRulesEnabled:      true,
	}
}

func (uc *GetDefaultConfigUseCase) DefaultPlatformConfigs() paramdomain.PlatformConfigValue {
	return paramdomain.PlatformConfigValue{
		FinishOnboard:                     true,
		FinishProjectManagementConnection: true,
		DrixyLearningStatus:               paramdomain.DrixyLearningStatusEnabled,
	}
}

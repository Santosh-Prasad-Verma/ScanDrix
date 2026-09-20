// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package configengine

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/domain"
)

// InRepoConfigParser parses and validates repository-embedded configuration files.
type InRepoConfigParser struct{}

// NewInRepoConfigParser constructs a parser for in-repo configuration.
func NewInRepoConfigParser() *InRepoConfigParser {
	return &InRepoConfigParser{}
}

// IsConfigFilePath checks whether a path matches known ScanDrix in-repo configuration paths.
func (p *InRepoConfigParser) IsConfigFilePath(filePath string) bool {
	clean := filepath.ToSlash(filepath.Clean(filePath))
	clean = strings.TrimPrefix(strings.TrimPrefix(clean, "./"), "/")
	base := filepath.Base(clean)

	if base == ".scandrix.yml" || base == ".scandrix.yaml" || base == ".scandrix.json" {
		return true
	}
	if clean == ".scandrix/config.json" || clean == ".scandrix/config.yml" || clean == ".scandrix/config.yaml" {
		return true
	}
	if clean == ".drixy/config.json" || clean == ".drixy/config.yml" || clean == ".drixy/config.yaml" {
		return true
	}
	return false
}

// ParseContent parses raw bytes into an InRepoConfiguration according to file format.
func (p *InRepoConfigParser) ParseContent(filePath string, rawContent []byte) (*InRepoConfiguration, []string, error) {
	if len(rawContent) == 0 {
		return nil, nil, fmt.Errorf("configuration file is empty: %s", filePath)
	}

	var warnings []string
	cfg := &InRepoConfiguration{}

	clean := strings.ToLower(filepath.ToSlash(filePath))
	if strings.HasSuffix(clean, ".json") {
		if err := json.Unmarshal(rawContent, cfg); err != nil {
			return nil, nil, fmt.Errorf("failed to parse JSON config in %s: %w", filePath, err)
		}
	} else if strings.HasSuffix(clean, ".yml") || strings.HasSuffix(clean, ".yaml") {
		if err := yaml.Unmarshal(rawContent, cfg); err != nil {
			return nil, nil, fmt.Errorf("failed to parse YAML config in %s: %w", filePath, err)
		}
	} else {
		// Fallback: try JSON then YAML
		if err := json.Unmarshal(rawContent, cfg); err != nil {
			if yamlErr := yaml.Unmarshal(rawContent, cfg); yamlErr != nil {
				return nil, nil, fmt.Errorf("unrecognized config format in %s: %w", filePath, yamlErr)
			}
		}
	}

	// Semantic Validation & Normalization
	if cfg.ReviewMode != nil {
		mode := ReviewMode(strings.ToLower(string(*cfg.ReviewMode)))
		switch mode {
		case ModeFast, ModeNormal, ModeDeep:
			cfg.ReviewMode = &mode
		default:
			warnings = append(warnings, fmt.Sprintf("invalid review_mode '%s'; defaulting to normal", *cfg.ReviewMode))
			normalized := ModeNormal
			cfg.ReviewMode = &normalized
		}
	}

	if cfg.Strictness != nil {
		str := domain.ModelStrictness(strings.ToUpper(string(*cfg.Strictness)))
		switch str {
		case domain.StrictnessStrict, domain.StrictnessBalanced, domain.StrictnessLenient, domain.StrictnessPermissive:
			cfg.Strictness = &str
		default:
			warnings = append(warnings, fmt.Sprintf("invalid strictness '%s'; defaulting to BALANCED", *cfg.Strictness))
			norm := domain.StrictnessBalanced
			cfg.Strictness = &norm
		}
		if cfg.Sensitivity == nil {
			var s ReviewSensitivity
			switch *cfg.Strictness {
			case domain.StrictnessStrict:
				s = SensitivityStrict
			case domain.StrictnessBalanced:
				s = SensitivityStandard
			case domain.StrictnessLenient, domain.StrictnessPermissive:
				s = SensitivityLenient
			}
			cfg.Sensitivity = &s
		}
	} else if cfg.Sensitivity != nil {
		sens := ReviewSensitivity(strings.ToUpper(string(*cfg.Sensitivity)))
		switch sens {
		case SensitivityLenient, SensitivityStandard, SensitivityStrict, SensitivityPedantic:
			cfg.Sensitivity = &sens
		default:
			warnings = append(warnings, fmt.Sprintf("invalid sensitivity '%s'; defaulting to standard", *cfg.Sensitivity))
			normalized := SensitivityStandard
			cfg.Sensitivity = &normalized
		}
		var str domain.ModelStrictness
		switch *cfg.Sensitivity {
		case SensitivityStrict, SensitivityPedantic:
			str = domain.StrictnessStrict
		case SensitivityStandard:
			str = domain.StrictnessBalanced
		case SensitivityLenient:
			str = domain.StrictnessLenient
		}
		cfg.Strictness = &str
	}

	if cfg.MaxSuggestions != nil && cfg.MaxCommentsPerReview == nil {
		cfg.MaxCommentsPerReview = cfg.MaxSuggestions
	} else if cfg.MaxCommentsPerReview != nil && cfg.MaxSuggestions == nil {
		cfg.MaxSuggestions = cfg.MaxCommentsPerReview
	}

	if cfg.MaxCommentsPerReview != nil {
		if *cfg.MaxCommentsPerReview < 0 {
			warnings = append(warnings, "max_comments_per_review cannot be negative; clamping to 0")
			val := 0
			cfg.MaxCommentsPerReview = &val
			cfg.MaxSuggestions = &val
		} else if *cfg.MaxCommentsPerReview > 100 {
			warnings = append(warnings, "max_comments_per_review exceeds safe ceiling (100); clamping to 100")
			val := 100
			cfg.MaxCommentsPerReview = &val
			cfg.MaxSuggestions = &val
		}
	}

	return cfg, warnings, nil
}

// ExtractFromPatches extracts in-repo configuration from a set of pull request diff patches.
func (p *InRepoConfigParser) ExtractFromPatches(patches []*diff.FilePatch) (*InRepoConfiguration, string, []string, error) {
	for _, patch := range patches {
		targetPath := patch.NewPath
		if targetPath == "" {
			targetPath = patch.OldPath
		}

		if p.IsConfigFilePath(targetPath) {
			var sb strings.Builder
			for _, h := range patch.Hunks {
				for _, l := range h.Lines {
					if l.Type != diff.LineDeletion {
						sb.WriteString(l.Content)
						sb.WriteString("\n")
					}
				}
			}
			content := sb.String()
			if strings.TrimSpace(content) == "" {
				continue
			}

			cfg, warnings, err := p.ParseContent(targetPath, []byte(content))
			if err != nil {
				return nil, targetPath, warnings, err
			}
			return cfg, targetPath, warnings, nil
		}
	}
	return nil, "", nil, nil
}

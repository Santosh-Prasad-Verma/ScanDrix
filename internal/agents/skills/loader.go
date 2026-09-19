// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Dynamic Skills Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package skills

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	frontmatterRegex = regexp.MustCompile(`(?s)^---\s*\r?\n(.*?)\r?\n---\s*\r?\n?(.*)$`)
)

// RequiredMcp declares an external integration dependency needed by a skill.
type RequiredMcp struct {
	Category string `yaml:"category" json:"category"`
	Label    string `yaml:"label" json:"label"`
	Examples string `yaml:"examples,omitempty" json:"examples,omitempty"`
}

// SkillExecutionPolicy configures run-time limits, timeouts, and verification behavior.
type SkillExecutionPolicy struct {
	OnMissingMcp         string `yaml:"on-missing-mcp,omitempty" json:"on_missing_mcp,omitempty"` // "fail" | "fallback"
	OnMcpConnectError    string `yaml:"on-mcp-connect-error,omitempty" json:"on_mcp_connect_error,omitempty"`
	FetcherTimeoutMs     int    `yaml:"fetcher-timeout-ms,omitempty" json:"fetcher_timeout_ms,omitempty"`
	AnalyzerTimeoutMs    int    `yaml:"analyzer-timeout-ms,omitempty" json:"analyzer_timeout_ms,omitempty"`
	FetcherMaxIterations int    `yaml:"fetcher-max-iterations,omitempty" json:"fetcher_max_iterations,omitempty"`
	AnalyzerMaxIterations int   `yaml:"analyzer-max-iterations,omitempty" json:"analyzer_max_iterations,omitempty"`
	ContextWindowTokens  int    `yaml:"context-window-tokens,omitempty" json:"context_window_tokens,omitempty"`
	VerifyAnalyzerResult bool   `yaml:"verify-analyzer-result,omitempty" json:"verify_analyzer_result,omitempty"`
}

// SkillFetcherPolicy governs how tools are matched and executed for a skill's fetcher stage.
type SkillFetcherPolicy struct {
	ToolMode          string `yaml:"tool-mode,omitempty" json:"tool_mode,omitempty"` // "any" | "all"
	AllowWithoutTools bool   `yaml:"allow-without-tools,omitempty" json:"allow_without_tools,omitempty"`
}

// SkillContracts defines mandatory inputs and output fields for runtime validation.
type SkillContracts struct {
	RequiredContextFields []string `yaml:"required-context-fields,omitempty" json:"required_context_fields,omitempty"`
	RequiredOutputFields  []string `yaml:"required-fields,omitempty" json:"required_output_fields,omitempty"`
}

// SkillCapabilityDefinition describes how a specific capability is satisfied.
type SkillCapabilityDefinition struct {
	Mode  string   `yaml:"mode" json:"mode"` // "fixed_tools" | "provider_dynamic"
	Tools []string `yaml:"tools,omitempty" json:"tools,omitempty"`
}

// SkillManifest represents the complete specification of an agent skill loaded from SKILL.md.
type SkillManifest struct {
	Name                  string                               `json:"name"`
	Description           string                               `json:"description"`
	Version               string                               `json:"version,omitempty"`
	License               string                               `json:"license,omitempty"`
	AllowedTools          []string                             `json:"allowed_tools,omitempty"`
	Capabilities          []string                             `json:"capabilities,omitempty"`
	CapabilityToolMap     map[string][]string                  `json:"capability_tool_map,omitempty"`
	CapabilityDefinitions map[string]SkillCapabilityDefinition `json:"capability_definitions,omitempty"`
	RequiredMcps          []RequiredMcp                        `json:"required_mcps,omitempty"`
	ExecutionPolicy       SkillExecutionPolicy                 `json:"execution_policy"`
	FetcherPolicy         SkillFetcherPolicy                   `json:"fetcher_policy"`
	Contracts             SkillContracts                       `json:"contracts"`
	Instructions          string                               `json:"instructions"`
	RawPath               string                               `json:"raw_path,omitempty"`
}

// DefaultSkillExecutionPolicy provides safe production defaults.
func DefaultSkillExecutionPolicy() SkillExecutionPolicy {
	return SkillExecutionPolicy{
		OnMissingMcp:         "fail",
		OnMcpConnectError:    "fail",
		FetcherTimeoutMs:     120000,
		AnalyzerTimeoutMs:    120000,
		FetcherMaxIterations: 4,
		AnalyzerMaxIterations: 1,
		VerifyAnalyzerResult: false,
	}
}

// DefaultSkillFetcherPolicy provides default tool matching.
func DefaultSkillFetcherPolicy() SkillFetcherPolicy {
	return SkillFetcherPolicy{
		ToolMode:          "any",
		AllowWithoutTools: false,
	}
}

// LoadSkillFromFile reads and parses a SKILL.md file from disk.
func LoadSkillFromFile(filePath string) (*SkillManifest, error) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed reading skill file %s: %w", filePath, err)
	}

	manifest, err := ParseSkillManifest(string(bytes))
	if err != nil {
		return nil, fmt.Errorf("failed parsing skill %s: %w", filePath, err)
	}
	manifest.RawPath = filePath
	return manifest, nil
}

// ParseSkillManifest parses frontmatter YAML and markdown instructions from a SKILL.md string.
func ParseSkillManifest(content string) (*SkillManifest, error) {
	cleaned := strings.TrimPrefix(content, "\ufeff")
	cleaned = strings.TrimPrefix(cleaned, "\xef\xbb\xbf")
	cleaned = strings.TrimSpace(cleaned)

	if cleaned == "" {
		return nil, errors.New("empty skill content")
	}

	match := frontmatterRegex.FindStringSubmatch(cleaned)
	if len(match) < 3 {
		return nil, errors.New("missing valid YAML frontmatter delimiters (---)")
	}

	rawYAML := match[1]
	instructions := strings.TrimSpace(match[2])

	var root map[string]any
	if err := yaml.Unmarshal([]byte(rawYAML), &root); err != nil {
		return nil, fmt.Errorf("invalid YAML in frontmatter: %w", err)
	}

	manifest := &SkillManifest{
		ExecutionPolicy: DefaultSkillExecutionPolicy(),
		FetcherPolicy:   DefaultSkillFetcherPolicy(),
		Instructions:    instructions,
	}

	// 1. Root level properties
	if name, ok := root["name"].(string); ok {
		manifest.Name = strings.TrimSpace(name)
	}
	if desc, ok := root["description"].(string); ok {
		manifest.Description = strings.TrimSpace(desc)
	}
	if lic, ok := root["license"].(string); ok {
		manifest.License = strings.TrimSpace(lic)
	}

	// Allowed tools can be array or whitespace/comma-separated string
	manifest.AllowedTools = parseStringOrSlice(root["allowed-tools"])

	// 2. Discover platform extensions under metadata or root
	var extMap map[string]any
	if meta, ok := root["metadata"].(map[string]any); ok {
		if ver, ok := meta["version"].(string); ok {
			manifest.Version = ver
		}
		// Inspect nested extension dicts
		for _, k := range []string{"scandrix", "extensions", "platform"} {
			if sub, ok := meta[k].(map[string]any); ok {
				extMap = sub
				break
			}
		}
		if extMap == nil {
			// Find first map in metadata if none of the explicit keys matched
			for _, v := range meta {
				if sub, ok := v.(map[string]any); ok {
					extMap = sub
					break
				}
			}
		}
	}
	if extMap == nil {
		extMap = root
	}

	// 3. Capabilities
	manifest.Capabilities = parseStringSlice(extMap["capabilities"])

	// 4. Capability Definitions
	if defs, ok := extMap["capability-definitions"].(map[string]any); ok {
		manifest.CapabilityDefinitions = make(map[string]SkillCapabilityDefinition)
		for k, v := range defs {
			if defMap, ok := v.(map[string]any); ok {
				var cd SkillCapabilityDefinition
				if m, ok := defMap["mode"].(string); ok {
					cd.Mode = m
				}
				cd.Tools = parseStringOrSlice(defMap["tools"])
				manifest.CapabilityDefinitions[k] = cd
			}
		}
	}

	// 5. Execution Policy
	if ep, ok := extMap["execution-policy"].(map[string]any); ok {
		if v, ok := ep["on-missing-mcp"].(string); ok {
			manifest.ExecutionPolicy.OnMissingMcp = v
		}
		if v, ok := ep["on-mcp-connect-error"].(string); ok {
			manifest.ExecutionPolicy.OnMcpConnectError = v
		}
		if v, ok := toInt(ep["fetcher-timeout-ms"]); ok {
			manifest.ExecutionPolicy.FetcherTimeoutMs = v
		}
		if v, ok := toInt(ep["analyzer-timeout-ms"]); ok {
			manifest.ExecutionPolicy.AnalyzerTimeoutMs = v
		}
		if v, ok := toInt(ep["fetcher-max-iterations"]); ok {
			manifest.ExecutionPolicy.FetcherMaxIterations = v
		}
		if v, ok := toInt(ep["analyzer-max-iterations"]); ok {
			manifest.ExecutionPolicy.AnalyzerMaxIterations = v
		}
		if v, ok := toInt(ep["context-window-tokens"]); ok {
			manifest.ExecutionPolicy.ContextWindowTokens = v
		}
		if v, ok := ep["verify-analyzer-result"].(bool); ok {
			manifest.ExecutionPolicy.VerifyAnalyzerResult = v
		}
	}

	// 6. Fetcher Policy
	if fp, ok := extMap["fetcher-policy"].(map[string]any); ok {
		if v, ok := fp["tool-mode"].(string); ok {
			manifest.FetcherPolicy.ToolMode = v
		}
		if v, ok := fp["allow-without-tools"].(bool); ok {
			manifest.FetcherPolicy.AllowWithoutTools = v
		}
	}

	// 7. Contracts
	if c, ok := extMap["contracts"].(map[string]any); ok {
		if in, ok := c["input"].(map[string]any); ok {
			manifest.Contracts.RequiredContextFields = parseStringSlice(in["required-context-fields"])
		}
		if out, ok := c["output"].(map[string]any); ok {
			manifest.Contracts.RequiredOutputFields = parseStringSlice(out["required-fields"])
		}
	}

	// 8. Required MCPs
	if rmcps, ok := extMap["required-mcps"].([]any); ok {
		for _, item := range rmcps {
			if m, ok := item.(map[string]any); ok {
				rm := RequiredMcp{}
				if c, ok := m["category"].(string); ok {
					rm.Category = c
				}
				if l, ok := m["label"].(string); ok {
					rm.Label = l
				}
				if e, ok := m["examples"].(string); ok {
					rm.Examples = e
				}
				if rm.Category != "" {
					manifest.RequiredMcps = append(manifest.RequiredMcps, rm)
				}
			}
		}
	}

	return manifest, nil
}

func parseStringOrSlice(val any) []string {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case string:
		fields := strings.FieldsFunc(v, func(r rune) bool {
			return r == ' ' || r == ','
		})
		var clean []string
		for _, f := range fields {
			trimmed := strings.TrimSpace(f)
			if trimmed != "" {
				clean = append(clean, trimmed)
			}
		}
		return clean
	case []any:
		var clean []string
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				clean = append(clean, strings.TrimSpace(s))
			}
		}
		return clean
	case []string:
		return v
	default:
		return nil
	}
}

func parseStringSlice(val any) []string {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []any:
		var clean []string
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				clean = append(clean, strings.TrimSpace(s))
			}
		}
		return clean
	case []string:
		return v
	default:
		return nil
	}
}

func toInt(val any) (int, bool) {
	if val == nil {
		return 0, false
	}
	switch v := val.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

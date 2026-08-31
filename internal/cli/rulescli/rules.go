// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package rulescli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/rules"
)

// StarterRulesYAML defines the default template for local custom rules.
const StarterRulesYAML = `# ScanDrix Custom Repository Rules
# Edit this file to enforce team and organization security & quality policies.
# Syntax reference: https://docs.scandrix.dev/rules

version: "1.0"
rules:
  - id: custom-no-hardcoded-dev-secrets
    title: Prohibit Hardcoded Development Keys & Secrets
    severity: CRITICAL
    category: SECURITY
    description: Flags exposed API keys, private tokens, or connection credentials.
    pattern: "(?i)(api[_-]?key|secret[_-]?key|private[_-]?key)\\s*[:=]\\s*['\"][A-Za-z0-9_\\-]{16,}['\"]"
    remediation: Store credentials in environment variables and read at runtime via os.Getenv() or process.env.

  - id: custom-prohibit-raw-sql-concat
    title: Prohibit Raw SQL String Concatenation
    severity: HIGH
    category: SECURITY
    description: Detects potential SQL injection via string concatenation or Sprintf.
    pattern: "(?i)(SELECT|INSERT|UPDATE|DELETE)\\s+.*\\+\\s*\\w+|fmt\\.Sprintf\\(.*(SELECT|INSERT|UPDATE|DELETE)\\)"
    remediation: Use parameterized queries with bind variables ($1, $2) or an ORM.

  - id: custom-no-console-log-in-prod
    title: Disallow console.log in Production Code
    severity: LOW
    category: QUALITY
    description: Prevents noisy or sensitive console logs from leaking into production.
    pattern: "console\\.log\\("
    remediation: Use a structured logger with log levels instead of raw console.log.
`

// RuleModel models a custom rule definition.
type RuleModel struct {
	ID          string `json:"id" yaml:"id"`
	Title       string `json:"title" yaml:"title"`
	Name        string `json:"name,omitempty" yaml:"name,omitempty"`
	Severity    string `json:"severity" yaml:"severity"`
	Category    string `json:"category" yaml:"category"`
	Description string `json:"description" yaml:"description"`
	Pattern     string `json:"pattern" yaml:"pattern"`
	RegexRule   string `json:"regex_rule,omitempty" yaml:"regex_rule,omitempty"`
	PathPattern string `json:"path_pattern,omitempty" yaml:"path_pattern,omitempty"`
	Remediation string `json:"remediation,omitempty" yaml:"remediation,omitempty"`
	Scope       string `json:"scope,omitempty" yaml:"scope,omitempty"`
	RepoID      string `json:"repo_id,omitempty" yaml:"repo_id,omitempty"`
}

// Init creates a starter .scandrix/rules.yaml file in the specified directory.
func Init(workDir string) (string, error) {
	if workDir == "" {
		workDir = "."
	}
	dir := filepath.Join(workDir, ".scandrix")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed creating .scandrix directory: %w", err)
	}

	targetPath := filepath.Join(dir, "rules.yaml")
	if _, err := os.Stat(targetPath); err == nil {
		return targetPath, fmt.Errorf("rules file already exists at %s", targetPath)
	}

	if err := os.WriteFile(targetPath, []byte(StarterRulesYAML), 0644); err != nil {
		return "", fmt.Errorf("failed writing starter rules: %w", err)
	}

	return targetPath, nil
}

// CreateRule registers a new rule on the remote ScanDrix server.
func CreateRule(ctx context.Context, serverURL, token, title, rulePattern, repoID, severity, scope, pathPattern string) (*RuleModel, error) {
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
	if severity == "" {
		severity = "MEDIUM"
	}

	payload := map[string]any{
		"name":         title,
		"regex_rule":   rulePattern,
		"severity":     severity,
		"category":     "SECURITY",
		"description":  rulePattern,
		"path_pattern": pathPattern,
		"repo_id":      repoID,
		"scope":        scope,
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/v1/rules", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed connecting to ScanDrix server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var res RuleModel
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	if res.Title == "" {
		res.Title = title
	}
	if res.Pattern == "" {
		res.Pattern = rulePattern
	}
	return &res, nil
}

// ViewRules lists available rules from the server catalog or local catalog.
func ViewRules(ctx context.Context, serverURL, token, ruleID, repoID string) ([]RuleModel, error) {
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/v1/rules/catalog", nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var catalogRules []rules.RuleSpec
		if err := json.NewDecoder(resp.Body).Decode(&catalogRules); err == nil && len(catalogRules) > 0 {
			res := make([]RuleModel, 0, len(catalogRules))
			for _, r := range catalogRules {
				res = append(res, RuleModel{
					ID:          r.ID.String(),
					Title:       r.Name,
					Severity:    string(r.Severity),
					Category:    r.Category,
					Description: r.Description,
					Pattern:     r.RegexRule,
					Remediation: r.Remediation,
				})
			}
			return res, nil
		}
	}
	if resp != nil {
		resp.Body.Close()
	}

	// Fallback to local catalog
	cat := rules.DefaultCatalog()
	res := make([]RuleModel, 0, len(cat))
	for _, r := range cat {
		res = append(res, RuleModel{
			ID:          r.ID.String(),
			Title:       r.Name,
			Severity:    string(r.Severity),
			Category:    r.Category,
			Description: r.Description,
			Pattern:     r.RegexRule,
			Remediation: r.Remediation,
		})
	}
	return res, nil
}

// Sync fetches the workspace's active rules from the ScanDrix server.
func Sync(ctx context.Context, serverURL, authToken, workDir string) (int, error) {
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
	if authToken == "" {
		return 0, fmt.Errorf("authentication required to sync rules from workspace")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/v1/rules/catalog", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+authToken)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed connecting to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("server returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var rulesList []rules.RuleSpec
	if err := json.NewDecoder(resp.Body).Decode(&rulesList); err != nil {
		return 0, fmt.Errorf("failed parsing server rules response: %w", err)
	}

	if workDir == "" {
		workDir = "."
	}
	dir := filepath.Join(workDir, ".scandrix")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, fmt.Errorf("failed creating .scandrix directory: %w", err)
	}

	// Format synced rules as YAML
	var yamlBuf bytes.Buffer
	yamlBuf.WriteString("# ScanDrix Synchronized Organization Rules\n")
	yamlBuf.WriteString(fmt.Sprintf("# Synced at: %s\n\n", time.Now().UTC().Format(time.RFC3339)))
	yamlBuf.WriteString("version: \"1.0\"\nrules:\n")

	for _, r := range rulesList {
		yamlBuf.WriteString(fmt.Sprintf("  - id: %s\n", r.ID.String()))
		yamlBuf.WriteString(fmt.Sprintf("    title: %q\n", r.Name))
		yamlBuf.WriteString(fmt.Sprintf("    severity: %s\n", r.Severity))
		yamlBuf.WriteString(fmt.Sprintf("    category: %s\n", r.Category))
		yamlBuf.WriteString(fmt.Sprintf("    description: %q\n", r.Description))
		yamlBuf.WriteString(fmt.Sprintf("    pattern: %q\n", r.RegexRule))
		if r.Remediation != "" {
			yamlBuf.WriteString(fmt.Sprintf("    remediation: %q\n", r.Remediation))
		}
		yamlBuf.WriteString("\n")
	}

	rulesPath := filepath.Join(dir, "rules.yaml")
	if err := os.WriteFile(rulesPath, yamlBuf.Bytes(), 0644); err != nil {
		return 0, fmt.Errorf("failed writing rules file %s: %w", rulesPath, err)
	}

	return len(rulesList), nil
}

// Validate checks the syntax and regex patterns of the local .scandrix/rules.yaml file.
func Validate(workDir string) (int, error) {
	if workDir == "" {
		workDir = "."
	}
	rulesFile := filepath.Join(workDir, ".scandrix", "rules.yaml")
	data, err := os.ReadFile(rulesFile)
	if err != nil {
		return 0, fmt.Errorf("could not read %s: %w", rulesFile, err)
	}

	if len(data) == 0 {
		return 0, fmt.Errorf("rules file %s is empty", rulesFile)
	}

	// Basic validation of regular expression patterns
	validCount := 0
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "pattern:") {
			pat := strings.TrimSpace(strings.TrimPrefix(trimmed, "pattern:"))
			if strings.HasPrefix(pat, "\"") && strings.HasSuffix(pat, "\"") {
				if unquoted, err := strconv.Unquote(pat); err == nil {
					pat = unquoted
				} else {
					pat = strings.Trim(pat, "\"")
				}
			} else {
				pat = strings.Trim(pat, "'\"")
			}
			if _, err := regexp.Compile(pat); err != nil {
				return validCount, fmt.Errorf("invalid regex pattern %q: %w", pat, err)
			}
			validCount++
		}
	}

	if validCount == 0 {
		validCount = len(rules.DefaultCatalog())
	}

	return validCount, nil
}

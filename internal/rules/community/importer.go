// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package community

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	MaxImportPayloadSize = 2 * 1024 * 1024 // 2MB safety limit
	DefaultHTTPTimeout   = 15 * time.Second
)

// Importer fetches and validates community rule packs from remote Git repositories or raw URLs.
type Importer struct {
	httpClient *http.Client
}

// NewImporter creates a new rule pack importer.
func NewImporter(client ...*http.Client) *Importer {
	c := &http.Client{Timeout: DefaultHTTPTimeout}
	if len(client) > 0 && client[0] != nil {
		c = client[0]
	}
	return &Importer{httpClient: c}
}

// NormalizeRemoteURL converts common GitHub/GitLab blob URLs into their direct raw file counterparts.
func NormalizeRemoteURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return rawURL
	}

	// GitHub: https://github.com/owner/repo/blob/main/rules.json -> https://raw.githubusercontent.com/owner/repo/main/rules.json
	if parsed.Host == "github.com" {
		parts := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
		if len(parts) >= 4 && parts[2] == "blob" {
			owner := parts[0]
			repo := parts[1]
			ref := parts[3]
			filePath := strings.Join(parts[4:], "/")
			return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, ref, filePath)
		}
	}

	// GitLab: https://gitlab.com/owner/repo/-/blob/main/rules.json -> https://gitlab.com/owner/repo/-/raw/main/rules.json
	if parsed.Host == "gitlab.com" && strings.Contains(parsed.Path, "/-/blob/") {
		return strings.Replace(rawURL, "/-/blob/", "/-/raw/", 1)
	}

	return rawURL
}

// FetchAndValidate downloads a remote rule pack JSON/YAML, sanitizes and verifies each rule.
func (imp *Importer) FetchAndValidate(ctx context.Context, targetURL string) (*RulePack, error) {
	directURL := NormalizeRemoteURL(targetURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, directURL, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid request URL: %w", err)
	}
	req.Header.Set("User-Agent", "ScanDrix-RulePack-Importer/1.0")

	resp, err := imp.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed fetching remote rule pack: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("remote server returned HTTP %d", resp.StatusCode)
	}

	lr := io.LimitReader(resp.Body, MaxImportPayloadSize)
	content, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("failed reading remote content: %w", err)
	}

	return ParseAndValidateRulePack(content)
}

// ParseAndValidateRulePack parses raw bytes as JSON or YAML, then validates regex patterns.
func ParseAndValidateRulePack(data []byte) (*RulePack, error) {
	var pack RulePack

	// Try JSON first
	err := json.Unmarshal(data, &pack)
	if err != nil {
		// Fallback to YAML
		yamlErr := yaml.Unmarshal(data, &pack)
		if yamlErr != nil {
			return nil, fmt.Errorf("failed parsing as JSON (%v) or YAML (%v)", err, yamlErr)
		}
	}

	if strings.TrimSpace(pack.Title) == "" {
		return nil, errors.New("rule pack must contain a non-empty 'title'")
	}
	if len(pack.Rules) == 0 {
		return nil, errors.New("rule pack must contain at least one valid rule in 'rules'")
	}

	// Validate each rule's regex pattern to avoid ReDoS or runtime crashes
	for idx, r := range pack.Rules {
		if strings.TrimSpace(r.Name) == "" {
			return nil, fmt.Errorf("rule #%d is missing a name", idx+1)
		}
		if strings.TrimSpace(r.RegexRule) != "" {
			if _, compileErr := regexp.Compile(r.RegexRule); compileErr != nil {
				return nil, fmt.Errorf("rule '%s' has invalid regex syntax: %w", r.Name, compileErr)
			}
		}
	}

	if pack.ID == "" {
		pack.ID = fmt.Sprintf("imported-%d", time.Now().UnixNano())
	}
	if pack.Category == "" {
		pack.Category = "COMMUNITY"
	}
	if pack.Author == "" {
		pack.Author = "Community Contributor"
	}
	if pack.Icon == "" {
		pack.Icon = "📦"
	}

	return &pack, nil
}

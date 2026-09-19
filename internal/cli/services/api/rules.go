// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package api

import (
	"context"
	"encoding/json"
	"net/http"
)

// DATA CONTRACTS & DTOs

// RuleModel represents a custom security or quality rule.
type RuleModel struct {
	UUID         string `json:"uuid"`
	Title        string `json:"title"`
	Rule         string `json:"rule"`
	Description  string `json:"description,omitempty"`
	Severity     string `json:"severity"`
	Scope        string `json:"scope"`
	Path         string `json:"path"`
	RepoID       string `json:"repo_id,omitempty"`
	RepositoryID string `json:"repositoryId,omitempty"`
}

// UnmarshalJSON handles both repositoryId and repo_id fields.
func (r *RuleModel) UnmarshalJSON(data []byte) error {
	type Alias RuleModel
	var a struct {
		Alias
		RepositoryID string `json:"repositoryId"`
	}
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*r = RuleModel(a.Alias)
	if a.RepositoryID != "" && r.RepoID == "" {
		r.RepoID = a.RepositoryID
	}
	return nil
}

// RuleMutationResponse represents rule creation or update result, which may be a direct update or a PR proposal.
type RuleMutationResponse struct {
	Rule     *RuleModel `json:"rule,omitempty"`
	Message  string     `json:"message,omitempty"`
	PRURL    string     `json:"pr_url,omitempty"`
	PRNumber int        `json:"pr_number,omitempty"`
	Mode     string     `json:"mode,omitempty"`
}

// UnmarshalJSON parses both direct rule responses, envelope shapes, and centralized PR payloads.
func (r *RuleMutationResponse) UnmarshalJSON(data []byte) error {
	type Alias RuleMutationResponse
	var a struct {
		Alias
		Mode     string `json:"mode"`
		PRURL    string `json:"prUrl"`
		PRNumber int    `json:"prNumber"`
	}
	if err := json.Unmarshal(data, &a); err == nil {
		*r = RuleMutationResponse(a.Alias)
		if a.PRURL != "" && r.PRURL == "" {
			r.PRURL = a.PRURL
		}
		if a.PRNumber != 0 && r.PRNumber == 0 {
			r.PRNumber = a.PRNumber
		}
		if a.Mode != "" {
			r.Mode = a.Mode
		}
		if a.Mode == "centralized-pr" || r.PRURL != "" {
			return nil
		}
	}

	// Try unmarshaling as a RuleModel directly
	var rule RuleModel
	if err := json.Unmarshal(data, &rule); err == nil && rule.UUID != "" {
		r.Rule = &rule
		return nil
	}
	return nil
}

// RULES API OPERATIONS (Create, Update, View, Generate)

// CreateRule creates a custom rule on the central server or creates a PR proposal.
func (c *Client) CreateRule(ctx context.Context, title, rule, repoID, severity, scope, path string) (*RuleMutationResponse, error) {
	payload := map[string]string{
		"title":        title,
		"rule":         rule,
		"repositoryId": repoID,
		"repo_id":      repoID,
		"severity":     severity,
		"scope":        scope,
		"path":         path,
	}

	var resp RuleMutationResponse
	if err := c.Do(ctx, http.MethodPost, "/cli/drixy-rules", payload, &resp); err != nil {
		if errFallback := c.Do(ctx, http.MethodPost, "/cli/rules", payload, &resp); errFallback != nil {
			return nil, err
		}
	}
	return &resp, nil
}

// UpdateRule modifies an existing custom rule.
func (c *Client) UpdateRule(ctx context.Context, uuid, title, rule, repoID, severity, scope, path string) (*RuleMutationResponse, error) {
	payload := map[string]string{
		"uuid":         uuid,
		"title":        title,
		"rule":         rule,
		"repositoryId": repoID,
		"repo_id":      repoID,
		"severity":     severity,
		"scope":        scope,
		"path":         path,
	}

	var resp RuleMutationResponse
	if err := c.Do(ctx, http.MethodPatch, "/cli/drixy-rules/"+uuid, payload, &resp); err != nil {
		if errFallback := c.Do(ctx, http.MethodPatch, "/cli/rules/"+uuid, payload, &resp); errFallback != nil {
			return nil, err
		}
	}
	return &resp, nil
}

// ViewRules lists custom rules optionally filtered by rule UUID or repo ID.
func (c *Client) ViewRules(ctx context.Context, ruleID, repoID string) ([]RuleModel, error) {
	endpoint := "/cli/drixy-rules"
	params := []string{}
	if ruleID != "" {
		params = append(params, "ruleId="+ruleID)
	}
	if repoID != "" {
		params = append(params, "repositoryId="+repoID)
	}
	if len(params) > 0 {
		endpoint += "?" + params[0]
		for _, p := range params[1:] {
			endpoint += "&" + p
		}
	}

	var rules []RuleModel
	if err := c.Do(ctx, http.MethodGet, endpoint, nil, &rules); err != nil {
		fallbackEndpoint := "/cli/rules"
		if len(params) > 0 {
			fallbackEndpoint += "?" + params[0]
			for _, p := range params[1:] {
				fallbackEndpoint += "&" + p
			}
		}
		if errFallback := c.Do(ctx, http.MethodGet, fallbackEndpoint, nil, &rules); errFallback != nil {
			return nil, err
		}
	}
	return rules, nil
}

// GenerateRule asks Drixy AI to synthesize an AST / regex rule from natural language.
func (c *Client) GenerateRule(ctx context.Context, prompt string) (*RuleModel, error) {
	payload := map[string]string{"prompt": prompt}
	var resp RuleModel
	if err := c.Do(ctx, http.MethodPost, "/api/v1/rules/generate", payload, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

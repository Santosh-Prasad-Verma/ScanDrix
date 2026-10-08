package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"path"
	"strings"
)

// RepositoryReviewSettings is the effective policy for a tracked repository.
// Active is read from is_active; the other fields are persisted in review_settings.
type RepositoryReviewSettings struct {
	Active            bool     `json:"active"`
	AutoReviewEnabled bool     `json:"autoReviewEnabled"`
	DryRunEnabled     bool     `json:"dryRunEnabled"`
	IgnoreBots        bool     `json:"ignoreBots"`
	BranchesMonitored []string `json:"branchesMonitored"`
	IgnoredPaths      []string `json:"ignoredPaths"`
	ModelOverride     string   `json:"modelOverride"`
	// CodeReviewConfig is the dashboard's per-repository review configuration.
	// Always non-nil so a caller never has to distinguish "no configuration" from
	// "an absent value"; an empty map means nothing has been configured yet.
	CodeReviewConfig map[string]any `json:"codeReviewConfig"`
}

type RepositoryReviewSettingsPatch struct {
	Active            *bool     `json:"active,omitempty"`
	AutoReviewEnabled *bool     `json:"autoReviewEnabled,omitempty"`
	DryRunEnabled     *bool     `json:"dryRunEnabled,omitempty"`
	IgnoreBots        *bool     `json:"ignoreBots,omitempty"`
	BranchesMonitored *[]string `json:"branchesMonitored,omitempty"`
	IgnoredPaths      *[]string `json:"ignoredPaths,omitempty"`
	ModelOverride     *string   `json:"modelOverride,omitempty"`
	// CodeReviewConfig is the dashboard's per-repository review configuration as a
	// document: custom messages, PR summary, review categories, suggestion
	// control, ignored title keywords, prompt overrides and so on.
	//
	// It is stored in its own column rather than in workspace_parameters because
	// that table is keyed by workspace alone: saving one repository's settings
	// through it would overwrite every other repository's.
	CodeReviewConfig *map[string]any `json:"codeReviewConfig,omitempty"`
}

func (p *RepositoryReviewSettingsPatch) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("settings cannot be null")
		}
	}
	type patch RepositoryReviewSettingsPatch
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*patch)(p))
}

// DecodeRepositoryReviewSettings builds the settings from the two per-repository
// columns. configRaw is the code_review_config document and may be empty.
func DecodeRepositoryReviewSettings(raw []byte, active bool, configRaw ...[]byte) (RepositoryReviewSettings, error) {
	cfg := RepositoryReviewSettings{Active: active, AutoReviewEnabled: true, BranchesMonitored: []string{}, IgnoredPaths: []string{}, CodeReviewConfig: map[string]any{}}
	for _, document := range configRaw {
		if len(document) > 2 {
			var decoded map[string]any
			if err := json.Unmarshal(document, &decoded); err == nil && decoded != nil {
				cfg.CodeReviewConfig = decoded
			}
		}
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return cfg, err
		}
	}
	cfg.Active = active
	if cfg.BranchesMonitored == nil || cfg.IgnoredPaths == nil {
		return cfg, errors.New("stored patterns must be arrays")
	}
	if err := (RepositoryReviewSettingsPatch{BranchesMonitored: &cfg.BranchesMonitored, IgnoredPaths: &cfg.IgnoredPaths, ModelOverride: &cfg.ModelOverride}).Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (p RepositoryReviewSettingsPatch) Validate() error {
	for _, list := range []*[]string{p.BranchesMonitored, p.IgnoredPaths} {
		if list == nil {
			continue
		}
		if len(*list) > 100 {
			return errors.New("too many patterns")
		}
		for _, pattern := range *list {
			if pattern == "" || len(pattern) > 300 || strings.ContainsAny(pattern, "\x00\r\n") {
				return errors.New("invalid pattern")
			}
			if strings.Contains(pattern, "**") && !strings.HasSuffix(pattern, "/**") {
				return errors.New("recursive patterns must end in /**")
			}
			if _, err := path.Match(pattern, ""); err != nil {
				return errors.New("invalid glob pattern")
			}
		}
	}
	if p.ModelOverride != nil && (len(*p.ModelOverride) > 200 || strings.ContainsAny(*p.ModelOverride, "\x00\r\n")) {
		return errors.New("invalid model override")
	}
	return nil
}

// MatchesRepositoryPattern supports path globs, basename globs, and directory/**.
func MatchesRepositoryPattern(value, pattern string) bool {
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		for candidate := value; candidate != "." && candidate != "/"; candidate = path.Dir(candidate) {
			if match, _ := path.Match(prefix, candidate); match {
				return true
			}
		}
		return false
	}
	match, _ := path.Match(pattern, value)
	if !match && !strings.Contains(pattern, "/") {
		match, _ = path.Match(pattern, path.Base(value))
	}
	return match
}

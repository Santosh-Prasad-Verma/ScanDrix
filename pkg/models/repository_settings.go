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
}

type RepositoryReviewSettingsPatch struct {
	Active            *bool     `json:"active,omitempty"`
	AutoReviewEnabled *bool     `json:"autoReviewEnabled,omitempty"`
	DryRunEnabled     *bool     `json:"dryRunEnabled,omitempty"`
	IgnoreBots        *bool     `json:"ignoreBots,omitempty"`
	BranchesMonitored *[]string `json:"branchesMonitored,omitempty"`
	IgnoredPaths      *[]string `json:"ignoredPaths,omitempty"`
	ModelOverride     *string   `json:"modelOverride,omitempty"`
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

func DecodeRepositoryReviewSettings(raw []byte, active bool) (RepositoryReviewSettings, error) {
	cfg := RepositoryReviewSettings{Active: active, AutoReviewEnabled: true, BranchesMonitored: []string{}, IgnoredPaths: []string{}}
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

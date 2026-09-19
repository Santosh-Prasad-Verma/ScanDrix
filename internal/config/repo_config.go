package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// RepoReviewConfig specifies pull request review parameters.
type RepoReviewConfig struct {
	Enabled        bool     `yaml:"enabled" json:"enabled"`
	Mode           string   `yaml:"mode" json:"mode"` // "strict", "standard", "lenient"
	BotName        string   `yaml:"bot_name" json:"bot_name"`
	IgnorePatterns []string `yaml:"ignore_patterns" json:"ignore_patterns"`
	MaxComments    int      `yaml:"max_comments" json:"max_comments"`
	AutoApprove    bool     `yaml:"auto_approve" json:"auto_approve"`
}

// RepoRulesConfig defines rule engine behavior per repository.
type RepoRulesConfig struct {
	CustomRulesDir    string   `yaml:"custom_rules_dir" json:"custom_rules_dir"` // e.g. ".drixy/rules" or ".scandrix/rules"
	DisabledRules     []string `yaml:"disabled_rules" json:"disabled_rules"`
	SeverityThreshold string   `yaml:"severity_threshold" json:"severity_threshold"` // "LOW", "MEDIUM", "HIGH", "CRITICAL"
}

// RepoNotificationsConfig defines alert channel routing per repo.
type RepoNotificationsConfig struct {
	SlackChannel string   `yaml:"slack_channel" json:"slack_channel"`
	TeamsWebhook string   `yaml:"teams_webhook" json:"teams_webhook"`
	NotifyOn     []string `yaml:"notify_on" json:"notify_on"` // "CRITICAL", "FAILURE", "COMPLETED"
}

// RepoPMConfig configures Jira / Linear defect auto-export per repository.
type RepoPMConfig struct {
	ProjectKey           string   `yaml:"project_key" json:"project_key"`
	AutoTicketSeverities []string `yaml:"auto_ticket_severities" json:"auto_ticket_severities"`
}

// RepositoryConfiguration represents the complete .scandrix.yml repository definition.
type RepositoryConfiguration struct {
	Version       string                  `yaml:"version" json:"version"`
	Review        RepoReviewConfig        `yaml:"review" json:"review"`
	Rules         RepoRulesConfig         `yaml:"rules" json:"rules"`
	Notifications RepoNotificationsConfig `yaml:"notifications" json:"notifications"`
	PM            RepoPMConfig            `yaml:"pm" json:"pm"`
}

// DefaultRepoConfig returns the standard enterprise defaults.
func DefaultRepoConfig() *RepositoryConfiguration {
	return &RepositoryConfiguration{
		Version: "1.0",
		Review: RepoReviewConfig{
			Enabled:        true,
			Mode:           "standard",
			BotName:        "drixy[bot]",
			IgnorePatterns: []string{"vendor/**", "node_modules/**", "*.min.js", "dist/**", "build/**", "*.lock"},
			MaxComments:    20,
			AutoApprove:    false,
		},
		Rules: RepoRulesConfig{
			CustomRulesDir:    ".drixy/rules",
			DisabledRules:     []string{},
			SeverityThreshold: "LOW",
		},
		Notifications: RepoNotificationsConfig{
			NotifyOn: []string{"CRITICAL", "HIGH"},
		},
		PM: RepoPMConfig{
			AutoTicketSeverities: []string{"CRITICAL"},
		},
	}
}

// ParseRepoConfig parses YAML content into a structured RepositoryConfiguration with defaults applied.
func ParseRepoConfig(rawYAML []byte) (*RepositoryConfiguration, error) {
	cfg := DefaultRepoConfig()

	if len(rawYAML) == 0 {
		return cfg, nil
	}

	if err := yaml.Unmarshal(rawYAML, cfg); err != nil {
		return nil, fmt.Errorf("failed parsing repository config YAML: %w", err)
	}

	// Sanitize and normalize fields
	cfg.Review.Mode = strings.ToLower(strings.TrimSpace(cfg.Review.Mode))
	if cfg.Review.Mode == "" {
		cfg.Review.Mode = "standard"
	}
	if cfg.Review.MaxComments <= 0 {
		cfg.Review.MaxComments = 20
	}
	if cfg.Rules.CustomRulesDir == "" {
		cfg.Rules.CustomRulesDir = ".drixy/rules"
	}

	return cfg, nil
}

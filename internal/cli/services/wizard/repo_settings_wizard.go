package wizard

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RepoSettingsWizard orchestrates interactive configuration of repository-level ScanDrix settings.
type RepoSettingsWizard struct {
	reader io.Reader
	writer io.Writer
}

// WizardOptions encapsulates interactive prompts and responses for repository configuration.
type WizardOptions struct {
	EnableHooks          bool     `yaml:"enable_hooks" json:"enable_hooks"`
	ReviewMode           string   `yaml:"review_mode" json:"review_mode"` // "fast", "heavy", "rules-only"
	FailOnSeverity       string   `yaml:"fail_on_severity" json:"fail_on_severity"` // "info", "warning", "error", "critical"
	AutoFix              bool     `yaml:"auto_fix" json:"auto_fix"`
	FocusAreas           []string `yaml:"focus_areas" json:"focus_areas"`
	OfflineFallback      bool     `yaml:"offline_fallback" json:"offline_fallback"`
	EnableASTParsing     bool     `yaml:"enable_ast_parsing" json:"enable_ast_parsing"`
	MaxConcurrency       int      `yaml:"max_concurrency" json:"max_concurrency"`
	IgnoredPaths         []string `yaml:"ignored_paths" json:"ignored_paths"`
}

// NewRepoSettingsWizard creates a new wizard instance.
func NewRepoSettingsWizard(r io.Reader, w io.Writer) *RepoSettingsWizard {
	if r == nil {
		r = os.Stdin
	}
	if w == nil {
		w = os.Stdout
	}
	return &RepoSettingsWizard{
		reader: r,
		writer: w,
	}
}

// PromptConfig interactively gathers configuration values from the user.
func (w *RepoSettingsWizard) PromptConfig(ctx context.Context, initial WizardOptions) (*WizardOptions, error) {
	scanner := bufio.NewScanner(w.reader)
	result := initial

	fmt.Fprintf(w.writer, "\n--- ScanDrix Repository Setup Wizard ---\n\n")

	// 1. Review Mode
	fmt.Fprintf(w.writer, "Select review mode [fast, heavy, rules-only] (default: %s): ", defaultIfEmpty(result.ReviewMode, "fast"))
	if scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text != "" {
			result.ReviewMode = text
		} else if result.ReviewMode == "" {
			result.ReviewMode = "fast"
		}
	}

	// 2. Fail-on Severity
	fmt.Fprintf(w.writer, "Select fail-on severity threshold [info, warning, error, critical] (default: %s): ", defaultIfEmpty(result.FailOnSeverity, "error"))
	if scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text != "" {
			result.FailOnSeverity = text
		} else if result.FailOnSeverity == "" {
			result.FailOnSeverity = "error"
		}
	}

	// 3. Enable Git Hooks
	fmt.Fprintf(w.writer, "Enable automated Git pre-commit / pre-push hooks? [y/N]: ")
	if scanner.Scan() {
		text := strings.ToLower(strings.TrimSpace(scanner.Text()))
		result.EnableHooks = text == "y" || text == "yes"
	}

	// 4. Auto-Fix
	fmt.Fprintf(w.writer, "Enable automatic fixes for deterministic rule violations? [y/N]: ")
	if scanner.Scan() {
		text := strings.ToLower(strings.TrimSpace(scanner.Text()))
		result.AutoFix = text == "y" || text == "yes"
	}

	// 5. Offline Fallback
	fmt.Fprintf(w.writer, "Enable offline fallback cache when API is unreachable? [Y/n]: ")
	if scanner.Scan() {
		text := strings.ToLower(strings.TrimSpace(scanner.Text()))
		result.OfflineFallback = text != "n" && text != "no"
	}

	// 6. AST Parsing
	result.EnableASTParsing = true
	if result.MaxConcurrency <= 0 {
		result.MaxConcurrency = 4
	}

	if len(result.IgnoredPaths) == 0 {
		result.IgnoredPaths = []string{
			"vendor/**",
			"node_modules/**",
			"dist/**",
			".git/**",
			"**/*.min.js",
		}
	}

	fmt.Fprintf(w.writer, "\nRepository configuration completed successfully!\n\n")
	return &result, nil
}

// SaveConfig writes the configuration to .scandrix.yml in repoRoot.
func (w *RepoSettingsWizard) SaveConfig(repoRoot string, opts WizardOptions) error {
	path := filepath.Join(repoRoot, ".scandrix.yml")
	data, err := yaml.Marshal(opts)
	if err != nil {
		return fmt.Errorf("marshal yaml config: %w", err)
	}

	header := []byte("# ScanDrix Repository Configuration\n# https://scandrix.dev/docs/config\n\n")
	content := append(header, data...)

	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}

// LoadConfig reads the configuration from .scandrix.yml in repoRoot.
func (w *RepoSettingsWizard) LoadConfig(repoRoot string) (*WizardOptions, error) {
	path := filepath.Join(repoRoot, ".scandrix.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var opts WizardOptions
	if err := yaml.Unmarshal(data, &opts); err != nil {
		return nil, fmt.Errorf("unmarshal yaml config: %w", err)
	}
	return &opts, nil
}

func defaultIfEmpty(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

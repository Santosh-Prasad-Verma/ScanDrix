package review

import (
	"fmt"
	"strings"
)

var validFailOnSeverities = map[string]bool{
	"info":     true,
	"warning":  true,
	"error":    true,
	"critical": true,
}

// ReviewOptions contains command-line flags and parameters for review execution.
type ReviewOptions struct {
	Interactive bool
	Fix         bool
	PromptOnly  bool
	FailOn      string
	Staged      bool
	Branch      string
	Commit      string
	Files       []string
	Format      string
	Output      string
	NoHunk      bool
	Verbose     bool
	RulesOnly   bool
	Fast        bool
	Heavy       bool
	Focus       string
	ContextFile string
	Fields      string
	GithubPAT   string
	Quiet       bool
	IsAgent     bool
	OutputFile  string
}

// ValidateReviewOptions enforces flag compatibility and validation rules.
func ValidateReviewOptions(opts ReviewOptions) error {
	if opts.Interactive && opts.PromptOnly {
		return fmt.Errorf("the `--interactive` and `--prompt-only` options cannot be used together")
	}

	if opts.Interactive && opts.Fix {
		return fmt.Errorf("the `--interactive` and `--fix` options cannot be used together")
	}

	if opts.FailOn != "" {
		normalized := strings.ToLower(strings.TrimSpace(opts.FailOn))
		if !validFailOnSeverities[normalized] {
			return fmt.Errorf("invalid value for `--fail-on`: `%s`. Use one of: info, warning, error, critical", opts.FailOn)
		}
	}

	return nil
}

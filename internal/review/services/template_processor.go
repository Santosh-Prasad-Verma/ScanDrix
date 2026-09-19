package services

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/review/domain"
)

// MessageTemplateProcessor implements domain.IMessageTemplateProcessor.
type MessageTemplateProcessor struct {
	aliases map[string]string
}

// NewMessageTemplateProcessor constructs a template processor with standard placeholder aliases.
func NewMessageTemplateProcessor() *MessageTemplateProcessor {
	return &MessageTemplateProcessor{
		aliases: map[string]string{
			"consolidatedLLMPrompt": "agentPrompt",
			"findingsCount":         "findings_count",
			"rulesChecked":          "rules_checked",
			"errorMessage":          "error_message",
		},
	}
}

var (
	curlyRegex = regexp.MustCompile(`\{\{\s*(\w+)\s*\}\}`)
	atRegex    = regexp.MustCompile(`@(\w+)`)
)

// Process replaces placeholders in template strings with actual review run values.
func (p *MessageTemplateProcessor) Process(template string, vars domain.TemplateVariables) string {
	if template == "" {
		return ""
	}

	replaceVar := func(token string) (string, bool) {
		if alias, ok := p.aliases[token]; ok {
			token = alias
		}

		switch strings.ToLower(token) {
		case "author":
			return vars.Author, true
		case "prnumber", "pr_number":
			return fmt.Sprintf("%d", vars.PRNumber), true
		case "reponame", "repo_name":
			return vars.RepoName, true
		case "summary", "changesummary":
			return vars.Summary, true
		case "findings", "agentprompt":
			return vars.FindingsList, true
		case "findingscount", "findings_count":
			return fmt.Sprintf("%d", vars.FindingsCount), true
		case "ruleschecked", "rules_checked":
			return fmt.Sprintf("%d", vars.RulesChecked), true
		case "duration", "durationsecs", "duration_secs":
			return fmt.Sprintf("%.2fs", vars.DurationSecs), true
		case "errormessage", "error_message":
			return vars.ErrorMessage, true
		default:
			return "", false
		}
	}

	// 1. Process {{ token }} placeholders
	out := curlyRegex.ReplaceAllStringFunc(template, func(match string) string {
		submatches := curlyRegex.FindStringSubmatch(match)
		if len(submatches) >= 2 {
			if val, ok := replaceVar(submatches[1]); ok {
				return val
			}
		}
		return match
	})

	// 2. Process @token placeholders
	out = atRegex.ReplaceAllStringFunc(out, func(match string) string {
		submatches := atRegex.FindStringSubmatch(match)
		if len(submatches) >= 2 {
			if val, ok := replaceVar(submatches[1]); ok {
				return val
			}
		}
		return match
	})

	return out
}

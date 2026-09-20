// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ci

import (
	"fmt"
	"io"
	"strings"

	"github.com/scandrix/backend/pkg/models"
)

// GitHubAnnotation represents a structured GitHub Actions workflow command annotation.
type GitHubAnnotation struct {
	Level     string // error, warning, notice
	File      string
	Line      int
	EndLine   int
	Title     string
	Message   string
}

// FormatWorkflowCommand formats the annotation into GitHub Actions workflow command syntax.
func (a GitHubAnnotation) FormatWorkflowCommand() string {
	var params []string
	if a.File != "" {
		params = append(params, fmt.Sprintf("file=%s", a.File))
	}
	if a.Line > 0 {
		params = append(params, fmt.Sprintf("line=%d", a.Line))
	}
	if a.EndLine > 0 && a.EndLine >= a.Line {
		params = append(params, fmt.Sprintf("endLine=%d", a.EndLine))
	}
	if a.Title != "" {
		escapedTitle := escapeWorkflowProperty(a.Title)
		params = append(params, fmt.Sprintf("title=%s", escapedTitle))
	}

	paramStr := ""
	if len(params) > 0 {
		paramStr = " " + strings.Join(params, ",")
	}

	escapedMsg := escapeWorkflowData(a.Message)
	level := strings.ToLower(a.Level)
	if level != "error" && level != "warning" && level != "notice" {
		level = "warning"
	}

	return fmt.Sprintf("::%s%s::%s", level, paramStr, escapedMsg)
}

// EmitGitHubAnnotations converts code review findings into GitHub workflow commands.
func EmitGitHubAnnotations(w io.Writer, findings []models.CodeFinding) {
	for _, f := range findings {
		level := "warning"
		sev := strings.ToLower(string(f.Severity))
		if sev == "critical" || sev == "high" || sev == "error" {
			level = "error"
		} else if sev == "low" || sev == "info" {
			level = "notice"
		}

		ann := GitHubAnnotation{
			Level:   level,
			File:    f.FilePath,
			Line:    f.StartLine,
			EndLine: f.EndLine,
			Title:   fmt.Sprintf("[ScanDrix] %s", f.Title),
			Message: f.Description,
		}

		fmt.Fprintln(w, ann.FormatWorkflowCommand())
	}
}

func escapeWorkflowProperty(s string) string {
	r := strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
		":", "%3A",
		",", "%2C",
	)
	return r.Replace(s)
}

func escapeWorkflowData(s string) string {
	r := strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
	)
	return r.Replace(s)
}

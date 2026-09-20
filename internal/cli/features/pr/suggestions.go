// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package pr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/pr"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/scandrix/backend/pkg/models"
)

// SuggestionsResult holds filtered suggestions and summary counts.
type SuggestionsResult struct {
	TotalSuggestions int                  `json:"total_suggestions"`
	Findings         []models.CodeFinding `json:"findings"`
	FilteredCount    int                  `json:"filtered_count"`
	RepositoryID     string               `json:"repository_id,omitempty"`
	PRNumber         int                  `json:"pr_number,omitempty"`
}

// FetchAndFilterSuggestions queries the PR client and filters findings.
func FetchAndFilterSuggestions(ctx context.Context, client *pr.PRClient, opts SuggestionsOptions) (*SuggestionsResult, error) {
	if client == nil {
		client = pr.NewPRClient("", "")
	}

	filterOpts := pr.SuggestionFilterOptions{
		PRURL:    opts.PRURL,
		PRNumber: opts.PRNumber,
		RepoID:   opts.RepoID,
		Format:   opts.Format,
	}

	rawFindings, err := client.FetchPRSuggestions(ctx, filterOpts)
	if err != nil {
		return nil, fmt.Errorf("failed fetching suggestions from ScanDrix API: %w", err)
	}

	severitySet := make(map[string]bool)
	for _, s := range opts.Severity {
		severitySet[strings.ToLower(s)] = true
	}

	categorySet := make(map[string]bool)
	for _, c := range opts.Category {
		categorySet[strings.ToLower(c)] = true
	}

	var filtered []models.CodeFinding
	for _, finding := range rawFindings {
		sev := strings.ToLower(string(finding.Severity))
		cat := strings.ToLower(finding.Category)

		if len(severitySet) > 0 && !severitySet[sev] {
			continue
		}
		if len(categorySet) > 0 && !categorySet[cat] {
			continue
		}
		filtered = append(filtered, finding)
	}

	return &SuggestionsResult{
		TotalSuggestions: len(rawFindings),
		Findings:         filtered,
		FilteredCount:    len(filtered),
		RepositoryID:     opts.RepoID,
		PRNumber:         opts.PRNumber,
	}, nil
}

// FormatSuggestionsOutput produces the formatted report string according to format flag.
func FormatSuggestionsOutput(res *SuggestionsResult, format string) (string, error) {
	if res == nil {
		return "", nil
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil

	case "markdown", "md":
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# Pull Request Suggestions Report\n\n"))
		sb.WriteString(fmt.Sprintf("**Total Suggestions:** %d | **Matched Filters:** %d\n\n", res.TotalSuggestions, res.FilteredCount))
		if len(res.Findings) == 0 {
			sb.WriteString("No suggestions matched the specified criteria.\n")
			return sb.String(), nil
		}

		sb.WriteString("| File | Line | Severity | Category | Description |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
		for _, f := range res.Findings {
			sb.WriteString(fmt.Sprintf("| `%s` | %d | **%s** | %s | %s |\n",
				f.FilePath, f.StartLine, string(f.Severity), f.Category, strings.ReplaceAll(f.Title, "|", "\\|")))
		}
		return sb.String(), nil

	default: // terminal
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("📋 Pull Request Suggestions (%d matching filters of %d total):\n\n", res.FilteredCount, res.TotalSuggestions))
		if len(res.Findings) == 0 {
			sb.WriteString("  No suggestions matched the specified criteria.\n")
			return sb.String(), nil
		}

		for i, f := range res.Findings {
			sb.WriteString(fmt.Sprintf("  %d. [%s] %s:%d\n", i+1, string(f.Severity), f.FilePath, f.StartLine))
			sb.WriteString(fmt.Sprintf("     Title: %s\n", f.Title))
			if f.Description != "" {
				sb.WriteString(fmt.Sprintf("     Details: %s\n", f.Description))
			}
			if f.SuggestedDiff != "" {
				sb.WriteString("     Suggested Fix:\n")
				lines := strings.Split(f.SuggestedDiff, "\n")
				for _, l := range lines {
					sb.WriteString(fmt.Sprintf("       + %s\n", l))
				}
			}
			sb.WriteString("\n")
		}
		return sb.String(), nil
	}
}

// ExecuteSuggestionsAction runs the suggestions query and presentation flow.
func ExecuteSuggestionsAction(ctx context.Context, client *pr.PRClient, opts SuggestionsOptions) error {
	startTime := time.Now().UTC()

	if err := ValidateSuggestionsOptions(opts); err != nil {
		return err
	}

	res, err := FetchAndFilterSuggestions(ctx, client, opts)
	if err != nil {
		return err
	}

	if opts.IsAgent {
		envelope := utils.BuildAgentSuccessEnvelope("pr suggestions", res, startTime)
		return utils.EmitAgentEnvelope(envelope, opts.OutputFile)
	}

	out, err := FormatSuggestionsOutput(res, opts.Format)
	if err != nil {
		return fmt.Errorf("failed formatting output: %w", err)
	}

	if opts.Output != "" {
		if err := os.WriteFile(opts.Output, []byte(out), 0644); err != nil {
			return fmt.Errorf("failed writing output file %s: %w", opts.Output, err)
		}
		if !opts.Quiet {
			utils.Success("Output written to %s", opts.Output)
		}
	} else {
		fmt.Println(out)
	}

	return nil
}

// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package engine

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// ANSI color codes.
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorYellow = "\033[33m"
	ColorCyan   = "\033[36m"
	ColorGreen  = "\033[32m"
	ColorBold   = "\033[1m"
	ColorDim    = "\033[2m"
)

// OutputFormatter renders review results into diverse human- and machine-readable formats.
type OutputFormatter struct{}

// NewOutputFormatter initializes the renderer.
func NewOutputFormatter() *OutputFormatter {
	return &OutputFormatter{}
}

// Render writes the formatted results according to specified format.
func (f *OutputFormatter) Render(w io.Writer, res *CLIResult, format OutputFormat) error {
	return f.RenderWithFields(w, res, format, "")
}

// RenderWithFields writes the formatted results and applies field masking if provided.
func (f *OutputFormatter) RenderWithFields(w io.Writer, res *CLIResult, format OutputFormat, fieldsCSV string) error {
	switch format {
	case FormatJSON:
		return f.renderJSON(w, res, fieldsCSV)
	case FormatSARIF:
		return f.renderSARIF(w, res)
	case FormatCSV:
		return f.renderCSV(w, res)
	case FormatMarkdown:
		return f.renderMarkdown(w, res)
	case FormatAgent:
		return f.renderAgent(w, res, fieldsCSV)
	case FormatPrompt:
		return f.renderPrompt(w, res)
	default:
		return f.renderTable(w, res)
	}
}

func (f *OutputFormatter) renderMarkdown(w io.Writer, res *CLIResult) error {
	fmt.Fprintln(w, "## 🛡️ ScanDrix Automated Security & Quality Review")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "**Summary**: Reviewed `%d` files in `%s`.\n\n", res.FilesReviewed, res.Duration.Round(time.Millisecond))

	if len(res.Findings) == 0 {
		fmt.Fprintln(w, "> ✅ **PASSED**: No security vulnerabilities or policy violations discovered. Clean review!")
		return nil
	}

	fmt.Fprintln(w, "| Severity | Category | Finding | Location |")
	fmt.Fprintln(w, "|:---|:---|:---|:---|")
	for _, finding := range res.Findings {
		icon := "🟡"
		switch finding.Severity {
		case models.SeverityCritical:
			icon = "🔴"
		case models.SeverityHigh:
			icon = "🟠"
		case models.SeverityLow:
			icon = "🔵"
		}
		fmt.Fprintf(w, "| %s **%s** | `%s` | %s | [`%s:%d`](#) |\n",
			icon, finding.Severity, finding.Category, finding.Title, finding.FilePath, finding.StartLine)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "### Detailed Findings & Remediations")
	fmt.Fprintln(w)
	for i, finding := range res.Findings {
		fmt.Fprintf(w, "<details>\n<summary><b>%d. [%s] %s</b> (<code>%s:%d</code>)</summary>\n\n",
			i+1, finding.Severity, finding.Title, finding.FilePath, finding.StartLine)
		fmt.Fprintf(w, "**Description**:\n%s\n\n", finding.Description)
		if finding.Remediation != "" {
			fmt.Fprintf(w, "**Remediation**:\n%s\n\n", finding.Remediation)
		}
		if finding.SuggestedDiff != "" {
			fmt.Fprintln(w, "**Suggested Fix**:")
			fmt.Fprintln(w, "```diff")
			fmt.Fprintln(w, finding.SuggestedDiff)
			fmt.Fprintln(w, "```")
		}
		fmt.Fprintln(w, "</details>")
		fmt.Fprintln(w)
	}

	if res.IsBlocking {
		fmt.Fprintln(w, "> ❌ **ACTION REQUIRED**: Critical or High severity findings exceed the workspace block threshold.")
	}

	return nil
}

func (f *OutputFormatter) renderPrompt(w io.Writer, res *CLIResult) error {
	fmt.Fprintln(w, "# ScanDrix Autonomous Review Remediation Context")
	fmt.Fprintln(w, "Apply the following remediations directly to the identified code locations:")
	fmt.Fprintln(w)

	for i, finding := range res.Findings {
		fmt.Fprintf(w, "--- Finding %d ---\n", i+1)
		fmt.Fprintf(w, "File: %s:%d\n", finding.FilePath, finding.StartLine)
		fmt.Fprintf(w, "Severity: %s\n", finding.Severity)
		fmt.Fprintf(w, "Issue: %s - %s\n", finding.Title, finding.Description)
		if finding.Remediation != "" {
			fmt.Fprintf(w, "Remediation: %s\n", finding.Remediation)
		}
		if finding.SuggestedDiff != "" {
			fmt.Fprintf(w, "Suggested Fix:\n%s\n", finding.SuggestedDiff)
		}
		fmt.Fprintln(w)
	}

	return nil
}

func (f *OutputFormatter) renderAgent(w io.Writer, res *CLIResult, fieldsCSV string) error {
	type AgentFinding struct {
		FilePath      string `json:"file_path"`
		StartLine     int    `json:"start_line"`
		EndLine       int    `json:"end_line"`
		Severity      string `json:"severity"`
		Category      string `json:"category"`
		Title         string `json:"title"`
		Description   string `json:"description"`
		Remediation   string `json:"remediation,omitempty"`
		SuggestedDiff string `json:"suggested_diff,omitempty"`
	}

	type AgentOutput struct {
		Status        string         `json:"status"` // "passed" or "failed"
		FilesReviewed int            `json:"files_reviewed"`
		TotalFindings int            `json:"total_findings"`
		CriticalCount int            `json:"critical_count"`
		HighCount     int            `json:"high_count"`
		MediumCount   int            `json:"medium_count"`
		LowCount      int            `json:"low_count"`
		IsBlocking    bool           `json:"is_blocking"`
		DurationMs    int64          `json:"duration_ms"`
		Findings      []AgentFinding `json:"findings"`
	}

	agentFindings := make([]AgentFinding, 0, len(res.Findings))
	for _, finding := range res.Findings {
		agentFindings = append(agentFindings, AgentFinding{
			FilePath:      finding.FilePath,
			StartLine:     finding.StartLine,
			EndLine:       finding.EndLine,
			Severity:      string(finding.Severity),
			Category:      finding.Category,
			Title:         finding.Title,
			Description:   finding.Description,
			Remediation:   finding.Remediation,
			SuggestedDiff: finding.SuggestedDiff,
		})
	}

	status := "passed"
	if res.IsBlocking || res.CriticalCount > 0 || res.HighCount > 0 {
		status = "failed"
	}

	out := AgentOutput{
		Status:        status,
		FilesReviewed: res.FilesReviewed,
		TotalFindings: res.TotalFindings,
		CriticalCount: res.CriticalCount,
		HighCount:     res.HighCount,
		MediumCount:   res.MediumCount,
		LowCount:      res.LowCount,
		IsBlocking:    res.IsBlocking,
		DurationMs:    res.Duration.Milliseconds(),
		Findings:      agentFindings,
	}

	if fieldsCSV != "" {
		filtered := ApplyFieldMask(out, fieldsCSV)
		encoder := json.NewEncoder(w)
		return encoder.Encode(filtered)
	}

	encoder := json.NewEncoder(w)
	return encoder.Encode(out)
}

func (f *OutputFormatter) renderTable(w io.Writer, res *CLIResult) error {
	fmt.Fprintln(w, "\n"+ColorBold+"=== ScanDrix Security & Review Summary ==="+ColorReset)
	fmt.Fprintf(w, "Files Reviewed: %d | Duration: %s\n", res.FilesReviewed, res.Duration.Round(100*time.Millisecond))
	fmt.Fprintf(w, "Total Findings: %d (🔴 Critical: %d, 🟠 High: %d, 🟡 Medium: %d, 🔵 Low: %d)\n\n",
		res.TotalFindings, res.CriticalCount, res.HighCount, res.MediumCount, res.LowCount)

	if len(res.Findings) == 0 {
		fmt.Fprintln(w, ColorGreen+"✔ No security or quality vulnerabilities discovered. Clean review!"+ColorReset)
		return nil
	}

	for i, finding := range res.Findings {
		color := ColorYellow
		badge := "🟡 MEDIUM"
		if finding.Severity == models.SeverityCritical {
			color = ColorRed
			badge = "🔴 CRITICAL"
		} else if finding.Severity == models.SeverityHigh {
			color = ColorYellow
			badge = "🟠 HIGH"
		} else if finding.Severity == models.SeverityLow {
			badge = "🔵 LOW"
			color = ColorCyan
		}

		fmt.Fprintf(w, "%s[%d] %s: %s%s\n", color, i+1, badge, finding.Title, ColorReset)
		fmt.Fprintf(w, "  File: %s:%d\n", finding.FilePath, finding.StartLine)
		fmt.Fprintf(w, "  Category: %s\n", finding.Category)
		if finding.Remediation != "" {
			fmt.Fprintf(w, "  Remediation: %s\n", finding.Remediation)
		}
		if finding.SuggestedDiff != "" {
			fmt.Fprintf(w, "  %sSuggested Diff:%s\n%s\n", ColorDim, ColorReset, finding.SuggestedDiff)
		}
		fmt.Fprintln(w, "  --------------------------------------------------")
	}

	if res.IsBlocking {
		fmt.Fprintln(w, ColorRed+ColorBold+"\n✖ REVIEW FAILED: Critical/High vulnerabilities violate workspace security threshold."+ColorReset)
	} else {
		fmt.Fprintln(w, ColorGreen+ColorBold+"\n✔ REVIEW PASSED: No threshold violations detected."+ColorReset)
	}

	return nil
}

func (f *OutputFormatter) renderJSON(w io.Writer, res *CLIResult, fieldsCSV string) error {
	if fieldsCSV != "" {
		filtered := ApplyFieldMask(res, fieldsCSV)
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(filtered)
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(res)
}

func (f *OutputFormatter) renderSARIF(w io.Writer, res *CLIResult) error {
	sarifResults := make([]SARIFResult, 0, len(res.Findings))

	for _, finding := range res.Findings {
		level := "warning"
		if finding.Severity == models.SeverityCritical || finding.Severity == models.SeverityHigh {
			level = "error"
		} else if finding.Severity == models.SeverityLow {
			level = "note"
		}

		sarifResults = append(sarifResults, SARIFResult{
			RuleID: finding.Title,
			Level:  level,
			Message: SARIFMessage{
				Text: fmt.Sprintf("%s: %s", finding.Description, finding.Remediation),
			},
			Locations: []SARIFLocation{
				{
					PhysicalLocation: SARIFPhysicalLocation{
						ArtifactLocation: SARIFArtifactLocation{
							URI: finding.FilePath,
						},
						Region: SARIFRegion{
							StartLine: finding.StartLine,
						},
					},
				},
			},
		})
	}

	sarif := SARIFLog{
		Version: "2.1.0",
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		Runs: []SARIFRun{
			{
				Tool: SARIFTool{
					Driver: SARIFDriver{
						Name:           "ScanDrix CLI",
						Version:        "1.2.0",
						InformationURI: "https://scandrix.dev",
					},
				},
				Results: sarifResults,
			},
		},
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(sarif)
}

// ApplyFieldMask filters an arbitrary struct/map using a comma-separated list of field names.
func ApplyFieldMask(data any, fieldsCSV string) any {
	if strings.TrimSpace(fieldsCSV) == "" {
		return data
	}

	rawBytes, err := json.Marshal(data)
	if err != nil {
		return data
	}

	var root any
	if err := json.Unmarshal(rawBytes, &root); err != nil {
		return data
	}

	fields := strings.Split(fieldsCSV, ",")
	fieldMap := make(map[string]bool)
	for _, f := range fields {
		trimmed := strings.TrimSpace(f)
		if trimmed != "" {
			fieldMap[trimmed] = true
		}
	}

	if len(fieldMap) == 0 {
		return data
	}

	return filterNode(root, "", fieldMap)
}

func filterNode(node any, currentPath string, fieldMap map[string]bool) any {
	switch v := node.(type) {
	case map[string]any:
		res := make(map[string]any)
		for k, val := range v {
			fullPath := k
			if currentPath != "" {
				fullPath = currentPath + "." + k
			}

			// Exact match or prefix match
			matched := fieldMap[fullPath] || fieldMap[k]
			if !matched {
				for f := range fieldMap {
					if strings.HasPrefix(f, fullPath+".") || strings.HasPrefix(f, k+".") {
						matched = true
						break
					}
				}
			}

			if matched {
				res[k] = filterNode(val, fullPath, fieldMap)
			}
		}
		return res

	case []any:
		res := make([]any, 0, len(v))
		for _, item := range v {
			res = append(res, filterNode(item, currentPath, fieldMap))
		}
		return res

	default:
		return v
	}
}

func (f *OutputFormatter) renderCSV(w io.Writer, res *CLIResult) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	header := []string{"ID", "Severity", "Category", "File", "StartLine", "EndLine", "Title", "Description", "Remediation"}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, finding := range res.Findings {
		row := []string{
			finding.ID.String(),
			string(finding.Severity),
			finding.Category,
			finding.FilePath,
			fmt.Sprintf("%d", finding.StartLine),
			fmt.Sprintf("%d", finding.EndLine),
			finding.Title,
			finding.Description,
			finding.Remediation,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	return nil
}

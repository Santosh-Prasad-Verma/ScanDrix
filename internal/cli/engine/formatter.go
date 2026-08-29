package engine

import (
	"encoding/json"
	"fmt"
	"io"

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
)

// OutputFormatter renders review results into diverse human- and machine-readable formats.
type OutputFormatter struct{}

// NewOutputFormatter initializes the renderer.
func NewOutputFormatter() *OutputFormatter {
	return &OutputFormatter{}
}

// Render writes the formatted results according to specified format.
func (f *OutputFormatter) Render(w io.Writer, res *CLIResult, format OutputFormat) error {
	switch format {
	case FormatJSON:
		return f.renderJSON(w, res)
	case FormatSARIF:
		return f.renderSARIF(w, res)
	default:
		return f.renderTable(w, res)
	}
}

func (f *OutputFormatter) renderTable(w io.Writer, res *CLIResult) error {
	fmt.Fprintln(w, "\n"+ColorBold+"=== ScanDrix Code Review Summary ==="+ColorReset)
	fmt.Fprintf(w, "Files Reviewed: %d | Duration: %s\n", res.FilesReviewed, res.Duration.Round(100))
	fmt.Fprintf(w, "Total Findings: %d (Critical: %d, High: %d, Medium: %d, Low: %d)\n\n",
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
		}

		fmt.Fprintf(w, "%s[%d] %s: %s%s\n", color, i+1, badge, finding.Title, ColorReset)
		fmt.Fprintf(w, "  File: %s:%d\n", finding.FilePath, finding.StartLine)
		fmt.Fprintf(w, "  Category: %s\n", finding.Category)
		fmt.Fprintf(w, "  Description: %s\n", finding.Description)
		if finding.Remediation != "" {
			fmt.Fprintf(w, "  Remediation: %s\n", finding.Remediation)
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

func (f *OutputFormatter) renderJSON(w io.Writer, res *CLIResult) error {
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
							EndLine:   finding.EndLine,
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
						Version:        "1.0.0",
						InformationURI: "https://scandrix.io",
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

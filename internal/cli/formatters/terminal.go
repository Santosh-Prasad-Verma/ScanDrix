package formatters

import (
	"fmt"
	"io"
	"strings"

	"github.com/scandrix/backend/internal/cli/types"
)

// ANSI Color and Styling constants
const (
	AnsiReset     = "\033[0m"
	AnsiBold      = "\033[1m"
	AnsiDim       = "\033[2m"
	AnsiUnderline = "\033[4m"
	AnsiRed       = "\033[31m"
	AnsiGreen     = "\033[32m"
	AnsiYellow    = "\033[33m"
	AnsiBlue      = "\033[34m"
	AnsiMagenta   = "\033[35m"
	AnsiCyan      = "\033[36m"
	AnsiWhite     = "\033[37m"
	AnsiGray      = "\033[90m"
	AnsiBrightRed = "\033[91m"
	AnsiBgRed     = "\033[41m"
	AnsiBgYellow  = "\033[43m"
	AnsiBgGreen   = "\033[42m"
)

// TerminalFormatter renders ScanDrix review findings into rich CLI terminal output.
type TerminalFormatter struct {
	NoColor bool
	Verbose bool
}

// NewTerminalFormatter creates a new TerminalFormatter instance.
func NewTerminalFormatter(noColor bool, verbose bool) *TerminalFormatter {
	return &TerminalFormatter{
		NoColor: noColor,
		Verbose: verbose,
	}
}

func (f *TerminalFormatter) colorize(color string, text string) string {
	if f.NoColor {
		return text
	}
	return color + text + AnsiReset
}

func (f *TerminalFormatter) getSeverityColor(sev types.Severity) string {
	switch sev {
	case types.SeverityCritical:
		return AnsiBrightRed + AnsiBold
	case types.SeverityError:
		return AnsiRed
	case types.SeverityWarning:
		return AnsiYellow
	case types.SeverityInfo:
		return AnsiCyan
	default:
		return AnsiWhite
	}
}

func (f *TerminalFormatter) getSeverityIcon(sev types.Severity) string {
	switch sev {
	case types.SeverityCritical:
		return "🔴"
	case types.SeverityError:
		return "❌"
	case types.SeverityWarning:
		return "⚠️ "
	case types.SeverityInfo:
		return "ℹ️ "
	default:
		return "•"
	}
}

// FormatIssue formats a single review issue into readable terminal blocks.
func (f *TerminalFormatter) FormatIssue(issue types.ReviewIssue, index int) string {
	var sb strings.Builder

	color := f.getSeverityColor(issue.Severity)
	icon := f.getSeverityIcon(issue.Severity)
	sevLabel := strings.ToUpper(string(issue.Severity))

	fixableBadge := ""
	if issue.Fixable {
		fixableBadge = " " + f.colorize(AnsiGreen+AnsiBold, "[fixable]")
	}

	// Line 1: Header with index, severity, file path, and line number
	indexPrefix := f.colorize(AnsiDim, fmt.Sprintf("%d.", index+1))
	loc := f.colorize(AnsiCyan, issue.File)
	if issue.Line > 0 {
		loc += f.colorize(AnsiDim, fmt.Sprintf(":%d", issue.Line))
	}

	sb.WriteString(fmt.Sprintf("%s %s %s %s %s%s\n",
		indexPrefix,
		icon,
		f.colorize(color, sevLabel),
		f.colorize(AnsiDim, "in"),
		loc,
		fixableBadge,
	))

	// Line 2: Category
	if issue.Category != "" {
		sb.WriteString(fmt.Sprintf("   %s\n", f.colorize(AnsiMagenta, fmt.Sprintf("[%s]", issue.Category))))
	}

	// Line 3: Finding Message
	sb.WriteString(fmt.Sprintf("   %s\n", issue.Message))

	// Line 4: Suggestion / Recommendation
	if issue.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("   %s %s\n",
			f.colorize(AnsiDim, "💡 Suggestion:"),
			f.colorize(AnsiGreen, issue.Suggestion),
		))
	} else if issue.Recommendation != "" {
		sb.WriteString(fmt.Sprintf("   %s %s\n",
			f.colorize(AnsiDim, "💡 Recommendation:"),
			f.colorize(AnsiCyan, issue.Recommendation),
		))
	}

	// Line 5: Fix Preview
	if issue.Fixable && issue.Fix != nil {
		sb.WriteString(fmt.Sprintf("   %s\n", f.colorize(AnsiGreen, "✓ Auto-fix available")))
		if issue.Fix.NewCode != "" {
			firstLine := strings.Split(issue.Fix.NewCode, "\n")[0]
			if len(firstLine) > 80 {
				firstLine = firstLine[:77] + "..."
			}
			sb.WriteString(fmt.Sprintf("   %s %s\n",
				f.colorize(AnsiDim, "Fix preview:"),
				f.colorize(AnsiGreen, firstLine),
			))
		}
	}

	// Line 6: Hunk Context if present
	if f.Verbose && issue.HunkContext != "" {
		sb.WriteString(fmt.Sprintf("   %s\n", f.colorize(AnsiDim, "── Context ─────────────────────")))
		for _, l := range strings.Split(issue.HunkContext, "\n") {
			if strings.HasPrefix(l, "+") {
				sb.WriteString(fmt.Sprintf("   %s\n", f.colorize(AnsiGreen, l)))
			} else if strings.HasPrefix(l, "-") {
				sb.WriteString(fmt.Sprintf("   %s\n", f.colorize(AnsiRed, l)))
			} else {
				sb.WriteString(fmt.Sprintf("   %s\n", f.colorize(AnsiDim, l)))
			}
		}
		sb.WriteString(fmt.Sprintf("   %s\n", f.colorize(AnsiDim, "────────────────────────────────")))
	}

	// Line 7: Rule Identifier
	if issue.RuleID != "" {
		sb.WriteString(fmt.Sprintf("   %s %s\n",
			f.colorize(AnsiDim, "Rule:"),
			f.colorize(AnsiDim+AnsiUnderline, issue.RuleID),
		))
	}

	return sb.String()
}

// Format formats the entire ReviewResult into a terminal report.
func (f *TerminalFormatter) Format(w io.Writer, result *types.ReviewResult) error {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(f.colorize(AnsiBold+AnsiCyan, "ScanDrix Code Review Report\n"))
	sb.WriteString(f.colorize(AnsiDim, strings.Repeat("═", 50)) + "\n\n")

	// Summary statistics header
	sb.WriteString(fmt.Sprintf("Status:         %s\n", f.formatStatus(result.Status)))
	sb.WriteString(fmt.Sprintf("Files Analyzed: %d\n", result.FilesAnalyzed))
	if result.DurationMs > 0 {
		sb.WriteString(fmt.Sprintf("Duration:       %dms\n", result.DurationMs))
	}

	if result.GitContext != nil && result.GitContext.Branch != "" {
		sb.WriteString(fmt.Sprintf("Branch:         %s (%s)\n",
			f.colorize(AnsiCyan, result.GitContext.Branch),
			f.colorize(AnsiDim, result.GitContext.HeadSHA[:min(8, len(result.GitContext.HeadSHA))]),
		))
	}

	sb.WriteString("\n")

	// Issues List
	if len(result.Issues) == 0 {
		sb.WriteString(f.colorize(AnsiGreen+AnsiBold, "✓ No issues found. Clean code!\n\n"))
	} else {
		sb.WriteString(f.colorize(AnsiBold, fmt.Sprintf("Findings (%d):\n\n", len(result.Issues))))
		for i, issue := range result.Issues {
			sb.WriteString(f.FormatIssue(issue, i))
			sb.WriteString("\n")
		}
	}

	// Breakdown Table
	sb.WriteString(f.formatSummaryTable(result))

	_, err := io.WriteString(w, sb.String())
	return err
}

func (f *TerminalFormatter) formatStatus(status string) string {
	switch strings.ToLower(status) {
	case "passed":
		return f.colorize(AnsiGreen+AnsiBold, "PASSED")
	case "failed":
		return f.colorize(AnsiRed+AnsiBold, "FAILED")
	case "warning":
		return f.colorize(AnsiYellow+AnsiBold, "WARNING")
	default:
		return status
	}
}

func (f *TerminalFormatter) formatSummaryTable(result *types.ReviewResult) string {
	var sb strings.Builder

	sb.WriteString(f.colorize(AnsiDim, "─ Summary ───────────────────────────────────────────\n"))
	sb.WriteString(fmt.Sprintf("  🔴 Critical: %-4d   ❌ Error:   %-4d   ⚠️  Warning: %-4d\n",
		result.Stats.CriticalCount, result.Stats.ErrorCount, result.Stats.WarningCount))
	sb.WriteString(fmt.Sprintf("  ℹ️  Info:     %-4d   ✓ Fixable: %-4d   Total:      %-4d\n",
		result.Stats.InfoCount, result.Stats.FixableCount, result.Stats.TotalIssues))
	sb.WriteString(f.colorize(AnsiDim, "─────────────────────────────────────────────────────\n\n"))

	return sb.String()
}

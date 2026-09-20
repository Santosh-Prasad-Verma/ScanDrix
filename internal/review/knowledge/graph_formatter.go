// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package knowledge

import (
	"fmt"
	"strings"
)

// GraphFormatter renders knowledge graph relationships and blast radius reports into markdown and prompts.
type GraphFormatter struct{}

// NewGraphFormatter constructs a new graph formatter.
func NewGraphFormatter() *GraphFormatter {
	return &GraphFormatter{}
}

// FormatBlastRadiusMarkdown renders an executive summary of the blast radius for PR descriptions or review summaries.
func (f *GraphFormatter) FormatBlastRadiusMarkdown(report *BlastRadiusReport) string {
	if report == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### 🌐 Architectural Blast Radius Analysis\n\n")

	var badge string
	switch report.RiskRating {
	case RiskCritical:
		badge = "🔴 **CRITICAL RISK**"
	case RiskHigh:
		badge = "🟠 **HIGH RISK**"
	case RiskMedium:
		badge = "🟡 **MEDIUM RISK**"
	default:
		badge = "🟢 **LOW RISK**"
	}

	sb.WriteString(fmt.Sprintf("- **Change Impact Rating**: %s (Score: `%.2f`)\n", badge, report.ImpactScore))
	sb.WriteString(fmt.Sprintf("- **Total Impacted Files**: **%d** (%d directly modified, %d Tier-1 callers, %d Tier-2 downstream)\n",
		report.TotalImpactedFiles, len(report.DirectlyModifiedFiles), len(report.Tier1ImpactedFiles), len(report.Tier2ImpactedFiles)))

	if len(report.DirectlyModifiedSymbols) > 0 {
		sb.WriteString(fmt.Sprintf("- **Directly Altered Symbols**: `%s`\n", strings.Join(report.DirectlyModifiedSymbols, "`, `")))
	}

	if len(report.Tier1ImpactedSymbols) > 0 {
		sb.WriteString(fmt.Sprintf("- **Direct Callers At Risk (Tier 1)**: `%s`\n", strings.Join(report.Tier1ImpactedSymbols, "`, `")))
	}

	if len(report.Tier1ImpactedFiles) > 0 {
		sb.WriteString("\n**Tier 1 Dependent Files**:\n")
		for _, file := range report.Tier1ImpactedFiles {
			sb.WriteString(fmt.Sprintf("- `%s`\n", file))
		}
	}

	return sb.String()
}

// FormatMermaidDiagram generates a Mermaid graph definition representing the call topology.
func (f *GraphFormatter) FormatMermaidDiagram(report *BlastRadiusReport, graph *CallGraph) string {
	if report == nil || graph == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("```mermaid\ngraph TD\n")

	// Directly modified symbols highlighted with red/orange styling
	for i, sym := range report.DirectlyModifiedSymbols {
		nodeID := fmt.Sprintf("M%d", i)
		sb.WriteString(fmt.Sprintf("    %s[\"%s (Modified)\"]:::modified\n", nodeID, sym))
	}

	// Direct callers
	for i, sym := range report.Tier1ImpactedSymbols {
		nodeID := fmt.Sprintf("T1_%d", i)
		sb.WriteString(fmt.Sprintf("    %s[\"%s (Caller)\"]:::caller\n", nodeID, sym))
	}

	// Add edges between callers and modified symbols
	for _, sym := range report.DirectlyModifiedSymbols {
		callers := graph.GetCallers(sym)
		for _, edge := range callers {
			sb.WriteString(fmt.Sprintf("    \"%s\" --> \"%s\"\n", edge.CallerName, edge.CalleeName))
		}
	}

	sb.WriteString("    classDef modified fill:#ffebee,stroke:#c62828,stroke-width:2px;\n")
	sb.WriteString("    classDef caller fill:#e8f5e9,stroke:#2e7d32,stroke-width:1px;\n")
	sb.WriteString("```\n")

	return sb.String()
}

// FormatForReviewerPrompt formats a dense, token-budgeted prompt slice describing cross-file caller context.
func (f *GraphFormatter) FormatForReviewerPrompt(report *BlastRadiusReport, maxTokens int) string {
	if report == nil || (len(report.DirectlyModifiedSymbols) == 0 && len(report.Tier1ImpactedSymbols) == 0) {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("=== CROSS-FILE KNOWLEDGE GRAPH & CALL CONTEXT ===\n")
	sb.WriteString(fmt.Sprintf("Risk Rating: %s (Impact Score: %.2f, Total Files: %d)\n",
		report.RiskRating, report.ImpactScore, report.TotalImpactedFiles))

	if len(report.DirectlyModifiedSymbols) > 0 {
		sb.WriteString(fmt.Sprintf("Modified Core Symbols: %s\n", strings.Join(report.DirectlyModifiedSymbols, ", ")))
	}

	if len(report.Tier1ImpactedSymbols) > 0 {
		sb.WriteString(fmt.Sprintf("Direct Inbound Callers at Risk: %s\n", strings.Join(report.Tier1ImpactedSymbols, ", ")))
	}

	if len(report.Tier1ImpactedFiles) > 0 {
		sb.WriteString(fmt.Sprintf("Direct Inbound Caller Files: %s\n", strings.Join(report.Tier1ImpactedFiles, ", ")))
	}
	sb.WriteString("Reviewers MUST verify that modifications to the core symbols do not break contracts expected by the listed inbound callers.\n")
	sb.WriteString("==================================================\n")

	text := sb.String()
	// Quick clamp to maxTokens (~4 chars per token)
	charLimit := maxTokens * 4
	if charLimit > 0 && len(text) > charLimit {
		return text[:charLimit] + "\n[Context Truncated for Budget]...\n"
	}

	return text
}

// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Business Rules Validation Agent
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package businessrules

import (
	"fmt"
	"strings"
)

// RequiredMcpInfo holds metadata about an MCP integration requirement.
type RequiredMcpInfo struct {
	Category string `json:"category"`
	Label    string `json:"label"`
	Examples string `json:"examples,omitempty"`
}

// RequiredMcpFeedbackParams parameters for generating required MCP feedback.
type RequiredMcpFeedbackParams struct {
	RequiredMcps       []RequiredMcpInfo
	UserLanguage       string
	AvailableProviders []string
}

// McpConnectionFailureFeedbackParams parameters for connection failure feedback.
type McpConnectionFailureFeedbackParams struct {
	UserLanguage       string
	AvailableProviders []string
}

// BuildRequiredMcpFeedback builds markdown feedback when required MCP providers are missing.
func BuildRequiredMcpFeedback(params RequiredMcpFeedbackParams) string {
	var requiredLabels string
	var requiredList string

	if len(params.RequiredMcps) > 0 {
		var labels []string
		var items []string
		for _, mcp := range params.RequiredMcps {
			labels = append(labels, mcp.Label)
			if mcp.Examples != "" {
				items = append(items, fmt.Sprintf("- **%s** (%s)", mcp.Label, mcp.Examples))
			} else {
				items = append(items, fmt.Sprintf("- **%s**", mcp.Label))
			}
		}
		requiredLabels = strings.Join(labels, ", ")
		requiredList = strings.Join(items, "\n")
	} else {
		requiredLabels = "Task Management"
		requiredList = "- **Task Management** (Jira, Linear, Notion)"
	}

	available := formatAvailableProviders(params.AvailableProviders)
	summary := fmt.Sprintf("MCP integration required: no compatible provider is connected. Required: %s. Available: %s.", requiredLabels, available)

	return fmt.Sprintf(`%s

## 🔌 MCP Integration Required

Business validation compares the PR implementation with task/ticket requirements.
I could not fetch task context because no compatible MCP integration is currently connected.

### Required integrations
%s

### Detected MCP providers
- %s

### Next steps
- Connect at least one MCP provider from the required categories in organization/repository settings.
- Ensure the provider is healthy and authenticated (OAuth/token/scopes).
- Re-run business validation after the connection is active.`, summary, requiredList, available)
}

// BuildMcpConnectionFailureFeedback builds markdown feedback when MCP connection fails.
func BuildMcpConnectionFailureFeedback(params McpConnectionFailureFeedbackParams) string {
	available := formatAvailableProviders(params.AvailableProviders)
	summary := fmt.Sprintf("MCP connection failed: connected providers did not expose the tools this skill needs. Available: %s.", available)

	return fmt.Sprintf(`%s

## ⚠️ MCP Connection Failed

MCP integrations are configured, but I couldn't connect to any provider right now.

### Detected MCP providers
- %s

### Next steps
- Check whether the MCP provider/server is online and healthy.
- Review OAuth/credentials (token, client, scopes, expiration).
- Confirm integration base URL and protocol.
- Re-run business validation.`, summary, available)
}

func formatAvailableProviders(providers []string) string {
	if len(providers) > 0 {
		return strings.Join(providers, ", ")
	}
	return "none"
}

// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package providers

import (
	"strings"

	"github.com/scandrix/backend/internal/mcp/manager/models"
)

// DefaultReadOnlyToolSlugs filters tools prioritizing safe read-only operations for automated code review agents.
func DefaultReadOnlyToolSlugs(tools []models.MCPTool) []string {
	var readOnlySlugs []string
	var allSlugs []string

	for _, tool := range tools {
		allSlugs = append(allSlugs, tool.Slug)
		if tool.ReadOnly {
			readOnlySlugs = append(readOnlySlugs, tool.Slug)
		}
	}

	if len(readOnlySlugs) > 0 {
		return readOnlySlugs
	}
	return allSlugs
}

// WarningKeywords contains destructive operation triggers.
var WarningKeywords = []string{
	"delete", "remove", "archive", "destroy", "drop",
	"clear", "erase", "purge", "terminate", "kill",
	"stop", "disable", "suspend", "revoke", "cancel",
	"reject", "deny", "block", "ban", "uninstall",
	"reset", "revert", "undo", "rollback", "flush",
	"wipe", "truncate",
}

// HasWarning tests if a tool name/slug contains dangerous or destructive operations.
func HasWarning(toolName string) bool {
	lower := strings.ToLower(toolName)
	for _, kw := range WarningKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

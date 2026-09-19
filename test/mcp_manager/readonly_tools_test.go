// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp_manager_test

import (
	"testing"

	"github.com/scandrix/backend/internal/mcp/manager/models"
	"github.com/scandrix/backend/internal/mcp/manager/providers"
)

func TestDefaultReadOnlyToolSlugs(t *testing.T) {
	// Case 1: Server provides read-only annotations
	toolsWithReadOnly := []models.MCPTool{
		{Slug: "read_file", ReadOnly: true},
		{Slug: "get_pull_request", ReadOnly: true},
		{Slug: "delete_repository", ReadOnly: false},
		{Slug: "merge_pull_request", ReadOnly: false},
	}

	slugs := providers.DefaultReadOnlyToolSlugs(toolsWithReadOnly)
	if len(slugs) != 2 {
		t.Fatalf("Expected 2 read-only slugs, got %d: %v", len(slugs), slugs)
	}
	if slugs[0] != "read_file" || slugs[1] != "get_pull_request" {
		t.Fatalf("Unexpected slugs: %v", slugs)
	}

	// Case 2: Server provides NO read-only annotations (fallback to all)
	toolsWithoutReadOnly := []models.MCPTool{
		{Slug: "tool_alpha", ReadOnly: false},
		{Slug: "tool_beta", ReadOnly: false},
	}

	allSlugs := providers.DefaultReadOnlyToolSlugs(toolsWithoutReadOnly)
	if len(allSlugs) != 2 {
		t.Fatalf("Expected 2 fallback slugs, got %d", len(allSlugs))
	}
}

func TestHasWarning(t *testing.T) {
	dangerous := []string{
		"delete_drixy_rule",
		"purge_cache",
		"drop_database_table",
		"remove_user_permission",
		"kill_process",
		"rollback_migration",
	}

	for _, name := range dangerous {
		if !providers.HasWarning(name) {
			t.Fatalf("Expected warning for dangerous tool: %s", name)
		}
	}

	safe := []string{
		"get_repository_files",
		"list_pull_requests",
		"review_patch_diff",
		"find_memories",
	}

	for _, name := range safe {
		if providers.HasWarning(name) {
			t.Fatalf("Expected NO warning for safe tool: %s", name)
		}
	}
}

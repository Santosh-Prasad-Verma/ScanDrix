// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package scandrixmcp

import (
	"github.com/scandrix/backend/internal/mcp/manager/models"
)

// DefaultPrimaryIntegrationID represents the canonical identifier of the built-in ScanDrix MCP integration.
const DefaultPrimaryIntegrationID = "sd_mcp_primary_default"

// DefaultScandrixMCPIntegration returns the root built-in ScanDrix MCP integration descriptor.
func DefaultScandrixMCPIntegration() models.MCPIntegration {
	tools := DefaultScandrixToolsCatalog()
	slugs := make([]string, len(tools))
	for i, t := range tools {
		slugs[i] = t.Slug
	}

	activeStatus := models.ConnectionStatusActive
	return models.MCPIntegration{
		ID:               DefaultPrimaryIntegrationID,
		Name:             "ScanDrix MCP",
		Description:      "Manage integrations, manage connections, and manage tools with ScanDrix MCP integration.",
		AuthScheme:       "OAUTH",
		AppName:          "ScanDrix MCP",
		Logo:             "https://scandrix.io/assets/logo.png",
		Provider:         models.ProviderScandrixMCP,
		IsConnected:      true,
		ConnectionStatus: &activeStatus,
		IsDefault:        true,
		Active:           true,
		AllowedTools:     slugs,
		AuthMethods: []models.PublicAuthMethod{
			{
				ID:      "oauth",
				Label:   "OAuth",
				Type:    models.AuthTypeOAuth2,
				Default: true,
			},
		},
	}
}

// DefaultScandrixToolsCatalog returns the full set of 23 native MCP tools exposed by ScanDrix.
func DefaultScandrixToolsCatalog() []models.MCPTool {
	return []models.MCPTool{
		{
			Slug:        "SCANDRIX_LIST_REPOSITORIES",
			Name:        "SCANDRIX_LIST_REPOSITORIES",
			Description: "List all repositories accessible to the team. Use this to discover available repositories, check repository metadata (private/public, archived status, languages), or when you need to see what repositories exist before performing other operations.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_LIST_PULL_REQUESTS",
			Name:        "SCANDRIX_LIST_PULL_REQUESTS",
			Description: "List pull requests with advanced filtering (by state, repository, author, date range). Use this to find specific PRs, analyze PR patterns, or get overview of team activity. Returns PR metadata only - use get_pull_request for full PR content.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_LIST_COMMITS",
			Name:        "SCANDRIX_LIST_COMMITS",
			Description: "List commit history from repositories with filtering by author, date range, or branch. Use this to analyze commit patterns, find specific commits, or track development activity. Returns commit metadata and messages.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_GET_PULL_REQUEST",
			Name:        "SCANDRIX_GET_PULL_REQUEST",
			Description: "Get complete details of a specific pull request including description, commits, reviews, and list of modified files. Use this when you need full PR context - NOT for file content (use get_pull_request_file_content for that).",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_GET_REPOSITORY_FILES",
			Name:        "SCANDRIX_GET_REPOSITORY_FILES",
			Description: "Get file tree/listing from a repository branch with pattern filtering. Use this to explore repository structure, find specific files by pattern, or get overview of codebase organization. Returns file paths only - NOT file content.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_GET_REPOSITORY_CONTENT",
			Name:        "SCANDRIX_GET_REPOSITORY_CONTENT",
			Description: "Get the current content of a specific file from a repository branch. Use this to read files from the main/current branch - NOT from pull requests (use get_pull_request_file_content for PR files).",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_GET_REPOSITORY_LANGUAGES",
			Name:        "SCANDRIX_GET_REPOSITORY_LANGUAGES",
			Description: "Get programming languages breakdown and statistics for a repository. Use this to understand technology stack, language distribution, or filter repositories by technology.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_GET_PULL_REQUEST_FILE_CONTENT",
			Name:        "SCANDRIX_GET_PULL_REQUEST_FILE_CONTENT",
			Description: "Get the modified content of a specific file within a pull request context. Use this to read how a file looks AFTER the PR changes are applied - NOT the original version.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_GET_DIFF_FOR_FILE",
			Name:        "SCANDRIX_GET_DIFF_FOR_FILE",
			Description: "Get the exact diff/patch showing what changed in a specific file within a pull request. Use this to see the precise changes made - additions, deletions, and modifications line by line.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_GET_PULL_REQUEST_DIFF",
			Name:        "SCANDRIX_GET_PULL_REQUEST_DIFF",
			Description: "Get the complete diff/patch for an entire Pull Request showing all changes across all files. Use this to see the full context of what changed in the PR, including additions, deletions, and modifications across all modified files.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_GET_DRIXY_RULES",
			Name:        "SCANDRIX_GET_DRIXY_RULES",
			Description: "Get all active Drixy Rules at organization level. Use this to see organization-wide coding standards, global rules that apply across all repositories, or when you need a complete overview of all active rules. Returns only ACTIVE status rules.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_GET_DRIXY_RULES_REPOSITORY",
			Name:        "SCANDRIX_GET_DRIXY_RULES_REPOSITORY",
			Description: "Get active Drixy Rules specific to a particular repository. Use this to see repository-specific coding standards, rules that only apply to one codebase, or when analyzing rules for a specific project. More focused than get_drixy_rules.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_CREATE_DRIXY_RULE",
			Name:        "SCANDRIX_CREATE_DRIXY_RULE",
			Description: "Create a new Drixy Rule with custom scope and severity. pull_request scope: analyzes entire PR context for PR-level rules. file scope: analyzes individual files one by one for file-level rules. Rule starts in pending status.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_UPDATE_DRIXY_RULE",
			Name:        "SCANDRIX_UPDATE_DRIXY_RULE",
			Description: "Update an existing Drixy Rule. Only the fields provided in drixyRule will be updated. Use this to modify rule details, change severity, scope, or status of existing rules.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_DELETE_DRIXY_RULE",
			Name:        "SCANDRIX_DELETE_DRIXY_RULE",
			Description: "Delete a Drixy Rule permanently from the system. This action cannot be undone. Use this to remove rules that are no longer needed or relevant.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     true,
		},
		{
			Slug:        "SCANDRIX_CREATE_DRIXY_ISSUE",
			Name:        "SCANDRIX_CREATE_DRIXY_ISSUE",
			Description: "Create a new Drixy Issue linked to a pull request suggestion. Use this to escalate Drixy review comments into trackable issues with metadata like file path, severity, and reporter.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_LIST_DRIXY_ISSUES",
			Name:        "SCANDRIX_LIST_DRIXY_ISSUES",
			Description: "List Drixy Issues with optional filters (repository, severity, label). Use this to audit outstanding Drixy findings, triage by severity, or review the issue backlog.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_GET_DRIXY_ISSUE_DETAILS",
			Name:        "SCANDRIX_GET_DRIXY_ISSUE_DETAILS",
			Description: "Get full details for a specific Drixy Issue by id. Use this to inspect metadata, status, and linked suggestions before taking action.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_UPDATE_DRIXY_ISSUE_STATUS",
			Name:        "SCANDRIX_UPDATE_DRIXY_ISSUE_STATUS",
			Description: "Update the status of a Drixy Issue (e.g. open, resolved, dismissed). Use this to move issues through the workflow directly from MCP.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_UPDATE_DRIXY_ISSUE_CATEGORY",
			Name:        "SCANDRIX_UPDATE_DRIXY_ISSUE_CATEGORY",
			Description: "Update the category/label for a Drixy Issue. Use this to reclassify findings during triage and keep taxonomy accurate.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_DELETE_DRIXY_ISSUE",
			Name:        "SCANDRIX_DELETE_DRIXY_ISSUE",
			Description: "Dismiss a Drixy Issue by updating its status to dismissed. Use this when an issue is no longer relevant or was created by mistake.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     true,
		},
		{
			Slug:        "SCANDRIX_CREATE_MEMORY",
			Name:        "SCANDRIX_CREATE_MEMORY",
			Description: "Create a new memory entry in ScanDrix MCP. Use this to store important information, context, or notes that can be referenced later within the MCP environment.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
		{
			Slug:        "SCANDRIX_FIND_MEMORIES",
			Name:        "SCANDRIX_FIND_MEMORIES",
			Description: "Search for memories in ScanDrix MCP using keywords or filters. Use this to quickly retrieve relevant information, context, or notes that have been previously stored.",
			Provider:    models.ProviderScandrixMCP,
			Warning:     false,
		},
	}
}

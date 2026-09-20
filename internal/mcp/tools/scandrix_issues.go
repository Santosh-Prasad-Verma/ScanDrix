// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise MCP Issues Tools
// File: scandrix_issues.go
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ScanDrixIssueRecord represents a code review or security issue tracked in ScanDrix.
type ScanDrixIssueRecord struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	TeamID         string    `json:"teamId,omitempty"`
	RepositoryID   string    `json:"repositoryId,omitempty"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	Severity       string    `json:"severity"` // "CRITICAL", "HIGH", "MEDIUM", "LOW"
	Category       string    `json:"category"`
	Status         string    `json:"status"` // "OPEN", "RESOLVED", "WONT_FIX", "IGNORED"
	FilePath       string    `json:"filePath,omitempty"`
	Line           int       `json:"line,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// SCMIssueRecord represents an issue on GitHub, GitLab, Bitbucket or Forgejo in ScanDrix.
type SCMIssueRecord struct {
	ID        string         `json:"id"`
	Number    int            `json:"number"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	State     string         `json:"state"` // "open", "closed"
	URL       string         `json:"url"`
	Labels    []string       `json:"labels"`
	Assignees []string       `json:"assignees"`
	Author    map[string]any `json:"author"`
	CreatedAt string         `json:"createdAt"`
	UpdatedAt string         `json:"updatedAt"`
	ClosedAt  string         `json:"closedAt,omitempty"`
	Platform  string         `json:"platform"`
}

var (
	issuesStoreMu sync.RWMutex
	issuesStore   = make(map[string]*ScanDrixIssueRecord)
	scmIssuesMu   sync.RWMutex
	scmIssues     = make(map[string]*SCMIssueRecord)
)

// GetSCMIssuesTools returns SCM issue tracker tools for repositories.
func GetSCMIssuesTools() []MCPTool {
	listIssuesHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		if orgID == "" {
			return nil, fmt.Errorf("organizationId is required")
		}

		scmIssuesMu.RLock()
		defer scmIssuesMu.RUnlock()

		var matched []SCMIssueRecord
		for _, iss := range scmIssues {
			matched = append(matched, *iss)
		}
		if matched == nil {
			matched = []SCMIssueRecord{}
		}

		return map[string]any{
			"success": true,
			"count":   len(matched),
			"data":    matched,
		}, nil
	}

	getIssueHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		if orgID == "" {
			return nil, fmt.Errorf("organizationId is required")
		}

		issueNumFloat, _ := args["issueNumber"].(float64)
		scmIssuesMu.RLock()
		defer scmIssuesMu.RUnlock()

		for _, iss := range scmIssues {
			if iss.Number == int(issueNumFloat) {
				return map[string]any{
					"success": true,
					"data":    iss,
				}, nil
			}
		}

		return map[string]any{
			"success": false,
			"data":    nil,
			"message": "Issue not found",
		}, nil
	}

	return []MCPTool{
		{
			Name:        "SCANDRIX_LIST_SCM_ISSUES",
			Description: "List issues from the repository's issue tracker (GitHub, GitLab, Bitbucket, or Forgejo) using the team's code-management integration.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"teamId":         map[string]string{"type": "string"},
					"repository":     map[string]any{"type": "object"},
					"filters":        map[string]any{"type": "object"},
				},
				"required": []string{"organizationId"},
			},
			Handler: listIssuesHandler,
		},
		{
			Name:        "list_scm_issues",
			Description: "List issues from the repository's issue tracker.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: listIssuesHandler,
		},
		{
			Name:        "list_issues",
			Description: "List issues from the repository's issue tracker.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: listIssuesHandler,
		},
		{
			Name:        "SCANDRIX_GET_SCM_ISSUE",
			Description: "Get a single issue by number from the repository's issue tracker using the team's code-management integration.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"teamId":         map[string]string{"type": "string"},
					"repository":     map[string]any{"type": "object"},
					"issueNumber":    map[string]string{"type": "integer"},
				},
				"required": []string{"organizationId", "issueNumber"},
			},
			Handler: getIssueHandler,
		},
		{
			Name:        "get_scm_issue",
			Description: "Get a single issue by number from the repository's issue tracker.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"issueNumber":    map[string]string{"type": "integer"},
				},
				"required": []string{"organizationId", "issueNumber"},
			},
			Handler: getIssueHandler,
		},
		{
			Name:        "get_issue",
			Description: "Get a single issue by number from the repository's issue tracker.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"issueNumber":    map[string]string{"type": "integer"},
				},
				"required": []string{"organizationId", "issueNumber"},
			},
			Handler: getIssueHandler,
		},
	}
}

// GetReviewIssuesTools returns review finding tools for ScanDrix.
func GetReviewIssuesTools() []MCPTool {
	createHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		title, _ := args["title"].(string)
		if orgID == "" || title == "" {
			return nil, fmt.Errorf("organizationId and title are required")
		}

		desc, _ := args["description"].(string)
		sev, _ := args["severity"].(string)
		if sev == "" {
			sev = "HIGH"
		}
		cat, _ := args["category"].(string)
		if cat == "" {
			cat = "SECURITY"
		}
		teamID, _ := args["teamId"].(string)
		repoID, _ := args["repositoryId"].(string)
		filePath, _ := args["filePath"].(string)
		lineFloat, _ := args["line"].(float64)

		issueID := uuid.New().String()
		now := time.Now().UTC()

		issue := &ScanDrixIssueRecord{
			ID:             issueID,
			OrganizationID: orgID,
			TeamID:         teamID,
			RepositoryID:   repoID,
			Title:          title,
			Description:    desc,
			Severity:       sev,
			Category:       cat,
			Status:         "OPEN",
			FilePath:       filePath,
			Line:           int(lineFloat),
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		issuesStoreMu.Lock()
		issuesStore[issueID] = issue
		issuesStoreMu.Unlock()

		return map[string]any{
			"success": true,
			"data":    issue,
		}, nil
	}

	listHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		if orgID == "" {
			return nil, fmt.Errorf("organizationId is required")
		}

		statusFilter, _ := args["status"].(string)
		sevFilter, _ := args["severity"].(string)

		issuesStoreMu.RLock()
		defer issuesStoreMu.RUnlock()

		var matched []*ScanDrixIssueRecord
		for _, iss := range issuesStore {
			if iss.OrganizationID == orgID || orgID == "*" {
				if statusFilter != "" && iss.Status != statusFilter {
					continue
				}
				if sevFilter != "" && iss.Severity != sevFilter {
					continue
				}
				matched = append(matched, iss)
			}
		}

		if matched == nil {
			matched = []*ScanDrixIssueRecord{}
		}

		return map[string]any{
			"success": true,
			"count":   len(matched),
			"data":    matched,
		}, nil
	}

	getHandler := func(ctx context.Context, args map[string]any) (any, error) {
		issueID, _ := args["issueId"].(string)
		if issueID == "" {
			return nil, fmt.Errorf("issueId is required")
		}

		issuesStoreMu.RLock()
		defer issuesStoreMu.RUnlock()

		if iss, ok := issuesStore[issueID]; ok {
			return map[string]any{
				"success": true,
				"data":    iss,
			}, nil
		}

		return map[string]any{
			"success": false,
			"data":    nil,
			"message": "Issue not found",
		}, nil
	}

	updateStatusHandler := func(ctx context.Context, args map[string]any) (any, error) {
		issueID, _ := args["issueId"].(string)
		status, _ := args["status"].(string)
		if issueID == "" || status == "" {
			return nil, fmt.Errorf("issueId and status are required")
		}

		issuesStoreMu.Lock()
		defer issuesStoreMu.Unlock()

		if iss, ok := issuesStore[issueID]; ok {
			iss.Status = status
			iss.UpdatedAt = time.Now().UTC()
			return map[string]any{
				"success": true,
				"data":    iss,
			}, nil
		}

		return map[string]any{
			"success": false,
			"message": "Issue not found",
		}, nil
	}

	updateCategoryHandler := func(ctx context.Context, args map[string]any) (any, error) {
		issueID, _ := args["issueId"].(string)
		category, _ := args["category"].(string)
		if issueID == "" || category == "" {
			return nil, fmt.Errorf("issueId and category are required")
		}

		issuesStoreMu.Lock()
		defer issuesStoreMu.Unlock()

		if iss, ok := issuesStore[issueID]; ok {
			iss.Category = category
			iss.UpdatedAt = time.Now().UTC()
			return map[string]any{
				"success": true,
				"data":    iss,
			}, nil
		}

		return map[string]any{
			"success": false,
			"message": "Issue not found",
		}, nil
	}

	deleteHandler := func(ctx context.Context, args map[string]any) (any, error) {
		issueID, _ := args["issueId"].(string)
		if issueID == "" {
			return nil, fmt.Errorf("issueId is required")
		}

		issuesStoreMu.Lock()
		defer issuesStoreMu.Unlock()

		if _, ok := issuesStore[issueID]; ok {
			delete(issuesStore, issueID)
			return map[string]any{"success": true}, nil
		}

		return map[string]any{"success": false, "message": "Issue not found"}, nil
	}

	tools := []MCPTool{
		// 1. SCANDRIX_CREATE_ISSUE
		{
			Name:        "SCANDRIX_CREATE_ISSUE",
			Description: "Create a new tracked code review issue or finding in ScanDrix.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"title":          map[string]string{"type": "string"},
					"description":    map[string]string{"type": "string"},
					"filePath":       map[string]string{"type": "string"},
					"severity":       map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "title"},
			},
			Handler: createHandler,
		},
		{
			Name:        "create_issue",
			Description: "Create a new tracked code review issue or finding in ScanDrix.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"title":          map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "title"},
			},
			Handler: createHandler,
		},

		// 2. SCANDRIX_LIST_ISSUES
		{
			Name:        "SCANDRIX_LIST_ISSUES",
			Description: "List all review issues and findings for an organization with optional filters.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"status":         map[string]string{"type": "string"},
					"severity":       map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: listHandler,
		},
		{
			Name:        "list_tracked_issues",
			Description: "List all review issues and findings for an organization with optional filters.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: listHandler,
		},

		// 3. SCANDRIX_GET_ISSUE_DETAILS
		{
			Name:        "SCANDRIX_GET_ISSUE_DETAILS",
			Description: "Get details for a specific code review finding issue.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"issueId": map[string]string{"type": "string"},
				},
				"required": []string{"issueId"},
			},
			Handler: getHandler,
		},
		{
			Name:        "get_issue_details",
			Description: "Get details for a specific code review finding issue.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"issueId": map[string]string{"type": "string"},
				},
				"required": []string{"issueId"},
			},
			Handler: getHandler,
		},

		// 4. SCANDRIX_UPDATE_ISSUE_STATUS
		{
			Name:        "SCANDRIX_UPDATE_ISSUE_STATUS",
			Description: "Update the status of a review issue (OPEN, RESOLVED, WONT_FIX, IGNORED).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"issueId": map[string]string{"type": "string"},
					"status":  map[string]string{"type": "string"},
				},
				"required": []string{"issueId", "status"},
			},
			Handler: updateStatusHandler,
		},
		{
			Name:        "update_issue_status",
			Description: "Update the status of a review issue (OPEN, RESOLVED, WONT_FIX, IGNORED).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"issueId": map[string]string{"type": "string"},
					"status":  map[string]string{"type": "string"},
				},
				"required": []string{"issueId", "status"},
			},
			Handler: updateStatusHandler,
		},

		// 5. SCANDRIX_UPDATE_ISSUE_CATEGORY
		{
			Name:        "SCANDRIX_UPDATE_ISSUE_CATEGORY",
			Description: "Update the category or label of a review issue.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"issueId":  map[string]string{"type": "string"},
					"category": map[string]string{"type": "string"},
				},
				"required": []string{"issueId", "category"},
			},
			Handler: updateCategoryHandler,
		},
		{
			Name:        "update_issue_category",
			Description: "Update the category or label of a review issue.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"issueId":  map[string]string{"type": "string"},
					"category": map[string]string{"type": "string"},
				},
				"required": []string{"issueId", "category"},
			},
			Handler: updateCategoryHandler,
		},

		// 6. SCANDRIX_DELETE_ISSUE
		{
			Name:        "SCANDRIX_DELETE_ISSUE",
			Description: "Delete or dismiss a review issue.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"issueId": map[string]string{"type": "string"},
				},
				"required": []string{"issueId"},
			},
			Handler: deleteHandler,
		},
		{
			Name:        "delete_issue",
			Description: "Delete or dismiss a review issue.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"issueId": map[string]string{"type": "string"},
				},
				"required": []string{"issueId"},
			},
			Handler: deleteHandler,
		},
	}

	return tools
}

// GetScanDrixIssuesTools returns all issue tools.
func GetScanDrixIssuesTools() []MCPTool {
	var all []MCPTool
	all = append(all, GetSCMIssuesTools()...)
	all = append(all, GetReviewIssuesTools()...)
	return all
}

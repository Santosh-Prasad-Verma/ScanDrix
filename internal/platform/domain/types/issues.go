// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package types

import (
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// CodeManagementIssue represents a normalized issue across GitHub, GitLab, Bitbucket, and Forgejo.
type CodeManagementIssue struct {
	ID          string              `json:"id"`
	Number      int                 `json:"number"`
	Title       string              `json:"title"`
	Body        *string             `json:"body"`
	Description string              `json:"description,omitempty"`
	State       string              `json:"state"` // "open", "closed"
	URL         string              `json:"url"`
	Labels    []string            `json:"labels"`
	Assignees []string            `json:"assignees"`
	Author    *IssueAuthor        `json:"author,omitempty"`
	CreatedAt string              `json:"createdAt"`
	UpdatedAt string              `json:"updatedAt"`
	ClosedAt  *string             `json:"closedAt,omitempty"`
	Platform  models.SCMProvider  `json:"platform"`
}

// IssueAuthor identifies an issue creator.
type IssueAuthor struct {
	Username string  `json:"username"`
	ID       *string `json:"id,omitempty"`
}

// ListIssuesParams defines filters for listing issues on a repository.
type ListIssuesParams struct {
	OrganizationAndTeamData OrganizationAndTeamData `json:"organizationAndTeamData"`
	Repository              RepoOwnerName           `json:"repository"`
	Filters                 *IssueFilters           `json:"filters,omitempty"`
}

// RepoOwnerName holds owner/org and repository name.
type RepoOwnerName struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

// IssueFilters specifies filtering parameters for issue queries.
type IssueFilters struct {
	State    string   `json:"state,omitempty"` // "open", "closed", "all"
	Labels   []string `json:"labels,omitempty"`
	Assignee string   `json:"assignee,omitempty"`
	Since    string   `json:"since,omitempty"`
	Page     int      `json:"page,omitempty"`
	PerPage  int      `json:"perPage,omitempty"`
}

// GetIssueParams retrieves a specific issue by number.
type GetIssueParams struct {
	OrganizationAndTeamData OrganizationAndTeamData `json:"organizationAndTeamData"`
	Repository              RepoOwnerName           `json:"repository"`
	IssueNumber             int                     `json:"issueNumber"`
}

// GitHubIssue captures raw GitHub issue properties.
type GitHubIssue struct {
	ID        int64      `json:"id"`
	NodeID    string     `json:"nodeId"`
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      *string    `json:"body"`
	State     string     `json:"state"`
	Locked    bool       `json:"locked"`
	HTMLURL   string     `json:"htmlUrl"`
	Comments  int        `json:"comments"`
	Labels    []string   `json:"labels"`
	Assignees []string   `json:"assignees"`
	User      *GitHubUser `json:"user,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	ClosedAt  *time.Time `json:"closedAt,omitempty"`
}

// GitHubUser holds GitHub user login and avatar metadata.
type GitHubUser struct {
	Login     string `json:"login"`
	ID        int64  `json:"id"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	HTMLURL   string `json:"htmlUrl,omitempty"`
}

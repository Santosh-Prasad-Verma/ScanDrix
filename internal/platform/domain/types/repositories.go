// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package types

import (
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// AuthMode dictates authentication method for Git operations.
type AuthMode string

const (
	AuthModeOAuth AuthMode = "oauth"
	AuthModeApp   AuthMode = "app"
	AuthModePAT   AuthMode = "pat"
	AuthModeBasic AuthMode = "basic"
	AuthModeSSH   AuthMode = "ssh"
)

// Repositories provides list-view repository metadata.
type Repositories struct {
	ID                           string              `json:"id"`
	Name                         string              `json:"name"`
	FullName                     string              `json:"full_name,omitempty"`
	Owner                        string              `json:"owner,omitempty"`
	Private                      bool                `json:"private,omitempty"`
	Archived                     bool                `json:"archived,omitempty"`
	HTTPURL                      string              `json:"http_url"`
	AvatarURL                    string              `json:"avatar_url"`
	OrganizationName             string              `json:"organizationName"`
	Visibility                   string              `json:"visibility"` // "public" or "private"
	Selected                     bool                `json:"selected"`
	DefaultBranch                string              `json:"default_branch,omitempty"`
	Language                     string              `json:"language,omitempty"`
	LastActivityAt               string              `json:"lastActivityAt,omitempty"`
	Project                      *RepositoryProject  `json:"project,omitempty"`
	WorkspaceID                  string              `json:"workspaceId,omitempty"`
	Directories                  []string            `json:"directories,omitempty"`
	RecentPullRequestsCount      int                 `json:"recentPullRequestsCount,omitempty"`
	RecentPullRequestsWindowDays int                 `json:"recentPullRequestsWindowDays,omitempty"`
}

// RepositoryProject links repositories to organizational projects (e.g. Azure/GitLab groups).
type RepositoryProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Repository encapsulates detailed repository properties used in code review.
type Repository struct {
	ID               string              `json:"id"`
	Name             string              `json:"name"`
	FullName         string              `json:"full_name"`
	Owner            string              `json:"owner,omitempty"`
	CloneURL         string              `json:"clone_url"`
	HTMLURL          string              `json:"html_url,omitempty"`
	SSHURL           string              `json:"ssh_url,omitempty"`
	DefaultBranch    string              `json:"default_branch"`
	IsPrivate        bool                `json:"is_private"`
	Private          bool                `json:"private,omitempty"`
	Archived         bool                `json:"archived,omitempty"`
	Language         string              `json:"language"`
	OrganizationName string              `json:"organization_name"`
	Provider         models.SCMProvider  `json:"provider"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

// TreeItem represents a file or directory node in Git tree traversal.
type TreeItem struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"` // "blob", "tree", "commit"
	SHA  string `json:"sha"`
	Size int64  `json:"size,omitempty"`
	URL  string `json:"url,omitempty"`
}

// RepositoryFile contains file metadata and raw content fetched from Git.
type RepositoryFile struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding,omitempty"`
	SHA      string `json:"sha,omitempty"`
	Size     int64  `json:"size,omitempty"`
	IsBinary bool   `json:"is_binary,omitempty"`
}

// Commit represents Git commit metadata and parent links.
type Commit struct {
	SHA         string    `json:"sha"`
	Message     string    `json:"message"`
	Author      GitActor  `json:"author"`
	AuthorName  string    `json:"author_name,omitempty"`
	AuthorEmail string    `json:"author_email,omitempty"`
	AuthorLogin string    `json:"author_login,omitempty"`
	Committer   GitActor  `json:"committer"`
	Date        time.Time `json:"date"`
	Parents     []string  `json:"parents,omitempty"`
	URL         string    `json:"url,omitempty"`
}

// GitCloneParams carries credentials and parameters needed by worker clone containers.
type GitCloneParams struct {
	URL            string              `json:"url"`
	Provider       models.SCMProvider  `json:"provider"`
	Branch         string              `json:"branch,omitempty"`
	Auth           *GitCloneAuth       `json:"auth,omitempty"`
	Token          string              `json:"token,omitempty"`
	Username       string              `json:"username,omitempty"`
	OrganizationID string              `json:"organizationId"`
	RepositoryID   string              `json:"repositoryId"`
	RepositoryName string              `json:"repositoryName"`
}

// GitCloneAuth provides credentials for Git transport cloning.
type GitCloneAuth struct {
	Type     AuthMode `json:"type,omitempty"`
	Username string   `json:"username,omitempty"`
	Token    string   `json:"token,omitempty"`
	Org      string   `json:"org,omitempty"`
}

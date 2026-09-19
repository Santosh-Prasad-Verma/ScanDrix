// Package webhooks provides unified normalization and parsing for multi-VCS webhooks (GitHub, GitLab, Azure, Bitbucket, Forgejo).
package webhooks

import (
	"regexp"
	"strings"
)

// MappedAction represents standard lifecycle actions across all VCS platforms.
type MappedAction string

const (
	ActionOpened         MappedAction = "opened"
	ActionUpdated        MappedAction = "updated"
	ActionClosed         MappedAction = "closed"
	ActionReopened       MappedAction = "reopened"
	ActionMerged         MappedAction = "merged"
	ActionCommentCreated MappedAction = "comment_created"
	ActionUnknown        MappedAction = "unknown"
)

// MappedUser contains unified user identity metadata.
type MappedUser struct {
	ID        string `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatarUrl"`
}

// MappedCommitRef holds branch and commit SHA references.
type MappedCommitRef struct {
	RepoFullName string `json:"repoFullName"`
	Ref          string `json:"ref"`
	SHA          string `json:"sha"`
}

// MappedRepository contains normalized repository information.
type MappedRepository struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"fullName"`
	URL           string `json:"url"`
	Language      string `json:"language"`
	DefaultBranch string `json:"defaultBranch"`
	IsPrivate     bool   `json:"isPrivate"`
}

// MappedPullRequest contains normalized pull request attributes.
type MappedPullRequest struct {
	ID         string           `json:"id"`
	Number     int              `json:"number"`
	Title      string           `json:"title"`
	Body       string           `json:"body"`
	URL        string           `json:"url"`
	State      string           `json:"state"`
	IsDraft    bool             `json:"isDraft"`
	Head       MappedCommitRef  `json:"head"`
	Base       MappedCommitRef  `json:"base"`
	User       *MappedUser      `json:"user"`
	Assignees  []MappedUser     `json:"assignees"`
	Reviewers  []MappedUser     `json:"reviewers"`
	Labels     []string         `json:"labels"`
	Repository *MappedRepository `json:"repository"`
}

// MappedComment contains normalized comment information.
type MappedComment struct {
	ID        string      `json:"id"`
	Body      string      `json:"body"`
	URL       string      `json:"url"`
	Path      string      `json:"path,omitempty"`
	Line      int         `json:"line,omitempty"`
	User      *MappedUser `json:"user"`
	CreatedAt string      `json:"createdAt"`
}

// PlatformMapper defines standard transformation contract from raw JSON payload to unified models.
type PlatformMapper interface {
	MapPullRequest(payload map[string]any) *MappedPullRequest
	MapComment(payload map[string]any) *MappedComment
	MapRepository(payload map[string]any) *MappedRepository
	MapAction(action string) MappedAction
}

var draftRegex = regexp.MustCompile(`(?i)^\s*(\[draft\]|draft:|wip:|\(draft\))\s*`)

// IsDraftTitle checks if pull request or MR title marks it as a draft / work-in-progress.
func IsDraftTitle(title string) bool {
	return draftRegex.MatchString(strings.TrimSpace(title))
}

// StripDraftPrefix returns title without draft markers.
func StripDraftPrefix(title string) string {
	return strings.TrimSpace(draftRegex.ReplaceAllString(title, ""))
}

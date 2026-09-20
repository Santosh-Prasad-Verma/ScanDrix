package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ConfigLevel specifies hierarchy depth of PR custom message templates.
type ConfigLevel string

const (
	ConfigLevelGlobal     ConfigLevel = "global"
	ConfigLevelRepository ConfigLevel = "repository"
	ConfigLevelDirectory  ConfigLevel = "directory"
)

// PullRequestMessageStatus controls triggering rules for review comments.
type PullRequestMessageStatus string

const (
	MessageStatusEveryPush      PullRequestMessageStatus = "every_push"
	MessageStatusOnlyWhenOpened PullRequestMessageStatus = "only_when_opened"
	MessageStatusOff            PullRequestMessageStatus = "off"
	MessageStatusActive         PullRequestMessageStatus = "active"
	MessageStatusInactive       PullRequestMessageStatus = "inactive"
)

// PullRequestMessageContent encapsulates markdown template text and activation condition.
type PullRequestMessageContent struct {
	Content string                   `json:"content"`
	Status  PullRequestMessageStatus `json:"status"`
}

// GlobalMessageSettings configures SCM comment display behaviors.
type GlobalMessageSettings struct {
	HideComments         bool `json:"hideComments"`
	SuggestionCopyPrompt bool `json:"suggestionCopyPrompt"`
}

// PullRequestMessages defines custom review template configurations across org, repo, and subdirectories.
type PullRequestMessages struct {
	ID                 uuid.UUID                  `json:"id"`
	OrganizationID     string                     `json:"organizationId"`
	ConfigLevel        ConfigLevel                `json:"configLevel"`
	RepositoryID       string                     `json:"repositoryId,omitempty"`
	DirectoryID        string                     `json:"directoryId,omitempty"`
	DirectoryPath      string                     `json:"directoryPath,omitempty"`
	StartReviewMessage *PullRequestMessageContent `json:"startReviewMessage,omitempty"`
	EndReviewMessage   *PullRequestMessageContent `json:"endReviewMessage,omitempty"`
	ErrorReviewMessage *PullRequestMessageContent `json:"errorReviewMessage,omitempty"`
	GlobalSettings     *GlobalMessageSettings     `json:"globalSettings,omitempty"`
	CreatedAt          time.Time                  `json:"createdAt"`
	UpdatedAt          time.Time                  `json:"updatedAt"`
}

// DirectoryOverrideCount summarizes per-repository directory-level custom rules.
type DirectoryOverrideCount struct {
	RepositoryID   string `json:"repositoryId"`
	RepositoryName string `json:"repositoryName"`
	Count          int    `json:"count"`
}

// MessagesFilter provides query criteria for retrieving messages.
type MessagesFilter struct {
	OrganizationID string
	ConfigLevel    ConfigLevel
	RepositoryID   string
	DirectoryID    string
	DirectoryPath  string
}

// IPullRequestMessagesRepository defines the persistence contract for PR messages.
type IPullRequestMessagesRepository interface {
	Create(ctx context.Context, msg *PullRequestMessages) (*PullRequestMessages, error)
	Update(ctx context.Context, msg *PullRequestMessages) (*PullRequestMessages, error)
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByFilter(ctx context.Context, filter MessagesFilter) (int64, error)
	Find(ctx context.Context, filter MessagesFilter) ([]PullRequestMessages, error)
	FindOne(ctx context.Context, filter MessagesFilter) (*PullRequestMessages, error)
	FindByID(ctx context.Context, id uuid.UUID) (*PullRequestMessages, error)
	FindOverrideCountsByOrg(ctx context.Context, orgID string) ([]DirectoryOverrideCount, error)
}

// IPullRequestMessagesService provides template resolution following Directory -> Repository -> Global hierarchy.
type IPullRequestMessagesService interface {
	ResolveEffectiveMessages(ctx context.Context, orgID, repoID, filePath string) (*PullRequestMessages, error)
	CreateOrUpdate(ctx context.Context, msg *PullRequestMessages) (*PullRequestMessages, error)
	DeleteByRepoOrDirectory(ctx context.Context, orgID, repoID, directoryID string) error
	FindOverrideCounts(ctx context.Context, orgID string) ([]DirectoryOverrideCount, error)
}

// DefaultStartReviewTemplate returns standard greeting comment.
func DefaultStartReviewTemplate() PullRequestMessageContent {
	return PullRequestMessageContent{
		Content: "🤖 **ScanDrix AI** is reviewing your changes against repository rules and best practices...",
		Status:  MessageStatusEveryPush,
	}
}

// DefaultEndReviewTemplate returns standard summary footer.
func DefaultEndReviewTemplate() PullRequestMessageContent {
	return PullRequestMessageContent{
		Content: "### 🔍 ScanDrix Review Completed\n\n{{summary}}\n\n{{findings}}\n\n---\n*Powered by [ScanDrix](https://scandrix.dev)*",
		Status:  MessageStatusEveryPush,
	}
}

// DefaultErrorReviewTemplate returns standard error diagnostic comment.
func DefaultErrorReviewTemplate() PullRequestMessageContent {
	return PullRequestMessageContent{
		Content: "⚠️ **ScanDrix AI** encountered an issue analyzing this pull request:\n\n> {{error_message}}\n\n*Our team has been notified.*",
		Status:  MessageStatusActive,
	}
}

package operations

import (
	"time"

	"github.com/google/uuid"
)

// GitCommandType classifies developer bot commands in PR comments.
type GitCommandType string

const (
	CommandBusinessLogicValidation GitCommandType = "business_logic_validation"
	CommandInvalidContext          GitCommandType = "invalid_context"
	CommandInteractiveChat         GitCommandType = "interactive_chat"
	CommandTriggerFullReview       GitCommandType = "trigger_review"
	CommandUnknown                 GitCommandType = "unknown"
)

// GitCommentContext defines where a comment was placed in the SCM.
type GitCommentContext string

const (
	ContextGeneralPR GitCommentContext = "general_pr"
	ContextInline    GitCommentContext = "inline_comment"
)

// GitChatInput holds the incoming webhook or SCM comment trigger.
type GitChatInput struct {
	WorkspaceID  uuid.UUID         `json:"workspace_id"`
	RepositoryID uuid.UUID         `json:"repository_id"`
	PRNumber     int               `json:"pr_number"`
	Author       string            `json:"author"`
	CommentBody  string            `json:"comment_body"`
	CommentID    string            `json:"comment_id"`
	ThreadID     string            `json:"thread_id,omitempty"`
	Context      GitCommentContext `json:"context"` // inline or general PR
	FilePath     string            `json:"file_path,omitempty"`
	LineNumber   int               `json:"line_number,omitempty"`
}

// GitChatResponse represents the automated reply to the PR discussion.
type GitChatResponse struct {
	CommandType   GitCommandType `json:"command_type"`
	ReplyMessage  string         `json:"reply_message"`
	ShouldReply   bool           `json:"should_reply"`
	WatermarkTag  string         `json:"watermark_tag"`
	TriggerReview bool           `json:"trigger_review"`
}

// CommitStatusState represents the GitHub/GitLab commit build status.
type CommitStatusState string

const (
	StatusPending CommitStatusState = "pending"
	StatusSuccess CommitStatusState = "success"
	StatusFailure CommitStatusState = "failure"
	StatusError   CommitStatusState = "error"
)

// CommitStatusCheck details a commit check run reported to the SCM.
type CommitStatusCheck struct {
	SHA         string            `json:"sha"`
	State       CommitStatusState `json:"state"`
	TargetURL   string            `json:"target_url"`
	Description string            `json:"description"`
	Context     string            `json:"context"` // e.g. "scandrix/security-assurance"
	CreatedAt   time.Time         `json:"created_at"`
}

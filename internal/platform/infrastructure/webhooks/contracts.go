package webhooks

import (
	"context"
	"encoding/json"

	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

// ReviewJobRequest contains context required to trigger an asynchronous AI review.
type ReviewJobRequest struct {
	PlatformType            models.SCMProvider            `json:"platformType"`
	Event                   string                        `json:"event"`
	Action                  string                        `json:"action"`
	Origin                  string                        `json:"origin,omitempty"` // "webhook", "command", "command-force"
	PullRequestNumber       int                           `json:"pullRequestNumber"`
	RepositoryID            string                        `json:"repositoryId"`
	RepositoryName          string                        `json:"repositoryName"`
	RepositoryFullName      string                        `json:"repositoryFullName"`
	OrganizationAndTeamData types.OrganizationAndTeamData `json:"organizationAndTeamData"`
	TeamAutomationID        string                        `json:"teamAutomationId"`
	CorrelationID           string                        `json:"correlationId,omitempty"`
	ReviewDirective         string                        `json:"reviewDirective,omitempty"`
	Heavy                   bool                          `json:"heavy"`
	TriggerCommentID        string                        `json:"triggerCommentId,omitempty"`
	RawPayload              json.RawMessage               `json:"rawPayload"`
}

// ImplementationCheckRequest contains context for verification on commit updates.
type ImplementationCheckRequest struct {
	PlatformType            models.SCMProvider            `json:"platformType"`
	Event                   string                        `json:"event"`
	Trigger                 string                        `json:"trigger"`
	PullRequestNumber       int                           `json:"pullRequestNumber"`
	CommitSHA               string                        `json:"commitSha"`
	RepositoryID            string                        `json:"repositoryId"`
	RepositoryName          string                        `json:"repositoryName"`
	OrganizationAndTeamData types.OrganizationAndTeamData `json:"organizationAndTeamData"`
}

// AstGraphUpdateRequest triggers an incremental AST graph re-index upon default branch merge.
type AstGraphUpdateRequest struct {
	PlatformType            models.SCMProvider            `json:"platformType"`
	PullRequestNumber       int                           `json:"pullRequestNumber"`
	RepoExternalID          string                        `json:"repoExternalId"`
	RepoName                string                        `json:"repoName"`
	BaseBranch              string                        `json:"baseBranch"`
	NewSHA                  string                        `json:"newSha"`
	OrganizationAndTeamData types.OrganizationAndTeamData `json:"organizationAndTeamData"`
}

// SandboxInvalidatePayload payload for invalidating review sandboxes.
type SandboxInvalidatePayload struct {
	PRKey  string `json:"prKey"`  // "orgId:repoId:prNumber"
	Reason string `json:"reason"` // "force_pushed" or "pr_closed"
}

// IReviewJobEnqueuer provides execution bridges to the review pipeline.
type IReviewJobEnqueuer interface {
	EnqueueReviewJob(ctx context.Context, req ReviewJobRequest) (string, error)
	EnqueueImplementationCheck(ctx context.Context, req ImplementationCheckRequest) error
	EnqueueAstGraphUpdate(ctx context.Context, req AstGraphUpdateRequest) error
}

// IOutboxRepository persists durable domain events for resilient asynchronous delivery.
type IOutboxRepository interface {
	CreateMessage(ctx context.Context, exchange, routingKey string, payload any) error
}

// IEventEmitter emits local in-process domain events (e.g. pull-request.closed).
type IEventEmitter interface {
	Emit(eventName string, data any)
}

// IChatWithDrixyUseCase handles conversational Git chat when @drixy is mentioned in a comment.
type IChatWithDrixyUseCase interface {
	Execute(ctx context.Context, platform models.SCMProvider, repoID string, prNumber int, commentID, commentBody string) error
}

// IPullRequestSaver persists pull request state to database.
type IPullRequestSaver interface {
	SavePullRequest(ctx context.Context, pr *types.PullRequest) error
}

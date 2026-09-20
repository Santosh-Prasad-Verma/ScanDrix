package clireview

import (
	"github.com/scandrix/backend/internal/clireview/application/usecases"
)

// TraceCommentMarker identifies automated trace decision comments on PRs.
const TraceCommentMarker = usecases.TraceCommentMarker

// PostTracePrCommentInput defines parameters for posting decisions on pull requests.
type PostTracePrCommentInput = usecases.PostTracePrCommentInput

// PostTracePrCommentOutcome conveys the result of the comment post attempt.
type PostTracePrCommentOutcome = usecases.PostTracePrCommentOutcome

// IIssueCommentManager manages PR comment interactions.
type IIssueCommentManager = usecases.IIssueCommentManager

// IssueCommentInfo provides metadata on an existing issue comment.
type IssueCommentInfo = usecases.IssueCommentInfo

// PostTracePrCommentUseCase handles posting decision comments to pull requests.
type PostTracePrCommentUseCase = usecases.PostTracePrCommentUseCase

// NewPostTracePrCommentUseCase creates an initialized PR comment use case.
var NewPostTracePrCommentUseCase = usecases.NewPostTracePrCommentUseCase

// RenderTraceComment builds GitHub Markdown comment for decision summaries.
var RenderTraceComment = usecases.RenderTraceComment

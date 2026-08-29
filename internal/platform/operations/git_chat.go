package operations

import (
	"fmt"
	"strings"
)

const (
	WatermarkCommentMarker = "<!-- scandrix-review -->\n&#8203;"
	ScandrixBotMention     = "@scandrix"
	ScandrixReviewTrigger  = "@scandrix review"
	ScandrixBizLogicCmd    = "@scandrix -v business-logic"
)

// GitChatHandler processes interactive comments left on pull requests by developers.
type GitChatHandler struct{}

func NewGitChatHandler() *GitChatHandler {
	return &GitChatHandler{}
}

// HandleComment evaluates an incoming PR comment and produces a structured action and reply.
func (h *GitChatHandler) HandleComment(input GitChatInput) GitChatResponse {
	trimmed := strings.TrimSpace(input.CommentBody)
	lower := strings.ToLower(trimmed)

	// 1. Anti-Recursion Loop Prevention: Ignore comments produced by Scandrix itself
	if strings.Contains(input.CommentBody, "<!-- scandrix-review -->") {
		return GitChatResponse{
			CommandType: CommandUnknown,
			ShouldReply: false,
		}
	}

	// 2. Filter: Only process comments that mention @scandrix
	if !strings.Contains(lower, ScandrixBotMention) {
		return GitChatResponse{
			CommandType: CommandUnknown,
			ShouldReply: false,
		}
	}

	// 3. Command: Business Logic Validation (@scandrix -v business-logic)
	if strings.HasPrefix(lower, ScandrixBizLogicCmd) || strings.Contains(lower, "-v business-logic") {
		if input.Context == ContextInline {
			return GitChatResponse{
				CommandType:  CommandInvalidContext,
				ShouldReply:  true,
				WatermarkTag: WatermarkCommentMarker,
				ReplyMessage: "The \"@scandrix -v business-logic\" command can only be executed in the main PR discussion thread, not on inline code comments. Please run it from the main discussion tab.\n\n" + WatermarkCommentMarker,
			}
		}
		return GitChatResponse{
			CommandType:   CommandBusinessLogicValidation,
			ShouldReply:   true,
			WatermarkTag:  WatermarkCommentMarker,
			ReplyMessage:  "Analyzing ticket requirements, acceptance criteria, and PR diff for business logic compliance...\n\n" + WatermarkCommentMarker,
			TriggerReview: true,
		}
	}

	// 4. Command: Trigger On-Demand Review (@scandrix review)
	if strings.HasPrefix(lower, ScandrixReviewTrigger) || strings.HasPrefix(lower, "@scandrix scan") {
		return GitChatResponse{
			CommandType:   CommandTriggerFullReview,
			ShouldReply:   true,
			WatermarkTag:  WatermarkCommentMarker,
			ReplyMessage:  "On-demand security and code quality review triggered by " + input.Author + ". Analysis in progress...\n\n" + WatermarkCommentMarker,
			TriggerReview: true,
		}
	}

	// 5. Interactive Chat / Explanations (@scandrix <question>)
	query := strings.TrimSpace(strings.ReplaceAll(trimmed, ScandrixBotMention, ""))
	if query == "" {
		return GitChatResponse{
			CommandType:  CommandInteractiveChat,
			ShouldReply:  true,
			WatermarkTag: WatermarkCommentMarker,
			ReplyMessage: "Hello @" + input.Author + "! You can ask me questions about this PR, run `@scandrix review` to trigger a re-scan, or run `@scandrix -v business-logic` in the main discussion to verify ticket requirements.\n\n" + WatermarkCommentMarker,
		}
	}

	return GitChatResponse{
		CommandType:  CommandInteractiveChat,
		ShouldReply:  true,
		WatermarkTag: WatermarkCommentMarker,
		ReplyMessage: fmt.Sprintf("Regarding your question on line %d: I am analyzing the AST scope and security context to provide recommendations.\n\n%s", input.LineNumber, WatermarkCommentMarker),
	}
}

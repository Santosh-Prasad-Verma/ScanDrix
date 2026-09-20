package operations

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/drixy"
)

const (
	WatermarkCommentMarker = "<!-- drixy-review -->\n<!-- scandrix-review -->\n&#8203;"
	DrixyBotMention        = "@drixy"
	ScandrixBotMention     = "@scandrix"
	DrixyReviewTrigger     = "@drixy review"
	ScandrixReviewTrigger  = "@scandrix review"
	DrixyBizLogicCmd       = "@drixy -v business-logic"
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

	// 1. Anti-Recursion Loop Prevention: Ignore comments produced by Drixy or ScanDrix itself
	if strings.Contains(input.CommentBody, "<!-- drixy-review -->") || strings.Contains(input.CommentBody, "<!-- scandrix-review -->") {
		return GitChatResponse{
			CommandType: CommandUnknown,
			ShouldReply: false,
		}
	}

	// 2. Filter: Only process comments that mention @drixy or @scandrix
	isMentioned := strings.Contains(lower, DrixyBotMention) || strings.Contains(lower, ScandrixBotMention)
	if !isMentioned {
		return GitChatResponse{
			CommandType: CommandUnknown,
			ShouldReply: false,
		}
	}

	// 3. Command: Business Logic Validation (@drixy -v business-logic / @scandrix -v business-logic)
	isBizCmd := strings.HasPrefix(lower, DrixyBizLogicCmd) || strings.HasPrefix(lower, ScandrixBizLogicCmd) || strings.Contains(lower, "-v business-logic")
	if isBizCmd {
		if input.Context == ContextInline {
			return GitChatResponse{
				CommandType:  CommandInvalidContext,
				ShouldReply:  true,
				WatermarkTag: WatermarkCommentMarker,
				ReplyMessage: "The \"@drixy -v business-logic\" command can only be executed in the main PR discussion thread, not on inline code comments. Please run it from the main discussion tab.\n\n" + WatermarkCommentMarker,
			}
		}
		return GitChatResponse{
			CommandType:   CommandBusinessLogicValidation,
			ShouldReply:   true,
			WatermarkTag:  WatermarkCommentMarker,
			ReplyMessage:  "⚡ **Drixy**: Analyzing ticket requirements, acceptance criteria, and PR diff for business logic compliance...\n\n" + WatermarkCommentMarker,
			TriggerReview: true,
		}
	}

	// 4. Command: Trigger On-Demand Review (@drixy review / @scandrix review)
	isReviewCmd := strings.HasPrefix(lower, DrixyReviewTrigger) || strings.HasPrefix(lower, ScandrixReviewTrigger) ||
		strings.HasPrefix(lower, "@drixy scan") || strings.HasPrefix(lower, "@scandrix scan")
	if isReviewCmd {
		return GitChatResponse{
			CommandType:   CommandTriggerFullReview,
			ShouldReply:   true,
			WatermarkTag:  WatermarkCommentMarker,
			ReplyMessage:  "⚡ **Drixy**: On-demand security and code quality review triggered by " + input.Author + ". Analysis in progress...\n\n" + WatermarkCommentMarker,
			TriggerReview: true,
		}
	}

	// 5. Interactive Chat / Explanations (@drixy <question> / @scandrix <question>)
	query := trimmed
	query = strings.ReplaceAll(query, DrixyBotMention, "")
	query = strings.ReplaceAll(query, ScandrixBotMention, "")
	query = strings.TrimSpace(query)

	if query == "" {
		greeting := drixy.FormatDrixyGreeting(input.Author)
		return GitChatResponse{
			CommandType:  CommandInteractiveChat,
			ShouldReply:  true,
			WatermarkTag: WatermarkCommentMarker,
			ReplyMessage: fmt.Sprintf("%s\n\nHere are some quick actions:\n* `@drixy review` - Trigger an on-demand re-scan\n* `@drixy -v business-logic` - Verify ticket requirements\n* Ask any technical question about this PR!\n\n%s", greeting, WatermarkCommentMarker),
		}
	}

	return GitChatResponse{
		CommandType:  CommandInteractiveChat,
		ShouldReply:  true,
		WatermarkTag: WatermarkCommentMarker,
		ReplyMessage: fmt.Sprintf("⚡ **Drixy**: Regarding your question on line %d: I am analyzing the AST scope and security context to provide recommendations.\n\n%s", input.LineNumber, WatermarkCommentMarker),
	}
}

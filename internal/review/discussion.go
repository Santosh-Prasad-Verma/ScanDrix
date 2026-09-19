package review

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/drixy"
	"github.com/scandrix/backend/internal/llm"
)

// CommentReplyPublisher abstracts posting replies back to SCM review comment threads.
type CommentReplyPublisher interface {
	CreateCommentReply(ctx context.Context, owner, repo string, pullNumber int, commentID int64, body string) error
}

// PRCommentDiscussionEvent models an incoming developer comment requiring AI analysis.
type PRCommentDiscussionEvent struct {
	EventID       uuid.UUID `json:"event_id"`
	WorkspaceID   uuid.UUID `json:"workspace_id"`
	RepoNamespace string    `json:"repo_namespace"`
	PullNumber    int       `json:"pull_number"`
	CommentID     int64     `json:"comment_id"`
	Author        string    `json:"author"`
	CommentBody   string    `json:"comment_body"`
	FilePath      string    `json:"file_path,omitempty"`
	DiffHunk      string    `json:"diff_hunk,omitempty"`
}

// DiscussionOrchestrator handles interactive in-line conversations between engineers and AI.
type DiscussionOrchestrator struct {
	llmGateway   *llm.Gateway
	scmPublisher CommentReplyPublisher
}

// NewDiscussionOrchestrator initializes the interactive PR chat engine.
func NewDiscussionOrchestrator(llmGateway *llm.Gateway, scmPublisher CommentReplyPublisher) *DiscussionOrchestrator {
	return &DiscussionOrchestrator{
		llmGateway:   llmGateway,
		scmPublisher: scmPublisher,
	}
}

// IsBotMentioned checks whether the comment contains @drixy or @scandrix invocation.
func IsBotMentioned(comment string) bool {
	lower := strings.ToLower(comment)
	return strings.Contains(lower, drixy.BotMention) || strings.Contains(lower, "@scandrix")
}

// HandleComment processes a discussion comment and posts an inline reply if the bot is mentioned.
func (d *DiscussionOrchestrator) HandleComment(ctx context.Context, event PRCommentDiscussionEvent) (bool, error) {
	if !IsBotMentioned(event.CommentBody) {
		return false, nil // Not addressed to the bot
	}

	slog.Info("Processing interactive PR discussion mention",
		"repo", event.RepoNamespace,
		"pr", event.PullNumber,
		"author", event.Author,
		"comment_id", event.CommentID,
	)

	// Clean prompt query
	cleanQuery := event.CommentBody
	cleanQuery = strings.ReplaceAll(cleanQuery, drixy.BotMention, "")
	cleanQuery = strings.ReplaceAll(cleanQuery, "@scandrix", "")
	cleanQuery = strings.TrimSpace(cleanQuery)

	// Construct contextual prompt for the LLM
	prompt := fmt.Sprintf(`%s

Developer @%s asked about code at %s:
"%s"

Diff context:
%s

Provide an expert, concise, and technically accurate reply. Include exact code snippets if demonstrating a fix.`,
		drixy.DrixySystemPrompt, event.Author, event.FilePath, cleanQuery, event.DiffHunk,
	)

	replyText := ""
	if d.llmGateway != nil {
		req := llm.ReviewRequest{
			RepoNamespace: event.RepoNamespace,
			PullTitle:     fmt.Sprintf("Discussion PR #%d", event.PullNumber),
			DiffContent:   prompt,
		}
		resp, err := d.llmGateway.AnalyzeDiff(ctx, req)
		if err == nil && resp != nil && resp.Summary != "" {
			replyText = resp.Summary
		}
	}

	if replyText == "" {
		replyText = fmt.Sprintf("%s\n\nBased on security and architectural best practices, ensure all external inputs are strictly parameterized and validated against domain schemas.", drixy.FormatDrixyGreeting(event.Author))
	}

	greeting := drixy.FormatDrixyGreeting(event.Author)
	formattedReply := fmt.Sprintf("⚡ **%s** (ScanDrix Assistant)\n\n%s\n\n%s%s", drixy.Name, greeting, replyText, drixy.PRCommentFooter())

	// Publish reply back to GitHub/GitLab
	if d.scmPublisher != nil && event.RepoNamespace != "" && event.PullNumber > 0 && event.CommentID > 0 {
		parts := strings.Split(event.RepoNamespace, "/")
		if len(parts) == 2 {
			owner, repo := parts[0], parts[1]
			if err := d.scmPublisher.CreateCommentReply(ctx, owner, repo, event.PullNumber, event.CommentID, formattedReply); err != nil {
				return false, fmt.Errorf("failed publishing comment reply: %w", err)
			}
		}
	}

	return true, nil
}

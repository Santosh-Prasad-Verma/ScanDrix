package operations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/platform/operations"
)

func TestGitChatHandlerCommandsAndContexts(t *testing.T) {
	handler := operations.NewGitChatHandler()
	wsID := uuid.New()
	repoID := uuid.New()

	// 1. Anti-recursion: Comment containing Scandrix watermark is ignored
	resWatermark := handler.HandleComment(operations.GitChatInput{
		WorkspaceID:  wsID,
		RepositoryID: repoID,
		PRNumber:     42,
		Author:       "scandrix-bot[bot]",
		CommentBody:  "Analysis in progress...\n<!-- scandrix-review -->\n&#8203;",
	})
	if resWatermark.ShouldReply {
		t.Fatal("expected comment with watermark to be ignored to prevent loops")
	}

	// 2. Unrelated comment: No @scandrix mention
	resUnrelated := handler.HandleComment(operations.GitChatInput{
		WorkspaceID:  wsID,
		RepositoryID: repoID,
		PRNumber:     42,
		Author:       "alice",
		CommentBody:  "Looks good to me, ready for merge!",
	})
	if resUnrelated.ShouldReply {
		t.Fatal("expected unrelated comment without mention to be ignored")
	}

	// 3. Business logic command in inline context -> rejected with invalid context warning
	resInlineBiz := handler.HandleComment(operations.GitChatInput{
		WorkspaceID:  wsID,
		RepositoryID: repoID,
		PRNumber:     42,
		Author:       "bob",
		CommentBody:  "@scandrix -v business-logic",
		Context:      operations.ContextInline,
		FilePath:     "auth/jwt.go",
		LineNumber:   45,
	})
	if resInlineBiz.CommandType != operations.CommandInvalidContext || !resInlineBiz.ShouldReply {
		t.Fatalf("expected CommandInvalidContext for inline business logic check, got %+v", resInlineBiz)
	}
	if !strings.Contains(resInlineBiz.ReplyMessage, "main PR discussion thread") {
		t.Fatalf("expected error message to guide user to main discussion tab")
	}

	// 4. Business logic command in general PR discussion -> accepted
	resGeneralBiz := handler.HandleComment(operations.GitChatInput{
		WorkspaceID:  wsID,
		RepositoryID: repoID,
		PRNumber:     42,
		Author:       "bob",
		CommentBody:  "@scandrix -v business-logic",
		Context:      operations.ContextGeneralPR,
	})
	if resGeneralBiz.CommandType != operations.CommandBusinessLogicValidation || !resGeneralBiz.TriggerReview {
		t.Fatalf("expected CommandBusinessLogicValidation with review trigger, got %+v", resGeneralBiz)
	}

	// 5. On-demand review trigger
	resReview := handler.HandleComment(operations.GitChatInput{
		WorkspaceID:  wsID,
		RepositoryID: repoID,
		PRNumber:     42,
		Author:       "carol",
		CommentBody:  "@scandrix review please",
		Context:      operations.ContextGeneralPR,
	})
	if resReview.CommandType != operations.CommandTriggerFullReview || !resReview.TriggerReview {
		t.Fatalf("expected CommandTriggerFullReview, got %+v", resReview)
	}

	// 6. Interactive query on inline comment
	resChat := handler.HandleComment(operations.GitChatInput{
		WorkspaceID:  wsID,
		RepositoryID: repoID,
		PRNumber:     42,
		Author:       "dave",
		CommentBody:  "@scandrix why is this cipher weak?",
		Context:      operations.ContextInline,
		FilePath:     "pkg/crypto/des.go",
		LineNumber:   88,
	})
	if resChat.CommandType != operations.CommandInteractiveChat || !resChat.ShouldReply {
		t.Fatalf("expected CommandInteractiveChat, got %+v", resChat)
	}
	if !strings.Contains(resChat.ReplyMessage, "88") {
		t.Fatalf("expected reply to reference line 88")
	}

	// 7. Drixy specific triggers (@drixy review)
	resDrixyReview := handler.HandleComment(operations.GitChatInput{
		WorkspaceID:  wsID,
		RepositoryID: repoID,
		PRNumber:     42,
		Author:       "eve",
		CommentBody:  "@drixy review",
		Context:      operations.ContextGeneralPR,
	})
	if resDrixyReview.CommandType != operations.CommandTriggerFullReview || !resDrixyReview.TriggerReview {
		t.Fatalf("expected CommandTriggerFullReview for @drixy review, got %+v", resDrixyReview)
	}
	if !strings.Contains(resDrixyReview.ReplyMessage, "Drixy") {
		t.Fatalf("expected reply to mention Drixy: %s", resDrixyReview.ReplyMessage)
	}

	// 8. Drixy business logic validation
	resDrixyBiz := handler.HandleComment(operations.GitChatInput{
		WorkspaceID:  wsID,
		RepositoryID: repoID,
		PRNumber:     42,
		Author:       "eve",
		CommentBody:  "@drixy -v business-logic",
		Context:      operations.ContextGeneralPR,
	})
	if resDrixyBiz.CommandType != operations.CommandBusinessLogicValidation || !resDrixyBiz.TriggerReview {
		t.Fatalf("expected CommandBusinessLogicValidation for @drixy -v business-logic, got %+v", resDrixyBiz)
	}

	// 9. Drixy greeting inquiry (@drixy hello)
	resDrixyHello := handler.HandleComment(operations.GitChatInput{
		WorkspaceID:  wsID,
		RepositoryID: repoID,
		PRNumber:     42,
		Author:       "eve",
		CommentBody:  "@drixy",
		Context:      operations.ContextGeneralPR,
	})
	if resDrixyHello.CommandType != operations.CommandInteractiveChat || !strings.Contains(resDrixyHello.ReplyMessage, "Drixy") {
		t.Fatalf("expected Drixy greeting for bare mention, got: %s", resDrixyHello.ReplyMessage)
	}
}

func TestCodeManagerStatusAndSummary(t *testing.T) {
	mgr := operations.NewCodeManager()
	ctx := context.Background()

	// 1. Commit status check
	sha := "a1b2c3d4e5f67890"
	err := mgr.SetCommitStatus(ctx, operations.CommitStatusCheck{
		SHA:         sha,
		State:       operations.StatusSuccess,
		TargetURL:   "https://scandrix.io/reports/42",
		Description: "All security and quality checks passed",
		Context:     "scandrix/assurance",
	})
	if err != nil {
		t.Fatalf("failed setting commit status: %v", err)
	}

	status, ok := mgr.GetCommitStatus(sha)
	if !ok || status.State != operations.StatusSuccess {
		t.Fatalf("unexpected status retrieved: %+v", status)
	}

	// 2. Summary comment generation
	summaryClean := operations.BuildSummaryComment(uuid.New(), 42, 0, 0, 15)
	if !strings.Contains(summaryClean, "Passed") || !strings.Contains(summaryClean, "<!-- scandrix-review -->") {
		t.Fatalf("clean summary missing expected banner or watermark")
	}

	summaryAlert := operations.BuildSummaryComment(uuid.New(), 42, 2, 1, 15)
	if !strings.Contains(summaryAlert, "Critical Vulnerabilities Detected") {
		t.Fatalf("alert summary missing critical banner")
	}
}

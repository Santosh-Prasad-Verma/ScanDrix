package review_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/review"
)

type mockCommentPublisher struct {
	replies []string
}

func (m *mockCommentPublisher) CreateCommentReply(ctx context.Context, owner, repo string, pullNumber int, commentID int64, body string) error {
	m.replies = append(m.replies, body)
	return nil
}

func TestDiscussionOrchestrator(t *testing.T) {
	ctx := context.Background()
	mockPub := &mockCommentPublisher{}
	aiGateway := llm.NewGateway("", "", "", "")
	orchestrator := review.NewDiscussionOrchestrator(aiGateway, mockPub)

	// 1. Comment not mentioning the bot
	ignoredEvent := review.PRCommentDiscussionEvent{
		EventID:       uuid.New(),
		RepoNamespace: "acme/service",
		PullNumber:    10,
		CommentID:     101,
		Author:        "bob",
		CommentBody:   "Looks good to me, LGTM!",
	}
	handled, err := orchestrator.HandleComment(ctx, ignoredEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Error("expected ignored comment not to be handled")
	}
	if len(mockPub.replies) != 0 {
		t.Errorf("expected 0 replies, got %d", len(mockPub.replies))
	}

	// 2. Comment invoking @drixy
	invokedEvent := review.PRCommentDiscussionEvent{
		EventID:       uuid.New(),
		RepoNamespace: "acme/service",
		PullNumber:    10,
		CommentID:     102,
		Author:        "alice",
		CommentBody:   "@drixy why is this query reported as dangerous?",
		FilePath:      "db.go",
		DiffHunk:      "+ db.Query(fmt.Sprintf(\"SELECT * FROM users WHERE id = '%s'\", id))",
	}
	handled, err = orchestrator.HandleComment(ctx, invokedEvent)
	if err != nil {
		t.Fatalf("unexpected error handling bot mention: %v", err)
	}
	if !handled {
		t.Error("expected bot mention to be handled")
	}
	if len(mockPub.replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(mockPub.replies))
	}
	reply := mockPub.replies[0]
	if !strings.Contains(reply, "Drixy") || !strings.Contains(reply, "@alice") {
		t.Errorf("unexpected reply body: %s", reply)
	}

	// 3. Backward compatible comment invoking @scandrix
	scandrixEvent := review.PRCommentDiscussionEvent{
		EventID:       uuid.New(),
		RepoNamespace: "acme/service",
		PullNumber:    10,
		CommentID:     103,
		Author:        "bob",
		CommentBody:   "@scandrix please inspect this logic",
		FilePath:      "main.go",
	}
	handled, err = orchestrator.HandleComment(ctx, scandrixEvent)
	if err != nil || !handled {
		t.Errorf("expected @scandrix alias to be handled, err: %v", err)
	}
}

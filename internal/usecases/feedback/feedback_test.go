package feedback_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/usecases/feedback"
)

func TestPRMessagePathMatching(t *testing.T) {
	mgr := feedback.NewPRMessageManager()
	wsID := uuid.New()

	// Add rule for database migrations
	mgr.AddRule(feedback.ScopedMessageRule{
		ID:           uuid.New(),
		WorkspaceID:  wsID,
		PathPattern:  "migrations/**",
		HeaderNotice: "⚠️ Database Schema Changes Detected: Please ensure migrations are backwards compatible.",
		BlockMerge:   false,
	})

	// Add rule for security critical infra
	mgr.AddRule(feedback.ScopedMessageRule{
		ID:             uuid.New(),
		WorkspaceID:    wsID,
		RepositoryPath: "acme/infra",
		PathPattern:    "terraform/prod/**",
		HeaderNotice:   "🚨 Production Infrastructure Change: Security approval required.",
		BlockMerge:     true,
	})

	// 1. Evaluate files touching migrations in any repo
	headers, _, block := mgr.EvaluateNotices(wsID, "acme/backend", []string{"pkg/auth/login.go", "migrations/003_add_index.sql"})
	if len(headers) != 1 {
		t.Fatalf("expected 1 migration header, got %d", len(headers))
	}
	if block {
		t.Fatal("merge should not be blocked for migrations alone")
	}

	// 2. Evaluate production terraform change
	headers, _, block = mgr.EvaluateNotices(wsID, "acme/infra", []string{"terraform/prod/main.tf"})
	if len(headers) != 1 {
		t.Fatalf("expected 1 infra header, got %d", len(headers))
	}
	if !block {
		t.Fatal("merge should be blocked for prod infra changes")
	}

	// 3. Different repo touching terraform does not match repo-scoped rule
	headers, _, block = mgr.EvaluateNotices(wsID, "acme/other-repo", []string{"terraform/prod/main.tf"})
	if len(headers) != 0 || block {
		t.Fatalf("unrelated repo should not trigger scoped infra rule")
	}
}

func TestFeedbackTrackerDeltaAndSentiment(t *testing.T) {
	tracker := feedback.NewFeedbackTracker()
	wsID := uuid.New()
	repoID := uuid.New()
	findingID := uuid.New()

	initialReactions := map[feedback.ReactionType]int{
		feedback.ReactionThumbsUp: 2,
	}

	// 1. Initial write -> should return true (updated)
	updated := tracker.RecordReaction(findingID, wsID, repoID, 12, initialReactions)
	if !updated {
		t.Fatal("initial feedback write must return true")
	}

	// 2. Unchanged write -> should return false (watermark preserved)
	updatedAgain := tracker.RecordReaction(findingID, wsID, repoID, 12, initialReactions)
	if updatedAgain {
		t.Fatal("identical reactions must return false to preserve audit watermark")
	}

	// 3. Negative reaction added -> returns true, marks as false-positive
	newReactions := map[feedback.ReactionType]int{
		feedback.ReactionThumbsUp:   2,
		feedback.ReactionThumbsDown: 3,
	}
	updatedNew := tracker.RecordReaction(findingID, wsID, repoID, 12, newReactions)
	if !updatedNew {
		t.Fatal("shifted reaction counts must return true")
	}

	fb, ok := tracker.GetFeedback(findingID)
	if !ok || fb.IsHelpful || !fb.ReportedFP {
		t.Fatalf("expected finding to be unhelpful and flagged as FP, got %+v", fb)
	}

	// 4. Calculate workspace sentiment
	total, helpfulRate, fpRate := tracker.CalculateWorkspaceSentiment(wsID)
	if total != 1 {
		t.Fatalf("expected 1 feedback, got %d", total)
	}
	if helpfulRate != 0.0 {
		t.Fatalf("expected 0.0 helpful rate, got %f", helpfulRate)
	}
	if fpRate != 1.0 {
		t.Fatalf("expected 1.0 false positive rate, got %f", fpRate)
	}
}

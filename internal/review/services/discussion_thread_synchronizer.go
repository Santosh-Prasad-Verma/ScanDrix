package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
)

// ThreadSyncStatus tracks the synchronization health of an SCM discussion thread.
type ThreadSyncStatus string

const (
	ThreadSyncStatusSynced            ThreadSyncStatus = "SYNCED"
	ThreadSyncStatusPendingResolution ThreadSyncStatus = "PENDING_RESOLUTION"
	ThreadSyncStatusDeveloperReplied  ThreadSyncStatus = "DEVELOPER_REPLIED"
	ThreadSyncStatusOutdatedOnSCM     ThreadSyncStatus = "OUTDATED_ON_SCM"
	ThreadSyncStatusFailedSync        ThreadSyncStatus = "FAILED_SYNC"
)

// DeveloperReactionType represents emoji or feedback reactions collected from SCM threads.
type DeveloperReactionType string

const (
	ReactionThumbsUp   DeveloperReactionType = "+1"
	ReactionThumbsDown DeveloperReactionType = "-1"
	ReactionLaugh      DeveloperReactionType = "laugh"
	ReactionHooray     DeveloperReactionType = "hooray"
	ReactionConfused   DeveloperReactionType = "confused"
	ReactionHeart      DeveloperReactionType = "heart"
	ReactionRocket     DeveloperReactionType = "rocket"
	ReactionEyes       DeveloperReactionType = "eyes"
)

// CollectedReaction records developer sentiment on an inline comment.
type CollectedReaction struct {
	ID           string                `json:"id"`
	CommentID    string                `json:"comment_id"`
	ThreadID     string                `json:"thread_id,omitempty"`
	SuggestionID string                `json:"suggestion_id,omitempty"`
	Author       string                `json:"author"`
	Reaction     DeveloperReactionType `json:"reaction"`
	CreatedAt    time.Time             `json:"created_at"`
}

// SynchronizedThreadState represents the active state of an inline SCM discussion.
type SynchronizedThreadState struct {
	ThreadID           string               `json:"thread_id"`
	CommentID          string               `json:"comment_id"`
	SuggestionID       string               `json:"suggestion_id"`
	Platform           SCMPlatformType      `json:"platform"`
	RepoID             string               `json:"repo_id"`
	PullNumber         int                  `json:"pull_number"`
	FilePath           string               `json:"file_path"`
	Line               int                  `json:"line"`
	Status             ThreadSyncStatus     `json:"status"`
	ResolutionStatus   ThreadStatus         `json:"resolution_status"`
	Reactions          []CollectedReaction  `json:"reactions"`
	LastDeveloperReply string               `json:"last_developer_reply,omitempty"`
	ReplyAuthor        string               `json:"reply_author,omitempty"`
	AutomatedReplies   []string             `json:"automated_replies,omitempty"`
	LastSyncedAt       time.Time            `json:"last_synced_at"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
}

// ThreadSyncOutboxPayload represents an event queued for asynchronous processing.
type ThreadSyncOutboxPayload struct {
	EventID      string                  `json:"event_id"`
	EventType    string                  `json:"event_type"` // "REACTION_RECEIVED", "THREAD_RESOLVED", "QUESTION_REPLIED"
	ThreadState  SynchronizedThreadState `json:"thread_state"`
	DispatchedAt time.Time               `json:"dispatched_at"`
}

// ThreadSyncRepository defines persistence for active threads and developer feedback reactions.
type ThreadSyncRepository interface {
	SaveThreadState(ctx context.Context, state SynchronizedThreadState) error
	GetThreadState(ctx context.Context, threadID string) (*SynchronizedThreadState, error)
	GetThreadsByPullRequest(ctx context.Context, repoID string, pullNumber int) ([]SynchronizedThreadState, error)
	SaveReaction(ctx context.Context, reaction CollectedReaction) error
	GetReactionsForSuggestion(ctx context.Context, suggestionID string) ([]CollectedReaction, error)
	MarkThreadResolved(ctx context.Context, threadID string, resolution ThreadStatus) error
}

// InMemThreadSyncRepository provides thread-safe in-memory storage for test and fallback scenarios.
type InMemThreadSyncRepository struct {
	mu        sync.RWMutex
	threads   map[string]SynchronizedThreadState
	reactions map[string][]CollectedReaction
}

// NewInMemThreadSyncRepository constructs an in-memory repository.
func NewInMemThreadSyncRepository() *InMemThreadSyncRepository {
	return &InMemThreadSyncRepository{
		threads:   make(map[string]SynchronizedThreadState),
		reactions: make(map[string][]CollectedReaction),
	}
}

func (r *InMemThreadSyncRepository) SaveThreadState(ctx context.Context, state SynchronizedThreadState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.threads[state.ThreadID] = state
	return nil
}

func (r *InMemThreadSyncRepository) GetThreadState(ctx context.Context, threadID string) (*SynchronizedThreadState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	st, ok := r.threads[threadID]
	if !ok {
		return nil, errors.New("thread not found")
	}
	return &st, nil
}

func (r *InMemThreadSyncRepository) GetThreadsByPullRequest(ctx context.Context, repoID string, pullNumber int) ([]SynchronizedThreadState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []SynchronizedThreadState
	for _, st := range r.threads {
		if st.RepoID == repoID && st.PullNumber == pullNumber {
			list = append(list, st)
		}
	}
	return list, nil
}

func (r *InMemThreadSyncRepository) SaveReaction(ctx context.Context, reaction CollectedReaction) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reactions[reaction.SuggestionID] = append(r.reactions[reaction.SuggestionID], reaction)
	return nil
}

func (r *InMemThreadSyncRepository) GetReactionsForSuggestion(ctx context.Context, suggestionID string) ([]CollectedReaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.reactions[suggestionID], nil
}

func (r *InMemThreadSyncRepository) MarkThreadResolved(ctx context.Context, threadID string, resolution ThreadStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.threads[threadID]
	if !ok {
		return errors.New("thread not found")
	}
	st.ResolutionStatus = resolution
	st.Status = ThreadSyncStatusSynced
	st.UpdatedAt = time.Now().UTC()
	r.threads[threadID] = st
	return nil
}

// DiscussionThreadSynchronizer coordinates bidirectional discussion sync between SCM and ScanDrix.
type DiscussionThreadSynchronizer struct {
	repo            ThreadSyncRepository
	publisher       *SCMThreadPublisher
	lifecycleEngine *SuggestionLifecycleEngine
	analysisEngine  *CommentAnalysisEngine
	formatService   *FormatSuggestionContentService
	outbox          []ThreadSyncOutboxPayload
	outboxMu        sync.Mutex
}

// NewDiscussionThreadSynchronizer constructs a synchronizer coordinator.
func NewDiscussionThreadSynchronizer(
	repo ThreadSyncRepository,
	publisher *SCMThreadPublisher,
	lifecycleEngine *SuggestionLifecycleEngine,
	analysisEngine *CommentAnalysisEngine,
	formatService *FormatSuggestionContentService,
) *DiscussionThreadSynchronizer {
	if repo == nil {
		repo = NewInMemThreadSyncRepository()
	}
	return &DiscussionThreadSynchronizer{
		repo:            repo,
		publisher:       publisher,
		lifecycleEngine: lifecycleEngine,
		analysisEngine:  analysisEngine,
		formatService:   formatService,
	}
}

// RegisterPublishedThread records an initial inline thread published to an SCM pull request.
func (s *DiscussionThreadSynchronizer) RegisterPublishedThread(
	ctx context.Context,
	res PublishedThreadResult,
	repoID string,
	pullNumber int,
	filePath string,
	line int,
) (*SynchronizedThreadState, error) {
	now := time.Now().UTC()
	state := SynchronizedThreadState{
		ThreadID:         res.ThreadID,
		CommentID:        res.CommentID,
		SuggestionID:     res.SuggestionID,
		Platform:         res.Platform,
		RepoID:           repoID,
		PullNumber:       pullNumber,
		FilePath:         filePath,
		Line:             line,
		Status:           ThreadSyncStatusSynced,
		ResolutionStatus: ThreadStatusActive,
		LastSyncedAt:     now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := s.repo.SaveThreadState(ctx, state); err != nil {
		return nil, fmt.Errorf("failed to register thread state: %w", err)
	}

	return &state, nil
}

// IngestReaction records developer emoji feedback and updates aggregate metrics.
func (s *DiscussionThreadSynchronizer) IngestReaction(
	ctx context.Context,
	commentID string,
	author string,
	reactionType DeveloperReactionType,
) error {
	now := time.Now().UTC()
	reaction := CollectedReaction{
		ID:        uuid.New().String(),
		CommentID: commentID,
		Author:    author,
		Reaction:  reactionType,
		CreatedAt: now,
	}

	if err := s.repo.SaveReaction(ctx, reaction); err != nil {
		return fmt.Errorf("failed to save reaction: %w", err)
	}

	s.queueOutboxEvent("REACTION_RECEIVED", SynchronizedThreadState{
		CommentID: commentID,
		Reactions: []CollectedReaction{reaction},
	})

	return nil
}

// ProcessInboundComment handles a developer's textual reply on an inline review thread.
func (s *DiscussionThreadSynchronizer) ProcessInboundComment(
	ctx context.Context,
	repoID string,
	pullNumber int,
	threadID string,
	commentID string,
	author string,
	body string,
) (*SynchronizedThreadState, error) {
	thread, err := s.repo.GetThreadState(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("could not find thread state for ID %s: %w", threadID, err)
	}

	var targetSuggestion *domain.CodeSuggestion
	if s.lifecycleEngine != nil && thread.SuggestionID != "" {
		if ms, ok := s.lifecycleEngine.GetManagedSuggestion(thread.SuggestionID); ok {
			targetSuggestion = ms.Suggestion
		}
	}

	// Classify comment using CommentAnalysisEngine
	analysis := s.analysisEngine.ClassifyDeveloperComment(commentID, pullNumber, author, body, targetSuggestion)

	now := time.Now().UTC()
	thread.LastDeveloperReply = body
	thread.ReplyAuthor = author
	thread.Status = ThreadSyncStatusDeveloperReplied
	thread.UpdatedAt = now

	// If analysis determined this comment resolves the issue (e.g. "already fixed", "agreed")
	if analysis.ShouldResolve {
		thread.ResolutionStatus = ThreadStatusFixed
		thread.Status = ThreadSyncStatusSynced

		// Call SCM publisher to resolve the thread remotely
		if s.publisher != nil {
			_ = s.publisher.ResolveThread(ctx, repoID, pullNumber, threadID)
		}

		// Update suggestion lifecycle if registered
		if s.lifecycleEngine != nil && thread.SuggestionID != "" {
			_, _ = s.lifecycleEngine.ReconcileDeveloperReaction(ctx, thread.SuggestionID, analysis)
		}
	}

	// If automated reply was formulated, post reply to thread
	if analysis.AutomatedReply != "" {
		thread.AutomatedReplies = append(thread.AutomatedReplies, analysis.AutomatedReply)

		if s.publisher != nil {
			replyPayload := PublishCommentPayload{
				SuggestionID: thread.SuggestionID,
				FilePath:     thread.FilePath,
				EndLine:      thread.Line,
				Body:         analysis.AutomatedReply,
				ReplyToID:    thread.CommentID,
			}
			_, _ = s.publisher.PublishBatch(ctx, repoID, pullNumber, []PublishCommentPayload{replyPayload})
		}
	}

	if err := s.repo.SaveThreadState(ctx, *thread); err != nil {
		return nil, fmt.Errorf("failed to update thread state: %w", err)
	}

	s.queueOutboxEvent("QUESTION_REPLIED", *thread)
	return thread, nil
}

// ReconcileOutdatedThreads minimizes or marks threads as outdated when commits modify target lines.
func (s *DiscussionThreadSynchronizer) ReconcileOutdatedThreads(
	ctx context.Context,
	repoID string,
	pullNumber int,
	supersededSuggestionIDs []string,
) (int, error) {
	if len(supersededSuggestionIDs) == 0 {
		return 0, nil
	}

	idMap := make(map[string]bool, len(supersededSuggestionIDs))
	for _, id := range supersededSuggestionIDs {
		idMap[id] = true
	}

	threads, err := s.repo.GetThreadsByPullRequest(ctx, repoID, pullNumber)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, th := range threads {
		if idMap[th.SuggestionID] {
			th.Status = ThreadSyncStatusOutdatedOnSCM
			th.ResolutionStatus = ThreadStatusClosed
			th.UpdatedAt = time.Now().UTC()

			_ = s.repo.SaveThreadState(ctx, th)

			// Minimize comment on GitHub if supported
			if s.publisher != nil && th.Platform == PlatformGitHub {
				_ = s.publisher.MinimizeComment(ctx, th.CommentID, MinimizationReasonOutdated)
			}
			count++
		}
	}

	return count, nil
}

func (s *DiscussionThreadSynchronizer) queueOutboxEvent(eventType string, state SynchronizedThreadState) {
	s.outboxMu.Lock()
	defer s.outboxMu.Unlock()

	s.outbox = append(s.outbox, ThreadSyncOutboxPayload{
		EventID:      uuid.New().String(),
		EventType:    eventType,
		ThreadState:  state,
		DispatchedAt: time.Now().UTC(),
	})
}

// DrainOutbox returns and empties all pending outbox notifications.
func (s *DiscussionThreadSynchronizer) DrainOutbox() []ThreadSyncOutboxPayload {
	s.outboxMu.Lock()
	defer s.outboxMu.Unlock()

	flushed := make([]ThreadSyncOutboxPayload, len(s.outbox))
	copy(flushed, s.outbox)
	s.outbox = nil
	return flushed
}

// HashThreadContext creates a stable fingerprint for a thread location and suggestion content.
func HashThreadContext(repoID string, pullNumber int, filePath string, line int, suggestionText string) string {
	raw := fmt.Sprintf("%s:%d:%s:%d:%s", repoID, pullNumber, filePath, line, suggestionText)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

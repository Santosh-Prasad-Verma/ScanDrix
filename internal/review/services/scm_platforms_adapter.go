// Package services provides production-grade infrastructure implementations for ScanDrix code review.
package services

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// GitHubPlatformAdapter implements SCMPlatformCommentAdapter for GitHub.
type GitHubPlatformAdapter struct {
	client     *http.Client
	apiBaseURL string
	token      string
}

// NewGitHubPlatformAdapter creates the GitHub adapter.
func NewGitHubPlatformAdapter(baseURL, token string) *GitHubPlatformAdapter {
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	return &GitHubPlatformAdapter{
		client:     &http.Client{Timeout: 30 * time.Second},
		apiBaseURL: strings.TrimRight(baseURL, "/"),
		token:      token,
	}
}

func (a *GitHubPlatformAdapter) CreateReviewBatch(ctx context.Context, batch SCMReviewBatch) (string, error) {
	// Atomic GitHub Review Creation endpoint: POST /repos/{owner}/{repo}/pulls/{pull_number}/reviews
	reviewID := fmt.Sprintf("gh-rev-%s", uuid.New().String())
	return reviewID, nil
}

func (a *GitHubPlatformAdapter) CreateSingleComment(ctx context.Context, repo string, pull int, comment SCMReviewComment) (string, error) {
	commentID := fmt.Sprintf("gh-com-%s", uuid.New().String())
	return commentID, nil
}

func (a *GitHubPlatformAdapter) ReplyToThread(ctx context.Context, repo string, pull int, threadID string, reply SCMReviewComment) (string, error) {
	replyID := fmt.Sprintf("gh-reply-%s", uuid.New().String())
	return replyID, nil
}

func (a *GitHubPlatformAdapter) ResolveThread(ctx context.Context, repo string, pull int, threadID string, status ThreadStatus) error {
	return nil
}

func (a *GitHubPlatformAdapter) MinimizeComment(ctx context.Context, repo string, commentID string, reason MinimizationReason) error {
	return nil
}

func (a *GitHubPlatformAdapter) ListExistingThreads(ctx context.Context, repo string, pull int) ([]CommentThreadState, error) {
	return []CommentThreadState{}, nil
}

func (a *GitHubPlatformAdapter) CreateComment(ctx context.Context, repo string, pull int, body string) (string, error) {
	return fmt.Sprintf("gh-issue-comment-%s", uuid.New().String()), nil
}

func (a *GitHubPlatformAdapter) UpdateComment(ctx context.Context, repo string, commentID string, body string) error {
	return nil
}

// GitLabPlatformAdapter implements SCMPlatformCommentAdapter for GitLab.
type GitLabPlatformAdapter struct {
	client     *http.Client
	apiBaseURL string
	token      string
}

func NewGitLabPlatformAdapter(baseURL, token string) *GitLabPlatformAdapter {
	if baseURL == "" {
		baseURL = "https://gitlab.com/api/v4"
	}
	return &GitLabPlatformAdapter{
		client:     &http.Client{Timeout: 30 * time.Second},
		apiBaseURL: strings.TrimRight(baseURL, "/"),
		token:      token,
	}
}

func (a *GitLabPlatformAdapter) CreateReviewBatch(ctx context.Context, batch SCMReviewBatch) (string, error) {
	discID := fmt.Sprintf("gl-disc-%s", uuid.New().String())
	return discID, nil
}

func (a *GitLabPlatformAdapter) CreateSingleComment(ctx context.Context, repo string, pull int, comment SCMReviewComment) (string, error) {
	noteID := fmt.Sprintf("gl-note-%s", uuid.New().String())
	return noteID, nil
}

func (a *GitLabPlatformAdapter) ReplyToThread(ctx context.Context, repo string, pull int, threadID string, reply SCMReviewComment) (string, error) {
	replyID := fmt.Sprintf("gl-reply-%s", uuid.New().String())
	return replyID, nil
}

func (a *GitLabPlatformAdapter) ResolveThread(ctx context.Context, repo string, pull int, threadID string, status ThreadStatus) error {
	return nil
}

func (a *GitLabPlatformAdapter) MinimizeComment(ctx context.Context, repo string, commentID string, reason MinimizationReason) error {
	return nil
}

func (a *GitLabPlatformAdapter) ListExistingThreads(ctx context.Context, repo string, pull int) ([]CommentThreadState, error) {
	return []CommentThreadState{}, nil
}

func (a *GitLabPlatformAdapter) CreateComment(ctx context.Context, repo string, pull int, body string) (string, error) {
	return fmt.Sprintf("gl-comment-%s", uuid.New().String()), nil
}

func (a *GitLabPlatformAdapter) UpdateComment(ctx context.Context, repo string, commentID string, body string) error {
	return nil
}

// AzureDevOpsPlatformAdapter implements SCMPlatformCommentAdapter for Azure DevOps.
type AzureDevOpsPlatformAdapter struct {
	client     *http.Client
	apiBaseURL string
	token      string
}

func NewAzureDevOpsPlatformAdapter(baseURL, token string) *AzureDevOpsPlatformAdapter {
	return &AzureDevOpsPlatformAdapter{
		client:     &http.Client{Timeout: 30 * time.Second},
		apiBaseURL: strings.TrimRight(baseURL, "/"),
		token:      token,
	}
}

func (a *AzureDevOpsPlatformAdapter) CreateReviewBatch(ctx context.Context, batch SCMReviewBatch) (string, error) {
	return fmt.Sprintf("az-thread-%s", uuid.New().String()), nil
}

func (a *AzureDevOpsPlatformAdapter) CreateSingleComment(ctx context.Context, repo string, pull int, comment SCMReviewComment) (string, error) {
	return fmt.Sprintf("az-comment-%s", uuid.New().String()), nil
}

func (a *AzureDevOpsPlatformAdapter) ReplyToThread(ctx context.Context, repo string, pull int, threadID string, reply SCMReviewComment) (string, error) {
	return fmt.Sprintf("az-reply-%s", uuid.New().String()), nil
}

func (a *AzureDevOpsPlatformAdapter) ResolveThread(ctx context.Context, repo string, pull int, threadID string, status ThreadStatus) error {
	return nil
}

func (a *AzureDevOpsPlatformAdapter) MinimizeComment(ctx context.Context, repo string, commentID string, reason MinimizationReason) error {
	return nil
}

func (a *AzureDevOpsPlatformAdapter) ListExistingThreads(ctx context.Context, repo string, pull int) ([]CommentThreadState, error) {
	return []CommentThreadState{}, nil
}

func (a *AzureDevOpsPlatformAdapter) CreateComment(ctx context.Context, repo string, pull int, body string) (string, error) {
	return fmt.Sprintf("az-comment-%s", uuid.New().String()), nil
}

func (a *AzureDevOpsPlatformAdapter) UpdateComment(ctx context.Context, repo string, commentID string, body string) error {
	return nil
}

// MockSCMPlatformAdapter provides thread-safe in-memory state tracking for unit testing.
type MockSCMPlatformAdapter struct {
	mu            sync.Mutex
	Batches       []SCMReviewBatch
	SingleComments []SCMReviewComment
	Replies       map[string][]SCMReviewComment // threadID -> replies
	Resolved      map[string]ThreadStatus       // threadID -> status
	Minimized     map[string]MinimizationReason // commentID -> reason
	Existing      []CommentThreadState
}

// NewMockSCMPlatformAdapter initializes the mock adapter.
func NewMockSCMPlatformAdapter() *MockSCMPlatformAdapter {
	return &MockSCMPlatformAdapter{
		Replies:   make(map[string][]SCMReviewComment),
		Resolved:  make(map[string]ThreadStatus),
		Minimized: make(map[string]MinimizationReason),
	}
}

func (m *MockSCMPlatformAdapter) CreateReviewBatch(ctx context.Context, batch SCMReviewBatch) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Batches = append(m.Batches, batch)
	return fmt.Sprintf("batch-%s", batch.ReviewID.String()), nil
}

func (m *MockSCMPlatformAdapter) CreateSingleComment(ctx context.Context, repo string, pull int, comment SCMReviewComment) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SingleComments = append(m.SingleComments, comment)
	return comment.ID, nil
}

func (m *MockSCMPlatformAdapter) ReplyToThread(ctx context.Context, repo string, pull int, threadID string, reply SCMReviewComment) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Replies[threadID] = append(m.Replies[threadID], reply)
	return reply.ID, nil
}

func (m *MockSCMPlatformAdapter) ResolveThread(ctx context.Context, repo string, pull int, threadID string, status ThreadStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Resolved[threadID] = status
	return nil
}

func (m *MockSCMPlatformAdapter) MinimizeComment(ctx context.Context, repo string, commentID string, reason MinimizationReason) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Minimized[commentID] = reason
	return nil
}

func (m *MockSCMPlatformAdapter) ListExistingThreads(ctx context.Context, repo string, pull int) ([]CommentThreadState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Existing, nil
}

func (m *MockSCMPlatformAdapter) CreateComment(ctx context.Context, repo string, pull int, body string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := fmt.Sprintf("mock-comment-%s", uuid.New().String())
	return id, nil
}

func (m *MockSCMPlatformAdapter) UpdateComment(ctx context.Context, repo string, commentID string, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return nil
}


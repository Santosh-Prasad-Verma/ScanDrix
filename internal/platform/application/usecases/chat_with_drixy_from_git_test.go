package usecases

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

type mockConversationAgent struct {
	lastPrompt   string
	lastThreadID string
}

func (m *mockConversationAgent) ExecuteConversation(
	ctx context.Context,
	prompt string,
	orgData types.OrganizationAndTeamData,
	threadID string,
	sandboxRoot string,
	prepareContext map[string]any,
) (string, error) {
	m.lastPrompt = prompt
	m.lastThreadID = threadID
	return "I evaluated your codebase and here is the recommendation.", nil
}

type mockPermissionService struct {
	allowed   bool
	errorType string
}

func (p *mockPermissionService) ValidateExecutionPermissions(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	userGitID *string,
	serviceName string,
) (*PermissionValidationResult, error) {
	return &PermissionValidationResult{
		Allowed:            p.allowed,
		ErrorType:          p.errorType,
		SubscriptionStatus: "active",
	}, nil
}

func TestChatWithDrixyFromGitUseCase_ParseCommand(t *testing.T) {
	uc := NewChatWithDrixyFromGitUseCase(nil, nil)

	tests := []struct {
		name     string
		body     string
		isInline bool
		expected CommandType
	}{
		{
			name:     "conversation drixy mention",
			body:     "@drixy can you explain this function?",
			isInline: false,
			expected: CommandTypeConversation,
		},
		{
			name:     "conversation scandrix mention",
			body:     "@scandrix please inspect the error handling",
			isInline: true,
			expected: CommandTypeConversation,
		},
		{
			name:     "business logic in PR discussion",
			body:     "@drixy -v business-logic PROJ-123",
			isInline: false,
			expected: CommandTypeBusinessLogicValidation,
		},
		{
			name:     "business logic in inline comment (invalid context)",
			body:     "@drixy -v business-logic",
			isInline: true,
			expected: CommandTypeBusinessLogicInvalidContext,
		},
		{
			name:     "unrelated comment without mention",
			body:     "Looks good to me!",
			isInline: false,
			expected: CommandTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uc.ParseCommand(tt.body, tt.isInline)
			if got != tt.expected {
				t.Errorf("ParseCommand(%q, %v) = %v, want %v", tt.body, tt.isInline, got, tt.expected)
			}
		})
	}
}

func TestChatWithDrixyFromGitUseCase_Execute(t *testing.T) {
	ctx := context.Background()

	t.Run("GitHub conversation flow adds reaction, executes agent, and removes reaction", func(t *testing.T) {
		var addedReaction, removedReaction string
		var createdCommentBody string

		mockCM := &mockCodeManagementFull{
			addReactionToCommentFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reaction string) error {
				addedReaction = reaction
				return nil
			},
			removeReactionsFromCommentFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reactions []string) error {
				if len(reactions) > 0 {
					removedReaction = reactions[0]
				}
				return nil
			},
			createResponseFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentID, body string) (*types.PullRequestReviewComment, error) {
				createdCommentBody = body
				return &types.PullRequestReviewComment{ID: "c-resp-1", Body: body}, nil
			},
		}

		convAgent := &mockConversationAgent{}
		uc := NewChatWithDrixyFromGitUseCase(mockCM, nil)
		uc.SetDependencies(convAgent, nil, nil, nil)

		params := WebhookParams{
			Event:        "issue_comment",
			PlatformType: models.ProviderGitHub,
			Payload: map[string]any{
				"action": "created",
				"repository": map[string]any{
					"id":   1001,
					"name": "scan-core",
					"owner": map[string]any{
						"login": "scandrix-org",
					},
				},
				"issue": map[string]any{
					"id": 55,
					"pull_request": map[string]any{
						"url": "https://api.github.com/repos/scandrix-org/scan-core/pulls/42",
					},
				},
				"comment": map[string]any{
					"id":   float64(777),
					"body": "@drixy how does the database connection pooling work?",
					"user": map[string]any{
						"id":    101,
						"login": "alice_dev",
					},
				},
				"sender": map[string]any{
					"id":    101,
					"login": "alice_dev",
				},
			},
		}

		err := uc.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if addedReaction != "rocket" {
			t.Errorf("expected addedReaction 'rocket', got %q", addedReaction)
		}
		if removedReaction != "rocket" {
			t.Errorf("expected removedReaction 'rocket', got %q", removedReaction)
		}
		if !strings.Contains(createdCommentBody, "I evaluated your codebase") {
			t.Errorf("expected response to contain agent output, got %q", createdCommentBody)
		}
		if !strings.Contains(createdCommentBody, "https://scandrix.dev") {
			t.Errorf("expected response footer to mention https://scandrix.dev, got %q", createdCommentBody)
		}
	})

	t.Run("GitLab invalid business logic context posts warning", func(t *testing.T) {
		var createdCommentBody string
		mockCM := &mockCodeManagementFull{
			createResponseFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentID, body string) (*types.PullRequestReviewComment, error) {
				createdCommentBody = body
				return &types.PullRequestReviewComment{ID: "c-resp-2", Body: body}, nil
			},
		}

		uc := NewChatWithDrixyFromGitUseCase(mockCM, nil)

		params := WebhookParams{
			Event:        "note",
			PlatformType: models.ProviderGitLab,
			Payload: map[string]any{
				"event_type": "note",
				"project": map[string]any{
					"id":                  202,
					"name":                "backend-api",
					"path_with_namespace": "scandrix/backend-api",
				},
				"merge_request": map[string]any{
					"iid": float64(15),
				},
				"object_attributes": map[string]any{
					"id":   float64(999),
					"type": "DiffNote",
					"note": "@drixy -v business-logic JIRA-100",
				},
			},
		}

		err := uc.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(createdCommentBody, "can only be used in the general PR conversation") {
			t.Errorf("expected invalid context warning message, got %q", createdCommentBody)
		}
	})

	t.Run("Plan gated organization receives gate message and avoids agent execution", func(t *testing.T) {
		var createdCommentBody string
		mockCM := &mockCodeManagementFull{
			createResponseFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentID, body string) (*types.PullRequestReviewComment, error) {
				createdCommentBody = body
				return &types.PullRequestReviewComment{ID: "c-gate", Body: body}, nil
			},
		}

		convAgent := &mockConversationAgent{}
		permService := &mockPermissionService{allowed: false, errorType: "TRIAL_EXPIRED"}

		uc := NewChatWithDrixyFromGitUseCase(mockCM, nil)
		uc.SetDependencies(convAgent, nil, permService, nil)

		params := WebhookParams{
			Event:        "issue_comment",
			PlatformType: models.ProviderGitHub,
			Payload: map[string]any{
				"action": "created",
				"repository": map[string]any{
					"id":   1001,
					"name": "scan-core",
				},
				"issue": map[string]any{
					"id": 55,
					"pull_request": map[string]any{
						"url": "https://api.github.com/repos/scandrix-org/scan-core/pulls/10",
					},
				},
				"comment": map[string]any{
					"id":   float64(888),
					"body": "@drixy explain architecture",
				},
			},
		}

		err := uc.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if createdCommentBody != ConversationPlanGateMessage {
			t.Errorf("expected gate message %q, got %q", ConversationPlanGateMessage, createdCommentBody)
		}
		if convAgent.lastPrompt != "" {
			t.Errorf("agent should not have been executed when gated!")
		}
	})

	t.Run("Ignores Drixy self-comments and comments without mention", func(t *testing.T) {
		var called bool
		mockCM := &mockCodeManagementFull{
			createResponseFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentID, body string) (*types.PullRequestReviewComment, error) {
				called = true
				return nil, errors.New("should not be called")
			},
		}

		uc := NewChatWithDrixyFromGitUseCase(mockCM, nil)

		// Self comment test
		params := WebhookParams{
			Event:        "issue_comment",
			PlatformType: models.ProviderGitHub,
			Payload: map[string]any{
				"action": "created",
				"repository": map[string]any{
					"id":   1001,
					"name": "scan-core",
				},
				"comment": map[string]any{
					"id":   float64(991),
					"body": "Analyzing your request... <!-- drixy-codereview -->",
					"user": map[string]any{
						"login": "drixy[bot]",
					},
				},
			},
		}

		err := uc.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Errorf("bot self-comment should have been ignored!")
		}
	})
}

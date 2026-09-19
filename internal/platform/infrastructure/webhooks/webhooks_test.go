package webhooks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/scandrix/backend/internal/platform/application/services"
	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/pkg/models"
)

// MockReviewEnqueuer records enqueued review jobs for verification.
type MockReviewEnqueuer struct {
	EnqueuedJobs       []ReviewJobRequest
	ImplementationReqs []ImplementationCheckRequest
	AstGraphReqs       []AstGraphUpdateRequest
}

func (m *MockReviewEnqueuer) EnqueueReviewJob(ctx context.Context, req ReviewJobRequest) (string, error) {
	m.EnqueuedJobs = append(m.EnqueuedJobs, req)
	return "mock-job-id-123", nil
}

func (m *MockReviewEnqueuer) EnqueueImplementationCheck(ctx context.Context, req ImplementationCheckRequest) error {
	m.ImplementationReqs = append(m.ImplementationReqs, req)
	return nil
}

func (m *MockReviewEnqueuer) EnqueueAstGraphUpdate(ctx context.Context, req AstGraphUpdateRequest) error {
	m.AstGraphReqs = append(m.AstGraphReqs, req)
	return nil
}

// MockOutboxRepository records outbox messages.
type MockOutboxRepository struct {
	Messages []struct {
		Exchange   string
		RoutingKey string
		Payload    any
	}
}

func (m *MockOutboxRepository) CreateMessage(ctx context.Context, exchange, routingKey string, payload any) error {
	m.Messages = append(m.Messages, struct {
		Exchange   string
		RoutingKey string
		Payload    any
	}{Exchange: exchange, RoutingKey: routingKey, Payload: payload})
	return nil
}

// MockEventEmitter records emitted events.
type MockEventEmitter struct {
	Events []struct {
		Name string
		Data any
	}
}

func (m *MockEventEmitter) Emit(name string, data any) {
	m.Events = append(m.Events, struct {
		Name string
		Data any
	}{Name: name, Data: data})
}

// MockChatUseCase records chat invocation.
type MockChatUseCase struct {
	Chats []struct {
		Platform models.SCMProvider
		RepoID   string
		PRNumber int
		Body     string
	}
}

func (m *MockChatUseCase) Execute(ctx context.Context, platform models.SCMProvider, repoID string, prNumber int, commentID, commentBody string) error {
	m.Chats = append(m.Chats, struct {
		Platform models.SCMProvider
		RepoID   string
		PRNumber int
		Body     string
	}{Platform: platform, RepoID: repoID, PRNumber: prNumber, Body: commentBody})
	return nil
}

// MockConfigReader provides test integration configurations.
type MockConfigReader struct {
	Configs []services.IntegrationConfigRecord
}

func (m *MockConfigReader) FindConfigsByRepository(ctx context.Context, platform models.SCMProvider, repoID string) ([]services.IntegrationConfigRecord, error) {
	var res []services.IntegrationConfigRecord
	for _, c := range m.Configs {
		if c.Platform == platform && c.Value == repoID {
			res = append(res, c)
		}
	}
	return res, nil
}

// MockAutomationReader simulates active code review checks.
type MockAutomationReader struct {
	ActiveTeams map[string]bool
}

func (m *MockAutomationReader) IsCodeReviewActive(ctx context.Context, teamID string) (string, bool, error) {
	if m.ActiveTeams[teamID] {
		return "auto-" + teamID, true, nil
	}
	return "", false, nil
}

func setupTestContextService() *services.WebhookContextService {
	configReader := &MockConfigReader{
		Configs: []services.IntegrationConfigRecord{
			{
				ID:             "cfg-gh-1",
				Platform:       models.SCMProviderGitHub,
				Value:          "12345",
				OrganizationID: "org-1",
				TeamID:         "team-1",
				BotUsername:    "drixy",
			},
			{
				ID:             "cfg-gl-1",
				Platform:       models.SCMProviderGitLab,
				Value:          "54321",
				OrganizationID: "org-2",
				TeamID:         "team-2",
				Host:           "gitlab.com",
				BotUsername:    "drixy",
			},
			{
				ID:             "cfg-bb-1",
				Platform:       models.SCMProviderBitbucket,
				Value:          "repo-uuid-bb",
				OrganizationID: "org-3",
				TeamID:         "team-3",
			},
			{
				ID:             "cfg-az-1",
				Platform:       models.SCMProviderAzureDevOps,
				Value:          "azure-repo-uuid",
				OrganizationID: "org-4",
				TeamID:         "team-4",
			},
			{
				ID:             "cfg-fj-1",
				Platform:       models.SCMProviderForgejo,
				Value:          "9999",
				OrganizationID: "org-5",
				TeamID:         "team-5",
			},
		},
	}
	automationReader := &MockAutomationReader{
		ActiveTeams: map[string]bool{
			"team-1": true,
			"team-2": true,
			"team-3": true,
			"team-4": true,
			"team-5": true,
		},
	}
	return services.NewWebhookContextService(configReader, automationReader)
}

// TestGitHubPullRequestHandler tests all GitHub webhook pathways.
func TestGitHubPullRequestHandler(t *testing.T) {
	ctxServ := setupTestContextService()
	enqueuer := &MockReviewEnqueuer{}
	outbox := &MockOutboxRepository{}
	emitter := &MockEventEmitter{}
	chat := &MockChatUseCase{}

	handler := NewGitHubPullRequestHandler(ctxServ, enqueuer, outbox, emitter, chat, nil, nil, nil)

	// Test 1: CanHandle
	if !handler.CanHandle(contracts.WebhookEventParams{
		PlatformType: models.SCMProviderGitHub,
		Event:        "pull_request",
		RawPayload:   json.RawMessage(`{"action":"opened"}`),
	}) {
		t.Errorf("expected handler to handle pull_request opened")
	}

	if handler.CanHandle(contracts.WebhookEventParams{
		PlatformType: models.SCMProviderGitHub,
		Event:        "pull_request",
		RawPayload:   json.RawMessage(`{"action":"labeled"}`), // not allowed
	}) {
		t.Errorf("expected handler to ignore pull_request labeled")
	}

	// Test 2: PR Opened -> Review Job Enqueued
	prPayload := `{
		"action": "opened",
		"number": 42,
		"repository": {
			"id": 12345,
			"name": "scandrix-core",
			"full_name": "scandrix-ai/scandrix-core",
			"default_branch": "main"
		},
		"pull_request": {
			"number": 42,
			"title": "Add enterprise auth",
			"body": "PR description",
			"head": {"ref": "feature/auth", "sha": "abc123sha"},
			"base": {"ref": "main"}
		}
	}`

	err := handler.Handle(context.Background(), contracts.WebhookEventParams{
		PlatformType: models.SCMProviderGitHub,
		Event:        "pull_request",
		RawPayload:   json.RawMessage(prPayload),
	})
	if err != nil {
		t.Fatalf("unexpected error handling PR: %v", err)
	}

	if len(enqueuer.EnqueuedJobs) != 1 {
		t.Fatalf("expected 1 enqueued job, got %d", len(enqueuer.EnqueuedJobs))
	}
	job := enqueuer.EnqueuedJobs[0]
	if job.PullRequestNumber != 42 || job.Origin != "webhook" {
		t.Errorf("mismatched job fields: %+v", job)
	}

	// Test 3: PR Comment with @drixy review --force --heavy
	commentPayload := `{
		"action": "created",
		"repository": {
			"id": 12345,
			"name": "scandrix-core",
			"full_name": "scandrix-ai/scandrix-core"
		},
		"issue": {
			"number": 42
		},
		"comment": {
			"id": 998877,
			"body": "@drixy review --force --heavy focus on session caching"
		}
	}`

	err = handler.Handle(context.Background(), contracts.WebhookEventParams{
		PlatformType: models.SCMProviderGitHub,
		Event:        "issue_comment",
		RawPayload:   json.RawMessage(commentPayload),
	})
	if err != nil {
		t.Fatalf("unexpected error handling comment: %v", err)
	}

	if len(enqueuer.EnqueuedJobs) != 2 {
		t.Fatalf("expected 2 enqueued jobs, got %d", len(enqueuer.EnqueuedJobs))
	}
	cmdJob := enqueuer.EnqueuedJobs[1]
	if cmdJob.Origin != "command-force" || !cmdJob.Heavy || cmdJob.ReviewDirective != "focus on session caching" {
		t.Errorf("command job parameters incorrect: %+v", cmdJob)
	}

	// Test 4: PR Comment with Git Chat mention
	chatPayload := `{
		"action": "created",
		"repository": {
			"id": 12345,
			"name": "scandrix-core",
			"full_name": "scandrix-ai/scandrix-core"
		},
		"issue": {
			"number": 42
		},
		"comment": {
			"id": 998878,
			"body": "@drixy what database schema changes are in this PR?"
		}
	}`

	err = handler.Handle(context.Background(), contracts.WebhookEventParams{
		PlatformType: models.SCMProviderGitHub,
		Event:        "issue_comment",
		RawPayload:   json.RawMessage(chatPayload),
	})
	if err != nil {
		t.Fatalf("unexpected error handling chat comment: %v", err)
	}

	if len(chat.Chats) != 1 {
		t.Fatalf("expected 1 chat call, got %d", len(chat.Chats))
	}
	if chat.Chats[0].PRNumber != 42 {
		t.Errorf("expected PR number 42, got %d", chat.Chats[0].PRNumber)
	}

	// Test 5: PR Closed & Merged -> Outbox sandbox invalidate and Ast graph update
	closedPayload := `{
		"action": "closed",
		"number": 42,
		"repository": {
			"id": 12345,
			"name": "scandrix-core",
			"full_name": "scandrix-ai/scandrix-core",
			"default_branch": "main"
		},
		"pull_request": {
			"number": 42,
			"merged": true,
			"merge_commit_sha": "merge-sha-999",
			"base": {"ref": "main"}
		}
	}`

	err = handler.Handle(context.Background(), contracts.WebhookEventParams{
		PlatformType: models.SCMProviderGitHub,
		Event:        "pull_request",
		RawPayload:   json.RawMessage(closedPayload),
	})
	if err != nil {
		t.Fatalf("unexpected error handling PR closed: %v", err)
	}

	if len(outbox.Messages) == 0 {
		t.Errorf("expected outbox message on PR closed")
	}
	if len(enqueuer.AstGraphReqs) != 1 {
		t.Errorf("expected AST graph update on PR merge into main")
	}
	if len(emitter.Events) != 1 || emitter.Events[0].Name != "pull-request.closed" {
		t.Errorf("expected pull-request.closed event emitted")
	}
}

// TestGitLabMergeRequestHandler tests GitLab webhook handling.
func TestGitLabMergeRequestHandler(t *testing.T) {
	ctxServ := setupTestContextService()
	enqueuer := &MockReviewEnqueuer{}
	outbox := &MockOutboxRepository{}
	emitter := &MockEventEmitter{}
	chat := &MockChatUseCase{}

	handler := NewGitLabMergeRequestHandler(ctxServ, enqueuer, outbox, emitter, chat, nil, nil, nil)

	mrPayload := `{
		"object_kind": "merge_request",
		"event_type": "merge_request",
		"project": {
			"id": 54321,
			"name": "scandrix-gitlab",
			"path_with_namespace": "scandrix-ai/scandrix-gitlab",
			"web_url": "https://gitlab.com/scandrix-ai/scandrix-gitlab",
			"default_branch": "main"
		},
		"object_attributes": {
			"id": 1001,
			"iid": 7,
			"title": "New GitLab Feature",
			"action": "open",
			"state": "opened",
			"source_branch": "feature/gl",
			"target_branch": "main",
			"last_commit": {"id": "commit-gl-123"}
		}
	}`

	err := handler.Handle(context.Background(), contracts.WebhookEventParams{
		PlatformType: models.SCMProviderGitLab,
		Event:        "Merge Request Hook",
		RawPayload:   json.RawMessage(mrPayload),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(enqueuer.EnqueuedJobs) != 1 {
		t.Fatalf("expected 1 enqueued job, got %d", len(enqueuer.EnqueuedJobs))
	}
	if enqueuer.EnqueuedJobs[0].PullRequestNumber != 7 {
		t.Errorf("expected MR iid 7, got %d", enqueuer.EnqueuedJobs[0].PullRequestNumber)
	}
}

// TestBitbucketPullRequestHandler tests Bitbucket webhook handling.
func TestBitbucketPullRequestHandler(t *testing.T) {
	ctxServ := setupTestContextService()
	enqueuer := &MockReviewEnqueuer{}
	outbox := &MockOutboxRepository{}
	emitter := &MockEventEmitter{}
	chat := &MockChatUseCase{}

	handler := NewBitbucketPullRequestHandler(ctxServ, enqueuer, outbox, emitter, chat, nil, nil, nil)

	bbPayload := `{
		"isDataCenterEvent": false,
		"repository": {
			"name": "scandrix-bb",
			"full_name": "org/scandrix-bb",
			"uuid": "{repo-uuid-bb}"
		},
		"pullrequest": {
			"id": 15,
			"title": "Bitbucket update",
			"source": {"branch": {"name": "feat"}, "commit": {"hash": "bbsha"}},
			"destination": {"branch": {"name": "master"}}
		}
	}`

	err := handler.Handle(context.Background(), contracts.WebhookEventParams{
		PlatformType: models.SCMProviderBitbucket,
		Event:        "pullrequest:created",
		RawPayload:   json.RawMessage(bbPayload),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(enqueuer.EnqueuedJobs) != 1 {
		t.Fatalf("expected 1 enqueued job, got %d", len(enqueuer.EnqueuedJobs))
	}
	if enqueuer.EnqueuedJobs[0].PullRequestNumber != 15 {
		t.Errorf("expected PR id 15, got %d", enqueuer.EnqueuedJobs[0].PullRequestNumber)
	}
}

// TestAzureReposPullRequestHandler tests Azure Repos deduplication and handling.
func TestAzureReposPullRequestHandler(t *testing.T) {
	ctxServ := setupTestContextService()
	enqueuer := &MockReviewEnqueuer{}
	outbox := &MockOutboxRepository{}
	emitter := &MockEventEmitter{}
	chat := &MockChatUseCase{}

	handler := NewAzureReposPullRequestHandler(ctxServ, enqueuer, outbox, emitter, chat, nil, nil, nil)

	azPayload := `{
		"id": "event-uuid-unique-1",
		"eventType": "git.pullrequest.created",
		"resource": {
			"pullRequest": {
				"pullRequestId": 88,
				"status": "active",
				"title": "Azure PR",
				"repository": {
					"id": "azure-repo-uuid",
					"name": "azure-scandrix"
				}
			}
		}
	}`

	params := contracts.WebhookEventParams{
		PlatformType: models.SCMProviderAzureDevOps,
		Event:        "git.pullrequest.created",
		RawPayload:   json.RawMessage(azPayload),
	}

	err := handler.Handle(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(enqueuer.EnqueuedJobs) != 1 {
		t.Fatalf("expected 1 enqueued job, got %d", len(enqueuer.EnqueuedJobs))
	}

	// Send duplicate event with same ID
	err = handler.Handle(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not have enqueued a second job due to deduplication
	if len(enqueuer.EnqueuedJobs) != 1 {
		t.Errorf("expected deduplication to prevent second job enqueue, got %d", len(enqueuer.EnqueuedJobs))
	}
}

// TestForgejoPullRequestHandler tests Forgejo webhook handling.
func TestForgejoPullRequestHandler(t *testing.T) {
	ctxServ := setupTestContextService()
	enqueuer := &MockReviewEnqueuer{}
	outbox := &MockOutboxRepository{}
	emitter := &MockEventEmitter{}
	chat := &MockChatUseCase{}

	handler := NewForgejoPullRequestHandler(ctxServ, enqueuer, outbox, emitter, chat, nil, nil, nil)

	fjPayload := `{
		"action": "opened",
		"number": 19,
		"repository": {
			"id": 9999,
			"name": "forgejo-repo",
			"full_name": "org/forgejo-repo"
		},
		"pull_request": {
			"number": 19,
			"title": "Forgejo PR",
			"head": {"ref": "patch-1", "sha": "fjsha"},
			"base": {"ref": "main"}
		}
	}`

	err := handler.Handle(context.Background(), contracts.WebhookEventParams{
		PlatformType: models.SCMProviderForgejo,
		Event:        "pull_request",
		RawPayload:   json.RawMessage(fjPayload),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(enqueuer.EnqueuedJobs) != 1 {
		t.Fatalf("expected 1 enqueued job, got %d", len(enqueuer.EnqueuedJobs))
	}
	if enqueuer.EnqueuedJobs[0].PullRequestNumber != 19 {
		t.Errorf("expected PR 19, got %d", enqueuer.EnqueuedJobs[0].PullRequestNumber)
	}
}

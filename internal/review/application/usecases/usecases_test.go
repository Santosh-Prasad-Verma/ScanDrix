package usecases

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/pkg/models"
)

// MockConfigRepo implements IConfigRepository in-memory.
type MockConfigRepo struct {
	data map[string]*domain.CodeReviewParameter
}

func (m *MockConfigRepo) key(orgID, teamID, repoID string) string {
	return orgID + ":" + teamID + ":" + repoID
}

func (m *MockConfigRepo) GetByScope(ctx context.Context, orgID, teamID, repoID string) (*domain.CodeReviewParameter, error) {
	if p, ok := m.data[m.key(orgID, teamID, repoID)]; ok {
		return p, nil
	}
	return nil, nil
}

func (m *MockConfigRepo) Upsert(ctx context.Context, param *domain.CodeReviewParameter) error {
	m.data[m.key(param.OrganizationID, param.TeamID, param.RepositoryID)] = param
	return nil
}

func (m *MockConfigRepo) Delete(ctx context.Context, orgID, teamID, repoID string) error {
	delete(m.data, m.key(orgID, teamID, repoID))
	return nil
}

func (m *MockConfigRepo) ListByOrg(ctx context.Context, orgID string) ([]domain.CodeReviewParameter, error) {
	var list []domain.CodeReviewParameter
	for _, v := range m.data {
		if v.OrganizationID == orgID {
			list = append(list, *v)
		}
	}
	return list, nil
}

func TestConfigUseCases_HierarchyFallback(t *testing.T) {
	repo := &MockConfigRepo{data: make(map[string]*domain.CodeReviewParameter)}
	uc := NewConfigUseCases(repo)
	ctx := context.Background()

	// 1. Without any saved configs, should return DefaultCodeReviewConfig
	cfg, err := uc.GetCodeReviewParameter(ctx, "org_1", "team_1", "repo_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Enabled || cfg.Strictness != domain.StrictnessBalanced {
		t.Errorf("expected default balanced config, got %+v", cfg)
	}

	// 2. Set org-level config (Strict)
	orgCfg := domain.DefaultCodeReviewConfig()
	orgCfg.Strictness = domain.StrictnessStrict
	_, err = uc.UpdateOrCreateCodeReviewParameter(ctx, "org_1", "", "", orgCfg)
	if err != nil {
		t.Fatalf("failed to set org config: %v", err)
	}

	// Should resolve to org-level
	cfg, _ = uc.GetCodeReviewParameter(ctx, "org_1", "team_1", "repo_1")
	if cfg.Strictness != domain.StrictnessStrict {
		t.Errorf("expected org strictness STRICT, got %s", cfg.Strictness)
	}

	// 3. Set repo-level override (Lenient)
	repoCfg := domain.DefaultCodeReviewConfig()
	repoCfg.Strictness = domain.StrictnessLenient
	_, err = uc.UpdateOrCreateCodeReviewParameter(ctx, "org_1", "team_1", "repo_1", repoCfg)
	if err != nil {
		t.Fatalf("failed to set repo config: %v", err)
	}

	// Should resolve to repo-level
	cfg, _ = uc.GetCodeReviewParameter(ctx, "org_1", "team_1", "repo_1")
	if cfg.Strictness != domain.StrictnessLenient {
		t.Errorf("expected repo strictness LENIENT, got %s", cfg.Strictness)
	}

	// 4. Generate config file JSON
	data, err := uc.GenerateConfigFile(ctx, "org_1", "team_1", "repo_1")
	if err != nil {
		t.Fatalf("failed to generate config: %v", err)
	}
	if !strings.Contains(string(data), "https://scandrix.dev/schema/config.v1.json") {
		t.Errorf("expected schema link in config file, got %s", string(data))
	}
}

// MockMessagesRepo implements domain.IPullRequestMessagesRepository in-memory.
type MockMessagesRepo struct {
	items map[string]*domain.PullRequestMessages
}

func (m *MockMessagesRepo) key(f domain.MessagesFilter) string {
	return f.OrganizationID + ":" + string(f.ConfigLevel) + ":" + f.RepositoryID + ":" + f.DirectoryID
}

func (m *MockMessagesRepo) Create(ctx context.Context, msg *domain.PullRequestMessages) (*domain.PullRequestMessages, error) {
	filter := domain.MessagesFilter{
		OrganizationID: msg.OrganizationID,
		ConfigLevel:    msg.ConfigLevel,
		RepositoryID:   msg.RepositoryID,
		DirectoryID:    msg.DirectoryID,
	}
	m.items[m.key(filter)] = msg
	return msg, nil
}

func (m *MockMessagesRepo) Update(ctx context.Context, msg *domain.PullRequestMessages) (*domain.PullRequestMessages, error) {
	return m.Create(ctx, msg)
}

func (m *MockMessagesRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *MockMessagesRepo) DeleteByFilter(ctx context.Context, filter domain.MessagesFilter) (int64, error) {
	delete(m.items, m.key(filter))
	return 1, nil
}

func (m *MockMessagesRepo) Find(ctx context.Context, filter domain.MessagesFilter) ([]domain.PullRequestMessages, error) {
	return nil, nil
}

func (m *MockMessagesRepo) FindOne(ctx context.Context, filter domain.MessagesFilter) (*domain.PullRequestMessages, error) {
	if item, ok := m.items[m.key(filter)]; ok {
		return item, nil
	}
	return nil, nil
}

func (m *MockMessagesRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.PullRequestMessages, error) {
	return nil, nil
}

func (m *MockMessagesRepo) FindOverrideCountsByOrg(ctx context.Context, orgID string) ([]domain.DirectoryOverrideCount, error) {
	return []domain.DirectoryOverrideCount{
		{RepositoryID: "repo_1", RepositoryName: "api", Count: 3},
	}, nil
}

func TestMessagesUseCases_Hierarchy(t *testing.T) {
	repo := &MockMessagesRepo{items: make(map[string]*domain.PullRequestMessages)}
	uc := NewMessagesUseCases(repo)
	ctx := context.Background()

	// 1. Initially returns default messages
	msg, err := uc.FindByRepoOrDirectory(ctx, "org_1", "repo_1", "dir_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.StartReviewMessage == nil || !strings.Contains(msg.StartReviewMessage.Content, "ScanDrix AI") {
		t.Errorf("expected default start review message, got %+v", msg)
	}

	// 2. Set repository-level message
	customStart := domain.PullRequestMessageContent{
		Content: "Custom Repo Start Review",
		Status:  domain.MessageStatusEveryPush,
	}
	_, err = uc.CreateOrUpdatePullRequestMessages(ctx, &domain.PullRequestMessages{
		OrganizationID:     "org_1",
		ConfigLevel:        domain.ConfigLevelRepository,
		RepositoryID:       "repo_1",
		StartReviewMessage: &customStart,
	})
	if err != nil {
		t.Fatalf("failed to create repo message: %v", err)
	}

	// Resolves to repo-level message
	msg, _ = uc.FindByRepoOrDirectory(ctx, "org_1", "repo_1", "")
	if msg.StartReviewMessage.Content != "Custom Repo Start Review" {
		t.Errorf("expected custom repo message, got %s", msg.StartReviewMessage.Content)
	}

	// 3. Override counts
	counts, err := uc.FindOverrideCountsByRepository(ctx, "org_1")
	if err != nil || len(counts) != 1 || counts[0].Count != 3 {
		t.Errorf("unexpected counts: %v", counts)
	}
}

// MockPRDataProvider implements IPullRequestDataProvider in-memory.
type MockPRDataProvider struct {
	files       []PRFileChange
	suggestions []domain.CodeSuggestion
}

func (m *MockPRDataProvider) GetChangedFiles(ctx context.Context, repo models.TrackedRepository, prNumber int) ([]PRFileChange, error) {
	return m.files, nil
}

func (m *MockPRDataProvider) GetSavedSuggestions(ctx context.Context, orgID string, prNumber int) ([]domain.CodeSuggestion, error) {
	return m.suggestions, nil
}

func TestPRUseCases_GetSuggestions(t *testing.T) {
	provider := &MockPRDataProvider{
		suggestions: []domain.CodeSuggestion{
			{
				ID:                 uuid.New(),
				RelevantFile:       "auth.go",
				Severity:           domain.SeverityCritical,
				Category:           domain.CategorySecurity,
				OneSentenceSummary: "Fix SQL Injection",
				SuggestionContent:  "Use parameterized query instead of fmt.Sprintf",
				ImprovedCode:       "db.Query(ctx, query, id)",
			},
			{
				ID:                 uuid.New(),
				RelevantFile:       "logger.go",
				Severity:           domain.SeverityInfo,
				Category:           domain.CategoryRules,
				OneSentenceSummary: "Add log prefix",
				SuggestionContent:  "Log prefix required",
			},
		},
	}

	uc := NewPRUseCases(provider, nil)
	ctx := context.Background()

	// 1. Filter by CRITICAL severity
	res, err := uc.GetPullRequestSuggestions(ctx, PRSuggestionsQuery{
		OrganizationID: "org_1",
		PRNumber:       10,
		Format:         "json",
		Severity:       "CRITICAL",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Count != 1 || res.Suggestions[0].Severity != domain.SeverityCritical {
		t.Errorf("expected 1 critical suggestion, got %d", res.Count)
	}

	// 2. Markdown output
	mdRes, err := uc.GetPullRequestSuggestions(ctx, PRSuggestionsQuery{
		OrganizationID: "org_1",
		PRNumber:       10,
		Format:         "markdown",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(mdRes.Markdown, "ScanDrix Review Findings (2)") {
		t.Errorf("expected markdown findings header, got %s", mdRes.Markdown)
	}

	// 3. Summary Preview
	preview := uc.PreviewPRSummary(ctx, "developer1", "my-repo", 10, provider.suggestions, "")
	if !strings.Contains(preview, "ScanDrix Review Completed") || !strings.Contains(preview, "developer1") {
		t.Errorf("expected preview summary to contain completed header and author, got %s", preview)
	}
}

// MockJobPublisher implements WorkflowJobPublisher in-memory.
type MockJobPublisher struct {
	published []string
}

func (m *MockJobPublisher) PublishJob(ctx context.Context, eventType string, payload []byte) error {
	m.published = append(m.published, eventType)
	return nil
}

func TestWorkflowUseCases(t *testing.T) {
	pub := &MockJobPublisher{}
	uc := NewWorkflowUseCases(pub)
	ctx := context.Background()

	err := uc.EnqueueAstGraphUpdateOnMerged(ctx, uuid.New(), uuid.New(), "main", "abc1234", "def5678")
	if err != nil {
		t.Fatalf("failed to enqueue ast update: %v", err)
	}

	corrID, err := uc.EnqueueImplementationCheck(ctx, uuid.New(), uuid.New(), 42, "abc1234")
	if err != nil {
		t.Fatalf("failed to enqueue impl check: %v", err)
	}
	if !strings.HasPrefix(corrID, "impl-check-") {
		t.Errorf("expected correlation id to start with impl-check-, got %s", corrID)
	}

	if len(pub.published) != 2 {
		t.Errorf("expected 2 published jobs, got %d", len(pub.published))
	}
	if pub.published[0] != "workflow.ast_graph.update" {
		t.Errorf("expected workflow.ast_graph.update, got %s", pub.published[0])
	}
	if pub.published[1] != "workflow.suggestion.check_implementation" {
		t.Errorf("expected workflow.suggestion.check_implementation, got %s", pub.published[1])
	}
}

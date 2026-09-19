package clireview_test

import (
	"context"
	"errors"
	"testing"

	"github.com/scandrix/backend/internal/clireview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock implementations for ExecuteCliReview testing
type mockCodeManagementService struct {
	platform string
	err      error
}

func (m *mockCodeManagementService) GetTypeIntegration(ctx context.Context, orgAndTeam clireview.OrganizationAndTeamData) (string, error) {
	return m.platform, m.err
}

type mockParametersService struct {
	config *clireview.CodeReviewConfig
	repos  []clireview.RepositoryRef
	err    error
}

func (m *mockParametersService) GetCodeReviewConfig(ctx context.Context, orgAndTeam clireview.OrganizationAndTeamData) (*clireview.CodeReviewConfig, []clireview.RepositoryRef, error) {
	return m.config, m.repos, m.err
}

type mockDrixyRulesService struct {
	rules       []clireview.DrixyRule
	syncedRules []clireview.DrixyRule
	findErr     error
	syncErr     error
}

func (m *mockDrixyRulesService) FindByOrganizationID(ctx context.Context, orgID string) ([]clireview.DrixyRule, error) {
	return m.rules, m.findErr
}

func (m *mockDrixyRulesService) SyncRulesWithPlanLimit(ctx context.Context, orgAndTeam clireview.OrganizationAndTeamData, rules []clireview.DrixyRule) ([]clireview.DrixyRule, error) {
	if m.syncedRules != nil {
		return m.syncedRules, m.syncErr
	}
	return rules, m.syncErr
}

func setupExecuteMocks() (*clireview.ExecuteCliReviewUseCase, *mockCodeManagementService, *mockParametersService, *mockDrixyRulesService, *clireview.InMemoryAutomationExecutionService) {
	converter := clireview.NewCliInputConverter()
	strategy := clireview.NewCliReviewPipelineStrategy(nil, nil)
	codeMgmt := &mockCodeManagementService{}
	params := &mockParametersService{}
	rulesSvc := &mockDrixyRulesService{}
	execSvc := clireview.NewInMemoryAutomationExecutionService()

	uc := clireview.NewExecuteCliReviewUseCase(
		converter,
		strategy,
		codeMgmt,
		params,
		execSvc,
		rulesSvc,
		nil,
	)

	return uc, codeMgmt, params, rulesSvc, execSvc
}

func TestExecuteCliReviewUseCase_ResolveCliPlatform(t *testing.T) {
	ctx := context.Background()
	orgAndTeam := clireview.OrganizationAndTeamData{
		OrganizationID: "org-123",
		TeamID:         "team-456",
	}

	t.Run("uses the platform the CLI inferred from a known SaaS host", func(t *testing.T) {
		uc, _, _, _, _ := setupExecuteMocks()
		gitCtx := &clireview.GitContext{
			InferredPlatform: "gitlab",
		}
		platform := uc.ResolveCliPlatform(ctx, orgAndTeam, gitCtx, false)
		assert.Equal(t, "gitlab", platform)
	})

	t.Run("falls back to the connected integration for a self-managed host", func(t *testing.T) {
		uc, codeMgmt, _, _, _ := setupExecuteMocks()
		codeMgmt.platform = "bitbucket"

		gitCtx := &clireview.GitContext{
			Remote: "https://git.internal.corp/repo.git",
		}
		platform := uc.ResolveCliPlatform(ctx, orgAndTeam, gitCtx, false)
		assert.Equal(t, "bitbucket", platform)
	})

	t.Run("never guesses GitHub when the organization has no integration", func(t *testing.T) {
		uc, codeMgmt, _, _, _ := setupExecuteMocks()
		codeMgmt.platform = ""

		gitCtx := &clireview.GitContext{
			Remote: "https://git.internal.corp/repo.git",
		}
		platform := uc.ResolveCliPlatform(ctx, orgAndTeam, gitCtx, false)
		assert.Empty(t, platform)
	})

	t.Run("skips the integration lookup for anonymous trial traffic", func(t *testing.T) {
		uc, codeMgmt, _, _, _ := setupExecuteMocks()
		codeMgmt.platform = "github"

		gitCtx := &clireview.GitContext{
			Remote: "https://github.com/owner/repo.git",
		}
		platform := uc.ResolveCliPlatform(ctx, orgAndTeam, gitCtx, true)
		assert.Empty(t, platform)
	})

	t.Run("degrades to empty when the lookup blows up", func(t *testing.T) {
		uc, codeMgmt, _, _, _ := setupExecuteMocks()
		codeMgmt.err = errors.New("database connection failed")

		gitCtx := &clireview.GitContext{
			Remote: "https://gitlab.custom.org/project.git",
		}
		platform := uc.ResolveCliPlatform(ctx, orgAndTeam, gitCtx, false)
		assert.Empty(t, platform)
	})
}

func TestExecuteCliReviewUseCase_ResolveRepositoryFromRemote(t *testing.T) {
	uc, _, _, _, _ := setupExecuteMocks()

	repos := []clireview.RepositoryRef{
		{
			ID:      "repo-1",
			Name:    "my-app",
			HTTPURL: "https://github.com/my-org/my-app.git",
		},
		{
			ID:      "repo-2",
			Name:    "backend-api",
			HTTPURL: "https://gitlab.com/company/backend-api",
		},
		{
			ID:       "repo-3",
			Name:     "web-frontend",
			CloneURL: "git@github.com:my-org/web-frontend.git",
		},
	}

	t.Run("should return global when remote is empty", func(t *testing.T) {
		id, name := uc.ResolveRepositoryFromRemote("", repos)
		assert.Equal(t, "global", id)
		assert.Nil(t, name)
	})

	t.Run("should return global when repositories list is empty", func(t *testing.T) {
		id, name := uc.ResolveRepositoryFromRemote("https://github.com/my-org/my-app", []clireview.RepositoryRef{})
		assert.Equal(t, "global", id)
		assert.Nil(t, name)
	})

	t.Run("should match HTTPS remote to http_url", func(t *testing.T) {
		id, name := uc.ResolveRepositoryFromRemote("https://github.com/my-org/my-app", repos)
		assert.Equal(t, "repo-1", id)
		require.NotNil(t, name)
		assert.Equal(t, "my-app", *name)
	})

	t.Run("should match SSH remote to http_url", func(t *testing.T) {
		id, name := uc.ResolveRepositoryFromRemote("git@github.com:my-org/my-app.git", repos)
		assert.Equal(t, "repo-1", id)
		require.NotNil(t, name)
		assert.Equal(t, "my-app", *name)
	})

	t.Run("should match case-insensitively", func(t *testing.T) {
		id, name := uc.ResolveRepositoryFromRemote("HTTPS://GITHUB.COM/MY-ORG/MY-APP.GIT", repos)
		assert.Equal(t, "repo-1", id)
		require.NotNil(t, name)
		assert.Equal(t, "my-app", *name)
	})

	t.Run("should fallback to name matching when http_url does not match", func(t *testing.T) {
		id, name := uc.ResolveRepositoryFromRemote("git@custom-host.com:team/backend-api.git", repos)
		assert.Equal(t, "repo-2", id)
		require.NotNil(t, name)
		assert.Equal(t, "backend-api", *name)
	})

	t.Run("should fallback to name matching case-insensitively", func(t *testing.T) {
		id, name := uc.ResolveRepositoryFromRemote("https://forgejo.org/team/BACKEND-API.git", repos)
		assert.Equal(t, "repo-2", id)
		require.NotNil(t, name)
		assert.Equal(t, "backend-api", *name)
	})

	t.Run("should return global when no match is found", func(t *testing.T) {
		id, name := uc.ResolveRepositoryFromRemote("https://github.com/other-org/unknown-service", repos)
		assert.Equal(t, "global", id)
		assert.Nil(t, name)
	})

	t.Run("should match with trailing slashes in remote", func(t *testing.T) {
		id, name := uc.ResolveRepositoryFromRemote("https://github.com/my-org/my-app///", repos)
		assert.Equal(t, "repo-1", id)
		require.NotNil(t, name)
		assert.Equal(t, "my-app", *name)
	})
}

func TestExecuteCliReviewUseCase_NormalizeGitURL(t *testing.T) {
	uc, _, _, _, _ := setupExecuteMocks()

	assert.Equal(t, "github.com/org/repo", uc.NormalizeGitURL("https://github.com/org/repo.git"))
	assert.Equal(t, "github.com/org/repo", uc.NormalizeGitURL("http://github.com/org/repo"))
	assert.Equal(t, "github.com/org/repo", uc.NormalizeGitURL("git@github.com:org/repo.git"))
	assert.Equal(t, "github.com/org/repo", uc.NormalizeGitURL("ssh://git@github.com/org/repo.git"))
	assert.Equal(t, "github.com/org/repo", uc.NormalizeGitURL("https://github.com/org/repo/"))
	assert.Equal(t, "gitlab.com/group/subgroup/project", uc.NormalizeGitURL("https://gitlab.com/group/subgroup/project.git/"))
}

func TestExecuteCliReviewUseCase_ExtractRepoNameFromRemote(t *testing.T) {
	uc, _, _, _, _ := setupExecuteMocks()

	name1 := uc.ExtractRepoNameFromRemote("https://github.com/scandrix/backend.git")
	require.NotNil(t, name1)
	assert.Equal(t, "backend", *name1)

	name2 := uc.ExtractRepoNameFromRemote("git@github.com:scandrix/cli.git")
	require.NotNil(t, name2)
	assert.Equal(t, "cli", *name2)

	name3 := uc.ExtractRepoNameFromRemote("ssh://git@server:22/my-service")
	require.NotNil(t, name3)
	assert.Equal(t, "my-service", *name3)

	nameNil := uc.ExtractRepoNameFromRemote("")
	assert.Nil(t, nameNil)
}

func TestExecuteCliReviewUseCase_LoadUserConfigWithRules(t *testing.T) {
	ctx := context.Background()
	orgAndTeam := clireview.OrganizationAndTeamData{
		OrganizationID: "org-1",
		TeamID:         "team-1",
	}

	t.Run("should return default config when parameters service returns nil", func(t *testing.T) {
		uc, _, params, _, _ := setupExecuteMocks()
		params.config = nil

		cfg, repoID, repoName := uc.LoadUserConfigWithRules(ctx, orgAndTeam, nil)
		assert.Equal(t, "global", repoID)
		assert.Nil(t, repoName)
		assert.Equal(t, "v2", cfg.CodeReviewVersion)
		assert.True(t, cfg.AutomatedReviewActive)
	})

	t.Run("should load drixy rules and filter by resolved repositoryId", func(t *testing.T) {
		uc, _, params, rulesSvc, _ := setupExecuteMocks()
		params.config = &clireview.CodeReviewConfig{
			LanguageResultPrompt: "es",
		}
		params.repos = []clireview.RepositoryRef{
			{ID: "repo-target", Name: "target-repo", HTTPURL: "https://github.com/org/target-repo"},
		}

		rulesSvc.rules = []clireview.DrixyRule{
			{ID: "r1", Rule: "Rule 1", Active: true, RepositoryID: "global"},
			{ID: "r2", Rule: "Rule 2", Active: true, RepositoryID: "repo-target"},
			{ID: "r3", Rule: "Rule 3", Active: true, RepositoryID: "other-repo"},
			{ID: "r4", Rule: "Memory Rule 1", Active: true, Type: "memory", RepositoryID: "repo-target"},
		}

		gitCtx := &clireview.GitContext{
			Remote: "https://github.com/org/target-repo.git",
		}

		cfg, repoID, repoName := uc.LoadUserConfigWithRules(ctx, orgAndTeam, gitCtx)
		assert.Equal(t, "repo-target", repoID)
		require.NotNil(t, repoName)
		assert.Equal(t, "target-repo", *repoName)

		assert.Len(t, cfg.Rules, 2) // global + repo-target
		assert.Len(t, cfg.MemoryRules, 1) // memory rule for repo-target
	})

	t.Run("reconciles plan-locked rules and applies synced set", func(t *testing.T) {
		uc, _, params, rulesSvc, _ := setupExecuteMocks()
		params.config = &clireview.CodeReviewConfig{}
		rulesSvc.rules = []clireview.DrixyRule{
			{ID: "r1", Rule: "Active Rule", Active: true},
			{ID: "r2", Rule: "Locked Rule", Active: true, Locked: true},
		}
		rulesSvc.syncedRules = []clireview.DrixyRule{
			{ID: "r1", Rule: "Active Rule", Active: true},
		}

		cfg, _, _ := uc.LoadUserConfigWithRules(ctx, orgAndTeam, nil)
		assert.Len(t, cfg.Rules, 1)
		assert.Equal(t, "r1", cfg.Rules[0].ID)
	})
}

func TestExecuteCliReviewUseCase_Execute(t *testing.T) {
	ctx := context.Background()

	t.Run("returns early when diff has 0 changed files", func(t *testing.T) {
		uc, _, _, _, _ := setupExecuteMocks()
		res, err := uc.Execute(ctx, clireview.ExecuteCliReviewInput{
			OrganizationAndTeamData: clireview.OrganizationAndTeamData{
				OrganizationID: "org-1",
				TeamID:         "team-1",
			},
			Input: clireview.CliReviewInput{
				Diff: "",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, 0, res.FilesAnalyzed)
		assert.Equal(t, "No files to analyze", res.Summary)
		assert.Empty(t, res.Issues)
	})

	t.Run("uses global repositoryId in trial mode", func(t *testing.T) {
		uc, _, _, _, execSvc := setupExecuteMocks()
		diff := `diff --git a/src/main.go b/src/main.go
index 0000000..1111111 100644
--- a/src/main.go
+++ b/src/main.go
@@ -0,0 +1,5 @@
+package main
+import "fmt"
+func main() {
+    fmt.Println("hello world")
+}
`
		res, err := uc.Execute(ctx, clireview.ExecuteCliReviewInput{
			OrganizationAndTeamData: clireview.OrganizationAndTeamData{
				OrganizationID: "trial",
				TeamID:         "trial",
			},
			Input: clireview.CliReviewInput{
				Diff: diff,
			},
			IsTrialMode: true,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, res.FilesAnalyzed)
		// Trial mode skips creating database execution records
		assert.Empty(t, execSvc.Records())
	})

	t.Run("records execution telemetry for authenticated review", func(t *testing.T) {
		uc, _, params, _, execSvc := setupExecuteMocks()
		params.config = &clireview.CodeReviewConfig{
			LanguageResultPrompt: "en-US",
		}

		diff := `diff --git a/app.go b/app.go
index 1111111..2222222 100644
--- a/app.go
+++ b/app.go
@@ -1,3 +1,4 @@
 package app
+const apiKey = "sk_live_1234567890abcdef"
`
		res, err := uc.Execute(ctx, clireview.ExecuteCliReviewInput{
			OrganizationAndTeamData: clireview.OrganizationAndTeamData{
				OrganizationID: "org-prod",
				TeamID:         "team-backend",
			},
			Input: clireview.CliReviewInput{
				Diff: diff,
				Config: &clireview.CliReviewConfig{
					Focus: "check secrets and credentials",
				},
			},
			UserEmail: "eng@scandrix.dev",
			GitContext: &clireview.GitContext{
				Remote: "https://github.com/scandrix/backend.git",
				Branch: "feature/auth",
			},
			CliAuth: &clireview.ExecutionAuthContext{
				Mode:        "team-key",
				TeamKeyName: "CI Bot",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, 1, res.FilesAnalyzed)
		assert.NotEmpty(t, res.Issues)
		assert.Equal(t, "sec-no-hardcoded-secrets", res.Issues[0].RuleID)

		records := execSvc.Records()
		require.Len(t, records, 1)
		assert.Equal(t, "success", records[0].Status)
		assert.Equal(t, "org-prod", records[0].OrganizationID)
	})
}

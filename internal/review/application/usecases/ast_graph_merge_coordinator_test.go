// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package usecases_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/application/usecases"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAstRepoLookup struct {
	repos map[string]*usecases.AstRepoMetadata
	err   error
}

func (m *mockAstRepoLookup) FindRepository(ctx context.Context, platform models.SCMProvider, externalID string) (*usecases.AstRepoMetadata, error) {
	if m.err != nil {
		return nil, m.err
	}
	key := fmt.Sprintf("%s:%s", platform, externalID)
	return m.repos[key], nil
}

type mockFilesFetcher struct {
	files map[string][]string
	err   error
}

func (m *mockFilesFetcher) GetPRChangedFiles(ctx context.Context, repoFullName string, prNumber int) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	key := fmt.Sprintf("%s:%d", repoFullName, prNumber)
	return m.files[key], nil
}

type mockWorkflowJobPublisher struct {
	mu        sync.Mutex
	published []struct {
		EventType string
		Payload   []byte
	}
	err error
}

func (m *mockWorkflowJobPublisher) PublishJob(ctx context.Context, eventType string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.published = append(m.published, struct {
		EventType string
		Payload   []byte
	}{EventType: eventType, Payload: payload})
	return nil
}

func TestAstGraphMergeCoordinator_RepositoryNotFound(t *testing.T) {
	lookup := &mockAstRepoLookup{repos: make(map[string]*usecases.AstRepoMetadata)}
	fetcher := &mockFilesFetcher{files: make(map[string][]string)}
	pub := &mockWorkflowJobPublisher{}
	coordinator := usecases.NewAstGraphMergeCoordinator(lookup, fetcher, pub, nil)

	input := usecases.EnqueueAstGraphUpdateInput{
		PRNumber:       1,
		RepoExternalID: "ext-unknown",
		Platform:       models.SCMProviderGitHub,
		BaseBranch:     "main",
	}

	res, err := coordinator.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.False(t, res.Enqueued)
	assert.Equal(t, "repo not tracked", res.Reason)
	assert.Empty(t, pub.published)
}

func TestAstGraphMergeCoordinator_LookupError(t *testing.T) {
	lookup := &mockAstRepoLookup{err: errors.New("database connection pool exhausted")}
	fetcher := &mockFilesFetcher{files: make(map[string][]string)}
	pub := &mockWorkflowJobPublisher{}
	coordinator := usecases.NewAstGraphMergeCoordinator(lookup, fetcher, pub, nil)

	input := usecases.EnqueueAstGraphUpdateInput{
		PRNumber:       1,
		RepoExternalID: "ext-1",
		Platform:       models.SCMProviderGitHub,
	}

	res, err := coordinator.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.False(t, res.Enqueued)
	assert.Equal(t, "repo lookup failed", res.Reason)
}

func TestAstGraphMergeCoordinator_NonDefaultBranch(t *testing.T) {
	repoID := uuid.New()
	lookup := &mockAstRepoLookup{
		repos: map[string]*usecases.AstRepoMetadata{
			"github:ext-1": {
				ID:             repoID,
				FullName:       "scandrix/engine",
				DefaultBranch:  "main",
				AstGraphStatus: usecases.AstGraphStatusReady,
				Platform:       models.SCMProviderGitHub,
			},
		},
	}
	fetcher := &mockFilesFetcher{files: make(map[string][]string)}
	pub := &mockWorkflowJobPublisher{}
	coordinator := usecases.NewAstGraphMergeCoordinator(lookup, fetcher, pub, nil)

	input := usecases.EnqueueAstGraphUpdateInput{
		PRNumber:       10,
		RepoExternalID: "ext-1",
		Platform:       models.SCMProviderGitHub,
		BaseBranch:     "develop", // not default
	}

	res, err := coordinator.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.False(t, res.Enqueued)
	assert.Contains(t, res.Reason, "base branch develop is not default")
}

func TestAstGraphMergeCoordinator_GraphNotReady(t *testing.T) {
	repoID := uuid.New()
	lookup := &mockAstRepoLookup{
		repos: map[string]*usecases.AstRepoMetadata{
			"github:ext-1": {
				ID:             repoID,
				FullName:       "scandrix/engine",
				DefaultBranch:  "main",
				AstGraphStatus: usecases.AstGraphStatusBuilding, // still building
				Platform:       models.SCMProviderGitHub,
			},
		},
	}
	fetcher := &mockFilesFetcher{files: make(map[string][]string)}
	pub := &mockWorkflowJobPublisher{}
	coordinator := usecases.NewAstGraphMergeCoordinator(lookup, fetcher, pub, nil)

	input := usecases.EnqueueAstGraphUpdateInput{
		PRNumber:       10,
		RepoExternalID: "ext-1",
		Platform:       models.SCMProviderGitHub,
		BaseBranch:     "main",
	}

	res, err := coordinator.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.False(t, res.Enqueued)
	assert.Equal(t, "graph not ready (BUILDING)", res.Reason)
}

func TestAstGraphMergeCoordinator_FetchFilesError(t *testing.T) {
	repoID := uuid.New()
	lookup := &mockAstRepoLookup{
		repos: map[string]*usecases.AstRepoMetadata{
			"github:ext-1": {
				ID:             repoID,
				FullName:       "scandrix/engine",
				DefaultBranch:  "main",
				AstGraphStatus: usecases.AstGraphStatusReady,
				Platform:       models.SCMProviderGitHub,
			},
		},
	}
	fetcher := &mockFilesFetcher{err: errors.New("GitHub API 500 internal server error")}
	pub := &mockWorkflowJobPublisher{}
	coordinator := usecases.NewAstGraphMergeCoordinator(lookup, fetcher, pub, nil)

	input := usecases.EnqueueAstGraphUpdateInput{
		PRNumber:       12,
		RepoExternalID: "ext-1",
		Platform:       models.SCMProviderGitHub,
		BaseBranch:     "main",
	}

	res, err := coordinator.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.False(t, res.Enqueued)
	assert.Equal(t, "failed to fetch PR files", res.Reason)
}

func TestAstGraphMergeCoordinator_NoChangedFiles(t *testing.T) {
	repoID := uuid.New()
	lookup := &mockAstRepoLookup{
		repos: map[string]*usecases.AstRepoMetadata{
			"github:ext-1": {
				ID:             repoID,
				FullName:       "scandrix/engine",
				DefaultBranch:  "main",
				AstGraphStatus: usecases.AstGraphStatusReady,
				Platform:       models.SCMProviderGitHub,
			},
		},
	}
	fetcher := &mockFilesFetcher{
		files: map[string][]string{
			"scandrix/engine:15": {}, // empty
		},
	}
	pub := &mockWorkflowJobPublisher{}
	coordinator := usecases.NewAstGraphMergeCoordinator(lookup, fetcher, pub, nil)

	input := usecases.EnqueueAstGraphUpdateInput{
		PRNumber:       15,
		RepoExternalID: "ext-1",
		Platform:       models.SCMProviderGitHub,
		BaseBranch:     "main",
	}

	res, err := coordinator.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.False(t, res.Enqueued)
	assert.Equal(t, "no changed files", res.Reason)
}

func TestAstGraphMergeCoordinator_EnqueuesIncrementalUpdate(t *testing.T) {
	repoID := uuid.New()
	wsID := uuid.New()
	lookup := &mockAstRepoLookup{
		repos: map[string]*usecases.AstRepoMetadata{
			"github:ext-1": {
				ID:             repoID,
				WorkspaceID:    wsID,
				FullName:       "scandrix/engine",
				DefaultBranch:  "main",
				AstGraphStatus: usecases.AstGraphStatusReady,
				Platform:       models.SCMProviderGitHub,
			},
		},
	}
	fetcher := &mockFilesFetcher{
		files: map[string][]string{
			"scandrix/engine:20": {"pkg/auth/jwt.go", "internal/api/handler.go"},
		},
	}
	pub := &mockWorkflowJobPublisher{}
	coordinator := usecases.NewAstGraphMergeCoordinator(lookup, fetcher, pub, nil)

	input := usecases.EnqueueAstGraphUpdateInput{
		PRNumber:       20,
		RepoExternalID: "ext-1",
		Platform:       models.SCMProviderGitHub,
		BaseBranch:     "main",
		NewSHA:         "head-merged-sha",
		WorkspaceID:    wsID,
	}

	res, err := coordinator.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.True(t, res.Enqueued)
	assert.Equal(t, "incremental", res.JobType)
	assert.Equal(t, 2, res.FilesCount)
	assert.NotEmpty(t, res.JobID)

	require.Len(t, pub.published, 1)
	assert.Equal(t, "workflow.ast_graph.incremental", pub.published[0].EventType)

	var payload usecases.AstGraphJobPayload
	err = json.Unmarshal(pub.published[0].Payload, &payload)
	require.NoError(t, err)
	assert.Equal(t, repoID, payload.RepositoryID)
	assert.Equal(t, "incremental", payload.JobType)
	assert.Equal(t, "head-merged-sha", payload.NewSHA)
	assert.Equal(t, []string{"pkg/auth/jwt.go", "internal/api/handler.go"}, payload.ChangedFiles)
}

func TestAstGraphMergeCoordinator_EnqueuesFullRebuildWhenFilesExceed500(t *testing.T) {
	repoID := uuid.New()
	wsID := uuid.New()
	lookup := &mockAstRepoLookup{
		repos: map[string]*usecases.AstRepoMetadata{
			"gitlab:ext-large": {
				ID:             repoID,
				WorkspaceID:    wsID,
				FullName:       "scandrix/monorepo",
				DefaultBranch:  "master",
				AstGraphStatus: usecases.AstGraphStatusReady,
				Platform:       models.SCMProviderGitLab,
			},
		},
	}

	// 505 files > 500 threshold
	largeFileList := make([]string, 505)
	for i := 0; i < 505; i++ {
		largeFileList[i] = fmt.Sprintf("src/module_%d/file.go", i)
	}

	fetcher := &mockFilesFetcher{
		files: map[string][]string{
			"scandrix/monorepo:100": largeFileList,
		},
	}
	pub := &mockWorkflowJobPublisher{}
	coordinator := usecases.NewAstGraphMergeCoordinator(lookup, fetcher, pub, nil)

	input := usecases.EnqueueAstGraphUpdateInput{
		PRNumber:       100,
		RepoExternalID: "ext-large",
		Platform:       models.SCMProviderGitLab,
		BaseBranch:     "master",
		NewSHA:         "master-rebuild-sha",
		WorkspaceID:    wsID,
	}

	res, err := coordinator.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.True(t, res.Enqueued)
	assert.Equal(t, "full-rebuild", res.JobType)
	assert.Equal(t, 505, res.FilesCount)

	require.Len(t, pub.published, 1)
	assert.Equal(t, "workflow.ast_graph.build", pub.published[0].EventType)

	var payload usecases.AstGraphJobPayload
	err = json.Unmarshal(pub.published[0].Payload, &payload)
	require.NoError(t, err)
	assert.Equal(t, "full-rebuild", payload.JobType)
	assert.Equal(t, 505, len(payload.ChangedFiles))
}

func TestAstGraphMergeCoordinator_ConcurrentStress(t *testing.T) {
	repoID := uuid.New()
	wsID := uuid.New()
	lookup := &mockAstRepoLookup{
		repos: map[string]*usecases.AstRepoMetadata{
			"github:ext-stress": {
				ID:             repoID,
				WorkspaceID:    wsID,
				FullName:       "scandrix/stress",
				DefaultBranch:  "main",
				AstGraphStatus: usecases.AstGraphStatusReady,
				Platform:       models.SCMProviderGitHub,
			},
		},
	}

	fetcher := &mockFilesFetcher{
		files: map[string][]string{
			"scandrix/stress:1": {"pkg/a.go"},
			"scandrix/stress:2": {"pkg/b.go"},
		},
	}
	pub := &mockWorkflowJobPublisher{}
	coordinator := usecases.NewAstGraphMergeCoordinator(lookup, fetcher, pub, nil)

	var wg sync.WaitGroup
	workers := 20
	iterations := 25

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				prNum := (workerID % 2) + 1
				input := usecases.EnqueueAstGraphUpdateInput{
					PRNumber:       prNum,
					RepoExternalID: "ext-stress",
					Platform:       models.SCMProviderGitHub,
					BaseBranch:     "main",
					NewSHA:         fmt.Sprintf("sha-%d-%d", workerID, i),
					WorkspaceID:    wsID,
				}

				res, err := coordinator.Execute(context.Background(), input)
				if err != nil || !res.Enqueued {
					t.Errorf("unexpected failure on worker %d iteration %d: %v", workerID, i, err)
				}
			}
		}(w)
	}

	wg.Wait()
	assert.Equal(t, workers*iterations, len(pub.published))
}

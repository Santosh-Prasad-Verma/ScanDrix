package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAstRepoConfigStore struct {
	repos map[string][]*types.Repositories
	err   error
}

func (m *mockAstRepoConfigStore) SaveRepositories(ctx context.Context, organizationID, teamID string, repos []*types.Repositories) error {
	return nil
}

func (m *mockAstRepoConfigStore) GetRepositories(ctx context.Context, organizationID, teamID string) ([]*types.Repositories, error) {
	if m.err != nil {
		return nil, m.err
	}
	key := organizationID + ":" + teamID
	return m.repos[key], nil
}

func (m *mockAstRepoConfigStore) UpdateTeamStatus(ctx context.Context, teamID string, status string) error {
	return nil
}

type mockAstQueueService struct {
	enqueued []string
	err      error
}

func (m *mockAstQueueService) EnqueueAstGraphBuild(ctx context.Context, orgID, teamID, repoID, repoName, cloneURL, defaultBranch string) error {
	if m.err != nil {
		return m.err
	}
	m.enqueued = append(m.enqueued, repoID)
	return nil
}

type mockAstStatusStore struct {
	statuses map[string]AstGraphStatus
}

func (m *mockAstStatusStore) GetAstGraphStatus(ctx context.Context, orgID, teamID, repoID string) (AstGraphStatus, error) {
	return m.statuses[repoID], nil
}

func TestBackfillAstGraphBuildUseCase_Validation(t *testing.T) {
	uc := NewBackfillAstGraphBuildUseCase(nil, nil, nil, nil)

	_, err := uc.Execute(context.Background(), BackfillAstGraphBuildInput{
		OrganizationID: "",
		TeamID:         "team-1",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organizationId is required")

	_, err = uc.Execute(context.Background(), BackfillAstGraphBuildInput{
		OrganizationID: "org-1",
		TeamID:         "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "teamId is required")
}

func TestBackfillAstGraphBuildUseCase_StatusFiltering(t *testing.T) {
	ctx := context.Background()
	orgID := "org-alpha"
	teamID := "team-beta"

	repo1 := &types.Repositories{ID: "101", Name: "repo-building", FullName: "acme/repo-building", Selected: true}
	repo2 := &types.Repositories{ID: "102", Name: "repo-ready", FullName: "acme/repo-ready", Selected: true}
	repo3 := &types.Repositories{ID: "103", Name: "repo-pending", FullName: "acme/repo-pending", Selected: true}
	repo4 := &types.Repositories{ID: "104", Name: "repo-unselected", FullName: "acme/repo-unselected", Selected: false}
	repo5 := &types.Repositories{ID: "105", Name: "repo-failed", FullName: "acme/repo-failed", Selected: true}

	configStore := &mockAstRepoConfigStore{
		repos: map[string][]*types.Repositories{
			orgID + ":" + teamID: {repo1, repo2, repo3, repo4, repo5},
		},
	}

	statusStore := &mockAstStatusStore{
		statuses: map[string]AstGraphStatus{
			"101": AstGraphStatusBuilding,
			"102": AstGraphStatusReady,
			"103": AstGraphStatusPending,
			"105": AstGraphStatusFailed,
		},
	}

	queueService := &mockAstQueueService{}
	uc := NewBackfillAstGraphBuildUseCase(configStore, queueService, statusStore, nil)

	// Sweep without force
	out, err := uc.Execute(ctx, BackfillAstGraphBuildInput{
		OrganizationID: orgID,
		TeamID:         teamID,
		Force:          false,
		Limit:          10,
	})
	require.NoError(t, err)
	require.NotNil(t, out)

	// Selected count = 4 (repo4 was false)
	assert.Equal(t, 4, out.Matched)
	// Enqueued = 2 (repo-pending 103, repo-failed 105)
	assert.Equal(t, 2, out.Enqueued)
	assert.ElementsMatch(t, []string{"103", "105"}, queueService.enqueued)

	// Skipped = 2 (repo-building and repo-ready without force)
	assert.Len(t, out.Skipped, 2)
	assert.Equal(t, "BUILDING (job already in flight)", out.Skipped[0].Reason)
	assert.Equal(t, "READY (use force=true to rebuild)", out.Skipped[1].Reason)

	// Sweep with force = true
	queueService.enqueued = nil
	outForce, err := uc.Execute(ctx, BackfillAstGraphBuildInput{
		OrganizationID: orgID,
		TeamID:         teamID,
		Force:          true,
		Limit:          10,
	})
	require.NoError(t, err)
	// Now repo-ready 102 should be enqueued along with 103 and 105 (total 3)
	assert.Equal(t, 3, outForce.Enqueued)
	assert.ElementsMatch(t, []string{"102", "103", "105"}, queueService.enqueued)
	assert.Len(t, outForce.Skipped, 1) // Only repo-building is skipped
}

func TestBackfillAstGraphBuildUseCase_LimitAndErrorHandling(t *testing.T) {
	ctx := context.Background()
	orgID := "org-1"
	teamID := "team-1"

	repos := []*types.Repositories{
		{ID: "1", Name: "r1", Selected: true},
		{ID: "2", Name: "r2", Selected: true},
		{ID: "3", Name: "r3", Selected: true},
	}

	configStore := &mockAstRepoConfigStore{
		repos: map[string][]*types.Repositories{
			orgID + ":" + teamID: repos,
		},
	}

	queueService := &mockAstQueueService{
		err: errors.New("queue full"),
	}

	uc := NewBackfillAstGraphBuildUseCase(configStore, queueService, nil, nil)
	out, err := uc.Execute(ctx, BackfillAstGraphBuildInput{
		OrganizationID: orgID,
		TeamID:         teamID,
		Limit:          2,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, out.Matched)
	assert.Equal(t, 0, out.Enqueued)
	assert.Len(t, out.Errors, 3)
}

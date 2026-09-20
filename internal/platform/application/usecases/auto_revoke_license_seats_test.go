package usecases

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/platform/application/services"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAutoLicenseParamsStore struct {
	configs map[string]*AutoAssignLicenseConfig
}

func (m *mockAutoLicenseParamsStore) GetAutoAssignConfig(ctx context.Context, orgID, teamID string) (*AutoAssignLicenseConfig, error) {
	key := orgID + ":" + teamID
	return m.configs[key], nil
}

func (m *mockAutoLicenseParamsStore) SaveAutoAssignConfig(ctx context.Context, orgID, teamID string, cfg *AutoAssignLicenseConfig) error {
	key := orgID + ":" + teamID
	m.configs[key] = cfg
	return nil
}

type mockLicenseSeatPruneService struct {
	seats   []string
	revoked []string
}

func (m *mockLicenseSeatPruneService) GetLicensedUsers(ctx context.Context, orgID, teamID string) ([]string, error) {
	return m.seats, nil
}

func (m *mockLicenseSeatPruneService) RevokeSeat(ctx context.Context, orgID, teamID, gitID string) error {
	m.revoked = append(m.revoked, gitID)
	remaining := make([]string, 0, len(m.seats))
	for _, s := range m.seats {
		if s != gitID {
			remaining = append(remaining, s)
		}
	}
	m.seats = remaining
	return nil
}

func TestAutoRevokeRemovedLicenseSeats_Disabled(t *testing.T) {
	ctx := context.Background()
	orgData := types.OrganizationAndTeamData{
		OrganizationID: "org-1",
		TeamID:         "team-1",
	}

	store := &mockAutoLicenseParamsStore{
		configs: map[string]*AutoAssignLicenseConfig{
			"org-1:team-1": {
				AutoRevokeRemovedUsers: false,
			},
		},
	}

	uc := NewAutoRevokeRemovedLicenseSeatsUseCase(store, nil, nil)
	res, err := uc.Execute(ctx, orgData)
	require.NoError(t, err)
	assert.Equal(t, "disabled", res.Status)
	assert.Empty(t, res.Pending)
	assert.Empty(t, res.Revoked)
}

func TestAutoRevokeRemovedLicenseSeats_GracePeriodCountdown(t *testing.T) {
	ctx := context.Background()
	orgData := types.OrganizationAndTeamData{
		OrganizationID: "org-1",
		TeamID:         "team-1",
	}

	t0 := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	currentTime := t0

	// 3 licensed users: git-active, git-missing-recent, git-missing-old
	seatService := &mockLicenseSeatPruneService{
		seats: []string{"git-active", "git-missing-recent", "git-missing-old"},
	}

	// Git provider only has git-active
	mockCM := &mockCodeManagementFull{
		getMembersFunc: func(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error) {
			return []types.PullRequestAuthor{
				{ID: "git-active", Username: "active-dev"},
			}, nil
		},
	}
	memberService := services.NewOrganizationMemberListService(mockCM, nil)
	pruneUseCase := NewPruneRemovedLicenseSeatsUseCase(memberService, seatService)

	// User git-missing-old was first seen missing 8 days ago
	missingOldSeen := t0.Add(-8 * 24 * time.Hour).Format(time.RFC3339)

	store := &mockAutoLicenseParamsStore{
		configs: map[string]*AutoAssignLicenseConfig{
			"org-1:team-1": {
				AutoRevokeRemovedUsers: true,
				RevokeGraceDays:        7,
				PendingRevocations: map[string]string{
					"git-missing-old": missingOldSeen,
				},
			},
		},
	}

	uc := NewAutoRevokeRemovedLicenseSeatsUseCase(store, pruneUseCase, nil)
	uc.clock = func() time.Time { return currentTime }

	// First execution at t0:
	// - git-missing-recent is newly noticed (started pending at t0)
	// - git-missing-old has exceeded 7 days (8 days elapsed) -> must be revoked
	res, err := uc.Execute(ctx, orgData)
	require.NoError(t, err)
	assert.Equal(t, "ok", res.Status)

	// git-missing-old revoked, git-missing-recent pending
	assert.ElementsMatch(t, []string{"git-missing-old"}, res.Revoked)
	assert.ElementsMatch(t, []string{"git-missing-recent"}, res.Pending)
	assert.ElementsMatch(t, []string{"git-missing-old"}, seatService.revoked)

	// Verify state persisted to store
	savedConfig := store.configs["org-1:team-1"]
	require.NotNil(t, savedConfig)
	assert.Contains(t, savedConfig.PendingRevocations, "git-missing-recent")
	assert.NotContains(t, savedConfig.PendingRevocations, "git-missing-old")

	// Advance time by 8 days: now git-missing-recent should also mature and be revoked
	currentTime = t0.Add(8 * 24 * time.Hour)
	seatService.revoked = nil

	res2, err := uc.Execute(ctx, orgData)
	require.NoError(t, err)
	assert.Equal(t, "ok", res2.Status)
	assert.ElementsMatch(t, []string{"git-missing-recent"}, res2.Revoked)
	assert.Empty(t, res2.Pending)
	assert.ElementsMatch(t, []string{"git-missing-recent"}, seatService.revoked)
}

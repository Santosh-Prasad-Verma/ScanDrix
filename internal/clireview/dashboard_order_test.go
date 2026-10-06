package clireview

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDashboardPaginationIsStableAndTenantScoped(t *testing.T) {
	store := NewDashboardStore()
	when := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, record := range []CliReviewSummary{
		{ID: "old", OrganizationID: "workspace-a", Summary: "Auth change", CreatedAt: when.Add(-time.Hour)},
		{ID: "b", OrganizationID: "workspace-a", Summary: "Auth change", CreatedAt: when},
		{ID: "a", OrganizationID: "workspace-a", Summary: "Auth change", CreatedAt: when},
		{ID: "foreign", OrganizationID: "workspace-b", Summary: "Auth change", CreatedAt: when.Add(time.Hour)},
	} {
		store.RecordReview(record)
	}
	for i := 0; i < 10; i++ {
		first := store.GetCliReviews(CliReviewsQuery{OrganizationID: "workspace-a", Search: "auth", Limit: 1})
		second := store.GetCliReviews(CliReviewsQuery{OrganizationID: "workspace-a", Search: "auth", Limit: 1, Offset: 1})
		require.Equal(t, 3, first.Total)
		require.Equal(t, "a", first.Items[0].ID)
		require.Equal(t, "b", second.Items[0].ID)
	}
	_, err := store.GetCliReviewByID("foreign", "workspace-a")
	require.Error(t, err)
}

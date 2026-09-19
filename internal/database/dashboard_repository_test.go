package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/review/application/usecases"
)

func TestPostgresDashboardRepository_NilGuards(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()

	// 1. Completely nil repo
	dashNil := database.NewPostgresDashboardRepository(nil)

	_, err := dashNil.ListReviews(ctx, wsID, 10, 0, usecases.DashboardFilters{})
	if err == nil {
		t.Fatal("expected error on nil repo client for ListReviews")
	}

	_, err = dashNil.CountReviews(ctx, wsID, usecases.DashboardFilters{})
	if err == nil {
		t.Fatal("expected error on nil repo client for CountReviews")
	}

	_, err = dashNil.ListDistinctAuthors(ctx, wsID)
	if err == nil {
		t.Fatal("expected error on nil repo client for ListDistinctAuthors")
	}

	_, err = dashNil.GetDailyDigest(ctx, wsID, time.Now().UTC())
	if err == nil {
		t.Fatal("expected error on nil repo client for GetDailyDigest")
	}

	_, err = dashNil.GetFacets(ctx, wsID, "user1")
	if err == nil {
		t.Fatal("expected error on nil repo client for GetFacets")
	}

	_, err = dashNil.ListAwaiting(ctx, wsID, 10)
	if err == nil {
		t.Fatal("expected error on nil repo client for ListAwaiting")
	}

	_, _, _, err = dashNil.GetFindingsBreakdown(ctx, uuid.New())
	if err == nil {
		t.Fatal("expected error on nil repo client for GetFindingsBreakdown")
	}

	// 2. Repo with nil client
	repo := database.NewRepository(nil)
	dash := database.NewPostgresDashboardRepository(repo)

	_, err = dash.ListReviews(ctx, wsID, 10, 0, usecases.DashboardFilters{})
	if err == nil {
		t.Fatal("expected error on repo with nil client for ListReviews")
	}

	_, err = dash.CountReviews(ctx, wsID, usecases.DashboardFilters{})
	if err == nil {
		t.Fatal("expected error on repo with nil client for CountReviews")
	}

	_, err = dash.ListDistinctAuthors(ctx, wsID)
	if err == nil {
		t.Fatal("expected error on repo with nil client for ListDistinctAuthors")
	}

	_, err = dash.GetDailyDigest(ctx, wsID, time.Now().UTC())
	if err == nil {
		t.Fatal("expected error on repo with nil client for GetDailyDigest")
	}

	_, err = dash.GetFacets(ctx, wsID, "user1")
	if err == nil {
		t.Fatal("expected error on repo with nil client for GetFacets")
	}

	_, err = dash.ListAwaiting(ctx, wsID, 10)
	if err == nil {
		t.Fatal("expected error on repo with nil client for ListAwaiting")
	}

	_, _, _, err = dash.GetFindingsBreakdown(ctx, uuid.New())
	if err == nil {
		t.Fatal("expected error on repo with nil client for GetFindingsBreakdown")
	}
}

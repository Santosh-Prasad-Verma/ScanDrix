package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

// cockpitRepo returns fixed metrics, or an error, to exercise the handler.
type cockpitRepo struct {
	WorkspaceRepository
	metrics *models.CockpitMetrics
	err     error
}

func (r *cockpitRepo) GetCockpitMetrics(context.Context, uuid.UUID) (*models.CockpitMetrics, error) {
	return r.metrics, r.err
}

func cockpitRequest(t *testing.T, ctrl *WorkspaceController) *httptest.ResponseRecorder {
	t.Helper()
	wsID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/workspaces/cockpit", nil)
	ctx := auth.WithWorkspaceContext(req.Context(), wsID)
	ctx = auth.WithAccountContext(ctx, &models.AccountProfile{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Email:       "u@example.test",
		Role:        models.RoleOwner,
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	root := chi.NewRouter()
	root.Mount("/workspaces", ctrl.Routes())
	root.ServeHTTP(w, req)
	return w
}

// TestCockpitPassRateIsNullWhenNoReviews pins AUDIT_REMEDIATION.md F-07.
//
// A workspace with zero reviews used to report a 100% pass rate, because the
// SQL returned 100.0 for a zero denominator and the model was a value type. A
// new customer saw a perfect security score on an empty dashboard.
func TestCockpitPassRateIsNullWhenNoReviews(t *testing.T) {
	ctrl := NewWorkspaceController(&cockpitRepo{
		metrics: &models.CockpitMetrics{
			TotalReviews:       0,
			Unavailable:        []string{"pass_rate_percentage:no_data_source"},
			PassRatePercentage: nil,
		},
	})

	w := cockpitRequest(t, ctrl)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body struct {
		PassRatePercentage *float64 `json:"pass_rate_percentage"`
		TotalReviews       int      `json:"total_reviews"`
		Unavailable        []string `json:"unavailable"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %s)", err, w.Body.String())
	}

	if body.PassRatePercentage != nil {
		t.Fatalf("pass rate must be null with zero reviews, got %v", *body.PassRatePercentage)
	}
	if !strings.Contains(w.Body.String(), `"pass_rate_percentage":null`) {
		t.Fatalf("response must serialise the pass rate as null: %s", w.Body.String())
	}
	if len(body.Unavailable) == 0 {
		t.Fatalf("response must state why the pass rate is absent: %s", w.Body.String())
	}
	if !strings.Contains(body.Unavailable[0], "no_data_source") {
		t.Fatalf("unavailable must carry a documented reason code, got %q", body.Unavailable[0])
	}
	if strings.Contains(w.Body.String(), `"pass_rate_percentage":100`) {
		t.Fatalf("a fabricated 100%% pass rate is still present: %s", w.Body.String())
	}
}

// TestCockpitPassRateIsRealWhenReviewsExist is the counterweight: when data
// exists, the real number must be returned, not suppressed.
func TestCockpitPassRateIsRealWhenReviewsExist(t *testing.T) {
	pass := 87.5
	ctrl := NewWorkspaceController(&cockpitRepo{
		metrics: &models.CockpitMetrics{
			TotalReviews:       40,
			TotalFindings:      5,
			PassRatePercentage: &pass,
		},
	})

	w := cockpitRequest(t, ctrl)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body struct {
		PassRatePercentage *float64 `json:"pass_rate_percentage"`
		TotalReviews       int      `json:"total_reviews"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.PassRatePercentage == nil {
		t.Fatalf("a real pass rate must be returned when reviews exist")
	}
	if *body.PassRatePercentage != 87.5 {
		t.Fatalf("expected 87.5, got %v", *body.PassRatePercentage)
	}
	if body.TotalReviews != 40 {
		t.Fatalf("expected 40 reviews, got %d", body.TotalReviews)
	}
}

// TestCockpitFailsClosedOnRepositoryError pins the outage case: a database
// error must not be rendered as a perfect score.
func TestCockpitFailsClosedOnRepositoryError(t *testing.T) {
	ctrl := NewWorkspaceController(&cockpitRepo{err: context.DeadlineExceeded})

	w := cockpitRequest(t, ctrl)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on a repository error, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "100") {
		t.Fatalf("an outage was reported as a metric: %s", w.Body.String())
	}
}

// TestCockpitFailsClosedWithoutRepository covers the nil-repo path, which also
// used to yield 100.0.
func TestCockpitFailsClosedWithoutRepository(t *testing.T) {
	ctrl := NewWorkspaceController(nil)

	w := cockpitRequest(t, ctrl)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with no repository, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "pass_rate_percentage\":100") {
		t.Fatalf("nil repository still produced a fabricated pass rate: %s", w.Body.String())
	}
}

// TestCockpitRequiresWorkspaceContext ensures tenant scoping is still enforced.
func TestCockpitRequiresWorkspaceContext(t *testing.T) {
	ctrl := NewWorkspaceController(&cockpitRepo{metrics: &models.CockpitMetrics{}})

	req := httptest.NewRequest(http.MethodGet, "/workspaces/cockpit", nil)
	w := httptest.NewRecorder()
	root := chi.NewRouter()
	root.Mount("/workspaces", ctrl.Routes())
	root.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without workspace context, got %d: %s", w.Code, w.Body.String())
	}
}

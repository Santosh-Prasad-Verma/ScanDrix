// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System Controller Tests
// File: rule_like_controller_test.go
// ═══════════════════════════════════════════════════════════════

package controllers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/rules/drixy/application/usecases"
	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/repositories"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
	"github.com/scandrix/backend/pkg/models"
)

// withRuleLikeSession installs a real authenticated context on the request.
//
// The test previously identified the caller with an `x-user-id` header. That
// header fallback was removed from resolveUserID because it is client-controlled
// identity (AUDIT_REMEDIATION.md F-37, and the F-02 pattern), and the workspace
// now has to come from the session so the repository can open a tenant-scoped
// RLS transaction. Tests must supply a session like the real middleware does.
func withRuleLikeSession(req *http.Request, wsID uuid.UUID) *http.Request {
	ctx := auth.WithWorkspaceContext(req.Context(), wsID)
	ctx = auth.WithAccountContext(ctx, &models.AccountProfile{
		ID:          uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		WorkspaceID: wsID,
		Email:       "rule-like-test@example.test",
		Role:        models.RoleMember,
	})
	return req.WithContext(ctx)
}

func TestRuleLikeController_VoteAndRemove(t *testing.T) {
	wsID := uuid.New()
	ruleLikeRepo := repositories.NewPostgresRuleLikeRepository(nil)
	ruleLikeSvc := services.NewRuleLikeService(ruleLikeRepo)

	setLikeUC := usecases.NewSetRuleLikeUseCase(ruleLikeSvc)
	removeLikeUC := usecases.NewRemoveRuleLikeUseCase(ruleLikeSvc)

	ctrl := controllers.NewRuleLikeController(setLikeUC, removeLikeUC)

	r := chi.NewRouter()
	r.Mount("/api/v1/rule-like", ctrl.Routes())

	// 1. Post upvote
	votePayload, _ := json.Marshal(dtos.SetRuleFeedbackDto{
		Feedback: entities.RuleFeedbackTypePositive,
	})
	req := httptest.NewRequest("POST", "/api/v1/rule-like/rule-lib-123/feedback", bytes.NewReader(votePayload))
	req.Header.Set("Content-Type", "application/json")
	req = withRuleLikeSession(req, wsID)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on vote, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Delete vote
	delReq := httptest.NewRequest("DELETE", "/api/v1/rule-like/rule-lib-123/feedback", nil)
	delReq = withRuleLikeSession(delReq, wsID)

	wDel := httptest.NewRecorder()
	r.ServeHTTP(wDel, delReq)

	if wDel.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on remove vote, got %d: %s", wDel.Code, wDel.Body.String())
	}
}

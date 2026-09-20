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
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/rules/drixy/application/usecases"
	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/repositories"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

func TestRuleLikeController_VoteAndRemove(t *testing.T) {
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
	req.Header.Set("x-user-id", "user-voter-1")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on vote, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Delete vote
	delReq := httptest.NewRequest("DELETE", "/api/v1/rule-like/rule-lib-123/feedback", nil)
	delReq.Header.Set("x-user-id", "user-voter-1")

	wDel := httptest.NewRecorder()
	r.ServeHTTP(wDel, delReq)

	if wDel.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on remove vote, got %d: %s", wDel.Code, wDel.Body.String())
	}
}

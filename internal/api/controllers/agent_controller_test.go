package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/agents/businessrules"
	"github.com/scandrix/backend/internal/agents/conversation"
	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/persistence"
)

type testRunner struct {
	answer string
}

func (t *testRunner) Run(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
	return &contracts.RunState{
		Status: contracts.StatusCompleted,
		Steps: []contracts.RunStep{
			{
				Index: 0,
				Message: contracts.AgentMessage{
					Role:    contracts.RoleAssistant,
					Content: t.answer,
				},
			},
		},
		Usage: contracts.TokenUsage{InputTokens: 50, OutputTokens: 50},
	}, nil
}

func TestAgentController_Conversation(t *testing.T) {
	runner := &testRunner{
		answer: `{"content": "This is ScanDrix answering your question."}`,
	}
	store := persistence.NewInMemoryConversationStore(10)
	convProvider := conversation.NewConversationAgentProvider(runner, store)
	brvProvider := businessrules.NewBusinessRulesValidationAgentProvider(runner)

	controller := NewAgentController(convProvider, brvProvider)
	router := controller.Routes()

	body := AgentConversationRequestBody{
		Prompt: "Hello agent",
		OrganizationAndTeamData: OrganizationAndTeamDataDto{
			OrganizationID: "org-test",
			TeamID:         "team-alpha",
		},
		ConversationID: "conv-123",
	}

	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/conversation", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var res conversation.ConversationResponse
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if res.Response != "This is ScanDrix answering your question." {
		t.Errorf("unexpected response text: %s", res.Response)
	}
	if res.ThreadID != "cmc:org-test:team-alpha:conv-123" {
		t.Errorf("unexpected threadID: %s", res.ThreadID)
	}
}

func TestAgentController_BusinessRulesValidation(t *testing.T) {
	runner := &testRunner{
		answer: `{
			"is_compliant": true,
			"needs_more_info": false,
			"summary": "Verified: all acceptance criteria implemented in the diff.",
			"missing_requirements": []
		}`,
	}
	convProvider := conversation.NewConversationAgentProvider(runner, persistence.NewInMemoryConversationStore(10))
	brvProvider := businessrules.NewBusinessRulesValidationAgentProvider(runner)

	controller := NewAgentController(convProvider, brvProvider)
	router := controller.Routes()

	body := BusinessRulesValidationRequestBody{
		TaskContext: "- [x] Add rate limiting",
		PRDiff:      "+ func RateLimit() { ... }",
		OrganizationAndTeamData: OrganizationAndTeamDataDto{
			OrganizationID: "org-test",
			TeamID:         "team-alpha",
		},
	}

	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/business-rules-validation", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var res businessrules.ValidationResult
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if !res.IsCompliant {
		t.Errorf("expected compliant, got %v", res)
	}
}

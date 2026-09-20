package controllers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/rules"
)

func TestRulesControllerCRUDAndGenerate(t *testing.T) {
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())
	ctrl := controllers.NewRulesController(evaluator)
	routes := ctrl.Routes()

	wsID := uuid.New()

	// 1. GET /catalog
	reqCat := httptest.NewRequest(http.MethodGet, "/catalog", nil)
	wCat := httptest.NewRecorder()
	routes.ServeHTTP(wCat, reqCat)
	if wCat.Code != http.StatusOK {
		t.Fatalf("expected 200 for /catalog, got %d", wCat.Code)
	}

	// 2a. POST /generate without LLM gateway -> 503 Service Unavailable
	genReq := dtos.GenerateRuleRequest{
		Prompt: "Prohibit raw MD5 hashing in authentication flow",
	}
	genBody, _ := json.Marshal(genReq)
	reqGen := httptest.NewRequest(http.MethodPost, "/generate", bytes.NewReader(genBody))
	reqGen.Header.Set("Content-Type", "application/json")
	wGen := httptest.NewRecorder()
	routes.ServeHTTP(wGen, reqGen)
	if wGen.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without LLM gateway, got %d", wGen.Code)
	}

	// 2b. POST /generate with LLM gateway attached -> 200 OK
	mockLLMServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		respPayload := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": `{
							"name": "Prohibit Raw MD5 Hashing",
							"regex_rule": "md5\\.New\\(\\)",
							"path_pattern": "**/*.go",
							"severity": "CRITICAL",
							"category": "SECURITY",
							"description": "MD5 is cryptographically vulnerable to collision attacks",
							"remediation": "Use sha256 or bcrypt instead",
							"explanation": "Flags md5.New instantiation"
						}`,
					},
				},
			},
		}
		_ = json.NewEncoder(rw).Encode(respPayload)
	}))
	defer mockLLMServer.Close()

	llmGw := llm.NewGateway("", "mock-key", "", "", llm.WithOpenAIBaseURL(mockLLMServer.URL))
	ctrl.SetLLMGateway(llmGw)

	reqGen2 := httptest.NewRequest(http.MethodPost, "/generate", bytes.NewReader(genBody))
	reqGen2.Header.Set("Content-Type", "application/json")
	wGen2 := httptest.NewRecorder()
	routes.ServeHTTP(wGen2, reqGen2)
	if wGen2.Code != http.StatusOK {
		t.Fatalf("expected 200 for /generate with LLM gateway, got %d: %s", wGen2.Code, wGen2.Body.String())
	}
	var genResp dtos.GenerateRuleResponse
	if err := json.NewDecoder(wGen2.Body).Decode(&genResp); err != nil || genResp.Name != "Prohibit Raw MD5 Hashing" {
		t.Fatalf("invalid generate rule response: %+v, err: %v", genResp, err)
	}

	// 3. POST / (Create Rule with workspace context)
	createReq := dtos.CreateRuleRequest{
		Name:        "No Raw MD5",
		RegexRule:   `md5\.New\(\)`,
		Severity:    "HIGH",
		Category:    "SECURITY",
		Description: "MD5 is cryptographically broken",
		Enabled:     true,
	}
	createBody, _ := json.Marshal(createReq)
	reqCreate := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(createBody))
	reqCreate.Header.Set("Content-Type", "application/json")
	ctx := auth.WithWorkspaceContext(reqCreate.Context(), wsID)
	reqCreate = reqCreate.WithContext(ctx)
	wCreate := httptest.NewRecorder()
	routes.ServeHTTP(wCreate, reqCreate)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 for POST /, got %d, body: %s", wCreate.Code, wCreate.Body.String())
	}

	// 4. GET / (List Rules)
	reqList := httptest.NewRequest(http.MethodGet, "/", nil)
	reqList = reqList.WithContext(ctx)
	wList := httptest.NewRecorder()
	routes.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET /, got %d", wList.Code)
	}

	// 5. DELETE /{ruleID}
	ruleID := uuid.New()
	reqDel := httptest.NewRequest(http.MethodDelete, "/"+ruleID.String(), nil)
	reqDel = reqDel.WithContext(ctx)
	wDel := httptest.NewRecorder()
	routes.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("expected 200 for DELETE /{ruleID}, got %d", wDel.Code)
	}
}

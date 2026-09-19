package rulescli_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/cli/rulescli"
)

func TestRulesInitAndValidate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scandrix-rules-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 1. Initialize starter rules
	path, err := rulescli.Init(tempDir)
	if err != nil || path == "" {
		t.Fatalf("rules init failed: %v", err)
	}

	// 2. Validate rules
	count, err := rulescli.Validate(tempDir)
	if err != nil || count == 0 {
		t.Fatalf("rules validate failed: %v", err)
	}

	// 3. Double init should fail cleanly
	if _, err := rulescli.Init(tempDir); err == nil {
		t.Fatal("expected error on re-initializing existing rules file")
	}

	// 4. Test ActiveRulesPath
	active := rulescli.ActiveRulesPath(tempDir)
	if active == "" {
		t.Fatal("expected active rules path to be non-empty")
	}
}

func TestRulesGenerate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rules/generate" {
			http.NotFound(w, r)
			return
		}
		var req dtos.GenerateRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := dtos.GenerateRuleResponse{
			Name:        "Test Rule",
			RegexRule:   `test_pattern`,
			Severity:    "HIGH",
			Category:    "SECURITY",
			Description: "Test description",
			Remediation: "Fix it",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	res, err := rulescli.GenerateRule(context.Background(), ts.URL, "token123", "test prompt")
	if err != nil {
		t.Fatalf("GenerateRule failed: %v", err)
	}
	if res.Name != "Test Rule" || res.RegexRule != "test_pattern" {
		t.Fatalf("unexpected rule result: %+v", res)
	}
}

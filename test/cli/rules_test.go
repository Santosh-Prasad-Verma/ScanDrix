// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cli_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/cli/cmd"
	"github.com/scandrix/backend/internal/cli/services/api"
)

// RULES API CLIENT INTEGRATION TESTS

func TestRulesAPI_CreateRule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.URL.Path != "/cli/drixy-rules" && r.URL.Path != "/cli/rules" && r.URL.Path != "/api/v1/rules") || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}

		var payload map[string]string
		_ = json.NewDecoder(r.Body).Decode(&payload)

		if payload["title"] != "No Hardcoded Secrets" {
			http.Error(w, "invalid title", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.RuleMutationResponse{
			Rule: &api.RuleModel{
				UUID:        "rule-uuid-123",
				Title:       payload["title"],
				Rule:        payload["rule"],
				Severity:    payload["severity"],
				Scope:       payload["scope"],
				Path:        payload["path"],
			},
			Message: "Rule created successfully",
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "bearer-test-token", "")
	res, err := client.CreateRule(context.Background(), "No Hardcoded Secrets", "Detect AWS keys in strings", "repo-1", "critical", "security", "internal/**")
	if err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}

	if res.Rule == nil || res.Rule.UUID != "rule-uuid-123" {
		t.Fatalf("expected rule-uuid-123, got %+v", res.Rule)
	}
	if res.Rule.Severity != "critical" {
		t.Errorf("expected severity critical, got %s", res.Rule.Severity)
	}
}

func TestRulesAPI_UpdateRule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		validPath := r.URL.Path == "/cli/drixy-rules/rule-uuid-123" || r.URL.Path == "/cli/rules/rule-uuid-123" || r.URL.Path == "/api/v1/rules/rule-uuid-123"
		validMethod := r.Method == http.MethodPatch || r.Method == http.MethodPut
		if !validPath || !validMethod {
			http.NotFound(w, r)
			return
		}

		var payload map[string]string
		_ = json.NewDecoder(r.Body).Decode(&payload)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.RuleMutationResponse{
			Rule: &api.RuleModel{
				UUID:     "rule-uuid-123",
				Title:    payload["title"],
				Severity: "high",
			},
			Message: "Rule updated successfully",
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "bearer-test-token", "")
	res, err := client.UpdateRule(context.Background(), "rule-uuid-123", "Updated Rule", "new pattern", "repo-1", "high", "all", "")
	if err != nil {
		t.Fatalf("UpdateRule failed: %v", err)
	}

	if res.Rule.Severity != "high" {
		t.Errorf("expected updated severity high, got %s", res.Rule.Severity)
	}
}

func TestRulesAPI_ViewRules(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.URL.Path != "/cli/drixy-rules" && r.URL.Path != "/cli/rules" && r.URL.Path != "/api/v1/rules") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		repoID := r.URL.Query().Get("repositoryId")
		if repoID == "" {
			repoID = r.URL.Query().Get("repo_id")
		}
		if repoID != "repo-456" {
			http.Error(w, "missing or invalid repo_id filter", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]api.RuleModel{
			{UUID: "r1", Title: "Rule 1", Severity: "critical", RepoID: repoID},
			{UUID: "r2", Title: "Rule 2", Severity: "medium", RepoID: repoID},
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "bearer-test-token", "")
	rules, err := client.ViewRules(context.Background(), "", "repo-456")
	if err != nil {
		t.Fatalf("ViewRules failed: %v", err)
	}

	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Title != "Rule 1" {
		t.Errorf("expected Rule 1, got %s", rules[0].Title)
	}
}

func TestRulesAPI_GenerateRule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rules/generate" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"title":       "Generated SQL Injection Rule",
			"rule":        "pattern: db.Query(fmt.Sprintf(...))",
			"severity":    "critical",
			"scope":       "security",
			"description": "Flags string formatted SQL queries",
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "bearer-test-token", "")
	gen, err := client.GenerateRule(context.Background(), "detect string formatting inside sql query")
	if err != nil {
		t.Fatalf("GenerateRule failed: %v", err)
	}

	if gen.Title != "Generated SQL Injection Rule" {
		t.Errorf("unexpected title: %s", gen.Title)
	}
	if gen.Severity != "critical" {
		t.Errorf("unexpected severity: %s", gen.Severity)
	}
}

// RULES COBRA COMMAND STRUCTURE & FLAG REGISTRATION TESTS

func TestRulesCommand_Subcommands(t *testing.T) {
	var rulesCmdName = "rules"
	var found bool

	for _, c := range cmd.RootCmd.Commands() {
		if c.Name() == rulesCmdName {
			found = true
			expectedSubcommands := []string{"create", "update", "view", "init", "validate", "generate"}
			subNames := make(map[string]bool)
			for _, sc := range c.Commands() {
				subNames[sc.Name()] = true
			}

			for _, exp := range expectedSubcommands {
				if !subNames[exp] {
					t.Errorf("expected subcommand '%s' in 'rules' command", exp)
				}
			}
			break
		}
	}

	if !found {
		t.Fatalf("command 'rules' not found in RootCmd")
	}
}

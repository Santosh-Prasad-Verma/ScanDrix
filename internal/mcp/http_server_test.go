// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise MCP HTTP Server Tests
// File: http_server_test.go
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestStreamableHTTPServerProtocol(t *testing.T) {
	server := NewServer()
	handler := NewHTTPServer(server)

	// 1. GET /mcp must return 405 Method Not Allowed with Allow: POST header and JSON-RPC error
	reqGet, _ := http.NewRequest(http.MethodGet, "/mcp", nil)
	rrGet := httptest.NewRecorder()
	handler.ServeHTTP(rrGet, reqGet)

	if rrGet.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed on GET, got %d", rrGet.Code)
	}
	if rrGet.Header().Get("Allow") != "POST" {
		t.Fatalf("expected Allow: POST header, got %s", rrGet.Header().Get("Allow"))
	}
	if rrGet.Header().Get("Access-Control-Expose-Headers") == "" {
		t.Fatalf("expected Access-Control-Expose-Headers to be set on 405 response")
	}

	var errResp JSONRPCResponse
	if err := json.NewDecoder(rrGet.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed decoding 405 JSON-RPC error body: %v", err)
	}
	errMap, ok := errResp.Error.(map[string]any)
	if !ok || errMap["message"] != "Method not allowed." {
		t.Fatalf("expected JSON-RPC 'Method not allowed.' error, got: %v", errResp.Error)
	}

	// 2. DELETE /mcp must return 405 Method Not Allowed
	reqDel, _ := http.NewRequest(http.MethodDelete, "/mcp", nil)
	rrDel := httptest.NewRecorder()
	handler.ServeHTTP(rrDel, reqDel)

	if rrDel.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 on DELETE, got %d", rrDel.Code)
	}

	// 3. POST /mcp with initialize
	initReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}
	initBody, _ := json.Marshal(initReq)
	reqInit, _ := http.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(initBody))
	rrInit := httptest.NewRecorder()
	handler.ServeHTTP(rrInit, reqInit)

	if rrInit.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on POST initialize, got %d", rrInit.Code)
	}
	var initResp JSONRPCResponse
	if err := json.NewDecoder(rrInit.Body).Decode(&initResp); err != nil {
		t.Fatalf("failed decoding init response: %v", err)
	}
	resultMap, ok := initResp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected result map in initialize response")
	}
	serverInfo, ok := resultMap["serverInfo"].(map[string]any)
	if !ok || serverInfo["name"] != "scandrix-code-management" {
		t.Fatalf("expected serverInfo.name scandrix-code-management, got %v", serverInfo)
	}

	// 4. POST /mcp with tools/list
	listReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}
	listBody, _ := json.Marshal(listReq)
	reqList, _ := http.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(listBody))
	rrList := httptest.NewRecorder()
	handler.ServeHTTP(rrList, reqList)

	var listResp JSONRPCResponse
	_ = json.NewDecoder(rrList.Body).Decode(&listResp)
	toolsResult, ok := listResp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected tools result map")
	}
	toolsList, ok := toolsResult["tools"].([]any)
	if !ok || len(toolsList) == 0 {
		t.Fatalf("expected non-empty tools list, got %v", toolsList)
	}

	// Verify code management, issue, rule, and diff review tools are listed
	foundMap := make(map[string]bool)
	for _, raw := range toolsList {
		if tMap, ok := raw.(map[string]any); ok {
			if name, ok := tMap["name"].(string); ok {
				foundMap[name] = true
			}
		}
	}

	expectedTools := []string{
		"SCANDRIX_LIST_REPOSITORIES",
		"list_repositories",
		"SCANDRIX_LIST_PULL_REQUESTS",
		"list_pull_requests",
		"SCANDRIX_LIST_COMMITS",
		"list_commits",
		"SCANDRIX_GET_PULL_REQUEST",
		"get_pull_request_details",
		"SCANDRIX_GET_REPOSITORY_FILES",
		"get_repository_files",
		"SCANDRIX_GET_REPOSITORY_CONTENT",
		"get_repository_content",
		"SCANDRIX_GET_REPOSITORY_LANGUAGES",
		"get_repository_languages",
		"SCANDRIX_GET_PULL_REQUEST_FILE_CONTENT",
		"get_pull_request_file_content",
		"SCANDRIX_GET_DIFF_FOR_FILE",
		"get_diff_for_file",
		"SCANDRIX_GET_PULL_REQUEST_DIFF",
		"get_pull_request_diff",
		"SCANDRIX_CREATE_ISSUE",
		"create_issue",
		"SCANDRIX_LIST_ISSUES",
		"list_tracked_issues",
		"SCANDRIX_GET_ISSUE_DETAILS",
		"get_issue_details",
		"SCANDRIX_UPDATE_ISSUE_STATUS",
		"update_issue_status",
		"SCANDRIX_UPDATE_ISSUE_CATEGORY",
		"update_issue_category",
		"SCANDRIX_DELETE_ISSUE",
		"delete_issue",
		"DRIXY_GET_RULES",
		"SCANDRIX_GET_RULES",
		"get_drixy_rules",
		"list_rules",
		"DRIXY_CREATE_RULE",
		"SCANDRIX_CREATE_RULE",
		"create_drixy_rule",
		"create_rule",
		"DRIXY_CREATE_MEMORY",
		"SCANDRIX_CREATE_MEMORY",
		"create_drixy_memory",
		"create_memory",
		"DRIXY_FIND_MEMORIES",
		"SCANDRIX_FIND_MEMORIES",
		"find_drixy_memories",
		"find_memories",
		"drixy_review_diff",
	}

	for _, exp := range expectedTools {
		if !foundMap[exp] {
			t.Errorf("expected tool %s to be listed in MCP catalog", exp)
		}
	}

	// 5. POST /mcp with tools/call for SCANDRIX_CREATE_ISSUE
	callParams, _ := json.Marshal(map[string]any{
		"name": "SCANDRIX_CREATE_ISSUE",
		"arguments": map[string]any{
			"organizationId": "org-test-uuid",
			"title":          "Hardcoded secret detected in auth_config.go",
			"severity":       "CRITICAL",
			"category":       "SECURITY",
			"filePath":       "internal/auth/auth_config.go",
			"line":           42,
		},
	})
	callReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params:  callParams,
	}
	callBody, _ := json.Marshal(callReq)
	reqCall, _ := http.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(callBody))
	rrCall := httptest.NewRecorder()
	handler.ServeHTTP(rrCall, reqCall)

	if rrCall.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on tools/call, got %d", rrCall.Code)
	}

	var callResp JSONRPCResponse
	_ = json.NewDecoder(rrCall.Body).Decode(&callResp)
	if callResp.Error != nil {
		t.Fatalf("expected no error in tools/call, got %v", callResp.Error)
	}

	// 6. POST /mcp/issues with tools/list
	issuesListReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/list",
	}
	issuesBody, _ := json.Marshal(issuesListReq)
	reqIssues, _ := http.NewRequest(http.MethodPost, "/mcp/issues", bytes.NewReader(issuesBody))
	rrIssues := httptest.NewRecorder()
	handler.ServeHTTP(rrIssues, reqIssues)

	var issuesResp JSONRPCResponse
	_ = json.NewDecoder(rrIssues.Body).Decode(&issuesResp)
	issMap := issuesResp.Result.(map[string]any)
	issList := issMap["tools"].([]any)
	if len(issList) == 0 {
		t.Fatalf("expected issues tools on /mcp/issues, got %d", len(issList))
	}
}

func TestMcpServerEnabledGuard(t *testing.T) {
	os.Setenv("SCANDRIX_MCP_SERVER_ENABLED", "false")
	defer os.Unsetenv("SCANDRIX_MCP_SERVER_ENABLED")

	server := NewServer()
	handler := NewHTTPServer(server)

	req, _ := http.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when SCANDRIX_MCP_SERVER_ENABLED=false, got %d", rr.Code)
	}

	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["message"] != "MCP Service is disabled" {
		t.Fatalf("expected 'MCP Service is disabled', got: %v", body["message"])
	}
}

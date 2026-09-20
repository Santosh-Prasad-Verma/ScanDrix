package gateway_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/mcp/gateway"
)

func TestMCPServeStdioAndBuiltinTools(t *testing.T) {
	server := gateway.NewMCPServer()

	// Prepare sequence of JSON-RPC requests
	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"scandrix_validate_syntax","role":"admin","arguments":{"code":"x := 10 + 20"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"scandrix_scan","role":"admin","arguments":{"path":".","fast":true}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"resources/read","params":{"uri":"scandrix://rules/catalog"}}`,
	}

	input := strings.Join(requests, "\n") + "\n"
	inBuf := strings.NewReader(input)
	outBuf := &bytes.Buffer{}

	ctx := context.Background()
	if err := server.ServeStdio(ctx, inBuf, outBuf); err != nil {
		t.Fatalf("ServeStdio error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(outBuf.String()), "\n")
	if len(lines) != len(requests) {
		t.Fatalf("expected %d responses, got %d. Output:\n%s", len(requests), len(lines), outBuf.String())
	}

	// 1. Validate initialize response
	var initResp gateway.JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &initResp); err != nil {
		t.Fatalf("failed decoding initialize response: %v", err)
	}
	if initResp.Error != nil {
		t.Fatalf("initialize returned error: %+v", initResp.Error)
	}

	// 2. Validate tools/list response
	var listResp gateway.JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[1]), &listResp); err != nil {
		t.Fatalf("failed decoding tools/list response: %v", err)
	}
	resultMap, ok := listResp.Result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected tools/list result type: %+v", listResp.Result)
	}
	toolsArr, ok := resultMap["tools"].([]any)
	if !ok || len(toolsArr) < 7 {
		t.Fatalf("expected at least 7 tools, got %d", len(toolsArr))
	}

	// 3. Validate syntax check response
	var syntaxResp gateway.JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[2]), &syntaxResp); err != nil {
		t.Fatalf("failed decoding syntax response: %v", err)
	}
	if syntaxResp.Error != nil {
		t.Fatalf("syntax check returned error: %+v", syntaxResp.Error)
	}

	// 4. Validate scan response
	var scanResp gateway.JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[3]), &scanResp); err != nil {
		t.Fatalf("failed decoding scan response: %v", err)
	}
	if scanResp.Error != nil {
		t.Fatalf("scan check returned error: %+v", scanResp.Error)
	}

	// 5. Validate resource read response
	var resResp gateway.JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[4]), &resResp); err != nil {
		t.Fatalf("failed decoding resource response: %v", err)
	}
	if resResp.Error != nil {
		t.Fatalf("resource read returned error: %+v", resResp.Error)
	}
}

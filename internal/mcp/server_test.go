package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/mcp"
)

func TestMCPServerStdio(t *testing.T) {
	ctx := context.Background()
	srv := mcp.NewServer()

	// 1. Test Initialize
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"
	// 2. Test Tools List
	input += `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n"
	// 3. Test Tools Call
	diffPayload := "diff --git a/app.go b/app.go\n+var token = \"AKIA1234567890123456\""
	callParams, _ := json.Marshal(map[string]any{
		"name": "scandrix_review_diff",
		"arguments": map[string]string{
			"diff": diffPayload,
		},
	})
	input += `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":` + string(callParams) + `}` + "\n"
	// 4. Test Unknown Method
	input += `{"jsonrpc":"2.0","id":4,"method":"unknown/method","params":{}}` + "\n"
	// 5. Test Malformed JSON
	input += `{invalid json}` + "\n"

	inBuf := strings.NewReader(input)
	outBuf := &bytes.Buffer{}

	err := srv.ServeStdio(ctx, inBuf, outBuf)
	if err != nil {
		t.Fatalf("ServeStdio failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(outBuf.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 responses, got: %d (%s)", len(lines), outBuf.String())
	}

	// Verify initialize response
	var initResp mcp.JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &initResp); err != nil {
		t.Fatalf("failed unmarshaling init response: %v", err)
	}
	if initResp.Result == nil {
		t.Fatal("expected result for initialize")
	}

	// Verify tools list response
	var listResp mcp.JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[1]), &listResp); err != nil {
		t.Fatalf("failed unmarshaling list response: %v", err)
	}
	if listResp.Result == nil {
		t.Fatal("expected result for tools/list")
	}

	// Verify tools call response
	var callResp mcp.JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[2]), &callResp); err != nil {
		t.Fatalf("failed unmarshaling call response: %v", err)
	}
	if callResp.Result == nil {
		t.Fatal("expected result for tools/call")
	}

	// Verify unknown method error
	var unkResp mcp.JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[3]), &unkResp); err != nil {
		t.Fatalf("failed unmarshaling unknown method response: %v", err)
	}
	if unkResp.Error == nil {
		t.Fatal("expected error for unknown method")
	}

	// Verify parse error
	var parseResp mcp.JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[4]), &parseResp); err != nil {
		t.Fatalf("failed unmarshaling parse error response: %v", err)
	}
	if parseResp.Error == nil {
		t.Fatal("expected error for parse error")
	}
}

// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp_manager_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/mcp/manager/client"
	"github.com/scandrix/backend/internal/mcp/manager/models"
)

func TestJSONRPCClientGetToolsSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "invalid content type", http.StatusBadRequest)
			return
		}

		var req client.JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		toolsRaw, _ := json.Marshal(map[string]any{
			"tools": []map[string]any{
				{
					"name":        "fetch_data",
					"description": "Fetches remote data",
					"annotations": map[string]any{
						"readOnlyHint": true,
					},
				},
			},
		})

		resp := client.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  toolsRaw,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	c := client.NewMCPClient(ts.URL, map[string]string{"Authorization": "Bearer test-token"}, "test-server", models.ProviderCustom)
	ctx := context.Background()

	tools, err := c.GetTools(ctx)
	if err != nil {
		t.Fatalf("expected successful call, got error: %v", err)
	}

	if len(tools) != 1 || tools[0].Slug != "fetch_data" {
		t.Fatalf("expected tool fetch_data, got: %v", tools)
	}
	if !tools[0].ReadOnly {
		t.Errorf("expected readOnlyHint to be preserved")
	}
}

func TestJSONRPCClientCallToolSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req client.JSONRPCRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		resultRaw, _ := json.Marshal(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "success data"},
			},
		})

		resp := client.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resultRaw,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	c := client.NewMCPClient(ts.URL, nil, "test-server", models.ProviderCustom)
	ctx := context.Background()

	res, err := c.CallTool(ctx, "sample_tool", map[string]any{"query": "test"})
	if err != nil {
		t.Fatalf("expected CallTool success, got: %v", err)
	}

	if res == "" {
		t.Fatalf("expected non-empty CallTool success string, got empty")
	}
}

func TestJSONRPCClientErrorResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := client.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Error: map[string]any{
				"code":    -32601,
				"message": "Method not found",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	c := client.NewMCPClient(ts.URL, nil, "test-server", models.ProviderCustom)
	ctx := context.Background()

	_, err := c.GetTools(ctx)
	if err == nil {
		t.Fatalf("expected error from server, got nil")
	}
}

func TestJSONRPCClientHTTPFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := client.NewMCPClient(ts.URL, nil, "test-server", models.ProviderCustom)
	ctx := context.Background()

	_, err := c.GetTools(ctx)
	if err == nil {
		t.Fatalf("expected HTTP 500 failure error, got nil")
	}
}

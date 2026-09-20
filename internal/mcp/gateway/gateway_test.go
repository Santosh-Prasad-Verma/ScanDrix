package gateway_test

import (
	"context"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/mcp/gateway"
)

func TestMCPGatewayServerAndClient(t *testing.T) {
	ctx := gateway.WithCallerRole(context.Background(), gateway.RoleReviewer)

	// 1. Initialize Server and Client
	server := gateway.NewMCPServer()
	client := gateway.NewInProcessClient(server)

	// 2. Handshake / Initialize
	initResult, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("client initialization failed: %v", err)
	}
	serverInfo, ok := initResult["serverInfo"].(map[string]string)
	if !ok || serverInfo["name"] != "scandrix-mcp-gateway" {
		t.Fatalf("unexpected serverInfo: %+v", initResult)
	}

	// 3. Tool Discovery
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("failed listing tools: %v", err)
	}
	if len(tools) < 3 {
		t.Fatalf("expected at least 3 tools, got %d", len(tools))
	}

	toolMap := make(map[string]bool)
	for _, tool := range tools {
		toolMap[tool.Name] = true
	}
	if !toolMap["scandrix_analyze_snippet"] || !toolMap["scandrix_catalog_search"] || !toolMap["scandrix_validate_syntax"] {
		t.Fatalf("missing expected native tools in list: %+v", toolMap)
	}

	// 4. Execute Code Analysis Tool with Insecure Snippet
	insecureCode := `func GetKey() string { return "AKIAIOSFODNN7EXAMPLE" }`
	analysisRes, err := client.CallTool(ctx, "scandrix_analyze_snippet", map[string]any{
		"language": "go",
		"code":     insecureCode,
	})
	if err != nil {
		t.Fatalf("call to scandrix_analyze_snippet failed: %v", err)
	}
	if len(analysisRes.Content) == 0 || !strings.Contains(analysisRes.Content[0].Text, "Security findings detected") {
		t.Fatalf("expected detection of AKIA key, got %+v", analysisRes)
	}

	// 5. Execute Syntax Validator on Valid Snippet
	validCode := `x := 10 + 20`
	validRes, err := client.CallTool(ctx, "scandrix_validate_syntax", map[string]any{
		"language": "go",
		"code":     validCode,
	})
	if err != nil || validRes.IsError {
		t.Fatalf("expected valid syntax, got: %+v, err: %v", validRes, err)
	}

	// 6. Execute Syntax Validator on Broken Snippet
	brokenCode := `func Broken( { return`
	brokenRes, err := client.CallTool(ctx, "scandrix_validate_syntax", map[string]any{
		"language": "go",
		"code":     brokenCode,
	})
	if err != nil {
		t.Fatalf("unexpected call failure: %v", err)
	}
	if !brokenRes.IsError || !strings.Contains(brokenRes.Content[0].Text, "Syntax error") {
		t.Fatalf("expected syntax error flag, got: %+v", brokenRes)
	}

	// 7. Read Resource
	resourceText, err := client.ReadResource(ctx, "scandrix://rules/catalog")
	if err != nil || resourceText == "" {
		t.Fatalf("failed reading resource: %v, text: %s", err, resourceText)
	}
	if !strings.Contains(resourceText, "OWASP") && !strings.Contains(resourceText, "CRITICAL") {
		t.Fatalf("resource missing catalog content: %s", resourceText)
	}

	// 8. Missing Tool Error Handling
	missingRes, err := client.CallTool(ctx, "non_existent_tool", map[string]any{})
	if err == nil && !missingRes.IsError {
		t.Fatalf("expected error for non-existent tool, got nil")
	}
}

func TestMCPGatewayRoleBasedAccessControl(t *testing.T) {
	server := gateway.NewMCPServer()

	// Register an admin-only destructive tool
	server.RegisterTool(gateway.Tool{
		Name:         "scandrix_delete_repo_comments",
		Description:  "Deletes all previous PR comments (Admin only)",
		AllowedRoles: []gateway.AgentRole{gateway.RoleAdmin},
		IsReadOnly:   false,
		InputSchema:  map[string]any{"type": "object"},
	}, func(ctx context.Context, args map[string]any) (*gateway.ToolCallResult, error) {
		return &gateway.ToolCallResult{
			Content: []gateway.ToolContent{{Type: "text", Text: "deleted 5 comments"}},
		}, nil
	})

	client := gateway.NewInProcessClient(server)

	// 1. Reviewer role should be FORBIDDEN from calling admin tool
	reviewerCtx := gateway.WithCallerRole(context.Background(), gateway.RoleReviewer)
	resForbidden, err := client.CallTool(reviewerCtx, "scandrix_delete_repo_comments", map[string]any{})
	if err == nil && !resForbidden.IsError {
		t.Fatalf("expected reviewer role to be blocked from admin-only tool")
	}

	// 2. Admin role should SUCCEED calling admin tool
	adminCtx := gateway.WithCallerRole(context.Background(), gateway.RoleAdmin)
	resAdmin, err := client.CallTool(adminCtx, "scandrix_delete_repo_comments", map[string]any{})
	if err != nil || resAdmin.IsError {
		t.Fatalf("expected admin role to succeed calling admin tool, got: %+v, err: %v", resAdmin, err)
	}

	// 3. Reviewer role can call read-only builtin tool
	resReviewer, err := client.CallTool(reviewerCtx, "scandrix_catalog_search", map[string]any{"query": "OWASP"})
	if err != nil || resReviewer.IsError {
		t.Fatalf("expected reviewer role to succeed calling read-only tool, got: %+v, err: %v", resReviewer, err)
	}

	// 4. Fail-closed: Anonymous / roleless caller MUST be rejected from restricted tool
	resAnon, err := client.CallTool(context.Background(), "scandrix_delete_repo_comments", map[string]any{})
	if err == nil && !resAnon.IsError {
		t.Fatalf("expected anonymous roleless caller to be rejected from restricted tool, got: %+v", resAnon)
	}
}

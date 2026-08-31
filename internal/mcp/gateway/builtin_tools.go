package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/rules"
)

// ToolHandler defines the execution function for an MCP tool.
type ToolHandler func(ctx context.Context, args map[string]any) (*ToolCallResult, error)

// GetBuiltinTools returns the native ScanDrix code analysis tools for MCP agents.
func GetBuiltinTools() ([]Tool, map[string]ToolHandler) {
	toolList := []Tool{
		{
			Name:         "scandrix_analyze_snippet",
			Description:  "Analyzes a source code snippet against the ScanDrix OWASP/CWE static security catalog",
			AllowedRoles: []AgentRole{RoleReviewer, RoleAuditor, RoleAdmin},
			IsReadOnly:   true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"language": map[string]string{"type": "string"},
					"code":     map[string]string{"type": "string"},
				},
				"required": []string{"code"},
			},
		},
		{
			Name:         "scandrix_catalog_search",
			Description:  "Searches the ScanDrix security rules catalog by keyword, severity, or CWE ID",
			AllowedRoles: []AgentRole{RoleReviewer, RoleAuditor, RoleAdmin},
			IsReadOnly:   true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]string{"type": "string"},
				},
			},
		},
		{
			Name:         "scandrix_validate_syntax",
			Description:  "Performs an AST dry-run compiler check to verify that a code suggestion has valid syntax",
			AllowedRoles: []AgentRole{RoleReviewer, RoleAuditor, RoleAdmin},
			IsReadOnly:   true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"language": map[string]string{"type": "string"},
					"code":     map[string]string{"type": "string"},
				},
				"required": []string{"code"},
			},
		},
		{
			Name:         "scandrix_lookup_osv_vulnerability",
			Description:  "Queries the Open Source Vulnerabilities (OSV) database for known security advisories, CVEs, and affected versions for a package dependency or commit",
			AllowedRoles: []AgentRole{RoleReviewer, RoleAuditor, RoleAdmin},
			IsReadOnly:   true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"package":   map[string]string{"type": "string"},
					"version":   map[string]string{"type": "string"},
					"ecosystem": map[string]string{"type": "string"},
					"commit":    map[string]string{"type": "string"},
				},
			},
		},
	}

	handlers := map[string]ToolHandler{
		"scandrix_analyze_snippet":          handleAnalyzeSnippet,
		"scandrix_catalog_search":           handleCatalogSearch,
		"scandrix_validate_syntax":          handleValidateSyntax,
		"scandrix_lookup_osv_vulnerability": handleLookupOSV,
	}

	return toolList, handlers
}

func handleAnalyzeSnippet(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	code, ok := args["code"].(string)
	if !ok || code == "" {
		return &ToolCallResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "missing 'code' parameter"}}}, nil
	}

	catalog := rules.DefaultCatalog()
	var matches []string

	for _, r := range catalog {
		if r.RegexRule == "" {
			continue
		}
		rx, err := regexp.Compile(r.RegexRule)
		if err != nil {
			continue
		}
		if rx.MatchString(code) {
			matches = append(matches, fmt.Sprintf("[%s] %s: %s", r.Severity, r.Name, r.Description))
		}
	}

	resultText := "No security flaws detected."
	if len(matches) > 0 {
		resultText = fmt.Sprintf("Security findings detected:\n- %s", strings.Join(matches, "\n- "))
	}

	return &ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: resultText}},
	}, nil
}

func handleCatalogSearch(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	query, _ := args["query"].(string)
	query = strings.ToLower(query)

	catalog := rules.DefaultCatalog()
	var matchingRules []rules.RuleSpec

	for _, r := range catalog {
		if query == "" || strings.Contains(strings.ToLower(r.Name), query) || strings.Contains(strings.ToLower(r.Category), query) {
			matchingRules = append(matchingRules, r)
		}
	}

	bytes, _ := json.MarshalIndent(matchingRules, "", "  ")
	return &ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: string(bytes)}},
	}, nil
}

func handleValidateSyntax(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	code, ok := args["code"].(string)
	if !ok || code == "" {
		return &ToolCallResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "missing 'code' parameter"}}}, nil
	}

	fset := token.NewFileSet()
	wrapped := fmt.Sprintf("package main\nfunc _test() {\n%s\n}", code)
	_, err := parser.ParseFile(fset, "test.go", wrapped, parser.AllErrors)

	if err != nil {
		return &ToolCallResult{
			IsError: true,
			Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Syntax error: %v", err)}},
		}, nil
	}

	return &ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: "Syntax valid: Code parsed successfully without syntax errors."}},
	}, nil
}

// OSVQueryRequest models the payload sent to https://api.osv.dev/v1/query.
type OSVQueryRequest struct {
	Commit  string      `json:"commit,omitempty"`
	Version string      `json:"version,omitempty"`
	Package *OSVPackage `json:"package,omitempty"`
}

type OSVPackage struct {
	Name      string `json:"name,omitempty"`
	Ecosystem string `json:"ecosystem,omitempty"`
	PURL      string `json:"purl,omitempty"`
}

type OSVQueryResponse struct {
	Vulns []OSVVulnerability `json:"vulns"`
}

type OSVVulnerability struct {
	ID        string        `json:"id"`
	Summary   string        `json:"summary"`
	Details   string        `json:"details"`
	Aliases   []string      `json:"aliases"`
	Modified  string        `json:"modified"`
	Published string        `json:"published"`
	Severity  []OSVSeverity `json:"severity"`
	Affected  []OSVAffected `json:"affected"`
}

type OSVSeverity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

type OSVAffected struct {
	Package  OSVPackage `json:"package"`
	Versions []string   `json:"versions"`
}

var osvHTTPClient = &http.Client{Timeout: 10 * time.Second}

func handleLookupOSV(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	pkgName, _ := args["package"].(string)
	version, _ := args["version"].(string)
	ecosystem, _ := args["ecosystem"].(string)
	commit, _ := args["commit"].(string)

	if pkgName == "" && commit == "" {
		return &ToolCallResult{
			IsError: true,
			Content: []ToolContent{{Type: "text", Text: "either 'package' (with optional 'version'/'ecosystem') or 'commit' parameter is required"}},
		}, nil
	}

	reqPayload := OSVQueryRequest{
		Commit:  commit,
		Version: version,
	}
	if pkgName != "" {
		reqPayload.Package = &OSVPackage{
			Name:      pkgName,
			Ecosystem: ecosystem,
		}
	}

	reqBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return &ToolCallResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("serialization error: %v", err)}}}, nil
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.osv.dev/v1/query", bytes.NewReader(reqBytes))
	if err != nil {
		return &ToolCallResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("request creation error: %v", err)}}}, nil
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "ScanDrix-MCP/1.2.0")

	resp, err := osvHTTPClient.Do(httpReq)
	if err != nil {
		return &ToolCallResult{
			IsError: true,
			Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("OSV query failed or timed out: %v", err)}},
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &ToolCallResult{
			IsError: true,
			Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("OSV service returned HTTP %d: %s", resp.StatusCode, string(body))}},
		}, nil
	}

	var osvResp OSVQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&osvResp); err != nil {
		return &ToolCallResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("failed parsing OSV response: %v", err)}}}, nil
	}

	if len(osvResp.Vulns) == 0 {
		targetDesc := pkgName
		if version != "" {
			targetDesc += "@" + version
		}
		if commit != "" {
			targetDesc = "commit " + commit
		}
		return &ToolCallResult{
			Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("No known vulnerabilities found in OSV database for %s.", targetDesc)}},
		}, nil
	}

	var summaryLines []string
	summaryLines = append(summaryLines, fmt.Sprintf("🛡️ Found %d security advisories in OSV database:\n", len(osvResp.Vulns)))

	for i, v := range osvResp.Vulns {
		aliases := ""
		if len(v.Aliases) > 0 {
			aliases = fmt.Sprintf(" (%s)", strings.Join(v.Aliases, ", "))
		}
		summaryLines = append(summaryLines, fmt.Sprintf("%d. [%s]%s: %s", i+1, v.ID, aliases, v.Summary))
		if len(v.Severity) > 0 {
			summaryLines = append(summaryLines, fmt.Sprintf("   • Severity: %s (%s)", v.Severity[0].Type, v.Severity[0].Score))
		}
	}

	outJSON, _ := json.MarshalIndent(osvResp.Vulns, "", "  ")
	return &ToolCallResult{
		Content: []ToolContent{
			{Type: "text", Text: strings.Join(summaryLines, "\n") + "\n\nRaw OSV Findings:\n" + string(outJSON)},
		},
	}, nil
}

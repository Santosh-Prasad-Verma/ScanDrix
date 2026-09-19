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
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/configcli"
	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/internal/cli/trace"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

// ToolHandler defines the execution function for an MCP tool.
type ToolHandler func(ctx context.Context, args map[string]any) (*ToolCallResult, error)

// GetBuiltinTools returns the native ScanDrix code analysis tools for MCP agents.
func GetBuiltinTools() ([]Tool, map[string]ToolHandler) {
	toolList := []Tool{
		{
			Name:         "scandrix_scan",
			Description:  "Deep scan a codebase directory or file for security vulnerabilities, OWASP violations, secrets, and code smells",
			AllowedRoles: []AgentRole{RoleReviewer, RoleAuditor, RoleAdmin},
			IsReadOnly:   true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":               map[string]string{"type": "string", "description": "Target directory or file path (default: .)"},
					"fast":               map[string]string{"type": "boolean", "description": "Fast AST/rule scan mode without deep AI delay"},
					"rules_only":         map[string]string{"type": "boolean", "description": "Check only custom & catalog invariant rules"},
					"severity_threshold": map[string]string{"type": "string", "description": "Filter findings by minimum severity: CRITICAL, HIGH, MEDIUM, LOW"},
				},
			},
		},
		{
			Name:         "scandrix_review",
			Description:  "Perform automated security and quality code review on local git diffs (staged changes, branch comparison, or specific commits)",
			AllowedRoles: []AgentRole{RoleReviewer, RoleAuditor, RoleAdmin},
			IsReadOnly:   true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"staged": map[string]string{"type": "boolean", "description": "Review staged git changes (git diff --cached)"},
					"branch": map[string]string{"type": "string", "description": "Target branch to diff against (e.g. main)"},
					"commit": map[string]string{"type": "string", "description": "Specific commit SHA to review"},
					"focus":  map[string]string{"type": "string", "description": "Steer review focus area (e.g. 'auth and sessions', 'database')"},
					"fast":   map[string]string{"type": "boolean", "description": "Fast review mode"},
				},
			},
		},
		{
			Name:         "scandrix_apply_fix",
			Description:  "Applies an AST remediation patch or unified diff to a source file",
			AllowedRoles: []AgentRole{RoleMutator, RoleAdmin},
			IsReadOnly:   false,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path":  map[string]string{"type": "string", "description": "Target file path"},
					"patch":      map[string]string{"type": "string", "description": "Unified diff or replacement patch"},
					"start_line": map[string]string{"type": "integer", "description": "Starting line number (optional)"},
					"end_line":   map[string]string{"type": "integer", "description": "Ending line number (optional)"},
				},
				"required": []string{"file_path", "patch"},
			},
		},
		{
			Name:         "scandrix_trace_recall",
			Description:  "Recalls prior developer decisions, prompts, and architectural rationale for specified files from the trace memory store",
			AllowedRoles: []AgentRole{RoleReviewer, RoleAuditor, RoleAdmin},
			IsReadOnly:   true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_paths": map[string]any{
						"type":        "array",
						"items":       map[string]string{"type": "string"},
						"description": "List of file paths to recall history for",
					},
					"limit": map[string]string{"type": "integer", "description": "Maximum number of decisions to recall (default: 10)"},
				},
			},
		},
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
		"scandrix_scan":                     handleScanTool,
		"scandrix_review":                   handleReviewTool,
		"scandrix_apply_fix":                handleApplyFixTool,
		"scandrix_trace_recall":             handleTraceRecallTool,
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

	// 1. If it already has a package declaration, parse directly
	if strings.Contains(code, "package ") {
		if _, err := parser.ParseFile(fset, "test.go", code, parser.AllErrors); err == nil {
			return &ToolCallResult{
				Content: []ToolContent{{Type: "text", Text: "Syntax valid: Code parsed successfully without syntax errors."}},
			}, nil
		}
	}

	// 2. Try parsing as top-level declarations (e.g. func, type, var)
	wrappedTopLevel := fmt.Sprintf("package main\n%s\n", code)
	if _, err := parser.ParseFile(fset, "test.go", wrappedTopLevel, parser.AllErrors); err == nil {
		return &ToolCallResult{
			Content: []ToolContent{{Type: "text", Text: "Syntax valid: Code parsed successfully without syntax errors."}},
		}, nil
	}

	// 3. Try parsing inside a function body (statements, expressions)
	wrappedFunc := fmt.Sprintf("package main\nfunc _test() {\n%s\n}", code)
	if _, err := parser.ParseFile(fset, "test.go", wrappedFunc, parser.AllErrors); err == nil {
		return &ToolCallResult{
			Content: []ToolContent{{Type: "text", Text: "Syntax valid: Code parsed successfully without syntax errors."}},
		}, nil
	}

	// If all fail, return the error from statement wrap or direct parse
	_, err := parser.ParseFile(fset, "test.go", wrappedFunc, parser.AllErrors)
	return &ToolCallResult{
		IsError: true,
		Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Syntax error: %v", err)}},
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

func resolveAuthForMCP(ctx context.Context, targetDir string) (token string, baseURL string, offline bool) {
	if t, ok := ctx.Value(ContextKeyToken).(string); ok && t != "" {
		token = t
	}
	cfg := configcli.Load(targetDir)
	if cfg != nil {
		if token == "" {
			if cfg.AccessToken != "" {
				token = cfg.AccessToken
			} else if cfg.APIKey != "" {
				token = cfg.APIKey
			}
		}
		if cfg.ServerURL != "" {
			baseURL = cfg.ServerURL
		}
	}
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	offline = (token == "")
	return token, baseURL, offline
}

func handleScanTool(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	targetDir := "."
	if p, ok := args["path"].(string); ok && p != "" {
		targetDir = p
	}
	fast, _ := args["fast"].(bool)
	rulesOnly, _ := args["rules_only"].(bool)
	sevStr, _ := args["severity_threshold"].(string)

	threshold := models.SeverityMedium
	if sevStr != "" {
		threshold = models.FindingSeverity(strings.ToUpper(sevStr))
	}

	token, baseURL, defaultOffline := resolveAuthForMCP(ctx, targetDir)
	isOffline := defaultOffline
	if off, ok := args["offline"].(bool); ok && off {
		isOffline = true
	}

	runner := engine.NewCLIRunner()
	opts := engine.CLIOptions{
		TargetDirectory:   targetDir,
		Format:            engine.FormatJSON,
		AgentMode:         true,
		SeverityThreshold: threshold,
		Fast:              fast,
		RulesOnly:         rulesOnly,
		Offline:           isOffline,
		AccessToken:       token,
		APIBaseURL:        baseURL,
	}

	res, err := runner.RunScan(ctx, targetDir, opts)
	if err != nil {
		return &ToolCallResult{
			IsError: true,
			Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("scan failed: %v", err)}},
		}, nil
	}

	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return &ToolCallResult{IsError: true, Content: []ToolContent{{Type: "text", Text: err.Error()}}}, nil
	}

	return &ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: string(data)}},
	}, nil
}

func handleReviewTool(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	staged, _ := args["staged"].(bool)
	branch, _ := args["branch"].(string)
	commit, _ := args["commit"].(string)
	focus, _ := args["focus"].(string)
	fast, _ := args["fast"].(bool)

	gitRefRegex := regexp.MustCompile(`^[a-zA-Z0-9_./-]+$`)
	if branch != "" {
		branch = strings.TrimSpace(branch)
		if strings.HasPrefix(branch, "-") || !gitRefRegex.MatchString(branch) {
			return nil, fmt.Errorf("invalid or dangerous git branch: %q", branch)
		}
	}
	if commit != "" {
		commit = strings.TrimSpace(commit)
		if strings.HasPrefix(commit, "-") || !gitRefRegex.MatchString(commit) {
			return nil, fmt.Errorf("invalid or dangerous git commit: %q", commit)
		}
	}

	var gitArgs []string
	if staged {
		gitArgs = []string{"diff", "--cached", "--"}
	} else if branch != "" {
		gitArgs = []string{"diff", fmt.Sprintf("origin/%s...HEAD", branch), "--"}
	} else if commit != "" {
		gitArgs = []string{"show", commit, "--"}
	} else {
		gitArgs = []string{"diff", "HEAD", "--"}
	}

	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		cmd = exec.CommandContext(ctx, "git", "diff")
		out, _ = cmd.Output()
	}

	rawDiff := string(out)
	if strings.TrimSpace(rawDiff) == "" {
		return &ToolCallResult{
			Content: []ToolContent{{Type: "text", Text: `{"status":"passed","files_reviewed":0,"total_findings":0,"findings":[],"message":"clean working tree, no diff detected"}`}},
		}, nil
	}

	token, baseURL, defaultOffline := resolveAuthForMCP(ctx, ".")
	isOffline := defaultOffline
	if off, ok := args["offline"].(bool); ok && off {
		isOffline = true
	}

	runner := engine.NewCLIRunner()
	opts := engine.CLIOptions{
		TargetDirectory:   ".",
		Format:            engine.FormatJSON,
		AgentMode:         true,
		SeverityThreshold: models.SeverityMedium,
		Focus:             focus,
		Fast:              fast,
		Offline:           isOffline,
		AccessToken:       token,
		APIBaseURL:        baseURL,
	}

	res, err := runner.RunReview(ctx, rawDiff, opts)
	if err != nil {
		return &ToolCallResult{
			IsError: true,
			Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("review failed: %v", err)}},
		}, nil
	}

	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return &ToolCallResult{IsError: true, Content: []ToolContent{{Type: "text", Text: err.Error()}}}, nil
	}

	return &ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: string(data)}},
	}, nil
}

func handleApplyFixTool(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	filePath, _ := args["file_path"].(string)
	patch, _ := args["patch"].(string)
	startLine, _ := args["start_line"].(float64)
	endLine, _ := args["end_line"].(float64)

	if filePath == "" || patch == "" {
		return &ToolCallResult{
			IsError: true,
			Content: []ToolContent{{Type: "text", Text: "missing 'file_path' or 'patch' parameter"}},
		}, nil
	}

	finding := models.CodeFinding{
		FilePath:      filePath,
		SuggestedDiff: patch,
		StartLine:     int(startLine),
		EndLine:       int(endLine),
	}

	msg, err := engine.ApplyFindingFix(".", finding)
	if err != nil {
		return &ToolCallResult{
			IsError: true,
			Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("failed applying fix: %v", err)}},
		}, nil
	}

	return &ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: msg}},
	}, nil
}

func handleTraceRecallTool(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	var paths []string
	if rawPaths, ok := args["file_paths"].([]any); ok {
		for _, p := range rawPaths {
			if s, ok := p.(string); ok && s != "" {
				paths = append(paths, s)
			}
		}
	} else if p, ok := args["path"].(string); ok && p != "" {
		paths = append(paths, p)
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	store := trace.NewTraceStore()
	decisions, err := store.Recall(paths, limit)
	if err != nil {
		return &ToolCallResult{
			IsError: true,
			Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("trace recall failed: %v", err)}},
		}, nil
	}

	data, err := json.MarshalIndent(decisions, "", "  ")
	if err != nil {
		return &ToolCallResult{IsError: true, Content: []ToolContent{{Type: "text", Text: err.Error()}}}, nil
	}

	return &ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: string(data)}},
	}, nil
}


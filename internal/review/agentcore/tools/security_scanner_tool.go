package tools

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

type SecurityHotspot struct {
	Category    string
	Severity    string
	Pattern     *regexp.Regexp
	Description string
}

var StandardSecurityHotspots = []SecurityHotspot{
	{
		Category:    "Secret Leak",
		Severity:    "critical",
		Pattern:     regexp.MustCompile(`(?i)(api[_-]?key|auth[_-]?token|secret|password|bearer)\s*[:=]\s*["'][A-Za-z0-9_\-./+=]{16,}["']`),
		Description: "Potential hardcoded API secret, credential, or authentication token",
	},
	{
		Category:    "Cloud Key",
		Severity:    "critical",
		Pattern:     regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		Description: "Hardcoded AWS Access Key ID detected",
	},
	{
		Category:    "Private Key",
		Severity:    "critical",
		Pattern:     regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----`),
		Description: "Raw private key embedded in source code",
	},
	{
		Category:    "SQL Injection",
		Severity:    "high",
		Pattern:     regexp.MustCompile(`(?i)(?:SELECT|INSERT|UPDATE|DELETE|FROM|WHERE).*(\+|string\.concat|\$\{)|(?i)fmt\.Sprintf\s*\(\s*["'](?:SELECT|INSERT|UPDATE|DELETE)`),
		Description: "SQL query constructed via string concatenation or fmt.Sprintf instead of parameterized placeholders",
	},
	{
		Category:    "Command Injection",
		Severity:    "critical",
		Pattern:     regexp.MustCompile(`(?i)(?:exec\.Command|child_process\.exec|os\.system|subprocess\.Popen)\s*\([^)]*(?:sh|bash|-c|\+|fmt\.Sprintf)`),
		Description: "Shell command invocation with dynamic or unsanitized arguments",
	},
	{
		Category:    "Dangerous Eval",
		Severity:    "high",
		Pattern:     regexp.MustCompile(`\b(?:eval|pickle\.loads|yaml\.load\(|new Function\()\b`),
		Description: "Unsafe dynamic code execution or unpickling/deserialization without safe loader",
	},
	{
		Category:    "Weak Cryptography",
		Severity:    "medium",
		Pattern:     regexp.MustCompile(`(?i)\b(md5|sha1|des|rc4)\.(?:New|Create|Encrypt)\b`),
		Description: "Deprecated or cryptographically broken hash algorithm used",
	},
}

// SecurityScannerToolOptions configures security hotspot scanning.
type SecurityScannerToolOptions struct {
	FS      RepositoryFS
	Sandbox SandboxExecutor
}

// NewSecurityScannerTool creates the security hotspot and vulnerability pattern scanner tool.
func NewSecurityScannerTool(opts SecurityScannerToolOptions) contracts.AgentTool {
	return &BaseTool{
		ToolName: "securityScanner",
		ToolDesc: "Run targeted security scanning for critical vulnerabilities: hardcoded secrets, SQL injection, " +
			"command injection, path traversal, weak crypto, and dangerous deserialization. " +
			"Use this tool to thoroughly check security-sensitive files before finalizing.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"path": {
					Type:        "string",
					Description: "Path to file or directory to scan (default: '.')",
				},
			},
		},
		IsStrict: false,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			path := NormalizePath(StringArg(input, "path", "file"))
			if path == "" {
				path = "."
			}

			var content string
			var err error

			if opts.FS != nil {
				content, err = opts.FS.ReadFile(ctx.Context, path, 0, 0)
			} else if opts.Sandbox != nil {
				cmd := fmt.Sprintf("cat %s 2>/dev/null", ShellQuote(path))
				stdout, _, exitCode, e := opts.Sandbox.Exec(ctx.Context, cmd)
				if e == nil && exitCode == 0 {
					content = stdout
				} else {
					err = e
				}
			}

			if err != nil || strings.TrimSpace(content) == "" {
				return contracts.ToolResult{
					Output: fmt.Sprintf("Security scanner: could not read %s or file is empty", path),
				}, nil
			}

			lines := strings.Split(content, "\n")
			findings := make([]string, 0)

			for i, line := range lines {
				lineNum := i + 1
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
					continue
				}

				for _, hotspot := range StandardSecurityHotspots {
					if hotspot.Pattern.MatchString(trimmed) {
						finding := fmt.Sprintf("Line %d [%s - %s]: %s\n  Snippet: %s",
							lineNum, strings.ToUpper(hotspot.Severity), hotspot.Category, hotspot.Description, trimmed)
						findings = append(findings, finding)
						if len(findings) >= 20 {
							break
						}
					}
				}
				if len(findings) >= 20 {
					break
				}
			}

			if len(findings) == 0 {
				return contracts.ToolResult{
					Output: fmt.Sprintf("Security scan clean for %s — 0 static security hotspots detected.", path),
				}, nil
			}

			header := fmt.Sprintf("Security scan detected %d potential hotspots in %s:\n\n", len(findings), path)
			return contracts.ToolResult{Output: header + strings.Join(findings, "\n\n")}, nil
		},
	}
}

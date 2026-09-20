// Package codereview provides default review parameters, category descriptions, severity thresholds, and configuration validation for ScanDrix.
package codereview

import "strings"

// V2CategoryDescriptions holds detailed guidance for review categories.
var V2CategoryDescriptions = map[string]string{
	"bug": strings.Join([]string{
		"- Execution breaks: Code throws unhandled exceptions",
		"- Wrong results: Output doesn't match expected behavior",
		"- Resource leaks: Unclosed files, connections, memory accumulation",
		"- State corruption: Invalid object/data states",
		"- Logic errors: Control flow produces incorrect outcomes",
		"- Race conditions: Concurrent access causes inconsistent state or duplicates",
		"- Incorrect measurements: Metrics/timings that don't reflect actual operations",
		"- Invariant violations: Broken constraints (size limits, uniqueness, etc.)",
		"- Async timing bugs: Variables captured incorrectly in async closures",
		"- Dead computation: Values computed but never used",
		"- Unbounded growth: Collections that grow indefinitely within loops",
	}, "\n"),

	"performance": strings.Join([]string{
		"- Algorithm complexity: O(n²) when O(n) is possible",
		"- Redundant operations: Duplicate calculations, unnecessary loops",
		"- Memory waste: Large allocations or leaks over time",
		"- Blocking operations: Synchronous I/O in critical paths",
		"- Database inefficiency: N+1 queries, missing indexes, full scans",
		"- Cache misses: Not leveraging available caching mechanisms",
	}, "\n"),

	"security": strings.Join([]string{
		"- Injection vulnerabilities: SQL/NoSQL/command/LDAP injection",
		"- AuthZ/AuthN flaws: Missing checks, privilege escalation",
		"- Data exposure: Sensitive data in logs, responses, or errors",
		"- Crypto issues: Weak algorithms, hardcoded keys, improper validation",
		"- Input validation gaps: Missing sanitization or bounds checks",
		"- Session management: Predictable tokens or missing expiration",
		"- Timing attacks: Non-constant-time comparisons of secrets or credentials",
		"- Insecure fallback values: Using empty strings or weak defaults when env vars are missing",
		"- SSRF: Unvalidated user URLs in network operations",
	}, "\n"),
}

// V2SeverityFlags defines severity classification criteria.
var V2SeverityFlags = map[string]string{
	"critical": strings.Join([]string{
		"Application crash/downtime",
		"Data loss/corruption",
		"Security breach (unauthorized access/data exfiltration)",
		"Critical operation failure (auth/payment/authorization)",
		"Direct financial loss operations",
		"Memory leaks that inevitably crash production",
	}, "\n"),
	"high": strings.Join([]string{
		"Important functionality broken",
		"Memory leaks that cause eventual crash",
		"Performance degradation affecting UX under normal load",
		"Security issues with indirect exploitation paths",
		"Financial calculation errors affecting revenue",
	}, "\n"),
	"medium": strings.Join([]string{
		"Partially broken functionality",
		"Performance issues in specific scenarios",
		"Security weaknesses requiring specific conditions",
		"Incorrect but recoverable data",
		"Non-critical business logic errors with workarounds",
	}, "\n"),
	"low": strings.Join([]string{
		"Minor performance overhead",
		"Low-risk security improvements",
		"Incorrect metrics/logs",
		"Rarely affecting few users",
		"Edge-case issues",
	}, "\n"),
}

// V2LevelText defines review level thresholds.
var V2LevelText = map[string]string{
	"critical": "The code WILL crash, lose/corrupt data, or open a severe security breach in production. Immediate fix required before merge.",
	"issue":    "The code produces WRONG results or fails to perform its intended function in at least one scenario, but does not cause catastrophic failure.",
	"warning":  "The code produces CORRECT results and performs its intended function in ALL scenarios but is suboptimal in style, performance, or maintainability.",
}

// ScanDrixConfigFile represents the parsed .scandrix/config.yaml configuration file.
type ScanDrixConfigFile struct {
	Version                     string                 `json:"version" yaml:"version"`
	AutomaticPRApprovalActive   bool                   `json:"automaticPRApprovalActive" yaml:"automaticPRApprovalActive"`
	ReviewIgnoredBranches       []string               `json:"reviewIgnoredBranches" yaml:"reviewIgnoredBranches"`
	ReviewIgnoredFiles          []string               `json:"reviewIgnoredFiles" yaml:"reviewIgnoredFiles"`
	MaxReviewLines              int                    `json:"maxReviewLines" yaml:"maxReviewLines"`
	ReviewCadence               string                 `json:"reviewCadence" yaml:"reviewCadence"`
	Language                    string                 `json:"language" yaml:"language"`
	V2PromptOverrides           map[string]any         `json:"v2PromptOverrides,omitempty" yaml:"v2PromptOverrides,omitempty"`
}

// GetDefaultScanDrixConfigFile returns the default configuration.
func GetDefaultScanDrixConfigFile() ScanDrixConfigFile {
	return ScanDrixConfigFile{
		Version:                   "1.2",
		AutomaticPRApprovalActive: false,
		ReviewIgnoredBranches:     []string{"main", "master", "release/*"},
		ReviewIgnoredFiles:        []string{"*.lock", "vendor/**", "node_modules/**", "dist/**", "build/**"},
		MaxReviewLines:            1000,
		ReviewCadence:             "automatic",
		Language:                  "en-US",
	}
}

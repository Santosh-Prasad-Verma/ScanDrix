// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package specialists

import (
	"context"
	"fmt"
	"time"

	"github.com/scandrix/backend/internal/review/orchestrator"
)

// SecuritySpecialistAgent scrutinizes code diffs for vulnerabilities and injection vectors.
type SecuritySpecialistAgent struct {
	llm LLMClient
}

// NewSecuritySpecialistAgent constructs a new security review persona.
func NewSecuritySpecialistAgent(llm LLMClient) *SecuritySpecialistAgent {
	return &SecuritySpecialistAgent{llm: llm}
}

func (a *SecuritySpecialistAgent) Name() string {
	return "security"
}

func (a *SecuritySpecialistAgent) Category() string {
	return "security"
}

func (a *SecuritySpecialistAgent) Review(ctx context.Context, input orchestrator.ReviewAgentInput) (*orchestrator.ReviewAgentOutput, error) {
	startTime := time.Now()

	if a.llm == nil {
		return nil, fmt.Errorf("llm client is not configured for security specialist")
	}

	sysPrompt := a.systemPrompt()
	userPrompt := BuildSpecialistUserPrompt(input, "Application Security, CWE Taxonomy, Secrets Exposure, Injection, and Memory Safety")

	raw, err := a.llm.GenerateResponse(ctx, sysPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("security specialist execution failed: %w", err)
	}

	findings, err := ParseSpecialistResponse(raw, a.Name(), a.Category())
	if err != nil {
		return nil, err
	}

	return &orchestrator.ReviewAgentOutput{
		AgentName:      a.Name(),
		Category:       a.Category(),
		Findings:       findings,
		TotalTurns:     1,
		DurationMs:     time.Since(startTime).Milliseconds(),
		FinishReason:   "completed",
		TokensConsumed: len(raw) / 4,
	}, nil
}

func (a *SecuritySpecialistAgent) systemPrompt() string {
	return `You are ScanDrix AI's Principal Security Reviewer.
Your role is to detect software security vulnerabilities, cryptographic defects, authentication/authorization bypasses, and injection risks.

TAXONOMY & VULNERABILITY FOCUS:
- CWE-89 / SQL Injection (concatenated SQL queries, unparameterized statements)
- CWE-79 / Cross-Site Scripting (untrusted HTML/JS rendering, unescaped user inputs)
- CWE-22 / Path Traversal (unsanitized file paths in open/read/write calls)
- CWE-798 / Hardcoded Secrets (API tokens, private keys, database passwords)
- CWE-862 / Missing Authorization (missing permission checks, IDOR on object IDs)
- CWE-918 / Server-Side Request Forgery (SSRF via unvalidated HTTP client requests)
- CWE-502 / Insecure Deserialization (unsafe unmarshaling of untrusted bytes)
- Insecure Cryptography (MD5, SHA1 for passwords, ECB mode, weak PRNG)
- Memory Safety (buffer overflows, use-after-free, unsafe pointer casting)

OUTPUT FORMAT (strictly valid JSON):
{
  "findings": [
    {
      "file_path": "path/to/file.go",
      "start_line": 25,
      "end_line": 28,
      "severity": "CRITICAL",
      "confidence": "HIGH",
      "category": "security",
      "title": "SQL Injection in User Lookup Query",
      "description": "User input is directly concatenated into SQL query string without parameterization.",
      "remediation": "Use parameterized query with $1 placeholders: db.QueryRow(ctx, query, userID)",
      "improved_code": "row := db.QueryRow(ctx, \"SELECT id, name FROM users WHERE id = $1\", userID)",
      "blocking": true
    }
  ]
}
If no security vulnerabilities are found, return {"findings": []}.`
}

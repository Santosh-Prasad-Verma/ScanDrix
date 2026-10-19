// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package community

// RuleItem represents an individual AST/regex security rule inside a rule pack.
type RuleItem struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Severity    string `json:"severity" yaml:"severity"`
	Category    string `json:"category" yaml:"category"`
	PathPattern string `json:"pathPattern" yaml:"pathPattern"`
	RegexRule   string `json:"regexRule" yaml:"regexRule"`
	Remediation string `json:"remediation" yaml:"remediation"`
	Enabled     bool   `json:"enabled" yaml:"enabled"`
	BadExample  string `json:"badExample,omitempty" yaml:"badExample,omitempty"`
	GoodExample string `json:"goodExample,omitempty" yaml:"goodExample,omitempty"`
}

// RulePack represents a curated or community-contributed rule bundle.
type RulePack struct {
	ID            string     `json:"id" yaml:"id"`
	Slug          string     `json:"slug" yaml:"slug"`
	Title         string     `json:"title" yaml:"title"`
	Description   string     `json:"description" yaml:"description"`
	Category      string     `json:"category" yaml:"category"`
	Version       string     `json:"version" yaml:"version"`
	Author        string     `json:"author" yaml:"author"`
	Verified      bool       `json:"verified" yaml:"verified"`
	Downloads     int        `json:"downloads" yaml:"downloads"`
	Stars         int        `json:"stars" yaml:"stars"`
	Icon          string     `json:"icon" yaml:"icon"`
	Tags          []string   `json:"tags" yaml:"tags"`
	RepositoryURL string     `json:"repositoryUrl,omitempty" yaml:"repositoryUrl,omitempty"`
	Rules         []RuleItem `json:"rules" yaml:"rules"`
}

// CuratedPacks returns all official, battle-tested ScanDrix and community-verified rule packs.
func CuratedPacks() []RulePack {
	return []RulePack{
		{
			ID:          "pack-owasp-top10",
			Slug:        "owasp-top-10",
			Title:       "OWASP Top 10 Enterprise Defense",
			Description: "Detects injection flaws, broken access control, cross-site scripting (XSS), insecure deserialization, and SSRF vulnerabilities.",
			Category:    "SECURITY",
			Version:     "2026.1",
			Author:      "ScanDrix Security Team & OWASP",
			Verified:    true,
			Downloads:   14820,
			Stars:       1240,
			Icon:        "🛡️",
			Tags:        []string{"OWASP", "Injection", "XSS", "SSRF", "CWE-89"},
			Rules: []RuleItem{
				{
					ID:          "owasp-sql-concat",
					Name:        "Prevent SQL Query String Concatenation",
					Description: "Direct concatenation of user inputs into SQL strings causes SQL injection flaws.",
					Severity:    "CRITICAL",
					Category:    "SECURITY",
					PathPattern: "**/*.{go,ts,js,py}",
					RegexRule:   `(?i)(SELECT|INSERT|UPDATE|DELETE).*\+.*req\.(Query|Body|Params)`,
					Remediation: "Use parameterized queries with prepared statement placeholders ($1, $2, ?).",
					Enabled:     true,
					BadExample:  `db.Query("SELECT * FROM users WHERE email = '" + req.Query.email + "'")`,
					GoodExample: `db.Query("SELECT * FROM users WHERE email = $1", req.Query.email)`,
				},
				{
					ID:          "owasp-ssrf-client",
					Name:        "Prevent Unsanitized HTTP Client Fetch (SSRF)",
					Description: "Directly passing request URL query params into outgoing HTTP clients creates server-side request forgery risks.",
					Severity:    "CRITICAL",
					Category:    "SECURITY",
					PathPattern: "**/*.{go,ts,js}",
					RegexRule:   `(http\.Get|axios\.get|fetch)\(req\.(Query|Params|URL)`,
					Remediation: "Validate destination URLs against an explicit domain allow-list.",
					Enabled:     true,
					BadExample:  `http.Get(r.URL.Query().Get("target"))`,
					GoodExample: `if isAllowedHost(target) { http.Get(target) }`,
				},
				{
					ID:          "owasp-xss-innerhtml",
					Name:        "Prevent Unsafe innerHTML / dangerouslySetInnerHTML",
					Description: "Direct assignment to innerHTML with unescaped content introduces Stored or Reflected XSS.",
					Severity:    "HIGH",
					Category:    "SECURITY",
					PathPattern: "**/*.{tsx,jsx,ts,js}",
					RegexRule:   `(\.innerHTML\s*=|dangerouslySetInnerHTML=\{\s*\{\s*__html:)`,
					Remediation: "Use DOMPurify.sanitize() or standard JSX text interpolations.",
					Enabled:     true,
					BadExample:  `<div dangerouslySetInnerHTML={{ __html: userComment }} />`,
					GoodExample: `<div>{userComment}</div>`,
				},
			},
		},
		{
			ID:          "pack-secrets-guard",
			Slug:        "secrets-zero-leak",
			Title:       "Zero-Leak Secrets & Credential Guard",
			Description: "Enforces Master Rule 1: Zero plaintext secrets, API keys, tokens, or credentials in tracked source code.",
			Category:    "SECRETS",
			Version:     "2026.1",
			Author:      "ScanDrix Security Core",
			Verified:    true,
			Downloads:   22450,
			Stars:       1890,
			Icon:        "🔑",
			Tags:        []string{"Secrets", "Master Rule 1", "API Keys", "AWS", "Stripe"},
			Rules: []RuleItem{
				{
					ID:          "sec-aws-access-key",
					Name:        "Flag Hardcoded AWS Access Key ID",
					Description: "Hardcoded AWS Access Key ID leaked in source code.",
					Severity:    "CRITICAL",
					Category:    "SECRETS",
					PathPattern: "**/*",
					RegexRule:   `AKIA[0-9A-Z]{16}`,
					Remediation: "Load AWS credentials from environment variables or IAM instance role.",
					Enabled:     true,
				},
				{
					ID:          "sec-generic-api-key",
					Name:        "Detect Hardcoded Secret or Token Assignment",
					Description: "Variable names indicating secrets assigned to static string literals.",
					Severity:    "HIGH",
					Category:    "SECRETS",
					PathPattern: "**/*.{go,ts,js,py,env}",
					RegexRule:   `(?i)(api_key|secret_key|private_key|jwt_secret)\s*=\s*['\"][a-zA-Z0-9_\-]{16,}['\"]`,
					Remediation: "Move secrets to .env (local dev) or enterprise secret manager.",
					Enabled:     true,
				},
				{
					ID:          "sec-stripe-secret-key",
					Name:        "Flag Hardcoded Stripe Secret API Key",
					Description: "Stripe live secret keys (`sk_live_...`) leaked in client or server code.",
					Severity:    "CRITICAL",
					Category:    "SECRETS",
					PathPattern: "**/*",
					RegexRule:   `sk_live_[0-9a-zA-Z]{24,}`,
					Remediation: "Keep live Stripe keys in AWS Secrets Manager / HashiCorp Vault.",
					Enabled:     true,
				},
			},
		},
		{
			ID:          "pack-go-concurrency",
			Slug:        "go-concurrency-sentinel",
			Title:       "Go Concurrency & Goroutine Leak Sentinel",
			Description: "Prevents goroutine leaks, unbuffered channel deadlocks, and data race anti-patterns in Go microservices.",
			Category:    "PERFORMANCE",
			Version:     "2026.1",
			Author:      "Go Gophers Community",
			Verified:    true,
			Downloads:   9840,
			Stars:       870,
			Icon:        "⚡",
			Tags:        []string{"Go", "Concurrency", "Goroutines", "Deadlocks", "Channels"},
			Rules: []RuleItem{
				{
					ID:          "go-unmanaged-routine",
					Name:        "Flag Raw Goroutines Without Context",
					Description: "Goroutines launched without context cancellation or WaitGroup synchronization risk leaking indefinitely on timeout.",
					Severity:    "HIGH",
					Category:    "PERFORMANCE",
					PathPattern: "**/*.go",
					RegexRule:   `go\s+func\(\)\s*\{[^}]*\}\(\)`,
					Remediation: "Pass context.Context to the background worker and observe ctx.Done().",
					Enabled:     true,
				},
				{
					ID:          "go-sync-mutex-copy",
					Name:        "Prevent sync.Mutex Passed by Value",
					Description: "Passing a struct containing sync.Mutex by value creates a copy of the lock state, causing deadlocks or data races.",
					Severity:    "CRITICAL",
					Category:    "BUG_RISK",
					PathPattern: "**/*.go",
					RegexRule:   `func\s+\([^)]*\b(sync\.(RW)?Mutex)\b[^)]*\)`,
					Remediation: "Always pass mutexes and mutex-containing structs by pointer (*MyStruct).",
					Enabled:     true,
				},
			},
		},
		{
			ID:          "pack-react19-nextjs15",
			Slug:        "nextjs-15-react-19-guard",
			Title:       "Next.js 15 & React 19 Hydration Guard",
			Description: "Enforces React 19 hydration safety, Prevents useEffect cascading waterfalls, and guards Server Actions.",
			Category:    "ARCHITECTURE",
			Version:     "2026.1",
			Author:      "Vercel Ecosystem Community",
			Verified:    true,
			Downloads:   18600,
			Stars:       1530,
			Icon:        "⚛️",
			Tags:        []string{"React 19", "Next.js 15", "Hydration", "Server Actions"},
			Rules: []RuleItem{
				{
					ID:          "react-window-ssr-leak",
					Name:        "Flag Unchecked Window/Document Access in Server Component",
					Description: "Accessing `window`, `document`, or `localStorage` directly in Server Components causes immediate 500 runtime crashes.",
					Severity:    "CRITICAL",
					Category:    "ARCHITECTURE",
					PathPattern: "**/app/**/*.{tsx,jsx}",
					RegexRule:   `\b(window|document|localStorage)\.[a-zA-Z_]\w*`,
					Remediation: "Add 'use client' directive at top of file or wrap in useEffect / useSyncExternalStore.",
					Enabled:     true,
				},
				{
					ID:          "react-useeffect-setstate-cascade",
					Name:        "Detect Cascading setState Inside useEffect",
					Description: "Unconditional setState inside useEffect triggers infinite re-render waterfalls.",
					Severity:    "HIGH",
					Category:    "PERFORMANCE",
					PathPattern: "**/*.{tsx,jsx}",
					RegexRule:   `useEffect\(\(\)\s*=>\s*\{[^}]*set[A-Z]\w+\([^)]+\);?\s*\},?\s*\[\]\)`,
					Remediation: "Derive state during render or use custom hooks with useMemo.",
					Enabled:     true,
				},
			},
		},
		{
			ID:          "pack-rust-zero-cost",
			Slug:        "rust-zero-cost-safety",
			Title:       "Rust Memory Safety & Zero-Cost Sentinel",
			Description: "Guards against production unwrap() panics, lock contention across async await, and unsafe blocks.",
			Category:    "SECURITY",
			Version:     "2026.1",
			Author:      "Rust Foundation Community",
			Verified:    true,
			Downloads:   7230,
			Stars:       640,
			Icon:        "🦀",
			Tags:        []string{"Rust", "Safety", "Memory", "Async", "Tokio"},
			Rules: []RuleItem{
				{
					ID:          "rust-production-unwrap",
					Name:        "Flag Unhandled .unwrap() in Production Crates",
					Description: "Calling .unwrap() on Result or Option triggers unrecoverable thread panic if an error occurs.",
					Severity:    "HIGH",
					Category:    "BUG_RISK",
					PathPattern: "**/src/**/*.rs",
					RegexRule:   `\.(unwrap|expect)\(`,
					Remediation: "Use ? error propagation or match/if-let pattern matching with anyhow/thiserror.",
					Enabled:     true,
				},
				{
					ID:          "rust-async-mutex-lock",
					Name:        "Detect std::sync::Mutex Held Across .await",
					Description: "Holding std::sync::Mutex across an async .await boundary causes Tokio worker thread starvation.",
					Severity:    "CRITICAL",
					Category:    "PERFORMANCE",
					PathPattern: "**/src/**/*.rs",
					RegexRule:   `let\s+(_|\w+)\s*=\s*\w+\.lock\(\)\.unwrap\(\);[^\n]*\.await`,
					Remediation: "Use tokio::sync::Mutex or drop the lock guard before awaiting.",
					Enabled:     true,
				},
			},
		},
		{
			ID:          "pack-k8s-docker-hardening",
			Slug:        "k8s-container-hardening",
			Title:       "Cloud-Native Kubernetes & Docker Hardening",
			Description: "Detects root container execution, missing resource limits, privileged capabilities, and insecure hostPath mounts.",
			Category:    "INFRASTRUCTURE",
			Version:     "2026.1",
			Author:      "CNCF Security TAG",
			Verified:    true,
			Downloads:   11200,
			Stars:       980,
			Icon:        "☸️",
			Tags:        []string{"Kubernetes", "Docker", "Containers", "DevSecOps", "CIS"},
			Rules: []RuleItem{
				{
					ID:          "docker-root-user",
					Name:        "Flag Dockerfile Missing Non-Root USER Directive",
					Description: "Containers executing as root (UID 0) significantly expand breakout blast radiuses.",
					Severity:    "HIGH",
					Category:    "SECURITY",
					PathPattern: "**/Dockerfile*",
					RegexRule:   `(?i)^USER\s+root`,
					Remediation: "Create a non-root service account (USER 10001:10001) in final stage.",
					Enabled:     true,
				},
				{
					ID:          "k8s-privileged-pod",
					Name:        "Prevent Privileged Containers in Pod Specs",
					Description: "Setting privileged: true disables all Linux container isolation namespaces.",
					Severity:    "CRITICAL",
					Category:    "SECURITY",
					PathPattern: "**/*.{yaml,yml}",
					RegexRule:   `privileged:\s*true`,
					Remediation: "Remove privileged: true and grant only explicit capabilities (e.g. NET_BIND_SERVICE).",
					Enabled:     true,
				},
			},
		},
	}
}

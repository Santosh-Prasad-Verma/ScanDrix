// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package specialists

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/orchestrator"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// matrixMockLLMClient provides a controllable mock for LLM evaluations.
type matrixMockLLMClient struct {
	mu           sync.Mutex
	callCount    int
	lastSystem   string
	lastUser     string
	responseFunc func(systemPrompt, userPrompt string) (string, error)
}

func (m *matrixMockLLMClient) GenerateResponse(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	m.mu.Lock()
	m.callCount++
	m.lastSystem = systemPrompt
	m.lastUser = userPrompt
	rf := m.responseFunc
	m.mu.Unlock()

	if rf != nil {
		return rf(systemPrompt, userPrompt)
	}
	return `{"findings": []}`, nil
}

func (m *matrixMockLLMClient) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func (m *matrixMockLLMClient) GetLastPrompts() (string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastSystem, m.lastUser
}

// TestSpecialistMatrix_SecurityAgent_CWEPatterns verifies detection of OWASP Top 10 and CWE flaws.
func TestSpecialistMatrix_SecurityAgent_CWEPatterns(t *testing.T) {
	testCases := []struct {
		name          string
		cweID         string
		expectedTitle string
		severity      models.FindingSeverity
		filePath      string
		patchContent  string
		mockJSON      string
	}{
		{
			name:          "CWE-89 SQL Injection via String Concatenation",
			cweID:         "CWE-89",
			expectedTitle: "CWE-89: SQL Injection Vulnerability",
			severity:      models.SeverityCritical,
			filePath:      "internal/store/user_repo.go",
			patchContent:  `+ q := "SELECT * FROM users WHERE email = '" + req.Email + "'"`,
			mockJSON: `{
				"findings": [{
					"file_path": "internal/store/user_repo.go",
					"start_line": 42,
					"end_line": 45,
					"severity": "CRITICAL",
					"confidence": "HIGH",
					"category": "security",
					"title": "CWE-89: SQL Injection Vulnerability",
					"description": "Untrusted parameter interpolated directly into raw SQL query without parameterized bind variables.",
					"remediation": "Use parameterized query: db.QueryContext(ctx, 'SELECT * FROM users WHERE email = $1', req.Email)",
					"suggested_diff": "- q := \"SELECT * FROM users WHERE email = '\" + req.Email + \"'\"\n+ rows, err := db.QueryContext(ctx, \"SELECT * FROM users WHERE email = $1\", req.Email)",
					"blocking": true
				}]
			}`,
		},
		{
			name:          "CWE-79 Stored/Reflected Cross-Site Scripting (XSS)",
			cweID:         "CWE-79",
			expectedTitle: "CWE-79: Cross-Site Scripting via dangerouslySetInnerHTML",
			severity:      models.SeverityHigh,
			filePath:      "web/components/UserProfile.tsx",
			patchContent:  `+ <div dangerouslySetInnerHTML={{ __html: user.bio }} />`,
			mockJSON: `{
				"findings": [{
					"file_path": "web/components/UserProfile.tsx",
					"start_line": 88,
					"end_line": 89,
					"severity": "HIGH",
					"confidence": "HIGH",
					"category": "security",
					"title": "CWE-79: Cross-Site Scripting via dangerouslySetInnerHTML",
					"description": "User profile bio is rendered directly into HTML without sanitization using DOMPurify.",
					"remediation": "Sanitize with DOMPurify.sanitize(user.bio) or use standard JSX children.",
					"suggested_diff": "- <div dangerouslySetInnerHTML={{ __html: user.bio }} />\n+ <div>{user.bio}</div>",
					"blocking": true
				}]
			}`,
		},
		{
			name:          "CWE-22 Path Traversal via Unvalidated Join",
			cweID:         "CWE-22",
			expectedTitle: "CWE-22: Arbitrary File Read via Path Traversal",
			severity:      models.SeverityCritical,
			filePath:      "internal/files/server.go",
			patchContent:  `+ targetPath := filepath.Join(uploadDir, userInputPath)`,
			mockJSON: `{
				"findings": [{
					"file_path": "internal/files/server.go",
					"start_line": 115,
					"end_line": 120,
					"severity": "CRITICAL",
					"confidence": "HIGH",
					"category": "security",
					"title": "CWE-22: Arbitrary File Read via Path Traversal",
					"description": "filepath.Join does not prevent directory escape if userInputPath contains '../' leading out of uploadDir.",
					"remediation": "Verify cleaned target path begins with filepath.Clean(uploadDir) + filepath.Separator.",
					"blocking": true
				}]
			}`,
		},
		{
			name:          "CWE-502 Insecure Deserialization of Untrusted Data",
			cweID:         "CWE-502",
			expectedTitle: "CWE-502: Insecure Deserialization via Gob/Pickle",
			severity:      models.SeverityCritical,
			filePath:      "internal/worker/job_consumer.go",
			patchContent:  `+ dec := gob.NewDecoder(payloadReader); err := dec.Decode(&arbitraryObj)`,
			mockJSON: `{
				"findings": [{
					"file_path": "internal/worker/job_consumer.go",
					"start_line": 210,
					"end_line": 215,
					"severity": "CRITICAL",
					"confidence": "HIGH",
					"category": "security",
					"title": "CWE-502: Insecure Deserialization via Gob/Pickle",
					"description": "Decoding arbitrary payload structures directly without schema verification allows remote code execution.",
					"remediation": "Decode strictly into concrete, strongly typed struct with known bounds.",
					"blocking": true
				}]
			}`,
		},
		{
			name:          "CWE-798 Hardcoded Secrets and Cloud Tokens",
			cweID:         "CWE-798",
			expectedTitle: "CWE-798: Hardcoded AWS Credentials Detected",
			severity:      models.SeverityCritical,
			filePath:      "pkg/cloud/aws.go",
			patchContent:  `+ const awsAccessKey = "AKIAIOSFODNN7EXAMPLE"`,
			mockJSON: `{
				"findings": [{
					"file_path": "pkg/cloud/aws.go",
					"start_line": 14,
					"end_line": 15,
					"severity": "CRITICAL",
					"confidence": "HIGH",
					"category": "security",
					"title": "CWE-798: Hardcoded AWS Credentials Detected",
					"description": "Production credentials committed directly into repository source code.",
					"remediation": "Inject credentials via environment variables: os.Getenv('AWS_ACCESS_KEY_ID').",
					"blocking": true
				}]
			}`,
		},
		{
			name:          "CWE-918 Server-Side Request Forgery (SSRF)",
			cweID:         "CWE-918",
			expectedTitle: "CWE-918: Server-Side Request Forgery via Webhook URL",
			severity:      models.SeverityHigh,
			filePath:      "internal/webhooks/dispatcher.go",
			patchContent:  `+ resp, err := http.Get(targetWebhookURL)`,
			mockJSON: `{
				"findings": [{
					"file_path": "internal/webhooks/dispatcher.go",
					"start_line": 64,
					"end_line": 68,
					"severity": "HIGH",
					"confidence": "HIGH",
					"category": "security",
					"title": "CWE-918: Server-Side Request Forgery via Webhook URL",
					"description": "Direct HTTP request to unvalidated URL allows callers to scan internal 169.254.169.254 or localhost services.",
					"remediation": "Validate target URL host against private IP and link-local ranges before dispatch.",
					"blocking": true
				}]
			}`,
		},
		{
			name:          "CWE-327 Broken Cryptography (MD5 for Password Hashing)",
			cweID:         "CWE-327",
			expectedTitle: "CWE-327: Use of Broken Cryptographic Hash Algorithm",
			severity:      models.SeverityHigh,
			filePath:      "internal/auth/hasher.go",
			patchContent:  `+ hash := md5.Sum([]byte(password))`,
			mockJSON: `{
				"findings": [{
					"file_path": "internal/auth/hasher.go",
					"start_line": 30,
					"end_line": 32,
					"severity": "HIGH",
					"confidence": "HIGH",
					"category": "security",
					"title": "CWE-327: Use of Broken Cryptographic Hash Algorithm",
					"description": "MD5 is cryptographically broken and vulnerable to rapid collision and rainbow-table attacks.",
					"remediation": "Use Argon2id or bcrypt with a work factor >= 12.",
					"blocking": true
				}]
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &matrixMockLLMClient{
				responseFunc: func(systemPrompt, userPrompt string) (string, error) {
					// Verify focus area in user prompt
					assert.Contains(t, userPrompt, "Application Security")
					assert.Contains(t, userPrompt, tc.filePath)
					return tc.mockJSON, nil
				},
			}

			agent := NewSecuritySpecialistAgent(mockClient)
			assert.Equal(t, "security", agent.Name())
			assert.Equal(t, "security", agent.Category())

			input := orchestrator.ReviewAgentInput{
				PRNumber: 1001,
				Title:    fmt.Sprintf("Fix %s flaw", tc.cweID),
				ChangedFiles: []orchestrator.ChangedFile{
					{
						Filename:  tc.filePath,
						Patch:     tc.patchContent,
						Additions: 5,
						Deletions: 1,
					},
				},
			}

			out, err := agent.Review(context.Background(), input)
			require.NoError(t, err)
			require.Len(t, out.Findings, 1)

			finding := out.Findings[0]
			assert.Equal(t, "security", finding.AgentName)
			assert.Equal(t, tc.expectedTitle, finding.Title)
			assert.Equal(t, tc.severity, finding.Severity)
			assert.Equal(t, tc.filePath, finding.FilePath)
			assert.True(t, finding.Blocking)
			assert.NotEmpty(t, finding.Fingerprint)
			assert.Contains(t, finding.ContributingAgents, "security")
		})
	}
}

// TestSpecialistMatrix_SecurityAgent_Resilience verifies markdown trimming, fallback parsing, and error paths.
func TestSpecialistMatrix_SecurityAgent_Resilience(t *testing.T) {
	t.Run("Markdown Code Fence Trimming", func(t *testing.T) {
		wrapped := "```json\n{\n  \"findings\": [{\n    \"file_path\": \"sec.go\",\n    \"start_line\": 1,\n    \"end_line\": 2,\n    \"severity\": \"CRITICAL\",\n    \"title\": \"Security Finding\",\n    \"description\": \"Test\"\n  }]\n}\n```"
		mockClient := &matrixMockLLMClient{
			responseFunc: func(s, u string) (string, error) {
				return wrapped, nil
			},
		}

		agent := NewSecuritySpecialistAgent(mockClient)
		out, err := agent.Review(context.Background(), orchestrator.ReviewAgentInput{
			PRNumber: 1002,
			ChangedFiles: []orchestrator.ChangedFile{
				{Filename: "sec.go", Patch: "+ test"},
			},
		})
		require.NoError(t, err)
		require.Len(t, out.Findings, 1)
		assert.Equal(t, "Security Finding", out.Findings[0].Title)
	})

	t.Run("Raw Slice Fallback Parsing", func(t *testing.T) {
		rawSlice := `[{"file_path": "sec.go", "start_line": 5, "end_line": 6, "severity": "HIGH", "title": "Slice Finding", "description": "Raw array response"}]`
		mockClient := &matrixMockLLMClient{
			responseFunc: func(s, u string) (string, error) {
				return rawSlice, nil
			},
		}

		agent := NewSecuritySpecialistAgent(mockClient)
		out, err := agent.Review(context.Background(), orchestrator.ReviewAgentInput{
			PRNumber: 1003,
			ChangedFiles: []orchestrator.ChangedFile{
				{Filename: "sec.go", Patch: "+ slice"},
			},
		})
		require.NoError(t, err)
		require.Len(t, out.Findings, 1)
		assert.Equal(t, "Slice Finding", out.Findings[0].Title)
	})

	t.Run("LLM Client Error Returns Error", func(t *testing.T) {
		mockClient := &matrixMockLLMClient{
			responseFunc: func(s, u string) (string, error) {
				return "", errors.New("upstream provider connection timed out")
			},
		}

		agent := NewSecuritySpecialistAgent(mockClient)
		_, err := agent.Review(context.Background(), orchestrator.ReviewAgentInput{
			PRNumber: 1004,
			ChangedFiles: []orchestrator.ChangedFile{
				{Filename: "sec.go", Patch: "+ fail"},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection timed out")
	})

	t.Run("Invalid JSON Returns Parse Error", func(t *testing.T) {
		mockClient := &matrixMockLLMClient{
			responseFunc: func(s, u string) (string, error) {
				return "non-json random text emitted by model", nil
			},
		}

		agent := NewSecuritySpecialistAgent(mockClient)
		_, err := agent.Review(context.Background(), orchestrator.ReviewAgentInput{
			PRNumber: 1005,
			ChangedFiles: []orchestrator.ChangedFile{
				{Filename: "sec.go", Patch: "+ invalid"},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse specialist findings")
	})
}

// TestSpecialistMatrix_PerformanceAgent_Bottlenecks verifies performance anti-patterns.
func TestSpecialistMatrix_PerformanceAgent_Bottlenecks(t *testing.T) {
	testCases := []struct {
		name          string
		expectedTitle string
		severity      models.FindingSeverity
		filePath      string
		patchContent  string
		mockJSON      string
	}{
		{
			name:          "N+1 Database Query in Iteration Loop",
			expectedTitle: "N+1 Database Query in Loop",
			severity:      models.SeverityHigh,
			filePath:      "internal/services/order_service.go",
			patchContent:  `+ for _, order := range orders { customer, _ := db.GetCustomer(order.CustomerID) }`,
			mockJSON: `{
				"findings": [{
					"file_path": "internal/services/order_service.go",
					"start_line": 50,
					"end_line": 55,
					"severity": "HIGH",
					"confidence": "HIGH",
					"category": "performance",
					"title": "N+1 Database Query in Loop",
					"description": "Fetching individual customer records inside an order iteration loop produces O(N) database network round trips.",
					"remediation": "Batch customer IDs and perform a single WHERE id IN (...) query before looping.",
					"blocking": true
				}]
			}`,
		},
		{
			name:          "Goroutine Leak via Unbuffered Channel Without Receiver",
			expectedTitle: "Unbuffered Channel Goroutine Leak",
			severity:      models.SeverityCritical,
			filePath:      "internal/async/runner.go",
			patchContent:  `+ ch := make(chan error); go func() { ch <- doWork() }(); return nil`,
			mockJSON: `{
				"findings": [{
					"file_path": "internal/async/runner.go",
					"start_line": 80,
					"end_line": 85,
					"severity": "CRITICAL",
					"confidence": "HIGH",
					"category": "performance",
					"title": "Unbuffered Channel Goroutine Leak",
					"description": "Spawning background goroutine writing to unbuffered channel without reading receiver leaks goroutine permanently.",
					"remediation": "Use buffered channel of size 1 or coordinate termination using context.Context.",
					"blocking": true
				}]
			}`,
		},
		{
			name:          "Memory Reallocation Hotspot Without Capacity Hint",
			expectedTitle: "Excessive Slice Reallocations in Ingestion Loop",
			severity:      models.SeverityMedium,
			filePath:      "internal/ingest/stream.go",
			patchContent:  `+ var records []Record; for i := 0; i < 100000; i++ { records = append(records, nextRecord()) }`,
			mockJSON: `{
				"findings": [{
					"file_path": "internal/ingest/stream.go",
					"start_line": 12,
					"end_line": 16,
					"severity": "MEDIUM",
					"confidence": "HIGH",
					"category": "performance",
					"title": "Excessive Slice Reallocations in Ingestion Loop",
					"description": "Repeated slice appending without initial capacity triggers multiple memory allocations and copying overhead.",
					"remediation": "Pre-allocate slice capacity: make([]Record, 0, expectedCount).",
					"blocking": false
				}]
			}`,
		},
		{
			name:          "Quadratic O(N^2) Nested Lookup",
			expectedTitle: "Quadratic O(N^2) Complexity in Collection Matching",
			severity:      models.SeverityHigh,
			filePath:      "internal/matcher/lookup.go",
			patchContent:  `+ for _, a := range itemsA { for _, b := range itemsB { if a.ID == b.ID { matches = append(matches, a) } } }`,
			mockJSON: `{
				"findings": [{
					"file_path": "internal/matcher/lookup.go",
					"start_line": 34,
					"end_line": 40,
					"severity": "HIGH",
					"confidence": "HIGH",
					"category": "performance",
					"title": "Quadratic O(N^2) Complexity in Collection Matching",
					"description": "Nested iteration over two dynamic lists yields quadratic execution time.",
					"remediation": "Index itemsB into a hash map by ID for O(N + M) linear complexity.",
					"blocking": true
				}]
			}`,
		},
		{
			name:          "Regex Compilation Inside Function Call",
			expectedTitle: "Repeated Regex Compilation in Hot Path",
			severity:      models.SeverityMedium,
			filePath:      "pkg/sanitizer/email.go",
			patchContent:  `+ func Validate(e string) bool { re, _ := regexp.Compile("^[a-z]+@.*$"); return re.MatchString(e) }`,
			mockJSON: `{
				"findings": [{
					"file_path": "pkg/sanitizer/email.go",
					"start_line": 20,
					"end_line": 24,
					"severity": "MEDIUM",
					"confidence": "HIGH",
					"category": "performance",
					"title": "Repeated Regex Compilation in Hot Path",
					"description": "Compiling regular expression on every invocation incurs severe parsing and DFA construction overhead.",
					"remediation": "Declare compiled regex as a package-level global with regexp.MustCompile.",
					"blocking": false
				}]
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &matrixMockLLMClient{
				responseFunc: func(s, u string) (string, error) {
					assert.Contains(t, u, "Latency, N+1 Queries")
					return tc.mockJSON, nil
				},
			}

			agent := NewPerformanceSpecialistAgent(mockClient)
			assert.Equal(t, "performance", agent.Name())

			out, err := agent.Review(context.Background(), orchestrator.ReviewAgentInput{
				PRNumber: 2001,
				Title:    tc.name,
				ChangedFiles: []orchestrator.ChangedFile{
					{
						Filename: tc.filePath,
						Patch:    tc.patchContent,
					},
				},
			})
			require.NoError(t, err)
			require.Len(t, out.Findings, 1)

			f := out.Findings[0]
			assert.Equal(t, "performance", f.AgentName)
			assert.Equal(t, tc.expectedTitle, f.Title)
			assert.Equal(t, tc.severity, f.Severity)
		})
	}
}

// TestSpecialistMatrix_BugAgent_Defects verifies bug and correctness detection.
func TestSpecialistMatrix_BugAgent_Defects(t *testing.T) {
	testCases := []struct {
		name          string
		expectedTitle string
		severity      models.FindingSeverity
		filePath      string
		patchContent  string
		mockJSON      string
	}{
		{
			name:          "Concurrent Map Read/Write Data Race",
			expectedTitle: "Concurrent Map Access Without Synchronization",
			severity:      models.SeverityCritical,
			filePath:      "internal/sessions/store.go",
			patchContent:  `+ func (s *Store) Set(k, v string) { s.m[k] = v }`,
			mockJSON: `{
				"findings": [{
					"file_path": "internal/sessions/store.go",
					"start_line": 15,
					"end_line": 17,
					"severity": "CRITICAL",
					"confidence": "HIGH",
					"category": "bug",
					"title": "Concurrent Map Access Without Synchronization",
					"description": "Go runtime panics fatal error: concurrent map writes if multiple goroutines invoke Set concurrently.",
					"remediation": "Protect access with sync.RWMutex or use sync.Map.",
					"blocking": true
				}]
			}`,
		},
		{
			name:          "Nil Pointer Dereference on Unchecked Error",
			expectedTitle: "Nil Pointer Dereference on Error Result",
			severity:      models.SeverityCritical,
			filePath:      "internal/client/api.go",
			patchContent:  `+ res, _ := client.Do(req); return res.StatusCode`,
			mockJSON: `{
				"findings": [{
					"file_path": "internal/client/api.go",
					"start_line": 45,
					"end_line": 48,
					"severity": "CRITICAL",
					"confidence": "HIGH",
					"category": "bug",
					"title": "Nil Pointer Dereference on Error Result",
					"description": "If client.Do returns an error, res is nil. Accessing res.StatusCode will panic with SIGSEGV.",
					"remediation": "Check error: if err != nil { return 0, err } before inspecting response.",
					"blocking": true
				}]
			}`,
		},
		{
			name:          "Resource Leak via Missing defer resp.Body.Close()",
			expectedTitle: "HTTP Response Body File Descriptor Leak",
			severity:      models.SeverityHigh,
			filePath:      "internal/gateway/fetcher.go",
			patchContent:  `+ resp, err := http.Get(url); if err != nil { return err }; io.ReadAll(resp.Body)`,
			mockJSON: `{
				"findings": [{
					"file_path": "internal/gateway/fetcher.go",
					"start_line": 30,
					"end_line": 35,
					"severity": "HIGH",
					"confidence": "HIGH",
					"category": "bug",
					"title": "HTTP Response Body File Descriptor Leak",
					"description": "resp.Body is never closed, leaking TCP socket connections and OS file descriptors under sustained traffic.",
					"remediation": "Add defer resp.Body.Close() immediately following successful error check.",
					"blocking": true
				}]
			}`,
		},
		{
			name:          "Channel Deadlock on Single Goroutine",
			expectedTitle: "Synchronous Channel Send Deadlock",
			severity:      models.SeverityCritical,
			filePath:      "internal/pipeline/sync.go",
			patchContent:  `+ ch := make(chan int); ch <- 42; v := <-ch`,
			mockJSON: `{
				"findings": [{
					"file_path": "internal/pipeline/sync.go",
					"start_line": 22,
					"end_line": 25,
					"severity": "CRITICAL",
					"confidence": "HIGH",
					"category": "bug",
					"title": "Synchronous Channel Send Deadlock",
					"description": "Sending to unbuffered channel on same goroutine before receiver is attached blocks permanently (fatal error: all goroutines are asleep - deadlock!).",
					"remediation": "Buffer channel make(chan int, 1) or execute send in separate goroutine.",
					"blocking": true
				}]
			}`,
		},
		{
			name:          "Off-by-One Slice Boundary Panic",
			expectedTitle: "Off-by-One Index Out of Range",
			severity:      models.SeverityHigh,
			filePath:      "pkg/algo/window.go",
			patchContent:  `+ for i := 0; i <= len(items); i++ { process(items[i]) }`,
			mockJSON: `{
				"findings": [{
					"file_path": "pkg/algo/window.go",
					"start_line": 10,
					"end_line": 12,
					"severity": "HIGH",
					"confidence": "HIGH",
					"category": "bug",
					"title": "Off-by-One Index Out of Range",
					"description": "Loop condition i <= len(items) accesses items[len(items)], triggering panic: runtime error: index out of range.",
					"remediation": "Change condition to i < len(items) or use range items.",
					"blocking": true
				}]
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &matrixMockLLMClient{
				responseFunc: func(s, u string) (string, error) {
					assert.Contains(t, u, "Runtime Reliability")
					return tc.mockJSON, nil
				},
			}

			agent := NewBugSpecialistAgent(mockClient)
			assert.Equal(t, "bug", agent.Name())

			out, err := agent.Review(context.Background(), orchestrator.ReviewAgentInput{
				PRNumber: 3001,
				Title:    tc.name,
				ChangedFiles: []orchestrator.ChangedFile{
					{
						Filename: tc.filePath,
						Patch:    tc.patchContent,
					},
				},
			})
			require.NoError(t, err)
			require.Len(t, out.Findings, 1)

			f := out.Findings[0]
			assert.Equal(t, "bug", f.AgentName)
			assert.Equal(t, tc.expectedTitle, f.Title)
			assert.Equal(t, tc.severity, f.Severity)
		})
	}
}

// TestSpecialistMatrix_ArchitectureAgent_Violations verifies architectural boundaries and coupling.
func TestSpecialistMatrix_ArchitectureAgent_Violations(t *testing.T) {
	testCases := []struct {
		name          string
		expectedTitle string
		severity      models.FindingSeverity
		filePath      string
		patchContent  string
		mockJSON      string
	}{
		{
			name:          "Clean Architecture Boundary Leakage",
			expectedTitle: "Domain Entity Violates Dependency Rule",
			severity:      models.SeverityHigh,
			filePath:      "internal/domain/user.go",
			patchContent:  `+ import "github.com/jackc/pgx/v5"`,
			mockJSON: `{
				"findings": [{
					"file_path": "internal/domain/user.go",
					"start_line": 5,
					"end_line": 8,
					"severity": "HIGH",
					"confidence": "HIGH",
					"category": "architecture",
					"title": "Domain Entity Violates Dependency Rule",
					"description": "Core domain layer imports infrastructure database driver directly, violating Clean Architecture inward dependency rule.",
					"remediation": "Define repository interface in domain; implement database adapters in infrastructure layer.",
					"blocking": true
				}]
			}`,
		},
		{
			name:          "Cyclic Package Dependency Introduced",
			expectedTitle: "Cyclic Package Import Graph",
			severity:      models.SeverityCritical,
			filePath:      "pkg/auth/authenticator.go",
			patchContent:  `+ import "github.com/scandrix/backend/pkg/users"`,
			mockJSON: `{
				"findings": [{
					"file_path": "pkg/auth/authenticator.go",
					"start_line": 10,
					"end_line": 12,
					"severity": "CRITICAL",
					"confidence": "HIGH",
					"category": "architecture",
					"title": "Cyclic Package Import Graph",
					"description": "pkg/users already imports pkg/auth. Importing pkg/users from pkg/auth creates an import cycle not allowed in Go.",
					"remediation": "Extract shared user credential model to a neutral shared types package.",
					"blocking": true
				}]
			}`,
		},
		{
			name:          "God Object / Monolithic Controller Anti-Pattern",
			expectedTitle: "God Object Service Violates SRP",
			severity:      models.SeverityMedium,
			filePath:      "internal/services/mega_manager.go",
			patchContent:  `+ type MegaManager struct { auth Auth; billing Billing; notifications Notif; search Search }`,
			mockJSON: `{
				"findings": [{
					"file_path": "internal/services/mega_manager.go",
					"start_line": 25,
					"end_line": 40,
					"severity": "MEDIUM",
					"confidence": "HIGH",
					"category": "architecture",
					"title": "God Object Service Violates SRP",
					"description": "MegaManager concentrates unrelated cross-cutting domains, increasing coupling and impeding independent testing.",
					"remediation": "Split into cohesive, single-responsibility domain services.",
					"blocking": false
				}]
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &matrixMockLLMClient{
				responseFunc: func(s, u string) (string, error) {
					assert.Contains(t, u, "Clean Architecture")
					return tc.mockJSON, nil
				},
			}

			agent := NewArchitectureSpecialistAgent(mockClient)
			assert.Equal(t, "architecture", agent.Name())

			out, err := agent.Review(context.Background(), orchestrator.ReviewAgentInput{
				PRNumber: 4001,
				Title:    tc.name,
				ChangedFiles: []orchestrator.ChangedFile{
					{
						Filename: tc.filePath,
						Patch:    tc.patchContent,
					},
				},
				ExternalContext: "System Architecture: Hexagonal / Clean Architecture standard.",
			})
			require.NoError(t, err)
			require.Len(t, out.Findings, 1)

			f := out.Findings[0]
			assert.Equal(t, "architecture", f.AgentName)
			assert.Equal(t, tc.expectedTitle, f.Title)
			assert.Equal(t, tc.severity, f.Severity)
		})
	}
}

// TestSpecialistMatrix_DrixyRulesAgent_ShardingAndMatching verifies rule filtering, globbing, and sharded execution.
func TestSpecialistMatrix_DrixyRulesAgent_ShardingAndMatching(t *testing.T) {
	rule1ID := uuid.New()
	rule2ID := uuid.New()
	ruleInactiveID := uuid.New()

	rules := []orchestrator.DrixyRule{
		{
			ID:          rule1ID,
			Name:        "Require Structured Logging in API Handlers",
			Description: "Never use fmt.Println or standard log; always use slog or structured logger.",
			Severity:    models.SeverityHigh,
			Scope:       "file",
			PathGlobs:   []string{"internal/api/**/*.go", "cmd/**/*.go"},
			IsActive:    true,
		},
		{
			ID:          rule2ID,
			Name:        "Context Propagation In Client Calls",
			Description: "All external network calls must receive ctx context.Context as first argument.",
			Severity:    models.SeverityCritical,
			Scope:       "file",
			PathGlobs:   []string{"**/*.go"},
			IsActive:    true,
		},
		{
			ID:          ruleInactiveID,
			Name:        "Deprecated Rule",
			Description: "Should be skipped because IsActive is false.",
			Severity:    models.SeverityLow,
			Scope:       "file",
			PathGlobs:   []string{"**/*"},
			IsActive:    false,
		},
	}

	mockExec := &mockJudgeExecutor{
		response: fmt.Sprintf(`{
			"violations": [
				{
					"rule_id": 1,
					"file_path": "internal/api/orders.go",
					"relevant_lines_start": 35,
					"relevant_lines_end": 36,
					"suggestion_content": "fmt.Println used for request logging instead of structured logger",
					"suggested_code": "logger.InfoContext(ctx, \"order created\", \"id\", order.ID)"
				},
				{
					"rule_id": 2,
					"file_path": "internal/api/orders.go",
					"relevant_lines_start": 40,
					"relevant_lines_end": 42,
					"suggestion_content": "client.Call invoked without context.Context",
					"suggested_code": "client.CallWithContext(ctx, req)"
				}
			]
		}`),
	}

	agent := NewDrixyRulesSpecialistAgent(mockExec)
	assert.Equal(t, "drixy_rules", agent.Name())

	input := orchestrator.ReviewAgentInput{
		PRNumber: 5001,
		ChangedFiles: []orchestrator.ChangedFile{
			{
				Filename: "internal/api/orders.go",
				Patch:    "+ fmt.Println(order)\n+ client.Call(req)",
			},
			{
				Filename: "docs/readme.md",
				Patch:    "+ # Readme updates",
			},
		},
		DrixyRules: rules,
	}

	out, err := agent.Review(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, out.Findings, 2)

	// Verify rule ID attributions
	assert.Equal(t, "drixy_rules", out.Findings[0].AgentName)
	assert.Equal(t, &rule1ID, out.Findings[0].RuleID)
	assert.Equal(t, 35, out.Findings[0].StartLine)
	assert.Equal(t, 36, out.Findings[0].EndLine)

	assert.Equal(t, "drixy_rules", out.Findings[1].AgentName)
	assert.Equal(t, &rule2ID, out.Findings[1].RuleID)
	assert.Equal(t, 40, out.Findings[1].StartLine)
	assert.Equal(t, 42, out.Findings[1].EndLine)
}

// TestSpecialistMatrix_BusinessLogicAgent_Validation verifies ticket key extraction and requirement checks.
func TestSpecialistMatrix_BusinessLogicAgent_Validation(t *testing.T) {
	agent := NewBusinessLogicSpecialistAgent(nil)

	t.Run("Multi-Platform Ticket Key Extraction", func(t *testing.T) {
		prDesc := `
		Implements requirements for PROJ-9921 and fixes bug ENG-4128.
		Relates to github issue #1084 and #33.
		Also mentions SCANDRIX-7712.
		`
		keys := agent.ExtractTicketKeys(prDesc)
		assert.Contains(t, keys, "PROJ-9921")
		assert.Contains(t, keys, "ENG-4128")
		assert.Contains(t, keys, "#1084")
		assert.Contains(t, keys, "#33")
		assert.Contains(t, keys, "SCANDRIX-7712")
	})

	t.Run("Requirements Section Detection", func(t *testing.T) {
		validDesc1 := "## Acceptance Criteria\n- User must receive verification email\n- Session token expires in 15m"
		validDesc2 := "### Requirements:\n1. Limit payload to 10MB"
		validDesc3 := "Expected Behavior: Status should be 201 Created on success"
		invalidDesc := "Just updating typos in documentation and bumping package versions."

		assert.True(t, agent.HasRequirementsDeclared(validDesc1))
		assert.True(t, agent.HasRequirementsDeclared(validDesc2))
		assert.True(t, agent.HasRequirementsDeclared(validDesc3))
		assert.False(t, agent.HasRequirementsDeclared(invalidDesc))
	})

	t.Run("Graceful Skip on Non-Logic PR", func(t *testing.T) {
		out, err := agent.Review(context.Background(), orchestrator.ReviewAgentInput{
			PRNumber:    6001,
			Title:       "chore: update dependencies",
			Description: "routine dependabot bump",
			ChangedFiles: []orchestrator.ChangedFile{
				{Filename: "go.sum", Patch: "+ updated"},
			},
		})
		require.NoError(t, err)
		assert.Empty(t, out.Findings)
	})

	t.Run("Business Logic Validation with LLM", func(t *testing.T) {
		mockClient := &matrixMockLLMClient{
			responseFunc: func(s, u string) (string, error) {
				assert.Contains(t, u, "Acceptance Criteria")
				return `{
					"findings": [{
						"file_path": "internal/billing/checkout.go",
						"start_line": 88,
						"end_line": 95,
						"severity": "HIGH",
						"confidence": "HIGH",
						"category": "business_logic",
						"title": "Acceptance Criteria Omission: Currency Conversion Missing",
						"description": "PR description requires multi-currency conversion for EUR transactions, but checkout.go only charges USD.",
						"remediation": "Invoke CurrencyConverterService before creating Stripe charge.",
						"blocking": true
					}]
				}`, nil
			},
		}

		blAgent := NewBusinessLogicSpecialistAgent(mockClient)
		out, err := blAgent.Review(context.Background(), orchestrator.ReviewAgentInput{
			PRNumber:    6002,
			Title:       "feat(checkout): support European merchants (ENG-882)",
			Description: "Acceptance Criteria: All EUR transactions must be converted using the spot rate.",
			ChangedFiles: []orchestrator.ChangedFile{
				{
					Filename: "internal/billing/checkout.go",
					Patch:    "+ func Charge(c Customer) { stripe.Charge(c.Amount) }",
				},
			},
		})
		require.NoError(t, err)
		require.Len(t, out.Findings, 1)
		assert.Equal(t, "business_logic", out.Findings[0].AgentName)
		assert.Equal(t, "Acceptance Criteria Omission: Currency Conversion Missing", out.Findings[0].Title)
	})
}

// TestSpecialistMatrix_GeneralistAgent_Aggregation verifies multi-category review.
func TestSpecialistMatrix_GeneralistAgent_Aggregation(t *testing.T) {
	mockResp := `{
		"findings": [
			{
				"file_path": "internal/orders/handler.go",
				"start_line": 10,
				"end_line": 15,
				"severity": "HIGH",
				"category": "security",
				"title": "Missing Authorization Header Validation",
				"description": "Endpoint does not verify JWT claims before fulfilling order.",
				"blocking": true
			},
			{
				"file_path": "internal/orders/handler.go",
				"start_line": 25,
				"end_line": 30,
				"severity": "MEDIUM",
				"category": "performance",
				"title": "Synchronous Audit Log Flush",
				"description": "Flushing audit logs synchronously blocks HTTP response loop.",
				"blocking": false
			}
		]
	}`

	mockClient := &matrixMockLLMClient{
		responseFunc: func(s, u string) (string, error) {
			return mockResp, nil
		},
	}

	agent := NewGeneralistSpecialistAgent(mockClient)
	assert.Equal(t, "generalist", agent.Name())
	assert.Equal(t, "generalist", agent.Category())

	out, err := agent.Review(context.Background(), orchestrator.ReviewAgentInput{
		PRNumber: 7001,
		Title:    "Add order processing endpoint",
		ChangedFiles: []orchestrator.ChangedFile{
			{Filename: "internal/orders/handler.go", Patch: "+ code"},
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Findings, 2)

	assert.Equal(t, "generalist", out.Findings[0].AgentName)
	assert.Equal(t, "security", out.Findings[0].Category)
	assert.True(t, out.Findings[0].Blocking)

	assert.Equal(t, "generalist", out.Findings[1].AgentName)
	assert.Equal(t, "performance", out.Findings[1].Category)
	assert.False(t, out.Findings[1].Blocking)
}

// TestSpecialistMatrix_ConcurrentStressAndRaceDetector executes all specialists under heavy concurrency.
func TestSpecialistMatrix_ConcurrentStressAndRaceDetector(t *testing.T) {
	workers := 25
	iterationsPerWorker := 10

	mockResp := `{
		"findings": [{
			"file_path": "internal/concurrent/worker.go",
			"start_line": 10,
			"end_line": 12,
			"severity": "HIGH",
			"confidence": "HIGH",
			"category": "bug",
			"title": "Concurrent Access Bug",
			"description": "Simulated concurrent finding",
			"blocking": true
		}]
	}`

	mockClient := &matrixMockLLMClient{
		responseFunc: func(s, u string) (string, error) {
			// Simulate slight network latency
			time.Sleep(1 * time.Millisecond)
			return mockResp, nil
		},
	}

	securityAgent := NewSecuritySpecialistAgent(mockClient)
	perfAgent := NewPerformanceSpecialistAgent(mockClient)
	bugAgent := NewBugSpecialistAgent(mockClient)
	archAgent := NewArchitectureSpecialistAgent(mockClient)
	generalistAgent := NewGeneralistSpecialistAgent(mockClient)

	agents := []orchestrator.IReviewSpecialist{
		securityAgent,
		perfAgent,
		bugAgent,
		archAgent,
		generalistAgent,
	}

	var wg sync.WaitGroup
	errCh := make(chan error, workers*iterationsPerWorker*len(agents))

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterationsPerWorker; i++ {
				for _, ag := range agents {
					input := orchestrator.ReviewAgentInput{
						PRNumber: workerID*1000 + i,
						Title:    fmt.Sprintf("Concurrent Review %d-%d", workerID, i),
						ChangedFiles: []orchestrator.ChangedFile{
							{
								Filename:  fmt.Sprintf("pkg/worker_%d/file_%d.go", workerID, i),
								Patch:     "+ func DoConcurrentWork() {}",
								Additions: 1,
								Deletions: 0,
							},
						},
					}

					out, err := ag.Review(context.Background(), input)
					if err != nil {
						errCh <- fmt.Errorf("agent %s worker %d iter %d failed: %w", ag.Name(), workerID, i, err)
						return
					}
					if len(out.Findings) != 1 {
						errCh <- fmt.Errorf("agent %s expected 1 finding got %d", ag.Name(), len(out.Findings))
						return
					}
				}
			}
		}(w)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrency error detected: %v", err)
	}

	totalExpectedCalls := workers * iterationsPerWorker * len(agents)
	assert.Equal(t, totalExpectedCalls, mockClient.GetCallCount())
}

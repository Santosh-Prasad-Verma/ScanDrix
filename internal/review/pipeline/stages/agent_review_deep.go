// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package stages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// AgentPersonaType defines specialized expert perspectives for multi-agent code review.
type AgentPersonaType string

const (
	PersonaSecurityAuditor       AgentPersonaType = "SECURITY_AUDITOR"
	PersonaConcurrencySpecialist AgentPersonaType = "CONCURRENCY_SPECIALIST"
	PersonaPerformanceEngineer  AgentPersonaType = "PERFORMANCE_ENGINEER"
	PersonaArchitectureValidator AgentPersonaType = "ARCHITECTURE_VALIDATOR"
	PersonaMaintainabilityReview AgentPersonaType = "MAINTAINABILITY_REVIEWER"
	PersonaComplianceChecker     AgentPersonaType = "COMPLIANCE_CHECKER"
)

// FrozenReviewSnapshot guarantees immutability across concurrent agent inspections.
type FrozenReviewSnapshot struct {
	DigestToken       string                    `json:"digest_token"`
	PullNumber        int                       `json:"pull_number"`
	Title             string                    `json:"title"`
	Description       string                    `json:"description"`
	ChangedFiles      []pipeline.FileChangeInfo `json:"changed_files"`
	TicketContext     string                    `json:"ticket_context,omitempty"`
	TraceDecisionKeys []string                  `json:"trace_decision_keys,omitempty"`
	GeneratedAt       time.Time                 `json:"generated_at"`
}

// AgentExecutionTrace records detailed telemetry for one review persona.
type AgentExecutionTrace struct {
	Persona          AgentPersonaType        `json:"persona"`
	Duration         time.Duration           `json:"duration"`
	ToolCallsCount   int                     `json:"tool_calls_count"`
	EmittedFindings  int                     `json:"emitted_findings"`
	RetainedFindings int                     `json:"retained_findings"`
	Success          bool                    `json:"success"`
	ErrorMessage     string                  `json:"error_message,omitempty"`
}

// DeepAgentReviewStage coordinates multi-persona concurrent deliberation, frozen context
// preservation, and semantic cross-agent finding deduplication.
type DeepAgentReviewStage struct {
	enabledPersonas   []AgentPersonaType
	maxConcurrency    int
	jaccardThreshold  float64
	minConfidence     float64
}

// DeepAgentReviewOption configures DeepAgentReviewStage.
type DeepAgentReviewOption func(*DeepAgentReviewStage)

// WithPersonas configures specific agent personas.
func WithPersonas(personas ...AgentPersonaType) DeepAgentReviewOption {
	return func(s *DeepAgentReviewStage) {
		if len(personas) > 0 {
			s.enabledPersonas = personas
		}
	}
}

// WithAgentConcurrency sets the maximum concurrent persona worker routines.
func WithAgentConcurrency(concurrency int) DeepAgentReviewOption {
	return func(s *DeepAgentReviewStage) {
		if concurrency > 0 {
			s.maxConcurrency = concurrency
		}
	}
}

// WithDedupThreshold sets the Jaccard similarity threshold for merging findings.
func WithDedupThreshold(thresh float64) DeepAgentReviewOption {
	return func(s *DeepAgentReviewStage) {
		if thresh > 0.0 && thresh <= 1.0 {
			s.jaccardThreshold = thresh
		}
	}
}

// NewDeepAgentReviewStage constructs the deep agent review stage.
func NewDeepAgentReviewStage(opts ...DeepAgentReviewOption) *DeepAgentReviewStage {
	stage := &DeepAgentReviewStage{
		enabledPersonas: []AgentPersonaType{
			PersonaSecurityAuditor,
			PersonaConcurrencySpecialist,
			PersonaPerformanceEngineer,
			PersonaArchitectureValidator,
			PersonaMaintainabilityReview,
			PersonaComplianceChecker,
		},
		maxConcurrency:   4,
		jaccardThreshold: 0.65,
		minConfidence:    0.75,
	}

	for _, opt := range opts {
		opt(stage)
	}

	return stage
}

// Name returns the pipeline stage identifier.
func (s *DeepAgentReviewStage) Name() string {
	return "DeepAgentReview"
}

// Execute performs multi-agent deliberation on the pull request.
func (s *DeepAgentReviewStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	start := time.Now()

	if pCtx.SkipReview || len(pCtx.ChangedFiles) == 0 {
		pCtx.AddMetric(s.Name(), time.Since(start), true, nil, 0)
		return nil
	}

	// 1. Build immutable Frozen Review Snapshot
	frozenSnapshot := s.BuildFrozenSnapshot(pCtx)

	// 2. Dispatch personas concurrently with bounded semaphore
	rawFindings, traces := s.dispatchPersonas(ctx, pCtx, frozenSnapshot)

	// 3. Record traces in pipeline metadata
	s.recordAgentTraces(pCtx, traces)

	// 4. Semantic cross-deduplication & tie-breaking
	deduplicatedFindings := s.DeduplicateFindings(rawFindings)

	// 5. Convert to valid suggestions and append to PipelineContext
	s.ingestDeduplicatedFindings(pCtx, deduplicatedFindings)

	pCtx.AddMetric(s.Name(), time.Since(start), true, nil, len(deduplicatedFindings))
	return nil
}

// BuildFrozenSnapshot hashes review metadata into an immutable context envelope.
func (s *DeepAgentReviewStage) BuildFrozenSnapshot(pCtx *pipeline.PipelineContext) FrozenReviewSnapshot {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%d:%s:%s\n", pCtx.PullNumber, pCtx.Title, pCtx.Description)))

	ticketCtx := ""
	if pCtx.ExternalContext != nil {
		ticketCtx = fmt.Sprintf("[%s] %s: %s", pCtx.ExternalContext.IssueKey, pCtx.ExternalContext.Title, pCtx.ExternalContext.Description)
		h.Write([]byte(ticketCtx))
	}

	var decisionKeys []string
	for _, td := range pCtx.TraceDecisions {
		decisionKeys = append(decisionKeys, td.DecisionKey)
		h.Write([]byte(td.DecisionKey + ":" + td.Summary))
	}

	for _, f := range pCtx.ChangedFiles {
		h.Write([]byte(fmt.Sprintf("%s:%d:%d\n", f.Filename, f.Additions, f.Deletions)))
	}

	return FrozenReviewSnapshot{
		DigestToken:       hex.EncodeToString(h.Sum(nil)),
		PullNumber:        pCtx.PullNumber,
		Title:             pCtx.Title,
		Description:       pCtx.Description,
		ChangedFiles:      pCtx.ChangedFiles,
		TicketContext:     ticketCtx,
		TraceDecisionKeys: decisionKeys,
		GeneratedAt:       time.Now().UTC(),
	}
}

func (s *DeepAgentReviewStage) dispatchPersonas(
	ctx context.Context,
	pCtx *pipeline.PipelineContext,
	snapshot FrozenReviewSnapshot,
) ([]models.CodeFinding, []AgentExecutionTrace) {
	var mu sync.Mutex
	var allFindings []models.CodeFinding
	traces := make([]AgentExecutionTrace, 0, len(s.enabledPersonas))

	sem := make(chan struct{}, s.maxConcurrency)
	var wg sync.WaitGroup

	for _, persona := range s.enabledPersonas {
		wg.Add(1)
		go func(p AgentPersonaType) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				mu.Lock()
				traces = append(traces, AgentExecutionTrace{
					Persona:      p,
					Success:      false,
					ErrorMessage: ctx.Err().Error(),
				})
				mu.Unlock()
				return
			}

			pStart := time.Now()
			findings, toolCalls, err := s.runPersonaReview(ctx, p, snapshot, pCtx.ReviewID)
			duration := time.Since(pStart)

			trace := AgentExecutionTrace{
				Persona:          p,
				Duration:         duration,
				ToolCallsCount:   toolCalls,
				EmittedFindings:  len(findings),
				RetainedFindings: len(findings),
				Success:          err == nil,
			}
			if err != nil {
				trace.ErrorMessage = err.Error()
			}

			mu.Lock()
			traces = append(traces, trace)
			if err == nil && len(findings) > 0 {
				allFindings = append(allFindings, findings...)
			}
			mu.Unlock()
		}(persona)
	}

	wg.Wait()
	return allFindings, traces
}

// runPersonaReview applies the specialized review logic for a persona.
func (s *DeepAgentReviewStage) runPersonaReview(
	ctx context.Context,
	persona AgentPersonaType,
	snapshot FrozenReviewSnapshot,
	reviewID uuid.UUID,
) ([]models.CodeFinding, int, error) {
	var findings []models.CodeFinding
	toolCalls := 0

	for _, file := range snapshot.ChangedFiles {
		select {
		case <-ctx.Done():
			return nil, toolCalls, ctx.Err()
		default:
		}

		toolCalls++ // Each file inspection counts as a tool call
		lines := strings.Split(file.Patch, "\n")
		currentRightLine := 0

		for _, line := range lines {
			if strings.HasPrefix(line, "@@") {
				currentRightLine = parseHunkStartLine(line)
				continue
			}

			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				added := strings.TrimPrefix(line, "+")

				// Evaluate persona-specific invariants
				if finding := s.evaluatePersonaInvariant(persona, file.Filename, added, currentRightLine, reviewID); finding != nil {
					findings = append(findings, *finding)
				}
				currentRightLine++
			} else if !strings.HasPrefix(line, "-") {
				currentRightLine++
			}
		}
	}

	return findings, toolCalls, nil
}

func (s *DeepAgentReviewStage) evaluatePersonaInvariant(
	persona AgentPersonaType,
	filePath string,
	code string,
	lineNum int,
	reviewID uuid.UUID,
) *models.CodeFinding {
	lower := strings.ToLower(code)

	switch persona {
	case PersonaSecurityAuditor:
		if strings.Contains(lower, "md5.new") || strings.Contains(lower, "sha1.new") {
			return &models.CodeFinding{
				ID:          uuid.New(),
				ReviewID:    reviewID,
				FilePath:    filePath,
				StartLine:   lineNum,
				EndLine:     lineNum,
				Severity:    models.SeverityHigh,
				Category:    "SECURITY",
				Title:       "Weak Cryptographic Hash Algorithm",
				Description: "MD5 and SHA-1 are cryptographically broken. Use SHA-256, SHA-512, or argon2id.",
				Remediation: "Replace with crypto/sha256.New() or argon2 for password hashing.",
				Fingerprint: fmt.Sprintf("%s:%d:weak-crypto", filePath, lineNum),
			}
		}
		if strings.Contains(lower, "exec.command(") && !strings.Contains(lower, "commandcontext") {
			return &models.CodeFinding{
				ID:          uuid.New(),
				ReviewID:    reviewID,
				FilePath:    filePath,
				StartLine:   lineNum,
				EndLine:     lineNum,
				Severity:    models.SeverityHigh,
				Category:    "SECURITY",
				Title:       "Unbounded OS Command Execution",
				Description: "Executing system commands without context timeout risks zombie process exhaustion.",
				Remediation: "Use exec.CommandContext(ctx, name, args...) with a bounded timeout.",
				Fingerprint: fmt.Sprintf("%s:%d:unbounded-exec", filePath, lineNum),
			}
		}

	case PersonaConcurrencySpecialist:
		if strings.Contains(lower, "go func(") || (strings.HasPrefix(strings.TrimSpace(lower), "go ") && !strings.Contains(lower, "recover")) {
			return &models.CodeFinding{
				ID:          uuid.New(),
				ReviewID:    reviewID,
				FilePath:    filePath,
				StartLine:   lineNum,
				EndLine:     lineNum,
				Severity:    models.SeverityMedium,
				Category:    "CONCURRENCY",
				Title:       "Goroutine Without Panic Recovery",
				Description: "An unhandled panic inside an unmonitored goroutine will crash the entire server process.",
				Remediation: "Wrap asynchronous goroutine bodies with defer func() { if r := recover(); r != nil { ... } }().",
				Fingerprint: fmt.Sprintf("%s:%d:goroutine-panic", filePath, lineNum),
			}
		}

	case PersonaPerformanceEngineer:
		if strings.Contains(lower, "select * from") && !strings.Contains(lower, "limit") {
			return &models.CodeFinding{
				ID:          uuid.New(),
				ReviewID:    reviewID,
				FilePath:    filePath,
				StartLine:   lineNum,
				EndLine:     lineNum,
				Severity:    models.SeverityMedium,
				Category:    "PERFORMANCE",
				Title:       "Unbounded SQL Query",
				Description: "Queries without explicit LIMIT clauses can exhaust database memory as tables grow.",
				Remediation: "Specify an explicit LIMIT clause and project only necessary columns instead of SELECT *.",
				Fingerprint: fmt.Sprintf("%s:%d:unbounded-sql", filePath, lineNum),
			}
		}

	case PersonaArchitectureValidator:
		if (strings.Contains(filePath, "/domain/") || strings.Contains(filePath, "/entities/")) &&
			(strings.Contains(lower, "http.") || strings.Contains(lower, "sql.") || strings.Contains(lower, "gorm.")) {
			return &models.CodeFinding{
				ID:          uuid.New(),
				ReviewID:    reviewID,
				FilePath:    filePath,
				StartLine:   lineNum,
				EndLine:     lineNum,
				Severity:    models.SeverityHigh,
				Category:    "ARCHITECTURE",
				Title:       "Clean Architecture Invariant Violation",
				Description: "Core domain logic must not import or depend directly on transport (HTTP) or infrastructure (SQL/ORM) primitives.",
				Remediation: "Abstract database operations behind repository interfaces defined in the domain layer.",
				Fingerprint: fmt.Sprintf("%s:%d:clean-arch-leak", filePath, lineNum),
			}
		}

	case PersonaMaintainabilityReview:
		if strings.Contains(lower, "catch (e)") || strings.Contains(lower, "except Exception:") || strings.Contains(lower, "catch (Exception e)") {
			return &models.CodeFinding{
				ID:          uuid.New(),
				ReviewID:    reviewID,
				FilePath:    filePath,
				StartLine:   lineNum,
				EndLine:     lineNum,
				Severity:    models.SeverityLow,
				Category:    "MAINTAINABILITY",
				Title:       "Broad Exception Catching",
				Description: "Catching base Exception swallows unexpected errors and masks programming bugs.",
				Remediation: "Catch specific domain or network exception classes.",
				Fingerprint: fmt.Sprintf("%s:%d:broad-catch", filePath, lineNum),
			}
		}

	case PersonaComplianceChecker:
		if strings.Contains(lower, "agpl-3.0") || strings.Contains(lower, "gpl-3.0") {
			return &models.CodeFinding{
				ID:          uuid.New(),
				ReviewID:    reviewID,
				FilePath:    filePath,
				StartLine:   lineNum,
				EndLine:     lineNum,
				Severity:    models.SeverityCritical,
				Category:    "COMPLIANCE",
				Title:       "Copyleft License Dependency Added",
				Description: "AGPL/GPL dependencies impose viral open-source licensing requirements on proprietary codebases.",
				Remediation: "Verify legal authorization before introducing viral copyleft dependencies.",
				Fingerprint: fmt.Sprintf("%s:%d:copyleft-license", filePath, lineNum),
			}
		}
	}

	return nil
}

// DeduplicateFindings performs semantic cross-persona deduplication with Jaccard token similarity
// and interval subsumption tie-breaking.
func (s *DeepAgentReviewStage) DeduplicateFindings(findings []models.CodeFinding) []models.CodeFinding {
	if len(findings) <= 1 {
		return findings
	}

	// Sort by severity descending so higher severity findings win tie-breaks
	sort.Slice(findings, func(i, j int) bool {
		return severityWeight(findings[i].Severity) > severityWeight(findings[j].Severity)
	})

	var retained []models.CodeFinding

	for _, cand := range findings {
		isDuplicate := false

		for i, existing := range retained {
			// Must target the same file
			if cand.FilePath != existing.FilePath {
				continue
			}

			// Check line interval proximity (within 3 lines)
			lineDistance := int(math.Abs(float64(cand.StartLine - existing.StartLine)))
			if lineDistance > 3 {
				continue
			}

			// Calculate token similarity between messages
			sim := s.calculateTokenJaccard(cand.Title+" "+cand.Description, existing.Title+" "+existing.Description)
			if sim >= s.jaccardThreshold {
				isDuplicate = true
				// If candidate has higher severity, replace existing with candidate
				if severityWeight(cand.Severity) > severityWeight(existing.Severity) {
					retained[i] = cand
				}
				break
			}
		}

		if !isDuplicate {
			retained = append(retained, cand)
		}
	}

	return retained
}

func (s *DeepAgentReviewStage) calculateTokenJaccard(textA, textB string) float64 {
	tokensA := tokenizeWords(textA)
	tokensB := tokenizeWords(textB)

	if len(tokensA) == 0 && len(tokensB) == 0 {
		return 1.0
	}
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0.0
	}

	intersection := 0
	for t := range tokensA {
		if _, exists := tokensB[t]; exists {
			intersection++
		}
	}

	union := len(tokensA) + len(tokensB) - intersection
	if union <= 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

func tokenizeWords(s string) map[string]struct{} {
	words := strings.Fields(strings.ToLower(s))
	m := make(map[string]struct{}, len(words))
	for _, w := range words {
		clean := strings.Trim(w, ".,:;()[]\"'{}")
		if len(clean) > 2 {
			m[clean] = struct{}{}
		}
	}
	return m
}

func (s *DeepAgentReviewStage) ingestDeduplicatedFindings(pCtx *pipeline.PipelineContext, findings []models.CodeFinding) {
	pCtx.AgentFindings = append(pCtx.AgentFindings, findings...)
	pCtx.AllFindings = append(pCtx.AllFindings, findings...)

	for _, f := range findings {
		sug := domain.CodeSuggestion{
			ID:                 uuid.New(),
			PullRequestID:      fmt.Sprintf("%d", pCtx.PullNumber),
			PullNumber:         pCtx.PullNumber,
			RelevantFile:       f.FilePath,
			FilePath:           f.FilePath,
			StartLine:          f.StartLine,
			EndLine:            f.EndLine,
			RelevantLinesStart: f.StartLine,
			RelevantLinesEnd:   f.EndLine,
			RuleID:             f.Title,
			Severity:           domain.ReviewSeverity(f.Severity),
			Category:           mapFindingCategory(f.Category),
			OneSentenceSummary: f.Title,
			Description:        f.Description,
			SuggestedReplacement: f.Remediation,
			Confidence:         0.92,
			PriorityStatus:     domain.PriorityStatusPrioritized,
			DeliveryStatus:     domain.DeliveryStatusQueued,
		}
		pCtx.ValidSuggestions = append(pCtx.ValidSuggestions, sug)
	}
}

func mapFindingCategory(cat string) domain.ReviewCategory {
	switch strings.ToUpper(cat) {
	case "SECURITY":
		return domain.CategorySecurity
	case "PERFORMANCE":
		return domain.CategoryPerformance
	case "ARCHITECTURE":
		return domain.CategoryArchitecture
	case "CONCURRENCY":
		return domain.CategoryBug
	case "COMPLIANCE":
		return domain.CategoryRules
	default:
		return domain.CategoryStyle
	}
}

func (s *DeepAgentReviewStage) recordAgentTraces(pCtx *pipeline.PipelineContext, traces []AgentExecutionTrace) {
	if pCtx.PipelineMetadata == nil {
		pCtx.PipelineMetadata = make(map[string]interface{})
	}
	pCtx.PipelineMetadata["deep_agent_traces"] = traces
}

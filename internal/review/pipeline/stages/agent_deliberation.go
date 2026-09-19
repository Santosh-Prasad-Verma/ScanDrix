package stages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agents"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/review/agentcore"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/orchestrator"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/rules"
	sandboxcontracts "github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/scandrix/backend/pkg/models"
)

// AgentDeliberationOption configures optional behavior for AgentDeliberationStage.
type AgentDeliberationOption func(*AgentDeliberationStage)

// WithAgentRunner sets the agent harness runner for core agent loops.
func WithAgentRunner(runner contracts.AgentRunner) AgentDeliberationOption {
	return func(s *AgentDeliberationStage) {
		s.agentRunner = runner
	}
}

// WithEngineMode sets the execution mode ("legacy" | "core" | "finder" | "orchestrator").
func WithEngineMode(mode string) AgentDeliberationOption {
	return func(s *AgentDeliberationStage) {
		s.engineMode = mode
	}
}

// WithOrchestratorService sets the ReviewOrchestratorService for multi-agent deliberation.
func WithOrchestratorService(svc *orchestrator.ReviewOrchestratorService) AgentDeliberationOption {
	return func(s *AgentDeliberationStage) {
		s.orchestratorService = svc
	}
}

// AgentDeliberationStage performs multi-turn AI reasoning on complex logic diffs.
type AgentDeliberationStage struct {
	reviewer            *agents.AutonomousReviewer
	agentRunner         contracts.AgentRunner
	orchestratorService *orchestrator.ReviewOrchestratorService
	engineMode          string
}

func NewAgentDeliberationStage(evaluator *rules.Evaluator, opts ...AgentDeliberationOption) *AgentDeliberationStage {
	stage := &AgentDeliberationStage{
		reviewer: agents.NewAutonomousReviewer(evaluator),
	}
	for _, opt := range opts {
		opt(stage)
	}
	return stage
}

func (s *AgentDeliberationStage) Name() string {
	return "agent_deliberation"
}

func (s *AgentDeliberationStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if len(pCtx.FilteredPatches) == 0 {
		pCtx.AllFindings = append(pCtx.StaticFindings, pCtx.AgentFindings...)
		return nil
	}

	mode := s.engineMode
	if env := os.Getenv("SCANDRIX_REVIEW_AGENT_ENGINE"); env != "" {
		mode = env
	}

	// 1. If orchestratorService is configured or mode is "orchestrator", execute multi-agent orchestrator
	if s.orchestratorService != nil || mode == "orchestrator" {
		s.executeOrchestratorReview(ctx, pCtx)
	} else if (mode == "core" || mode == "finder") && s.agentRunner != nil {
		s.executeCoreAgentLoop(ctx, pCtx)
	} else if s.reviewer != nil {
		s.executeLegacyReview(ctx, pCtx)
	}

	// Combine static findings and agent findings into AllFindings
	pCtx.AllFindings = append(pCtx.StaticFindings, pCtx.AgentFindings...)
	return nil
}

func (s *AgentDeliberationStage) executeCoreAgentLoop(ctx context.Context, pCtx *pipeline.PipelineContext) {
	var changedFiles []agentcore.ChangedFile
	for _, p := range pCtx.FilteredPatches {
		filename := p.NewPath
		if filename == "" {
			filename = p.OldPath
		}
		var hunks []agentcore.DiffHunk
		for _, h := range p.Hunks {
			hunks = append(hunks, agentcore.DiffHunk{
				OldStart: h.OldStart,
				OldLines: h.OldLines,
				NewStart: h.NewStart,
				NewLines: h.NewLines,
				Header:   h.Header,
			})
		}
		changedFiles = append(changedFiles, agentcore.ChangedFile{
			Filename:  filename,
			Hunks:     hunks,
			Additions: p.Additions,
			Deletions: p.Deletions,
		})
	}

	var repoFS agentcore.RepositoryFS = newPatchRepositoryFS(pCtx.FilteredPatches)
	if pCtx.SandboxHandle != nil && pCtx.SandboxHandle.RemoteCommands() != nil {
		repoFS = newSandboxRepositoryFS(pCtx.SandboxHandle, repoFS)
	}

	userPrompt := fmt.Sprintf("Review pull request #%d: %s\nAuthor: %s", pCtx.PullNumber, pCtx.Title, pCtx.Author)
	if pCtx.PipelineMetadata != nil {
		if cpPrompt, ok := pCtx.PipelineMetadata["context_pack_prompt"].(string); ok && cpPrompt != "" {
			userPrompt = userPrompt + "\n\n" + cpPrompt
		}
		if docPrompt, ok := pCtx.PipelineMetadata["documentation_prompt"].(string); ok && docPrompt != "" {
			userPrompt = userPrompt + "\n\n" + docPrompt
		}
	}

	input := agentcore.ReviewAgentInput{
		PRNumber:       pCtx.PullNumber,
		RepositoryName: pCtx.RepoNamespace,
		AgentName:      "generalist",
		SystemPrompt:   "You are an expert enterprise code review agent. Investigate code changes and report verified bugs, security issues, and performance problems.",
		UserPrompt:     userPrompt,
		ChangedFiles:   changedFiles,
		FS:             repoFS,
		HeavyMode:      false,
		EnableVerify:   true,
	}

	toolCtx := contracts.ToolContext{
		RunID:   fmt.Sprintf("review-%s-%d", pCtx.ReviewID.String(), time.Now().UnixNano()),
		Context: ctx,
		Services: map[string]any{
			"workspace_id":  pCtx.WorkspaceID.String(),
			"review_id":     pCtx.ReviewID.String(),
			"repository_id": pCtx.RepositoryID.String(),
		},
	}

	output, err := agentcore.RunAgentLoopViaCore(ctx, s.agentRunner, input, toolCtx)
	if err == nil && output != nil {
		for _, sug := range output.VerifiedFindings {
			pCtx.AgentFindings = append(pCtx.AgentFindings, mapFinderSuggestionToCodeFinding(sug, pCtx))
		}
		pCtx.AddMetric("agent_raw_findings", output.Duration, true, nil, output.RawFindingsCount)
		pCtx.AddMetric("agent_verified_findings", output.Duration, true, nil, len(output.VerifiedFindings))
		pCtx.AddMetric("agent_dropped_findings", output.Duration, true, nil, output.DroppedFindings)
	} else if err != nil {
		classified := llm.ClassifyLLMError(err, 0)
		pCtx.LastReviewError = &pipeline.ReviewErrorInfo{
			Category:        string(classified.Category),
			Provider:        classified.Provider,
			Model:           classified.Model,
			HTTPStatus:      classified.HTTPStatus,
			FriendlyMessage: classified.FriendlyMessage,
			ProviderMessage: classified.ProviderMessage,
			AgentName:       "core_deliberation_agent",
			OccurredAt:      time.Now().UTC(),
		}
	}
}

func (s *AgentDeliberationStage) executeLegacyReview(ctx context.Context, pCtx *pipeline.PipelineContext) {
	findings, thoughts, err := s.reviewer.ExecuteAgenticReview(ctx, pCtx.ReviewID, pCtx.WorkspaceID, pCtx.FilteredPatches)
	if err == nil {
		pCtx.AgentFindings = findings
		pCtx.AddMetric("agent_thoughts", 0, true, nil, len(thoughts))
	} else {
		classified := llm.ClassifyLLMError(err, 0)
		pCtx.LastReviewError = &pipeline.ReviewErrorInfo{
			Category:        string(classified.Category),
			Provider:        classified.Provider,
			Model:           classified.Model,
			HTTPStatus:      classified.HTTPStatus,
			FriendlyMessage: classified.FriendlyMessage,
			ProviderMessage: classified.ProviderMessage,
			AgentName:       "deliberation_agent",
			OccurredAt:      time.Now().UTC(),
		}
	}
}

func (s *AgentDeliberationStage) executeOrchestratorReview(ctx context.Context, pCtx *pipeline.PipelineContext) {
	if s.orchestratorService == nil {
		s.executeLegacyReview(ctx, pCtx)
		return
	}

	var changedFiles []orchestrator.ChangedFile
	for _, p := range pCtx.FilteredPatches {
		filename := p.NewPath
		if filename == "" {
			filename = p.OldPath
		}
		var patchSB strings.Builder
		patchSB.WriteString(fmt.Sprintf("--- a/%s\n+++ b/%s\n", p.OldPath, p.NewPath))
		for _, h := range p.Hunks {
			header := h.Header
			if !strings.HasPrefix(header, "@@") {
				header = fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
			}
			patchSB.WriteString(header + "\n")
			for _, l := range h.Lines {
				lineContent := l.Content
				switch l.Type {
				case diff.LineAddition:
					if !strings.HasPrefix(lineContent, "+") {
						lineContent = "+" + lineContent
					}
				case diff.LineDeletion:
					if !strings.HasPrefix(lineContent, "-") {
						lineContent = "-" + lineContent
					}
				case diff.LineContext:
					if !strings.HasPrefix(lineContent, " ") {
						lineContent = " " + lineContent
					}
				}
				patchSB.WriteString(lineContent + "\n")
			}
		}
		changedFiles = append(changedFiles, orchestrator.ChangedFile{
			Filename:    filename,
			OldFilename: p.OldPath,
			Additions:   p.Additions,
			Deletions:   p.Deletions,
			Patch:       patchSB.String(),
		})
	}

	var drixyRules []orchestrator.DrixyRule
	for _, r := range pCtx.ActiveRules {
		globs := []string{}
		if r.PathPattern != "" {
			globs = []string{r.PathPattern}
		}
		drixyRules = append(drixyRules, orchestrator.DrixyRule{
			ID:          r.ID,
			OrgID:       pCtx.WorkspaceID,
			Name:        r.Name,
			Description: r.Description,
			Prompt:      r.Description,
			Severity:    r.Severity,
			Scope:       "file",
			PathGlobs:   globs,
			IsActive:    true,
		})
	}

	opts := orchestrator.DefaultReviewOptions()
	rOpts := pCtx.ResolvedConfig.ReviewOptions
	if rOpts.Bug || rOpts.Security || rOpts.Performance || rOpts.Architecture || rOpts.BusinessLogic {
		opts.Bug = rOpts.Bug
		opts.Security = rOpts.Security
		opts.Performance = rOpts.Performance
		opts.Architecture = rOpts.Architecture
		opts.BusinessLogic = rOpts.BusinessLogic
	}

	contextPrompt := ""
	if pCtx.PipelineMetadata != nil {
		if cpPrompt, ok := pCtx.PipelineMetadata["context_pack_prompt"].(string); ok && cpPrompt != "" {
			contextPrompt = cpPrompt
		}
		if docPrompt, ok := pCtx.PipelineMetadata["documentation_prompt"].(string); ok && docPrompt != "" {
			if contextPrompt != "" {
				contextPrompt = contextPrompt + "\n\n" + docPrompt
			} else {
				contextPrompt = docPrompt
			}
		}
	}
	if contextPrompt == "" && pCtx.ExternalContext != nil {
		contextPrompt = fmt.Sprintf("Linked Issue: %s - %s\n%s", pCtx.ExternalContext.IssueKey, pCtx.ExternalContext.Title, pCtx.ExternalContext.Description)
	}

	input := orchestrator.ReviewAgentInput{
		PRNumber:        pCtx.PullNumber,
		Title:           pCtx.Title,
		Description:     pCtx.Description,
		RepositoryName:  pCtx.RepoNamespace,
		BaseSHA:         pCtx.BaseSHA,
		HeadSHA:         pCtx.HeadSHA,
		AuthorUsername:  pCtx.Author,
		ChangedFiles:    changedFiles,
		ReviewOptions:   opts,
		DrixyRules:      drixyRules,
		WorkspaceID:     pCtx.WorkspaceID,
		RepositoryID:    pCtx.RepositoryID,
		ExternalContext: contextPrompt,
	}

	out, err := s.orchestratorService.Execute(ctx, input)
	if err == nil && out != nil {
		pCtx.PRSummaryBody = out.Summary
		for _, f := range out.Findings {
			pCtx.AgentFindings = append(pCtx.AgentFindings, mapOrchestratorFindingToCodeFinding(f, pCtx))
		}
		pCtx.AddMetric("orchestrator_findings", time.Duration(out.TotalDurationMs)*time.Millisecond, true, nil, len(out.Findings))
		pCtx.AddMetric("orchestrator_tokens", 0, true, nil, out.TotalTokensConsumed)
		if len(out.Failures) > 0 {
			pCtx.AddMetric("orchestrator_failures", 0, false, nil, len(out.Failures))
		}
		if len(out.Incomplete) > 0 {
			pCtx.AddMetric("orchestrator_incomplete", 0, true, nil, len(out.Incomplete))
		}
	} else if err != nil {
		classified := llm.ClassifyLLMError(err, 0)
		pCtx.LastReviewError = &pipeline.ReviewErrorInfo{
			Category:        string(classified.Category),
			Provider:        classified.Provider,
			Model:           classified.Model,
			HTTPStatus:      classified.HTTPStatus,
			FriendlyMessage: classified.FriendlyMessage,
			ProviderMessage: classified.ProviderMessage,
			AgentName:       "review_orchestrator_service",
			OccurredAt:      time.Now().UTC(),
		}
	}
}

func mapOrchestratorFindingToCodeFinding(f orchestrator.AgentFinding, pCtx *pipeline.PipelineContext) models.CodeFinding {
	startLine := f.StartLine
	if startLine <= 0 {
		startLine = 1
	}
	endLine := f.EndLine
	if endLine < startLine {
		endLine = startLine
	}

	fp := f.Fingerprint
	if fp == "" {
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%s", f.FilePath, startLine, endLine, f.Title)))
		fp = hex.EncodeToString(hash[:16])
	}

	return models.CodeFinding{
		ID:            f.ID,
		ReviewID:      pCtx.ReviewID,
		WorkspaceID:   pCtx.WorkspaceID,
		FilePath:      f.FilePath,
		StartLine:     startLine,
		EndLine:       endLine,
		Severity:      f.Severity,
		Category:      f.Category,
		Title:         f.Title,
		Description:   f.Description,
		Remediation:   f.Remediation,
		SuggestedDiff: f.SuggestedDiff,
		Fingerprint:   fp,
		CreatedAt:     time.Now().UTC(),
	}
}

func mapFinderSuggestionToCodeFinding(s agentcore.FinderSuggestion, pCtx *pipeline.PipelineContext) models.CodeFinding {
	sev := models.SeverityMedium
	switch strings.ToLower(s.Severity) {
	case "critical":
		sev = models.SeverityCritical
	case "high":
		sev = models.SeverityHigh
	case "medium":
		sev = models.SeverityMedium
	case "low":
		sev = models.SeverityLow
	case "info":
		sev = models.SeverityInfo
	}

	title := s.OneSentenceSummary
	if title == "" {
		title = fmt.Sprintf("[%s] %s", strings.ToUpper(s.Label), s.RelevantFile)
	}

	startLine := s.RelevantLinesStart
	if startLine <= 0 {
		startLine = 1
	}
	endLine := s.RelevantLinesEnd
	if endLine < startLine {
		endLine = startLine
	}

	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%s", s.RelevantFile, startLine, endLine, title)))
	fp := hex.EncodeToString(hash[:16])

	return models.CodeFinding{
		ID:            uuid.New(),
		ReviewID:      pCtx.ReviewID,
		WorkspaceID:   pCtx.WorkspaceID,
		FilePath:      s.RelevantFile,
		StartLine:     startLine,
		EndLine:       endLine,
		Severity:      sev,
		Category:      s.Label,
		Title:         title,
		Description:   s.SuggestionContent,
		Remediation:   s.ImprovedCode,
		SuggestedDiff: s.ImprovedCode,
		Fingerprint:   fp,
		CreatedAt:     time.Now().UTC(),
	}
}

type patchRepositoryFS struct {
	patches []*diff.FilePatch
}

func newPatchRepositoryFS(patches []*diff.FilePatch) *patchRepositoryFS {
	return &patchRepositoryFS{patches: patches}
}

func (fs *patchRepositoryFS) ReadFile(ctx context.Context, path string, startLine, endLine int) (string, error) {
	for _, p := range fs.patches {
		if p.NewPath == path || p.OldPath == path {
			var lines []string
			for _, h := range p.Hunks {
				for _, l := range h.Lines {
					if l.Type != diff.LineDeletion {
						lines = append(lines, l.Content)
					}
				}
			}
			if len(lines) == 0 {
				return "", nil
			}
			if startLine > 0 && endLine >= startLine {
				startIdx := startLine - 1
				if startIdx >= len(lines) {
					return "", nil
				}
				endIdx := endLine
				if endIdx > len(lines) {
					endIdx = len(lines)
				}
				return strings.Join(lines[startIdx:endIdx], "\n"), nil
			}
			return strings.Join(lines, "\n"), nil
		}
	}
	return "", fmt.Errorf("file not found in patch set: %s", path)
}

func (fs *patchRepositoryFS) ListDir(ctx context.Context, path string) ([]string, error) {
	seen := make(map[string]bool)
	var files []string
	cleanPath := strings.Trim(path, "./")
	for _, p := range fs.patches {
		name := p.NewPath
		if name == "" {
			name = p.OldPath
		}
		if cleanPath == "" || strings.HasPrefix(name, cleanPath) {
			if !seen[name] {
				seen[name] = true
				files = append(files, name)
			}
		}
	}
	return files, nil
}

func (fs *patchRepositoryFS) Grep(ctx context.Context, query, path string) (string, error) {
	var results []string
	for _, p := range fs.patches {
		if path != "" && p.NewPath != path && p.OldPath != path {
			continue
		}
		for _, h := range p.Hunks {
			for _, l := range h.Lines {
				if strings.Contains(l.Content, query) {
					results = append(results, fmt.Sprintf("%s:%d: %s", p.NewPath, l.NewLineNo, l.Content))
				}
			}
		}
	}
	if len(results) == 0 {
		return "No matches found.", nil
	}
	return strings.Join(results, "\n"), nil
}

func (fs *patchRepositoryFS) GetCallers(ctx context.Context, symbol, file string) (string, error) {
	return fs.Grep(ctx, symbol+"(", "")
}

func (fs *patchRepositoryFS) GitDiff(ctx context.Context, path string) (string, error) {
	var sb strings.Builder
	for _, p := range fs.patches {
		if path != "" && p.NewPath != path && p.OldPath != path {
			continue
		}
		sb.WriteString(fmt.Sprintf("--- a/%s\n+++ b/%s\n", p.OldPath, p.NewPath))
		for _, h := range p.Hunks {
			sb.WriteString(h.Header + "\n")
			for _, l := range h.Lines {
				sb.WriteString(l.Content + "\n")
			}
		}
	}
	return sb.String(), nil
}

// sandboxRepositoryFS provides agent access to the live cloned repository inside the SandboxInstance,
// delegating to remoteCommands when present and gracefully falling back to diff patches if commands error.
type sandboxRepositoryFS struct {
	sandbox  sandboxcontracts.SandboxInstance
	fallback agentcore.RepositoryFS
}

func newSandboxRepositoryFS(sandbox sandboxcontracts.SandboxInstance, fallback agentcore.RepositoryFS) *sandboxRepositoryFS {
	return &sandboxRepositoryFS{
		sandbox:  sandbox,
		fallback: fallback,
	}
}

func (s *sandboxRepositoryFS) ReadFile(ctx context.Context, path string, startLine, endLine int) (string, error) {
	if s.sandbox == nil || s.sandbox.RemoteCommands() == nil {
		return s.fallback.ReadFile(ctx, path, startLine, endLine)
	}

	content, err := s.sandbox.RemoteCommands().Read(ctx, path, startLine, endLine)
	if err != nil {
		return s.fallback.ReadFile(ctx, path, startLine, endLine)
	}

	return content, nil
}

func (s *sandboxRepositoryFS) ListDir(ctx context.Context, path string) ([]string, error) {
	if s.sandbox == nil || s.sandbox.RemoteCommands() == nil {
		return s.fallback.ListDir(ctx, path)
	}

	raw, err := s.sandbox.RemoteCommands().ListDir(ctx, path, 3)
	if err != nil {
		return s.fallback.ListDir(ctx, path)
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var files []string
	for _, f := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(f)
		if trimmed != "" {
			files = append(files, trimmed)
		}
	}
	return files, nil
}

func (s *sandboxRepositoryFS) Grep(ctx context.Context, query, path string) (string, error) {
	if s.sandbox == nil || s.sandbox.RemoteCommands() == nil {
		return s.fallback.Grep(ctx, query, path)
	}

	out, err := s.sandbox.RemoteCommands().Grep(ctx, query, path, "")
	if err != nil || strings.TrimSpace(out) == "" {
		return s.fallback.Grep(ctx, query, path)
	}

	return out, nil
}

func (s *sandboxRepositoryFS) GetCallers(ctx context.Context, symbol, file string) (string, error) {
	return s.Grep(ctx, symbol+"(", "")
}

func (s *sandboxRepositoryFS) GitDiff(ctx context.Context, path string) (string, error) {
	if s.sandbox == nil || s.sandbox.RemoteCommands() == nil {
		return s.fallback.GitDiff(ctx, path)
	}

	cmd := "git diff HEAD~1"
	if path != "" {
		cmd += " -- " + sandboxcontracts.ShSingleQuote(path)
	}

	res, err := s.sandbox.RemoteCommands().Exec(ctx, cmd)
	if err != nil || res == nil || res.ExitCode != 0 || strings.TrimSpace(res.Stdout) == "" {
		return s.fallback.GitDiff(ctx, path)
	}

	return res.Stdout, nil
}

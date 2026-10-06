package review

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/codeanalysis/ast"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/integrations/github"
	"github.com/scandrix/backend/internal/integrations/pm"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/embedding"
	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/internal/provenance/intoto"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/feedback"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/review/pipeline/stages"
	"github.com/scandrix/backend/internal/review/priority"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/scandrix/backend/internal/sandbox/syntax"
	"github.com/scandrix/backend/internal/storage"
	"github.com/scandrix/backend/pkg/crypto"
	"github.com/scandrix/backend/pkg/models"
)

// 1. REVIEW ORCHESTRATOR MODEL & SCM CONTRACTS (Pipeline state & dependencies)

// SCMPublisher publishes inline reviews and updates commit checks on SCM providers.
type SCMPublisher interface {
	SubmitPullRequestReview(ctx context.Context, owner, repo string, pullNumber int, submission github.PullReviewSubmission) error
	UpdateCheckRun(ctx context.Context, owner, repo string, checkRunID int64, req github.UpdateCheckRunRequest) error
}

// Orchestrator coordinates the end-to-end pull request code review pipeline.
type Orchestrator struct {
	repo            *database.Repository
	llmGateway      *llm.Gateway
	artifactClient  *storage.ArtifactClient
	rulesEvaluator  *rules.Evaluator
	scmPublisher    SCMPublisher
	platformAdapter platform.SCMAdapter
	attestor        *intoto.ProvenanceAttestor
	autoTicketMgr   *pm.AutoTicketManager
	streamHub       *StreamHub
	feedbackService *feedback.SemanticFeedbackService
	sandboxLeaseMgr contracts.ISandboxLeaseManager
	settingsReader  repositorySettingsReader
}

// 2. DEPENDENCY INJECTION BINDINGS (Streaming hub, adapters & attestors)

// NewOrchestrator initializes the review orchestration engine.
func NewOrchestrator(
	repo *database.Repository,
	llmGateway *llm.Gateway,
	artifactClient *storage.ArtifactClient,
	rulesEvaluator *rules.Evaluator,
) *Orchestrator {
	var fbService *feedback.SemanticFeedbackService
	if repo != nil {
		fbService = feedback.NewSemanticFeedbackService(repo, embedding.NewDeterministicSemanticEmbedder())
	}
	var sr repositorySettingsReader
	if repo != nil {
		sr = repo
	}
	return &Orchestrator{
		repo:            repo,
		llmGateway:      llmGateway,
		artifactClient:  artifactClient,
		rulesEvaluator:  rulesEvaluator,
		feedbackService: fbService,
		settingsReader:  sr,
	}
}

// SetFeedbackService configures a custom semantic feedback learning service.
func (o *Orchestrator) SetFeedbackService(fb *feedback.SemanticFeedbackService) {
	o.feedbackService = fb
}

// SetStreamHub binds an active SSE streaming hub to broadcast review lifecycle events.
func (o *Orchestrator) SetStreamHub(hub *StreamHub) {
	o.streamHub = hub
}

// SetSCMPublisher binds an active SCM client for publishing reviews back to GitHub/GitLab.
func (o *Orchestrator) SetSCMPublisher(pub SCMPublisher) {
	o.scmPublisher = pub
}

// SetSCMAdapter binds a unified platform adapter across GitHub, GitLab, Bitbucket, Azure DevOps, and Forgejo.
func (o *Orchestrator) SetSCMAdapter(adapter platform.SCMAdapter) {
	o.platformAdapter = adapter
}

// SetProvenanceAttestor attaches an in-toto provenance signer.
func (o *Orchestrator) SetProvenanceAttestor(attestor *intoto.ProvenanceAttestor) {
	o.attestor = attestor
}

// LLMGateway returns the orchestrator's LLM gateway instance.
func (o *Orchestrator) LLMGateway() *llm.Gateway {
	if o == nil {
		return nil
	}
	return o.llmGateway
}

// SetAutoTicketManager attaches a project management auto-ticketing manager.
func (o *Orchestrator) SetAutoTicketManager(mgr *pm.AutoTicketManager) {
	o.autoTicketMgr = mgr
}

// SetSandboxLeaseManager attaches the sandbox lease manager for isolated VM/container reviews.
func (o *Orchestrator) SetSandboxLeaseManager(lm contracts.ISandboxLeaseManager) {
	o.sandboxLeaseMgr = lm
}

// SandboxLeaseManager returns the orchestrator's sandbox lease manager instance.
func (o *Orchestrator) SandboxLeaseManager() contracts.ISandboxLeaseManager {
	if o == nil {
		return nil
	}
	return o.sandboxLeaseMgr
}

// BuildPipelineEngine constructs a fully configured 10-stage review pipeline.
func (o *Orchestrator) BuildPipelineEngine(evaluator *rules.Evaluator) *pipeline.PipelineEngine {
	if evaluator == nil {
		evaluator = o.rulesEvaluator
	}
	return pipeline.NewPipelineEngine(
		stages.NewPrerequisitesStage(),
		stages.NewExternalContextStage(),
		stages.NewFileFilterStage(),
		stages.NewASTAnalysisStage(evaluator),
		stages.NewCreateSandboxStage(o.sandboxLeaseMgr),
		stages.NewAgentDeliberationStage(evaluator),
		stages.NewSuggestionValidatorStage(),
		stages.NewSemanticSuppressorStage(o.feedbackService),
		stages.NewHunkFormatterStage(),
		stages.NewPRSummaryStage(),
		stages.NewSCMPublisherStage(500*time.Millisecond),
	)
}

// ProcessReviewWithPipeline executes the complete 10-stage pipeline with context lifecycle tracking.
func (o *Orchestrator) ProcessReviewWithPipeline(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	engine := o.BuildPipelineEngine(o.rulesEvaluator)
	return engine.Execute(ctx, pCtx)
}

// 3. EXECUTION TASK SCHEMA (Job parameters & raw diff payload)

// ExecutionTask represents the input job submitted to the review pipeline.
type ExecutionTask struct {
	ReviewID      uuid.UUID
	WorkspaceID   uuid.UUID
	RepositoryID  uuid.UUID
	RepoNamespace string
	PullNumber    int
	Title         string
	HeadSHA       string
	BaseSHA       string
	BaseBranch    string
	Author        string
	RawDiff       string
	CustomRules   string
	CheckRunID    int64
	Manual        bool
	Provider      models.SCMProvider
	SCMAdapter    platform.SCMAdapter
}

// 4. REVIEW LIFECYCLE & DIFF INGESTION (Stage tracking & patch parsing)

// ProcessReview executes all analysis stages, records findings, and uploads scan artifacts.
func (o *Orchestrator) ProcessReview(ctx context.Context, task ExecutionTask) error {
	var settings models.RepositoryReviewSettings
	if o.settingsReader != nil {
		s, err := o.settingsReader.GetRepositoryReviewSettings(ctx, task.WorkspaceID, task.RepositoryID)
		if err != nil {
			return err
		}
		settings = s
		reason, err := repositoryPolicyReason(settings, task)
		if err != nil {
			return err
		}
		if reason != "" {
			slog.Info("Review skipped per repository policy", "reason", reason, "review_id", task.ReviewID)
			if o.repo != nil {
				now := time.Now().UTC()
				_ = o.repo.CreateReview(ctx, &models.PullRequestReview{
					ID:             task.ReviewID,
					WorkspaceID:    task.WorkspaceID,
					RepositoryID:   task.RepositoryID,
					PullNumber:     task.PullNumber,
					Title:          task.Title,
					HeadSHA:        task.HeadSHA,
					BaseSHA:        task.BaseSHA,
					AuthorUsername: task.Author,
					State:          models.ReviewStateSkipped,
					CompletedAt:    &now,
				})
			}
			return nil
		}
		task.RawDiff = filterRepositoryDiff(task.RawDiff, settings.IgnoredPaths)
		if len(strings.TrimSpace(task.RawDiff)) == 0 {
			slog.Info("Review skipped: all diff content filtered by repository policy", "review_id", task.ReviewID)
			return nil
		}
	}

	slog.Info("Starting pull request review execution",
		"review_id", task.ReviewID,
		"repo", task.RepoNamespace,
		"pr", task.PullNumber,
	)

	// Update state to PROCESSING (and ensure review record exists)
	if o.repo != nil {
		existing, _ := o.repo.GetReview(ctx, task.WorkspaceID, task.ReviewID)
		if existing == nil {
			// This used to discard the error with `_ =`. A review that could not
			// be persisted still ran to completion and logged success, so the
			// failure was invisible: no review row, no signal. Fail loudly
			// instead, and name the likely cause.
			createErr := o.repo.CreateReview(ctx, &models.PullRequestReview{
				ID:             task.ReviewID,
				WorkspaceID:    task.WorkspaceID,
				RepositoryID:   task.RepositoryID,
				PullNumber:     task.PullNumber,
				Title:          task.Title,
				HeadSHA:        task.HeadSHA,
				BaseSHA:        task.BaseSHA,
				AuthorUsername: task.Author,
				State:          models.ReviewStateProcessing,
				FindingsCount:  0,
			})
			if createErr != nil {
				slog.Error("Failed to persist review record; aborting so this is never silent",
					"review_id", task.ReviewID,
					"workspace_id", task.WorkspaceID,
					"repository_id", task.RepositoryID,
					"repo", task.RepoNamespace,
					"error", createErr)
				return fmt.Errorf("failed to persist review record: %w", createErr)
			}
		} else {
			_ = o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateProcessing, 0)
		}
	}

	// Step 1: Parse git unified diff stream
	if o.streamHub != nil {
		o.streamHub.Broadcast(task.ReviewID, "stage_started", "PARSING_DIFF", map[string]string{"title": task.Title})
	}
	patches, err := diff.ParseUnifiedDiff(strings.NewReader(task.RawDiff))
	if err != nil {
		slog.Error("Failed to parse unified diff", "error", err)
		if o.repo != nil {
			_ = o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateFailed, 0)
		}
		if o.streamHub != nil {
			o.streamHub.Broadcast(task.ReviewID, "review_failed", "FAILED", map[string]string{"error": err.Error()})
		}
		return fmt.Errorf("diff parse failed: %w", err)
	}

	var allFindings []models.CodeFinding

	// Ephemeral microVM / container sandbox lease acquisition
	var leaseID string
	var sandboxHandle contracts.SandboxInstance
	if o.sandboxLeaseMgr != nil && task.PullNumber > 0 && task.RepoNamespace != "" {
		if prKey, err := contracts.BuildPrKey(task.WorkspaceID.String(), task.RepositoryID.String(), task.PullNumber); err == nil {
			cloneURL := fmt.Sprintf("https://github.com/%s.git", task.RepoNamespace)
			token := ""
			if o.repo != nil && task.WorkspaceID != uuid.Nil {
				token, _ = o.repo.GetDecryptedIntegrationToken(ctx, task.WorkspaceID, task.Provider)
			}
			cloneParams := contracts.CreateSandboxParams{
				CloneURL:        cloneURL,
				AuthToken:       token,
				Platform:        task.Provider,
				PRNumber:        task.PullNumber,
				CheckoutSHA:     task.HeadSHA,
				UnifiedDiff:     task.RawDiff,
				SandboxMetadata: map[string]string{"stage": "review"},
			}
			if acq, err := o.sandboxLeaseMgr.Acquire(ctx, prKey, "review", 0, &cloneParams); err == nil && acq != nil {
				leaseID = acq.LeaseID
				sandboxHandle = acq.Sandbox
				slog.Info("Acquired sandbox lease for review", "pr_key", prKey, "lease_id", leaseID)
				defer func() {
					if leaseID != "" {
						_ = o.sandboxLeaseMgr.Release(ctx, leaseID, &contracts.ReleaseOptions{
							IdleTimeout: 30 * time.Second,
						})
					} else if sandboxHandle != nil {
						_ = sandboxHandle.Cleanup(ctx)
					}
				}()
			} else if err != nil {
				slog.Warn("Sandbox lease acquisition skipped or failed, continuing review self-contained", "pr_key", prKey, "error", err)
			}
		}
	}

	// 5. STATIC RULES & BLAST-RADIUS CALL GRAPH (AST smells & file prioritization)

	// Step 2: Evaluate deterministic custom workspace rules
	if o.rulesEvaluator != nil {
		if o.streamHub != nil {
			o.streamHub.Broadcast(task.ReviewID, "stage_started", "EVALUATING_RULES", nil)
		}
		ruleFindings := o.rulesEvaluator.EvaluatePatches(task.ReviewID, task.WorkspaceID, patches)
		allFindings = append(allFindings, ruleFindings...)
		slog.Info("Rule evaluation finished", "findings", len(ruleFindings))
	}

	// Step 2.1: Run AST structural complexity and code smell analysis
	complexityAnalyzer := ast.NewASTComplexityAnalyzer()
	for _, patch := range patches {
		if patch.NewPath != "" && len(patch.Hunks) > 0 {
			var patchContent strings.Builder
			for _, h := range patch.Hunks {
				for _, line := range h.Lines {
					if line.Type != diff.LineDeletion {
						patchContent.WriteString(line.Content)
						patchContent.WriteString("\n")
					}
				}
			}
			if report, err := complexityAnalyzer.Analyze(patch.NewPath, patchContent.String()); err == nil && report != nil {
				for _, cf := range report.Findings {
					cf.ReviewID = task.ReviewID
					cf.WorkspaceID = task.WorkspaceID
					if cf.Fingerprint == "" {
						cf.Fingerprint = crypto.FingerprintSHA256(patch.NewPath + ":" + cf.Title + ":" + cf.Category)
					}
					allFindings = append(allFindings, cf)
				}
			}
		}
	}

	// Step 2.2: Blast-Radius Call-Graph Scoring & File Prioritization
	var fileChanges []priority.ScoredFileChange
	patchMap := make(map[string]*diff.FilePatch)
	for _, p := range patches {
		filePath := p.NewPath
		if filePath == "" {
			filePath = p.OldPath
		}
		status := priority.StatusModified
		if p.IsNew {
			status = priority.StatusAdded
		} else if p.IsDeleted {
			status = priority.StatusRemoved
		} else if p.OldPath != "" && p.NewPath != "" && p.OldPath != p.NewPath {
			status = priority.StatusRenamed
		}

		fileChanges = append(fileChanges, priority.ScoredFileChange{
			FilePath:  filePath,
			Status:    status,
			Additions: p.Additions,
			Deletions: p.Deletions,
		})
		patchMap[filePath] = p
	}

	// Extract import/call relationships across changed files to construct call graph edges
	var graphEdges []priority.GraphEdge
	for _, f1 := range fileChanges {
		for _, f2 := range fileChanges {
			if f1.FilePath != f2.FilePath {
				base2 := f2.FilePath
				if idx := strings.LastIndex(base2, "/"); idx >= 0 {
					base2 = base2[idx+1:]
				}
				if extIdx := strings.LastIndex(base2, "."); extIdx > 0 {
					base2 = base2[:extIdx]
				}
				if p, ok := patchMap[f1.FilePath]; ok && len(base2) > 2 {
					for _, h := range p.Hunks {
						for _, line := range h.Lines {
							if line.Type != diff.LineDeletion && strings.Contains(line.Content, base2) {
								graphEdges = append(graphEdges, priority.GraphEdge{
									Kind:       priority.EdgeCalls,
									SourceFile: f1.FilePath,
									TargetFile: f2.FilePath,
								})
								break
							}
						}
					}
				}
			}
		}
	}

	scoredFiles := priority.ScoreAndPrioritizeFiles(fileChanges, graphEdges)
	slog.Info("Computed blast-radius call-graph scores for review",
		"review_id", task.ReviewID,
		"files_scored", len(scoredFiles),
	)

	// 6. AI GATEWAY DEEP SEMANTIC SYNTHESIS (Multi-model analysis & BYOK keys)

	// Step 3: Run AI Gateway deep semantic analysis
	aiSynthesisFailed := false
	var aiErrorMsg string
	diffTruncated := false
	const MaxDiffBytesForLLM = 250 * 1024 // 250 KB

	if o.llmGateway != nil && len(task.RawDiff) > 0 {
		if o.streamHub != nil {
			o.streamHub.Broadcast(task.ReviewID, "stage_started", "AI_SYNTHESIS", nil)
		}

		diffForLLM := task.RawDiff
		if len(diffForLLM) > MaxDiffBytesForLLM {
			// Reassemble diff prioritizing high blast-radius files first
			var b strings.Builder
			for _, sf := range scoredFiles {
				if p, ok := patchMap[sf.FilePath]; ok {
					var patchBuf strings.Builder
					patchBuf.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", p.OldPath, p.NewPath))
					for _, h := range p.Hunks {
						patchBuf.WriteString(h.Header + "\n")
						for _, line := range h.Lines {
							prefix := " "
							if line.Type == diff.LineAddition {
								prefix = "+"
							} else if line.Type == diff.LineDeletion {
								prefix = "-"
							}
							patchBuf.WriteString(prefix + line.Content + "\n")
						}
					}
					if b.Len()+patchBuf.Len() > MaxDiffBytesForLLM {
						diffTruncated = true
						break
					}
					b.WriteString(patchBuf.String())
				}
			}
			if b.Len() > 0 {
				diffForLLM = b.String() + "\n\n[... Diff truncated by ScanDrix: prioritized high blast-radius files within 250KB threshold ...]\n"
			} else {
				diffForLLM = diffForLLM[:MaxDiffBytesForLLM] + "\n\n[... Diff truncated by ScanDrix: exceeded 250KB context threshold ...]\n"
				diffTruncated = true
			}
		}

		llmReq := llm.ReviewRequest{
			WorkspaceID:   task.WorkspaceID,
			RepoNamespace: task.RepoNamespace,
			PullTitle:     task.Title,
			DiffContent:   diffForLLM,
			CustomRules:   task.CustomRules,
			Model:         settings.ModelOverride,
		}

		if o.repo != nil {
			if param, err := o.repo.GetOrganizationParameter(ctx, task.WorkspaceID, "byok_config"); err == nil && param != nil {
				if migrated, err := byok.MigrateLegacyToV2(param.ConfigValue); err == nil && migrated != nil && len(migrated.Models) > 0 {
					llmReq.BYOKConfig = migrated
				}
			}
			if llmReq.BYOKConfig == nil {
				if tok, _ := o.repo.GetDecryptedIntegrationToken(ctx, task.WorkspaceID, models.SCMProvider("anthropic")); tok != "" {
					llmReq.BYOKAnthropicKey = tok
				}
				if tok, _ := o.repo.GetDecryptedIntegrationToken(ctx, task.WorkspaceID, models.SCMProvider("openai")); tok != "" {
					llmReq.BYOKOpenAIKey = tok
				}
				if tok, _ := o.repo.GetDecryptedIntegrationToken(ctx, task.WorkspaceID, models.SCMProvider("gemini")); tok != "" {
					llmReq.BYOKGeminiKey = tok
				}
				if tok, _ := o.repo.GetDecryptedIntegrationToken(ctx, task.WorkspaceID, models.SCMProvider("deepseek")); tok != "" {
					llmReq.BYOKDeepSeekKey = tok
				}
				if tok, _ := o.repo.GetDecryptedIntegrationToken(ctx, task.WorkspaceID, models.SCMProvider("openrouter")); tok != "" {
					llmReq.BYOKOpenRouterKey = tok
				}
			}
		}

		aiResp, err := o.llmGateway.AnalyzeDiff(ctx, llmReq)
		if err != nil {
			aiSynthesisFailed = true
			classified := llm.ClassifyLLMError(err, 0)
			aiErrorMsg = classified.BuildReviewErrorMessage()
			slog.Error("AI review synthesis failed", "review_id", task.ReviewID, "error", err, "diagnostics", aiErrorMsg)
			if o.streamHub != nil {
				o.streamHub.Broadcast(task.ReviewID, "stage_failed", "AI_SYNTHESIS", map[string]string{"error": aiErrorMsg})
			}
		} else {
			for _, f := range aiResp.Findings {
				sev := models.SeverityMedium
				switch strings.ToUpper(f.Severity) {
				case "CRITICAL":
					sev = models.SeverityCritical
				case "HIGH":
					sev = models.SeverityHigh
				case "LOW":
					sev = models.SeverityLow
				case "INFO":
					sev = models.SeverityInfo
				}

				allFindings = append(allFindings, models.CodeFinding{
					ID:            uuid.New(),
					ReviewID:      task.ReviewID,
					WorkspaceID:   task.WorkspaceID,
					FilePath:      f.FilePath,
					StartLine:     f.StartLine,
					EndLine:       f.EndLine,
					Severity:      sev,
					Category:      f.Category,
					Title:         f.Title,
					Description:   f.Description,
					Remediation:   f.Remediation,
					SuggestedDiff: f.SuggestedDiff,
					Fingerprint:   crypto.FingerprintSHA256(f.FilePath + ":" + f.Title + ":" + f.Category),
				})
			}
			slog.Info("AI synthesis completed", "ai_findings", len(aiResp.Findings))
		}
	}

	// 7. ARTIFACT ARCHIVAL & RLS FINDINGS PERSISTENCE (Memory suppression & state updates)

	// Step 4: Archive diff and audit report to Appwrite Storage
	if o.artifactClient != nil {
		artifactID := uuid.New().String()
		_, err := o.artifactClient.UploadArtifact(
			ctx,
			"review-artifacts",
			artifactID,
			fmt.Sprintf("pr-%d.diff", task.PullNumber),
			bytes.NewReader([]byte(task.RawDiff)),
		)
		if err != nil {
			slog.Warn("Appwrite storage artifact upload deferred", "error", err)
		}
	}

	// Step 5: Two-Tier Semantic Memory & Feedback Suppressor (Fingerprint + pgvector Cosine Distance)
	if o.repo != nil {
		var activeFindings []models.CodeFinding
		for _, f := range allFindings {
			isSuppressed := false
			if o.feedbackService != nil {
				decision := o.feedbackService.CheckSuppression(ctx, task.WorkspaceID, &f)
				if decision.ShouldSuppress {
					slog.Info("Suppressing code review finding based on developer feedback memory",
						"workspace_id", task.WorkspaceID,
						"fingerprint", f.Fingerprint,
						"title", f.Title,
						"is_exact", decision.IsExactMatch,
						"similarity", decision.SimilarityScore,
						"reason", decision.Reason,
					)
					isSuppressed = true
				}
			} else {
				if mem, err := o.repo.GetSecurityMemoryByFingerprint(ctx, task.WorkspaceID, f.Fingerprint); err == nil && mem != nil {
					slog.Info("Suppressing repeat finding matching false-positive fingerprint memory", "fingerprint", f.Fingerprint, "title", f.Title)
					isSuppressed = true
				}
			}
			if !isSuppressed {
				activeFindings = append(activeFindings, f)
			}
		}
		allFindings = activeFindings

		// Multi-language syntax verification on proposed suggestions
		syntaxValidator := syntax.NewSandboxSyntaxValidator()
		for i := range allFindings {
			if allFindings[i].SuggestedDiff != "" {
				res := syntaxValidator.ValidateSuggestion(allFindings[i].FilePath, allFindings[i].SuggestedDiff)
				if !res.IsValid {
					slog.Debug("Suggestion syntax verification noted issues", "file", allFindings[i].FilePath, "error", res.ErrorMessage)
				}
			}
		}

		if err := o.repo.BatchInsertFindings(ctx, task.WorkspaceID, allFindings); err != nil {
			slog.Error("Failed saving review findings", "error", err)
			_ = o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateFailed, 0)
			return fmt.Errorf("failed persisting findings: %w", err)
		}

		// Step 6: Transition review state to COMPLETED or FAILED
		finalState := models.ReviewStateCompleted
		if aiSynthesisFailed {
			finalState = models.ReviewStateFailed
		}
		if err := o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, finalState, len(allFindings)); err != nil {
			return fmt.Errorf("failed finalizing review: %w", err)
		}

		if o.streamHub != nil {
			event := "review_completed"
			status := "COMPLETED"
			if aiSynthesisFailed {
				event = "review_failed"
				status = "FAILED"
			}
			o.streamHub.Broadcast(task.ReviewID, event, status, map[string]any{"total_findings": len(allFindings)})
		}

		// Step 6.1: Evaluate findings for automated PM ticket generation (Jira, Linear, Azure Boards)
		if o.autoTicketMgr != nil && len(allFindings) > 0 {
			prURL := ""
			if task.RepoNamespace != "" && task.PullNumber > 0 {
				prURL = fmt.Sprintf("https://github.com/%s/pull/%d", task.RepoNamespace, task.PullNumber)
			}
			ticketCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			_, _ = o.autoTicketMgr.ProcessFindingsForAutoTicket(ticketCtx, task.WorkspaceID, task.RepositoryID, allFindings, prURL)
			cancel()
		}
	}

	hasCriticalOrHigh := false
	criticalCount := 0
	highCount := 0
	for _, f := range allFindings {
		if f.Severity == models.SeverityCritical {
			hasCriticalOrHigh = true
			criticalCount++
		} else if f.Severity == models.SeverityHigh {
			hasCriticalOrHigh = true
			highCount++
		}
	}

	// Mid-flight commit race check: verify this review hasn't been superseded by a newer push
	if o.repo != nil && task.WorkspaceID != uuid.Nil && task.ReviewID != uuid.Nil {
		if isSuperseded, err := o.repo.HasNewerReviewForPR(ctx, task.WorkspaceID, task.ReviewID); err == nil && isSuperseded {
			slog.Warn("Review superseded by newer commit push, skipping comment dispatch to avoid PR race condition",
				"review_id", task.ReviewID,
				"repo", task.RepoNamespace,
				"pr", task.PullNumber,
			)
			_ = o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateCompleted, len(allFindings))
			return nil
		}
	}

	// 8. SCM INLINE REVIEW & CHECK-RUN PUBLISHING (GitHub / GitLab feedback loops)

	// Step 7: Dispatch Inline Review Comments and Check Run update to SCM Provider
	adapter := o.platformAdapter
	if adapter == nil && task.SCMAdapter != nil {
		adapter = task.SCMAdapter
	}

	conclusion := platform.ConclusionSuccess
	commitStatusState := platform.StatusSuccess
	summaryText := "ScanDrix automated security review passed with zero critical findings."

	if aiSynthesisFailed {
		conclusion = platform.ConclusionFailure
		commitStatusState = platform.StatusFailure
		summaryText = fmt.Sprintf("⚠️ **ScanDrix AI Review Incomplete:** Deep semantic review could not be completed.\n\n%s\n\nPlease trigger a re-scan via `@scandrix review`.", aiErrorMsg)
		if len(allFindings) > 0 {
			summaryText += fmt.Sprintf("\n\nScanDrix AST and static rule checks identified %d actionable findings before failure.", len(allFindings))
		}
	} else if hasCriticalOrHigh {
		conclusion = platform.ConclusionFailure
		commitStatusState = platform.StatusFailure
		summaryText = fmt.Sprintf("ScanDrix detected %d actionable findings requiring resolution.", len(allFindings))
	}

	if diffTruncated {
		summaryText += "\n\n> ℹ️ **Notice:** PR diff size exceeded the 250KB context threshold; high blast-radius files and critical call-graph paths were prioritized."
	}

	if !settings.DryRunEnabled && adapter != nil && task.RepoNamespace != "" && task.PullNumber > 0 {
		var inlineSpecs []platform.InlineCommentSpec
		for _, f := range allFindings {
			inlineSpecs = append(inlineSpecs, platform.InlineCommentSpec{
				FilePath:  f.FilePath,
				Line:      f.EndLine,
				StartLine: f.StartLine,
				Body:      formatCommentBody(f),
			})
		}
		if len(inlineSpecs) > 0 {
			if err := adapter.PostInlineComments(ctx, task.RepoNamespace, task.PullNumber, inlineSpecs); err != nil {
				slog.Warn("Failed posting platform inline comments", "error", err)
			}
		}
		if err := adapter.PostReviewSummary(ctx, task.RepoNamespace, task.PullNumber, sanitizeMarkdownComment(summaryText), conclusion); err != nil {
			slog.Warn("Failed posting platform review summary", "error", err)
		}
		if task.HeadSHA != "" {
			_ = adapter.SetCommitStatus(ctx, task.RepoNamespace, task.HeadSHA, "scandrix/review", commitStatusState, "https://scandrix.dev", summaryText)
		}
	} else if !settings.DryRunEnabled && o.scmPublisher != nil && task.RepoNamespace != "" && task.PullNumber > 0 {
		parts := strings.Split(task.RepoNamespace, "/")
		if len(parts) == 2 {
			owner, repoName := parts[0], parts[1]

			var comments []github.ReviewCommentPayload
			var annotations []github.CheckRunAnnotation

			for _, f := range allFindings {
				comments = append(comments, github.ReviewCommentPayload{
					Path: f.FilePath,
					Line: f.EndLine,
					Side: "RIGHT",
					Body: formatCommentBody(f),
				})

				level := "warning"
				if f.Severity == models.SeverityCritical || f.Severity == models.SeverityHigh {
					level = "failure"
				} else if f.Severity == models.SeverityInfo || f.Severity == models.SeverityLow {
					level = "notice"
				}

				if len(annotations) < 50 {
					annotations = append(annotations, github.CheckRunAnnotation{
						Path:            f.FilePath,
						StartLine:       f.StartLine,
						EndLine:         f.EndLine,
						AnnotationLevel: level,
						Title:           fmt.Sprintf("[%s] %s", f.Severity, f.Title),
						Message:         f.Description,
					})
				}
			}

			event := "APPROVE"
			if hasCriticalOrHigh || aiSynthesisFailed {
				event = "REQUEST_CHANGES"
			}

			submission := github.PullReviewSubmission{
				CommitID: task.HeadSHA,
				Body:     fmt.Sprintf("## 🚀 ScanDrix Automated Code Assurance Report\n\n%s\n\n- **Total Findings:** %d\n- **Review ID:** `%s`", summaryText, len(allFindings), task.ReviewID.String()),
				Event:    event,
				Comments: comments,
			}

			if err := o.scmPublisher.SubmitPullRequestReview(ctx, owner, repoName, task.PullNumber, submission); err != nil {
				slog.Warn("Failed publishing review comments to SCM", "error", err)
			} else {
				slog.Info("Successfully published review comments to SCM", "owner", owner, "repo", repoName, "pr", task.PullNumber)
			}

			if task.CheckRunID > 0 {
				now := time.Now().UTC()
				updateReq := github.UpdateCheckRunRequest{
					Status:      "completed",
					Conclusion:  strings.ToLower(string(conclusion)),
					CompletedAt: &now,
					Output: &github.CheckRunOutput{
						Title:       fmt.Sprintf("ScanDrix Review: %s", strings.ToUpper(string(conclusion))),
						Summary:     summaryText,
						Annotations: annotations,
					},
				}
				if err := o.scmPublisher.UpdateCheckRun(ctx, owner, repoName, task.CheckRunID, updateReq); err != nil {
					slog.Warn("Failed updating SCM check run", "error", err)
				}
			}
		}
	}

	// 9. CRYPTOGRAPHIC PROVENANCE & SLSA ATTESTATIONS (In-toto DSSE signatures)

	// Step 8: Generate and sign cryptographic in-toto DSSE and SLSA v1.0 provenance attestations
	attestor := o.attestor
	var keyID string
	if attestor == nil {
		masterSecret := os.Getenv("SCANDRIX_ENCRYPTION_KEY")
		if masterSecret == "" {
			masterSecret = os.Getenv("KMS_MASTER_KEY")
		}
		privKey, pubKey, kid := intoto.DeriveTenantKeypair(task.WorkspaceID, masterSecret)
		keyID = kid
		attestor = intoto.NewProvenanceAttestor(keyID, privKey, pubKey)
	}

	decision := intoto.DecisionApproved
	if aiSynthesisFailed {
		decision = intoto.DecisionBlockedCritical
	} else if criticalCount > 0 {
		decision = intoto.DecisionBlockedCritical
	} else if highCount > 0 {
		decision = intoto.DecisionChangesRequested
	}
	pred := intoto.ReviewAttestationPredicate{
		WorkspaceID:           task.WorkspaceID,
		RepoNamespace:         task.RepoNamespace,
		CommitSHA:             task.HeadSHA,
		PullRequestNumber:     task.PullNumber,
		Decision:              decision,
		TotalFindings:         len(allFindings),
		CriticalCount:         criticalCount,
		HighCount:             highCount,
		ConsensusScore:        1.0,
		AttestedAt:            time.Now().UTC(),
		ReviewerAgentIdentity: "scandrix-orchestrator-v1",
	}

	// 8.1 In-Toto Code Review Attestation
	if env, err := attestor.AttestAndSign(pred); err != nil {
		slog.Warn("Failed generating in-toto provenance attestation", "review_id", task.ReviewID, "error", err)
	} else {
		slog.Info("Successfully generated in-toto DSSE provenance attestation", "review_id", task.ReviewID, "payload_type", env.PayloadType)
		if o.repo != nil {
			_ = o.repo.InsertReviewAttestation(ctx, intoto.AttestationRecord{
				WorkspaceID:   task.WorkspaceID,
				ReviewID:      task.ReviewID,
				PredicateType: intoto.PredicateTypeReview,
				Decision:      decision,
				KeyID:         keyID,
				Envelope:      *env,
				CreatedAt:     time.Now().UTC(),
			})
		}
		if o.artifactClient != nil {
			if envJSON, err := json.Marshal(env); err == nil {
				_, _ = o.artifactClient.UploadArtifact(ctx, "attestations", uuid.New().String(), fmt.Sprintf("attestation-intoto-%s.json", task.ReviewID.String()), bytes.NewReader(envJSON))
			}
		}
	}

	// 8.2 SLSA Provenance v1.0 Attestation
	if slsaEnv, err := attestor.AttestAndSignSLSA(pred, task.ReviewID, time.Now().UTC().Add(-1*time.Minute)); err != nil {
		slog.Warn("Failed generating SLSA provenance attestation", "review_id", task.ReviewID, "error", err)
	} else {
		slog.Info("Successfully generated SLSA v1.0 provenance attestation", "review_id", task.ReviewID, "payload_type", slsaEnv.PayloadType)
		if o.repo != nil {
			_ = o.repo.InsertReviewAttestation(ctx, intoto.AttestationRecord{
				WorkspaceID:   task.WorkspaceID,
				ReviewID:      task.ReviewID,
				PredicateType: intoto.PredicateTypeSLSA,
				Decision:      decision,
				KeyID:         keyID,
				Envelope:      *slsaEnv,
				CreatedAt:     time.Now().UTC(),
			})
		}
		if o.artifactClient != nil {
			if envJSON, err := json.Marshal(slsaEnv); err == nil {
				_, _ = o.artifactClient.UploadArtifact(ctx, "attestations", uuid.New().String(), fmt.Sprintf("attestation-slsa-%s.json", task.ReviewID.String()), bytes.NewReader(envJSON))
			}
		}
	}

	slog.Info("Review execution completed successfully",
		"review_id", task.ReviewID,
		"total_findings", len(allFindings),
	)

	return nil
}

// 10. COMMENT SANITIZATION & MARKDOWN FORMATTERS (XSS prevention & suggestions)

func sanitizeMarkdownComment(input string) string {
	// Neutralize dangerous HTML tags & event handlers in PR comments
	sanitized := strings.ReplaceAll(input, "<script", "&lt;script")
	sanitized = strings.ReplaceAll(sanitized, "</script>", "&lt;/script&gt;")
	sanitized = strings.ReplaceAll(sanitized, "<iframe", "&lt;iframe")
	sanitized = strings.ReplaceAll(sanitized, "</iframe", "&lt;/iframe")
	sanitized = strings.ReplaceAll(sanitized, "<object", "&lt;object")
	sanitized = strings.ReplaceAll(sanitized, "<embed", "&lt;embed")
	sanitized = strings.ReplaceAll(sanitized, "javascript:", "blocked-javascript:")
	sanitized = strings.ReplaceAll(sanitized, "onerror=", "data-blocked-onerror=")
	sanitized = strings.ReplaceAll(sanitized, "onload=", "data-blocked-onload=")
	return sanitized
}

func formatCommentBody(f models.CodeFinding) string {
	body := fmt.Sprintf("### 🛡️ [%s] %s\n\n%s\n\n**Remediation:**\n%s", f.Severity, f.Title, f.Description, f.Remediation)
	if f.SuggestedDiff != "" {
		body += fmt.Sprintf("\n\n```suggestion\n%s\n```", strings.TrimSpace(f.SuggestedDiff))
	}
	return sanitizeMarkdownComment(body)
}
